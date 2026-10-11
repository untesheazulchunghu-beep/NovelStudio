package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSRejectsRequestsOutsideLoopback(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name   string
		method string
		host   string
		origin string
		want   int
	}{
		{name: "loopback host without origin", method: http.MethodGet, host: "127.0.0.1:8080", want: http.StatusOK},
		{name: "localhost host without origin", method: http.MethodGet, host: "localhost:8080", want: http.StatusOK},
		{name: "ipv6 loopback host without origin", method: http.MethodGet, host: "[::1]:8080", want: http.StatusOK},
		{name: "loopback host with same origin", method: http.MethodPost, host: "127.0.0.1:8080", origin: "http://127.0.0.1:8080", want: http.StatusOK},
		{name: "vite dev origin on loopback host", method: http.MethodPost, host: "localhost:8080", origin: "http://localhost:5173", want: http.StatusOK},
		// DNS rebinding: the attacker's domain resolves to 127.0.0.1, so the
		// Host header is the attacker's name and no Origin header is sent on
		// same-origin GET requests. The Host check must reject these.
		{name: "rebound domain reads without origin", method: http.MethodGet, host: "evil.example:8080", want: http.StatusForbidden},
		{name: "rebound domain with matching origin", method: http.MethodPost, host: "evil.example:8080", origin: "http://evil.example:8080", want: http.StatusForbidden},
		{name: "lan address", method: http.MethodGet, host: "192.168.1.20:8080", want: http.StatusForbidden},
		{name: "loopback-looking suffix domain", method: http.MethodGet, host: "localhost.evil.example:8080", want: http.StatusForbidden},
		{name: "empty host", method: http.MethodGet, host: "", want: http.StatusForbidden},
		{name: "foreign origin on loopback host", method: http.MethodPost, host: "127.0.0.1:8080", origin: "https://evil.example", want: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, "/api/workspace", nil)
			request.Host = tc.host
			if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %q)", recorder.Code, tc.want, recorder.Body.String())
			}
		})
	}
}

func TestIsLoopbackAuthority(t *testing.T) {
	cases := map[string]bool{
		"localhost":        true,
		"LOCALHOST:8080":   true,
		"127.0.0.1":        true,
		"127.0.0.1:5173":   true,
		"127.10.0.1:80":    true,
		"[::1]:8080":       true,
		"::1":              true,
		"0.0.0.0:8080":     false,
		"192.168.1.20:80":  false,
		"example.com:8080": false,
		"localhost.evil":   false,
		"":                 false,
	}
	for authority, want := range cases {
		if got := isLoopbackAuthority(authority); got != want {
			t.Errorf("isLoopbackAuthority(%q) = %v, want %v", authority, got, want)
		}
	}
}
