package novel

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

type testGenerator struct{}

func (testGenerator) Generate(_ context.Context, _ string, emit func(string)) (string, error) {
	text := "雨停后，渡口的灯一盏接一盏亮了起来。"
	emit(text)
	return text, nil
}

type blockingGenerator struct {
	started chan struct{}
}

type failOnceGenerator struct {
	mu    sync.Mutex
	calls int
}

func (g *failOnceGenerator) Generate(_ context.Context, _ string, emit func(string)) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.calls == 1 {
		return "", errors.New("temporary model failure")
	}
	text := "生成结果" + strconv.Itoa(g.calls)
	emit(text)
	return text, nil
}

func (g blockingGenerator) Generate(ctx context.Context, _ string, _ func(string)) (string, error) {
	close(g.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func TestExportNovelTextAndMarkdown(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "export.sqlite")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := task.NewManager(db, testGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, []string{"*"}, "")
	webServer := newTLSServer(t, application.Handler())
	defer webServer.Close()

	ownerClient := newClient(t, webServer)
	requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "exporter@example.test", "password": "safe-test-password-123"}, http.StatusCreated)
	profile := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	novel := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "雾港来信", "summary": "中文梗概", "genre": "悬疑", "plannedChapters": 3, "approvalMode": "manual",
	}, http.StatusCreated)
	novelID := novel["id"].(string)

	listed := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)
	first := listed["chapters"].([]any)[0].(map[string]any)
	first = saveChapterText(t, ownerClient, webServer.URL, novelID, first, "灯塔", "潮水拍打石阶。")
	deleted := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", map[string]string{"title": "删除稿"}, http.StatusCreated)
	deleted = saveChapterText(t, ownerClient, webServer.URL, novelID, deleted, "删除稿", "这段不该出现。")
	third := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", map[string]string{"title": "来信"}, http.StatusCreated)
	third = saveChapterText(t, ownerClient, webServer.URL, novelID, third, "来信", "信纸上写着沈知遥。")
	requestJSON(t, ownerClient, http.MethodDelete, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+deleted["id"].(string), nil, http.StatusOK)
	requestJSON(t, ownerClient, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/order", map[string]any{
		"chapterIds": []string{third["id"].(string), first["id"].(string)},
	}, http.StatusOK)

	requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/export?format=pdf", nil, http.StatusBadRequest)
	docx := exportNovel(t, ownerClient, webServer.URL, novelID, "docx")
	if docx.contentType != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("docx content type = %q", docx.contentType)
	}
	doc, err := zip.NewReader(bytes.NewReader(docx.body), int64(len(docx.body)))
	if err != nil {
		t.Fatalf("invalid docx archive: %v", err)
	}
	docEntries := map[string]bool{}
	for _, entry := range doc.File {
		docEntries[entry.Name] = true
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
		if !docEntries[name] {
			t.Fatalf("docx missing %s", name)
		}
	}

	markdown := exportNovel(t, ownerClient, webServer.URL, novelID, "md")
	if markdown.contentType != "text/markdown; charset=utf-8" {
		t.Fatalf("markdown content type = %q", markdown.contentType)
	}
	plain := exportNovel(t, ownerClient, webServer.URL, novelID, "txt")
	if plain.contentType != "text/plain; charset=utf-8" {
		t.Fatalf("text content type = %q", plain.contentType)
	}
	epub := exportNovel(t, ownerClient, webServer.URL, novelID, "epub")
	if epub.contentType != "application/epub+zip" {
		t.Fatalf("epub content type = %q", epub.contentType)
	}
	book, err := zip.NewReader(bytes.NewReader(epub.body), int64(len(epub.body)))
	if err != nil {
		t.Fatalf("invalid epub archive: %v", err)
	}
	entries := map[string]bool{}
	for _, entry := range book.File {
		entries[entry.Name] = true
	}
	for _, name := range []string{"mimetype", "META-INF/container.xml", "OEBPS/content.opf", "OEBPS/nav.xhtml"} {
		if !entries[name] {
			t.Fatalf("epub missing %s", name)
		}
	}
	wantMarkdown := "# 雾港来信\n\n## 第1章 来信\n\n信纸上写着沈知遥。\n\n## 第3章 灯塔\n\n潮水拍打石阶。\n\n"
	wantText := "雾港来信\r\n\r\n第1章 来信\r\n\r\n信纸上写着沈知遥。\r\n\r\n第3章 灯塔\r\n\r\n潮水拍打石阶。\r\n\r\n"
	if !bytes.Equal(markdown.body, []byte(wantMarkdown)) {
		t.Fatalf("markdown export bytes = %q", markdown.body)
	}
	if !bytes.Equal(plain.body, []byte(wantText)) {
		t.Fatalf("text export bytes = %q", plain.body)
	}
	for _, body := range [][]byte{markdown.body, plain.body} {
		if bytes.Contains(body, []byte("删除稿")) || bytes.Contains(body, []byte("这段不该出现")) {
			t.Fatalf("soft-deleted chapter leaked into export: %q", body)
		}
	}

	viewerClient := newClient(t, webServer)
	requestJSON(t, viewerClient, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "export-reader@example.test", "password": "safe-test-password-456"}, http.StatusCreated)
	requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/workspaces/"+workspaceID+"/members", map[string]string{"email": "export-reader@example.test", "role": "viewer"}, http.StatusCreated)
	viewerExport := exportNovel(t, viewerClient, webServer.URL, novelID, "txt")
	if viewerExport.status != http.StatusOK || !bytes.Equal(viewerExport.body, []byte(wantText)) {
		t.Fatalf("viewer export status=%d body=%q", viewerExport.status, viewerExport.body)
	}
}

func saveChapterText(t *testing.T, client *http.Client, baseURL, novelID string, chapter map[string]any, title, body string) map[string]any {
	t.Helper()
	revision, ok := chapter["revision"].(float64)
	updatedAt, updatedOK := chapter["updatedAt"].(string)
	id, idOK := chapter["id"].(string)
	if !ok || !updatedOK || !idOK {
		t.Fatalf("chapter response missing save fields: %#v", chapter)
	}
	return requestJSON(t, client, http.MethodPut, baseURL+"/api/v1/novels/"+novelID+"/chapters/"+id, map[string]any{
		"expectedRevision":  int(revision),
		"expectedUpdatedAt": updatedAt,
		"title":             title,
		"body":              body,
		"goal":              "",
		"keyEvents":         "",
		"suspenseHook":      "",
	}, http.StatusOK)
}

type exportResult struct {
	status      int
	contentType string
	body        []byte
}

func exportNovel(t *testing.T, client *http.Client, baseURL, novelID, format string) exportResult {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/novels/"+novelID+"/export?format="+format, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("export %s: status=%d body=%q", format, response.StatusCode, body)
	}
	return exportResult{status: response.StatusCode, contentType: response.Header.Get("Content-Type"), body: body}
}

func TestSaaSNovelAuthoringFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "authoring.sqlite")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := task.NewManager(db, testGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, []string{"*"}, "")
	webServer := newTLSServer(t, application.Handler())
	defer webServer.Close()
	defer func() {
		// Wait for any background tasks to finish writing to database
		time.Sleep(50 * time.Millisecond)
		db.Close()
	}()

	ownerClient := newClient(t, webServer)
	requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "owner@example.test", "password": "safe-test-password-123"}, http.StatusCreated)
	profile := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)

	novel := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "雾港来信", "summary": "一封迟到的信改变了渡口小镇。", "genre": "悬疑", "plannedChapters": 12, "approvalMode": "manual",
	}, http.StatusCreated)
	novelID := novel["id"].(string)
	chapterResponse := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)
	chapter := chapterResponse["chapters"].([]any)[0].(map[string]any)
	chapterID := chapter["id"].(string)
	requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/export?format=pdf", nil, http.StatusBadRequest)
	exportRequest, err := http.NewRequest(http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/export?format=md", nil)
	if err != nil {
		t.Fatal(err)
	}
	exportResponse, err := ownerClient.Do(exportRequest)
	if err != nil {
		t.Fatal(err)
	}
	exportBody, err := io.ReadAll(exportResponse.Body)
	exportResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if exportResponse.StatusCode != http.StatusOK || exportResponse.Header.Get("Content-Type") != "text/markdown; charset=utf-8" || !strings.Contains(string(exportBody), "# 雾港来信") {
		t.Fatalf("unexpected markdown export: status=%d type=%q body=%q", exportResponse.StatusCode, exportResponse.Header.Get("Content-Type"), string(exportBody))
	}

	firstNode := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/graph/nodes", map[string]any{"kind": "character", "label": "沈知遥", "summary": "在渡口经营旧书店。", "x": 80, "y": 80}, http.StatusCreated)
	secondNode := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/graph/nodes", map[string]any{"kind": "place", "label": "雾港", "summary": "潮雾常年不散。", "x": 300, "y": 80}, http.StatusCreated)
	edge := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/graph/edges", map[string]string{"sourceId": firstNode["id"].(string), "targetId": secondNode["id"].(string), "relation": "居住于"}, http.StatusCreated)
	requestJSON(t, ownerClient, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID+"/graph/nodes/"+firstNode["id"].(string), map[string]any{"kind": "character", "label": "沈知遥", "summary": "经营旧书店，也暗中调查失踪案。", "x": 80, "y": 80}, http.StatusOK)
	requestJSON(t, ownerClient, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID+"/graph/edges/"+edge["id"].(string), map[string]string{"sourceId": firstNode["id"].(string), "targetId": secondNode["id"].(string), "relation": "扎根于", "summary": "她的书店位于港口附近。"}, http.StatusOK)
	updatedGraph := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/graph", nil, http.StatusOK)
	if updatedGraph["nodes"].([]any)[0].(map[string]any)["summary"] != "经营旧书店，也暗中调查失踪案。" {
		t.Fatal("updated graph node should be returned in the knowledge graph")
	}
	if updatedGraph["edges"].([]any)[0].(map[string]any)["relation"] != "扎根于" {
		t.Fatal("updated graph relation should be returned in the knowledge graph")
	}

	workflow := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/workflows", map[string]any{
		"name": "分章写作",
		"definition": map[string]any{
			"nodes": []map[string]any{{"id": "context", "kind": "context", "title": "读取设定", "prompt": "使用图谱实体。", "x": 0, "y": 0}, {"id": "write", "kind": "generate", "title": "写作", "prompt": "保持悬疑氛围。", "x": 250, "y": 0}},
			"edges": []map[string]string{{"id": "edge-1", "source": "context", "target": "write"}},
		},
	}, http.StatusCreated)
	workflowID := workflow["id"].(string)
	removableWorkflow := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/workflows", map[string]any{
		"name": "临时流程",
		"definition": map[string]any{
			"nodes": []map[string]any{{"id": "context", "kind": "context", "title": "读取设定", "prompt": "", "x": 0, "y": 0}, {"id": "output", "kind": "output", "title": "输出", "prompt": "", "x": 250, "y": 0}},
			"edges": []map[string]string{{"id": "connect", "source": "context", "target": "output"}},
		},
	}, http.StatusCreated)
	requestJSON(t, ownerClient, http.MethodDelete, webServer.URL+"/api/v1/novels/"+novelID+"/workflows/"+removableWorkflow["id"].(string), nil, http.StatusOK)
	if count := len(requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/workflows", nil, http.StatusOK)["workflows"].([]any)); count != 1 {
		t.Fatalf("deleted workflow should be removed from the list, got %d workflows", count)
	}
	workflowTask := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/workflows/"+workflowID+"/run", map[string]any{"chapterIds": []string{chapterID}, "instruction": "按流程完成章节。"}, http.StatusAccepted)
	waitForTaskStatus(t, ownerClient, webServer.URL, workflowTask["id"].(string), "completed")

	secondClient := newClient(t, webServer)
	requestJSON(t, secondClient, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "reader@example.test", "password": "safe-test-password-456"}, http.StatusCreated)
	requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/workspaces/"+workspaceID+"/members", map[string]string{"email": "reader@example.test", "role": "viewer"}, http.StatusCreated)
	requestJSON(t, secondClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/graph/nodes", map[string]any{"kind": "character", "label": "越权写入", "summary": "", "x": 0, "y": 0}, http.StatusForbidden)
	requestJSON(t, secondClient, http.MethodDelete, webServer.URL+"/api/v1/novels/"+novelID+"/workflows/"+workflowID, nil, http.StatusForbidden)
	requestJSON(t, ownerClient, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID, map[string]any{
		"expectedRevision": int(chapter["revision"].(float64)), "title": chapter["title"], "body": chapter["body"],
		"goal": "沈知遥在雾港追查失踪案。", "keyEvents": chapter["keyEvents"], "suspenseHook": chapter["suspenseHook"],
	}, http.StatusOK)

	taskResponse := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{"chapterIds": []string{chapterID}, "instruction": "续写一个雨夜相遇的开场。"}, http.StatusAccepted)
	taskID := taskResponse["id"].(string)
	var taskSnapshot string
	if err := db.QueryRow(`SELECT request_json FROM tasks WHERE id = ?`, taskID).Scan(&taskSnapshot); err != nil {
		t.Fatal(err)
	}
	var taskRequest task.Request
	if err := json.Unmarshal([]byte(taskSnapshot), &taskRequest); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(taskRequest.Context, "暗中调查失踪案") || !strings.Contains(taskRequest.Context, "扎根于") {
		t.Fatal("new generation should use the latest edited graph entities and relations")
	}
	parsedURL, err := url.Parse(webServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	cookieValues := ownerClient.Jar.Cookies(parsedURL)
	cookieHeader := make([]string, 0, len(cookieValues))
	for _, cookie := range cookieValues {
		cookieHeader = append(cookieHeader, cookie.Name+"="+cookie.Value)
	}
	wsURL := "wss" + strings.TrimPrefix(webServer.URL, "https") + "/api/v1/tasks/" + taskID + "/ws?after=0"
	wsContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(wsContext, wsURL, &websocket.DialOptions{HTTPClient: ownerClient, HTTPHeader: http.Header{"Cookie": cookieHeader}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	seenToken := false
	seenCompleted := false
	var lastSequence int64
	for !seenCompleted {
		_, payload, err := connection.Read(wsContext)
		if err != nil {
			t.Fatal(err)
		}
		var event struct {
			Type     string `json:"type"`
			Sequence int64  `json:"sequence"`
		}
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		if event.Sequence <= lastSequence {
			t.Fatalf("WSS event sequence did not advance: %d after %d", event.Sequence, lastSequence)
		}
		lastSequence = event.Sequence
		seenToken = seenToken || event.Type == "task.token"
		seenCompleted = event.Type == "task.completed"
	}
	if !seenToken {
		t.Fatal("expected WSS token event")
	}
	replayURL := "wss" + strings.TrimPrefix(webServer.URL, "https") + "/api/v1/tasks/" + taskID + "/ws?after=" + strconv.FormatInt(lastSequence-1, 10)
	replayConnection, _, err := websocket.Dial(wsContext, replayURL, &websocket.DialOptions{HTTPClient: ownerClient, HTTPHeader: http.Header{"Cookie": cookieHeader}})
	if err != nil {
		t.Fatal(err)
	}
	defer replayConnection.CloseNow()
	_, replayPayload, err := replayConnection.Read(wsContext)
	if err != nil {
		t.Fatal(err)
	}
	var replayedEvent struct {
		Type     string `json:"type"`
		Sequence int64  `json:"sequence"`
	}
	if err := json.Unmarshal(replayPayload, &replayedEvent); err != nil {
		t.Fatal(err)
	}
	if replayedEvent.Type != "task.completed" || replayedEvent.Sequence != lastSequence {
		t.Fatalf("expected WSS cursor replay of terminal event, got %#v", replayedEvent)
	}

	proposalResponse := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/proposals", nil, http.StatusOK)
	proposal := proposalResponse["proposals"].([]any)[0].(map[string]any)
	requestJSON(t, secondClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/proposals/"+proposal["id"].(string)+"/regenerate", map[string]any{}, http.StatusForbidden)
	requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/proposals/"+proposal["id"].(string)+"/apply", map[string]any{}, http.StatusOK)
	revisions := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID+"/revisions", nil, http.StatusOK)
	if len(revisions["revisions"].([]any)) != 1 {
		t.Fatal("expected the pre-proposal chapter version to be preserved")
	}
	requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID+"/revisions/1/restore", map[string]any{}, http.StatusOK)
	requestJSON(t, ownerClient, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID, map[string]any{"expectedRevision": 1, "title": "第一章", "body": "旧版本"}, http.StatusConflict)
	currentChapter := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)[0].(map[string]any)
	baseUpdatedAt := currentChapter["updatedAt"].(string)
	baseRevision := int(currentChapter["revision"].(float64))
	time.Sleep(2 * time.Millisecond)
	requestJSON(t, ownerClient, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID, map[string]any{
		"expectedRevision": baseRevision, "expectedUpdatedAt": baseUpdatedAt, "title": currentChapter["title"], "body": currentChapter["body"],
		"goal": "蓝图更新", "keyEvents": currentChapter["keyEvents"], "suspenseHook": currentChapter["suspenseHook"],
	}, http.StatusOK)
	requestJSON(t, ownerClient, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID, map[string]any{
		"expectedRevision": baseRevision, "expectedUpdatedAt": baseUpdatedAt, "title": currentChapter["title"], "body": currentChapter["body"],
		"goal": "使用陈旧蓝图", "keyEvents": currentChapter["keyEvents"], "suspenseHook": currentChapter["suspenseHook"],
	}, http.StatusConflict)
	requestJSON(t, ownerClient, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID, map[string]any{"title": "雾港来信", "summary": "一封迟到的信改变了渡口小镇。", "genre": "悬疑", "plannedChapters": 12, "approvalMode": "auto"}, http.StatusOK)
	autoTask := requestJSON(t, ownerClient, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{"chapterIds": []string{chapterID}, "instruction": "继续这一章。"}, http.StatusAccepted)
	autoTaskID := autoTask["id"].(string)
	deadline := time.Now().Add(3 * time.Second)
	for {
		status := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/tasks/"+autoTaskID, nil, http.StatusOK)["status"]
		if status == "completed" {
			break
		}
		if status == "failed" || time.Now().After(deadline) {
			t.Fatalf("auto-approval task did not complete: %v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	autoChapters := requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)
	if autoChapters[0].(map[string]any)["body"] != "雨停后，渡口的灯一盏接一盏亮了起来。" {
		t.Fatal("auto-approval should apply generated chapter content")
	}
	requestJSON(t, ownerClient, http.MethodGet, webServer.URL+"/api/v1/tasks/"+taskID, nil, http.StatusOK)
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatal(err)
	}
}

func TestRetryTaskContinuesOnlyUnfinishedChapters(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "retry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	generator := &failOnceGenerator{}
	manager, err := task.NewManager(db, generator)
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	webServer := newTLSServer(t, application.Handler())
	defer webServer.Close()
	client := newClient(t, webServer)
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "retry@example.test", "password": "safe-test-password-789"}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	novel := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels", map[string]any{"workspaceId": workspaceID, "title": "重试测试", "plannedChapters": 2}, http.StatusCreated)
	novelID := novel["id"].(string)
	chapterResponse := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)
	firstChapterID := chapterResponse["chapters"].([]any)[0].(map[string]any)["id"].(string)
	secondChapter := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", map[string]string{"title": "第二章"}, http.StatusCreated)
	secondChapterID := secondChapter["id"].(string)

	originalTask := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{"chapterIds": []string{firstChapterID, secondChapterID}, "instruction": "按顺序生成两章。"}, http.StatusAccepted)
	originalTaskID := originalTask["id"].(string)
	waitForTaskStatus(t, client, webServer.URL, originalTaskID, "failed")
	initialProposals := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/proposals", nil, http.StatusOK)["proposals"].([]any)
	if len(initialProposals) != 1 || initialProposals[0].(map[string]any)["chapterId"] != secondChapterID {
		t.Fatalf("successful chapter should remain saved after another chapter fails: %#v", initialProposals)
	}

	retryTask := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/tasks/"+originalTaskID+"/retry", map[string]any{}, http.StatusAccepted)
	retryTaskID := retryTask["id"].(string)
	waitForTaskStatus(t, client, webServer.URL, retryTaskID, "completed")
	proposals := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/proposals", nil, http.StatusOK)["proposals"].([]any)
	if len(proposals) != 2 {
		t.Fatalf("retry should add only one proposal without duplicating successful work, got %d", len(proposals))
	}
	seen := map[string]int{}
	for _, rawProposal := range proposals {
		seen[rawProposal.(map[string]any)["chapterId"].(string)]++
	}
	if seen[firstChapterID] != 1 || seen[secondChapterID] != 1 {
		t.Fatalf("each chapter should have exactly one proposal: %#v", seen)
	}
	generator.mu.Lock()
	calls := generator.calls
	generator.mu.Unlock()
	if calls != 3 {
		t.Fatalf("retry should regenerate only the failed chapter, generator called %d times", calls)
	}
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/tasks/"+originalTaskID+"/retry", map[string]any{}, http.StatusConflict)
	events := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/tasks/"+originalTaskID+"/events?after=0", nil, http.StatusOK)["events"].([]any)
	chapterStatuses := map[string]string{}
	for _, rawEvent := range events {
		event := rawEvent.(map[string]any)
		if event["type"] == "chapter.failed" || event["type"] == "chapter.completed" {
			data := event["data"].(map[string]any)
			chapterStatuses[data["chapterId"].(string)] = event["type"].(string)
		}
	}
	if chapterStatuses[firstChapterID] != "chapter.failed" || chapterStatuses[secondChapterID] != "chapter.completed" {
		t.Fatalf("expected durable per-chapter outcomes, got %#v", chapterStatuses)
	}
}

func TestCancellationHTTPFlow(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "cancel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	started := make(chan struct{})
	manager, err := task.NewManager(db, blockingGenerator{started: started})
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	webServer := newTLSServer(t, application.Handler())
	defer webServer.Close()
	client := newClient(t, webServer)
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "cancel@example.test", "password": "safe-test-password-789"}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	novel := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels", map[string]any{"workspaceId": workspaceID, "title": "取消测试", "plannedChapters": 1}, http.StatusCreated)
	novelID := novel["id"].(string)
	chapterResponse := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)
	chapterID := chapterResponse["chapters"].([]any)[0].(map[string]any)["id"].(string)
	taskResponse := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{"chapterIds": []string{chapterID}, "instruction": "等候取消。"}, http.StatusAccepted)
	taskID := taskResponse["id"].(string)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("generation did not start")
	}
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/tasks/"+taskID+"/cancel", map[string]any{}, http.StatusAccepted)
	waitForTaskStatus(t, client, webServer.URL, taskID, "canceled")
	eventResponse := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/tasks/"+taskID+"/events?after=0", nil, http.StatusOK)
	foundCanceled := false
	for _, rawEvent := range eventResponse["events"].([]any) {
		if rawEvent.(map[string]any)["type"] == "task.canceled" {
			foundCanceled = true
		}
	}
	if !foundCanceled {
		t.Fatal("expected persisted task.canceled event")
	}
}

func waitForTaskStatus(t *testing.T, client *http.Client, baseURL, taskID, expected string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		result := requestJSON(t, client, http.MethodGet, baseURL+"/api/v1/tasks/"+taskID, nil, http.StatusOK)
		status := result["status"]
		if status == expected {
			return
		}
		if status == "failed" && expected != "failed" {
			t.Fatalf("task failed while waiting for %s", expected)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("task did not reach %s", expected)
}

func newTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	return server
}

func newClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: server.Client().Transport, Jar: jar}
}

func requestJSON(t *testing.T, client *http.Client, method, target string, body any, expectedStatus int) map[string]any {
	t.Helper()
	var requestBody *strings.Reader
	if body == nil {
		requestBody = strings.NewReader("")
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequest(method, target, requestBody)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expectedStatus {
		t.Fatalf("%s %s: got %d, want %d: %#v", method, target, response.StatusCode, expectedStatus, result)
	}
	return result
}
