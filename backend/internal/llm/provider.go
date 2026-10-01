package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Generator interface {
	Generate(context.Context, string, func(string)) (string, error)
}

type Provider struct {
	baseURL     string
	apiKey      string
	model       string
	client      *http.Client
	temperature float64
	maxTokens   int
}

type Options struct {
	Temperature float64
	MaxTokens   int
	Timeout     time.Duration
}

type GenerationError struct {
	message string
	cause   error
}

func (e *GenerationError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

func (e *GenerationError) UserMessage() string {
	return e.message
}

func New(baseURL, apiKey, model string, options ...Options) *Provider {
	option := Options{Temperature: 0.7, Timeout: 8 * time.Minute}
	if len(options) > 0 {
		option = options[0]
		if option.Timeout <= 0 {
			option.Timeout = 8 * time.Minute
		}
	}
	return &Provider{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		model:       model,
		client:      &http.Client{Timeout: option.Timeout},
		temperature: option.Temperature,
		maxTokens:   option.MaxTokens,
	}
}

func (p *Provider) DemoMode() bool {
	return p.apiKey == ""
}

func (p *Provider) TestConnection(ctx context.Context) error {
	if p.apiKey == "" {
		return &GenerationError{message: "请先填写 API Key"}
	}
	baseURL := p.baseURL
	baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
	if !strings.HasSuffix(baseURL, "/models") {
		baseURL += "/models"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return requestFailure(err)
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return requestFailure(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return classifiedError(response.StatusCode)
	}
	return nil
}

func (p *Provider) Generate(ctx context.Context, prompt string, emit func(string)) (string, error) {
	if p.apiKey == "" {
		text := "开发演示模式：任务已进入生成流程。请在后端配置 LLM_API_KEY、LLM_MODEL 和 LLM_BASE_URL 后启用真实模型生成。"
		emit(text)
		return text, nil
	}
	endpoint := p.baseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	requestBody := map[string]any{
		"model":       p.model,
		"stream":      true,
		"temperature": p.temperature,
		"messages": []map[string]string{{
			"role":    "system",
			"content": "你是一位严谨的中文小说创作搭档。遵守用户给定的设定，只返回可作为章节正文的修改提案，不要声称已经保存内容。",
		}, {
			"role":    "user",
			"content": prompt,
		}},
	}
	if p.maxTokens > 0 {
		requestBody["max_tokens"] = p.maxTokens
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}
	newRequest := func() (*http.Request, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "text/event-stream")
		return request, nil
	}
	request, err := newRequest()
	if err != nil {
		return "", err
	}
	var response *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		response, err = p.client.Do(request)
		if err == nil && !retryableStatus(response.StatusCode) {
			break
		}
		if response != nil {
			response.Body.Close()
		}
		if attempt == 2 {
			if err != nil {
				return "", requestFailure(err)
			}
			return "", classifiedError(response.StatusCode)
		}
		if err := waitForRetry(ctx, time.Duration(1<<attempt)*250*time.Millisecond); err != nil {
			return "", requestFailure(err)
		}
		request, err = newRequest()
		if err != nil {
			return "", err
		}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", classifiedError(response.StatusCode)
	}

	var output strings.Builder
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return output.String(), nil
		}
		var chunk struct {
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return output.String(), &GenerationError{message: "模型服务返回错误，请检查模型配置后重试"}
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		output.WriteString(delta)
		emit(delta)
	}
	if err := scanner.Err(); err != nil {
		return output.String(), requestFailure(err)
	}
	return output.String(), nil
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func requestFailure(err error) error {
	message := "连接模型服务失败，请检查网络后重试"
	if errors.Is(err, context.DeadlineExceeded) {
		message = "模型请求超时，请稍后重试"
	}
	return &GenerationError{message: message, cause: err}
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}
func classifiedError(status int) error {
	switch status {
	case 401, 403:
		return &GenerationError{message: "模型鉴权失败，请检查 API Key"}
	case 429:
		return &GenerationError{message: "模型服务限流，请稍后重试"}
	case 408:
		return &GenerationError{message: "模型服务超时，请稍后重试"}
	default:
		if status >= 500 {
			return &GenerationError{message: "模型服务暂时不可用，请稍后重试"}
		}
		return &GenerationError{message: "模型请求失败（HTTP " + strconv.Itoa(status) + "），请检查模型配置"}
	}
}
