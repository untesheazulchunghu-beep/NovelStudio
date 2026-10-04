package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"novelstudio/internal/database"
)

// Action represents the type of action performed.
type Action string

const (
	ActionApplyProposal    Action = "apply_proposal"
	ActionDeleteChapter    Action = "delete_chapter"
	ActionDeleteNovel      Action = "delete_novel"
	ActionRestoreChapter   Action = "restore_chapter"
	ActionRestoreNovel     Action = "restore_novel"
	ActionCreateWorkflow   Action = "create_workflow"
	ActionDeleteWorkflow   Action = "delete_workflow"
	ActionUpdateSettings   Action = "update_settings"
	ActionExportNovel      Action = "export_novel"
	ActionImportReference  Action = "import_reference"
	ActionDeleteReference  Action = "delete_reference"
	ActionChangeProvider   Action = "change_provider"
)

// TargetType represents the type of resource being acted upon.
type TargetType string

const (
	TargetTypeProposal  TargetType = "proposal"
	TargetTypeChapter   TargetType = "chapter"
	TargetTypeNovel     TargetType = "novel"
	TargetTypeWorkflow  TargetType = "workflow"
	TargetTypeSettings  TargetType = "settings"
	TargetTypeReference TargetType = "reference"
	TargetTypeProvider  TargetType = "provider"
)

// Entry represents a single audit log entry.
type Entry struct {
	ID         string                 `json:"id"`
	UserID     string                 `json:"user_id"`
	NovelID    *string                `json:"novel_id,omitempty"`
	Action     Action                 `json:"action"`
	TargetType TargetType             `json:"target_type"`
	TargetID   string                 `json:"target_id"`
	Metadata   map[string]interface{} `json:"metadata"`
	CreatedAt  string                 `json:"created_at"`
}

// Log writes an audit log entry to the database.
func Log(ctx context.Context, db *sql.DB, userID string, novelID *string, action Action, targetType TargetType, targetID string, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	id := database.NewID()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, user_id, novel_id, action, target_type, target_id, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, novelID, action, targetType, targetID, string(metadataJSON), now)

	return err
}

// LogTx writes an audit log entry within an existing transaction.
func LogTx(ctx context.Context, tx *sql.Tx, userID string, novelID *string, action Action, targetType TargetType, targetID string, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	id := database.NewID()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs (id, user_id, novel_id, action, target_type, target_id, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, novelID, action, targetType, targetID, string(metadataJSON), now)

	return err
}

// Query retrieves audit log entries with optional filters.
func Query(ctx context.Context, db *sql.DB, userID *string, novelID *string, action *Action, limit int, offset int) ([]Entry, error) {
	query := `
		SELECT id, user_id, novel_id, action, target_type, target_id, metadata, created_at
		FROM audit_logs
		WHERE 1=1
	`
	args := []interface{}{}

	if userID != nil {
		query += ` AND user_id = ?`
		args = append(args, *userID)
	}
	if novelID != nil {
		query += ` AND novel_id = ?`
		args = append(args, *novelID)
	}
	if action != nil {
		query += ` AND action = ?`
		args = append(args, *action)
	}

	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var novelID sql.NullString
		var metadataJSON string

		if err := rows.Scan(&e.ID, &e.UserID, &novelID, &e.Action, &e.TargetType, &e.TargetID, &metadataJSON, &e.CreatedAt); err != nil {
			return nil, err
		}

		if novelID.Valid {
			e.NovelID = &novelID.String
		}

		if err := json.Unmarshal([]byte(metadataJSON), &e.Metadata); err != nil {
			return nil, err
		}

		entries = append(entries, e)
	}

	return entries, rows.Err()
}
