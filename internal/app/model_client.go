package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func checkModelRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}

	previousURL := via[len(via)-1].URL
	if !strings.EqualFold(request.URL.Host, previousURL.Host) ||
		(strings.EqualFold(previousURL.Scheme, "https") && !strings.EqualFold(request.URL.Scheme, "https")) {
		return http.ErrUseLastResponse
	}
	return nil
}

func (s *Server) generate(ctx context.Context, prompt string, settings Settings, localContext localReplyContext) (string, string, error) {
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))
	if err := validateModelSettings(settings, provider); err != nil {
		return "", "", err
	}
	if provider == "openai" || provider == "compatible" {
		text, err := s.callCompatible(ctx, prompt, settings)
		if err != nil {
			return "", "", err
		}
		return text, strings.TrimSpace(settings.Model), nil
	}
	return localReply(localContext), "local-creative", nil
}

func (s *Server) generateStreaming(ctx context.Context, prompt string, settings Settings, localContext localReplyContext, onDelta func(string) error) (string, string, error) {
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))
	if err := validateModelSettings(settings, provider); err != nil {
		return "", "", err
	}
	if provider == "openai" || provider == "compatible" {
		text, err := s.callCompatibleStreaming(ctx, prompt, settings, onDelta)
		if err != nil {
			return "", "", err
		}
		return text, strings.TrimSpace(settings.Model), nil
	}
	text := localReply(localContext)
	for start, runes := 0, []rune(text); start < len(runes); {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		end := start + 18
		if end > len(runes) {
			end = len(runes)
		}
		if err := onDelta(string(runes[start:end])); err != nil {
			return "", "", err
		}
		start = end
	}
	return text, "local-creative", nil
}

func validateModelSettings(settings Settings, provider string) error {
	switch provider {
	case "", "local":
		return nil
	case "openai", "compatible":
		if strings.TrimSpace(settings.Model) == "" {
			return fmt.Errorf("%w: 模型名称不能为空", ErrInvalidModelSettings)
		}
		if provider == "openai" && strings.TrimSpace(settings.APIKey) == "" {
			return fmt.Errorf("%w: 请填写 API Key", ErrInvalidModelSettings)
		}
		_, err := parseModelEndpoint(settings.BaseURL)
		return err
	default:
		return fmt.Errorf("%w: 不支持的服务商 %q", ErrInvalidModelSettings, settings.Provider)
	}
}

func parseModelEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("%w: 模型接口地址必须是有效的 HTTP 或 HTTPS URL", ErrInvalidModelSettings)
	}
	if endpoint.Scheme == "http" && !isLoopbackModelHost(endpoint.Hostname()) {
		return nil, fmt.Errorf("%w: 明文 HTTP 模型接口仅允许使用 localhost 或回环 IP 地址；网络服务请使用 HTTPS", ErrInvalidModelSettings)
	}
	return endpoint, nil
}

func isLoopbackModelHost(host string) bool {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.IsLoopback()
	}
	return ip.IsLoopback()
}

func writeModelError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, ErrInvalidModelSettings) {
		status = http.StatusBadRequest
	}
	writeError(w, status, err.Error())
}

func (s *Server) callCompatible(ctx context.Context, prompt string, settings Settings) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/"))
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1/chat/completions"
	} else if !strings.HasSuffix(path, "/chat/completions") {
		path += "/chat/completions"
	}
	parsed.Path = path
	body, _ := json.Marshal(map[string]any{"model": strings.TrimSpace(settings.Model), "temperature": settings.Temperature, "messages": []map[string]string{{"role": "system", "content": "你是专业的中文小说写作助手。"}, {"role": "user", "content": prompt}}})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	if apiKey := strings.TrimSpace(settings.APIKey); apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("调用模型失败: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取模型响应失败: %w", err)
	}
	responseTooLarge := int64(len(data)) > maxModelResponseBytes
	if responseTooLarge {
		data = data[:maxModelResponseBytes]
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := trimPromptText(string(data), 2000)
		if responseTooLarge {
			if detail == "" {
				detail = "错误响应超过 4 MiB，内容已截断"
			} else {
				detail += "（错误响应超过 4 MiB，内容已截断）"
			}
		}
		if detail == "" {
			return "", fmt.Errorf("模型返回 %s", response.Status)
		}
		return "", fmt.Errorf("模型返回 %s: %s", response.Status, detail)
	}
	if responseTooLarge {
		return "", fmt.Errorf("模型响应超过 4 MiB")
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("解析模型响应失败: %w", err)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("模型没有返回内容")
	}
	return strings.TrimSpace(decoded.Choices[0].Message.Content), nil
}

func (s *Server) callCompatibleStreaming(ctx context.Context, prompt string, settings Settings, onDelta func(string) error) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/"))
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1/chat/completions"
	} else if !strings.HasSuffix(path, "/chat/completions") {
		path += "/chat/completions"
	}
	parsed.Path = path
	body, err := json.Marshal(map[string]any{
		"model": strings.TrimSpace(settings.Model), "temperature": settings.Temperature, "stream": true,
		"messages": []map[string]string{{"role": "system", "content": "你是专业的中文小说写作助手。"}, {"role": "user", "content": prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("encode model request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	if apiKey := strings.TrimSpace(settings.APIKey); apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("调用模型失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, readErr := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
		if readErr != nil {
			return "", fmt.Errorf("读取模型响应失败: %w", readErr)
		}
		detail := trimPromptText(string(data), 2000)
		if detail == "" {
			return "", fmt.Errorf("模型返回 %s", response.Status)
		}
		return "", fmt.Errorf("模型返回 %s: %s", response.Status, detail)
	}

	reader := bufio.NewReader(response.Body)
	var reply strings.Builder
	var bytesRead int64
	var sawEvent bool
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		line, readErr := reader.ReadString('\n')
		bytesRead += int64(len(line))
		if bytesRead > maxModelResponseBytes {
			return "", fmt.Errorf("模型响应超过 4 MiB")
		}
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
		}
		if strings.HasPrefix(line, "data:") {
			sawEvent = true
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				if readErr != nil {
					break
				}
				continue
			}
			if payload == "[DONE]" {
				break
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				return "", fmt.Errorf("解析模型流响应失败: %w", err)
			}
			if len(chunk.Choices) > 0 {
				text := chunk.Choices[0].Delta.Content
				if text == "" {
					text = chunk.Choices[0].Message.Content
				}
				if text != "" {
					reply.WriteString(text)
					if err := onDelta(text); err != nil {
						return "", err
					}
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return "", fmt.Errorf("读取模型流响应失败: %w", readErr)
		}
	}
	if !sawEvent {
		return "", fmt.Errorf("模型没有返回流式内容")
	}
	if strings.TrimSpace(reply.String()) == "" {
		return "", fmt.Errorf("模型没有返回内容")
	}
	return strings.TrimSpace(reply.String()), nil
}
