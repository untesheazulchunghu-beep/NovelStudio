package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func postChatForTest(t *testing.T, handler http.Handler, body string) ChatResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Workspace-ID", localWorkspaceID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("chat request returned %d: %s", response.Code, response.Body.String())
	}
	var chat ChatResponse
	if err := json.NewDecoder(response.Body).Decode(&chat); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	return chat
}

func TestChatRequestIDsReturnDistinctConflictCodes(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	handler := NewServer(store, "").Handler()
	cases := []struct {
		requestID        string
		existingMessage  string
		submittedMessage string
		fingerprint      string
		wantCode         string
	}{
		{requestID: "legacy-id", existingMessage: "same question", submittedMessage: "same question", fingerprint: "", wantCode: "chat_request_unverifiable"},
		{requestID: "reused-id", existingMessage: "old question", submittedMessage: "new question", fingerprint: "stored-fingerprint", wantCode: "chat_request_conflict"},
	}
	for _, testCase := range cases {
		if _, err := store.AppendChat(context.Background(), localWorkspaceID, testCase.requestID, testCase.existingMessage, "old reply", "write", testCase.fingerprint); err != nil {
			t.Fatalf("seed chat history: %v", err)
		}
		body, err := json.Marshal(ChatRequest{RequestID: testCase.requestID, Message: testCase.submittedMessage, Mode: "write"})
		if err != nil {
			t.Fatalf("encode chat request: %v", err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Workspace-ID", localWorkspaceID)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusConflict {
			t.Fatalf("request %q returned %d: %s", testCase.requestID, response.Code, response.Body.String())
		}
		var payload map[string]string
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("decode conflict response: %v", err)
		}
		if payload["code"] != testCase.wantCode {
			t.Fatalf("request %q conflict code = %q, want %q", testCase.requestID, payload["code"], testCase.wantCode)
		}
	}
}

func TestCanceledChatRequestDoesNotPersistMessages(t *testing.T) {
	modelStarted := make(chan struct{})
	modelRelease := make(chan struct{})
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(modelStarted)
		<-modelRelease
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"test reply"}}]}`))
	}))
	defer modelServer.Close()
	defer close(modelRelease)
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	if _, err := store.UpdateSettings(localWorkspaceID, SettingsInput{Provider: "compatible", BaseURL: modelServer.URL + "/v1", Model: "test-model", Temperature: 0.5}); err != nil {
		t.Fatalf("configure test model: %v", err)
	}
	handler := NewServer(store, "").Handler()
	body, err := json.Marshal(ChatRequest{RequestID: "cancel-chat-1", Message: "请继续这个场景", Mode: "write", ChapterID: 2})
	if err != nil {
		t.Fatalf("encode chat request: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body))).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Workspace-ID", localWorkspaceID)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-modelStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("chat request did not reach the model server")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled chat handler did not return")
	}
	workspace, exists := store.Snapshot(localWorkspaceID)
	if !exists {
		t.Fatal("workspace disappeared after cancellation")
	}
	if len(workspace.Chat) != 0 {
		t.Fatalf("canceled chat request persisted messages: %+v", workspace.Chat)
	}
	persisted, err := loadWorkspace(store.db, localWorkspaceID)
	if err != nil {
		t.Fatalf("reload workspace after cancellation: %v", err)
	}
	if len(persisted.Chat) != 0 {
		t.Fatalf("canceled chat request persisted messages to SQLite: %+v", persisted.Chat)
	}
	var claimCount int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM chat_request_claims").Scan(&claimCount); err != nil {
		t.Fatalf("count claims after cancellation: %v", err)
	}
	if claimCount != 0 {
		t.Fatalf("canceled chat request left %d claim rows", claimCount)
	}
}

func TestChatModesReturnModeSpecificSuggestions(t *testing.T) {
	cases := []struct {
		mode              string
		instructionMarker string
		suggestions       []string
	}{
		{mode: "write", instructionMarker: "写作模式", suggestions: []string{"续写下一段", "提炼本章摘要", "检查角色动机"}},
		{mode: "plan", instructionMarker: "规划模式", suggestions: []string{"把这个冲突拆成三个节拍", "为本章补一条伏笔", "检查角色动机"}},
		{mode: "review", instructionMarker: "审阅模式", suggestions: []string{"检查节奏和信息密度", "找出一个可删句子", "生成更有画面感的版本"}},
		{mode: "edit", instructionMarker: "改写模式", suggestions: []string{"改得更克制一些", "增强画面感", "缩短这段文字"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.mode, func(t *testing.T) {
			if got := normalizeWritingMode(testCase.mode); got != testCase.mode {
				t.Fatalf("normalized mode = %q, want %q", got, testCase.mode)
			}
			if instruction := writingModeInstruction(testCase.mode); !strings.Contains(instruction, testCase.instructionMarker) {
				t.Fatalf("instruction %q does not contain mode marker %q", instruction, testCase.instructionMarker)
			}
			got := suggestionsFor(testCase.mode)
			if len(got) != len(testCase.suggestions) {
				t.Fatalf("suggestions = %v, want %v", got, testCase.suggestions)
			}
			for index := range got {
				if got[index] != testCase.suggestions[index] {
					t.Fatalf("suggestions = %v, want %v", got, testCase.suggestions)
				}
			}
		})
	}
}

func TestEditModeRequiresNonWhitespaceSelection(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	handler := NewServer(store, "").Handler()
	requestIDs := []string{"missing-selection", "whitespace-selection"}
	selections := []string{"", "   "}
	for index, selection := range selections {
		body, err := json.Marshal(ChatRequest{RequestID: requestIDs[index], Message: "请改写", Mode: "edit", ChapterID: 2, Selection: selection})
		if err != nil {
			t.Fatalf("encode chat request: %v", err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Workspace-ID", localWorkspaceID)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("edit request with selection %q returned %d: %s", selection, response.Code, response.Body.String())
		}
		if messages, exists := store.Snapshot(localWorkspaceID); !exists || len(messages.Chat) != 0 {
			t.Fatalf("invalid edit request was persisted: %+v", messages.Chat)
		}
	}
}

func TestBuildChatPromptLabelsSelectedPassageContext(t *testing.T) {
	workspace := seedWorkspace()
	chapter := workspace.Chapters[0]
	input := ChatRequest{
		Mode:      "edit",
		Context:   "选区前文：潮声盖过脚步。选区后文：钟楼亮起一扇窗。",
		Selection: "她展开信纸。",
		Message:   "改得更克制。",
	}
	prompt := buildChatPrompt(workspace, chapter, input)
	for _, expected := range []string{"当前模式：edit", "改写模式", "当前章节选区附近的正文上下文（选区正文另附）", input.Context, input.Selection} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt is missing %q", expected)
		}
	}
}
