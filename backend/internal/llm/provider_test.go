package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProviderRetriesTransientStatusAndStreamsResponse(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("unexpected authorization header: %q", got)
		}
		if attempts.Add(1) == 1 {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	provider := New(server.URL, "secret", "test-model")
	var streamed strings.Builder
	output, err := provider.Generate(context.Background(), "write", func(delta string) {
		streamed.WriteString(delta)
	})
	if err != nil {
		t.Fatalf("Generate returned an unexpected error: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected one retry, got %d requests", attempts.Load())
	}
	if output != "你好" || streamed.String() != output {
		t.Fatalf("unexpected streamed output: returned=%q streamed=%q", output, streamed.String())
	}
}

func TestProviderClassifiesUpstreamStatus(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		userMessage string
		attempts    int32
	}{
		{name: "authentication", status: http.StatusUnauthorized, userMessage: "模型鉴权失败，请检查 API Key", attempts: 1},
		{name: "forbidden", status: http.StatusForbidden, userMessage: "模型鉴权失败，请检查 API Key", attempts: 1},
		{name: "rate limit", status: http.StatusTooManyRequests, userMessage: "模型服务限流，请稍后重试", attempts: 3},
		{name: "timeout status", status: http.StatusRequestTimeout, userMessage: "模型服务超时，请稍后重试", attempts: 3},
		{name: "upstream failure", status: http.StatusBadGateway, userMessage: "模型服务暂时不可用，请稍后重试", attempts: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.WriteHeader(test.status)
			}))
			defer server.Close()

			_, err := New(server.URL, "secret", "test-model").Generate(context.Background(), "write", func(string) {})
			if err == nil {
				t.Fatal("expected a classified error")
			}
			var userFacing interface{ UserMessage() string }
			if !errors.As(err, &userFacing) {
				t.Fatalf("expected a user-facing error, got %T: %v", err, err)
			}
			if userFacing.UserMessage() != test.userMessage {
				t.Fatalf("unexpected user message: %q", userFacing.UserMessage())
			}
			if attempts.Load() != test.attempts {
				t.Fatalf("expected %d requests, got %d", test.attempts, attempts.Load())
			}
		})
	}
}

func TestProviderDoesNotExposeServerSentErrorDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"error\":\"private upstream detail\"}\n\n")
	}))
	defer server.Close()

	_, err := New(server.URL, "secret", "test-model").Generate(context.Background(), "write", func(string) {})
	if err == nil {
		t.Fatal("expected the stream error to fail generation")
	}
	var userFacing interface{ UserMessage() string }
	if !errors.As(err, &userFacing) {
		t.Fatalf("expected a user-facing stream error, got %T: %v", err, err)
	}
	if strings.Contains(userFacing.UserMessage(), "private upstream detail") {
		t.Fatalf("user-facing message leaked upstream details: %q", userFacing.UserMessage())
	}
}

func TestProviderUsesTaskSpecificSystemPrompts(t *testing.T) {
	tests := []struct {
		name          string
		prompt        string
		systemMust    string
		systemMustNot string
	}{
		{
			name:          "chapter memory",
			prompt:        "请只输出章节记忆\n第1章\n正文\n摘要：",
			systemMust:    "仅根据用户给出的当前章节内容提取长期记忆",
			systemMustNot: "只返回可作为章节正文的修改提案",
		},
		{
			name:          "key-event review",
			prompt:        "审稿任务：\n逐条核对关键事件和正文",
			systemMust:    "逐条核对当前正文与关键事件清单",
			systemMustNot: "只返回可作为章节正文的修改提案",
		},
		{
			name:          "workflow review",
			prompt:        "工作流节点（review）：\n审稿结论：通过",
			systemMust:    "给出审稿结论",
			systemMustNot: "只返回可作为章节正文的修改提案",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
					return
				}
				if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[1].Role != "user" {
					t.Errorf("unexpected chat messages: %#v", body.Messages)
					return
				}
				if !strings.Contains(body.Messages[0].Content, test.systemMust) || strings.Contains(body.Messages[0].Content, test.systemMustNot) {
					t.Errorf("wrong system prompt: %q", body.Messages[0].Content)
				}
				if body.Messages[1].Content != test.prompt {
					t.Errorf("user prompt = %q, want %q", body.Messages[1].Content, test.prompt)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()

			if _, err := New(server.URL, "secret", "test-model").Generate(context.Background(), test.prompt, func(string) {}); err != nil {
				t.Fatalf("Generate returned an unexpected error: %v", err)
			}
		})
	}
}

func TestProviderRetryWaitCanBeCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	started := time.Now()
	_, err := New(server.URL, "secret", "test-model").Generate(ctx, "write", func(string) {})
	if err == nil {
		t.Fatal("expected canceled retry to fail")
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("retry did not stop promptly after cancellation: %s", elapsed)
	}
}
