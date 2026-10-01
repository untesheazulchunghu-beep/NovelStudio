package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"novelstudio/internal/database"
)

type waitingGenerator struct {
	started chan struct{}
}

func (g waitingGenerator) Generate(ctx context.Context, _ string, _ func(string)) (string, error) {
	close(g.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func TestCancelStopsRunningTask(t *testing.T) {
	db := openTaskTestDB(t)
	started := make(chan struct{})
	manager, err := NewManager(db, waitingGenerator{started: started})
	if err != nil {
		t.Fatal(err)
	}
	userID, novelID, taskID := seedTask(t, db, "queued")
	manager.Start(Request{ID: taskID, NovelID: novelID, UserID: userID, Instruction: "cancel test", ApprovalMode: "manual", Chapters: []Chapter{{ID: database.NewID(), Number: 1, Title: "第一章", Revision: 1}}})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("generation did not start")
	}
	if !manager.Cancel(taskID) {
		t.Fatal("running task should be cancellable")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		item, err := manager.Get(taskID)
		if err != nil {
			t.Fatal(err)
		}
		if item.Status == "canceled" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("task did not transition to canceled")
}

type sequenceGenerator struct {
	mu      sync.Mutex
	prompts chan string
	outputs []string
	index   int
}

func (g *sequenceGenerator) Generate(_ context.Context, prompt string, _ func(string)) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prompts <- prompt
	output := g.outputs[g.index]
	g.index++
	return output, nil
}

type demoGenerator struct{}

func (demoGenerator) Generate(_ context.Context, _ string, _ func(string)) (string, error) {
	return "演示占位文本", nil
}

func (demoGenerator) DemoMode() bool { return true }

type toggleModeGenerator struct {
	mu   sync.Mutex
	demo bool
}

func (g *toggleModeGenerator) Generate(ctx context.Context, prompt string, emit func(string)) (string, error) {
	text, err, _ := g.GenerateWithMode(ctx, prompt, emit)
	return text, err
}

func (g *toggleModeGenerator) GenerateWithMode(_ context.Context, _ string, _ func(string)) (string, error, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.demo {
		return "演示输出", nil, true
	}
	return "正式输出", nil, false
}

func (g *toggleModeGenerator) setDemo(demo bool) {
	g.mu.Lock()
	g.demo = demo
	g.mu.Unlock()
}

type streamingGenerator struct {
	chunks []string
}

func (g streamingGenerator) Generate(_ context.Context, _ string, emit func(string)) (string, error) {
	var output strings.Builder
	for _, chunk := range g.chunks {
		output.WriteString(chunk)
		emit(chunk)
	}
	return output.String(), nil
}

func TestDemoGenerationNeverAutoApplies(t *testing.T) {
	db := openTaskTestDB(t)
	userID, novelID, taskID := seedTask(t, db, "queued")
	now := database.Now()
	chapterID := database.NewID()
	if _, err := db.Exec(`UPDATE novels SET approval_mode = 'auto' WHERE id = ?`, novelID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chapters (id, novel_id, chapter_number, title, body, revision, updated_at) VALUES (?, ?, 1, '第一章', '原有正文', 1, ?)`, chapterID, novelID, now); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(db, demoGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start(Request{ID: taskID, NovelID: novelID, UserID: userID, Instruction: "生成", ApprovalMode: "auto", Chapters: []Chapter{{ID: chapterID, Number: 1, Title: "第一章", Body: "原有正文", Revision: 1}}})
	waitForTaskStatus(t, manager, taskID, "completed")

	var body string
	var revision int
	if err := db.QueryRow(`SELECT body, revision FROM chapters WHERE id = ?`, chapterID).Scan(&body, &revision); err != nil {
		t.Fatal(err)
	}
	if body != "原有正文" || revision != 1 {
		t.Fatalf("demo generation modified the manuscript: body=%q revision=%d", body, revision)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM proposals WHERE task_id = ?`, taskID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("demo proposal should require manual review, got %q", status)
	}
}

func TestDynamicModelSettingsControlAutomaticWritesSafely(t *testing.T) {
	db := openTaskTestDB(t)
	userID, novelID, demoTaskID := seedTask(t, db, "queued")
	_, _, liveTaskID := seedTask(t, db, "queued")
	chapterID := database.NewID()
	if _, err := db.Exec(`UPDATE novels SET approval_mode = 'auto' WHERE id = ?`, novelID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chapters (id, novel_id, chapter_number, title, body, revision, updated_at) VALUES (?, ?, 1, '第一章', '原有正文', 1, ?)`, chapterID, novelID, database.Now()); err != nil {
		t.Fatal(err)
	}
	generator := &toggleModeGenerator{demo: true}
	manager, err := NewManager(db, generator)
	if err != nil {
		t.Fatal(err)
	}

	request := Request{
		ID: demoTaskID, NovelID: novelID, UserID: userID, Instruction: "生成",
		ApprovalMode: "auto",
		Chapters:     []Chapter{{ID: chapterID, Number: 1, Title: "第一章", Body: "原有正文", Revision: 1}},
	}
	manager.Start(request)
	waitForTaskStatus(t, manager, demoTaskID, "completed")
	var proposalStatus string
	if err := db.QueryRow(`SELECT status FROM proposals WHERE task_id = ?`, demoTaskID).Scan(&proposalStatus); err != nil {
		t.Fatal(err)
	}
	if proposalStatus != "pending" {
		t.Fatalf("demo result should remain a proposal, got %q", proposalStatus)
	}

	generator.setDemo(false)
	request.ID = liveTaskID
	manager.Start(request)
	waitForTaskStatus(t, manager, liveTaskID, "completed")
	if err := db.QueryRow(`SELECT status FROM proposals WHERE task_id = ?`, liveTaskID).Scan(&proposalStatus); err != nil {
		t.Fatal(err)
	}
	var body string
	var revision int
	if err := db.QueryRow(`SELECT body, revision FROM chapters WHERE id = ?`, chapterID).Scan(&body, &revision); err != nil {
		t.Fatal(err)
	}
	if proposalStatus != "applied" || body != "正式输出" || revision != 2 {
		t.Fatalf("live provider should auto-apply after a UI config change: status=%q body=%q revision=%d", proposalStatus, body, revision)
	}

	_, _, clearedKeyTaskID := seedTask(t, db, "queued")
	generator.setDemo(true)
	request.ID = clearedKeyTaskID
	request.Chapters = []Chapter{{ID: chapterID, Number: 1, Title: "第一章", Body: "正式输出", Revision: 2}}
	manager.Start(request)
	waitForTaskStatus(t, manager, clearedKeyTaskID, "completed")
	if err := db.QueryRow(`SELECT status FROM proposals WHERE task_id = ?`, clearedKeyTaskID).Scan(&proposalStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT body, revision FROM chapters WHERE id = ?`, chapterID).Scan(&body, &revision); err != nil {
		t.Fatal(err)
	}
	if proposalStatus != "pending" || body != "正式输出" || revision != 2 {
		t.Fatalf("clearing the key should restore demo protection: proposal=%q body=%q revision=%d", proposalStatus, body, revision)
	}
}

func TestTokenEventsAreBatchedAndReplayable(t *testing.T) {
	db := openTaskTestDB(t)
	userID, novelID, taskID := seedTask(t, db, "queued")
	chapterID := database.NewID()
	if _, err := db.Exec(`INSERT INTO chapters (id, novel_id, chapter_number, title, updated_at) VALUES (?, ?, 1, '第一章', ?)`, chapterID, novelID, database.Now()); err != nil {
		t.Fatal(err)
	}
	chunks := make([]string, 600)
	for index := range chunks {
		chunks[index] = "字"
	}
	manager, err := NewManager(db, streamingGenerator{chunks: chunks})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start(Request{
		ID: taskID, NovelID: novelID, UserID: userID, Instruction: "生成",
		ApprovalMode: "manual",
		Chapters:     []Chapter{{ID: chapterID, Number: 1, Title: "第一章", Revision: 1}},
	})

	events := waitForTaskEvent(t, manager, taskID, "task.completed")
	tokenEvents := make([]Event, 0)
	var combined strings.Builder
	for _, event := range events {
		if event.Type != "task.token" {
			continue
		}
		tokenEvents = append(tokenEvents, event)
		var data struct {
			Text string `json:"text"`
		}
		payload, err := json.Marshal(event.Data)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(payload, &data); err != nil {
			t.Fatal(err)
		}
		combined.WriteString(data.Text)
	}
	if len(tokenEvents) == 0 || len(tokenEvents) >= len(chunks)/10 {
		t.Fatalf("expected batched token events, got %d events for %d chunks", len(tokenEvents), len(chunks))
	}
	if want := strings.Repeat("字", len(chunks)); combined.String() != want {
		t.Fatalf("replayed token text mismatch: got %d characters, want %d", len([]rune(combined.String())), len([]rune(want)))
	}
	for index := 1; index < len(events); index++ {
		if events[index].Sequence <= events[index-1].Sequence {
			t.Fatalf("event sequence is not increasing: %d then %d", events[index-1].Sequence, events[index].Sequence)
		}
	}
}

func TestEventSequenceResumesFromPersistedHistory(t *testing.T) {
	db := openTaskTestDB(t)
	_, _, taskID := seedTask(t, db, "completed")
	payload := `{"message":"old"}`
	if _, err := db.Exec(`INSERT INTO task_events (task_id, sequence, event_type, payload_json, created_at) VALUES (?, 7, 'task.old', ?, ?)`, taskID, payload, database.Now()); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(db, demoGenerator{})
	if err != nil {
		t.Fatal(err)
	}

	manager.emit(taskID, "task.new", map[string]any{"message": "new"})
	events, err := manager.EventsAfter(taskID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Sequence != 7 || events[1].Sequence != 8 {
		t.Fatalf("expected sequence to resume at 8, got %#v", events)
	}
}

func TestStartupPrunesOnlyExpiredTokenEvents(t *testing.T) {
	db := openTaskTestDB(t)
	_, _, expiredTaskID := seedTask(t, db, "completed")
	_, _, recentTaskID := seedTask(t, db, "completed")
	if _, err := db.Exec(`UPDATE tasks SET updated_at = '2000-01-01T00:00:00Z' WHERE id = ?`, expiredTaskID); err != nil {
		t.Fatal(err)
	}
	for _, event := range []struct {
		taskID string
		seq    int
		kind   string
	}{
		{taskID: expiredTaskID, seq: 1, kind: "task.token"},
		{taskID: expiredTaskID, seq: 2, kind: "task.completed"},
		{taskID: recentTaskID, seq: 1, kind: "task.token"},
	} {
		if _, err := db.Exec(`INSERT INTO task_events (task_id, sequence, event_type, payload_json, created_at) VALUES (?, ?, ?, '{}', ?)`, event.taskID, event.seq, event.kind, database.Now()); err != nil {
			t.Fatal(err)
		}
	}
	manager, err := NewManager(db, demoGenerator{})
	if err != nil {
		t.Fatal(err)
	}

	expiredEvents, err := manager.EventsAfter(expiredTaskID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(expiredEvents) != 1 || expiredEvents[0].Type != "task.completed" {
		t.Fatalf("expired cleanup should preserve non-token events, got %#v", expiredEvents)
	}
	recentEvents, err := manager.EventsAfter(recentTaskID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recentEvents) != 1 || recentEvents[0].Type != "task.token" {
		t.Fatalf("recent token event should be retained, got %#v", recentEvents)
	}
}

func TestBatchGenerationUsesPreviousGeneratedChapter(t *testing.T) {
	db := openTaskTestDB(t)
	userID, novelID, taskID := seedTask(t, db, "queued")
	now := database.Now()
	chapters := []Chapter{
		{ID: database.NewID(), Number: 1, Title: "第一章", Body: "第一章旧正文", Revision: 1},
		{ID: database.NewID(), Number: 2, Title: "第二章", Body: "第二章旧正文", Revision: 1, PreviousTail: "第一章旧正文"},
	}
	for _, chapter := range chapters {
		if _, err := db.Exec(`INSERT INTO chapters (id, novel_id, chapter_number, title, body, revision, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, chapter.ID, novelID, chapter.Number, chapter.Title, chapter.Body, chapter.Revision, now); err != nil {
			t.Fatal(err)
		}
	}
	prompts := make(chan string, 2)
	manager, err := NewManager(db, &sequenceGenerator{prompts: prompts, outputs: []string{"第一章生成的新结尾", "第二章生成正文"}})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start(Request{ID: taskID, NovelID: novelID, UserID: userID, Instruction: "生成", ApprovalMode: "manual", Chapters: chapters})
	waitForTaskStatus(t, manager, taskID, "completed")
	<-prompts
	secondPrompt := <-prompts
	if !strings.Contains(secondPrompt, "第一章生成的新结尾") {
		t.Fatalf("second chapter prompt should use generated previous chapter, got:\n%s", secondPrompt)
	}
	if strings.Contains(secondPrompt, "第一章旧正文") {
		t.Fatalf("second chapter prompt should not use stale previous chapter text, got:\n%s", secondPrompt)
	}
}

func waitForTaskStatus(t *testing.T, manager *Manager, taskID, expected string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		item, err := manager.Get(taskID)
		if err != nil {
			t.Fatal(err)
		}
		if item.Status == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	item, err := manager.Get(taskID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("task did not transition to %s, got %s", expected, item.Status)
}

func TestStartupPersistsInterruptionEvent(t *testing.T) {
	db := openTaskTestDB(t)
	_, _, taskID := seedTask(t, db, "running")
	manager, err := NewManager(db, waitingGenerator{started: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	item, err := manager.Get(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "interrupted" {
		t.Fatalf("expected interrupted task, got %s", item.Status)
	}
	events, err := manager.EventsAfter(taskID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "task.interrupted" {
		t.Fatalf("expected durable interruption event, got %#v", events)
	}
}

func waitForTaskEvent(t *testing.T, manager *Manager, taskID, expected string) []Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, err := manager.EventsAfter(taskID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) > 0 && events[len(events)-1].Type == expected {
			return events
		}
		time.Sleep(5 * time.Millisecond)
	}
	events, err := manager.EventsAfter(taskID, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("task event %s was not persisted; got %#v", expected, events)
	return nil
}

func openTaskTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "tasks.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedTask(t *testing.T, db *sql.DB, status string) (string, string, string) {
	t.Helper()
	userID := database.NewID()
	workspaceID := database.NewID()
	novelID := database.NewID()
	taskID := database.NewID()
	now := database.Now()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`, []any{userID, userID + "@example.test", "hash", now}},
		{`INSERT INTO workspaces (id, name, owner_id, created_at) VALUES (?, ?, ?, ?)`, []any{workspaceID, "Test", userID, now}},
		{`INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)`, []any{workspaceID, userID, now}},
		{`INSERT INTO novels (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, []any{novelID, workspaceID, "Test novel", now, now}},
		{`INSERT INTO tasks (id, novel_id, kind, status, created_by, created_at, updated_at) VALUES (?, ?, 'batch', ?, ?, ?, ?)`, []any{taskID, novelID, status, userID, now, now}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	return userID, novelID, taskID
}
