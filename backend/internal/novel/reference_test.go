package novel

import (
	"net/http"
	"path/filepath"
	"testing"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

func TestReferenceSearchAndDelete(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "references.sqlite"))
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
		"email": "references@example.test", "password": "safe-reference-password",
	}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	novel := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "资料测试", "summary": "", "genre": "悬疑", "plannedChapters": 1, "approvalMode": "manual",
	}, http.StatusCreated)
	novelID := novel["id"].(string)
	document := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/references", map[string]string{
		"title": "港口资料", "content": "foo-bar 的记录包含 100% 的内容。",
	}, http.StatusCreated)
	documentID := document["id"].(string)
	search := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/references/search?q=foo-bar", nil, http.StatusOK)
	if len(search["results"].([]any)) != 1 {
		t.Fatalf("expected punctuation-containing reference search to return one result: %#v", search)
	}
	requestJSON(t, client, http.MethodDelete, server.URL+"/api/v1/novels/"+novelID+"/references/"+documentID, nil, http.StatusOK)
	search = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/references/search?q=foo-bar", nil, http.StatusOK)
	if len(search["results"].([]any)) != 0 {
		t.Fatalf("deleted reference should not remain searchable: %#v", search)
	}
}
