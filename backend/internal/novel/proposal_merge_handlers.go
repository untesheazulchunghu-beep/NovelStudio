package novel

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

type proposalMergeSnapshot struct {
	ProposalID      string `json:"proposalId"`
	ChapterID       string `json:"chapterId"`
	BaseRevision    int    `json:"baseRevision"`
	CurrentRevision int    `json:"currentRevision"`
	BaseTitle       string `json:"baseTitle"`
	BaseText        string `json:"baseText"`
	CurrentTitle    string `json:"currentTitle"`
	CurrentText     string `json:"currentText"`
	ProposalTitle   string `json:"proposalTitle"`
	ProposalText    string `json:"proposalText"`
}

func (s *Server) getProposalMergePreview(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor", "reviewer", "viewer") {
		return
	}

	var snapshot proposalMergeSnapshot
	var status string
	err := s.db.QueryRowContext(r.Context(), `SELECT p.chapter_id, p.base_revision, p.proposed_title, p.proposed_text,
		p.status, c.title, c.body, c.revision
		FROM proposals p
		JOIN chapters c ON c.id = p.chapter_id AND c.novel_id = p.novel_id AND c.deleted_at IS NULL
		WHERE p.id = ? AND p.novel_id = ?`, r.PathValue("proposalID"), novelID).
		Scan(&snapshot.ChapterID, &snapshot.BaseRevision, &snapshot.ProposalTitle, &snapshot.ProposalText,
			&status, &snapshot.CurrentTitle, &snapshot.CurrentText, &snapshot.CurrentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "PROPOSAL_NOT_FOUND", "提案或对应章节不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取提案版本失败")
		return
	}
	if status != "pending" {
		writeError(w, http.StatusConflict, "PROPOSAL_RESOLVED", "该提案已处理")
		return
	}
	if snapshot.CurrentRevision == snapshot.BaseRevision {
		writeError(w, http.StatusConflict, "PROPOSAL_NOT_STALE", "该提案仍基于当前章节版本")
		return
	}
	err = s.db.QueryRowContext(r.Context(), `SELECT title, body FROM chapter_revisions
		WHERE novel_id = ? AND chapter_id = ? AND revision = ?`, novelID, snapshot.ChapterID, snapshot.BaseRevision).
		Scan(&snapshot.BaseTitle, &snapshot.BaseText)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusConflict, "BASE_REVISION_UNAVAILABLE", "找不到提案所依据的正文版本，请基于最新版重新生成")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取提案基准版本失败")
		return
	}
	snapshot.ProposalID = r.PathValue("proposalID")
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) applyMergedProposal(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	var input struct {
		ExpectedRevision int    `json:"expectedRevision"`
		Title            string `json:"title"`
		Text             string `json:"text"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.ExpectedRevision < 1 || input.Title == "" || len([]rune(input.Title)) > 120 {
		writeError(w, http.StatusBadRequest, "INVALID_MERGE", "合并稿标题或章节版本无效")
		return
	}

	proposalID := r.PathValue("proposalID")
	var chapterID, proposalStatus string
	var baseRevision int
	var demoMode bool
	err := s.db.QueryRowContext(r.Context(), `SELECT chapter_id, base_revision, status, demo_mode
		FROM proposals WHERE id = ? AND novel_id = ?`, proposalID, novelID).
		Scan(&chapterID, &baseRevision, &proposalStatus, &demoMode)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "PROPOSAL_NOT_FOUND", "提案不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取待合并提案失败")
		return
	}
	if proposalStatus != "pending" {
		writeError(w, http.StatusConflict, "PROPOSAL_RESOLVED", "该提案已处理")
		return
	}
	var current Chapter
	err = s.db.QueryRowContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, updated_at
		FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, chapterID, novelID).
		Scan(&current.ID, &current.NovelID, &current.Number, &current.Title, &current.Body, &current.Revision, &current.Status, &current.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "CHAPTER_NOT_FOUND", "章节不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取当前章节失败")
		return
	}
	if current.Revision == baseRevision {
		writeError(w, http.StatusConflict, "PROPOSAL_NOT_STALE", "该提案仍基于当前章节版本，请使用普通提案审核")
		return
	}
	if current.Revision != input.ExpectedRevision {
		writeError(w, http.StatusConflict, "STALE_REVISION", "预览后章节再次更新，请重新打开三方合并")
		return
	}
	var baseAvailable bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM chapter_revisions
		WHERE novel_id = ? AND chapter_id = ? AND revision = ?)`, novelID, chapterID, baseRevision).Scan(&baseAvailable); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "检查提案基准版本失败")
		return
	}
	if !baseAvailable {
		writeError(w, http.StatusConflict, "BASE_REVISION_UNAVAILABLE", "找不到提案所依据的正文版本，请基于最新版重新生成")
		return
	}
	if input.Title == current.Title && input.Text == current.Body {
		writeError(w, http.StatusBadRequest, "EMPTY_MERGE", "合并稿没有修改，章节和提案均未变化")
		return
	}

	storeMemory := !demoMode && !s.tasks.DemoMode()
	var memory task.ChapterMemory
	if storeMemory {
		memory, err = s.tasks.ComposeChapterMemory(r.Context(), task.Chapter{
			ID: current.ID, Number: current.Number, Title: input.Title, Body: input.Text,
		}, input.Text)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				writeError(w, http.StatusRequestTimeout, "MEMORY_GENERATION_CANCELED", "生成章节记忆已取消，合并稿尚未应用")
				return
			}
			storeMemory = false
		}
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "合并提案失败")
		return
	}
	defer tx.Rollback()
	var transactionStatus string
	var transactionBase int
	err = tx.QueryRowContext(r.Context(), `SELECT status, base_revision FROM proposals WHERE id = ? AND novel_id = ?`, proposalID, novelID).
		Scan(&transactionStatus, &transactionBase)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "PROPOSAL_NOT_FOUND", "提案不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "重新读取提案失败")
		return
	}
	if transactionStatus != "pending" {
		writeError(w, http.StatusConflict, "PROPOSAL_RESOLVED", "该提案已处理")
		return
	}
	if transactionBase != baseRevision {
		writeError(w, http.StatusConflict, "STALE_REVISION", "提案基准版本已变化，请重新打开三方合并")
		return
	}
	err = tx.QueryRowContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, updated_at
		FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, chapterID, novelID).
		Scan(&current.ID, &current.NovelID, &current.Number, &current.Title, &current.Body, &current.Revision, &current.Status, &current.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "CHAPTER_NOT_FOUND", "章节不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "重新读取章节失败")
		return
	}
	if current.Revision != input.ExpectedRevision {
		writeError(w, http.StatusConflict, "STALE_REVISION", "预览后章节再次更新，请重新打开三方合并")
		return
	}
	if err := storeChapterRevision(tx, current, userID, "proposal-merge"); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存合并前版本失败")
		return
	}
	result, err := tx.ExecContext(r.Context(), `UPDATE chapters SET title = ?, body = ?, revision = revision + 1, updated_at = ?
		WHERE id = ? AND novel_id = ? AND revision = ?`, input.Title, input.Text, database.Now(), chapterID, novelID, input.ExpectedRevision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存合并稿失败")
		return
	}
	updated, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "确认合并章节版本失败")
		return
	}
	if updated != 1 {
		writeError(w, http.StatusConflict, "STALE_REVISION", "预览后章节再次更新，请重新打开三方合并")
		return
	}
	proposalResult, err := tx.ExecContext(r.Context(), `UPDATE proposals SET status = 'applied', resolved_at = ?
		WHERE id = ? AND novel_id = ? AND status = 'pending' AND base_revision = ?`, database.Now(), proposalID, novelID, baseRevision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "完成提案合并失败")
		return
	}
	proposalUpdated, err := proposalResult.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "确认提案合并状态失败")
		return
	}
	if proposalUpdated != 1 {
		writeError(w, http.StatusConflict, "PROPOSAL_RESOLVED", "该提案已处理")
		return
	}
	if storeMemory {
		if err := task.SaveChapterMemory(r.Context(), tx, novelID, chapterID, current.Number, input.ExpectedRevision+1, memory); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存合并稿记忆失败")
			return
		}
	} else if err := task.RebuildNarrativeThreads(r.Context(), tx, novelID); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "更新章节记忆状态失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "合并提案失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revision": input.ExpectedRevision + 1})
}
