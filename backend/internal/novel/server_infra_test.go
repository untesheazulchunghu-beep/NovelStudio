package novel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/llm"
	"novelstudio/internal/task"
)

func newInfraServer(t *testing.T, staticDir string, origins []string) *httptest.Server {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "infra.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	manager, err := task.NewManager(db, testGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, origins, staticDir)
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)
	return server
}

func TestStaticServingAndSpaFallback(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(staticDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<!doctype html><title>拾卷</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "assets", "app.js"), []byte("console.log('hi')"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := newInfraServer(t, staticDir, nil)

	index := get(t, server.URL+"/")
	if index.StatusCode != http.StatusOK || index.Header.Get("Content-Type") == "application/json; charset=utf-8" {
		t.Fatalf("root should serve the SPA index, got %d %s", index.StatusCode, index.Header.Get("Content-Type"))
	}
	index.Body.Close()

	fallback := get(t, server.URL+"/novels/123/chapters")
	if fallback.StatusCode != http.StatusOK || fallback.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("client route should fall back to index.html, got %d %s", fallback.StatusCode, fallback.Header.Get("Cache-Control"))
	}
	fallback.Body.Close()

	asset := get(t, server.URL+"/assets/app.js")
	if asset.StatusCode != http.StatusOK || asset.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset should be served with immutable cache, got %d %s", asset.StatusCode, asset.Header.Get("Cache-Control"))
	}
	asset.Body.Close()

	unknownAPI := get(t, server.URL+"/api/v1/does-not-exist")
	if unknownAPI.StatusCode != http.StatusNotFound || unknownAPI.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unknown API route should return JSON 404, got %d %s", unknownAPI.StatusCode, unknownAPI.Header.Get("Content-Type"))
	}
	unknownAPI.Body.Close()
}

func TestLLMSettingsAreAuthenticatedAndNeverReturnTheAPIKey(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "model-settings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := task.NewManager(db, testGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(t.TempDir(), ".env")
	modelRuntime := llm.NewRuntime(llm.RuntimeSettings{
		BaseURL:     "https://api.example.test/v1",
		APIKey:      "original-secret",
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   0,
		TimeoutSec:  90,
	}, envPath)
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	application.SetLLMRuntime(modelRuntime)
	server := httptest.NewServer(application.Handler())
	defer server.Close()
	client := newClient(t, server)

	requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/settings/llm", nil, http.StatusUnauthorized)
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/settings/llm/test", map[string]any{}, http.StatusUnauthorized)
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "model-settings@example.test", "password": "safe-model-settings-password",
	}, http.StatusCreated)

	settingsResponse := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/settings/llm", nil, http.StatusOK)
	settings := settingsResponse["settings"].(map[string]any)
	encoded, err := json.Marshal(settingsResponse)
	if err != nil {
		t.Fatal(err)
	}
	if settingsResponse["canManage"] != true || settings["apiKeyConfigured"] != true || strings.Contains(string(encoded), "original-secret") {
		t.Fatalf("settings response should mask the configured key: %s", encoded)
	}

	saved := requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/settings/llm", map[string]any{
		"baseUrl": "https://api.example.test/v1", "model": "new-model", "apiKey": "replacement-secret",
		"clearApiKey": false, "temperature": 0.5, "maxTokens": 1024, "timeoutSec": 120,
	}, http.StatusOK)
	encoded, err = json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if saved["model"] != "new-model" || saved["apiKeyConfigured"] != true || strings.Contains(string(encoded), "replacement-secret") {
		t.Fatalf("save response should include public settings only: %s", encoded)
	}
	if modelRuntime.DemoMode() {
		t.Fatal("saved API settings should update the running model provider")
	}
	if _, err := os.Stat(envPath); err != nil {
		t.Fatalf("saved settings should be persisted locally: %v", err)
	}

	secondClient := newClient(t, server)
	requestJSON(t, secondClient, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "second-model-settings@example.test", "password": "safe-second-settings-password",
	}, http.StatusCreated)
	secondSettings := requestJSON(t, secondClient, http.MethodGet, server.URL+"/api/v1/settings/llm", nil, http.StatusOK)
	if secondSettings["canManage"] != false {
		t.Fatal("only the first local account should be able to manage the shared model key")
	}
	requestJSON(t, secondClient, http.MethodPut, server.URL+"/api/v1/settings/llm", map[string]any{
		"baseUrl": "https://api.example.test/v1", "model": "unauthorized-model", "apiKey": "",
		"clearApiKey": false, "temperature": 0.5, "maxTokens": 1024, "timeoutSec": 120,
	}, http.StatusForbidden)
	requestJSON(t, secondClient, http.MethodPost, server.URL+"/api/v1/settings/llm/test", map[string]any{
		"baseUrl": "https://api.example.test/v1", "model": "unauthorized-model", "apiKey": "ignored",
		"clearApiKey": false, "temperature": 0.5, "maxTokens": 1024, "timeoutSec": 120,
	}, http.StatusForbidden)
}

func TestSecurityHeadersAndCORS(t *testing.T) {
	server := newInfraServer(t, "", []string{"localhost:5173"})

	response := get(t, server.URL+"/healthz")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health check failed with %d", response.StatusCode)
	}
	if response.Header.Get("X-Content-Type-Options") != "nosniff" || response.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("expected hardened security headers")
	}

	allowed := preflight(t, server.URL+"/api/v1/novels", "http://localhost:5173")
	if allowed.StatusCode != http.StatusNoContent || allowed.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("allowed origin should receive CORS headers, got %d %q", allowed.StatusCode, allowed.Header.Get("Access-Control-Allow-Origin"))
	}
	if allowed.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("expected credentialed CORS response")
	}

	denied := preflight(t, server.URL+"/api/v1/novels", "http://evil.example")
	if denied.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unlisted origin must not receive CORS headers")
	}
}

func TestAuthRateLimiting(t *testing.T) {
	server := newInfraServer(t, "", nil)
	client := server.Client()
	last := 0
	for attempt := 0; attempt < 21; attempt++ {
		response, err := client.Post(server.URL+"/api/v1/auth/login", "application/json", jsonBody(t, map[string]string{"email": "nobody@example.test", "password": "wrong-password-value"}))
		if err != nil {
			t.Fatal(err)
		}
		last = response.StatusCode
		response.Body.Close()
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("expected the 21st login attempt to be rate limited, got %d", last)
	}
}

func TestLogoutRevokesSessionsOnOtherDevices(t *testing.T) {
	server := newInfraServer(t, "", nil)
	firstDevice := newClient(t, server)
	secondDevice := newClient(t, server)
	credentials := map[string]string{"email": "sessions@example.test", "password": "safe-session-password"}
	requestJSON(t, firstDevice, http.MethodPost, server.URL+"/api/v1/auth/register", credentials, http.StatusCreated)
	requestJSON(t, secondDevice, http.MethodPost, server.URL+"/api/v1/auth/login", credentials, http.StatusOK)
	requestJSON(t, firstDevice, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)
	requestJSON(t, secondDevice, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)

	requestJSON(t, firstDevice, http.MethodPost, server.URL+"/api/v1/auth/logout", nil, http.StatusOK)
	requestJSON(t, firstDevice, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusUnauthorized)
	requestJSON(t, secondDevice, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusUnauthorized)
}

func get(t *testing.T, target string) *http.Response {
	t.Helper()
	response, err := http.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func preflight(t *testing.T, target, origin string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodOptions, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", origin)
	request.Header.Set("Access-Control-Request-Method", "POST")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	return response
}

func jsonBody(t *testing.T, value any) *bytes.Reader {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(encoded)
}
