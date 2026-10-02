package novel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

type generationInput struct {
	ChapterIDs  []string            `json:"chapterIds"`
	Instruction string              `json:"instruction"`
	Kind        string              `json:"kind"`
	ProviderID  string              `json:"providerId,omitempty"`
	Nodes       []task.WorkflowNode `json:"-"`
}

type Proposal struct {
	ID            string `json:"id"`
	TaskID        string `json:"taskId"`
	NovelID       string `json:"novelId"`
	ChapterID     string `json:"chapterId"`
	ChapterNumber int    `json:"chapterNumber"`
	BaseRevision  int    `json:"baseRevision"`
	Title         string `json:"title"`
	Text          string `json:"text"`
	Status        string `json:"status"`
	DemoMode      bool   `json:"demoMode"`
	CreatedAt     string `json:"createdAt"`
}

func (s *Server) createBatchTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input generationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	providerID, exists := s.tasks.ResolveProviderID(input.ProviderID)
	if !exists {
		writeError(w, http.StatusBadRequest, "INVALID_MODEL_PROVIDER", "所选模型服务商不存在")
		return
	}
	input.ProviderID = providerID
	kind := "batch"
	switch strings.TrimSpace(input.Kind) {
	case "", "batch":
	case "review":
		if s.tasks.DemoModeFor(input.ProviderID) {
			writeError(w, http.StatusServiceUnavailable, "MODEL_NOT_CONFIGURED", "请先配置真实模型，再生成关键事件审稿清单")
			return
		}
		kind = "review"
	case "continuity":
		if s.tasks.DemoModeFor(input.ProviderID) {
			writeError(w, http.StatusServiceUnavailable, "MODEL_NOT_CONFIGURED", "请先配置真实模型，再启动连续性检查")
			return
		}
		kind = "continuity"
	default:
		writeError(w, http.StatusBadRequest, "INVALID_TASK", "未知的任务类型")
		return
	}
	item, err := s.startGeneration(r.Context(), r.PathValue("novelID"), userID, input, kind, "", "")
	if err != nil {
		writeError(w, err.status, err.code, err.message)
		return
	}
	writeJSON(w, http.StatusAccepted, item)
}

func (s *Server) refreshChapterMemory(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	chapterID := r.PathValue("chapterID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	if s.tasks.DemoMode() {
		writeError(w, http.StatusServiceUnavailable, "MODEL_NOT_CONFIGURED", "请先配置真实模型，再刷新章节记忆")
		return
	}
	var chapter task.Chapter
	err := s.db.QueryRowContext(r.Context(), `SELECT id, chapter_number, title, body, revision FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, chapterID, novelID).
		Scan(&chapter.ID, &chapter.Number, &chapter.Title, &chapter.Body, &chapter.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "CHAPTER_NOT_FOUND", "章节不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	if strings.TrimSpace(chapter.Body) == "" {
		writeError(w, http.StatusBadRequest, "EMPTY_CHAPTER", "章节正文为空，无需生成记忆")
		return
	}
	memory, err := s.tasks.ComposeChapterMemory(r.Context(), chapter, chapter.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "MEMORY_GENERATION_FAILED", "生成章节记忆失败，请稍后重试")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节记忆失败")
		return
	}
	defer tx.Rollback()
	var revision int
	if err := tx.QueryRowContext(r.Context(), `SELECT revision FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, chapterID, novelID).Scan(&revision); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "CHAPTER_NOT_FOUND", "章节不存在")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	if revision != chapter.Revision {
		writeError(w, http.StatusConflict, "STALE_REVISION", "章节在生成记忆期间已更新，请刷新后重试")
		return
	}
	if err := task.SaveChapterMemory(r.Context(), tx, novelID, chapterID, chapter.Number, chapter.Revision, memory); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节记忆失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节记忆失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"memoryStatus": "current"})
}

func (s *Server) runWorkflow(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input generationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	workflowID := r.PathValue("workflowID")
	name, definition, exists := s.loadWorkflow(r, workflowID, r.PathValue("novelID"))
	if !exists {
		writeError(w, http.StatusNotFound, "WORKFLOW_NOT_FOUND", "工作流不存在或定义无效")
		return
	}
	ordered := orderedWorkflowNodes(definition)
	if len(ordered) != len(definition.Nodes) {
		writeError(w, http.StatusNotFound, "WORKFLOW_NOT_FOUND", "工作流不存在或定义无效")
		return
	}
	input.Nodes = make([]task.WorkflowNode, len(ordered))
	for index, node := range ordered {
		input.Nodes[index] = task.WorkflowNode{ID: node.ID, Kind: node.Kind, Title: node.Title, Prompt: node.Prompt}
	}
	item, err := s.startGeneration(r.Context(), r.PathValue("novelID"), userID, input, "workflow", workflowID, name)
	if err != nil {
		writeError(w, err.status, err.code, err.message)
		return
	}
	writeJSON(w, http.StatusAccepted, item)
}

type apiFailure struct {
	status  int
	code    string
	message string
}

func (s *Server) startGeneration(ctx context.Context, novelID, userID string, input generationInput, kind, workflowID, workflowName string) (task.Task, *apiFailure) {
	providerID, exists := s.tasks.ResolveProviderID(input.ProviderID)
	if !exists {
		return task.Task{}, &apiFailure{status: http.StatusBadRequest, code: "INVALID_MODEL_PROVIDER", message: "所选模型服务商不存在"}
	}
	input.ProviderID = providerID
	if len(input.ChapterIDs) == 0 || len(input.ChapterIDs) > 20 || len(strings.TrimSpace(input.Instruction)) == 0 || len([]rune(input.Instruction)) > 5000 {
		return task.Task{}, &apiFailure{status: http.StatusBadRequest, code: "INVALID_TASK", message: "请选择 1–20 个章节，并填写不超过 5000 字的任务说明"}
	}
	var approvalMode string
	var title, summary, genre, writingGuide string
	if err := s.db.QueryRowContext(ctx, `SELECT title, summary, genre, approval_mode, writing_guide FROM novels WHERE id = ?`, novelID).Scan(&title, &summary, &genre, &approvalMode, &writingGuide); err != nil {
		return task.Task{}, &apiFailure{status: http.StatusNotFound, code: "NOVEL_NOT_FOUND", message: "小说不存在"}
	}
	if workflowID != "" && !s.workflowExists(workflowID, novelID) {
		return task.Task{}, &apiFailure{status: http.StatusNotFound, code: "WORKFLOW_NOT_FOUND", message: "工作流不存在"}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, chapter_number, title, body, revision, goal, key_events, suspense_hook FROM chapters WHERE novel_id = ? AND deleted_at IS NULL ORDER BY chapter_number`, novelID)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取章节失败"}
	}
	selected := make(map[string]bool, len(input.ChapterIDs))
	for _, id := range input.ChapterIDs {
		selected[id] = true
	}
	chapters := make([]task.Chapter, 0, len(input.ChapterIDs))
	previousBody := ""
	for rows.Next() {
		var chapter task.Chapter
		if err := rows.Scan(&chapter.ID, &chapter.Number, &chapter.Title, &chapter.Body, &chapter.Revision, &chapter.Goal, &chapter.KeyEvents, &chapter.SuspenseHook); err != nil {
			rows.Close()
			return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取章节失败"}
		}
		if selected[chapter.ID] {
			chapter.PreviousTail = tailRunes(previousBody, 600)
			chapters = append(chapters, chapter)
			delete(selected, chapter.ID)
		}
		previousBody = chapter.Body
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取章节失败"}
	}
	rows.Close()
	if len(selected) > 0 || len(chapters) != len(input.ChapterIDs) {
		return task.Task{}, &apiFailure{status: http.StatusBadRequest, code: "INVALID_CHAPTERS", message: "章节选择无效或包含其他小说的章节"}
	}
	if kind == "continuity" {
		for _, chapter := range chapters {
			if strings.TrimSpace(chapter.Body) == "" {
				return task.Task{}, &apiFailure{status: http.StatusBadRequest, code: "EMPTY_CHAPTER", message: "连续性检查需要章节正文，请先保存正文后再试"}
			}
		}
	}
	contextSources := []string{"小说题材与简介"}
	if strings.TrimSpace(writingGuide) != "" && kind != "continuity" {
		contextSources = append(contextSources, "作品写作指南")
	}
	for _, chapter := range chapters {
		contextSources = append(contextSources, "第"+strconv.Itoa(chapter.Number)+"章《"+chapter.Title+"》正文与蓝图")
	}
	graphTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱失败"}
	}
	defer graphTx.Rollback()
	graphRows, err := graphTx.QueryContext(ctx, `SELECT kind, label, summary FROM graph_nodes WHERE novel_id = ? ORDER BY kind, label`, novelID)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱失败"}
	}
	entities := make([]task.StoredEntity, 0)
	for graphRows.Next() {
		var entity task.StoredEntity
		if err := graphRows.Scan(&entity.Kind, &entity.Label, &entity.Summary); err != nil {
			graphRows.Close()
			return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱失败"}
		}
		entities = append(entities, entity)
	}
	if err := graphRows.Err(); err != nil {
		graphRows.Close()
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱失败"}
	}
	graphRows.Close()
	edgeRows, err := graphTx.QueryContext(ctx, `SELECT source_node.label, edge.relation, target_node.label, edge.summary FROM graph_edges edge JOIN graph_nodes source_node ON source_node.id = edge.source_id JOIN graph_nodes target_node ON target_node.id = edge.target_id WHERE edge.novel_id = ? ORDER BY edge.created_at, edge.id`, novelID)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识关系失败"}
	}
	relations := make([]task.StoredRelation, 0)
	for edgeRows.Next() {
		var relation task.StoredRelation
		if err := edgeRows.Scan(&relation.Source, &relation.Relation, &relation.Target, &relation.Summary); err != nil {
			edgeRows.Close()
			return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识关系失败"}
		}
		relations = append(relations, relation)
	}
	if err := edgeRows.Err(); err != nil {
		edgeRows.Close()
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识关系失败"}
	}
	edgeRows.Close()
	memories, err := task.LoadChapterMemories(ctx, graphTx, novelID)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取章节记忆失败"}
	}
	threads, err := task.LoadNarrativeThreads(ctx, graphTx, novelID)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取伏笔失败"}
	}
	characterStates, err := task.LoadCharacterStates(ctx, graphTx, novelID)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取人物状态失败"}
	}
	if err := graphTx.Commit(); err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱失败"}
	}
	if kind == "continuity" {
		for _, chapter := range chapters {
			var currentRevision int
			err := s.db.QueryRowContext(ctx, `SELECT revision FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, chapter.ID, novelID).Scan(&currentRevision)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && currentRevision != chapter.Revision) {
				return task.Task{}, &apiFailure{status: http.StatusConflict, code: "CHAPTER_CHANGED", message: "连续性检查准备期间章节已更新，请重新发起检查"}
			}
			if err != nil {
				return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "确认连续性检查章节失败"}
			}
		}
	}
	header := "小说：《" + title + "》\n题材：" + genre + "\n简介：" + summary
	budget := s.contextBudgetRunes()
	included := make([]string, 0)
	dropped := make([]string, 0)
	seenIncluded := make(map[string]bool)
	seenDropped := make(map[string]bool)
	truncatedEntities := 0
	truncatedRelations := 0
	knowledge := ""
	for index := range chapters {
		chapter := &chapters[index]
		blueprint := strings.Join([]string{chapter.Title, chapter.Goal, chapter.KeyEvents, chapter.SuspenseHook}, "\n")
		var assembled task.AssembledContext
		if kind == "continuity" {
			assembled = task.AssembleContinuityContext(budget, chapter.Number, blueprint+"\n"+chapter.Body, header, memories, entities, relations, threads, characterStates)
		} else {
			assembled = task.AssembleChapterContext(budget, chapter.Number, blueprint, header, memories, entities, relations, threads, characterStates)
		}
		chapter.InjectedContext = assembled.Text
		if index == 0 {
			knowledge = assembled.Text
		}
		for _, label := range assembled.Included {
			if !seenIncluded[label] {
				seenIncluded[label] = true
				included = append(included, label)
			}
		}
		for _, label := range assembled.Dropped {
			if !seenDropped[label] {
				seenDropped[label] = true
				dropped = append(dropped, label)
			}
		}
		truncatedEntities += assembled.TruncatedEntities
		truncatedRelations += assembled.TruncatedRelations
	}
	contextSources = append(contextSources, included...)
	taskID := database.NewID()
	now := database.Now()
	request := task.Request{
		ID: taskID, NovelID: novelID, ProviderID: input.ProviderID, WorkflowID: workflowID, WorkflowName: workflowName, UserID: userID,
		Instruction: strings.TrimSpace(input.Instruction), Context: knowledge,
		ContextManifest: task.ContextManifest{
			Sources: contextSources, Dropped: dropped, TruncatedEntities: truncatedEntities, TruncatedRelations: truncatedRelations,
		},
		WritingGuide: writingGuide, ApprovalMode: approvalMode, Chapters: chapters, Nodes: input.Nodes, Kind: kind,
	}
	if kind == "review" || kind == "continuity" {
		request.Nodes = nil
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "TASK_ENCODING_ERROR", message: "保存生成任务失败"}
	}
	chapterPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(chapters)), ",")
	insertQuery := `INSERT INTO tasks (id, novel_id, workflow_id, kind, status, progress, request_json, created_by, created_at, updated_at)
		SELECT ?, ?, NULLIF(?, ''), ?, 'queued', 0, ?, ?, ?, ?
		WHERE EXISTS (SELECT 1 FROM novels WHERE id = ? AND deleted_at IS NULL)
		AND (SELECT COUNT(*) FROM chapters WHERE novel_id = ? AND deleted_at IS NULL AND id IN (` + chapterPlaceholders + `)) = ?`
	insertArgs := []any{taskID, novelID, workflowID, kind, string(requestJSON), userID, now, now, novelID, novelID}
	for _, chapter := range chapters {
		insertArgs = append(insertArgs, chapter.ID)
	}
	insertArgs = append(insertArgs, len(chapters))
	result, err := s.db.ExecContext(ctx, insertQuery, insertArgs...)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "创建生成任务失败"}
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "创建生成任务失败"}
	}
	if inserted != 1 {
		var novelActive bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM novels WHERE id = ? AND deleted_at IS NULL)`, novelID).Scan(&novelActive); err != nil {
			return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "创建生成任务失败"}
		}
		if !novelActive {
			return task.Task{}, &apiFailure{status: http.StatusNotFound, code: "NOVEL_NOT_FOUND", message: "小说不存在"}
		}
		return task.Task{}, &apiFailure{status: http.StatusConflict, code: "CHAPTERS_CHANGED", message: "所选章节已被删除或变更，请刷新后重试"}
	}
	item := task.Task{ID: taskID, NovelID: novelID, WorkflowID: workflowID, Kind: kind, Status: "queued", CreatedAt: now, UpdatedAt: now}
	s.tasks.Start(request)
	return item, nil
}

func (s *Server) retryTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	originalID := r.PathValue("taskID")
	original, err := s.tasks.Get(originalID)
	if err != nil {
		writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	if !s.requireNovelRole(w, original.NovelID, userID, "owner", "editor") {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "准备重试任务失败")
		return
	}
	defer tx.Rollback()
	var status, requestJSON, retryTaskID, kind, workflowID string
	if err := tx.QueryRowContext(r.Context(), `SELECT status, request_json, retry_task_id, kind, COALESCE(workflow_id, '') FROM tasks WHERE id = ?`, originalID).Scan(&status, &requestJSON, &retryTaskID, &kind, &workflowID); err != nil {
		writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	if status != "failed" && status != "interrupted" && status != "canceled" {
		writeError(w, http.StatusConflict, "TASK_NOT_RETRYABLE", "只有失败、中断或取消的任务可以重试")
		return
	}
	if retryTaskID != "" {
		writeError(w, http.StatusConflict, "TASK_ALREADY_RETRIED", "该任务已经创建过重试任务")
		return
	}
	var request task.Request
	if requestJSON == "" || json.Unmarshal([]byte(requestJSON), &request) != nil {
		writeError(w, http.StatusConflict, "TASK_REQUEST_UNAVAILABLE", "该任务缺少可恢复的请求快照，请重新发起生成")
		return
	}
	providerID, providerExists := s.tasks.ResolveProviderID(request.ProviderID)
	if !providerExists {
		writeError(w, http.StatusConflict, "MODEL_PROVIDER_UNAVAILABLE", "原任务使用的模型服务商已被删除")
		return
	}
	request.ProviderID = providerID
	if (kind == "review" || kind == "continuity") && s.tasks.DemoModeFor(request.ProviderID) {
		message := "请先配置真实模型，再重试关键事件审稿"
		if kind == "continuity" {
			message = "请先配置真实模型，再重试连续性检查"
		}
		writeError(w, http.StatusServiceUnavailable, "MODEL_NOT_CONFIGURED", message)
		return
	}
	reviewed := make(map[string]bool, len(request.ReviewOutcomes)+len(request.ContinuityOutcomes))
	if kind == "review" {
		for _, outcome := range request.ReviewOutcomes {
			reviewed[outcome.ChapterID] = true
		}
	} else if kind == "continuity" {
		for _, outcome := range request.ContinuityOutcomes {
			reviewed[outcome.ChapterID] = true
		}
	}
	chapters := make([]task.Chapter, 0, len(request.Chapters))
	for index, chapter := range request.Chapters {
		if kind == "review" || kind == "continuity" {
			if reviewed[chapter.ID] {
				continue
			}
		} else {
			var hasProposal bool
			if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM proposals WHERE task_id = ? AND chapter_id = ?)`, originalID, chapter.ID).Scan(&hasProposal); err != nil {
				writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节任务状态失败")
				return
			}
			if hasProposal {
				continue
			}
		}
		if kind != "review" && kind != "continuity" && index > 0 && request.Chapters[index-1].Number+1 == chapter.Number {
			var previousText string
			err := tx.QueryRowContext(r.Context(), `SELECT proposed_text FROM proposals WHERE task_id = ? AND chapter_id = ?`, originalID, request.Chapters[index-1].ID).Scan(&previousText)
			if err == nil {
				chapter.PreviousTail = tailRunes(previousText, 600)
			} else if !errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取上一章提案失败")
				return
			}
		}
		chapters = append(chapters, chapter)
	}
	if len(chapters) == 0 {
		writeError(w, http.StatusConflict, "NO_CHAPTERS_TO_RETRY", "没有尚未成功的章节需要重试")
		return
	}

	retryID := database.NewID()
	request.ID = retryID
	request.UserID = userID
	request.Chapters = chapters
	encodedRequest, err := json.Marshal(request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TASK_ENCODING_ERROR", "保存重试任务失败")
		return
	}
	result, err := tx.ExecContext(r.Context(), `UPDATE tasks SET retry_task_id = ? WHERE id = ? AND retry_task_id = ''`, retryID, originalID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "更新原任务失败")
		return
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		writeError(w, http.StatusConflict, "TASK_ALREADY_RETRIED", "该任务已经创建过重试任务")
		return
	}
	now := database.Now()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO tasks (id, novel_id, workflow_id, kind, status, progress, request_json, created_by, created_at, updated_at) VALUES (?, ?, NULLIF(?, ''), ?, 'queued', 0, ?, ?, ?, ?)`, retryID, original.NovelID, workflowID, kind, string(encodedRequest), userID, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建重试任务失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建重试任务失败")
		return
	}
	item := task.Task{ID: retryID, NovelID: original.NovelID, WorkflowID: workflowID, Kind: kind, Status: "queued", CreatedAt: now, UpdatedAt: now}
	s.tasks.Start(request)
	writeJSON(w, http.StatusAccepted, item)
}

// tailRunes returns the trailing limit runes of value, used to give the model a
// short continuity anchor from the previous chapter.
func tailRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[len(runes)-limit:])
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, novel_id, workflow_id, kind, status, progress, error_text, created_at, updated_at FROM tasks WHERE novel_id = ? ORDER BY created_at DESC LIMIT 50`, r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取任务失败")
		return
	}
	defer rows.Close()
	tasks := make([]task.Task, 0)
	for rows.Next() {
		var item task.Task
		var workflowID sql.NullString
		if err := rows.Scan(&item.ID, &item.NovelID, &workflowID, &item.Kind, &item.Status, &item.Progress, &item.Error, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取任务失败")
			return
		}
		if workflowID.Valid {
			item.WorkflowID = workflowID.String
		}
		tasks = append(tasks, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取任务失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	item, err := s.tasks.Get(r.PathValue("taskID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	if !s.requireNovelRole(w, item.NovelID, userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) getTaskEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	item, err := s.tasks.Get(r.PathValue("taskID"))
	if err != nil || !s.requireNovelRole(w, item.NovelID, userID, "owner", "editor", "reviewer", "viewer") {
		if err != nil {
			writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		}
		return
	}
	after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if err != nil && r.URL.Query().Get("after") != "" {
		writeError(w, http.StatusBadRequest, "INVALID_CURSOR", "事件游标无效")
		return
	}
	events, err := s.tasks.EventsAfter(item.ID, after)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取任务事件失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) taskWebSocket(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	item, err := s.tasks.Get(r.PathValue("taskID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	if !s.requireNovelRole(w, item.NovelID, userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if err != nil && r.URL.Query().Get("after") != "" {
		writeError(w, http.StatusBadRequest, "INVALID_CURSOR", "事件游标无效")
		return
	}
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.origins})
	if err != nil {
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(1024)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := connection.Read(ctx); err != nil {
				return
			}
		}
	}()
	stream, unsubscribe := s.tasks.Subscribe(item.ID)
	defer unsubscribe()
	last := after
	events, err := s.tasks.EventsAfter(item.ID, after)
	if err != nil {
		return
	}
	for _, event := range events {
		if event.Sequence <= last {
			continue
		}
		if !sendEvent(ctx, connection, event) {
			return
		}
		last = event.Sequence
	}
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event, open := <-stream:
			if !open {
				return
			}
			if event.Sequence <= last {
				continue
			}
			if !sendEvent(ctx, connection, event) {
				return
			}
			last = event.Sequence
		case <-heartbeat.C:
			if err := connection.Ping(ctx); err != nil {
				return
			}
		case <-readDone:
			return
		case <-ctx.Done():
			return
		}
	}
}

func sendEvent(ctx context.Context, connection *websocket.Conn, event task.Event) bool {
	buffer, err := json.Marshal(event)
	if err != nil {
		return false
	}
	writeContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return connection.Write(writeContext, websocket.MessageText, buffer) == nil
}

func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	item, err := s.tasks.Get(r.PathValue("taskID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	if !s.requireNovelRole(w, item.NovelID, userID, "owner", "editor") {
		return
	}
	if !s.tasks.Cancel(item.ID) {
		writeError(w, http.StatusConflict, "TASK_NOT_RUNNING", "任务已结束或无法取消")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) regenerateStaleProposal(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	var chapterID, status, requestJSON, originalKind string
	var baseRevision, currentRevision int
	err := s.db.QueryRowContext(r.Context(), `SELECT p.chapter_id, p.status, p.base_revision, t.request_json, c.revision, t.kind
		FROM proposals p
		JOIN tasks t ON t.id = p.task_id
		JOIN chapters c ON c.id = p.chapter_id AND c.deleted_at IS NULL
		WHERE p.id = ? AND p.novel_id = ?`, r.PathValue("proposalID"), novelID).
		Scan(&chapterID, &status, &baseRevision, &requestJSON, &currentRevision, &originalKind)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "PROPOSAL_NOT_FOUND", "提案或对应章节不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取过期提案失败")
		return
	}
	if status != "pending" {
		writeError(w, http.StatusConflict, "PROPOSAL_RESOLVED", "该提案已处理")
		return
	}
	if currentRevision == baseRevision {
		writeError(w, http.StatusConflict, "PROPOSAL_NOT_STALE", "该提案仍基于当前章节版本")
		return
	}
	var original task.Request
	if requestJSON == "" || json.Unmarshal([]byte(requestJSON), &original) != nil || strings.TrimSpace(original.Instruction) == "" {
		writeError(w, http.StatusConflict, "TASK_REQUEST_UNAVAILABLE", "原任务缺少创作说明，请重新填写生成要求")
		return
	}
	item, failure := s.startGeneration(r.Context(), novelID, userID, generationInput{
		ChapterIDs: []string{chapterID}, Instruction: original.Instruction, ProviderID: original.ProviderID, Nodes: original.Nodes,
	}, originalKind, "", original.WorkflowName)
	if failure != nil {
		writeError(w, failure.status, failure.code, failure.message)
		return
	}
	writeJSON(w, http.StatusAccepted, item)
}

func (s *Server) listProposals(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.id, p.task_id, p.novel_id, p.chapter_id, c.chapter_number, p.base_revision, p.proposed_title, p.proposed_text, p.status, p.demo_mode, p.created_at FROM proposals p JOIN chapters c ON c.id = p.chapter_id AND c.deleted_at IS NULL WHERE p.novel_id = ? ORDER BY p.created_at DESC LIMIT 100`, r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取提案失败")
		return
	}
	defer rows.Close()
	proposals := make([]Proposal, 0)
	for rows.Next() {
		var item Proposal
		if err := rows.Scan(&item.ID, &item.TaskID, &item.NovelID, &item.ChapterID, &item.ChapterNumber, &item.BaseRevision, &item.Title, &item.Text, &item.Status, &item.DemoMode, &item.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取提案失败")
			return
		}
		proposals = append(proposals, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取提案失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposals": proposals})
}

func (s *Server) applyProposal(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor", "reviewer") {
		return
	}
	var input struct {
		SelectedHunks *[]int `json:"selectedHunks"`
		ApplyTitle    bool   `json:"applyTitle"`
	}
	if !decodeOptionalProposalJSON(w, r, &input) {
		return
	}
	proposalID := r.PathValue("proposalID")
	var chapterID, title, text, status string
	var baseRevision int
	var demoMode bool
	err := s.db.QueryRowContext(r.Context(), `SELECT chapter_id, base_revision, proposed_title, proposed_text, status, demo_mode FROM proposals WHERE id = ? AND novel_id = ?`, proposalID, novelID).Scan(&chapterID, &baseRevision, &title, &text, &status, &demoMode)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "PROPOSAL_NOT_FOUND", "提案不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取提案失败")
		return
	}
	if status != "pending" {
		writeError(w, http.StatusConflict, "PROPOSAL_RESOLVED", "该提案已处理")
		return
	}
	var current Chapter
	err = s.db.QueryRowContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, updated_at FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, chapterID, novelID).Scan(&current.ID, &current.NovelID, &current.Number, &current.Title, &current.Body, &current.Revision, &current.Status, &current.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "CHAPTER_NOT_FOUND", "章节不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	if current.Revision != baseRevision {
		writeError(w, http.StatusConflict, "STALE_REVISION", "章节已被更新，请刷新后重新生成提案")
		return
	}
	nextTitle, nextText := title, text
	selectedHunkCount := 0
	if input.SelectedHunks != nil {
		mergedText, _, valid := mergeProposalHunks(current.Body, text, *input.SelectedHunks)
		if !valid {
			writeError(w, http.StatusBadRequest, "INVALID_PROPOSAL_SELECTION", "所选段落范围无效或包含重复项")
			return
		}
		nextText = mergedText
		selectedHunkCount = len(*input.SelectedHunks)
		if selectedHunkCount == 0 && !input.ApplyTitle {
			writeError(w, http.StatusBadRequest, "EMPTY_PROPOSAL_SELECTION", "请至少选择一组段落修改或提案标题")
			return
		}
		if !input.ApplyTitle {
			nextTitle = current.Title
		}
		if nextTitle == current.Title && nextText == current.Body {
			writeError(w, http.StatusBadRequest, "EMPTY_PROPOSAL_SELECTION", "所选内容不会改变当前章节")
			return
		}
	}
	storeMemory := !demoMode && !s.tasks.DemoMode()
	var memory task.ChapterMemory
	if storeMemory {
		memory, err = s.tasks.ComposeChapterMemory(r.Context(), task.Chapter{
			ID: current.ID, Number: current.Number, Title: nextTitle, Body: nextText,
		}, nextText)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				writeError(w, http.StatusRequestTimeout, "MEMORY_GENERATION_CANCELED", "生成章节记忆已取消，章节尚未应用")
				return
			}
			// Applying a reviewed proposal must remain possible when the model
			// cannot produce the optional memory record. The chapter is still
			// finalized; the memory status stays missing/stale and can be
			// refreshed explicitly later.
			storeMemory = false
		}
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "应用提案失败")
		return
	}
	defer tx.Rollback()
	var currentStatus string
	var currentBase int
	err = tx.QueryRowContext(r.Context(), `SELECT status, base_revision FROM proposals WHERE id = ? AND novel_id = ?`, proposalID, novelID).Scan(&currentStatus, &currentBase)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "PROPOSAL_NOT_FOUND", "提案不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取提案失败")
		return
	}
	if currentStatus != "pending" {
		writeError(w, http.StatusConflict, "PROPOSAL_RESOLVED", "该提案已处理")
		return
	}
	err = tx.QueryRowContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, updated_at FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, chapterID, novelID).Scan(&current.ID, &current.NovelID, &current.Number, &current.Title, &current.Body, &current.Revision, &current.Status, &current.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "CHAPTER_NOT_FOUND", "章节不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	if current.Revision != baseRevision || currentBase != baseRevision {
		writeError(w, http.StatusConflict, "STALE_REVISION", "章节已被更新，请刷新后重新生成提案")
		return
	}
	if err := storeChapterRevision(tx, current, userID, "proposal"); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "记录章节版本失败")
		return
	}
	result, err := tx.ExecContext(r.Context(), `UPDATE chapters SET title = ?, body = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND novel_id = ? AND revision = ?`, nextTitle, nextText, database.Now(), chapterID, novelID, baseRevision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "应用提案失败")
		return
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		writeError(w, http.StatusConflict, "STALE_REVISION", "章节已被更新，请刷新后重新生成提案")
		return
	}
	proposalResult, err := tx.ExecContext(r.Context(), `UPDATE proposals SET status = 'applied', resolved_at = ? WHERE id = ? AND status = 'pending'`, database.Now(), proposalID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "应用提案失败")
		return
	}
	proposalUpdated, err := proposalResult.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "应用提案失败")
		return
	}
	if proposalUpdated != 1 {
		writeError(w, http.StatusConflict, "PROPOSAL_RESOLVED", "该提案已处理")
		return
	}
	if storeMemory {
		if err := task.SaveChapterMemory(r.Context(), tx, novelID, chapterID, current.Number, current.Revision+1, memory); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节记忆失败")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "应用提案失败")
		return
	}
	if input.SelectedHunks != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "appliedHunks": selectedHunkCount, "appliedTitle": input.ApplyTitle})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func decodeOptionalProposalJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求内容格式无效")
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}

func (s *Server) rejectProposal(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer") {
		return
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE proposals SET status = 'rejected', resolved_at = ? WHERE id = ? AND novel_id = ? AND status = 'pending'`, database.Now(), r.PathValue("proposalID"), r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "拒绝提案失败")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeError(w, http.StatusNotFound, "PROPOSAL_NOT_FOUND", "待处理提案不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
