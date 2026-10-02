package novel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

type countingMemoryGenerator struct {
	mu          sync.Mutex
	demo        bool
	memoryCalls int
}

func (g *countingMemoryGenerator) DemoMode() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.demo
}

func (g *countingMemoryGenerator) setDemo(demo bool) {
	g.mu.Lock()
	g.demo = demo
	g.mu.Unlock()
}

func (g *countingMemoryGenerator) Generate(_ context.Context, prompt string, emit func(string)) (string, error) {
	if g.DemoMode() {
		return "演示占位文本", nil
	}
	if strings.Contains(prompt, "请只输出章节记忆") {
		g.mu.Lock()
		g.memoryCalls++
		call := g.memoryCalls
		g.mu.Unlock()
		if call == 1 {
			return "摘要：第一版摘要\n出场人物：甲乙\n新增设定：新设定甲\n伏笔变化：\n+灯塔|甲|灯塔未明\n", nil
		}
		return "摘要：第二版摘要\n出场人物：丙丁\n新增设定：新设定乙\n伏笔变化：\n-灯塔\n", nil
	}
	emit("正式正文")
	return "正式正文", nil
}

func TestFinalizeStoresChapterMemory(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "memory.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	generator := &countingMemoryGenerator{}
	manager, err := task.NewManager(db, generator)
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	webServer := newTLSServer(t, application.Handler())
	defer webServer.Close()
	client := newClient(t, webServer)
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/auth/register", map[string]string{
		"email": "memory@example.test", "password": "safe-test-password-123",
	}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	novel := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "记忆测试", "plannedChapters": 2,
	}, http.StatusCreated)
	novelID := novel["id"].(string)
	second := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", map[string]string{"title": "第二章"}, http.StatusCreated)
	secondID := second["id"].(string)
	listed := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)
	var firstID string
	for _, raw := range listed {
		chapter := raw.(map[string]any)
		if int(chapter["number"].(float64)) == 1 {
			firstID = chapter["id"].(string)
		}
	}
	if firstID == "" || secondID == "" {
		t.Fatalf("missing chapters: first=%q second=%q", firstID, secondID)
	}

	manual := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{
		"chapterIds": []string{firstID}, "instruction": "写出第一章。",
	}, http.StatusAccepted)
	waitForTaskStatus(t, client, webServer.URL, manual["id"].(string), "completed")
	if _, _, _, _, ok := chapterMemory(t, db, firstID); ok {
		t.Fatal("unapplied chapter should not store memory")
	}
	if _, _, _, _, ok := chapterMemory(t, db, secondID); ok {
		t.Fatal("chapter that was never generated should not store memory")
	}

	proposal := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/proposals", nil, http.StatusOK)["proposals"].([]any)[0].(map[string]any)
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/proposals/"+proposal["id"].(string)+"/apply", map[string]any{}, http.StatusOK)
	summary, characters, settings, foreshadow, ok := chapterMemory(t, db, firstID)
	if !ok || summary != "第一版摘要" || characters != "甲乙" || settings != "新设定甲" || !strings.Contains(foreshadow, "+灯塔|甲|灯塔未明") {
		t.Fatalf("apply should store the four memory fields, got ok=%v summary=%q characters=%q settings=%q foreshadow=%q", ok, summary, characters, settings, foreshadow)
	}
	var memoryRevision, chapterRevision int
	if err := db.QueryRow(`SELECT source_revision FROM chapter_memories WHERE chapter_id = ?`, firstID).Scan(&memoryRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT revision FROM chapters WHERE id = ?`, firstID).Scan(&chapterRevision); err != nil {
		t.Fatal(err)
	}
	if memoryRevision != chapterRevision {
		t.Fatalf("finalized memory revision=%d, chapter revision=%d", memoryRevision, chapterRevision)
	}

	firstChapter := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)[0].(map[string]any)
	if firstChapter["memoryStatus"] != "current" {
		t.Fatalf("newly applied proposal memory should be current, got %v", firstChapter["memoryStatus"])
	}
	editedChapter := requestJSON(t, client, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+firstID, map[string]any{
		"expectedRevision": int(firstChapter["revision"].(float64)), "expectedUpdatedAt": firstChapter["updatedAt"],
		"title": firstChapter["title"], "body": "手工修改后的正文。", "goal": firstChapter["goal"],
		"keyEvents": firstChapter["keyEvents"], "suspenseHook": firstChapter["suspenseHook"],
	}, http.StatusOK)
	if editedChapter["memoryStatus"] != "stale" {
		t.Fatalf("editing finalized prose should mark its memory stale, got %v", editedChapter["memoryStatus"])
	}
	memories, err := task.LoadChapterMemories(context.Background(), db, novelID)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 0 {
		t.Fatalf("stale chapter memory must not enter generation context: %#v", memories)
	}
	refreshed := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+firstID+"/memory/refresh", map[string]any{}, http.StatusOK)
	if refreshed["memoryStatus"] != "current" {
		t.Fatalf("explicit memory refresh should mark the current revision, got %v", refreshed["memoryStatus"])
	}
	chapterHistory := requestJSON(t, client, http.MethodGet, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+firstID+"/revisions", nil, http.StatusOK)["revisions"].([]any)
	if len(chapterHistory) == 0 {
		t.Fatal("manual edit should create a revision that can be restored")
	}
	restoredChapter := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+firstID+"/revisions/"+strconv.Itoa(int(chapterHistory[0].(map[string]any)["revision"].(float64)))+"/restore", map[string]any{}, http.StatusOK)
	if restoredChapter["memoryStatus"] != "stale" {
		t.Fatalf("restoring a revision should invalidate its previous memory, got %v", restoredChapter["memoryStatus"])
	}
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/chapters/"+firstID+"/memory/refresh", map[string]any{}, http.StatusOK)

	requestJSON(t, client, http.MethodPut, webServer.URL+"/api/v1/novels/"+novelID, map[string]any{
		"title": "记忆测试", "summary": "", "genre": "", "plannedChapters": 2, "approvalMode": "auto",
	}, http.StatusOK)
	auto := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{
		"chapterIds": []string{firstID}, "instruction": "重写第一章。",
	}, http.StatusAccepted)
	waitForTaskStatus(t, client, webServer.URL, auto["id"].(string), "completed")
	summary, characters, settings, foreshadow, ok = chapterMemory(t, db, firstID)
	if !ok || summary != "第二版摘要" || characters != "丙丁" || settings != "新设定乙" || !strings.Contains(foreshadow, "-灯塔") {
		t.Fatalf("a later finalize should replace the current memory, got ok=%v summary=%q characters=%q settings=%q foreshadow=%q", ok, summary, characters, settings, foreshadow)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM chapter_memories WHERE novel_id = ?`, novelID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("expected one current memory row, got %d", rows)
	}

	generator.setDemo(true)
	demo := requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{
		"chapterIds": []string{secondID}, "instruction": "演示第二章。",
	}, http.StatusAccepted)
	waitForTaskStatus(t, client, webServer.URL, demo["id"].(string), "completed")
	var body string
	var revision int
	if err := db.QueryRow(`SELECT body, revision FROM chapters WHERE id = ?`, secondID).Scan(&body, &revision); err != nil {
		t.Fatal(err)
	}
	if body != "" || revision != 1 {
		t.Fatalf("demo mode should not auto-commit, body=%q revision=%d", body, revision)
	}
	if _, _, _, _, ok := chapterMemory(t, db, secondID); ok {
		t.Fatal("demo mode should not store memory for an unconfirmed generation")
	}
	var proposalStatus string
	if err := db.QueryRow(`SELECT status FROM proposals WHERE task_id = ?`, demo["id"].(string)).Scan(&proposalStatus); err != nil {
		t.Fatal(err)
	}
	if proposalStatus != "pending" {
		t.Fatalf("demo proposal should stay pending, got %q", proposalStatus)
	}
	generator.setDemo(false)
	requestJSON(t, client, http.MethodPost, webServer.URL+"/api/v1/novels/"+novelID+"/proposals/"+proposalIDForTask(t, db, demo["id"].(string))+"/apply", map[string]any{}, http.StatusOK)
	if _, _, _, _, ok := chapterMemory(t, db, secondID); ok {
		t.Fatal("applying a demo proposal should not store placeholder text as finalized memory")
	}
	summary, _, _, _, ok = chapterMemory(t, db, firstID)
	if !ok || summary != "第二版摘要" {
		t.Fatalf("demo generation should leave the finalized memory unchanged, got %q ok=%v", summary, ok)
	}
}

func proposalIDForTask(t *testing.T, db *sql.DB, taskID string) string {
	t.Helper()
	var proposalID string
	if err := db.QueryRow(`SELECT id FROM proposals WHERE task_id = ?`, taskID).Scan(&proposalID); err != nil {
		t.Fatal(err)
	}
	return proposalID
}

type scriptedMemoryGenerator struct {
	mu      sync.Mutex
	prompts []string
	memory  func(chapterNumber int) string
	prose   string
}

func (g *scriptedMemoryGenerator) Generate(_ context.Context, prompt string, emit func(string)) (string, error) {
	g.mu.Lock()
	g.prompts = append(g.prompts, prompt)
	g.mu.Unlock()
	if strings.Contains(prompt, "请只输出章节记忆") {
		return g.memory(chapterNumberFromPrompt(prompt)), nil
	}
	emit(g.prose)
	return g.prose, nil
}

func (g *scriptedMemoryGenerator) promptContaining(marker string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, prompt := range g.prompts {
		if strings.Contains(prompt, marker) && strings.Contains(prompt, "创作任务") && !strings.Contains(prompt, "请只输出章节记忆") {
			return prompt
		}
	}
	return ""
}

func TestGenerationPromptUsesMemoryGraphAndBudget(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "prompt.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	generator := &scriptedMemoryGenerator{
		prose:  strings.Repeat("文", 40),
		memory: promptTestMemory,
	}
	manager, err := task.NewManager(db, generator)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	userID, novelID, chapterIDs := seedMemoryNovel(t, db, "提示词测试", 10)
	if err := seedOutOfPrefixGraph(t, db, novelID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE chapters SET goal = ? WHERE id = ?`, "鹤归进入雾隐港", chapterIDs[10]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE novels SET approval_mode = 'auto' WHERE id = ?`, novelID); err != nil {
		t.Fatal(err)
	}

	finalize, failure := server.startGeneration(context.Background(), novelID, userID, generationInput{
		ChapterIDs: chapterIDs[1:10], Instruction: "定稿前九章",
	}, "batch", "", "")
	if failure != nil {
		t.Fatalf("finalize start failed: %s", failure.message)
	}
	waitForStoredTask(t, db, finalize.ID)
	if _, err := db.Exec(`UPDATE novels SET approval_mode = 'manual' WHERE id = ?`, novelID); err != nil {
		t.Fatal(err)
	}
	const budget = 2000
	server.SetContextBudget(budget)
	item, failure := server.startGeneration(context.Background(), novelID, userID, generationInput{
		ChapterIDs: []string{chapterIDs[10]}, Instruction: "写第十章",
	}, "batch", "", "")
	if failure != nil {
		t.Fatalf("startGeneration failed: %s", failure.message)
	}
	waitForStoredTask(t, db, item.ID)

	prompt := generator.promptContaining("目标章节：第10章")
	if prompt == "" {
		t.Fatal("chapter 10 generation prompt was not captured")
	}
	injected := injectedContext(t, prompt)
	if len([]rune(injected)) > budget {
		t.Fatalf("injected context is %d runes, budget is %d", len([]rune(injected)), budget)
	}
	for _, expected := range []string{"最近摘要锚点", "character：鹤归", "place：雾隐港", "驻留", "未拆信札"} {
		if !strings.Contains(injected, expected) {
			t.Fatalf("injected context should contain %q, got:\n%s", expected, injected)
		}
	}
	for _, absent := range []string{"人物000", "旧前缀关系", "已揭晓铜钥", "城南旧案编号ZX", "第一章窗外摘要", "冗"} {
		if strings.Contains(prompt, absent) {
			t.Fatalf("prompt should omit %q, got:\n%s", absent, prompt)
		}
	}

	manifest := storedManifest(t, db, item.ID)
	sources, _ := manifest["sources"].([]any)
	dropped, _ := manifest["dropped"].([]any)
	for _, expected := range []string{"摘要：第9章", "人物：鹤归", "地点：雾隐港", "关系：鹤归 —驻留→ 雾隐港", "伏笔：未拆信札"} {
		if !manifestNames(sources, expected) {
			t.Fatalf("manifest should name included %q, got %#v", expected, sources)
		}
	}
	for _, expected := range []string{"摘要：第1章", "超出近章窗口", "摘要：第2章", "超出上下文预算", "人物：人物000", "旧前缀关系", "伏笔：已揭晓铜钥", "已回收", "伏笔：城南旧案编号ZX", "与本章蓝图无关"} {
		if !manifestNames(dropped, expected) {
			t.Fatalf("manifest should name dropped %q, got %#v", expected, dropped)
		}
	}
	if manifestNames(sources, "摘要：第1章") || manifestNames(sources, "人物：人物000") || manifestNames(sources, "伏笔：已揭晓铜钥") {
		t.Fatalf("manifest inclusions should not keep dropped facts, got %#v", sources)
	}
}

func TestChapter51PromptStaysInsideBudget(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "chapter51.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	generator := &scriptedMemoryGenerator{
		prose:  strings.Repeat("文", 300),
		memory: chapter51Memory,
	}
	manager, err := task.NewManager(db, generator)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	userID, novelID, chapterIDs := seedMemoryNovel(t, db, "五十一章", 51)
	if _, err := db.Exec(`UPDATE chapters SET goal = ? WHERE id = ?`, "鹤归回到旧港", chapterIDs[51]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO graph_nodes (id, novel_id, kind, label, summary, created_at) VALUES (?, ?, 'character', '鹤归', '旧港来客', ?)`, database.NewID(), novelID, database.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE novels SET approval_mode = 'auto' WHERE id = ?`, novelID); err != nil {
		t.Fatal(err)
	}
	for start := 1; start <= 50; start += 20 {
		end := start + 19
		if end > 50 {
			end = 50
		}
		batch, failure := server.startGeneration(context.Background(), novelID, userID, generationInput{
			ChapterIDs: chapterIDs[start : end+1], Instruction: "定稿这一批章节",
		}, "batch", "", "")
		if failure != nil {
			t.Fatalf("finalize chapters %d-%d failed: %s", start, end, failure.message)
		}
		waitForStoredTask(t, db, batch.ID)
	}
	var storedSummary string
	if err := db.QueryRow(`SELECT m.summary FROM chapter_memories m JOIN chapters c ON c.id = m.chapter_id WHERE c.novel_id = ? AND c.chapter_number = 50`, novelID).Scan(&storedSummary); err != nil {
		t.Fatalf("chapter 50 memory should be written by finalize: %v", err)
	}
	if storedSummary != "第五十章记忆锚点" {
		t.Fatalf("finalize stored unexpected chapter 50 summary %q", storedSummary)
	}

	const budget = 1800
	concatenated := concatenatedChapterRunes(t, db, novelID)
	if budget >= concatenated {
		t.Fatalf("budget %d should be smaller than concatenated chapter text %d", budget, concatenated)
	}
	if _, err := db.Exec(`UPDATE novels SET approval_mode = 'manual' WHERE id = ?`, novelID); err != nil {
		t.Fatal(err)
	}
	server.SetContextBudget(budget)
	item, failure := server.startGeneration(context.Background(), novelID, userID, generationInput{
		ChapterIDs: []string{chapterIDs[51]}, Instruction: "写第五十一章",
	}, "batch", "", "")
	if failure != nil {
		t.Fatalf("chapter 51 start failed: %s", failure.message)
	}
	waitForStoredTask(t, db, item.ID)
	prompt := generator.promptContaining("目标章节：第51章")
	if prompt == "" {
		t.Fatal("chapter 51 generation prompt was not captured")
	}
	injected := injectedContext(t, prompt)
	if len([]rune(injected)) > budget {
		t.Fatalf("injected context is %d runes, budget is %d\n%s", len([]rune(injected)), budget, injected)
	}
	if !strings.Contains(injected, "第五十章记忆锚点") || !strings.Contains(injected, "旧港灯谜") {
		t.Fatalf("chapter 51 context should include chapter 50's summary and the older relevant foreshadowing, got:\n%s", injected)
	}
	if strings.Contains(prompt, "城南旧案编号ZX") || strings.Contains(prompt, "冗") || strings.Contains(prompt, "第1章摘要短句") {
		t.Fatalf("chapter 51 prompt should omit unrelated foreshadowing, oversized summaries, and chapters outside the window, got:\n%s", prompt)
	}
}

func promptTestMemory(number int) string {
	summary := fmt.Sprintf("第%d章普通摘要", number)
	foreshadow := ""
	switch number {
	case 1:
		summary = "第一章窗外摘要"
		foreshadow = "+未拆信札|鹤归|未拆信札仍未回收\n+已揭晓铜钥|鹤归|已揭晓铜钥不该再注入\n+城南旧案编号ZX|路人甲|城南旧案编号ZX与本章无关"
	case 3:
		foreshadow = "-已揭晓铜钥"
	case 9:
		summary = "最近摘要锚点"
	}
	if number >= 2 && number <= 8 {
		summary = strings.Repeat("冗", 4000)
	}
	return fmt.Sprintf("摘要：%s\n出场人物：鹤归\n新增设定：港雾\n伏笔变化：\n%s", summary, foreshadow)
}

func chapter51Memory(number int) string {
	summary := fmt.Sprintf("第%d章摘要短句", number)
	foreshadow := ""
	switch {
	case number == 1:
		summary = "第1章摘要短句"
		foreshadow = "+旧港灯谜|鹤归|鹤归在旧港留下的灯谜仍未揭晓\n+城南旧案编号ZX|路人甲|城南旧案编号ZX与本章无关"
	case number >= 43 && number <= 49:
		summary = strings.Repeat("冗", 3000)
	case number == 50:
		summary = "第五十章记忆锚点"
	}
	return fmt.Sprintf("摘要：%s\n出场人物：鹤归\n新增设定：无\n伏笔变化：\n%s", summary, foreshadow)
}

func seedMemoryNovel(t *testing.T, db *sql.DB, title string, chapters int) (string, string, []string) {
	t.Helper()
	userID := database.NewID()
	workspaceID := database.NewID()
	novelID := database.NewID()
	now := database.Now()
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, 'hash', ?)`, userID, userID+"@example.test", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, owner_id, created_at) VALUES (?, 'Test', ?, ?)`, workspaceID, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)`, workspaceID, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO novels (id, workspace_id, title, summary, genre, created_at, updated_at) VALUES (?, ?, ?, '一部用来检验记忆的长篇', '志怪', ?, ?)`, novelID, workspaceID, title, now, now); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, chapters+1)
	for number := 1; number <= chapters; number++ {
		ids[number] = database.NewID()
		chapterTitle := fmt.Sprintf("第%d章", number)
		if _, err := db.Exec(`INSERT INTO chapters (id, novel_id, chapter_number, title, body, revision, updated_at) VALUES (?, ?, ?, ?, '待写', 1, ?)`, ids[number], novelID, number, chapterTitle, now); err != nil {
			t.Fatal(err)
		}
	}
	return userID, novelID, ids
}

func seedOutOfPrefixGraph(t *testing.T, db *sql.DB, novelID string) error {
	t.Helper()
	now := database.Now()
	nodeIDs := make([]string, 80)
	for index := range nodeIDs {
		nodeIDs[index] = database.NewID()
		if _, err := db.Exec(`INSERT INTO graph_nodes (id, novel_id, kind, label, summary, created_at) VALUES (?, ?, 'character', ?, '路人', ?)`, nodeIDs[index], novelID, fmt.Sprintf("人物%03d", index), now); err != nil {
			return err
		}
	}
	heroID := database.NewID()
	placeID := database.NewID()
	if _, err := db.Exec(`INSERT INTO graph_nodes (id, novel_id, kind, label, summary, created_at) VALUES (?, ?, 'character', '鹤归', '章外人物', ?)`, heroID, novelID, now); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO graph_nodes (id, novel_id, kind, label, summary, created_at) VALUES (?, ?, 'place', '雾隐港', '章外地点', ?)`, placeID, novelID, now); err != nil {
		return err
	}
	for index := 0; index < 160; index++ {
		source := nodeIDs[index%len(nodeIDs)]
		target := nodeIDs[(index+1)%len(nodeIDs)]
		if _, err := db.Exec(`INSERT INTO graph_edges (id, novel_id, source_id, target_id, relation, summary, created_at) VALUES (?, ?, ?, ?, '旧前缀关系', '', ?)`, database.NewID(), novelID, source, target, fmt.Sprintf("t%04d", index)); err != nil {
			return err
		}
	}
	_, err := db.Exec(`INSERT INTO graph_edges (id, novel_id, source_id, target_id, relation, summary, created_at) VALUES (?, ?, ?, ?, '驻留', '住在港里', ?)`, database.NewID(), novelID, heroID, placeID, "t9999")
	return err
}

func chapterMemory(t *testing.T, db *sql.DB, chapterID string) (string, string, string, string, bool) {
	t.Helper()
	var summary, characters, settings, foreshadow string
	err := db.QueryRow(`SELECT summary, characters, new_settings, foreshadowing FROM chapter_memories WHERE chapter_id = ?`, chapterID).Scan(&summary, &characters, &settings, &foreshadow)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return summary, characters, settings, foreshadow, true
}

func waitForStoredTask(t *testing.T, db *sql.DB, taskID string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		var status, message string
		if err := db.QueryRow(`SELECT status, error_text FROM tasks WHERE id = ?`, taskID).Scan(&status, &message); err != nil {
			t.Fatal(err)
		}
		switch status {
		case "completed":
			return
		case "failed", "canceled", "interrupted":
			t.Fatalf("task %s ended as %s: %s", taskID, status, message)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s did not complete", taskID)
}

func storedManifest(t *testing.T, db *sql.DB, taskID string) map[string]any {
	t.Helper()
	var requestJSONText string
	if err := db.QueryRow(`SELECT request_json FROM tasks WHERE id = ?`, taskID).Scan(&requestJSONText); err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal([]byte(requestJSONText), &saved); err != nil {
		t.Fatal(err)
	}
	manifest, _ := saved["contextManifest"].(map[string]any)
	if manifest == nil {
		t.Fatalf("missing context manifest: %#v", saved["contextManifest"])
	}
	return manifest
}

func concatenatedChapterRunes(t *testing.T, db *sql.DB, novelID string) int {
	t.Helper()
	rows, err := db.Query(`SELECT body FROM chapters WHERE novel_id = ? AND deleted_at IS NULL`, novelID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		total += len([]rune(body))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return total
}

func injectedContext(t *testing.T, prompt string) string {
	t.Helper()
	const marker = "小说资料与知识图谱：\n"
	start := strings.Index(prompt, marker)
	if start < 0 {
		t.Fatalf("prompt missing context marker:\n%s", prompt)
	}
	rest := prompt[start+len(marker):]
	end := strings.Index(rest, "\n\n目标章节：")
	if end < 0 {
		t.Fatalf("prompt missing chapter marker:\n%s", prompt)
	}
	return rest[:end]
}

func chapterNumberFromPrompt(prompt string) int {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "第") {
			continue
		}
		rest := strings.TrimPrefix(line, "第")
		end := strings.Index(rest, "章")
		if end <= 0 {
			return 0
		}
		number, err := strconv.Atoi(rest[:end])
		if err != nil {
			return 0
		}
		return number
	}
	return 0
}
