package novel

import (
	"net/http"
	"path/filepath"
	"testing"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

func TestChangePasswordRequiresCorrectOldPassword(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "password.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := task.NewManager(db, testGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	server := newTLSServer(t, NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "").Handler())
	defer server.Close()
	client := newClient(t, server)
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "password-test@example.test", "password": "initial-password-safe",
	}, http.StatusCreated)
	requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/auth/password", map[string]string{
		"oldPassword": "wrong-password", "newPassword": "new-password-safe-enough",
	}, http.StatusUnauthorized)
	requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/auth/password", map[string]string{
		"oldPassword": "initial-password-safe", "newPassword": "new-password-safe-enough",
	}, http.StatusOK)
	client = newClient(t, server)
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{
		"email": "password-test@example.test", "password": "initial-password-safe",
	}, http.StatusUnauthorized)
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{
		"email": "password-test@example.test", "password": "new-password-safe-enough",
	}, http.StatusOK)
}

func TestChangePasswordRevokesOldSessions(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "password-revoke.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := task.NewManager(db, testGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	server := newTLSServer(t, NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "").Handler())
	defer server.Close()
	client1 := newClient(t, server)
	requestJSON(t, client1, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "revoke-test@example.test", "password": "initial-password-safe",
	}, http.StatusCreated)
	client2 := newClient(t, server)
	requestJSON(t, client2, http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{
		"email": "revoke-test@example.test", "password": "initial-password-safe",
	}, http.StatusOK)
	requestJSON(t, client1, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)
	requestJSON(t, client2, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)
	requestJSON(t, client1, http.MethodPut, server.URL+"/api/v1/auth/password", map[string]string{
		"oldPassword": "initial-password-safe", "newPassword": "new-password-safe-enough",
	}, http.StatusOK)
	requestJSON(t, client1, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)
	requestJSON(t, client2, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusUnauthorized)
}
