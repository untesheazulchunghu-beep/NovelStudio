package app

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrInvalidChapterRevisionInput = errors.New("invalid chapter revision input")

func (s *Store) CreateChapterRevision(workspaceID string, chapterID int64, input ChapterRevisionInput) (ChapterRevision, error) {
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = "手动版本"
	}
	if err := validateTextLength("chapter revision label", label, maxChapterRevisionLabelLength); err != nil {
		return ChapterRevision{}, fmt.Errorf("%w: %v", ErrInvalidChapterRevisionInput, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); err != nil {
		return ChapterRevision{}, err
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return ChapterRevision{}, os.ErrNotExist
	}
	chapter, err := loadChapter(s.db, workspaceID, chapterID)
	if errors.Is(err, os.ErrNotExist) {
		return ChapterRevision{}, os.ErrNotExist
	}
	if err != nil {
		return ChapterRevision{}, err
	}

	latest, err := latestChapterRevision(s.db, workspaceID, chapterID)
	if err == nil && sameChapterRevisionSnapshot(latest, chapter) && latest.Label == label {
		return latest, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ChapterRevision{}, fmt.Errorf("load latest chapter revision: %w", err)
	}

	createdAt := now()
	revision := chapterRevisionFromChapter(chapter, label, createdAt)
	tx, err := s.db.Begin()
	if err != nil {
		return ChapterRevision{}, fmt.Errorf("begin chapter revision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	revision.ID, err = insertChapterRevision(tx, workspaceID, revision)
	if err != nil {
		return ChapterRevision{}, err
	}
	if err := pruneChapterRevisions(tx, workspaceID, chapterID); err != nil {
		return ChapterRevision{}, err
	}
	if err := touchWorkspaceTx(tx, workspaceID, createdAt); err != nil {
		return ChapterRevision{}, err
	}
	if err := tx.Commit(); err != nil {
		return ChapterRevision{}, fmt.Errorf("commit chapter revision: %w", err)
	}
	workspace.Project.UpdatedAt = createdAt
	workspace.UpdatedAt = createdAt
	s.workspaces[workspaceID] = workspaceMetadata(workspace)
	return revision, nil
}

func (s *Store) ListChapterRevisions(workspaceID string, chapterID int64) ([]ChapterRevisionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); err != nil {
		return nil, err
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return nil, os.ErrNotExist
	}
	if _, exists := findChapter(workspace, chapterID); !exists {
		return nil, os.ErrNotExist
	}
	rows, err := s.db.Query(`SELECT id, chapter_id, label, title, summary, status, word_count, created_at
		FROM chapter_revisions WHERE workspace_id = ? AND chapter_id = ? ORDER BY id DESC LIMIT ?`, workspaceID, chapterID, maxChapterRevisions)
	if err != nil {
		return nil, fmt.Errorf("query chapter revisions: %w", err)
	}
	defer rows.Close()
	revisions := make([]ChapterRevisionSummary, 0)
	for rows.Next() {
		var revision ChapterRevisionSummary
		if err := rows.Scan(&revision.ID, &revision.ChapterID, &revision.Label, &revision.Title, &revision.Summary,
			&revision.Status, &revision.WordCount, &revision.CreatedAt); err != nil {
			return nil, fmt.Errorf("read chapter revision summary: %w", err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chapter revisions: %w", err)
	}
	return revisions, nil
}

func (s *Store) GetChapterRevision(workspaceID string, chapterID, revisionID int64) (ChapterRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); err != nil {
		return ChapterRevision{}, err
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return ChapterRevision{}, os.ErrNotExist
	}
	if _, exists := findChapter(workspace, chapterID); !exists {
		return ChapterRevision{}, os.ErrNotExist
	}
	return getChapterRevision(s.db, workspaceID, chapterID, revisionID)
}

func (s *Store) RestoreChapterRevision(workspaceID string, chapterID, revisionID int64) (Chapter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); err != nil {
		return Chapter{}, err
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return Chapter{}, os.ErrNotExist
	}
	current, err := loadChapter(s.db, workspaceID, chapterID)
	if errors.Is(err, os.ErrNotExist) {
		return Chapter{}, os.ErrNotExist
	}
	if err != nil {
		return Chapter{}, err
	}
	revision, err := getChapterRevision(s.db, workspaceID, chapterID, revisionID)
	if err != nil {
		return Chapter{}, err
	}
	if sameChapterRevisionSnapshot(revision, current) {
		return current, nil
	}

	updatedAt := now()
	restored := current
	restored.Title = revision.Title
	restored.Summary = revision.Summary
	restored.Content = revision.Content
	restored.Status = revision.Status
	restored.WordCount = countWords(restored.Content)
	restored.LastEditedAt = updatedAt

	tx, err := s.db.Begin()
	if err != nil {
		return Chapter{}, fmt.Errorf("begin chapter restore: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	backup := chapterRevisionFromChapter(current, "恢复前自动备份", updatedAt)
	if _, err := insertChapterRevision(tx, workspaceID, backup); err != nil {
		return Chapter{}, err
	}
	if err := pruneChapterRevisions(tx, workspaceID, chapterID); err != nil {
		return Chapter{}, err
	}
	if _, err := tx.Exec(`UPDATE chapters SET title = ?, summary = ?, content = ?, status = ?, word_count = ?, last_edited_at = ?
		WHERE workspace_id = ? AND id = ?`, restored.Title, restored.Summary, restored.Content, restored.Status,
		restored.WordCount, restored.LastEditedAt, workspaceID, chapterID); err != nil {
		return Chapter{}, fmt.Errorf("restore chapter content: %w", err)
	}
	if err := touchWorkspaceTx(tx, workspaceID, updatedAt); err != nil {
		return Chapter{}, err
	}
	if err := tx.Commit(); err != nil {
		return Chapter{}, fmt.Errorf("commit chapter restore: %w", err)
	}
	for index := range workspace.Chapters {
		if workspace.Chapters[index].ID == chapterID {
			workspace.Chapters[index] = restored
			break
		}
	}
	workspace.Project.UpdatedAt = updatedAt
	workspace.UpdatedAt = updatedAt
	s.workspaces[workspaceID] = workspaceMetadata(workspace)
	return restored, nil
}

func loadChapterRevisions(db *sql.DB, workspaceID string) ([]ChapterRevision, error) {
	rows, err := db.Query(`SELECT id, chapter_id, label, title, summary, content, status, word_count, created_at
		FROM chapter_revisions WHERE workspace_id = ? ORDER BY chapter_id, id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query project chapter revisions: %w", err)
	}
	defer rows.Close()
	revisions := make([]ChapterRevision, 0)
	for rows.Next() {
		var revision ChapterRevision
		if err := rows.Scan(&revision.ID, &revision.ChapterID, &revision.Label, &revision.Title,
			&revision.Summary, &revision.Content, &revision.Status, &revision.WordCount, &revision.CreatedAt); err != nil {
			return nil, fmt.Errorf("read project chapter revision: %w", err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project chapter revisions: %w", err)
	}
	return revisions, nil
}

func getChapterRevision(db *sql.DB, workspaceID string, chapterID, revisionID int64) (ChapterRevision, error) {
	var revision ChapterRevision
	err := db.QueryRow(`SELECT id, chapter_id, label, title, summary, content, status, word_count, created_at
		FROM chapter_revisions WHERE workspace_id = ? AND chapter_id = ? AND id = ?`, workspaceID, chapterID, revisionID).Scan(
		&revision.ID, &revision.ChapterID, &revision.Label, &revision.Title, &revision.Summary,
		&revision.Content, &revision.Status, &revision.WordCount, &revision.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ChapterRevision{}, os.ErrNotExist
	}
	if err != nil {
		return ChapterRevision{}, fmt.Errorf("load chapter revision: %w", err)
	}
	return revision, nil
}

func latestChapterRevision(db *sql.DB, workspaceID string, chapterID int64) (ChapterRevision, error) {
	var revision ChapterRevision
	err := db.QueryRow(`SELECT id, chapter_id, label, title, summary, content, status, word_count, created_at
		FROM chapter_revisions WHERE workspace_id = ? AND chapter_id = ? ORDER BY id DESC LIMIT 1`, workspaceID, chapterID).Scan(
		&revision.ID, &revision.ChapterID, &revision.Label, &revision.Title, &revision.Summary,
		&revision.Content, &revision.Status, &revision.WordCount, &revision.CreatedAt)
	return revision, err
}

func insertChapterRevision(tx *sql.Tx, workspaceID string, revision ChapterRevision) (int64, error) {
	result, err := tx.Exec(`INSERT INTO chapter_revisions (workspace_id, chapter_id, label, title, summary, content, status, word_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, workspaceID, revision.ChapterID, revision.Label, revision.Title,
		revision.Summary, revision.Content, revision.Status, revision.WordCount, revision.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("save chapter revision: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read chapter revision ID: %w", err)
	}
	return id, nil
}

func pruneChapterRevisions(tx *sql.Tx, workspaceID string, chapterID int64) error {
	_, err := tx.Exec(`DELETE FROM chapter_revisions
		WHERE workspace_id = ? AND chapter_id = ? AND id NOT IN (
			SELECT id FROM chapter_revisions WHERE workspace_id = ? AND chapter_id = ? ORDER BY id DESC LIMIT ?
		)`, workspaceID, chapterID, workspaceID, chapterID, maxChapterRevisions)
	if err != nil {
		return fmt.Errorf("prune old chapter revisions: %w", err)
	}
	return nil
}

func touchWorkspaceTx(tx *sql.Tx, workspaceID, updatedAt string) error {
	if _, err := tx.Exec(`UPDATE workspaces SET updated_at = ? WHERE workspace_id = ?`, updatedAt, workspaceID); err != nil {
		return fmt.Errorf("update workspace timestamp: %w", err)
	}
	return nil
}

func findChapter(workspace Workspace, chapterID int64) (Chapter, bool) {
	for _, chapter := range workspace.Chapters {
		if chapter.ID == chapterID {
			return chapter, true
		}
	}
	return Chapter{}, false
}

func chapterRevisionFromChapter(chapter Chapter, label, createdAt string) ChapterRevision {
	return ChapterRevision{
		ChapterID: chapter.ID, Label: label, Title: chapter.Title, Summary: chapter.Summary,
		Content: chapter.Content, Status: chapter.Status, WordCount: chapter.WordCount, CreatedAt: createdAt,
	}
}

func sameChapterRevisionSnapshot(revision ChapterRevision, chapter Chapter) bool {
	return revision.Title == chapter.Title && revision.Summary == chapter.Summary &&
		revision.Content == chapter.Content && revision.Status == chapter.Status
}
