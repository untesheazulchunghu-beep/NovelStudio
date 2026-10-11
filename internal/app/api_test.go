package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// apiFixture drives the public HTTP surface against a fresh SQLite store so
// these tests pin the wire behaviour (status codes, JSON shapes, secrecy rules)
// independently of how the router is implemented.
type apiFixture struct {
	t       *testing.T
	store   *Store
	handler http.Handler
}

func newAPIFixture(t *testing.T, webDir string) *apiFixture {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &apiFixture{t: t, store: store, handler: NewServer(store, webDir).Handler()}
}

// call sends a request to the default workspace. A string body is sent raw;
// any other non-nil body is JSON-encoded.
func (f *apiFixture) call(method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.callIn(localWorkspaceID, method, path, body)
}

func (f *apiFixture) callIn(workspaceID, method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	var reader io.Reader
	switch value := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(value)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			f.t.Fatalf("encode request body: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	if workspaceID != "" {
		request.Header.Set("X-Workspace-ID", workspaceID)
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func (f *apiFixture) expect(response *httptest.ResponseRecorder, status int) *httptest.ResponseRecorder {
	f.t.Helper()
	if response.Code != status {
		f.t.Fatalf("status = %d, want %d: %s", response.Code, status, response.Body.String())
	}
	return response
}

func decodeAPI[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
	return value
}

func errorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	return decodeAPI[map[string]any](t, response)["error"].(string)
}

func TestAPIRootAndHealthRespond(t *testing.T) {
	f := newAPIFixture(t, "")
	root := decodeAPI[map[string]any](t, f.expect(f.call(http.MethodGet, "/api/", nil), http.StatusOK))
	if root["name"] != "NovelStudio API" {
		t.Fatalf("root name = %v", root["name"])
	}
	health := decodeAPI[map[string]any](t, f.expect(f.call(http.MethodGet, "/api/health", nil), http.StatusOK))
	if health["status"] != "ok" {
		t.Fatalf("health status = %v", health["status"])
	}
}

func TestAPIUnknownEndpointsReturnNotFound(t *testing.T) {
	f := newAPIFixture(t, "")
	for _, path := range []string{"/api/nope", "/api/chapters/1/unknown", "/api/workspace/export/pdf"} {
		f.expect(f.call(http.MethodPost, path, nil), http.StatusNotFound)
	}
}

func TestAPIProjectLifecycle(t *testing.T) {
	f := newAPIFixture(t, "")
	listed := decodeAPI[struct {
		Projects []ProjectSummary `json:"projects"`
	}](t, f.expect(f.call(http.MethodGet, "/api/projects", nil), http.StatusOK))
	if len(listed.Projects) != 1 {
		t.Fatalf("seeded projects = %d, want 1", len(listed.Projects))
	}

	created := decodeAPI[ProjectSummary](t, f.expect(f.call(http.MethodPost, "/api/projects", ProjectInput{Title: "第二部小说", WordTarget: 50000}), http.StatusCreated))
	if created.WorkspaceID == "" || created.Project.Title != "第二部小说" {
		t.Fatalf("created project = %+v", created)
	}

	f.expect(f.call(http.MethodPost, "/api/projects", ProjectInput{Title: "   "}), http.StatusBadRequest)
	f.expect(f.call(http.MethodPost, "/api/projects", ProjectInput{Title: "负目标", WordTarget: -1}), http.StatusBadRequest)

	updated := decodeAPI[ProjectSummary](t, f.expect(f.callIn(created.WorkspaceID, http.MethodPut, "/api/projects/"+created.WorkspaceID, ProjectInput{Title: "改名后的小说", WordTarget: 60000}), http.StatusOK))
	if updated.Project.Title != "改名后的小说" || updated.Project.WordTarget != 60000 {
		t.Fatalf("updated project = %+v", updated.Project)
	}
	f.expect(f.call(http.MethodPut, "/api/projects/"+created.WorkspaceID, ProjectInput{Title: ""}), http.StatusBadRequest)
	f.expect(f.call(http.MethodPut, "/api/projects/missing-project", ProjectInput{Title: "无此项目"}), http.StatusNotFound)

	f.expect(f.call(http.MethodDelete, "/api/projects/"+created.WorkspaceID, nil), http.StatusOK)
	f.expect(f.call(http.MethodDelete, "/api/projects/"+created.WorkspaceID, nil), http.StatusNotFound)
	f.expect(f.call(http.MethodDelete, "/api/projects/"+localWorkspaceID, nil), http.StatusConflict)
}

func TestAPIWorkspaceAndSettingsNeverExposeAPIKey(t *testing.T) {
	f := newAPIFixture(t, "")
	f.expect(f.call(http.MethodGet, "/api/workspace", nil), http.StatusOK)
	f.expect(f.callIn("missing-workspace", http.MethodGet, "/api/workspace", nil), http.StatusNotFound)

	const secret = "sk-test-secret-value-1234567890"
	saved := f.expect(f.call(http.MethodPut, "/api/settings", SettingsInput{
		Provider:    "openai",
		BaseURL:     "https://api.example.com/v1",
		APIKey:      stringPointer(secret),
		Model:       "gpt-test",
		Temperature: 0.4,
	}), http.StatusOK)
	if strings.Contains(saved.Body.String(), secret) {
		t.Fatal("settings update response leaked the API key")
	}
	view := decodeAPI[SettingsView](t, f.expect(f.call(http.MethodGet, "/api/settings", nil), http.StatusOK))
	if !view.APIKeyConfigured || view.Model != "gpt-test" {
		t.Fatalf("settings view = %+v", view)
	}
	if workspace := f.call(http.MethodGet, "/api/workspace", nil).Body.String(); strings.Contains(workspace, secret) {
		t.Fatal("workspace response leaked the API key")
	}

	f.expect(f.call(http.MethodPut, "/api/settings", SettingsInput{Provider: "unsupported", Model: "x"}), http.StatusBadRequest)
	f.expect(f.call(http.MethodPut, "/api/settings", SettingsInput{Provider: "openai", BaseURL: "https://api.example.com/v1", Model: "gpt-test"}), http.StatusOK)
	view = decodeAPI[SettingsView](t, f.expect(f.call(http.MethodGet, "/api/settings", nil), http.StatusOK))
	if !view.APIKeyConfigured {
		t.Fatal("omitting api_key on update must keep the saved key")
	}
}

func TestAPIChapterLifecycleAndOrder(t *testing.T) {
	f := newAPIFixture(t, "")
	initial := decodeAPI[struct {
		Chapters []ChapterSummary `json:"chapters"`
	}](t, f.expect(f.call(http.MethodGet, "/api/chapters", nil), http.StatusOK))

	first := decodeAPI[Chapter](t, f.expect(f.call(http.MethodPost, "/api/chapters", ChapterInput{Title: "开端", Content: "第一章正文 alpha"}), http.StatusCreated))
	second := decodeAPI[Chapter](t, f.expect(f.call(http.MethodPost, "/api/chapters", ChapterInput{Title: "发展", Content: "第二章 beta"}), http.StatusCreated))
	f.expect(f.call(http.MethodPost, "/api/chapters", ChapterInput{Title: strings.Repeat("长", maxChapterTitleLength+1)}), http.StatusBadRequest)

	chapter := decodeAPI[Chapter](t, f.expect(f.call(http.MethodGet, "/api/chapters/"+idPath(first.ID), nil), http.StatusOK))
	if chapter.Content != "第一章正文 alpha" {
		t.Fatalf("chapter content = %q", chapter.Content)
	}

	updated := decodeAPI[Chapter](t, f.expect(f.call(http.MethodPut, "/api/chapters/"+idPath(first.ID), ChapterInput{Title: "开端（修订）", Content: "改写后的正文"}), http.StatusOK))
	if updated.Title != "开端（修订）" || updated.WordCount == 0 {
		t.Fatalf("updated chapter = %+v", updated)
	}

	list := decodeAPI[struct {
		Chapters []ChapterSummary `json:"chapters"`
	}](t, f.expect(f.call(http.MethodGet, "/api/chapters", nil), http.StatusOK))
	if len(list.Chapters) != len(initial.Chapters)+2 {
		t.Fatalf("chapters = %d, want %d", len(list.Chapters), len(initial.Chapters)+2)
	}

	reversed := make([]int64, 0, len(list.Chapters))
	for _, chapter := range slices.Backward(list.Chapters) {
		reversed = append(reversed, chapter.ID)
	}
	ordered := decodeAPI[map[string][]int64](t, f.expect(f.call(http.MethodPut, "/api/chapters/order", ChapterOrderInput{ChapterIDs: reversed}), http.StatusOK))
	if len(ordered["chapter_ids"]) != len(reversed) || ordered["chapter_ids"][0] != reversed[0] {
		t.Fatalf("reordered ids = %v, want %v", ordered["chapter_ids"], reversed)
	}
	f.expect(f.call(http.MethodPut, "/api/chapters/order", ChapterOrderInput{ChapterIDs: []int64{first.ID}}), http.StatusBadRequest)

	f.expect(f.call(http.MethodDelete, "/api/chapters/"+idPath(second.ID), nil), http.StatusOK)
	f.expect(f.call(http.MethodGet, "/api/chapters/"+idPath(second.ID), nil), http.StatusNotFound)
	f.expect(f.call(http.MethodGet, "/api/chapters/not-a-number", nil), http.StatusBadRequest)
	// Unknown statuses are normalized to draft rather than rejected, so older clients keep saving.
	normalized := decodeAPI[Chapter](t, f.expect(f.call(http.MethodPut, "/api/chapters/"+idPath(first.ID), ChapterInput{Title: "状态归一", Status: "bogus-status"}), http.StatusOK))
	if normalized.Status != "draft" {
		t.Fatalf("status = %q, want draft", normalized.Status)
	}
}

func TestAPIChapterSearchFindsSavedText(t *testing.T) {
	f := newAPIFixture(t, "")
	f.expect(f.call(http.MethodPost, "/api/chapters", ChapterInput{Title: "夜航", Content: "灯塔之下藏着一把银色钥匙。"}), http.StatusCreated)

	results := decodeAPI[struct {
		Chapters []ChapterSearchResult `json:"chapters"`
	}](t, f.expect(f.call(http.MethodGet, "/api/chapters/search?q="+url.QueryEscape("银色钥匙"), nil), http.StatusOK))
	if len(results.Chapters) != 1 || !strings.Contains(results.Chapters[0].Excerpt, "银色钥匙") {
		t.Fatalf("search results = %+v", results.Chapters)
	}

	f.expect(f.call(http.MethodGet, "/api/chapters/search?q="+url.QueryEscape(strings.Repeat("a", 513)), nil), http.StatusBadRequest)
	f.expect(f.call(http.MethodPost, "/api/chapters/search?q=x", nil), http.StatusMethodNotAllowed)
}

func TestAPIChapterRevisionsRoundTrip(t *testing.T) {
	f := newAPIFixture(t, "")
	chapter := decodeAPI[Chapter](t, f.expect(f.call(http.MethodPost, "/api/chapters", ChapterInput{Title: "修订", Content: "初稿"}), http.StatusCreated))
	base := "/api/chapters/" + idPath(chapter.ID) + "/revisions"

	revision := decodeAPI[ChapterRevision](t, f.expect(f.call(http.MethodPost, base, ChapterRevisionInput{Label: "初稿保存"}), http.StatusCreated))
	listed := decodeAPI[struct {
		Revisions []ChapterRevisionSummary `json:"revisions"`
	}](t, f.expect(f.call(http.MethodGet, base, nil), http.StatusOK))
	if len(listed.Revisions) != 1 || listed.Revisions[0].Label != "初稿保存" {
		t.Fatalf("revisions = %+v", listed.Revisions)
	}
	preview := decodeAPI[ChapterRevision](t, f.expect(f.call(http.MethodGet, base+"/"+idPath(revision.ID), nil), http.StatusOK))
	if preview.Content != "初稿" {
		t.Fatalf("revision content = %q", preview.Content)
	}

	f.expect(f.call(http.MethodPut, "/api/chapters/"+idPath(chapter.ID), ChapterInput{Title: "修订", Content: "改稿"}), http.StatusOK)
	restored := decodeAPI[Chapter](t, f.expect(f.call(http.MethodPost, base+"/"+idPath(revision.ID)+"/restore", nil), http.StatusOK))
	if restored.Content != "初稿" {
		t.Fatalf("restored content = %q, want the checkpoint body", restored.Content)
	}

	f.expect(f.call(http.MethodGet, base+"/999999", nil), http.StatusNotFound)
	f.expect(f.call(http.MethodGet, "/api/chapters/999999/revisions", nil), http.StatusNotFound)
}

func TestAPICharacterAndNoteLifecycle(t *testing.T) {
	f := newAPIFixture(t, "")
	character := decodeAPI[Character](t, f.expect(f.call(http.MethodPost, "/api/characters", CharacterInput{Name: "林澈", Role: "主角", Traits: []string{"冷静", "执拗"}}), http.StatusCreated))
	if character.Color == "" || len(character.Traits) != 2 {
		t.Fatalf("created character = %+v", character)
	}
	f.expect(f.call(http.MethodPost, "/api/characters", CharacterInput{Name: "  "}), http.StatusBadRequest)
	f.expect(f.call(http.MethodPut, "/api/characters/"+idPath(character.ID), CharacterInput{Name: "林澈", Color: "red"}), http.StatusBadRequest)

	renamed := decodeAPI[Character](t, f.expect(f.call(http.MethodPut, "/api/characters/"+idPath(character.ID), CharacterInput{Name: "林澈（改）", Role: "主角", Color: "#58c5a3"}), http.StatusOK))
	if renamed.Name != "林澈（改）" || renamed.Color != "#58c5a3" {
		t.Fatalf("renamed character = %+v", renamed)
	}
	f.expect(f.call(http.MethodGet, "/api/characters/"+idPath(character.ID), nil), http.StatusOK)
	f.expect(f.call(http.MethodDelete, "/api/characters/"+idPath(character.ID), nil), http.StatusOK)
	f.expect(f.call(http.MethodGet, "/api/characters/"+idPath(character.ID), nil), http.StatusNotFound)

	note := decodeAPI[Note](t, f.expect(f.call(http.MethodPost, "/api/notes", NoteInput{Title: "伏笔", Body: "钥匙", Kind: "mystery"}), http.StatusCreated))
	if note.Kind != "mystery" {
		t.Fatalf("note kind = %q", note.Kind)
	}
	f.expect(f.call(http.MethodPost, "/api/notes", NoteInput{Title: "未知", Kind: "gossip"}), http.StatusBadRequest)
	updated := decodeAPI[Note](t, f.expect(f.call(http.MethodPut, "/api/notes/"+idPath(note.ID), NoteInput{Title: "伏笔（已揭示）", Body: "钥匙在灯塔", Kind: "mystery"}), http.StatusOK))
	if updated.Title != "伏笔（已揭示）" {
		t.Fatalf("updated note = %+v", updated)
	}
	f.expect(f.call(http.MethodDelete, "/api/notes/"+idPath(note.ID), nil), http.StatusOK)
	f.expect(f.call(http.MethodDelete, "/api/notes/"+idPath(note.ID), nil), http.StatusNotFound)
}

func TestAPIExportsProduceDocumentsAndRejectWrongMethods(t *testing.T) {
	f := newAPIFixture(t, "")
	f.expect(f.call(http.MethodPost, "/api/chapters", ChapterInput{Title: "导出章", Content: "导出正文"}), http.StatusCreated)

	markdown := f.expect(f.call(http.MethodPost, "/api/workspace/export/markdown", nil), http.StatusOK)
	if !strings.Contains(markdown.Body.String(), "导出正文") {
		t.Fatal("markdown export is missing chapter text")
	}
	for _, format := range []string{"docx", "epub"} {
		response := f.expect(f.call(http.MethodPost, "/api/workspace/export/"+format, nil), http.StatusOK)
		if !bytes.HasPrefix(response.Body.Bytes(), []byte("PK")) {
			t.Fatalf("%s export is not a ZIP package", format)
		}
	}
	f.expect(f.call(http.MethodGet, "/api/workspace/export/docx", nil), http.StatusMethodNotAllowed)
	f.expect(f.call(http.MethodGet, "/api/stats", nil), http.StatusOK)
}

func TestAPIBackupExportAndImportCreatesIndependentProject(t *testing.T) {
	f := newAPIFixture(t, "")
	f.expect(f.call(http.MethodPost, "/api/chapters", ChapterInput{Title: "备份章", Content: "备份正文"}), http.StatusCreated)

	backupResponse := f.expect(f.call(http.MethodGet, "/api/workspace/backup", nil), http.StatusOK)
	if !strings.Contains(backupResponse.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("backup disposition = %q", backupResponse.Header().Get("Content-Disposition"))
	}
	backup := decodeAPI[ProjectBackup](t, backupResponse)

	before := decodeAPI[struct {
		Projects []ProjectSummary `json:"projects"`
	}](t, f.expect(f.call(http.MethodGet, "/api/projects", nil), http.StatusOK))
	imported := decodeAPI[ProjectSummary](t, f.expect(f.call(http.MethodPost, "/api/projects/import", backup), http.StatusCreated))
	if imported.WorkspaceID == localWorkspaceID {
		t.Fatal("imported backup must become a separate project")
	}
	after := decodeAPI[struct {
		Projects []ProjectSummary `json:"projects"`
	}](t, f.expect(f.call(http.MethodGet, "/api/projects", nil), http.StatusOK))
	if len(after.Projects) != len(before.Projects)+1 {
		t.Fatalf("projects after import = %d, want %d", len(after.Projects), len(before.Projects)+1)
	}

	f.expect(f.call(http.MethodPost, "/api/projects/import", `{"format":"not-novelstudio"}`), http.StatusBadRequest)
}

func TestAPIGenerateUsesLocalProviderForChapter(t *testing.T) {
	f := newAPIFixture(t, "")
	chapter := decodeAPI[Chapter](t, f.expect(f.call(http.MethodPost, "/api/chapters", ChapterInput{Title: "续写", Content: "夜色深沉。"}), http.StatusCreated))

	generated := decodeAPI[map[string]any](t, f.expect(f.call(http.MethodPost, "/api/chapters/"+idPath(chapter.ID)+"/generate", map[string]string{"instruction": "推进线索"}), http.StatusOK))
	if text, _ := generated["text"].(string); strings.TrimSpace(text) == "" {
		t.Fatal("generated text is empty")
	}
	if generated["provider"] != "local-creative" {
		t.Fatalf("provider = %v, want local-creative", generated["provider"])
	}
	f.expect(f.call(http.MethodPost, "/api/chapters/999999/generate", nil), http.StatusNotFound)
}

func TestAPIRejectsOversizedJSONBodies(t *testing.T) {
	f := newAPIFixture(t, "")
	oversized := `{"title":"` + strings.Repeat("a", int(maxSmallJSONRequestBytes)) + `"}`
	f.expect(f.call(http.MethodPut, "/api/projects/"+localWorkspaceID, oversized), http.StatusRequestEntityTooLarge)
	f.expect(f.call(http.MethodPost, "/api/chapters", `{"title":"a"} {"title":"b"}`), http.StatusBadRequest)
}

func TestAPIServesFrontendAndFallsBackForRoutes(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>NovelStudio</title>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatalf("create assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	f := newAPIFixture(t, webDir)

	index := f.expect(f.callIn("", http.MethodGet, "/", nil), http.StatusOK)
	if !strings.Contains(index.Body.String(), "NovelStudio") {
		t.Fatal("root did not serve index.html")
	}
	f.expect(f.callIn("", http.MethodGet, "/assets/app.js", nil), http.StatusOK)
	f.expect(f.callIn("", http.MethodGet, "/assets/missing.js", nil), http.StatusNotFound)
	f.expect(f.callIn("", http.MethodGet, "/workbench/deep-link", nil), http.StatusOK)
	f.expect(f.callIn("", http.MethodPost, "/", nil), http.StatusMethodNotAllowed)
}

func idPath(id int64) string {
	return strconv.FormatInt(id, 10)
}

func stringPointer(value string) *string {
	return &value
}
