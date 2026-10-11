package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

func migrateSQLite(db *sql.DB) error {
	var currentVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&currentVersion); err != nil {
		return fmt.Errorf("read SQLite schema version: %w", err)
	}
	if currentVersion > sqliteSchemaVersion {
		return fmt.Errorf("database schema version %d is newer than this app supports (%d)", currentVersion, sqliteSchemaVersion)
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
			workspace_id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL DEFAULT 'local',
			project_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			subtitle TEXT NOT NULL DEFAULT '',
			genre TEXT NOT NULL DEFAULT '',
			premise TEXT NOT NULL DEFAULT '',
			author TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			word_target INTEGER NOT NULL DEFAULT 120000,
			current_view TEXT NOT NULL DEFAULT 'editor'
		)`,
		`CREATE INDEX IF NOT EXISTS workspaces_owner_idx ON workspaces(owner_id)`,
		`CREATE TABLE IF NOT EXISTS chapters (
			id INTEGER PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			number INTEGER NOT NULL,
			title TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'outline',
			word_count INTEGER NOT NULL DEFAULT 0,
			last_edited_at TEXT NOT NULL,
			UNIQUE(workspace_id, number),
			FOREIGN KEY(workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS chapters_workspace_order_idx ON chapters(workspace_id, number)`,
		`CREATE TABLE IF NOT EXISTS chapter_revisions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			chapter_id INTEGER NOT NULL,
			label TEXT NOT NULL DEFAULT '手动版本',
			title TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			word_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			FOREIGN KEY(workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
			FOREIGN KEY(chapter_id) REFERENCES chapters(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS chapter_revisions_chapter_order_idx ON chapter_revisions(workspace_id, chapter_id, id DESC)`,
		`CREATE TABLE IF NOT EXISTS characters (
			id INTEGER PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			traits_json TEXT NOT NULL DEFAULT '[]',
			color TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT 'idea',
			updated_at TEXT NOT NULL,
			FOREIGN KEY(workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id INTEGER PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			request_fingerprint TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS chat_workspace_order_idx ON chat_messages(workspace_id, id)`,
		`CREATE TABLE IF NOT EXISTS chat_requests (
			workspace_id TEXT NOT NULL,
			request_id TEXT NOT NULL,
			request_fingerprint TEXT NOT NULL DEFAULT '',
			user_content TEXT NOT NULL DEFAULT '',
			response_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY(workspace_id, request_id),
			FOREIGN KEY(workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS chat_request_claims (
			workspace_id TEXT NOT NULL,
			request_id TEXT NOT NULL,
			request_fingerprint TEXT NOT NULL,
			user_content TEXT NOT NULL,
			owner_token TEXT NOT NULL,
			lease_until INTEGER NOT NULL,
			PRIMARY KEY(workspace_id, request_id),
			FOREIGN KEY(workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS chat_request_claim_lease_idx ON chat_request_claims(lease_until)`,
		`CREATE TABLE IF NOT EXISTS settings (
			workspace_id TEXT PRIMARY KEY,
			provider TEXT NOT NULL DEFAULT 'local',
			base_url TEXT NOT NULL DEFAULT '',
			api_key TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT 'local-creative',
			temperature REAL NOT NULL DEFAULT 0.7,
			FOREIGN KEY(workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("migrate SQLite schema: %w", err)
		}
	}
	if err := ensureSQLiteColumn(db, "chat_messages", "request_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate chat request IDs: %w", err)
	}
	if err := ensureSQLiteColumn(db, "chat_messages", "request_fingerprint", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate chat request fingerprints: %w", err)
	}
	if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS chat_request_id_idx ON chat_messages(workspace_id, request_id) WHERE request_id <> ''"); err != nil {
		return fmt.Errorf("index chat request IDs: %w", err)
	}
	if currentVersion < chatRequestSchemaVersion {
		if err := migrateChatRequestRecords(db); err != nil {
			return fmt.Errorf("migrate chat request results: %w", err)
		}
	}
	if currentVersion < chapterFTSSchemaVersion {
		if err := migrateChapterSearchIndex(db); err != nil {
			return fmt.Errorf("migrate chapter search index: %w", err)
		}
	}
	if currentVersion < sqliteSchemaVersion {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", sqliteSchemaVersion)); err != nil {
			return fmt.Errorf("record SQLite schema version: %w", err)
		}
	}
	return nil
}

func migrateChapterSearchIndex(db *sql.DB) error {
	statements := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS chapters_fts USING fts5(
			title, summary, content,
			content='chapters', content_rowid='id',
			tokenize='trigram'
		)`,
		`CREATE TRIGGER IF NOT EXISTS chapters_fts_insert AFTER INSERT ON chapters BEGIN
			INSERT INTO chapters_fts(rowid, title, summary, content)
			VALUES (new.id, new.title, new.summary, new.content);
		END`,
		`CREATE TRIGGER IF NOT EXISTS chapters_fts_update AFTER UPDATE OF title, summary, content ON chapters BEGIN
			INSERT INTO chapters_fts(chapters_fts, rowid, title, summary, content)
			VALUES ('delete', old.id, old.title, old.summary, old.content);
			INSERT INTO chapters_fts(rowid, title, summary, content)
			VALUES (new.id, new.title, new.summary, new.content);
		END`,
		`CREATE TRIGGER IF NOT EXISTS chapters_fts_delete AFTER DELETE ON chapters BEGIN
			INSERT INTO chapters_fts(chapters_fts, rowid, title, summary, content)
			VALUES ('delete', old.id, old.title, old.summary, old.content);
		END`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	_, err := db.Exec(`INSERT INTO chapters_fts(chapters_fts) VALUES ('rebuild')`)
	return err
}

func ensureSQLiteColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var index int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&index, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

func migrateChatRequestRecords(db *sql.DB) error {
	rows, err := db.Query(`SELECT workspace_id, id, role, content, created_at, mode, request_id, request_fingerprint
		FROM chat_messages ORDER BY workspace_id, id`)
	if err != nil {
		return fmt.Errorf("read chat messages for request migration: %w", err)
	}
	type storedMessage struct {
		workspaceID string
		message     ChatMessage
	}
	messages := make([]storedMessage, 0)
	for rows.Next() {
		var item storedMessage
		if err := rows.Scan(&item.workspaceID, &item.message.ID, &item.message.Role, &item.message.Content,
			&item.message.CreatedAt, &item.message.Mode, &item.message.RequestID, &item.message.RequestFingerprint); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read chat message for request migration: %w", err)
		}
		messages = append(messages, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate chat messages for request migration: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close chat message migration query: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin chat request migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for index, item := range messages {
		userMessage := item.message
		if userMessage.Role != "user" || userMessage.RequestID == "" {
			continue
		}
		responseJSON := ""
		if index+1 < len(messages) {
			next := messages[index+1]
			if next.workspaceID == item.workspaceID && next.message.Role == "assistant" && next.message.Mode == userMessage.Mode {
				response := ChatResponse{UserMessage: userMessage, Message: next.message, Suggestions: suggestionsFor(userMessage.Mode)}
				encoded, err := json.Marshal(response)
				if err != nil {
					return fmt.Errorf("encode migrated chat request %s: %w", userMessage.RequestID, err)
				}
				responseJSON = string(encoded)
			}
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO chat_requests
			(workspace_id, request_id, request_fingerprint, user_content, response_json, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, item.workspaceID, userMessage.RequestID, userMessage.RequestFingerprint,
			userMessage.Content, responseJSON, userMessage.CreatedAt); err != nil {
			return fmt.Errorf("migrate chat request %s: %w", userMessage.RequestID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit chat request migration: %w", err)
	}
	return nil
}
