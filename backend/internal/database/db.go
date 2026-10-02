package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, statErr := os.Stat(absolutePath)
	existingDatabase := statErr == nil && info.Size() > 0
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("inspect sqlite database: %w", statErr)
	}
	dsn := "file:" + filepath.ToSlash(absolutePath)
	dsn += "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if existingDatabase {
		version, err := currentMigrationVersion(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("read sqlite migration version: %w", err)
		}
		if version > latestMigrationVersion() {
			db.Close()
			return nil, fmt.Errorf("database schema version %d is newer than supported version %d", version, latestMigrationVersion())
		}
		if version < latestMigrationVersion() {
			if _, err := backupBeforeMigration(db, absolutePath); err != nil {
				db.Close()
				return nil, fmt.Errorf("backup sqlite database before migration: %w", err)
			}
		}
	}
	for _, statement := range schemaStatements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize sqlite schema: %w", err)
		}
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

type migration struct {
	version int
	name    string
	apply   func(*sql.Tx) error
}

// Versioned migrations make future schema changes explicit while keeping
// upgrades from the pre-versioned schema compatible.
var migrations = []migration{
	{
		version: 1,
		name:    "legacy-columns",
		apply:   migrateLegacyColumns,
	},
	{
		version: 2,
		name:    "chapter-memory",
		apply:   migrateChapterMemory,
	},
	{
		version: 3,
		name:    "chapter-memory-source-revision",
		apply:   migrateChapterMemorySourceRevision,
	},
	{
		version: 4,
		name:    "proposal-demo-mode",
		apply:   migrateProposalDemoMode,
	},
	{
		version: 5,
		name:    "character-states",
		apply:   migrateCharacterStates,
	},
}

type columnMigration struct {
	table      string
	column     string
	definition string
}

var columnMigrations = []columnMigration{
	{"users", "session_version", "INTEGER NOT NULL DEFAULT 1"},
	{"novels", "writing_guide", "TEXT NOT NULL DEFAULT ''"},
	{"novels", "deleted_at", "TEXT"},
	{"chapters", "deleted_at", "TEXT"},
	{"chapters", "goal", "TEXT NOT NULL DEFAULT ''"},
	{"chapters", "key_events", "TEXT NOT NULL DEFAULT ''"},
	{"chapters", "suspense_hook", "TEXT NOT NULL DEFAULT ''"},
	{"tasks", "request_json", "TEXT NOT NULL DEFAULT ''"},
	{"tasks", "retry_task_id", "TEXT NOT NULL DEFAULT ''"},
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema migration table: %w", err)
	}
	currentVersion, err := currentMigrationVersion(db)
	if err != nil {
		return fmt.Errorf("read sqlite migration version: %w", err)
	}
	if currentVersion > latestMigrationVersion() {
		return fmt.Errorf("database schema version %d is newer than supported version %d", currentVersion, latestMigrationVersion())
	}
	for _, next := range migrations {
		if next.version <= currentVersion {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d (%s): %w", next.version, next.name, err)
		}
		if err := next.apply(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d (%s): %w", next.version, next.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			next.version, next.name, Now()); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d (%s): %w", next.version, next.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d (%s): %w", next.version, next.name, err)
		}
	}
	return nil
}

func migrateLegacyColumns(tx *sql.Tx) error {
	for _, item := range columnMigrations {
		exists, err := columnExists(tx, item.table, item.column)
		if err != nil {
			return fmt.Errorf("inspect %s.%s: %w", item.table, item.column, err)
		}
		if exists {
			continue
		}
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", item.table, item.column, item.definition)
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("migrate %s.%s: %w", item.table, item.column, err)
		}
	}
	return nil
}

func migrateChapterMemory(tx *sql.Tx) error {
	for _, statement := range chapterMemoryStatements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func migrateChapterMemorySourceRevision(tx *sql.Tx) error {
	exists, err := columnExists(tx, "chapter_memories", "source_revision")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := tx.Exec(`ALTER TABLE chapter_memories ADD COLUMN source_revision INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	// Older versions did not record which body revision produced a memory, so
	// leave those rows stale instead of treating an unknown snapshot as current.
	_, err = tx.Exec(`DELETE FROM narrative_threads`)
	return err
}

func migrateProposalDemoMode(tx *sql.Tx) error {
	exists, err := columnExists(tx, "proposals", "demo_mode")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = tx.Exec(`ALTER TABLE proposals ADD COLUMN demo_mode INTEGER NOT NULL DEFAULT 0`)
	return err
}

func migrateCharacterStates(tx *sql.Tx) error {
	for _, statement := range characterStateStatements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
}

func columnExists(db queryer, table, column string) (bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func currentMigrationVersion(db *sql.DB) (int, error) {
	var tableName string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&tableName)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func latestMigrationVersion() int {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].version
}

func backupBeforeMigration(db *sql.DB, databasePath string) (string, error) {
	backupPath := fmt.Sprintf("%s.pre-migration-%s.sqlite", databasePath, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if _, err := db.Exec(`VACUUM INTO ?`, backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

func NewID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80
	value := hex.EncodeToString(buffer)
	return fmt.Sprintf("%s-%s-%s-%s-%s", value[:8], value[8:12], value[12:16], value[16:20], value[20:])
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// These statements establish tables and indexes for fresh databases. Changes
// to existing tables belong in a new versioned migration.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    session_version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);`,
	`CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL
);`,
	`CREATE TABLE IF NOT EXISTS workspace_members (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'reviewer', 'viewer')),
    joined_at TEXT NOT NULL,
    PRIMARY KEY (workspace_id, user_id)
);`,
	`CREATE TABLE IF NOT EXISTS novels (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    genre TEXT NOT NULL DEFAULT '',
    planned_chapters INTEGER NOT NULL DEFAULT 0,
    approval_mode TEXT NOT NULL DEFAULT 'manual' CHECK (approval_mode IN ('manual', 'auto')),
    writing_guide TEXT NOT NULL DEFAULT '',
    deleted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);`,
	`CREATE TABLE IF NOT EXISTS chapters (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    chapter_number INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'draft',
    goal TEXT NOT NULL DEFAULT '',
    key_events TEXT NOT NULL DEFAULT '',
    suspense_hook TEXT NOT NULL DEFAULT '',
    deleted_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE (novel_id, chapter_number)
);`,
	`CREATE TABLE IF NOT EXISTS chapter_revisions (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    chapter_id TEXT NOT NULL REFERENCES chapters(id) ON DELETE CASCADE,
    revision INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    source TEXT NOT NULL,
    edited_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    UNIQUE (chapter_id, revision)
);`,
	`CREATE TABLE IF NOT EXISTS graph_nodes (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('character', 'place', 'faction', 'item', 'concept')),
    label TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    x REAL NOT NULL DEFAULT 0,
    y REAL NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE (id, novel_id)
);`,
	`CREATE TABLE IF NOT EXISTS graph_edges (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    relation TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    FOREIGN KEY (source_id, novel_id) REFERENCES graph_nodes(id, novel_id) ON DELETE CASCADE,
    FOREIGN KEY (target_id, novel_id) REFERENCES graph_nodes(id, novel_id) ON DELETE CASCADE,
    CHECK (source_id <> target_id)
);`,
	`CREATE TABLE IF NOT EXISTS workflows (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    definition_json TEXT NOT NULL DEFAULT '{"nodes":[],"edges":[]}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);`,
	`CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    workflow_id TEXT REFERENCES workflows(id) ON DELETE SET NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    error_text TEXT NOT NULL DEFAULT '',
    request_json TEXT NOT NULL DEFAULT '',
    retry_task_id TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);`,
	`CREATE TABLE IF NOT EXISTS task_events (
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (task_id, sequence)
);`,
	`CREATE TABLE IF NOT EXISTS proposals (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    chapter_id TEXT NOT NULL REFERENCES chapters(id) ON DELETE CASCADE,
    base_revision INTEGER NOT NULL,
    proposed_title TEXT NOT NULL,
    proposed_text TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'applied', 'rejected')),
    demo_mode INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    resolved_at TEXT
);`,
	`CREATE INDEX IF NOT EXISTS idx_novels_workspace ON novels(workspace_id, updated_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_chapters_novel ON chapters(novel_id, chapter_number);`,
	`CREATE INDEX IF NOT EXISTS idx_graph_nodes_novel ON graph_nodes(novel_id, kind);`,
	`CREATE INDEX IF NOT EXISTS idx_graph_edges_novel ON graph_edges(novel_id);`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_novel ON tasks(novel_id, created_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_task_events_task ON task_events(task_id, sequence);`,
	`CREATE INDEX IF NOT EXISTS idx_proposals_novel_status ON proposals(novel_id, status, created_at DESC);`,
}

// chapterMemoryStatements create the chapter-memory tables. Fresh databases
// run them from schemaStatements; existing databases run them from migration
// version 2. Both use IF NOT EXISTS so a second startup does not duplicate rows.
var chapterMemoryStatements = []string{
	`CREATE TABLE IF NOT EXISTS chapter_memories (
    chapter_id TEXT PRIMARY KEY REFERENCES chapters(id) ON DELETE CASCADE,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    chapter_number INTEGER NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    characters TEXT NOT NULL DEFAULT '',
    new_settings TEXT NOT NULL DEFAULT '',
    foreshadowing TEXT NOT NULL DEFAULT '',
    source_revision INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);`,
	`CREATE TABLE IF NOT EXISTS narrative_threads (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    subjects TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('open', 'recovered')),
    opened_chapter_id TEXT NOT NULL,
    opened_chapter_number INTEGER NOT NULL,
    updated_chapter_id TEXT NOT NULL,
    updated_chapter_number INTEGER NOT NULL,
    UNIQUE (novel_id, name)
);`,
	`CREATE INDEX IF NOT EXISTS idx_chapter_memories_novel ON chapter_memories(novel_id, chapter_number);`,
	`CREATE INDEX IF NOT EXISTS idx_narrative_threads_novel ON narrative_threads(novel_id, status);`,
}

var characterStateStatements = []string{
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_chapters_id_novel ON chapters(id, novel_id);`,
	`CREATE TABLE IF NOT EXISTS character_states (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL REFERENCES novels(id) ON DELETE CASCADE,
    character_id TEXT NOT NULL,
    chapter_id TEXT NOT NULL,
    appellation TEXT NOT NULL DEFAULT '',
    age TEXT NOT NULL DEFAULT '',
    abilities TEXT NOT NULL DEFAULT '',
    stance TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (character_id, chapter_id),
    FOREIGN KEY (character_id, novel_id) REFERENCES graph_nodes(id, novel_id) ON DELETE CASCADE,
    FOREIGN KEY (chapter_id, novel_id) REFERENCES chapters(id, novel_id) ON DELETE CASCADE
);`,
	`CREATE INDEX IF NOT EXISTS idx_character_states_novel_chapter ON character_states(novel_id, chapter_id);`,
}

func init() {
	schemaStatements = append(schemaStatements, chapterMemoryStatements...)
	schemaStatements = append(schemaStatements, characterStateStatements...)
}
