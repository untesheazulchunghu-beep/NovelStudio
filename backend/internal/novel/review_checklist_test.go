package novel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

type reviewScriptGenerator struct {
	mu                 sync.Mutex
	prompts            []string
	reply              string
	err                error
	failOnceContaining string
	failedOnce         bool
	demo               bool
}

func (g *reviewScriptGenerator) DemoMode() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.demo
}

func (g *reviewScriptGenerator) setDemo(value bool) {
	g.mu.Lock()
	g.demo = value
	g.mu.Unlock()
}

func (g *reviewScriptGenerator) Generate(_ context.Context, prompt string, emit func(string)) (string, error) {
	g.mu.Lock()
	if g.demo {
		g.mu.Unlock()
		return "开发演示模式：任务已进入生成流程。", nil
	}
	g.prompts = append(g.prompts, prompt)
	reply, err := g.reply, g.err
	if err == nil && !g.failedOnce && g.failOnceContaining != "" && strings.Contains(prompt, g.failOnceContaining) {
		err = errors.New("temporary review failure")
		g.failedOnce = true
	}
	g.mu.Unlock()
	if err != nil {
		return "", err
	}
	emit(reply)
	return reply, nil
}

func (g *reviewScriptGenerator) set(reply string, err error) {
	g.mu.Lock()
	g.reply, g.err = reply, err
	g.mu.Unlock()
}

func (g *reviewScriptGenerator) snapshot() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.prompts...)
}

type reviewHarness struct {
	server  *httptest.Server
	client  *http.Client
	db      *sql.DB
	novelID string
	chapter string
	gen     *reviewScriptGenerator
}

func openReviewHarness(t *testing.T, gen *reviewScriptGenerator) *reviewHarness {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "review.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	manager, err := task.NewManager(db, gen)
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	webServer := newTLSServer(t, application.Handler())
	client := newClient(t, webServer)
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "review-checklist@example.test", "password": "safe-test-password-123"}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	novel := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "审稿试作", "summary": "用于审稿清单", "genre": "测试", "plannedChapters": 1, "approvalMode": "auto",
	}, http.StatusCreated)
	novelID := novel["id"].(string)
	chapters := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)
	chapterID := chapters["chapters"].([]any)[0].(map[string]any)["id"].(string)
	return &reviewHarness{server: webServer, client: client, db: db, novelID: novelID, chapter: chapterID, gen: gen}
}

func (h *reviewHarness) putChapter(t *testing.T, body, keyEvents string) {
	t.Helper()
	current := requestJSON(t, h.client, http.MethodGet, h.server.URL+"/api/v1/novels/"+h.novelID+"/chapters", nil, http.StatusOK)
	chapter := current["chapters"].([]any)[0].(map[string]any)
	requestJSON(t, h.client, http.MethodPut, h.server.URL+"/api/v1/novels/"+h.novelID+"/chapters/"+h.chapter, map[string]any{
		"expectedRevision": chapter["revision"], "title": chapter["title"], "body": body,
		"goal": chapter["goal"], "keyEvents": keyEvents, "suspenseHook": chapter["suspenseHook"],
	}, http.StatusOK)
}

func (h *reviewHarness) startReview(t *testing.T, instruction string) string {
	t.Helper()
	item := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/novels/"+h.novelID+"/tasks/batch", map[string]any{
		"chapterIds": []string{h.chapter}, "instruction": instruction, "kind": "review",
	}, http.StatusAccepted)
	if item["kind"] != "review" {
		t.Fatalf("task kind = %#v", item["kind"])
	}
	return item["id"].(string)
}

func (h *reviewHarness) outcomes(t *testing.T, taskID string) []task.ReviewOutcome {
	t.Helper()
	var raw string
	if err := h.db.QueryRow(`SELECT request_json FROM tasks WHERE id = ?`, taskID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var request task.Request
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Nodes) != 0 {
		t.Fatalf("review task stored workflow nodes: %#v", request.Nodes)
	}
	return request.ReviewOutcomes
}

func (h *reviewHarness) chapterRow(t *testing.T) (string, int) {
	t.Helper()
	var body string
	var revision int
	if err := h.db.QueryRow(`SELECT body, revision FROM chapters WHERE id = ?`, h.chapter).Scan(&body, &revision); err != nil {
		t.Fatal(err)
	}
	return body, revision
}

func (h *reviewHarness) proposalCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE novel_id = ?`, h.novelID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (h *reviewHarness) eventTypes(t *testing.T, taskID string) []string {
	t.Helper()
	payload := requestJSON(t, h.client, http.MethodGet, h.server.URL+"/api/v1/tasks/"+taskID+"/events", nil, http.StatusOK)
	raw := payload["events"].([]any)
	types := make([]string, 0, len(raw))
	for _, item := range raw {
		event := item.(map[string]any)
		types = append(types, event["type"].(string))
	}
	return types
}

func findingByEvent(t *testing.T, outcomes []task.ReviewOutcome, event string) task.ReviewFinding {
	t.Helper()
	for _, outcome := range outcomes {
		for _, item := range outcome.Items {
			if item.Event == event {
				return item
			}
		}
	}
	t.Fatalf("missing finding %q in %#v", event, outcomes)
	return task.ReviewFinding{}
}

func TestReviewChecklistScoresKeyEvents(t *testing.T) {
	const body = "他在雨夜里推开了门。灯还亮着。"
	gen := &reviewScriptGenerator{reply: "" +
		"关键事件：雨夜进门\n判断：已完成\n引文：他在雨夜里推开了门\n位置：开头\n" +
		"关键事件：灯还亮着\n判断：未完成\n引文：灯还亮着\n位置：后一句\n" +
		"关键事件：缺了一字\n判断：通过\n引文：他在雨夜推开了门\n位置：无\n"}
	h := openReviewHarness(t, gen)
	h.putChapter(t, body, "雨夜进门\n\n灯还亮着\n缺了一字")
	beforeBody, beforeRevision := h.chapterRow(t)
	taskID := h.startReview(t, "核对这一章的关键事件")
	waitForTaskStatus(t, h.client, h.server.URL, taskID, "completed")

	prompts := gen.snapshot()
	if len(prompts) != 1 {
		t.Fatalf("review should call the generator once, prompts=%#v", prompts)
	}
	for _, expected := range []string{body, "雨夜进门", "灯还亮着", "缺了一字"} {
		if !strings.Contains(prompts[0], expected) {
			t.Fatalf("review prompt missing %q: %s", expected, prompts[0])
		}
	}
	if strings.Contains(prompts[0], "工作流节点（") {
		t.Fatalf("review prompt ran a workflow node: %s", prompts[0])
	}
	for _, eventType := range h.eventTypes(t, taskID) {
		if eventType == "node.started" || eventType == "proposal.created" || eventType == "proposal.applied" {
			t.Fatalf("review task emitted %s", eventType)
		}
	}
	outcomes := h.outcomes(t, taskID)
	if len(outcomes) != 1 || len(outcomes[0].Items) != 3 {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	done := findingByEvent(t, outcomes, "雨夜进门")
	if done.Status != "已完成" || done.ModelJudgment != "已完成" || !done.QuoteVerbatim || done.Quote != "他在雨夜里推开了门" || done.Location != "开头" {
		t.Fatalf("verbatim completion = %#v", done)
	}
	kept := findingByEvent(t, outcomes, "灯还亮着")
	if kept.Status != "未完成" || kept.ModelJudgment != "未完成" || !kept.QuoteVerbatim {
		t.Fatalf("verbatim quote must not become 已完成: %#v", kept)
	}
	fuzzy := findingByEvent(t, outcomes, "缺了一字")
	if fuzzy.Status != "待核实" || fuzzy.ModelJudgment != "已完成" || fuzzy.QuoteVerbatim {
		t.Fatalf("通过 with a missing character = %#v", fuzzy)
	}
	afterBody, afterRevision := h.chapterRow(t)
	if afterBody != beforeBody || afterRevision != beforeRevision {
		t.Fatalf("review changed the chapter from %q rev %d to %q rev %d", beforeBody, beforeRevision, afterBody, afterRevision)
	}
	if h.proposalCount(t) != 0 {
		t.Fatal("review stored a chapter proposal")
	}
}

func TestReviewChecklistCoversMissingForgedEmptyAndErrors(t *testing.T) {
	const body = "他在雨夜里推开了门。"
	gen := &reviewScriptGenerator{}
	h := openReviewHarness(t, gen)
	h.putChapter(t, body, "写到了进门\n没写的离别")
	beforeBody, beforeRevision := h.chapterRow(t)

	gen.set("关键事件：写到了进门\n判断：已完成\n引文：他在雨夜里推开了门\n位置：全文\n", nil)
	missingID := h.startReview(t, "看漏写")
	waitForTaskStatus(t, h.client, h.server.URL, missingID, "completed")
	missing := h.outcomes(t, missingID)
	omitted := findingByEvent(t, missing, "没写的离别")
	if omitted.Status != "未完成" || omitted.ModelJudgment != "" {
		t.Fatalf("omitted event = %#v", omitted)
	}
	if findingByEvent(t, missing, "写到了进门").Status != "已完成" {
		t.Fatalf("covered event = %#v", missing)
	}

	h.putChapter(t, body, "伪造事件")
	gen.set("关键事件：伪造事件\n判断：已完成\n引文：这段正文里根本没有\n位置：虚构\n", nil)
	forgedID := h.startReview(t, "看伪造引文")
	waitForTaskStatus(t, h.client, h.server.URL, forgedID, "completed")
	forged := findingByEvent(t, h.outcomes(t, forgedID), "伪造事件")
	if forged.Status != "待核实" || forged.ModelJudgment != "已完成" || forged.QuoteVerbatim {
		t.Fatalf("forged quote = %#v", forged)
	}

	callsBeforeEmpty := len(gen.snapshot())
	h.putChapter(t, body, " \n\n")
	gen.set("关键事件：编造的事件\n判断：已完成\n引文：他在雨夜里推开了门\n位置：全文\n", nil)
	emptyID := h.startReview(t, "没有关键事件")
	waitForTaskStatus(t, h.client, h.server.URL, emptyID, "completed")
	if len(gen.snapshot()) != callsBeforeEmpty {
		t.Fatalf("empty blueprint called the generator: %#v", gen.snapshot())
	}
	emptyOutcomes := h.outcomes(t, emptyID)
	if len(emptyOutcomes) != 1 || len(emptyOutcomes[0].Items) != 0 {
		t.Fatalf("empty blueprint = %#v", emptyOutcomes)
	}
	sawChecklist := false
	for _, eventType := range h.eventTypes(t, emptyID) {
		if eventType == "review.checklist" {
			sawChecklist = true
		}
		if eventType == "node.started" {
			t.Fatal("empty review started a workflow node")
		}
	}
	if !sawChecklist {
		t.Fatal("empty blueprint did not publish an empty checklist")
	}

	h.putChapter(t, body, "仍要审的事件")
	gen.set("", errors.New("review exploded"))
	failedID := h.startReview(t, "生成器报错")
	waitForTaskStatus(t, h.client, h.server.URL, failedID, "failed")
	failed := requestJSON(t, h.client, http.MethodGet, h.server.URL+"/api/v1/tasks/"+failedID, nil, http.StatusOK)
	if failed["error"] == "" {
		t.Fatalf("generator error was not visible: %#v", failed)
	}
	if len(h.outcomes(t, failedID)) != 0 {
		t.Fatalf("failed review left a checklist: %#v", h.outcomes(t, failedID))
	}
	for _, eventType := range h.eventTypes(t, failedID) {
		if eventType == "review.checklist" {
			t.Fatal("failed review published a checklist")
		}
	}

	afterBody, afterRevision := h.chapterRow(t)
	if afterBody != beforeBody || afterRevision != beforeRevision {
		t.Fatalf("review cases changed the chapter from %q rev %d to %q rev %d", beforeBody, beforeRevision, afterBody, afterRevision)
	}
	if h.proposalCount(t) != 0 {
		t.Fatal("review cases stored a chapter proposal")
	}
}

func TestReviewChecklistRequiresAConfiguredModel(t *testing.T) {
	h := openReviewHarness(t, &reviewScriptGenerator{reply: "审稿结论：通过"})
	h.gen.setDemo(true)
	response := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/novels/"+h.novelID+"/tasks/batch", map[string]any{
		"chapterIds": []string{h.chapter}, "instruction": "审查关键事件", "kind": "review",
	}, http.StatusServiceUnavailable)
	if response["error"].(map[string]any)["code"] != "MODEL_NOT_CONFIGURED" {
		t.Fatalf("demo mode should explain that a real model is required for review, got %#v", response)
	}
}

func TestReviewRetrySkipsChaptersWithSavedOutcomes(t *testing.T) {
	gen := &reviewScriptGenerator{
		reply: "关键事件：第一章事件\n判断：已完成\n引文：第一正文\n位置：全文\n" +
			"关键事件：第二章事件\n判断：已完成\n引文：第二正文\n位置：全文\n",
		failOnceContaining: "第二正文",
	}
	h := openReviewHarness(t, gen)
	h.putChapter(t, "第一正文", "第一章事件")
	second := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/novels/"+h.novelID+"/chapters", map[string]string{
		"title": "第二章",
	}, http.StatusCreated)
	secondID := second["id"].(string)
	requestJSON(t, h.client, http.MethodPut, h.server.URL+"/api/v1/novels/"+h.novelID+"/chapters/"+secondID, map[string]any{
		"expectedRevision": second["revision"], "title": second["title"], "body": "第二正文",
		"goal": "", "keyEvents": "第二章事件", "suspenseHook": "",
	}, http.StatusOK)
	started := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/novels/"+h.novelID+"/tasks/batch", map[string]any{
		"chapterIds": []string{h.chapter, secondID}, "instruction": "逐章审稿", "kind": "review",
	}, http.StatusAccepted)
	originalID := started["id"].(string)
	waitForTaskStatus(t, h.client, h.server.URL, originalID, "failed")
	originalOutcomes := h.outcomes(t, originalID)
	if len(originalOutcomes) != 1 || originalOutcomes[0].ChapterID != h.chapter {
		t.Fatalf("first chapter should have a saved review before the second fails, got %#v", originalOutcomes)
	}
	if prompts := gen.snapshot(); len(prompts) != 2 {
		t.Fatalf("initial review should call the model once per chapter, got %#v", prompts)
	}

	retried := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/tasks/"+originalID+"/retry", map[string]any{}, http.StatusAccepted)
	retryID := retried["id"].(string)
	waitForTaskStatus(t, h.client, h.server.URL, retryID, "completed")
	prompts := gen.snapshot()
	if len(prompts) != 3 || !strings.Contains(prompts[2], "第二正文") || strings.Contains(prompts[2], "第一正文") {
		t.Fatalf("review retry should run only the unfinished chapter, prompts=%#v", prompts)
	}
	retryOutcomes := h.outcomes(t, retryID)
	if len(retryOutcomes) != 2 || retryOutcomes[0].ChapterID != h.chapter || retryOutcomes[1].ChapterID != secondID {
		t.Fatalf("retry should retain the earlier checklist and append the unfinished one once, got %#v", retryOutcomes)
	}
	if h.proposalCount(t) != 0 {
		t.Fatal("review retry stored chapter proposals")
	}
}
