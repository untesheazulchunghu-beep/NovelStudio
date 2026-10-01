package novel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

type generationInput struct {
	ChapterIDs  []string `json:"chapterIds"`
	Instruction string   `json:"instruction"`
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
	item, err := s.startGeneration(r.Context(), r.PathValue("novelID"), userID, input, "batch", "", "")
	if err != nil {
		writeError(w, err.status, err.code, err.message)
		return
	}
	writeJSON(w, http.StatusAccepted, item)
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
	name, steps, exists := s.workflowPrompt(r, workflowID, r.PathValue("novelID"))
	if !exists {
		writeError(w, http.StatusNotFound, "WORKFLOW_NOT_FOUND", "工作流不存在或定义无效")
		return
	}
	input.Instruction = strings.TrimSpace(input.Instruction + "\n\n" + steps)
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
	contextSources := []string{"小说题材与简介"}
	if strings.TrimSpace(writingGuide) != "" {
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
	var totalEntities, totalRelations int
	if err := graphTx.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_nodes WHERE novel_id = ?`, novelID).Scan(&totalEntities); err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱数量失败"}
	}
	if err := graphTx.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_edges WHERE novel_id = ?`, novelID).Scan(&totalRelations); err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识关系数量失败"}
	}
	graphRows, err := graphTx.QueryContext(ctx, `SELECT kind, label, summary FROM graph_nodes WHERE novel_id = ? ORDER BY kind, label LIMIT 80`, novelID)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱失败"}
	}
	var knowledge strings.Builder
	knowledge.WriteString("小说：《" + title + "》\n题材：" + genre + "\n简介：" + summary + "\n")
	includedEntities := 0
	for graphRows.Next() {
		var kind, label, nodeSummary string
		if err := graphRows.Scan(&kind, &label, &nodeSummary); err != nil {
			graphRows.Close()
			return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱失败"}
		}
		knowledge.WriteString(kind + "：" + label + "。" + nodeSummary + "\n")
		contextSources = append(contextSources, graphNodeKindLabel(kind)+"："+label)
		includedEntities++
	}
	if err := graphRows.Err(); err != nil {
		graphRows.Close()
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱失败"}
	}
	graphRows.Close()
	edgeRows, err := graphTx.QueryContext(ctx, `SELECT source_node.label, edge.relation, target_node.label, edge.summary FROM graph_edges edge JOIN graph_nodes source_node ON source_node.id = edge.source_id JOIN graph_nodes target_node ON target_node.id = edge.target_id WHERE edge.novel_id = ? ORDER BY edge.created_at LIMIT 160`, novelID)
	if err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识关系失败"}
	}
	includedRelations := 0
	for edgeRows.Next() {
		var source, relation, target, edgeSummary string
		if err := edgeRows.Scan(&source, &relation, &target, &edgeSummary); err != nil {
			edgeRows.Close()
			return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识关系失败"}
		}
		knowledge.WriteString(source + " —" + relation + "→ " + target + "。" + edgeSummary + "\n")
		contextSources = append(contextSources, "关系："+source+" —"+relation+"→ "+target)
		includedRelations++
	}
	if err := edgeRows.Err(); err != nil {
		edgeRows.Close()
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识关系失败"}
	}
	edgeRows.Close()
	if err := graphTx.Commit(); err != nil {
		return task.Task{}, &apiFailure{status: http.StatusInternalServerError, code: "DATABASE_ERROR", message: "读取知识图谱失败"}
	}
	truncatedEntities := totalEntities - includedEntities
	if truncatedEntities < 0 {
		truncatedEntities = 0
	}
	truncatedRelations := totalRelations - includedRelations
	if truncatedRelations < 0 {
		truncatedRelations = 0
	}
	taskID := database.NewID()
	now := database.Now()
	request := task.Request{
		ID: taskID, NovelID: novelID, WorkflowID: workflowID, WorkflowName: workflowName, UserID: userID,
		Instruction: strings.TrimSpace(input.Instruction), Context: knowledge.String(),
		ContextManifest: task.ContextManifest{
			Sources: contextSources, TruncatedEntities: truncatedEntities, TruncatedRelations: truncatedRelations,
		},
		WritingGuide: writingGuide, ApprovalMode: approvalMode, Chapters: chapters,
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

func graphNodeKindLabel(kind string) string {
	switch kind {
	case "character":
		return "人物"
	case "place":
		return "地点"
	case "faction":
		return "组织"
	case "item":
		return "物品"
	case "concept":
		return "设定"
	default:
		return "实体"
	}
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
	chapters := make([]task.Chapter, 0, len(request.Chapters))
	for index, chapter := range request.Chapters {
		var hasProposal bool
		if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM proposals WHERE task_id = ? AND chapter_id = ?)`, originalID, chapter.ID).Scan(&hasProposal); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节任务状态失败")
			return
		}
		if hasProposal {
			continue
		}
		if index > 0 && request.Chapters[index-1].Number+1 == chapter.Number {
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
		ChapterIDs: []string{chapterID}, Instruction: original.Instruction,
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
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.id, p.task_id, p.novel_id, p.chapter_id, c.chapter_number, p.base_revision, p.proposed_title, p.proposed_text, p.status, p.created_at FROM proposals p JOIN chapters c ON c.id = p.chapter_id AND c.deleted_at IS NULL WHERE p.novel_id = ? ORDER BY p.created_at DESC LIMIT 100`, r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取提案失败")
		return
	}
	defer rows.Close()
	proposals := make([]Proposal, 0)
	for rows.Next() {
		var item Proposal
		if err := rows.Scan(&item.ID, &item.TaskID, &item.NovelID, &item.ChapterID, &item.ChapterNumber, &item.BaseRevision, &item.Title, &item.Text, &item.Status, &item.CreatedAt); err != nil {
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
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer") {
		return
	}
	proposalID := r.PathValue("proposalID")
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "应用提案失败")
		return
	}
	var chapterID, title, text, status string
	var baseRevision int
	err = tx.QueryRowContext(r.Context(), `SELECT chapter_id, base_revision, proposed_title, proposed_text, status FROM proposals WHERE id = ? AND novel_id = ?`, proposalID, r.PathValue("novelID")).Scan(&chapterID, &baseRevision, &title, &text, &status)
	if errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		writeError(w, http.StatusNotFound, "PROPOSAL_NOT_FOUND", "提案不存在")
		return
	}
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取提案失败")
		return
	}
	if status != "pending" {
		tx.Rollback()
		writeError(w, http.StatusConflict, "PROPOSAL_RESOLVED", "该提案已处理")
		return
	}
	var current Chapter
	err = tx.QueryRowContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, updated_at FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, chapterID, r.PathValue("novelID")).Scan(&current.ID, &current.NovelID, &current.Number, &current.Title, &current.Body, &current.Revision, &current.Status, &current.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		writeError(w, http.StatusNotFound, "CHAPTER_NOT_FOUND", "章节不存在")
		return
	}
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	if current.Revision != baseRevision {
		tx.Rollback()
		writeError(w, http.StatusConflict, "STALE_REVISION", "章节已被更新，请刷新后重新生成提案")
		return
	}
	if err := storeChapterRevision(tx, current, userID, "proposal"); err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "记录章节版本失败")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE chapters SET title = ?, body = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND novel_id = ? AND revision = ?`, title, text, database.Now(), chapterID, r.PathValue("novelID"), baseRevision)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "应用提案失败")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE proposals SET status = 'applied', resolved_at = ? WHERE id = ?`, database.Now(), proposalID)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "应用提案失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "应用提案失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
