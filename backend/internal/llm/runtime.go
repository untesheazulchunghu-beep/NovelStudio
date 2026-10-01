package llm

import (
	"context"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"

	"novelstudio/internal/config"
)

type RuntimeSettings struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	MaxTokens   int
	TimeoutSec  int
}

type SettingsInput struct {
	BaseURL     string  `json:"baseUrl"`
	Model       string  `json:"model"`
	APIKey      string  `json:"apiKey"`
	ClearAPIKey bool    `json:"clearApiKey"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"maxTokens"`
	TimeoutSec  int     `json:"timeoutSec"`
}

type SettingsView struct {
	BaseURL          string  `json:"baseUrl"`
	Model            string  `json:"model"`
	Temperature      float64 `json:"temperature"`
	MaxTokens        int     `json:"maxTokens"`
	TimeoutSec       int     `json:"timeoutSec"`
	APIKeyConfigured bool    `json:"apiKeyConfigured"`
}

type SettingsValidationError struct {
	message string
}

func (e *SettingsValidationError) Error() string {
	return e.message
}

type Runtime struct {
	mu       sync.RWMutex
	settings RuntimeSettings
	provider *Provider
	envPath  string
}

func NewRuntime(settings RuntimeSettings, envPath string) *Runtime {
	if envPath == "" {
		envPath = ".env"
	}
	return &Runtime{
		settings: settings,
		provider: newRuntimeProvider(settings),
		envPath:  envPath,
	}
}

func (r *Runtime) Generate(ctx context.Context, prompt string, emit func(string)) (string, error) {
	text, err, _ := r.GenerateWithMode(ctx, prompt, emit)
	return text, err
}

func (r *Runtime) GenerateWithMode(ctx context.Context, prompt string, emit func(string)) (string, error, bool) {
	r.mu.RLock()
	provider := r.provider
	demoMode := r.settings.APIKey == ""
	r.mu.RUnlock()
	text, err := provider.Generate(ctx, prompt, emit)
	return text, err, demoMode
}

func (r *Runtime) DemoMode() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.settings.APIKey == ""
}

func (r *Runtime) Settings() SettingsView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return settingsView(r.settings)
}

func (r *Runtime) Update(input SettingsInput) (SettingsView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next, err := mergeSettings(r.settings, input)
	if err != nil {
		return SettingsView{}, err
	}
	if err := config.SaveLLMValues(r.envPath, config.LLMValues{
		BaseURL:     next.BaseURL,
		APIKey:      next.APIKey,
		Model:       next.Model,
		Temperature: next.Temperature,
		MaxTokens:   next.MaxTokens,
		TimeoutSec:  next.TimeoutSec,
	}); err != nil {
		return SettingsView{}, err
	}
	r.settings = next
	r.provider = newRuntimeProvider(next)
	return settingsView(next), nil
}

func (r *Runtime) TestConnection(ctx context.Context, input SettingsInput) error {
	r.mu.RLock()
	next, err := mergeSettings(r.settings, input)
	r.mu.RUnlock()
	if err != nil {
		return err
	}
	if next.APIKey == "" {
		return &SettingsValidationError{message: "请先填写 API Key"}
	}
	provider := New(next.BaseURL, next.APIKey, next.Model, Options{
		Temperature: next.Temperature,
		MaxTokens:   next.MaxTokens,
		Timeout:     15 * time.Second,
	})
	return provider.TestConnection(ctx)
}

func settingsView(settings RuntimeSettings) SettingsView {
	return SettingsView{
		BaseURL:          settings.BaseURL,
		Model:            settings.Model,
		Temperature:      settings.Temperature,
		MaxTokens:        settings.MaxTokens,
		TimeoutSec:       settings.TimeoutSec,
		APIKeyConfigured: settings.APIKey != "",
	}
}

func mergeSettings(current RuntimeSettings, input SettingsInput) (RuntimeSettings, error) {
	next := current
	next.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	next.Model = strings.TrimSpace(input.Model)
	next.Temperature = input.Temperature
	next.MaxTokens = input.MaxTokens
	next.TimeoutSec = input.TimeoutSec
	if input.ClearAPIKey {
		if strings.TrimSpace(input.APIKey) != "" {
			return RuntimeSettings{}, &SettingsValidationError{message: "清除密钥时不能同时填写新密钥"}
		}
		next.APIKey = ""
	} else if strings.TrimSpace(input.APIKey) != "" {
		next.APIKey = strings.TrimSpace(input.APIKey)
	}
	if err := validateRuntimeSettings(next); err != nil {
		return RuntimeSettings{}, err
	}
	return next, nil
}

func validateRuntimeSettings(settings RuntimeSettings) error {
	parsed, err := url.Parse(settings.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return &SettingsValidationError{message: "Base URL 必须是有效的 http 或 https 地址"}
	}
	if settings.Model == "" || len(settings.Model) > 200 || strings.ContainsAny(settings.Model, "\r\n\x00") {
		return &SettingsValidationError{message: "请填写有效的模型名称"}
	}
	if len(settings.APIKey) > 4096 || strings.ContainsAny(settings.APIKey, "\r\n\x00") {
		return &SettingsValidationError{message: "API Key 格式无效"}
	}
	if math.IsNaN(settings.Temperature) || math.IsInf(settings.Temperature, 0) ||
		settings.Temperature < 0 || settings.Temperature > 2 {
		return &SettingsValidationError{message: "Temperature 必须在 0 到 2 之间"}
	}
	if settings.MaxTokens < 0 || settings.MaxTokens > 1_000_000 {
		return &SettingsValidationError{message: "最大输出 Token 必须在 0 到 1000000 之间"}
	}
	if settings.TimeoutSec < 1 || settings.TimeoutSec > 1800 {
		return &SettingsValidationError{message: "请求超时必须在 1 到 1800 秒之间"}
	}
	return nil
}

func newRuntimeProvider(settings RuntimeSettings) *Provider {
	timeout := time.Duration(settings.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Minute
	}
	return New(settings.BaseURL, settings.APIKey, settings.Model, Options{
		Temperature: settings.Temperature,
		MaxTokens:   settings.MaxTokens,
		Timeout:     timeout,
	})
}
