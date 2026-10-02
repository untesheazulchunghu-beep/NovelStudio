package config

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr               string
	DatabasePath       string
	SessionSecret      []byte
	SessionSecure      bool
	AllowedOrigins     []string
	StaticDir          string
	LLMBaseURL         string
	LLMAPIKey          string
	LLMModel           string
	LLMTemperature     float64
	LLMMaxTokens       int
	LLMTimeoutSec      int
	ContextBudgetRunes int
}

type LLMValues struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	MaxTokens   int
	TimeoutSec  int
}

func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, err
	}
	secret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	production := strings.EqualFold(os.Getenv("APP_ENV"), "production")
	if secret == "" && production {
		return Config{}, fmt.Errorf("SESSION_SECRET must be set when APP_ENV=production")
	}
	if secret == "" {
		buffer := make([]byte, 32)
		if _, err := rand.Read(buffer); err != nil {
			return Config{}, fmt.Errorf("generate session secret: %w", err)
		}
		secret = hex.EncodeToString(buffer)
		if err := os.Setenv("SESSION_SECRET", secret); err != nil {
			return Config{}, fmt.Errorf("persist process session secret: %w", err)
		}
	}

	origins := strings.Split(env("ALLOWED_ORIGINS", "localhost:5173,127.0.0.1:5173"), ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}

	temperature := 0.7
	if value := strings.TrimSpace(os.Getenv("LLM_TEMPERATURE")); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed >= 0 && parsed <= 2 {
			temperature = parsed
		}
	}
	maxTokens := 0
	if value := strings.TrimSpace(os.Getenv("LLM_MAX_TOKENS")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			maxTokens = parsed
		}
	}
	timeout := 480
	if value := strings.TrimSpace(os.Getenv("LLM_TIMEOUT")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	return Config{
		Addr:               env("APP_ADDR", "127.0.0.1:8080"),
		DatabasePath:       env("DB_PATH", "data/novel-studio.db"),
		SessionSecret:      []byte(secret),
		SessionSecure:      os.Getenv("SESSION_SECURE") == "true" || production,
		AllowedOrigins:     origins,
		StaticDir:          env("STATIC_DIR", "frontend/dist"),
		LLMBaseURL:         env("LLM_BASE_URL", "https://api.openai.com/v1"),
		LLMAPIKey:          strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		LLMModel:           env("LLM_MODEL", "gpt-4o-mini"),
		LLMTemperature:     temperature,
		LLMMaxTokens:       maxTokens,
		LLMTimeoutSec:      timeout,
		ContextBudgetRunes: positiveIntEnv("CONTEXT_BUDGET_RUNES", defaultContextBudgetRunes),
	}, nil
}

const defaultContextBudgetRunes = 12000

func positiveIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func SaveLLMValues(path string, values LLMValues) error {
	updates := []struct {
		key   string
		value string
	}{
		{"LLM_BASE_URL", values.BaseURL},
		{"LLM_API_KEY", values.APIKey},
		{"LLM_MODEL", values.Model},
		{"LLM_TEMPERATURE", strconv.FormatFloat(values.Temperature, 'f', -1, 64)},
		{"LLM_MAX_TOKENS", strconv.Itoa(values.MaxTokens)},
		{"LLM_TIMEOUT", strconv.Itoa(values.TimeoutSec)},
	}
	for _, update := range updates {
		if strings.ContainsAny(update.value, "\r\n\x00") {
			return fmt.Errorf("%s contains an unsupported line break", update.key)
		}
	}

	contents, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read local environment file: %w", err)
	}
	lines := make([]string, 0)
	if len(contents) > 0 {
		lines = strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n")
	}
	updated := make(map[string]bool, len(updates))
	hasSessionSecret := false
	for index, line := range lines {
		key, _, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export ")), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "SESSION_SECRET" {
			hasSessionSecret = true
		}
		for _, update := range updates {
			if key == update.key {
				lines[index] = update.key + "=" + strconv.Quote(update.value)
				updated[key] = true
				break
			}
		}
	}
	for _, update := range updates {
		if !updated[update.key] {
			lines = append(lines, update.key+"="+strconv.Quote(update.value))
		}
	}
	if !hasSessionSecret {
		if sessionSecret := os.Getenv("SESSION_SECRET"); sessionSecret != "" {
			lines = append(lines, "SESSION_SECRET="+strconv.Quote(sessionSecret))
		}
	}
	output := strings.Join(lines, "\n") + "\n"

	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".novel-studio-env-*")
	if err != nil {
		return fmt.Errorf("create local environment update: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect local environment file: %w", err)
	}
	if _, err := temporary.WriteString(output); err != nil {
		temporary.Close()
		return fmt.Errorf("write local environment update: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("flush local environment update: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close local environment update: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace local environment file: %w", err)
	}
	for _, update := range updates {
		if err := os.Setenv(update.key, update.value); err != nil {
			return fmt.Errorf("apply %s to running process: %w", update.key, err)
		}
	}
	return nil
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\uFEFF"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, rawValue, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !validEnvKey(key) {
			return fmt.Errorf("%s:%d: expected a valid KEY=value entry", path, lineNumber)
		}
		value, err := parseDotEnvValue(strings.TrimSpace(rawValue))
		if err != nil {
			return fmt.Errorf("%s:%d: invalid value for %s", path, lineNumber, key)
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("%s:%d: set %s: %w", path, lineNumber, key, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for index, char := range key {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' ||
			index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func parseDotEnvValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] == '"' || value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != value[0] {
			return "", fmt.Errorf("unclosed quote")
		}
		if value[0] == '"' {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				return "", err
			}
			return unquoted, nil
		}
		return value[1 : len(value)-1], nil
	}
	for index, char := range value {
		if char == '#' && index > 0 && (value[index-1] == ' ' || value[index-1] == '\t') {
			return strings.TrimSpace(value[:index]), nil
		}
	}
	return value, nil
}
