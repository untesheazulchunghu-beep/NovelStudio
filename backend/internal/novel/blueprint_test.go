package novel

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

type capturingGenerator struct {
	prompts chan string
}

func (g capturingGenerator) Generate(_ context.Context, prompt string, emit func(string)) (string, error) {
	g.prompts <- prompt
	text := "雨停后，渡口的灯一盏接一盏亮了起来。"
	emit(text)
	return text, nil
}

func TestBlueprintDrivesGenerationPrompt(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "blueprint.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	prompts := make(chan string, 4)
	manager, err := task.NewManager(db, capturingGenerator{prompts: prompts})
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	webServer := newTLSServer(t, application.Handler())
	defer webServer.Close()
	client := newClient(t, webServer)

	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "blueprint@example.test", "password": "safe-test-password-123"}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)

	const guide = "禁止使用现代词汇，保持冷峻克制的叙述。"
	novel := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "雾港来信", "summary": "渡口小镇。", "genre": "悬疑", "plannedChapters": 3, "approvalMode": "manual", "writingGuide": guide,
	}, http.StatusCreated)
	novelID := novel["id"].(string)
	if novel["writingGuide"] != guide {
		t.Fatalf("createNovel should echo the writing guide, got %#v", novel["writingGuide"])
	}
	chapterResponse := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)
	chapterID := chapterResponse["chapters"].([]any)[0].(map[string]any)["id"].(string)

	// Blueprint-only edits must persist without bumping the manuscript revision.
	requestJSON(t, client, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID, map[string]any{
		"expectedRevision": 1, "title": "第一章", "body": "", "goal": "主角抵达雾港", "keyEvents": "与旧友重逢", "suspenseHook": "信件寄件人是谁",
	}, http.StatusOK)
	afterBlueprint := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)[0].(map[string]any)
	if afterBlueprint["revision"].(float64) != 1 {
		t.Fatalf("blueprint-only edit should keep revision at 1, got %v", afterBlueprint["revision"])
	}
	if afterBlueprint["goal"] != "主角抵达雾港" {
		t.Fatalf("chapter goal was not persisted: %#v", afterBlueprint["goal"])
	}

	taskResponse := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{"chapterIds": []string{chapterID}, "instruction": "续写本章。"}, http.StatusAccepted)
	waitForTaskStatus(t, client, webServer.URL, taskResponse["id"].(string), "completed")

	prompt := <-prompts
	for _, expected := range []string{guide, "主角抵达雾港", "与旧友重逢", "信件寄件人是谁", "写作指南"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("generation prompt should contain %q, got:\n%s", expected, prompt)
		}
	}
}
