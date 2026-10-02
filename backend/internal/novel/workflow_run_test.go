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
	"time"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

type workflowGenerator struct {
	mu             sync.Mutex
	prompts        []string
	reviewErr      bool
	reviewErrAfter int // 1-based review visit at which later reviews return an error; 0 disables
	reviewVisits   int
	verdicts       []string
	chapter        string
	demo           bool
	block          chan struct{}
	started        chan struct{}
	blockKind      string
}

func (g *workflowGenerator) DemoMode() bool { return g.demo }

func (g *workflowGenerator) Generate(ctx context.Context, prompt string, emit func(string)) (string, error) {
	g.mu.Lock()
	g.prompts = append(g.prompts, prompt)
	reviewErr := g.reviewErr
	reviewVisits := 0
	if strings.Contains(prompt, "工作流节点（review）") {
		g.reviewVisits++
		reviewVisits = g.reviewVisits
	}
	reviewErrAfter := g.reviewErrAfter
	verdict := ""
	if strings.Contains(prompt, "工作流节点（review）") && len(g.verdicts) > 0 {
		verdict = g.verdicts[0]
		g.verdicts = g.verdicts[1:]
	}
	chapter := g.chapter
	if chapter == "" {
		chapter = "节点正文甲"
	}
	blockKind := g.blockKind
	started := g.started
	block := g.block
	g.mu.Unlock()

	if block != nil && strings.Contains(prompt, "工作流节点（"+blockKind+"）") {
		if started != nil {
			select {
			case <-started:
			default:
				close(started)
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-block:
		}
	}
	if strings.Contains(prompt, "请只输出章节记忆") {
		text := "摘要：自动记忆\n出场人物：甲\n新增设定：乙\n伏笔变化：\n"
		emit(text)
		return text, nil
	}
	if strings.Contains(prompt, "工作流节点（review）") {
		if reviewErr || (reviewErrAfter > 0 && reviewVisits >= reviewErrAfter) {
			return "", errors.New("review exploded")
		}
		if verdict == "" {
			verdict = "审稿结论：通过"
		}
		emit(verdict)
		return verdict, nil
	}
	emit(chapter)
	return chapter, nil
}

func (g *workflowGenerator) snapshot() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.prompts...)
}

func (g *workflowGenerator) setReviewErr(value bool) {
	g.mu.Lock()
	g.reviewErr = value
	g.mu.Unlock()
}

func (g *workflowGenerator) setVerdicts(values ...string) {
	g.mu.Lock()
	g.verdicts = append([]string(nil), values...)
	g.mu.Unlock()
}

type workflowHarness struct {
	server  *httptest.Server
	client  *http.Client
	db      *sql.DB
	novelID string
	chapter string
	gen     *workflowGenerator
}

func openWorkflowHarness(t *testing.T, approval string, gen *workflowGenerator) *workflowHarness {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "workflow.sqlite"))
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
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "workflow-" + approval + "@example.test", "password": "safe-test-password-123"}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	novel := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "节点试作", "summary": "用于节点执行", "genre": "测试", "plannedChapters": 3, "approvalMode": approval,
	}, http.StatusCreated)
	novelID := novel["id"].(string)
	chapters := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)
	chapterID := chapters["chapters"].([]any)[0].(map[string]any)["id"].(string)
	return &workflowHarness{server: webServer, client: client, db: db, novelID: novelID, chapter: chapterID, gen: gen}
}

func (h *workflowHarness) createWorkflow(t *testing.T, body map[string]any) string {
	t.Helper()
	created := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/novels/"+h.novelID+"/workflows", body, http.StatusCreated)
	return created["id"].(string)
}

func (h *workflowHarness) runWorkflow(t *testing.T, workflowID, instruction string) string {
	t.Helper()
	taskItem := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/novels/"+h.novelID+"/workflows/"+workflowID+"/run", map[string]any{
		"chapterIds": []string{h.chapter}, "instruction": instruction,
	}, http.StatusAccepted)
	return taskItem["id"].(string)
}

func (h *workflowHarness) events(t *testing.T, taskID string) []map[string]any {
	t.Helper()
	payload := requestJSON(t, h.client, http.MethodGet, h.server.URL+"/api/v1/tasks/"+taskID+"/events", nil, http.StatusOK)
	raw := payload["events"].([]any)
	events := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		events = append(events, item.(map[string]any))
	}
	return events
}

func (h *workflowHarness) storedRequest(t *testing.T, taskID string) task.Request {
	t.Helper()
	var raw string
	if err := h.db.QueryRow(`SELECT request_json FROM tasks WHERE id = ?`, taskID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var request task.Request
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		t.Fatal(err)
	}
	return request
}

func countKind(prompts []string, kind string) int {
	count := 0
	needle := "工作流节点（" + kind + "）"
	for _, prompt := range prompts {
		if strings.Contains(prompt, "请只输出章节记忆") {
			continue
		}
		if strings.Contains(prompt, needle) {
			count++
		}
	}
	return count
}

func linkedWorkflow(nodes ...map[string]any) map[string]any {
	edges := make([]map[string]string, 0, len(nodes)-1)
	for index := 0; index < len(nodes)-1; index++ {
		edges = append(edges, map[string]string{
			"id":     "edge-" + nodes[index]["id"].(string),
			"source": nodes[index]["id"].(string),
			"target": nodes[index+1]["id"].(string),
		})
	}
	return map[string]any{"name": "测试流程", "definition": map[string]any{"nodes": nodes, "edges": edges}}
}

func TestWorkflowNodesRunSeparately(t *testing.T) {
	gen := &workflowGenerator{}
	h := openWorkflowHarness(t, "manual", gen)
	workflowID := h.createWorkflow(t, linkedWorkflow(
		map[string]any{"id": "context", "kind": "context", "title": "读取设定", "prompt": "资料锚点甲", "x": 0, "y": 0},
		map[string]any{"id": "draft", "kind": "generate", "title": "起草", "prompt": "写出正文", "x": 10, "y": 0},
		map[string]any{"id": "review", "kind": "review", "title": "审稿", "prompt": "检查正文", "x": 20, "y": 0},
		map[string]any{"id": "output", "kind": "output", "title": "输出", "prompt": "保存正文", "x": 30, "y": 0},
	))
	manualTask := h.runWorkflow(t, workflowID, "按节点写这一章")
	waitForTaskStatus(t, h.client, h.server.URL, manualTask, "completed")

	prompts := gen.snapshot()
	if countKind(prompts, "context") != 0 {
		t.Fatalf("context node called the generator: %#v", prompts)
	}
	if countKind(prompts, "generate") != 1 || countKind(prompts, "review") != 1 {
		t.Fatalf("generate and review should each run once, prompts=%#v", prompts)
	}
	if !strings.Contains(prompts[0], "资料锚点甲") {
		t.Fatalf("generate prompt should contain the context result, got %q", prompts[0])
	}
	if !strings.Contains(prompts[1], "节点正文甲") {
		t.Fatalf("review prompt should contain the generate result, got %q", prompts[1])
	}
	var body string
	var revision int
	if err := h.db.QueryRow(`SELECT body, revision FROM chapters WHERE id = ?`, h.chapter).Scan(&body, &revision); err != nil {
		t.Fatal(err)
	}
	if body != "" || revision != 1 {
		t.Fatalf("manual approval changed the chapter: body=%q revision=%d", body, revision)
	}
	var proposalStatus, proposalText string
	if err := h.db.QueryRow(`SELECT status, proposed_text FROM proposals WHERE task_id = ?`, manualTask).Scan(&proposalStatus, &proposalText); err != nil {
		t.Fatal(err)
	}
	if proposalStatus != "pending" || proposalText != "节点正文甲" {
		t.Fatalf("manual output = %s %q", proposalStatus, proposalText)
	}
	var memories int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM chapter_memories WHERE chapter_id = ?`, h.chapter).Scan(&memories); err != nil {
		t.Fatal(err)
	}
	if memories != 0 {
		t.Fatal("manual workflow stored a chapter memory")
	}

	requestJSON(t, h.client, http.MethodPut, h.server.URL+"/api/v1/novels/"+h.novelID, map[string]any{
		"title": "节点试作", "summary": "用于节点执行", "genre": "测试", "plannedChapters": 3, "approvalMode": "auto",
	}, http.StatusOK)
	gen.demo = true
	before := len(gen.snapshot())
	demoTask := h.runWorkflow(t, workflowID, "演示模式再写一次")
	waitForTaskStatus(t, h.client, h.server.URL, demoTask, "completed")
	if err := h.db.QueryRow(`SELECT body, revision FROM chapters WHERE id = ?`, h.chapter).Scan(&body, &revision); err != nil {
		t.Fatal(err)
	}
	var demoStatus string
	if err := h.db.QueryRow(`SELECT status FROM proposals WHERE task_id = ?`, demoTask).Scan(&demoStatus); err != nil {
		t.Fatal(err)
	}
	if body != "" || revision != 1 || demoStatus != "pending" {
		t.Fatalf("demo mode wrote the chapter: body=%q revision=%d proposal=%s", body, revision, demoStatus)
	}
	if countKind(gen.snapshot()[before:], "generate") != 1 {
		t.Fatalf("demo workflow should still generate once: %#v", gen.snapshot()[before:])
	}

	gen.demo = false
	autoTask := h.runWorkflow(t, workflowID, "自动写入这一章")
	waitForTaskStatus(t, h.client, h.server.URL, autoTask, "completed")
	if err := h.db.QueryRow(`SELECT body, revision FROM chapters WHERE id = ?`, h.chapter).Scan(&body, &revision); err != nil {
		t.Fatal(err)
	}
	var autoStatus, summary string
	if err := h.db.QueryRow(`SELECT status FROM proposals WHERE task_id = ?`, autoTask).Scan(&autoStatus); err != nil {
		t.Fatal(err)
	}
	if err := h.db.QueryRow(`SELECT summary FROM chapter_memories WHERE chapter_id = ?`, h.chapter).Scan(&summary); err != nil {
		t.Fatal(err)
	}
	if autoStatus != "applied" || body != "节点正文甲" || revision != 2 || !strings.Contains(summary, "自动记忆") {
		t.Fatalf("auto approval body=%q revision=%d proposal=%s summary=%q", body, revision, autoStatus, summary)
	}

	before = len(gen.snapshot())
	batch := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/novels/"+h.novelID+"/tasks/batch", map[string]any{
		"chapterIds": []string{h.chapter}, "instruction": "普通批量只生成一次",
	}, http.StatusAccepted)
	waitForTaskStatus(t, h.client, h.server.URL, batch["id"].(string), "completed")
	added := gen.snapshot()[before:]
	generations := 0
	for _, prompt := range added {
		if strings.Contains(prompt, "请只输出章节记忆") || strings.Contains(prompt, "工作流节点（") {
			continue
		}
		generations++
		if !strings.Contains(prompt, "普通批量只生成一次") {
			t.Fatalf("batch prompt lost its instruction: %q", prompt)
		}
	}
	if generations != 1 {
		t.Fatalf("batch should call the generator once, got %d from %#v", generations, added)
	}
}

func TestWorkflowNodeEventsCancelAndRetry(t *testing.T) {
	gen := &workflowGenerator{reviewErr: true, chapter: "已完成草稿"}
	h := openWorkflowHarness(t, "manual", gen)
	workflowID := h.createWorkflow(t, linkedWorkflow(
		map[string]any{"id": "context", "kind": "context", "title": "读取设定", "prompt": "资料锚点乙", "x": 0, "y": 0},
		map[string]any{"id": "draft", "kind": "generate", "title": "起草", "prompt": "写出正文", "x": 10, "y": 0},
		map[string]any{"id": "review", "kind": "review", "title": "审稿", "prompt": "检查正文", "x": 20, "y": 0},
		map[string]any{"id": "output", "kind": "output", "title": "输出", "prompt": "保存正文", "x": 30, "y": 0},
	))
	failedTask := h.runWorkflow(t, workflowID, "先写再审")
	waitForTaskStatus(t, h.client, h.server.URL, failedTask, "failed")
	seen := map[string]string{}
	started := map[string]bool{}
	for _, event := range h.events(t, failedTask) {
		data, _ := event["data"].(map[string]any)
		nodeID, _ := data["nodeId"].(string)
		switch event["type"] {
		case "node.started":
			started[nodeID] = true
		case "node.finished":
			seen[nodeID], _ = data["status"].(string)
		}
	}
	if !started["context"] || seen["context"] != "succeeded" || !started["draft"] || seen["draft"] != "succeeded" || !started["review"] || seen["review"] != "failed" || started["output"] {
		t.Fatalf("node events = started %#v finished %#v", started, seen)
	}
	stored := h.storedRequest(t, failedTask)
	var draftSaved, reviewSaved bool
	for _, result := range stored.NodeResults {
		if result.NodeID == "draft" && result.Status == "succeeded" && strings.Contains(result.Output, "已完成草稿") {
			draftSaved = true
		}
		if result.NodeID == "review" && result.Status == "succeeded" {
			reviewSaved = true
		}
	}
	if !draftSaved || reviewSaved {
		t.Fatalf("stored node results = %#v", stored.NodeResults)
	}

	gen.setReviewErr(false)
	before := len(gen.snapshot())
	retryTask := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/tasks/"+failedTask+"/retry", map[string]any{}, http.StatusAccepted)
	retryID := retryTask["id"].(string)
	waitForTaskStatus(t, h.client, h.server.URL, retryID, "completed")
	added := gen.snapshot()[before:]
	if countKind(added, "generate") != 0 || countKind(added, "review") != 1 {
		t.Fatalf("retry should run only the failed review, prompts=%#v", added)
	}
	if !strings.Contains(added[0], "已完成草稿") {
		t.Fatalf("retried review prompt lost the saved draft: %q", added[0])
	}
	reread := h.storedRequest(t, retryID)
	draftSaved = false
	for _, result := range reread.NodeResults {
		if result.NodeID == "draft" && result.Status == "succeeded" {
			draftSaved = true
		}
	}
	if !draftSaved {
		t.Fatalf("retry request read back from the database dropped the succeeded draft: %#v", reread.NodeResults)
	}

	blocker := &workflowGenerator{blockKind: "generate", started: make(chan struct{}), block: make(chan struct{})}
	cancelDB, err := database.Open(filepath.Join(t.TempDir(), "cancel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancelDB.Close() })
	manager, err := task.NewManager(cancelDB, blocker)
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(cancelDB, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	webServer := newTLSServer(t, application.Handler())
	client := newClient(t, webServer)
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{"email": "cancel-node@example.test", "password": "safe-test-password-123"}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	novel := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "取消试作", "summary": "取消", "genre": "测试", "plannedChapters": 1, "approvalMode": "manual",
	}, http.StatusCreated)
	novelID := novel["id"].(string)
	chapters := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)
	chapterID := chapters["chapters"].([]any)[0].(map[string]any)["id"].(string)
	created := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/workflows", linkedWorkflow(
		map[string]any{"id": "context", "kind": "context", "title": "读取设定", "prompt": "资料", "x": 0, "y": 0},
		map[string]any{"id": "draft", "kind": "generate", "title": "起草", "prompt": "写", "x": 10, "y": 0},
		map[string]any{"id": "review", "kind": "review", "title": "审稿", "prompt": "查", "x": 20, "y": 0},
	), http.StatusCreated)
	running := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/workflows/"+created["id"].(string)+"/run", map[string]any{
		"chapterIds": []string{chapterID}, "instruction": "写到一半取消",
	}, http.StatusAccepted)
	select {
	case <-blocker.started:
	case <-time.After(5 * time.Second):
		t.Fatal("generate node did not start")
	}
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/tasks/"+running["id"].(string)+"/cancel", map[string]any{}, http.StatusAccepted)
	waitForTaskStatus(t, client, webServer.URL, running["id"].(string), "canceled")
	payload := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/tasks/"+running["id"].(string)+"/events", nil, http.StatusOK)
	rawEvents := payload["events"].([]any)
	generateCanceled := false
	reviewStarted := false
	for _, item := range rawEvents {
		event := item.(map[string]any)
		data, _ := event["data"].(map[string]any)
		nodeID, _ := data["nodeId"].(string)
		if event["type"] == "node.started" && nodeID == "review" {
			reviewStarted = true
		}
		if event["type"] == "node.finished" && nodeID == "draft" && data["status"] == "canceled" {
			generateCanceled = true
		}
	}
	if !generateCanceled || reviewStarted {
		t.Fatalf("cancel should stop the running node and not start later nodes: %#v", rawEvents)
	}
	if chapterBody(t, cancelDB, chapterID) != "" {
		t.Fatal("canceled workflow changed the chapter")
	}
}

func chapterBody(t *testing.T, db *sql.DB, chapterID string) string {
	t.Helper()
	var body string
	if err := db.QueryRow(`SELECT body FROM chapters WHERE id = ?`, chapterID).Scan(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestWorkflowReviewBranchesBackToGenerate(t *testing.T) {
	gen := &workflowGenerator{chapter: "修改后正文", verdicts: []string{"审稿结论：不通过\n理由：缺结尾", "审稿结论：通过"}}
	h := openWorkflowHarness(t, "manual", gen)
	workflowID := h.createWorkflow(t, linkedWorkflow(
		map[string]any{"id": "context", "kind": "context", "title": "读取设定", "prompt": "资料", "x": 0, "y": 0},
		map[string]any{"id": "draft", "kind": "generate", "title": "起草", "prompt": "写出正文", "x": 10, "y": 0},
		map[string]any{"id": "review", "kind": "review", "title": "审稿", "prompt": "检查正文", "x": 20, "y": 0},
		map[string]any{"id": "output", "kind": "output", "title": "输出", "prompt": "保存正文", "x": 30, "y": 0},
	))
	taskID := h.runWorkflow(t, workflowID, "写完再改")
	waitForTaskStatus(t, h.client, h.server.URL, taskID, "completed")
	prompts := gen.snapshot()
	generatePrompts := promptsOfKind(prompts, "generate")
	if len(generatePrompts) != 2 || countKind(prompts, "review") != 2 {
		t.Fatalf("fail then pass should rerun generate once, prompts=%#v", prompts)
	}
	if len(generatePrompts) > 1+task.MaxReviewReturns {
		t.Fatalf("returns exceeded the maximum: %d", len(generatePrompts))
	}
	if !strings.Contains(generatePrompts[1], "审稿结论：不通过") || !strings.Contains(generatePrompts[1], "缺结尾") {
		t.Fatalf("repeated generate prompt missing the review: %q", generatePrompts[1])
	}
	var proposal string
	if err := h.db.QueryRow(`SELECT proposed_text FROM proposals WHERE task_id = ?`, taskID).Scan(&proposal); err != nil {
		t.Fatal(err)
	}
	if proposal != "修改后正文" {
		t.Fatalf("pass should reach output, proposal=%q", proposal)
	}

	limitGen := &workflowGenerator{chapter: "反复修改", verdicts: repeatVerdict("审稿结论：不通过\n仍不行", 1+task.MaxReviewReturns)}
	limit := openWorkflowHarness(t, "manual", limitGen)
	limitID := limit.createWorkflow(t, linkedWorkflow(
		map[string]any{"id": "draft", "kind": "generate", "title": "起草", "prompt": "写出正文", "x": 0, "y": 0},
		map[string]any{"id": "review", "kind": "review", "title": "审稿", "prompt": "检查正文", "x": 10, "y": 0},
		map[string]any{"id": "output", "kind": "output", "title": "输出", "prompt": "保存正文", "x": 20, "y": 0},
	))
	stopped := limit.runWorkflow(t, limitID, "一直不通过")
	waitForTaskStatus(t, limit.client, limit.server.URL, stopped, "failed")
	item := requestJSON(t, limit.client, http.MethodGet, limit.server.URL+"/api/v1/tasks/"+stopped, nil, http.StatusOK)
	if item["error"] != task.ReviewLimitMessage {
		t.Fatalf("review limit was not visible on the task: %#v", item["error"])
	}
	if countKind(limitGen.snapshot(), "generate") != 1+task.MaxReviewReturns {
		t.Fatalf("generate ran %d times, want %d: %#v", countKind(limitGen.snapshot(), "generate"), 1+task.MaxReviewReturns, limitGen.snapshot())
	}
	failedVisible := false
	for _, event := range limit.events(t, stopped) {
		if event["type"] != "node.finished" {
			continue
		}
		data, _ := event["data"].(map[string]any)
		if data["status"] == "failed" && data["message"] == task.ReviewLimitMessage {
			failedVisible = true
		}
	}
	if !failedVisible {
		t.Fatal("review limit was not visible in task events")
	}
	var proposals int
	if err := limit.db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE task_id = ?`, stopped).Scan(&proposals); err != nil {
		t.Fatal(err)
	}
	if proposals != 0 {
		t.Fatal("a stopped review stored a proposal")
	}
}

func promptsOfKind(prompts []string, kind string) []string {
	needle := "工作流节点（" + kind + "）"
	matched := make([]string, 0)
	for _, prompt := range prompts {
		if strings.Contains(prompt, needle) && !strings.Contains(prompt, "请只输出章节记忆") {
			matched = append(matched, prompt)
		}
	}
	return matched
}

func repeatVerdict(text string, count int) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = text
	}
	return values
}

func TestWorkflowFailedReviewIsRetriedAfterGeneratorError(t *testing.T) {
	gen := &workflowGenerator{
		chapter:        "改写正文",
		verdicts:       []string{"审稿结论：不通过\n理由：缺结尾"},
		reviewErrAfter: 2,
	}
	h := openWorkflowHarness(t, "manual", gen)
	workflowID := h.createWorkflow(t, linkedWorkflow(
		map[string]any{"id": "context", "kind": "context", "title": "读取设定", "prompt": "资料", "x": 0, "y": 0},
		map[string]any{"id": "draft", "kind": "generate", "title": "起草", "prompt": "写出正文", "x": 10, "y": 0},
		map[string]any{"id": "review", "kind": "review", "title": "审稿", "prompt": "检查正文", "x": 20, "y": 0},
		map[string]any{"id": "output", "kind": "output", "title": "输出", "prompt": "保存正文", "x": 30, "y": 0},
	))
	failedTask := h.runWorkflow(t, workflowID, "不通过后再审")
	waitForTaskStatus(t, h.client, h.server.URL, failedTask, "failed")
	firstRun := gen.snapshot()
	generatePrompts := promptsOfKind(firstRun, "generate")
	if len(generatePrompts) != 2 || countKind(firstRun, "review") != 2 {
		t.Fatalf("one fail should rewrite once before the review error, prompts=%#v", firstRun)
	}
	if !strings.Contains(generatePrompts[1], "审稿结论：不通过") || !strings.Contains(generatePrompts[1], "缺结尾") {
		t.Fatalf("rewrite prompt missing the fail text: %q", generatePrompts[1])
	}
	stored := h.storedRequest(t, failedTask)
	var draftSaved bool
	for _, result := range stored.NodeResults {
		if result.NodeID == "review" && result.Status == "succeeded" {
			t.Fatalf("a failing review was stored as a skippable success: %#v", stored.NodeResults)
		}
		if result.NodeID == "draft" && result.Status == "succeeded" && strings.Contains(result.Output, "改写正文") {
			draftSaved = true
		}
	}
	if !draftSaved {
		t.Fatalf("rewrite was not saved for retry: %#v", stored.NodeResults)
	}

	before := len(gen.snapshot())
	retryTask := requestJSON(t, h.client, http.MethodPost, h.server.URL+"/api/v1/tasks/"+failedTask+"/retry", map[string]any{}, http.StatusAccepted)
	retryID := retryTask["id"].(string)
	waitForTaskStatus(t, h.client, h.server.URL, retryID, "failed")
	added := gen.snapshot()[before:]
	if countKind(added, "review") != 1 || countKind(added, "generate") != 0 {
		t.Fatalf("retry should run only the review, prompts=%#v", added)
	}
	if !strings.Contains(added[0], "改写正文") {
		t.Fatalf("retried review prompt lost the rewrite: %q", added[0])
	}
	var proposals int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE novel_id = ?`, h.novelID).Scan(&proposals); err != nil {
		t.Fatal(err)
	}
	if proposals != 0 {
		t.Fatal("unreviewed rewrite was stored as a proposal")
	}
}

func TestBuiltinWorkflowTemplates(t *testing.T) {
	gen := &workflowGenerator{chapter: "模板正文", verdicts: []string{"审稿结论：通过"}}
	h := openWorkflowHarness(t, "manual", gen)
	draftID := h.createWorkflow(t, map[string]any{"template": "章节起草"})
	draftTask := h.runWorkflow(t, draftID, "用起草模板")
	waitForTaskStatus(t, h.client, h.server.URL, draftTask, "completed")
	if countKind(gen.snapshot(), "context") != 0 || countKind(gen.snapshot(), "generate") != 1 || countKind(gen.snapshot(), "review") != 0 {
		t.Fatalf("章节起草 call pattern = %#v", gen.snapshot())
	}

	reviewID := h.createWorkflow(t, map[string]any{"template": "起草 → 审稿 → 修改"})
	before := len(gen.snapshot())
	reviewTask := h.runWorkflow(t, reviewID, "用审稿模板")
	waitForTaskStatus(t, h.client, h.server.URL, reviewTask, "completed")
	added := gen.snapshot()[before:]
	if countKind(added, "generate") != 1 || countKind(added, "review") != 1 {
		t.Fatalf("起草 → 审稿 → 修改 call pattern = %#v", added)
	}

	polishID := h.createWorkflow(t, map[string]any{"template": "只润色"})
	before = len(gen.snapshot())
	polishTask := h.runWorkflow(t, polishID, "用润色模板")
	waitForTaskStatus(t, h.client, h.server.URL, polishTask, "completed")
	added = gen.snapshot()[before:]
	if countKind(added, "generate") != 1 || countKind(added, "review") != 0 {
		t.Fatalf("只润色 call pattern = %#v", added)
	}
	if !strings.Contains(added[0], "只润色当前草稿，保持情节不变。") {
		t.Fatalf("polish prompt missing its node instruction: %q", added[0])
	}
	var proposals int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE novel_id = ?`, h.novelID).Scan(&proposals); err != nil {
		t.Fatal(err)
	}
	if proposals != 3 {
		t.Fatalf("each template should store one proposal, got %d", proposals)
	}
}
