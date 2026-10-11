package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestChatRequestIDDeduplicatesAfterStoreRestart(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "novelstudio.db")
	store, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	handler := NewServer(store, "").Handler()
	requestBody := `{"request_id":"retry-check-1","message":"帮我规划一个开篇","mode":"plan","chapter_id":0}`
	first := postChatForTest(t, handler, requestBody)
	second := postChatForTest(t, handler, requestBody)
	if first.UserMessage.ID == 0 || first.Message.ID == 0 {
		t.Fatalf("chat response is missing persisted message IDs: %+v", first)
	}
	if first.UserMessage.ID != second.UserMessage.ID || first.Message.ID != second.Message.ID {
		t.Fatalf("retry returned different message IDs: first=%+v second=%+v", first, second)
	}
	if messages, exists := store.Snapshot(localWorkspaceID); !exists || len(messages.Chat) != 2 {
		t.Fatalf("expected one persisted Q&A pair, got %+v", messages.Chat)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store before restart: %v", err)
	}

	reopened, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	retriedAfterRestart := postChatForTest(t, NewServer(reopened, "").Handler(), requestBody)
	if retriedAfterRestart.UserMessage.ID != first.UserMessage.ID || retriedAfterRestart.Message.ID != first.Message.ID {
		t.Fatalf("restart retry returned different message IDs: first=%+v restarted=%+v", first, retriedAfterRestart)
	}
	if messages, exists := reopened.Snapshot(localWorkspaceID); !exists || len(messages.Chat) != 2 {
		t.Fatalf("expected one persisted Q&A pair after restart, got %+v", messages.Chat)
	}
}

func TestChatRequestResultSurvivesChatHistoryPruning(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	handler := NewServer(store, "").Handler()
	requestID := "kept-after-pruning"
	requestBody := fmt.Sprintf(
		`{"request_id":%q,"message":"请规划一个开篇","mode":"plan","chapter_id":0}`,
		requestID,
	)
	first := postChatForTest(t, handler, requestBody)
	for index := 0; index < 31; index++ {
		id := fmt.Sprintf("fill-%02d", index)
		if _, err := store.AppendChat(context.Background(), localWorkspaceID, id, "后续问题", "后续回复", "write", "fill-fingerprint"); err != nil {
			t.Fatalf("append history item %d: %v", index, err)
		}
	}
	workspace, exists := store.Snapshot(localWorkspaceID)
	if !exists || len(workspace.Chat) != 60 {
		t.Fatalf("chat history has %d messages, want 60", len(workspace.Chat))
	}
	for _, message := range workspace.Chat {
		if message.RequestID == requestID {
			t.Fatal("original request unexpectedly remains in the bounded chat history")
		}
	}

	replayed := postChatForTest(t, handler, requestBody)
	if replayed.UserMessage.ID != first.UserMessage.ID || replayed.Message.ID != first.Message.ID || replayed.Message.Content != first.Message.Content {
		t.Fatalf("retry did not return the original response: first=%+v replayed=%+v", first, replayed)
	}
	workspace, exists = store.Snapshot(localWorkspaceID)
	if !exists || len(workspace.Chat) != 60 {
		t.Fatalf("retry changed bounded history size: %+v", workspace.Chat)
	}
}

func TestChatRequestResultBackfillsDuringSchemaUpgrade(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "novelstudio.db")
	store, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	storeClosed := false
	defer func() {
		if !storeClosed {
			_ = store.Close()
		}
	}()

	input := ChatRequest{RequestID: "legacy-chat-result", Message: "继续这个场景", Mode: "write", ChapterID: 2}
	fingerprint, err := fingerprintChatRequest(input)
	if err != nil {
		t.Fatalf("fingerprint legacy request: %v", err)
	}
	messages, err := store.AppendChat(context.Background(), localWorkspaceID, input.RequestID, input.Message, "已保存的旧回复", input.Mode, fingerprint)
	if err != nil {
		t.Fatalf("seed legacy chat messages: %v", err)
	}
	if _, err := store.db.Exec("DROP TABLE chat_requests"); err != nil {
		t.Fatalf("remove new ledger table before upgrade: %v", err)
	}
	if _, err := store.db.Exec("PRAGMA user_version = 3"); err != nil {
		t.Fatalf("mark database as previous schema: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close pre-upgrade store: %v", err)
	}
	storeClosed = true

	reopened, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open upgraded store: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	body := `{"request_id":"legacy-chat-result","message":"继续这个场景","mode":"write","chapter_id":2}`
	replayed := postChatForTest(t, NewServer(reopened, "").Handler(), body)
	if replayed.UserMessage.ID != messages[0].ID || replayed.Message.ID != messages[1].ID || replayed.Message.Content != "已保存的旧回复" {
		t.Fatalf("schema migration did not preserve the old result: messages=%+v replayed=%+v", messages, replayed)
	}
}

func TestChatRequestIDWithIncompleteLegacyMessageDoesNotRegenerate(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	requestID := "legacy-partial-chat"
	if _, err := store.AppendChat(context.Background(), localWorkspaceID, requestID, "继续这个场景", "", "write", ""); err != nil {
		t.Fatalf("seed incomplete legacy request: %v", err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"request_id":"legacy-partial-chat","message":"继续这个场景","mode":"write","chapter_id":2}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Workspace-ID", localWorkspaceID)
	NewServer(store, "").Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("incomplete legacy request returned %d: %s", response.Code, response.Body.String())
	}
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode incomplete request response: %v", err)
	}
	if payload["code"] != "chat_request_unverifiable" {
		t.Fatalf("incomplete request error code = %q, want chat_request_unverifiable", payload["code"])
	}
	workspace, exists := store.Snapshot(localWorkspaceID)
	if !exists || len(workspace.Chat) != 1 {
		t.Fatalf("incomplete request unexpectedly generated a new chat pair: %+v", workspace.Chat)
	}
}

func TestChatRequestLockIsSharedAcrossStoresForSameDatabase(t *testing.T) {
	modelRequests := make(chan struct{}, 2)
	modelRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseModel := func() { releaseOnce.Do(func() { close(modelRelease) }) }
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		modelRequests <- struct{}{}
		<-modelRelease
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"shared-store reply"}}]}`))
	}))
	defer modelServer.Close()
	defer releaseModel()

	databasePath := filepath.Join(t.TempDir(), "novelstudio.db")
	store, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	defer func() { _ = store.Close() }()
	if _, err := store.UpdateSettings(localWorkspaceID, SettingsInput{Provider: "compatible", BaseURL: modelServer.URL + "/v1", Model: "test-model", Temperature: 0.5}); err != nil {
		t.Fatalf("configure test model: %v", err)
	}
	secondStore, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open second store for the same database: %v", err)
	}
	defer func() { _ = secondStore.Close() }()

	requestBody := `{"request_id":"shared-server-request","message":"请继续","mode":"write","chapter_id":2}`
	newRequest := func() *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(requestBody))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Workspace-ID", localWorkspaceID)
		return request
	}
	firstHandler := NewServer(store, "").Handler()
	secondServer := NewServer(secondStore, "")
	secondServer.chatRequestLocks = newChatRequestLockManager()
	secondHandler := secondServer.Handler()
	firstResponse := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() { firstHandler.ServeHTTP(firstResponse, newRequest()); close(firstDone) }()
	select {
	case <-modelRequests:
	case <-time.After(5 * time.Second):
		t.Fatal("first chat request did not reach the model")
	}

	secondResponse := httptest.NewRecorder()
	secondDone := make(chan struct{})
	go func() { secondHandler.ServeHTTP(secondResponse, newRequest()); close(secondDone) }()
	select {
	case <-modelRequests:
		t.Fatal("second Store called the model instead of waiting on the shared database lock")
	case <-time.After(50 * time.Millisecond):
	}
	releaseModel()
	select {
	case <-firstDone:
	case <-time.After(5 * time.Second):
		t.Fatal("first chat request did not finish")
	}
	select {
	case <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("second chat request did not reuse the saved reply")
	}
	if firstResponse.Code != http.StatusOK || secondResponse.Code != http.StatusOK {
		t.Fatalf("chat handlers returned statuses %d and %d", firstResponse.Code, secondResponse.Code)
	}
	var first, second ChatResponse
	if err := json.NewDecoder(firstResponse.Body).Decode(&first); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if err := json.NewDecoder(secondResponse.Body).Decode(&second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if first.UserMessage.ID != second.UserMessage.ID || first.Message.ID != second.Message.ID {
		t.Fatalf("handlers returned different persisted messages: first=%+v second=%+v", first, second)
	}
	var claimCount int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM chat_request_claims").Scan(&claimCount); err != nil {
		t.Fatalf("count completed request claims: %v", err)
	}
	if claimCount != 0 {
		t.Fatalf("completed request left %d durable claims", claimCount)
	}
}

func TestChatAppendsAcrossStoresUseFreshSQLiteState(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "novelstudio.db")
	firstStore, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	defer func() { _ = firstStore.Close() }()
	secondStore, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	defer func() { _ = secondStore.Close() }()
	project, exists := firstStore.Snapshot(localWorkspaceID)
	if !exists {
		t.Fatal("default workspace is missing")
	}
	if _, err := firstStore.UpdateProject(localWorkspaceID, ProjectInput{Title: "Changed after the second Store opened", Subtitle: project.Project.Subtitle, Genre: project.Project.Genre, Premise: project.Project.Premise, WordTarget: project.Project.WordTarget}); err != nil {
		t.Fatalf("update project from first Store: %v", err)
	}
	stores := []*Store{firstStore, secondStore}
	requestIDs := []string{"parallel-chat-one", "parallel-chat-two"}
	users := []string{"第一条并发请求", "第二条并发请求"}
	owners := make([]string, len(stores))
	for index, store := range stores {
		fingerprint, err := fingerprintChatRequest(ChatRequest{RequestID: requestIDs[index], Message: users[index], Mode: "write"})
		if err != nil {
			t.Fatalf("fingerprint request %d: %v", index, err)
		}
		_, owner, claimed, err := store.ClaimChatRequest(context.Background(), localWorkspaceID, requestIDs[index], users[index], fingerprint)
		if err != nil || !claimed {
			t.Fatalf("claim request %d: claimed=%v err=%v", index, claimed, err)
		}
		owners[index] = owner
	}
	type appendResult struct {
		response ChatResponse
		err      error
	}
	start := make(chan struct{})
	results := make(chan appendResult, len(stores))
	for index, store := range stores {
		go func(index int, store *Store) {
			<-start
			fingerprint, _ := fingerprintChatRequest(ChatRequest{RequestID: requestIDs[index], Message: users[index], Mode: "write"})
			response, err := store.AppendChatRequest(context.Background(), localWorkspaceID, requestIDs[index], users[index], "回复"+users[index], "write", fingerprint, "local", owners[index])
			results <- appendResult{response: response, err: err}
		}(index, store)
	}
	close(start)
	messageIDs := make(map[int64]struct{}, len(stores)*2)
	for range stores {
		result := <-results
		if result.err != nil {
			t.Fatalf("append concurrent chat request: %v", result.err)
		}
		for _, id := range []int64{result.response.UserMessage.ID, result.response.Message.ID} {
			if id == 0 {
				t.Fatal("chat append returned a message without a database ID")
			}
			if _, exists := messageIDs[id]; exists {
				t.Fatalf("concurrent chat appends reused message ID %d", id)
			}
			messageIDs[id] = struct{}{}
		}
	}
	var title string
	if err := firstStore.db.QueryRow("SELECT title FROM workspaces WHERE workspace_id = ?", localWorkspaceID).Scan(&title); err != nil {
		t.Fatalf("read project title after chat appends: %v", err)
	}
	if title != "Changed after the second Store opened" {
		t.Fatalf("chat append overwrote a newer project title with %q", title)
	}
	var requestMessageCount int
	if err := firstStore.db.QueryRow("SELECT COUNT(*) FROM chat_messages WHERE workspace_id = ? AND request_id IN (?, ?)", localWorkspaceID, requestIDs[0], requestIDs[1]).Scan(&requestMessageCount); err != nil {
		t.Fatalf("count appended chat requests: %v", err)
	}
	if requestMessageCount != len(stores) {
		t.Fatalf("persisted %d request messages, want %d", requestMessageCount, len(stores))
	}
}

func TestChatRequestClaimExpiresAndOwnerReleaseIsScoped(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "novelstudio.db")
	firstStore, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	defer func() { _ = firstStore.Close() }()
	secondStore, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	defer func() { _ = secondStore.Close() }()

	requestID := "lease-takeover"
	user := "请继续这个场景"
	fingerprint, err := fingerprintChatRequest(ChatRequest{RequestID: requestID, Message: user, Mode: "write", ChapterID: 2})
	if err != nil {
		t.Fatalf("fingerprint request: %v", err)
	}
	_, firstOwner, claimed, err := firstStore.ClaimChatRequest(context.Background(), localWorkspaceID, requestID, user, fingerprint)
	if err != nil || !claimed || firstOwner == "" {
		t.Fatalf("first claim = (%q, %v, %v), want an owned claim", firstOwner, claimed, err)
	}

	if _, err := tryClaimChatRequestDB(secondStore.db, localWorkspaceID, requestID, "different content", "different-fingerprint", "other-owner", time.Now()); !errors.Is(err, ErrChatRequestConflict) {
		t.Fatalf("mismatched claim error = %v, want ErrChatRequestConflict", err)
	}
	if _, err := firstStore.db.Exec(`UPDATE chat_request_claims SET lease_until = 0 WHERE workspace_id = ? AND request_id = ?`, localWorkspaceID, requestID); err != nil {
		t.Fatalf("expire first claim: %v", err)
	}
	_, secondOwner, claimed, err := secondStore.ClaimChatRequest(context.Background(), localWorkspaceID, requestID, user, fingerprint)
	if err != nil || !claimed || secondOwner == "" || secondOwner == firstOwner {
		t.Fatalf("replacement claim = (%q, %v, %v), want a new owner", secondOwner, claimed, err)
	}
	if err := firstStore.ReleaseChatRequestClaim(localWorkspaceID, requestID, firstOwner); err != nil {
		t.Fatalf("release expired owner claim: %v", err)
	}
	var storedOwner string
	if err := secondStore.db.QueryRow(`SELECT owner_token FROM chat_request_claims WHERE workspace_id = ? AND request_id = ?`, localWorkspaceID, requestID).Scan(&storedOwner); err != nil {
		t.Fatalf("read replacement claim: %v", err)
	}
	if storedOwner != secondOwner {
		t.Fatalf("old owner released replacement claim: got %q, want %q", storedOwner, secondOwner)
	}
	if _, err := firstStore.AppendChatRequest(context.Background(), localWorkspaceID, requestID, user, "stale reply", "write", fingerprint, "local", firstOwner); !errors.Is(err, ErrChatRequestClaimLost) {
		t.Fatalf("expired owner append error = %v, want ErrChatRequestClaimLost", err)
	}
	if err := secondStore.ReleaseChatRequestClaim(localWorkspaceID, requestID, secondOwner); err != nil {
		t.Fatalf("release replacement claim: %v", err)
	}
}

func TestChatRequestClaimWaitRespectsCancellation(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "novelstudio.db")
	firstStore, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	defer func() { _ = firstStore.Close() }()
	secondStore, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	defer func() { _ = secondStore.Close() }()

	requestID := "lease-cancel-wait"
	user := "等待中的请求"
	fingerprint, err := fingerprintChatRequest(ChatRequest{RequestID: requestID, Message: user, Mode: "write"})
	if err != nil {
		t.Fatalf("fingerprint request: %v", err)
	}
	_, owner, claimed, err := firstStore.ClaimChatRequest(context.Background(), localWorkspaceID, requestID, user, fingerprint)
	if err != nil || !claimed {
		t.Fatalf("first claim failed: claimed=%v err=%v", claimed, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, _, _, err = secondStore.ClaimChatRequest(ctx, localWorkspaceID, requestID, user, fingerprint)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting claim error = %v, want context deadline exceeded", err)
	}
	if err := firstStore.ReleaseChatRequestClaim(localWorkspaceID, requestID, owner); err != nil {
		t.Fatalf("release owner claim: %v", err)
	}
}

func TestChatRequestLockWaitsForOwner(t *testing.T) {
	server := NewServer(nil, "")
	releaseOwner, err := server.acquireChatRequest(context.Background(), localWorkspaceID, "lock-test")
	if err != nil {
		t.Fatalf("acquire owner lock: %v", err)
	}
	defer releaseOwner()

	waiterStarted := make(chan struct{})
	waiterAcquired := make(chan func(), 1)
	waiterErrors := make(chan error, 1)
	go func() {
		close(waiterStarted)
		releaseWaiter, err := server.acquireChatRequest(context.Background(), localWorkspaceID, "lock-test")
		if err != nil {
			waiterErrors <- err
			return
		}
		waiterAcquired <- releaseWaiter
	}()
	<-waiterStarted
	select {
	case releaseWaiter := <-waiterAcquired:
		releaseWaiter()
		t.Fatal("second request acquired a lock while the owner was still active")
	case err := <-waiterErrors:
		t.Fatalf("waiter failed to acquire lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	releaseOwner()
	select {
	case releaseWaiter := <-waiterAcquired:
		releaseWaiter()
	case err := <-waiterErrors:
		t.Fatalf("waiter failed after owner released lock: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("waiter did not acquire lock after owner released it")
	}
}
