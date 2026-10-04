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
	"sort"
	"strconv"
	"strings"
	"time"
)

type Generator interface {
	Generate(context.Context, string, func(string)) (string, error)
}

type Provider struct {
	baseURL              string
	apiKey               string
	model                string
	client               *http.Client
	temperature          float64
	maxTokens            int
	inputCostPerMillion  float64
	outputCostPerMillion float64
}

type Options struct {
	Temperature          float64
	MaxTokens            int
	Timeout              time.Duration
	InputCostPerMillion  float64
	OutputCostPerMillion float64
}

type GenerationUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Estimated        bool
	Model            string
	CostUSD          float64
	CostKnown        bool
}

type AvailableModel struct {
	ID string `json:"id"`
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
		baseURL:              strings.TrimRight(baseURL, "/"),
		apiKey:               apiKey,
		model:                model,
		client:               &http.Client{Timeout: option.Timeout},
		temperature:          option.Temperature,
		maxTokens:            option.MaxTokens,
		inputCostPerMillion:  option.InputCostPerMillion,
		outputCostPerMillion: option.OutputCostPerMillion,
	}
}

func (p *Provider) DemoMode() bool {
	return p.apiKey == ""
}

func (p *Provider) TestConnection(ctx context.Context) error {
	_, err := p.ListModels(ctx)
	return err
}

func (p *Provider) ListModels(ctx context.Context) ([]AvailableModel, error) {
	if p.apiKey == "" {
		return nil, &GenerationError{message: "请先填写 API Key"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL(p.baseURL), nil)
	if err != nil {
		return nil, requestFailure(err)
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, requestFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
		return nil, classifiedError(response.StatusCode)
	}
	var payload struct {
		Data []AvailableModel `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return []AvailableModel{}, nil
		}
		return nil, requestFailure(err)
	}
	models := make([]AvailableModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, model := range payload.Data {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		if _, exists := seen[model.ID]; exists {
			continue
		}
		seen[model.ID] = struct{}{}
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func modelsURL(baseURL string) string {
	baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
	if !strings.HasSuffix(baseURL, "/models") {
		baseURL += "/models"
	}
	return baseURL
}

func (p *Provider) Generate(ctx context.Context, prompt string, emit func(string)) (string, error) {
	text, _, err := p.GenerateWithUsage(ctx, prompt, emit)
	return text, err
}

func (p *Provider) GenerateWithUsage(ctx context.Context, prompt string, emit func(string)) (string, GenerationUsage, error) {
	if p.apiKey == "" {
		text := "开发演示模式：任务已进入生成流程。请在后端配置 LLM_API_KEY、LLM_MODEL 和 LLM_BASE_URL 后启用真实模型生成。"
		if strings.Contains(prompt, "工作流节点（review）") {
			text = "审稿结论：通过"
		}
		emit(text)
		return text, GenerationUsage{Model: p.model}, nil
	}
	endpoint := p.baseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	system := "你是一位严谨的中文小说创作搭档。遵守用户给定的设定，只返回可作为章节正文的修改提案，不要声称已经保存内容。"
	switch {
	case strings.Contains(prompt, "请只输出章节记忆"):
		system = "你是一位严谨的中文小说记忆整理助手。不要创作或重写章节正文。仅根据用户给出的当前章节内容提取长期记忆；严格按用户指定的摘要、出场人物、新增设定、伏笔变化格式输出，缺少内容时留空。"
	case strings.Contains(prompt, "审稿任务："):
		system = "你是一位严谨的中文小说审稿人。逐条核对当前正文与关键事件清单，严格按用户指定格式输出判断、逐字引文和位置。引文必须是正文中的连续原文；不要生成或修改章节正文。"
	case strings.Contains(prompt, "连续性审查任务："):
		system = "你是一位谨慎的长篇小说连续性审稿人。只根据当前正文、已定稿章节记忆和相关设定资料提示可能的时间线、人物状态或设定冲突；严格按用户格式给出原文引文和资料来源。证据不足时降低把握程度，不要臆造设定，不要输出或改写正文。"
	case strings.Contains(prompt, "工作流节点（review）"):
		system = "你是一位严谨的中文小说审稿人。只按用户要求检查此前输出并给出审稿结论，不要输出或修改章节正文。"
	}
	requestBody := map[string]any{
		"model":  p.model,
		"stream": true,
		"stream_options": map[string]any{
			"include_usage": true,
		},
		"temperature": p.temperature,
		"messages": []map[string]string{{
			"role":    "system",
			"content": system,
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
		return "", GenerationUsage{Model: p.model}, err
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
		return "", GenerationUsage{Model: p.model}, err
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
				return "", GenerationUsage{Model: p.model}, requestFailure(err)
			}
			return "", GenerationUsage{Model: p.model}, classifiedError(response.StatusCode)
		}
		if err := waitForRetry(ctx, time.Duration(1<<attempt)*250*time.Millisecond); err != nil {
			return "", GenerationUsage{Model: p.model}, requestFailure(err)
		}
		request, err = newRequest()
		if err != nil {
			return "", GenerationUsage{Model: p.model}, err
		}
	}
	if response.StatusCode == http.StatusBadRequest {
		failureBody, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		_ = response.Body.Close()
		failureText := strings.ToLower(string(failureBody))
		if strings.Contains(failureText, "stream_options") || strings.Contains(failureText, "include_usage") {
			delete(requestBody, "stream_options")
			body, err = json.Marshal(requestBody)
			if err != nil {
				return "", GenerationUsage{Model: p.model}, err
			}
			request, err = newRequest()
			if err != nil {
				return "", GenerationUsage{Model: p.model}, err
			}
			response, err = p.client.Do(request)
			if err != nil {
				return "", GenerationUsage{Model: p.model}, requestFailure(err)
			}
		}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", GenerationUsage{Model: p.model}, classifiedError(response.StatusCode)
	}

	var output strings.Builder
	usage := GenerationUsage{Model: p.model}
	usageReported := false
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return output.String(), p.completeUsage(usage, prompt, output.String(), usageReported), nil
		}
		var chunk struct {
			Error json.RawMessage `json:"error"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			usageReported = true
			usage.PromptTokens = max(0, chunk.Usage.PromptTokens)
			usage.CompletionTokens = max(0, chunk.Usage.CompletionTokens)
			usage.TotalTokens = max(0, chunk.Usage.TotalTokens)
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return output.String(), p.completeUsage(usage, prompt, output.String(), usageReported), &GenerationError{message: "模型服务返回错误，请检查模型配置后重试"}
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		output.WriteString(delta)
		emit(delta)
	}
	if err := scanner.Err(); err != nil {
		return output.String(), p.completeUsage(usage, prompt, output.String(), usageReported), requestFailure(err)
	}
	return output.String(), p.completeUsage(usage, prompt, output.String(), usageReported), nil
}

func (p *Provider) completeUsage(usage GenerationUsage, prompt, output string, usageReported bool) GenerationUsage {
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	if !usageReported || usage.TotalTokens == 0 {
		usage = EstimateUsage(prompt, output, usage.Model)
	} else {
		usage.Estimated = false
	}
	if p.inputCostPerMillion > 0 || p.outputCostPerMillion > 0 {
		usage.CostKnown = true
		usage.CostUSD = float64(usage.PromptTokens)*p.inputCostPerMillion/1_000_000 +
			float64(usage.CompletionTokens)*p.outputCostPerMillion/1_000_000
	}
	return usage
}

// EstimateUsage provides a conservative fallback for generators that do not
// return token counts. It counts non-ASCII runes individually and estimates
// ASCII text at roughly one token per four characters.
func EstimateUsage(prompt, output, model string) GenerationUsage {
	promptTokens := estimateTokenCount(prompt)
	completionTokens := estimateTokenCount(output)
	totalTokens := promptTokens + completionTokens
	return GenerationUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Estimated:        totalTokens > 0,
		Model:            model,
	}
}

// estimateTokenCount is a fallback for OpenAI-compatible servers that omit
// stream usage data. It counts non-ASCII runes individually and estimates
// ASCII text at roughly one token per four characters.
func estimateTokenCount(text string) int {
	if text == "" {
		return 0
	}
	asciiRunes := 0
	nonASCIITokens := 0
	flushASCII := func() {
		if asciiRunes > 0 {
			nonASCIITokens += (asciiRunes + 3) / 4
			asciiRunes = 0
		}
	}
	for _, value := range text {
		if value <= 0x7f {
			asciiRunes++
			continue
		}
		flushASCII()
		nonASCIITokens++
	}
	flushASCII()
	return nonASCIITokens
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
