package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRuntimeTestsSavesAndAppliesLocalModelSettings(t *testing.T) {
	restoreLLMEnvironment(t)
	var mu sync.Mutex
	var modelRequests, generationRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer local-secret" {
			t.Errorf("unexpected authorization header: %q", got)
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			modelRequests++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			generationRequests++
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"来自新模型\"}}]}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	envPath := filepath.Join(t.TempDir(), ".env")
	runtime := NewRuntime(RuntimeSettings{
		BaseURL:     server.URL + "/v1",
		Model:       "local-model",
		Temperature: 0.7,
		MaxTokens:   0,
		TimeoutSec:  30,
	}, envPath)
	if !runtime.DemoMode() {
		t.Fatal("runtime without an API key should start in demo mode")
	}

	input := SettingsInput{
		BaseURL:     server.URL + "/v1",
		Model:       "local-model",
		APIKey:      "local-secret",
		Temperature: 0.4,
		MaxTokens:   2048,
		TimeoutSec:  45,
	}
	if err := runtime.TestConnection(context.Background(), input); err != nil {
		t.Fatalf("connection test failed: %v", err)
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatal("testing unsaved settings should not write the .env file")
	}

	view, err := runtime.Update(input)
	if err != nil {
		t.Fatal(err)
	}
	if !view.APIKeyConfigured || view.Model != input.Model || runtime.DemoMode() {
		t.Fatalf("runtime did not switch to the saved model settings: %#v", view)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "local-secret") || strings.Contains(string(encoded), `"apiKey":`) {
		t.Fatalf("public model settings leaked the API key: %s", encoded)
	}

	output, err := runtime.Generate(context.Background(), "write", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if output != "来自新模型" {
		t.Fatalf("generation did not use the updated provider: %q", output)
	}

	input.APIKey = ""
	view, err = runtime.Update(input)
	if err != nil {
		t.Fatal(err)
	}
	if !view.APIKeyConfigured {
		t.Fatal("leaving the key field blank should preserve the saved API key")
	}
	input.ClearAPIKey = true
	view, err = runtime.Update(input)
	if err != nil {
		t.Fatal(err)
	}
	if view.APIKeyConfigured || !runtime.DemoMode() {
		t.Fatal("explicitly clearing the key should return the runtime to demo mode")
	}

	mu.Lock()
	defer mu.Unlock()
	if modelRequests != 1 || generationRequests != 1 {
		t.Fatalf("unexpected provider request counts: models=%d generation=%d", modelRequests, generationRequests)
	}
}

func TestRuntimeRejectsInvalidSettingsBeforePersisting(t *testing.T) {
	restoreLLMEnvironment(t)
	envPath := filepath.Join(t.TempDir(), ".env")
	runtime := NewRuntime(RuntimeSettings{
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		Temperature: 0.7,
		MaxTokens:   0,
		TimeoutSec:  480,
	}, envPath)

	_, err := runtime.Update(SettingsInput{
		BaseURL:     "file:///etc/passwd",
		Model:       "gpt-4o-mini",
		Temperature: 0.7,
		TimeoutSec:  480,
	})
	if _, ok := err.(*SettingsValidationError); !ok {
		t.Fatalf("expected an invalid settings error, got %v", err)
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatal("invalid settings should not be persisted")
	}
	if runtime.Settings().BaseURL != "https://api.openai.com/v1" {
		t.Fatal("invalid settings changed the running configuration")
	}
}

func restoreLLMEnvironment(t *testing.T) {
	t.Helper()
	keys := []string{"LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL", "LLM_TEMPERATURE", "LLM_MAX_TOKENS", "LLM_TIMEOUT"}
	previous := make(map[string]string, len(keys))
	existed := make(map[string]bool, len(keys))
	for _, key := range keys {
		previous[key], existed[key] = os.LookupEnv(key)
		_ = os.Unsetenv(key)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			if existed[key] {
				_ = os.Setenv(key, previous[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}
