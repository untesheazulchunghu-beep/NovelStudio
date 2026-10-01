package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReadsLocalEnvFileAndKeepsProcessEnvironment(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, key := range []string{"APP_ADDR", "APP_ENV", "SESSION_SECRET", "STATIC_DIR", "DB_PATH"} {
		unsetEnvForTest(t, key)
	}
	t.Setenv("NOVELSTUDIO_DOTENV_PRECEDENCE", "from-process")
	if err := os.WriteFile(".env", []byte(
		"APP_ADDR=127.0.0.1:9090\n"+
			"APP_ENV=development\n"+
			"SESSION_SECRET=stable-local-secret\n"+
			"NOVELSTUDIO_DOTENV_PRECEDENCE=from-file\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}

	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Addr != "127.0.0.1:9090" {
		t.Fatalf("expected APP_ADDR from .env, got %q", settings.Addr)
	}
	if string(settings.SessionSecret) != "stable-local-secret" {
		t.Fatalf("expected SESSION_SECRET from .env, got %q", settings.SessionSecret)
	}
	if settings.StaticDir != "frontend/dist" {
		t.Fatalf("expected package-relative static directory, got %q", settings.StaticDir)
	}
	if got := os.Getenv("NOVELSTUDIO_DOTENV_PRECEDENCE"); got != "from-process" {
		t.Fatalf("process environment should take precedence over .env, got %q", got)
	}
}

func TestLoadDefaultsToLoopbackForLocalUse(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, key := range []string{"APP_ADDR", "APP_ENV", "SESSION_SECRET", "STATIC_DIR"} {
		unsetEnvForTest(t, key)
	}

	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Addr != "127.0.0.1:8080" {
		t.Fatalf("local default should bind only to loopback, got %q", settings.Addr)
	}
	if os.Getenv("SESSION_SECRET") == "" {
		t.Fatal("generated session secret should remain available for local UI settings persistence")
	}
}

func TestLoadDotEnvParsesQuotesExportsAndComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	key := "NOVELSTUDIO_DOTENV_PARSE"
	unsetEnvForTest(t, key)
	unsetEnvForTest(t, "NOVELSTUDIO_DOTENV_COMMENT")
	if err := os.WriteFile(path, []byte(
		"\uFEFF# ignored\n"+
			"export "+key+"='value=with # symbols'\n"+
			"NOVELSTUDIO_DOTENV_COMMENT=plain-value # trailing comment\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(key); got != "value=with # symbols" {
		t.Fatalf("quoted values should preserve equals and comment characters, got %q", got)
	}
	if got := os.Getenv("NOVELSTUDIO_DOTENV_COMMENT"); got != "plain-value" {
		t.Fatalf("unquoted trailing comments should be ignored, got %q", got)
	}
}

func TestLoadRejectsMalformedDotEnvWithLineNumber(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnvForTest(t, "APP_ENV")
	if err := os.WriteFile(".env", []byte("# comment\nnot-an-assignment\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), ".env:2") {
		t.Fatalf("expected a line-numbered .env error, got %v", err)
	}
}

func TestSaveLLMValuesPreservesOtherEnvironmentAndReloadsSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	t.Setenv("SESSION_SECRET", "persistent-session-secret")
	for _, key := range []string{"LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL", "LLM_TEMPERATURE", "LLM_MAX_TOKENS", "LLM_TIMEOUT"} {
		unsetEnvForTest(t, key)
	}
	if err := os.WriteFile(path, []byte("# keep this comment\nAPP_ADDR=127.0.0.1:8080\nLLM_MODEL=old-model\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	values := LLMValues{
		BaseURL:     "https://local.example/v1",
		APIKey:      "secret=with # symbols",
		Model:       "novel-model",
		Temperature: 0.4,
		MaxTokens:   2048,
		TimeoutSec:  120,
	}
	if err := SaveLLMValues(path, values); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "# keep this comment") ||
		!strings.Contains(string(contents), "APP_ADDR=127.0.0.1:8080") ||
		!strings.Contains(string(contents), "SESSION_SECRET=\"persistent-session-secret\"") {
		t.Fatalf("saving model settings should preserve unrelated .env entries: %s", contents)
	}

	for _, key := range []string{"LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL", "LLM_TEMPERATURE", "LLM_MAX_TOKENS", "LLM_TIMEOUT"} {
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("LLM_API_KEY"); got != values.APIKey {
		t.Fatalf("API key did not survive a .env round trip: %q", got)
	}
	if got := os.Getenv("LLM_MODEL"); got != values.Model {
		t.Fatalf("model did not survive a .env round trip: %q", got)
	}
	if got := os.Getenv("LLM_TEMPERATURE"); got != "0.4" {
		t.Fatalf("temperature was not normalized: %q", got)
	}
}

func TestSaveLLMValuesRejectsMultilineSecretsWithoutChangingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	original := []byte("APP_ADDR=127.0.0.1:8080\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveLLMValues(path, LLMValues{APIKey: "first-line\nLLM_MODEL=overridden"}); err == nil {
		t.Fatal("expected multiline API keys to be rejected")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("invalid setting changed the .env file: %q", got)
	}
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	previous, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, previous)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
