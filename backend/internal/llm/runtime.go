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
	BaseURL              string
	APIKey               string
	Model                string
	Temperature          float64
	MaxTokens            int
	TimeoutSec           int
	InputCostPerMillion  float64
	OutputCostPerMillion float64
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
	BaseURL              string  `json:"baseUrl"`
	Model                string  `json:"model"`
	Temperature          float64 `json:"temperature"`
	MaxTokens            int     `json:"maxTokens"`
	TimeoutSec           int     `json:"timeoutSec"`
	InputCostPerMillion  float64 `json:"inputCostPerMillion"`
	OutputCostPerMillion float64 `json:"outputCostPerMillion"`
	APIKeyConfigured     bool    `json:"apiKeyConfigured"`
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
	catalog  *ProviderCatalog
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

// NewRuntimeWithCatalog creates a runtime backed by persistent provider
// profiles. NewRuntime remains available for callers that use the legacy
// single-provider settings flow.
func NewRuntimeWithCatalog(catalog *ProviderCatalog, envPath string) *Runtime {
	if envPath == "" {
		envPath = ".env"
	}
	settings, _ := catalog.Settings("")
	return &Runtime{
		settings: settings,
		provider: newRuntimeProvider(settings),
		catalog:  catalog,
		envPath:  envPath,
	}
}

func (r *Runtime) Generate(ctx context.Context, prompt string, emit func(string)) (string, error) {
	text, err, _ := r.GenerateWithMode(ctx, prompt, emit)
	return text, err
}

func (r *Runtime) GenerateWithMode(ctx context.Context, prompt string, emit func(string)) (string, error, bool) {
	return r.GenerateWithModeFor(ctx, "", prompt, emit)
}

func (r *Runtime) GenerateWithProvider(ctx context.Context, providerID, prompt string, emit func(string)) (string, error) {
	text, err, _ := r.GenerateWithModeFor(ctx, providerID, prompt, emit)
	return text, err
}

func (r *Runtime) GenerateWithModeFor(ctx context.Context, providerID, prompt string, emit func(string)) (string, error, bool) {
	text, _, err, demoMode := r.GenerateWithUsageFor(ctx, providerID, prompt, emit)
	return text, err, demoMode
}

func (r *Runtime) GenerateWithUsageFor(ctx context.Context, providerID, prompt string, emit func(string)) (string, GenerationUsage, error, bool) {
	if r.catalog != nil {
		provider, settings, exists := r.catalog.Provider(providerID)
		if !exists {
			return "", GenerationUsage{}, &SettingsValidationError{message: "所选模型服务商不存在"}, true
		}
		text, usage, err := provider.GenerateWithUsage(ctx, prompt, emit)
		return text, usage, err, settings.APIKey == ""
	}
	if providerID != "" {
		return "", GenerationUsage{}, &SettingsValidationError{message: "当前生成器不支持所选模型服务商"}, true
	}
	r.mu.RLock()
	provider := r.provider
	demoMode := r.settings.APIKey == ""
	r.mu.RUnlock()
	text, usage, err := provider.GenerateWithUsage(ctx, prompt, emit)
	return text, usage, err, demoMode
}

func (r *Runtime) DemoMode() bool {
	return r.DemoModeFor("")
}

func (r *Runtime) DemoModeFor(providerID string) bool {
	if r.catalog != nil {
		_, settings, exists := r.catalog.Provider(providerID)
		return !exists || settings.APIKey == ""
	}
	if providerID != "" {
		return true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.settings.APIKey == ""
}

func (r *Runtime) ResolveProviderID(providerID string) (string, bool) {
	if r.catalog != nil {
		if providerID == "" {
			providerID = r.catalog.DefaultID()
		}
		return providerID, r.catalog.Has(providerID)
	}
	return "", providerID == ""
}

func (r *Runtime) ProviderCatalog() *ProviderCatalog {
	return r.catalog
}

func (r *Runtime) Settings() SettingsView {
	if r.catalog != nil {
		settings, _ := r.catalog.Settings("")
		return settingsView(settings)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return settingsView(r.settings)
}

func (r *Runtime) Update(input SettingsInput) (SettingsView, error) {
	if r.catalog != nil {
		id := r.catalog.DefaultID()
		var current ProviderProfileView
		for _, profile := range r.catalog.List() {
			if profile.ID == id {
				current = profile
				break
			}
		}
		settings, ok := r.catalog.Settings(id)
		if !ok {
			return SettingsView{}, &SettingsValidationError{message: "默认模型服务商不可用"}
		}
		next, err := mergeSettings(settings, input)
		if err != nil {
			return SettingsView{}, err
		}
		_, err = r.catalog.Save(id, providerProfileInput(current.Name, next))
		if err != nil {
			return SettingsView{}, err
		}
		return settingsView(next), nil
	}
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
	if r.catalog != nil {
		return r.catalog.TestConnection(ctx, r.catalog.DefaultID(), ProviderProfileInput{
			BaseURL:     input.BaseURL,
			Model:       input.Model,
			APIKey:      input.APIKey,
			ClearAPIKey: input.ClearAPIKey,
			Temperature: input.Temperature,
			MaxTokens:   input.MaxTokens,
			TimeoutSec:  input.TimeoutSec,
		})
	}
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
		BaseURL:              settings.BaseURL,
		Model:                settings.Model,
		Temperature:          settings.Temperature,
		MaxTokens:            settings.MaxTokens,
		TimeoutSec:           settings.TimeoutSec,
		InputCostPerMillion:  settings.InputCostPerMillion,
		OutputCostPerMillion: settings.OutputCostPerMillion,
		APIKeyConfigured:     settings.APIKey != "",
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
	if math.IsNaN(settings.InputCostPerMillion) || math.IsInf(settings.InputCostPerMillion, 0) ||
		settings.InputCostPerMillion < 0 || settings.InputCostPerMillion > 1_000_000 ||
		math.IsNaN(settings.OutputCostPerMillion) || math.IsInf(settings.OutputCostPerMillion, 0) ||
		settings.OutputCostPerMillion < 0 || settings.OutputCostPerMillion > 1_000_000 {
		return &SettingsValidationError{message: "每百万 Token 单价必须在 0 到 1000000 美元之间"}
	}
	return nil
}

func newRuntimeProvider(settings RuntimeSettings) *Provider {
	timeout := time.Duration(settings.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Minute
	}
	return New(settings.BaseURL, settings.APIKey, settings.Model, Options{
		Temperature:          settings.Temperature,
		MaxTokens:            settings.MaxTokens,
		Timeout:              timeout,
		InputCostPerMillion:  settings.InputCostPerMillion,
		OutputCostPerMillion: settings.OutputCostPerMillion,
	})
}
