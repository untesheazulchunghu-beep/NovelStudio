package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"novelstudio/internal/database"
	"novelstudio/internal/llm"
)

type Chapter struct {
	ID              string
	Number          int
	Title           string
	Body            string
	Revision        int
	Goal            string
	KeyEvents       string
	SuspenseHook    string
	PreviousTail    string
	InjectedContext string
}

type Request struct {
	ID                 string
	NovelID            string
	ProviderID         string `json:"providerId,omitempty"`
	WorkflowID         string
	WorkflowName       string
	UserID             string
	Instruction        string
	Context            string
	ContextManifest    ContextManifest `json:"contextManifest,omitempty"`
	WritingGuide       string
	ApprovalMode       string
	Chapters           []Chapter
	Nodes              []WorkflowNode
	NodeResults        []NodeResult
	Kind               string              `json:"Kind,omitempty"`
	ReviewOutcomes     []ReviewOutcome     `json:"ReviewOutcomes,omitempty"`
	ContinuityOutcomes []ContinuityOutcome `json:"continuityOutcomes,omitempty"`
}

type ContextManifest struct {
	Sources            []string `json:"sources,omitempty"`
	Dropped            []string `json:"dropped,omitempty"`
	TruncatedEntities  int      `json:"truncatedEntities,omitempty"`
	TruncatedRelations int      `json:"truncatedRelations,omitempty"`
}

type Event struct {
	TaskID    string         `json:"taskId"`
	Sequence  int64          `json:"sequence"`
	Type      string         `json:"type"`
	Data      map[string]any `json:"data"`
	CreatedAt string         `json:"createdAt"`
}

type Task struct {
	ID                  string  `json:"id"`
	NovelID             string  `json:"novelId"`
	WorkflowID          string  `json:"workflowId,omitempty"`
	Kind                string  `json:"kind"`
	Status              string  `json:"status"`
	Progress            int     `json:"progress"`
	Error               string  `json:"error,omitempty"`
	PromptTokens        int     `json:"promptTokens"`
	CompletionTokens    int     `json:"completionTokens"`
	TotalTokens         int     `json:"totalTokens"`
	EstimatedCostUSD    float64 `json:"estimatedCostUsd"`
	ReportedTokenCalls  int     `json:"reportedTokenCalls"`
	EstimatedTokenCalls int     `json:"estimatedTokenCalls"`
	CostKnownCalls      int     `json:"costKnownCalls"`
	CostUnknownCalls    int     `json:"costUnknownCalls"`
	CostKnown           bool    `json:"costKnown"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
}

type Manager struct {
	db        *sql.DB
	generator llm.Generator
	mu        sync.Mutex
	eventMu   sync.Mutex
	subs      map[string]map[chan Event]struct{}
	cancels   map[string]context.CancelFunc
	sequences map[string]int64
	semaphore chan struct{}
}

const (
	tokenEventFlushInterval = 150 * time.Millisecond
	tokenEventMaxBytes      = 4096
)

func NewManager(db *sql.DB, generator llm.Generator) (*Manager, error) {
	rows, err := db.Query(`SELECT id FROM tasks WHERE status IN ('queued', 'running')`)
	if err != nil {
		return nil, err
	}
	interrupted := make([]string, 0)
	for rows.Next() {
		var taskID string
		if err := rows.Scan(&taskID); err != nil {
			rows.Close()
			return nil, err
		}
		interrupted = append(interrupted, taskID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, taskID := range interrupted {
		if _, err := db.Exec(`UPDATE tasks SET status = 'interrupted', error_text = '服务器重启，任务未完成', updated_at = ? WHERE id = ?`, database.Now(), taskID); err != nil {
			return nil, err
		}
		var sequence int64
		if err := db.QueryRow(`SELECT COALESCE(MAX(sequence), 0) + 1 FROM task_events WHERE task_id = ?`, taskID).Scan(&sequence); err != nil {
			return nil, err
		}
		payload, err := json.Marshal(map[string]string{"status": "interrupted", "message": "服务器重启，任务未完成"})
		if err != nil {
			return nil, err
		}
		if _, err := db.Exec(`INSERT INTO task_events (task_id, sequence, event_type, payload_json, created_at) VALUES (?, ?, 'task.interrupted', ?, ?)`, taskID, sequence, string(payload), database.Now()); err != nil {
			return nil, err
		}
	}
	if _, err := db.Exec(`DELETE FROM task_events
		WHERE event_type = 'task.token'
		AND task_id IN (
			SELECT id FROM tasks
			WHERE status IN ('completed', 'failed', 'canceled', 'interrupted')
			AND datetime(updated_at) < datetime('now', '-30 days')
		)`); err != nil {
		return nil, fmt.Errorf("clean up expired task token events: %w", err)
	}
	manager := &Manager{
		db:        db,
		generator: generator,
		subs:      make(map[string]map[chan Event]struct{}),
		cancels:   make(map[string]context.CancelFunc),
		sequences: make(map[string]int64),
		semaphore: make(chan struct{}, 2),
	}
	rows, err = db.Query(`SELECT task_id, MAX(sequence) FROM task_events GROUP BY task_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var taskID string
		var sequence int64
		if err := rows.Scan(&taskID, &sequence); err != nil {
			rows.Close()
			return nil, err
		}
		manager.sequences[taskID] = sequence
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return manager, nil
}

func (m *Manager) Start(request Request) {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancels[request.ID] = cancel
	m.mu.Unlock()
	go func() {
		select {
		case m.semaphore <- struct{}{}:
			defer func() { <-m.semaphore }()
			m.run(ctx, request)
		case <-ctx.Done():
			m.finish(request.ID, "canceled", "任务已取消")
		}
		m.mu.Lock()
		delete(m.cancels, request.ID)
		m.mu.Unlock()
	}()
}

func (m *Manager) Cancel(taskID string) bool {
	m.mu.Lock()
	cancel, exists := m.cancels[taskID]
	m.mu.Unlock()
	if exists {
		cancel()
	}
	return exists
}

func (m *Manager) Subscribe(taskID string) (chan Event, func()) {
	channel := make(chan Event, 128)
	m.mu.Lock()
	if m.subs[taskID] == nil {
		m.subs[taskID] = make(map[chan Event]struct{})
	}
	m.subs[taskID][channel] = struct{}{}
	m.mu.Unlock()
	return channel, func() {
		m.mu.Lock()
		delete(m.subs[taskID], channel)
		if len(m.subs[taskID]) == 0 {
			delete(m.subs, taskID)
		}
		m.mu.Unlock()
	}
}

func (m *Manager) EventsAfter(taskID string, after int64) ([]Event, error) {
	rows, err := m.db.Query(`SELECT sequence, event_type, payload_json, created_at FROM task_events WHERE task_id = ? AND sequence > ? ORDER BY sequence`, taskID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var payload string
		event.TaskID = taskID
		if err := rows.Scan(&event.Sequence, &event.Type, &payload, &event.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(payload), &event.Data); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (m *Manager) Get(taskID string) (Task, error) {
	var item Task
	var workflowID sql.NullString
	if err := m.db.QueryRow(`SELECT id, novel_id, workflow_id, kind, status, progress, error_text,
		prompt_tokens, completion_tokens, total_tokens, estimated_cost_usd,
		reported_token_calls, estimated_token_calls, cost_known_calls, cost_unknown_calls,
		created_at, updated_at FROM tasks WHERE id = ?`, taskID).
		Scan(&item.ID, &item.NovelID, &workflowID, &item.Kind, &item.Status, &item.Progress, &item.Error,
			&item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.EstimatedCostUSD,
			&item.ReportedTokenCalls, &item.EstimatedTokenCalls, &item.CostKnownCalls, &item.CostUnknownCalls,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
		return Task{}, err
	}
	if workflowID.Valid {
		item.WorkflowID = workflowID.String
	}
	item.CostKnown = item.CostKnownCalls > 0 && item.CostUnknownCalls == 0
	return item, nil
}

func (m *Manager) run(ctx context.Context, request Request) {
	m.update(request.ID, "running", 0, "")
	m.emit(request.ID, "task.started", map[string]any{
		"chapterCount":       len(request.Chapters),
		"workflowName":       request.WorkflowName,
		"demoMode":           m.demoModeFor(request.ProviderID),
		"contextSources":     request.ContextManifest.Sources,
		"droppedContext":     request.ContextManifest.Dropped,
		"truncatedEntities":  request.ContextManifest.TruncatedEntities,
		"truncatedRelations": request.ContextManifest.TruncatedRelations,
	})
	for _, chapter := range request.Chapters {
		m.emit(request.ID, "chapter.queued", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title})
	}
	if len(request.Nodes) > 0 {
		m.runWorkflowGraph(ctx, request)
		return
	}
	if request.Kind == "review" {
		m.runReviewChecklist(ctx, request)
		return
	}
	if request.Kind == "continuity" {
		m.runContinuityCheck(ctx, request)
		return
	}
	var previousGenerated Chapter
	hasPreviousGenerated := false
	failedChapters := 0
	updateProgress := func(index int, chapter Chapter) {
		progress := (index + 1) * 100 / len(request.Chapters)
		m.update(request.ID, "running", progress, "")
		m.emit(request.ID, "task.progress", map[string]any{"progress": progress, "chapterId": chapter.ID})
	}
	recordFailure := func(chapter Chapter, message string) {
		failedChapters++
		m.emit(request.ID, "chapter.failed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title, "message": message})
	}
	for index, chapter := range request.Chapters {
		if err := ctx.Err(); err != nil {
			m.finish(request.ID, "canceled", "任务已取消")
			return
		}
		m.emit(request.ID, "chapter.started", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title})
		promptChapter := chapter
		if hasPreviousGenerated && previousGenerated.Number+1 == chapter.Number {
			promptChapter.PreviousTail = tailRunes(previousGenerated.Body, 600)
		}
		prompt := buildPrompt(request, promptChapter)
		tokenBatch := newTokenBatcher(m, request.ID, chapter.ID)
		text, usage, err, demoMode := m.generate(ctx, request.ProviderID, prompt, func(delta string) {
			tokenBatch.add(delta)
		})
		tokenBatch.flush()
		m.recordUsage(request.ID, usage, map[string]any{
			"chapterId": chapter.ID, "chapterNumber": chapter.Number, "kind": "generate",
		})
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				m.emit(request.ID, "chapter.canceled", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number})
				m.finish(request.ID, "canceled", "任务已取消")
				return
			}
			log.Printf("task %s chapter %d generation failed: %v", request.ID, chapter.Number, err)
			recordFailure(chapter, generationFailureMessage(err))
			updateProgress(index, chapter)
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			recordFailure(chapter, "模型没有返回可用内容")
			updateProgress(index, chapter)
			continue
		}
		proposalID, applyAutomatically, err := m.persistChapterOutput(ctx, request, chapter, text, demoMode)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				m.emit(request.ID, "chapter.canceled", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number})
				m.finish(request.ID, "canceled", "任务已取消")
				return
			}
			recordFailure(chapter, err.Error())
			updateProgress(index, chapter)
			continue
		}
		if applyAutomatically {
			m.emit(request.ID, "proposal.applied", map[string]any{"proposalId": proposalID, "chapterId": chapter.ID, "title": chapter.Title, "text": text})
		} else {
			m.emit(request.ID, "proposal.created", map[string]any{"proposalId": proposalID, "chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title, "text": text, "baseRevision": chapter.Revision, "demoMode": demoMode})
		}
		m.emit(request.ID, "chapter.completed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title, "proposalId": proposalID})
		previousGenerated = Chapter{Number: chapter.Number, Body: text}
		hasPreviousGenerated = true
		updateProgress(index, chapter)
	}
	if failedChapters > 0 {
		m.finish(request.ID, "failed", fmt.Sprintf("%d 个章节生成失败，可重试未完成章节", failedChapters))
		return
	}
	m.finish(request.ID, "completed", "")
}

// ComposeChapterMemory asks the injected generator for the four memory fields.
func (m *Manager) ComposeChapterMemory(ctx context.Context, chapter Chapter, body string) (ChapterMemory, error) {
	raw, _, err, demoMode := m.generate(ctx, "", MemoryPrompt(chapter, body), func(string) {})
	if err != nil {
		return ChapterMemory{}, err
	}
	if demoMode {
		return ChapterMemory{}, errors.New("章节记忆暂不可用：当前处于演示模式")
	}
	if !ValidChapterMemoryOutput(raw) {
		return ChapterMemory{}, errors.New("章节记忆格式无效")
	}
	return ParseChapterMemory(raw), nil
}

func (m *Manager) DemoMode() bool {
	return m.demoMode()
}

func (m *Manager) DemoModeFor(providerID string) bool {
	return m.demoModeFor(providerID)
}

func (m *Manager) ResolveProviderID(providerID string) (string, bool) {
	resolver, ok := m.generator.(interface {
		ResolveProviderID(string) (string, bool)
	})
	if !ok {
		return "", providerID == ""
	}
	return resolver.ResolveProviderID(providerID)
}

func (m *Manager) demoMode() bool {
	return m.demoModeFor("")
}

func (m *Manager) demoModeFor(providerID string) bool {
	if generator, ok := m.generator.(interface{ DemoModeFor(string) bool }); ok {
		return generator.DemoModeFor(providerID)
	}
	if providerID != "" {
		return true
	}
	generator, ok := m.generator.(interface{ DemoMode() bool })
	return ok && generator.DemoMode()
}

func (m *Manager) generate(ctx context.Context, providerID, prompt string, emit func(string)) (string, llm.GenerationUsage, error, bool) {
	usageGenerator, supportsUsage := m.generator.(interface {
		GenerateWithUsageFor(context.Context, string, string, func(string)) (string, llm.GenerationUsage, error, bool)
	})
	if supportsUsage {
		return usageGenerator.GenerateWithUsageFor(ctx, providerID, prompt, emit)
	}
	generatorWithProvider, hasProvider := m.generator.(interface {
		GenerateWithModeFor(context.Context, string, string, func(string)) (string, error, bool)
	})
	if hasProvider {
		text, err, demo := generatorWithProvider.GenerateWithModeFor(ctx, providerID, prompt, emit)
		return text, estimateLegacyUsage(prompt, text, demo), err, demo
	}
	generator, ok := m.generator.(interface {
		GenerateWithMode(context.Context, string, func(string)) (string, error, bool)
	})
	if ok {
		if providerID != "" {
			return "", llm.GenerationUsage{}, errors.New("selected model provider is unavailable"), true
		}
		text, err, demo := generator.GenerateWithMode(ctx, prompt, emit)
		return text, estimateLegacyUsage(prompt, text, demo), err, demo
	}
	if providerID != "" {
		return "", llm.GenerationUsage{}, errors.New("selected model provider is unavailable"), true
	}
	demoMode := m.demoMode()
	text, err := m.generator.Generate(ctx, prompt, emit)
	return text, estimateLegacyUsage(prompt, text, demoMode), err, demoMode
}

func estimateLegacyUsage(prompt, output string, demoMode bool) llm.GenerationUsage {
	if demoMode {
		return llm.GenerationUsage{}
	}
	return llm.EstimateUsage(prompt, output, "")
}

func tailRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[len(runes)-limit:])
}

func buildPrompt(request Request, chapter Chapter) string {
	var prompt strings.Builder
	prompt.WriteString("创作任务：\n")
	prompt.WriteString(request.Instruction)
	if guide := strings.TrimSpace(request.WritingGuide); guide != "" {
		prompt.WriteString("\n\n写作指南（必须遵守，冲突时优先于其他资料）：\n")
		prompt.WriteString(guide)
	}
	prompt.WriteString("\n\n小说资料与知识图谱：\n")
	contextText := chapter.InjectedContext
	if strings.TrimSpace(contextText) == "" {
		contextText = request.Context
	}
	prompt.WriteString(contextText)
	if request.WorkflowName != "" {
		prompt.WriteString("\n\n工作流：")
		prompt.WriteString(request.WorkflowName)
	}
	prompt.WriteString("\n\n目标章节：第")
	prompt.WriteString(strconv.Itoa(chapter.Number))
	prompt.WriteString("章《")
	prompt.WriteString(chapter.Title)
	prompt.WriteString("》")
	writePromptField(&prompt, "本章目标", chapter.Goal)
	writePromptField(&prompt, "本章关键事件", chapter.KeyEvents)
	writePromptField(&prompt, "章末钩子", chapter.SuspenseHook)
	if tail := strings.TrimSpace(chapter.PreviousTail); tail != "" {
		prompt.WriteString("\n上一章结尾（仅用于承接，不要重复或改写）：\n")
		prompt.WriteString(tail)
	}
	prompt.WriteString("\n当前草稿：\n")
	prompt.WriteString(chapter.Body)
	prompt.WriteString("\n\n请根据任务要求产出完整章节正文。只生成提案，不要改动未提供的设定。")
	return prompt.String()
}

func writePromptField(prompt *strings.Builder, label, value string) {
	if value = strings.TrimSpace(value); value == "" {
		return
	}
	prompt.WriteString("\n")
	prompt.WriteString(label)
	prompt.WriteString("：")
	prompt.WriteString(value)
}

func generationFailureMessage(err error) string {
	var userFacing interface{ UserMessage() string }
	if errors.As(err, &userFacing) && userFacing.UserMessage() != "" {
		return userFacing.UserMessage()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "模型请求超时，请稍后重试"
	}
	return "模型生成失败，请检查服务端模型配置后重试"
}

func (m *Manager) update(taskID, status string, progress int, message string) {
	_, err := m.db.Exec(`UPDATE tasks SET status = ?, progress = ?, error_text = ?, updated_at = ? WHERE id = ?`, status, progress, message, database.Now(), taskID)
	if err != nil {
		log.Printf("task %s status update failed: %v", taskID, err)
	}
}

func (m *Manager) finish(taskID, status, message string) {
	if status == "completed" {
		m.update(taskID, status, 100, message)
	} else {
		_, err := m.db.Exec(`UPDATE tasks SET status = ?, error_text = ?, updated_at = ? WHERE id = ?`, status, message, database.Now(), taskID)
		if err != nil {
			log.Printf("task %s status update failed: %v", taskID, err)
		}
	}
	m.emit(taskID, "task."+status, map[string]any{"status": status, "message": message})
}

func (m *Manager) emit(taskID, eventType string, data map[string]any) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("task %s event encoding failed: %v", taskID, err)
		return
	}
	// Keep sequence allocation, persistence, and live delivery together. Token
	// batching uses a timer and can finish concurrently with a chapter event;
	// separating these steps lets a subscriber receive sequence 42 before 41
	// and then discard the older event as a duplicate. Use a dedicated lock so
	// a slow database write does not block cancellation or subscription changes.
	m.eventMu.Lock()
	defer m.eventMu.Unlock()
	m.sequences[taskID]++
	sequence := m.sequences[taskID]
	createdAt := database.Now()
	if _, err := m.db.Exec(`INSERT INTO task_events (task_id, sequence, event_type, payload_json, created_at) VALUES (?, ?, ?, ?, ?)`, taskID, sequence, eventType, string(payload), createdAt); err != nil {
		log.Printf("task %s event persistence failed: %v", taskID, err)
		return
	}
	event := Event{TaskID: taskID, Sequence: sequence, Type: eventType, Data: data, CreatedAt: createdAt}
	m.deliver(event)
}

func (m *Manager) deliver(event Event) {
	m.mu.Lock()
	for channel := range m.subs[event.TaskID] {
		select {
		case channel <- event:
		default:
			close(channel)
			delete(m.subs[event.TaskID], channel)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) recordUsage(taskID string, usage llm.GenerationUsage, details map[string]any) {
	usage.PromptTokens = max(0, usage.PromptTokens)
	usage.CompletionTokens = max(0, usage.CompletionTokens)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	if usage.TotalTokens == 0 {
		return
	}
	if math.IsNaN(usage.CostUSD) || math.IsInf(usage.CostUSD, 0) || usage.CostUSD < 0 {
		usage.CostUSD = 0
		usage.CostKnown = false
	}
	data := map[string]any{
		"promptTokens": usage.PromptTokens, "completionTokens": usage.CompletionTokens,
		"totalTokens": usage.TotalTokens, "estimated": usage.Estimated,
		"model": usage.Model, "costUsd": usage.CostUSD, "costKnown": usage.CostKnown,
	}
	for key, value := range details {
		data[key] = value
	}
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("task %s usage event encoding failed: %v", taskID, err)
		return
	}

	m.eventMu.Lock()
	defer m.eventMu.Unlock()
	sequence := m.sequences[taskID] + 1
	createdAt := database.Now()
	tx, err := m.db.Begin()
	if err != nil {
		log.Printf("task %s usage transaction failed: %v", taskID, err)
		return
	}
	reportedCalls, estimatedCalls := 1, 0
	if usage.Estimated {
		reportedCalls, estimatedCalls = 0, 1
	}
	knownCalls, unknownCalls := 0, 1
	if usage.CostKnown {
		knownCalls, unknownCalls = 1, 0
	}
	_, err = tx.Exec(`UPDATE tasks SET prompt_tokens = prompt_tokens + ?, completion_tokens = completion_tokens + ?,
		total_tokens = total_tokens + ?, estimated_cost_usd = estimated_cost_usd + ?,
		reported_token_calls = reported_token_calls + ?, estimated_token_calls = estimated_token_calls + ?,
		cost_known_calls = cost_known_calls + ?, cost_unknown_calls = cost_unknown_calls + ?, updated_at = ?
		WHERE id = ?`, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, usage.CostUSD,
		reportedCalls, estimatedCalls, knownCalls, unknownCalls, createdAt, taskID)
	if err == nil {
		_, err = tx.Exec(`INSERT INTO task_events (task_id, sequence, event_type, payload_json, created_at) VALUES (?, ?, 'task.usage', ?, ?)`,
			taskID, sequence, string(payload), createdAt)
	}
	if err != nil {
		_ = tx.Rollback()
		log.Printf("task %s usage persistence failed: %v", taskID, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("task %s usage commit failed: %v", taskID, err)
		return
	}
	m.sequences[taskID] = sequence
	m.deliver(Event{
		TaskID: taskID, Sequence: sequence, Type: "task.usage", Data: data, CreatedAt: createdAt,
	})
}

type tokenBatcher struct {
	manager   *Manager
	taskID    string
	chapterID string
	mu        sync.Mutex
	text      strings.Builder
	timer     *time.Timer
}

func newTokenBatcher(manager *Manager, taskID, chapterID string) *tokenBatcher {
	return &tokenBatcher{manager: manager, taskID: taskID, chapterID: chapterID}
}

func (b *tokenBatcher) add(delta string) {
	if delta == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text.WriteString(delta)
	if b.text.Len() >= tokenEventMaxBytes {
		b.flushLocked()
		return
	}
	if b.timer == nil {
		b.timer = time.AfterFunc(tokenEventFlushInterval, b.flush)
	}
}

func (b *tokenBatcher) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushLocked()
}

func (b *tokenBatcher) flushLocked() {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	if b.text.Len() == 0 {
		return
	}
	text := b.text.String()
	b.text.Reset()
	b.manager.emit(b.taskID, "task.token", map[string]any{
		"chapterId": b.chapterID,
		"text":      text,
	})
}
