package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const localWorkspaceID = "default"
const chatRequestSchemaVersion = 5
const sqliteSchemaVersion = 7
const chapterFTSSchemaVersion = 7
const maxLegacyWorkspaceBytes int64 = 256 << 20

func openSQLiteStore(path string) (*Store, string, error) {
	databasePath, legacyPath := storagePaths(path)
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		databasePath = path
		legacyPath = ""
	} else {
		absoluteDatabasePath, err := filepath.Abs(databasePath)
		if err != nil {
			return nil, "", fmt.Errorf("resolve database path: %w", err)
		}
		databasePath = absoluteDatabasePath
		if legacyPath != "" {
			absoluteLegacyPath, err := filepath.Abs(legacyPath)
			if err != nil {
				return nil, "", fmt.Errorf("resolve legacy workspace path: %w", err)
			}
			legacyPath = absoluteLegacyPath
		}
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		return nil, "", fmt.Errorf("create data directory: %w", err)
	}
	if protectedPath, shouldProtect := sqliteProtectionPath(databasePath); shouldProtect {
		if err := protectSQLiteFiles(protectedPath); err != nil {
			return nil, "", err
		}
	}
	if legacyPath != "" {
		if err := protectLocalFile(legacyPath, "legacy workspace file"); err != nil {
			return nil, "", err
		}
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, "", fmt.Errorf("open SQLite database: %w", err)
	}
	// One connection keeps the connection-scoped pragmas consistent and avoids
	// write contention while the first version has a single local workspace.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, databasePath: databasePath}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, "", fmt.Errorf("connect to SQLite database %s: %w", databasePath, err)
	}
	if protectedPath, shouldProtect := sqliteProtectionPath(databasePath); shouldProtect {
		if err := protectSQLiteFiles(protectedPath); err != nil {
			_ = db.Close()
			return nil, "", err
		}
	}
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			return nil, "", fmt.Errorf("configure SQLite: %w", err)
		}
	}
	if err := migrateSQLite(db); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if protectedPath, shouldProtect := sqliteProtectionPath(databasePath); shouldProtect {
		if err := protectSQLiteFiles(protectedPath); err != nil {
			_ = db.Close()
			return nil, "", err
		}
	}
	return store, legacyPath, nil
}

func sqliteProtectionPath(databasePath string) (string, bool) {
	if databasePath == ":memory:" {
		return "", false
	}
	if !strings.HasPrefix(databasePath, "file:") {
		return databasePath, true
	}
	parsed, err := url.Parse(databasePath)
	if err != nil || parsed.Scheme != "file" || parsed.Query().Get("mode") == "memory" || parsed.Opaque == ":memory:" {
		return "", false
	}
	path := parsed.Path
	if path == "" {
		path = parsed.Opaque
	}
	path, err = url.PathUnescape(path)
	if err != nil || path == "" {
		return "", false
	}
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return path, true
}

func protectSQLiteFiles(databasePath string) error {
	for _, filePath := range []string{databasePath, databasePath + "-wal", databasePath + "-shm", databasePath + "-journal"} {
		if err := protectLocalFile(filePath, "database file"); err != nil {
			return err
		}
	}
	return nil
}

func protectLocalFile(filePath, description string) error {
	info, err := os.Stat(filePath)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect local %s %s: %w", description, filePath, err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if err := os.Chmod(filePath, 0o600); err != nil && !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("protect local %s %s: %w", description, filePath, err)
	}
	return nil
}

func storagePaths(path string) (databasePath, legacyPath string) {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return strings.TrimSuffix(path, filepath.Ext(path)) + ".db", path
	}
	return path, filepath.Join(filepath.Dir(path), "workspace.json")
}

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

func (s *Store) loadOrInitialize(legacyPath string) error {
	rows, err := s.db.Query(`SELECT workspace_id FROM workspaces ORDER BY updated_at DESC`)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read workspace id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate workspace ids: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close workspace query: %w", err)
	}
	if len(ids) > 0 {
		for _, id := range ids {
			workspace, err := loadWorkspaceMetadata(s.db, id)
			if err != nil {
				return err
			}
			workspace.ID = id
			normalizeWorkspaceMetadata(&workspace)
			s.workspaces[id] = workspace
		}
		return nil
	}

	workspace, err := readLegacyWorkspace(legacyPath)
	if err != nil {
		return err
	}
	if workspace == nil {
		seeded := seedWorkspace()
		workspace = &seeded
	}
	if workspace.ID == "" {
		workspace.ID = localWorkspaceID
	}
	normalizeWorkspace(workspace)
	if err := persistWorkspace(s.db, workspace.ID, *workspace); err != nil {
		return fmt.Errorf("initialize local workspace: %w", err)
	}
	if err := migrateChatRequestRecords(s.db); err != nil {
		return fmt.Errorf("migrate initialized chat request results: %w", err)
	}
	s.workspaces[workspace.ID] = workspaceMetadata(*workspace)
	return nil
}

func readLegacyWorkspace(path string) (*Workspace, error) {
	if path == "" {
		return nil, nil
	}
	if err := protectLocalFile(path, "legacy workspace file"); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read legacy workspace %s: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect legacy workspace %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("legacy workspace %s is not a regular file", path)
	}
	if info.Size() > maxLegacyWorkspaceBytes {
		_ = file.Close()
		return nil, fmt.Errorf("legacy workspace %s exceeds the %d MiB migration limit", path, maxLegacyWorkspaceBytes/(1<<20))
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxLegacyWorkspaceBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read legacy workspace %s: %w", path, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close legacy workspace %s: %w", path, closeErr)
	}
	if int64(len(data)) > maxLegacyWorkspaceBytes {
		return nil, fmt.Errorf("legacy workspace %s exceeds the %d MiB migration limit", path, maxLegacyWorkspaceBytes/(1<<20))
	}
	var workspace Workspace
	if err := json.Unmarshal(data, &workspace); err != nil {
		return nil, fmt.Errorf("decode legacy workspace %s: %w", path, err)
	}
	var legacySettings struct {
		Settings *struct {
			Temperature *float64 `json:"temperature"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(data, &legacySettings); err != nil {
		return nil, fmt.Errorf("decode legacy workspace settings %s: %w", path, err)
	}
	if legacySettings.Settings == nil || legacySettings.Settings.Temperature == nil {
		workspace.Settings.Temperature = 0.7
	}
	if workspace.Project.ID == 0 || strings.TrimSpace(workspace.Project.Title) == "" {
		return nil, fmt.Errorf("legacy workspace %s has no valid project", path)
	}
	return &workspace, nil
}

func persistWorkspace(db *sql.DB, workspaceID string, workspace Workspace) error {
	return persistWorkspaceWithChapterRevisions(db, workspaceID, workspace, nil)
}

func persistWorkspaceWithChapterRevisions(db *sql.DB, workspaceID string, workspace Workspace, revisions []ChapterRevision) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin workspace transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	project := workspace.Project
	_, err = tx.Exec(`INSERT INTO workspaces (
		workspace_id, owner_id, project_id, title, subtitle, genre, premise, author, status,
		created_at, updated_at, word_target, current_view
	) VALUES (?, 'local', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(workspace_id) DO UPDATE SET
		project_id=excluded.project_id, title=excluded.title, subtitle=excluded.subtitle,
		genre=excluded.genre, premise=excluded.premise, author=excluded.author, status=excluded.status,
		created_at=excluded.created_at, updated_at=excluded.updated_at,
		word_target=excluded.word_target, current_view=excluded.current_view`,
		workspaceID, project.ID, project.Title, project.Subtitle, project.Genre, project.Premise,
		project.Author, project.Status, project.CreatedAt, project.UpdatedAt, project.WordTarget, project.CurrentView)
	if err != nil {
		return fmt.Errorf("save workspace project: %w", err)
	}
	for _, table := range []string{"chapter_revisions", "chapters", "characters", "notes", "chat_messages"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE workspace_id = ?`, workspaceID); err != nil {
			return fmt.Errorf("replace workspace %s: %w", table, err)
		}
	}
	for _, chapter := range workspace.Chapters {
		if _, err := tx.Exec(`INSERT INTO chapters (id, workspace_id, number, title, summary, content, status, word_count, last_edited_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, chapter.ID, workspaceID, chapter.Number, chapter.Title,
			chapter.Summary, chapter.Content, chapter.Status, chapter.WordCount, chapter.LastEditedAt); err != nil {
			return fmt.Errorf("save chapter %d: %w", chapter.ID, err)
		}
	}
	revisionChapterIDs := make(map[int64]struct{})
	for _, revision := range revisions {
		if _, err := tx.Exec(`INSERT INTO chapter_revisions (workspace_id, chapter_id, label, title, summary, content, status, word_count, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, workspaceID, revision.ChapterID, revision.Label, revision.Title,
			revision.Summary, revision.Content, revision.Status, revision.WordCount, revision.CreatedAt); err != nil {
			return fmt.Errorf("save chapter revision %d: %w", revision.ID, err)
		}
		revisionChapterIDs[revision.ChapterID] = struct{}{}
	}
	for chapterID := range revisionChapterIDs {
		if err := pruneChapterRevisions(tx, workspaceID, chapterID); err != nil {
			return err
		}
	}
	for _, character := range workspace.Characters {
		traits, err := json.Marshal(character.Traits)
		if err != nil {
			return fmt.Errorf("encode character traits: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO characters (id, workspace_id, name, role, description, traits_json, color)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, character.ID, workspaceID, character.Name, character.Role,
			character.Description, string(traits), character.Color); err != nil {
			return fmt.Errorf("save character %d: %w", character.ID, err)
		}
	}
	for _, note := range workspace.Notes {
		if _, err := tx.Exec(`INSERT INTO notes (id, workspace_id, title, body, kind, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, note.ID, workspaceID, note.Title, note.Body, note.Kind, note.UpdatedAt); err != nil {
			return fmt.Errorf("save note %d: %w", note.ID, err)
		}
	}
	for _, message := range workspace.Chat {
		if _, err := tx.Exec(`INSERT INTO chat_messages (id, workspace_id, role, content, created_at, mode, request_id, request_fingerprint)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, message.ID, workspaceID, message.Role, message.Content, message.CreatedAt, message.Mode, message.RequestID, message.RequestFingerprint); err != nil {
			return fmt.Errorf("save chat message %d: %w", message.ID, err)
		}
	}
	settings := workspace.Settings
	if _, err := tx.Exec(`INSERT INTO settings (workspace_id, provider, base_url, api_key, model, temperature)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			provider=excluded.provider, base_url=excluded.base_url, api_key=excluded.api_key,
			model=excluded.model, temperature=excluded.temperature`,
		workspaceID, settings.Provider, settings.BaseURL, settings.APIKey, settings.Model, settings.Temperature); err != nil {
		return fmt.Errorf("save workspace settings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workspace transaction: %w", err)
	}
	return nil
}

func loadWorkspace(db *sql.DB, workspaceID string) (Workspace, error) {
	return loadWorkspaceWithChapterContent(db, workspaceID, true)
}

func loadWorkspaceMetadata(db *sql.DB, workspaceID string) (Workspace, error) {
	return loadWorkspaceWithChapterContent(db, workspaceID, false)
}

func loadWorkspaceWithChapterContent(db *sql.DB, workspaceID string, includeContent bool) (Workspace, error) {
	workspace := Workspace{ID: workspaceID, Chapters: []Chapter{}, Characters: []Character{}, Notes: []Note{}, Chat: []ChatMessage{}}
	project := &workspace.Project
	if err := db.QueryRow(`SELECT project_id, title, subtitle, genre, premise, author, status,
		created_at, updated_at, word_target, current_view FROM workspaces WHERE workspace_id = ?`, workspaceID).Scan(
		&project.ID, &project.Title, &project.Subtitle, &project.Genre, &project.Premise, &project.Author,
		&project.Status, &project.CreatedAt, &project.UpdatedAt, &project.WordTarget, &project.CurrentView); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Workspace{}, fmt.Errorf("load workspace project: %w", os.ErrNotExist)
		}
		return Workspace{}, fmt.Errorf("load workspace project: %w", err)
	}
	workspace.UpdatedAt = project.UpdatedAt

	chapterQuery := `SELECT id, number, title, summary, content, status, word_count, last_edited_at
		FROM chapters WHERE workspace_id = ? ORDER BY number, id`
	if !includeContent {
		chapterQuery = `SELECT id, number, title, summary, '' AS content, status, word_count, last_edited_at
			FROM chapters WHERE workspace_id = ? ORDER BY number, id`
	}
	chapterRows, err := db.Query(chapterQuery, workspaceID)
	if err != nil {
		return Workspace{}, fmt.Errorf("query chapters: %w", err)
	}
	for chapterRows.Next() {
		var chapter Chapter
		if err := chapterRows.Scan(&chapter.ID, &chapter.Number, &chapter.Title, &chapter.Summary,
			&chapter.Content, &chapter.Status, &chapter.WordCount, &chapter.LastEditedAt); err != nil {
			_ = chapterRows.Close()
			return Workspace{}, fmt.Errorf("read chapter: %w", err)
		}
		workspace.Chapters = append(workspace.Chapters, chapter)
	}
	if err := chapterRows.Err(); err != nil {
		_ = chapterRows.Close()
		return Workspace{}, fmt.Errorf("iterate chapters: %w", err)
	}
	if err := chapterRows.Close(); err != nil {
		return Workspace{}, fmt.Errorf("close chapter query: %w", err)
	}

	characterRows, err := db.Query(`SELECT id, name, role, description, traits_json, color
		FROM characters WHERE workspace_id = ? ORDER BY id`, workspaceID)
	if err != nil {
		return Workspace{}, fmt.Errorf("query characters: %w", err)
	}
	for characterRows.Next() {
		var character Character
		var traits string
		if err := characterRows.Scan(&character.ID, &character.Name, &character.Role, &character.Description, &traits, &character.Color); err != nil {
			_ = characterRows.Close()
			return Workspace{}, fmt.Errorf("read character: %w", err)
		}
		if err := json.Unmarshal([]byte(traits), &character.Traits); err != nil {
			_ = characterRows.Close()
			return Workspace{}, fmt.Errorf("decode character traits: %w", err)
		}
		workspace.Characters = append(workspace.Characters, character)
	}
	if err := characterRows.Err(); err != nil {
		_ = characterRows.Close()
		return Workspace{}, fmt.Errorf("iterate characters: %w", err)
	}
	if err := characterRows.Close(); err != nil {
		return Workspace{}, fmt.Errorf("close character query: %w", err)
	}

	noteRows, err := db.Query(`SELECT id, title, body, kind, updated_at FROM notes WHERE workspace_id = ? ORDER BY id`, workspaceID)
	if err != nil {
		return Workspace{}, fmt.Errorf("query notes: %w", err)
	}
	for noteRows.Next() {
		var note Note
		if err := noteRows.Scan(&note.ID, &note.Title, &note.Body, &note.Kind, &note.UpdatedAt); err != nil {
			_ = noteRows.Close()
			return Workspace{}, fmt.Errorf("read note: %w", err)
		}
		workspace.Notes = append(workspace.Notes, note)
	}
	if err := noteRows.Err(); err != nil {
		_ = noteRows.Close()
		return Workspace{}, fmt.Errorf("iterate notes: %w", err)
	}
	if err := noteRows.Close(); err != nil {
		return Workspace{}, fmt.Errorf("close note query: %w", err)
	}

	chatRows, err := db.Query(`SELECT id, role, content, created_at, mode, request_id, request_fingerprint FROM chat_messages
		WHERE workspace_id = ? ORDER BY id DESC LIMIT 60`, workspaceID)
	if err != nil {
		return Workspace{}, fmt.Errorf("query chat history: %w", err)
	}
	for chatRows.Next() {
		var message ChatMessage
		if err := chatRows.Scan(&message.ID, &message.Role, &message.Content, &message.CreatedAt, &message.Mode, &message.RequestID, &message.RequestFingerprint); err != nil {
			_ = chatRows.Close()
			return Workspace{}, fmt.Errorf("read chat message: %w", err)
		}
		workspace.Chat = append(workspace.Chat, message)
	}
	if err := chatRows.Err(); err != nil {
		_ = chatRows.Close()
		return Workspace{}, fmt.Errorf("iterate chat history: %w", err)
	}
	if err := chatRows.Close(); err != nil {
		return Workspace{}, fmt.Errorf("close chat query: %w", err)
	}
	for left, right := 0, len(workspace.Chat)-1; left < right; left, right = left+1, right-1 {
		workspace.Chat[left], workspace.Chat[right] = workspace.Chat[right], workspace.Chat[left]
	}

	workspace.Settings = Settings{Provider: "local", Model: "local-creative", Temperature: 0.7}
	settings := &workspace.Settings
	err = db.QueryRow(`SELECT provider, base_url, api_key, model, temperature FROM settings WHERE workspace_id = ?`, workspaceID).Scan(
		&settings.Provider, &settings.BaseURL, &settings.APIKey, &settings.Model, &settings.Temperature)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, fmt.Errorf("load workspace settings: %w", err)
	}
	return workspace, nil
}

func loadChapter(db *sql.DB, workspaceID string, chapterID int64) (Chapter, error) {
	var chapter Chapter
	err := db.QueryRow(`SELECT id, number, title, summary, content, status, word_count, last_edited_at
		FROM chapters WHERE workspace_id = ? AND id = ?`, workspaceID, chapterID).Scan(
		&chapter.ID, &chapter.Number, &chapter.Title, &chapter.Summary, &chapter.Content,
		&chapter.Status, &chapter.WordCount, &chapter.LastEditedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Chapter{}, os.ErrNotExist
	}
	if err != nil {
		return Chapter{}, fmt.Errorf("load chapter %d: %w", chapterID, err)
	}
	return chapter, nil
}

type chatRequestRecord struct {
	WorkspaceID        string
	RequestID          string
	RequestFingerprint string
	UserContent        string
	OwnerToken         string
	Response           ChatResponse
	CreatedAt          string
}

func newChatRequestRecord(workspaceID, requestID, requestFingerprint, user string, response ChatResponse, ownerToken string) chatRequestRecord {
	return chatRequestRecord{
		WorkspaceID: workspaceID, RequestID: requestID, RequestFingerprint: requestFingerprint,
		UserContent: user, OwnerToken: ownerToken, Response: response, CreatedAt: now(),
	}
}

type chatRequestQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func lookupChatRequestDB(db chatRequestQueryer, workspaceID, requestID, user, requestFingerprint string) (ChatResponse, bool, error) {
	var storedFingerprint, storedUser, responseJSON string
	err := db.QueryRow(`SELECT request_fingerprint, user_content, response_json FROM chat_requests
		WHERE workspace_id = ? AND request_id = ?`, workspaceID, requestID).Scan(&storedFingerprint, &storedUser, &responseJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ChatResponse{}, false, nil
	}
	if err != nil {
		return ChatResponse{}, false, fmt.Errorf("load chat request result: %w", err)
	}
	existing := ChatMessage{Content: storedUser, RequestFingerprint: storedFingerprint}
	if err := chatRequestMismatchError(existing, user, requestFingerprint); err != nil {
		return ChatResponse{}, false, err
	}
	if responseJSON == "" {
		return ChatResponse{}, false, ErrChatRequestUnverifiable
	}
	var response ChatResponse
	if err := json.Unmarshal([]byte(responseJSON), &response); err != nil {
		return ChatResponse{}, false, fmt.Errorf("decode chat request result: %w", err)
	}
	return response, true, nil
}

func appendChatResponseDB(db *sql.DB, workspaceID, requestID, user, requestFingerprint, ownerToken string, response ChatResponse) (ChatResponse, error) {
	tx, err := db.Begin()
	if err != nil {
		return ChatResponse{}, fmt.Errorf("begin chat append transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Acquire SQLite's write lock before reading the request ledger or allocating
	// message IDs so independent Store instances cannot append from the same cache.
	if _, err := tx.Exec("UPDATE workspaces SET updated_at = updated_at WHERE workspace_id = ?", workspaceID); err != nil {
		return ChatResponse{}, fmt.Errorf("lock workspace for chat append: %w", err)
	}
	var exists int
	if err := tx.QueryRow("SELECT 1 FROM workspaces WHERE workspace_id = ?", workspaceID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ChatResponse{}, os.ErrNotExist
		}
		return ChatResponse{}, fmt.Errorf("check workspace for chat append: %w", err)
	}

	if requestID != "" {
		if stored, found, err := lookupChatRequestDB(tx, workspaceID, requestID, user, requestFingerprint); err != nil {
			return ChatResponse{}, err
		} else if found {
			if ownerToken != "" {
				if _, err := tx.Exec("DELETE FROM chat_request_claims WHERE workspace_id = ? AND request_id = ? AND owner_token = ?", workspaceID, requestID, ownerToken); err != nil {
					return ChatResponse{}, fmt.Errorf("release completed chat request claim: %w", err)
				}
			}
			if err := tx.Commit(); err != nil {
				return ChatResponse{}, fmt.Errorf("commit chat request replay: %w", err)
			}
			return stored, nil
		}

		var storedUser, storedFingerprint string
		err := tx.QueryRow("SELECT content, request_fingerprint FROM chat_messages WHERE workspace_id = ? AND request_id = ? AND role = 'user'", workspaceID, requestID).Scan(&storedUser, &storedFingerprint)
		if err == nil {
			if err := chatRequestMismatchError(ChatMessage{Content: storedUser, RequestFingerprint: storedFingerprint}, user, requestFingerprint); err != nil {
				return ChatResponse{}, err
			}
			return ChatResponse{}, ErrChatRequestUnverifiable
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return ChatResponse{}, fmt.Errorf("check existing chat message: %w", err)
		}
	}

	var maxID int64
	if err := tx.QueryRow("SELECT COALESCE(MAX(id), 0) FROM (SELECT project_id AS id FROM workspaces UNION ALL SELECT id FROM chapters UNION ALL SELECT id FROM characters UNION ALL SELECT id FROM notes UNION ALL SELECT id FROM chat_messages)").Scan(&maxID); err != nil {
		return ChatResponse{}, fmt.Errorf("allocate chat message IDs: %w", err)
	}
	response.UserMessage.ID = maxID + 1
	response.Message.ID = maxID + 2
	response.UserMessage.RequestID = requestID
	response.UserMessage.RequestFingerprint = requestFingerprint

	updatedAt := now()
	if _, err := tx.Exec("UPDATE workspaces SET updated_at = ? WHERE workspace_id = ?", updatedAt, workspaceID); err != nil {
		return ChatResponse{}, fmt.Errorf("update workspace activity: %w", err)
	}
	for _, message := range []ChatMessage{response.UserMessage, response.Message} {
		if _, err := tx.Exec("INSERT INTO chat_messages (id, workspace_id, role, content, created_at, mode, request_id, request_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", message.ID, workspaceID, message.Role, message.Content, message.CreatedAt, message.Mode, message.RequestID, message.RequestFingerprint); err != nil {
			return ChatResponse{}, fmt.Errorf("save chat message %d: %w", message.ID, err)
		}
	}
	if _, err := tx.Exec("DELETE FROM chat_messages WHERE workspace_id = ? AND id NOT IN (SELECT id FROM chat_messages WHERE workspace_id = ? ORDER BY id DESC LIMIT 60)", workspaceID, workspaceID); err != nil {
		return ChatResponse{}, fmt.Errorf("prune chat history: %w", err)
	}
	if requestID != "" {
		record := newChatRequestRecord(workspaceID, requestID, requestFingerprint, user, response, ownerToken)
		if err := insertChatRequestRecord(tx, record); err != nil {
			return ChatResponse{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ChatResponse{}, fmt.Errorf("commit chat append: %w", err)
	}
	return response, nil
}

func saveChatRequestRecord(db *sql.DB, record chatRequestRecord) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin chat request result transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertChatRequestRecord(tx, record); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit chat request result: %w", err)
	}
	return nil
}

func insertChatRequestRecord(tx *sql.Tx, record chatRequestRecord) error {
	if record.OwnerToken == "" {
		return ErrChatRequestClaimLost
	}
	var currentOwner string
	var leaseUntil int64
	if err := tx.QueryRow(`SELECT owner_token, lease_until FROM chat_request_claims
		WHERE workspace_id = ? AND request_id = ?`, record.WorkspaceID, record.RequestID).Scan(&currentOwner, &leaseUntil); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatRequestClaimLost
		}
		return fmt.Errorf("verify chat request claim: %w", err)
	}
	if currentOwner != record.OwnerToken || leaseUntil <= time.Now().UnixMilli() {
		return ErrChatRequestClaimLost
	}
	responseJSON, err := json.Marshal(record.Response)
	if err != nil {
		return fmt.Errorf("encode chat request result: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO chat_requests (workspace_id, request_id, request_fingerprint, user_content, response_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, record.WorkspaceID, record.RequestID, record.RequestFingerprint, record.UserContent, string(responseJSON), record.CreatedAt); err != nil {
		return fmt.Errorf("save chat request result: %w", err)
	}
	if record.OwnerToken != "" {
		if _, err := tx.Exec(`DELETE FROM chat_request_claims WHERE workspace_id = ? AND request_id = ? AND owner_token = ?`, record.WorkspaceID, record.RequestID, record.OwnerToken); err != nil {
			return fmt.Errorf("complete chat request claim: %w", err)
		}
	}
	return nil
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
