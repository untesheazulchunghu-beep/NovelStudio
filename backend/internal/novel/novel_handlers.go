package novel

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

type Novel struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspaceId"`
	Title           string `json:"title"`
	Summary         string `json:"summary"`
	Genre           string `json:"genre"`
	PlannedChapters int    `json:"plannedChapters"`
	ApprovalMode    string `json:"approvalMode"`
	WritingGuide    string `json:"writingGuide"`
	ChapterCount    int    `json:"chapterCount,omitempty"`
	DeletedAt       string `json:"deletedAt,omitempty"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type Chapter struct {
	ID           string `json:"id"`
	NovelID      string `json:"novelId"`
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	Revision     int    `json:"revision"`
	Status       string `json:"status"`
	Goal         string `json:"goal"`
	KeyEvents    string `json:"keyEvents"`
	SuspenseHook string `json:"suspenseHook"`
	DeletedAt    string `json:"deletedAt,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
	MemoryStatus string `json:"memoryStatus,omitempty"`
}

func (s *Server) listNovels(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT n.id, n.workspace_id, n.title, n.summary, n.genre, n.planned_chapters, n.approval_mode, n.writing_guide, COUNT(c.id), n.created_at, n.updated_at FROM novels n JOIN workspace_members wm ON wm.workspace_id = n.workspace_id LEFT JOIN chapters c ON c.novel_id = n.id AND c.deleted_at IS NULL WHERE wm.user_id = ? AND n.deleted_at IS NULL GROUP BY n.id ORDER BY n.updated_at DESC`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取小说项目失败")
		return
	}
	defer rows.Close()
	novels := make([]Novel, 0)
	for rows.Next() {
		var item Novel
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Title, &item.Summary, &item.Genre, &item.PlannedChapters, &item.ApprovalMode, &item.WritingGuide, &item.ChapterCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取小说项目失败")
			return
		}
		novels = append(novels, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取小说项目失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"novels": novels})
}

func (s *Server) listDeletedNovels(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT n.id, n.workspace_id, n.title, n.summary, n.genre, n.planned_chapters, n.approval_mode, n.writing_guide, COUNT(c.id), COALESCE(n.deleted_at, ''), n.created_at, n.updated_at
		FROM novels n
		JOIN workspace_members wm ON wm.workspace_id = n.workspace_id
		LEFT JOIN chapters c ON c.novel_id = n.id AND c.deleted_at IS NULL
		WHERE wm.user_id = ? AND n.deleted_at IS NOT NULL
		GROUP BY n.id ORDER BY n.deleted_at DESC`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取已删除作品失败")
		return
	}
	defer rows.Close()
	novels := make([]Novel, 0)
	for rows.Next() {
		var item Novel
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Title, &item.Summary, &item.Genre, &item.PlannedChapters, &item.ApprovalMode, &item.WritingGuide, &item.ChapterCount, &item.DeletedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取已删除作品失败")
			return
		}
		novels = append(novels, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取已删除作品失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"novels": novels})
}

func (s *Server) createNovel(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	var input struct {
		WorkspaceID     string `json:"workspaceId"`
		Title           string `json:"title"`
		Summary         string `json:"summary"`
		Genre           string `json:"genre"`
		PlannedChapters int    `json:"plannedChapters"`
		ApprovalMode    string `json:"approvalMode"`
		WritingGuide    string `json:"writingGuide"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.ApprovalMode = normalizeApprovalMode(input.ApprovalMode)
	input.WritingGuide = strings.TrimSpace(input.WritingGuide)
	if input.Title == "" || len([]rune(input.Title)) > 120 || input.PlannedChapters < 0 || input.PlannedChapters > 10000 || len([]rune(input.WritingGuide)) > 4000 {
		writeError(w, http.StatusBadRequest, "INVALID_NOVEL", "标题不能为空且不超过 120 字，计划章节数需在 0–10000 之间，写作指南不超过 4000 字")
		return
	}
	if input.WorkspaceID == "" {
		if err := s.db.QueryRowContext(r.Context(), `SELECT workspace_id FROM workspace_members WHERE user_id = ? AND role IN ('owner', 'editor') ORDER BY joined_at LIMIT 1`, userID).Scan(&input.WorkspaceID); err != nil {
			writeError(w, http.StatusBadRequest, "WORKSPACE_REQUIRED", "没有可创建小说的工作区")
			return
		}
	} else if !s.requireWorkspaceWriter(w, r, input.WorkspaceID, userID) {
		return
	}
	id := database.NewID()
	chapterID := database.NewID()
	now := database.Now()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建小说项目失败")
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO novels (id, workspace_id, title, summary, genre, planned_chapters, approval_mode, writing_guide, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, input.WorkspaceID, input.Title, strings.TrimSpace(input.Summary), strings.TrimSpace(input.Genre), input.PlannedChapters, input.ApprovalMode, input.WritingGuide, now, now)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO chapters (id, novel_id, chapter_number, title, body, revision, status, updated_at) VALUES (?, ?, 1, '第一章', '', 1, 'draft', ?)`, chapterID, id, now)
	}
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建小说项目失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建小说项目失败")
		return
	}
	writeJSON(w, http.StatusCreated, Novel{ID: id, WorkspaceID: input.WorkspaceID, Title: input.Title, Summary: input.Summary, Genre: input.Genre, PlannedChapters: input.PlannedChapters, ApprovalMode: input.ApprovalMode, WritingGuide: input.WritingGuide, ChapterCount: 1, CreatedAt: now, UpdatedAt: now})
}

func (s *Server) getNovel(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	var item Novel
	err := s.db.QueryRowContext(r.Context(), `SELECT n.id, n.workspace_id, n.title, n.summary, n.genre, n.planned_chapters, n.approval_mode, n.writing_guide, COUNT(c.id), n.created_at, n.updated_at FROM novels n LEFT JOIN chapters c ON c.novel_id = n.id AND c.deleted_at IS NULL WHERE n.id = ? AND n.deleted_at IS NULL GROUP BY n.id`, r.PathValue("novelID")).
		Scan(&item.ID, &item.WorkspaceID, &item.Title, &item.Summary, &item.Genre, &item.PlannedChapters, &item.ApprovalMode, &item.WritingGuide, &item.ChapterCount, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOVEL_NOT_FOUND", "小说不存在")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateNovel(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input struct {
		Title           string `json:"title"`
		Summary         string `json:"summary"`
		Genre           string `json:"genre"`
		PlannedChapters int    `json:"plannedChapters"`
		ApprovalMode    string `json:"approvalMode"`
		WritingGuide    string `json:"writingGuide"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.ApprovalMode = normalizeApprovalMode(input.ApprovalMode)
	input.WritingGuide = strings.TrimSpace(input.WritingGuide)
	if input.Title == "" || len([]rune(input.Title)) > 120 || input.PlannedChapters < 0 || input.PlannedChapters > 10000 || len([]rune(input.WritingGuide)) > 4000 {
		writeError(w, http.StatusBadRequest, "INVALID_NOVEL", "标题不能为空且不超过 120 字，计划章节数需在 0–10000 之间，写作指南不超过 4000 字")
		return
	}
	_, err := s.db.ExecContext(r.Context(), `UPDATE novels SET title = ?, summary = ?, genre = ?, planned_chapters = ?, approval_mode = ?, writing_guide = ?, updated_at = ? WHERE id = ?`, input.Title, strings.TrimSpace(input.Summary), strings.TrimSpace(input.Genre), input.PlannedChapters, input.ApprovalMode, input.WritingGuide, database.Now(), r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存小说设置失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) deleteNovel(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "移入回收站失败")
		return
	}
	defer tx.Rollback()
	if s.hasActiveNovelTasks(w, r, tx, novelID) {
		return
	}
	now := database.Now()
	result, err := tx.ExecContext(r.Context(), `UPDATE novels SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, now, now, novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "移入回收站失败")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, http.StatusNotFound, "NOVEL_NOT_FOUND", "小说不存在")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "移入回收站失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) restoreNovel(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	novelID := r.PathValue("novelID")
	var role string
	err := s.db.QueryRowContext(r.Context(), `SELECT wm.role FROM novels n JOIN workspace_members wm ON wm.workspace_id = n.workspace_id WHERE n.id = ? AND n.deleted_at IS NOT NULL AND wm.user_id = ?`, novelID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOVEL_NOT_FOUND", "已删除的小说不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取小说权限失败")
		return
	}
	if role != "owner" && role != "editor" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "当前角色不能恢复小说")
		return
	}
	_, err = s.db.ExecContext(r.Context(), `UPDATE novels SET deleted_at = NULL, updated_at = ? WHERE id = ? AND deleted_at IS NOT NULL`, database.Now(), novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "恢复小说失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) listChapters(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT c.id, c.novel_id, c.chapter_number, c.title, c.body, c.revision, c.status, c.goal, c.key_events, c.suspense_hook, COALESCE(c.deleted_at, ''), c.updated_at,
		CASE WHEN m.chapter_id IS NULL THEN 'missing' WHEN m.source_revision = c.revision THEN 'current' ELSE 'stale' END
		FROM chapters c LEFT JOIN chapter_memories m ON m.chapter_id = c.id
		WHERE c.novel_id = ? AND c.deleted_at IS NULL ORDER BY c.chapter_number`, r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	defer rows.Close()
	chapters := make([]Chapter, 0)
	for rows.Next() {
		var item Chapter
		if err := rows.Scan(&item.ID, &item.NovelID, &item.Number, &item.Title, &item.Body, &item.Revision, &item.Status, &item.Goal, &item.KeyEvents, &item.SuspenseHook, &item.DeletedAt, &item.UpdatedAt, &item.MemoryStatus); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
			return
		}
		chapters = append(chapters, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chapters": chapters})
}

func (s *Server) listDeletedChapters(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT c.id, c.novel_id, c.chapter_number, c.title, c.body, c.revision, c.status, c.goal, c.key_events, c.suspense_hook, COALESCE(c.deleted_at, ''), c.updated_at,
		CASE WHEN m.chapter_id IS NULL THEN 'missing' WHEN m.source_revision = c.revision THEN 'current' ELSE 'stale' END
		FROM chapters c LEFT JOIN chapter_memories m ON m.chapter_id = c.id
		WHERE c.novel_id = ? AND c.deleted_at IS NOT NULL ORDER BY c.chapter_number`, r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取已删除章节失败")
		return
	}
	defer rows.Close()
	chapters := make([]Chapter, 0)
	for rows.Next() {
		var item Chapter
		if err := rows.Scan(&item.ID, &item.NovelID, &item.Number, &item.Title, &item.Body, &item.Revision, &item.Status, &item.Goal, &item.KeyEvents, &item.SuspenseHook, &item.DeletedAt, &item.UpdatedAt, &item.MemoryStatus); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取已删除章节失败")
			return
		}
		chapters = append(chapters, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取已删除章节失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chapters": chapters})
}

func (s *Server) reorderChapters(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	var input struct {
		ChapterIDs []string `json:"chapterIds"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "调整章节顺序失败")
		return
	}
	defer tx.Rollback()
	if s.hasActiveNovelTasks(w, r, tx, novelID) {
		return
	}

	rows, err := tx.QueryContext(r.Context(), `SELECT id, chapter_number FROM chapters WHERE novel_id = ? AND deleted_at IS NULL ORDER BY chapter_number`, novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节顺序失败")
		return
	}
	activeIDs := make([]string, 0)
	slots := make([]int64, 0)
	for rows.Next() {
		var id string
		var number int64
		if err := rows.Scan(&id, &number); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节顺序失败")
			return
		}
		activeIDs = append(activeIDs, id)
		slots = append(slots, number)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节顺序失败")
		return
	}
	rows.Close()

	if len(input.ChapterIDs) != len(activeIDs) {
		writeError(w, http.StatusBadRequest, "INVALID_CHAPTER_ORDER", "章节顺序必须包含当前全部有效章节")
		return
	}
	activeSet := make(map[string]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		activeSet[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(input.ChapterIDs))
	for _, id := range input.ChapterIDs {
		if _, exists := activeSet[id]; !exists {
			writeError(w, http.StatusBadRequest, "INVALID_CHAPTER_ORDER", "章节顺序包含无效或已删除的章节")
			return
		}
		if _, duplicate := seen[id]; duplicate {
			writeError(w, http.StatusBadRequest, "INVALID_CHAPTER_ORDER", "章节顺序不能包含重复项")
			return
		}
		seen[id] = struct{}{}
	}
	var minimumNumber sql.NullInt64
	if err := tx.QueryRowContext(r.Context(), `SELECT MIN(chapter_number) FROM chapters WHERE novel_id = ?`, novelID).Scan(&minimumNumber); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "检查章节顺序失败")
		return
	}
	if minimumNumber.Valid && minimumNumber.Int64 < 1 {
		writeError(w, http.StatusConflict, "CHAPTER_ORDER_INVALID", "章节编号数据无效，无法安全调整顺序")
		return
	}
	if len(activeIDs) > 0 {
		result, err := tx.ExecContext(r.Context(), `UPDATE chapters SET chapter_number = -chapter_number WHERE novel_id = ? AND deleted_at IS NULL`, novelID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "暂存章节顺序失败")
			return
		}
		if changed, _ := result.RowsAffected(); changed != int64(len(activeIDs)) {
			writeError(w, http.StatusConflict, "CHAPTERS_CHANGED", "章节列表已变化，请刷新后重试")
			return
		}
		for index, chapterID := range input.ChapterIDs {
			result, err := tx.ExecContext(r.Context(), `UPDATE chapters SET chapter_number = ? WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, slots[index], chapterID, novelID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节顺序失败")
				return
			}
			if changed, _ := result.RowsAffected(); changed != 1 {
				writeError(w, http.StatusConflict, "CHAPTERS_CHANGED", "章节列表已变化，请刷新后重试")
				return
			}
		}
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE novels SET updated_at = ? WHERE id = ? AND deleted_at IS NULL`, database.Now(), novelID); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节顺序失败")
		return
	}
	if err := task.RebuildNarrativeThreads(r.Context(), tx, novelID); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "更新章节记忆状态失败")
		return
	}
	orderedRows, err := tx.QueryContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, goal, key_events, suspense_hook, COALESCE(deleted_at, ''), updated_at FROM chapters WHERE novel_id = ? AND deleted_at IS NULL ORDER BY chapter_number`, novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取新章节顺序失败")
		return
	}
	chapters := make([]Chapter, 0, len(activeIDs))
	for orderedRows.Next() {
		var chapter Chapter
		if err := orderedRows.Scan(&chapter.ID, &chapter.NovelID, &chapter.Number, &chapter.Title, &chapter.Body, &chapter.Revision, &chapter.Status, &chapter.Goal, &chapter.KeyEvents, &chapter.SuspenseHook, &chapter.DeletedAt, &chapter.UpdatedAt); err != nil {
			orderedRows.Close()
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取新章节顺序失败")
			return
		}
		chapters = append(chapters, chapter)
	}
	if err := orderedRows.Err(); err != nil {
		orderedRows.Close()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取新章节顺序失败")
		return
	}
	orderedRows.Close()
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节顺序失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chapters": chapters})
}

func (s *Server) deleteChapter(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "移入回收站失败")
		return
	}
	defer tx.Rollback()
	if s.hasActiveNovelTasks(w, r, tx, novelID) {
		return
	}
	now := database.Now()
	result, err := tx.ExecContext(r.Context(), `UPDATE chapters SET deleted_at = ?, updated_at = ? WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, now, now, r.PathValue("chapterID"), novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "移入回收站失败")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, http.StatusNotFound, "CHAPTER_NOT_FOUND", "章节不存在")
		return
	}
	if err := task.RebuildNarrativeThreads(r.Context(), tx, novelID); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "更新章节记忆状态失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "移入回收站失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) restoreChapter(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "恢复章节失败")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `UPDATE chapters SET deleted_at = NULL, updated_at = ? WHERE id = ? AND novel_id = ? AND deleted_at IS NOT NULL`, database.Now(), r.PathValue("chapterID"), r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "恢复章节失败")
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "恢复章节失败")
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "CHAPTER_NOT_FOUND", "已删除的章节不存在")
		return
	}
	if err := task.RebuildNarrativeThreads(r.Context(), tx, r.PathValue("novelID")); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "更新章节记忆状态失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "恢复章节失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) hasActiveNovelTasks(w http.ResponseWriter, r *http.Request, tx *sql.Tx, novelID string) bool {
	var count int
	if err := tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tasks WHERE novel_id = ? AND status IN ('queued', 'running')`, novelID).Scan(&count); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "检查运行中任务失败")
		return true
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "TASKS_RUNNING", "请等待当前生成任务完成后再移入回收站")
		return true
	}
	return false
}

func (s *Server) createChapter(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input struct {
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if len([]rune(input.Title)) > 120 {
		writeError(w, http.StatusBadRequest, "INVALID_CHAPTER", "章节标题不能超过 120 字")
		return
	}
	var number int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(MAX(chapter_number), 0) + 1 FROM chapters WHERE novel_id = ?`, r.PathValue("novelID")).Scan(&number); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建章节失败")
		return
	}
	if input.Title == "" {
		input.Title = "第" + strconv.Itoa(number) + "章"
	}
	chapter := Chapter{ID: database.NewID(), NovelID: r.PathValue("novelID"), Number: number, Title: input.Title, Revision: 1, Status: "draft", UpdatedAt: database.Now(), MemoryStatus: "missing"}
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO chapters (id, novel_id, chapter_number, title, body, revision, status, updated_at) VALUES (?, ?, ?, ?, '', 1, 'draft', ?)`, chapter.ID, chapter.NovelID, chapter.Number, chapter.Title, chapter.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建章节失败")
		return
	}
	writeJSON(w, http.StatusCreated, chapter)
}

func (s *Server) updateChapter(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input struct {
		ExpectedRevision  int    `json:"expectedRevision"`
		ExpectedUpdatedAt string `json:"expectedUpdatedAt"`
		Title             string `json:"title"`
		Body              string `json:"body"`
		Goal              string `json:"goal"`
		KeyEvents         string `json:"keyEvents"`
		SuspenseHook      string `json:"suspenseHook"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Goal = strings.TrimSpace(input.Goal)
	input.KeyEvents = strings.TrimSpace(input.KeyEvents)
	input.SuspenseHook = strings.TrimSpace(input.SuspenseHook)
	if input.ExpectedRevision < 1 || input.Title == "" || len([]rune(input.Title)) > 120 ||
		len([]rune(input.Goal)) > 1000 || len([]rune(input.KeyEvents)) > 2000 || len([]rune(input.SuspenseHook)) > 500 {
		writeError(w, http.StatusBadRequest, "INVALID_CHAPTER", "章节标题、版本号或蓝图字段无效")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节失败")
		return
	}
	var current Chapter
	err = tx.QueryRowContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, goal, key_events, suspense_hook, updated_at,
		CASE WHEN NOT EXISTS (SELECT 1 FROM chapter_memories m WHERE m.chapter_id = chapters.id) THEN 'missing' WHEN EXISTS (SELECT 1 FROM chapter_memories m WHERE m.chapter_id = chapters.id AND m.source_revision = chapters.revision) THEN 'current' ELSE 'stale' END
		FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, r.PathValue("chapterID"), r.PathValue("novelID")).Scan(&current.ID, &current.NovelID, &current.Number, &current.Title, &current.Body, &current.Revision, &current.Status, &current.Goal, &current.KeyEvents, &current.SuspenseHook, &current.UpdatedAt, &current.MemoryStatus)
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
	if current.Revision != input.ExpectedRevision {
		tx.Rollback()
		writeError(w, http.StatusConflict, "STALE_REVISION", "章节已被其他人更新，请刷新后再保存")
		return
	}
	if input.ExpectedUpdatedAt != "" && current.UpdatedAt != input.ExpectedUpdatedAt {
		tx.Rollback()
		writeError(w, http.StatusConflict, "STALE_REVISION", "章节蓝图或正文已被其他人更新，请比较后再保存")
		return
	}
	bodyChanged := current.Title != input.Title || current.Body != input.Body
	blueprintChanged := current.Goal != input.Goal || current.KeyEvents != input.KeyEvents || current.SuspenseHook != input.SuspenseHook
	if !bodyChanged && !blueprintChanged {
		tx.Rollback()
		writeJSON(w, http.StatusOK, current)
		return
	}
	if bodyChanged {
		if err := storeChapterRevision(tx, current, userID, "manual"); err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "记录章节版本失败")
			return
		}
	}
	now := database.Now()
	result, err := tx.ExecContext(r.Context(), `UPDATE chapters SET title = ?, body = ?, goal = ?, key_events = ?, suspense_hook = ?, revision = revision + ?, updated_at = ? WHERE id = ? AND novel_id = ? AND revision = ? AND (? = '' OR updated_at = ?)`, input.Title, input.Body, input.Goal, input.KeyEvents, input.SuspenseHook, boolToInt(bodyChanged), now, current.ID, current.NovelID, current.Revision, input.ExpectedUpdatedAt, input.ExpectedUpdatedAt)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节失败")
		return
	}
	if count, _ := result.RowsAffected(); count != 1 {
		tx.Rollback()
		writeError(w, http.StatusConflict, "STALE_REVISION", "章节已被其他人更新，请比较后再保存")
		return
	}
	if bodyChanged {
		if err := task.RebuildNarrativeThreads(r.Context(), tx, current.NovelID); err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "更新章节记忆状态失败")
			return
		}
	}
	var chapter Chapter
	if err := tx.QueryRowContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, goal, key_events, suspense_hook, updated_at,
		CASE WHEN NOT EXISTS (SELECT 1 FROM chapter_memories m WHERE m.chapter_id = chapters.id) THEN 'missing' WHEN EXISTS (SELECT 1 FROM chapter_memories m WHERE m.chapter_id = chapters.id AND m.source_revision = chapters.revision) THEN 'current' ELSE 'stale' END
		FROM chapters WHERE id = ? AND deleted_at IS NULL`, current.ID).Scan(&chapter.ID, &chapter.NovelID, &chapter.Number, &chapter.Title, &chapter.Body, &chapter.Revision, &chapter.Status, &chapter.Goal, &chapter.KeyEvents, &chapter.SuspenseHook, &chapter.UpdatedAt, &chapter.MemoryStatus); err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取已保存章节失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存章节失败")
		return
	}
	writeJSON(w, http.StatusOK, chapter)
}

func (s *Server) requireWorkspaceWriter(w http.ResponseWriter, r *http.Request, workspaceID, userID string) bool {
	var role string
	err := s.db.QueryRowContext(r.Context(), `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "工作区不存在或无权访问")
		return false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取工作区权限失败")
		return false
	}
	if role != "owner" && role != "editor" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "当前角色没有创建小说的权限")
		return false
	}
	return true
}

func normalizeApprovalMode(value string) string {
	if value == "auto" {
		return "auto"
	}
	return "manual"
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
