package novel

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"novelstudio/internal/database"
)

type ChapterRevision struct {
	Revision  int    `json:"revision"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"`
}

func storeChapterRevision(tx *sql.Tx, chapter Chapter, userID, source string) error {
	_, err := tx.Exec(`INSERT INTO chapter_revisions (id, novel_id, chapter_id, revision, title, body, source, edited_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, database.NewID(), chapter.NovelID, chapter.ID, chapter.Revision, chapter.Title, chapter.Body, source, userID, database.Now())
	return err
}

func (s *Server) listChapterRevisions(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT revision, title, body, source, created_at FROM chapter_revisions WHERE novel_id = ? AND chapter_id = ? AND EXISTS (SELECT 1 FROM chapters c WHERE c.id = chapter_revisions.chapter_id AND c.deleted_at IS NULL) ORDER BY revision DESC`, r.PathValue("novelID"), r.PathValue("chapterID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节版本失败")
		return
	}
	defer rows.Close()
	revisions := make([]ChapterRevision, 0)
	for rows.Next() {
		var revision ChapterRevision
		if err := rows.Scan(&revision.Revision, &revision.Title, &revision.Body, &revision.Source, &revision.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节版本失败")
			return
		}
		revisions = append(revisions, revision)
	}
	writeJSON(w, http.StatusOK, map[string]any{"revisions": revisions})
}

func (s *Server) restoreChapterRevision(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	revisionNumber, err := strconv.Atoi(r.PathValue("revision"))
	if err != nil || revisionNumber < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REVISION", "版本号无效")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "恢复章节版本失败")
		return
	}
	var current Chapter
	err = tx.QueryRowContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, goal, key_events, suspense_hook, updated_at FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, r.PathValue("chapterID"), r.PathValue("novelID")).Scan(&current.ID, &current.NovelID, &current.Number, &current.Title, &current.Body, &current.Revision, &current.Status, &current.Goal, &current.KeyEvents, &current.SuspenseHook, &current.UpdatedAt)
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
	var targetTitle, targetBody string
	err = tx.QueryRowContext(r.Context(), `SELECT title, body FROM chapter_revisions WHERE chapter_id = ? AND novel_id = ? AND revision = ?`, current.ID, current.NovelID, revisionNumber).Scan(&targetTitle, &targetBody)
	if errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		writeError(w, http.StatusNotFound, "REVISION_NOT_FOUND", "找不到指定章节版本")
		return
	}
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节版本失败")
		return
	}
	if current.Title == targetTitle && current.Body == targetBody {
		tx.Rollback()
		writeJSON(w, http.StatusOK, current)
		return
	}
	if err := storeChapterRevision(tx, current, userID, "restore"); err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "记录当前版本失败")
		return
	}
	now := database.Now()
	_, err = tx.ExecContext(r.Context(), `UPDATE chapters SET title = ?, body = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ?`, targetTitle, targetBody, now, current.ID, current.Revision)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "恢复章节版本失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "恢复章节版本失败")
		return
	}
	var restored Chapter
	if err := s.db.QueryRowContext(r.Context(), `SELECT id, novel_id, chapter_number, title, body, revision, status, goal, key_events, suspense_hook, updated_at FROM chapters WHERE id = ? AND deleted_at IS NULL`, current.ID).Scan(&restored.ID, &restored.NovelID, &restored.Number, &restored.Title, &restored.Body, &restored.Revision, &restored.Status, &restored.Goal, &restored.KeyEvents, &restored.SuspenseHook, &restored.UpdatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取恢复后的章节失败")
		return
	}
	writeJSON(w, http.StatusOK, restored)
}
