package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigrateAddsBlueprintColumns simulates a database created before the
// blueprint feature and verifies Open upgrades it in place without losing data.
func TestMigrateAddsBlueprintColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := `
CREATE TABLE novels (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    genre TEXT NOT NULL DEFAULT '',
    planned_chapters INTEGER NOT NULL DEFAULT 0,
    approval_mode TEXT NOT NULL DEFAULT 'manual',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE chapters (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL,
    chapter_number INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'draft',
    updated_at TEXT NOT NULL
);`
	if _, err := legacy.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO novels (id, workspace_id, title, created_at, updated_at) VALUES ('n1', 'w1', '旧作品', 'now', 'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO chapters (id, novel_id, chapter_number, title, updated_at) VALUES ('c1', 'n1', 1, '第一章', 'now')`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var guide string
	if err := db.QueryRow(`SELECT writing_guide FROM novels WHERE id = 'n1'`).Scan(&guide); err != nil {
		t.Fatalf("writing_guide should exist after migration: %v", err)
	}
	if guide != "" {
		t.Fatalf("existing row should default to an empty guide, got %q", guide)
	}
	var novelDeletedAt, chapterDeletedAt sql.NullString
	if err := db.QueryRow(`SELECT n.deleted_at, c.deleted_at FROM novels n JOIN chapters c ON c.novel_id = n.id WHERE n.id = 'n1'`).Scan(&novelDeletedAt, &chapterDeletedAt); err != nil {
		t.Fatalf("soft-delete columns should be added to legacy tables: %v", err)
	}
	if novelDeletedAt.Valid || chapterDeletedAt.Valid {
		t.Fatalf("legacy content should remain active after migration, got deleted_at values %q/%q", novelDeletedAt.String, chapterDeletedAt.String)
	}
	var title, goal string
	if err := db.QueryRow(`SELECT title, goal FROM chapters WHERE id = 'c1'`).Scan(&title, &goal); err != nil {
		t.Fatalf("chapters blueprint columns should exist after migration: %v", err)
	}
	if title != "第一章" || goal != "" {
		t.Fatalf("legacy chapter data should survive migration, got %q/%q", title, goal)
	}

	// Reopening must be idempotent rather than failing on duplicate columns.
	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopening a migrated database should succeed: %v", err)
	}
	again.Close()
}

func TestMigrateAddsRecoverableTaskFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-tasks.sqlite")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		novel_id TEXT NOT NULL,
		workflow_id TEXT,
		kind TEXT NOT NULL,
		status TEXT NOT NULL,
		progress INTEGER NOT NULL DEFAULT 0,
		error_text TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO tasks (id, novel_id, kind, status, created_by, created_at, updated_at) VALUES ('t1', 'n1', 'batch', 'failed', 'u1', 'now', 'now')`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var requestJSON, retryTaskID string
	if err := db.QueryRow(`SELECT request_json, retry_task_id FROM tasks WHERE id = 't1'`).Scan(&requestJSON, &retryTaskID); err != nil {
		t.Fatalf("legacy task fields should be readable after migration: %v", err)
	}
	if requestJSON != "" || retryTaskID != "" {
		t.Fatalf("legacy task fields should default to empty values, got %q/%q", requestJSON, retryTaskID)
	}
	if exists, err := columnExists(db, "tasks", "request_json"); err != nil || !exists {
		t.Fatalf("request_json migration missing: exists=%v err=%v", exists, err)
	}
	if exists, err := columnExists(db, "tasks", "retry_task_id"); err != nil || !exists {
		t.Fatalf("retry_task_id migration missing: exists=%v err=%v", exists, err)
	}
}

func TestMigrateAddsSessionVersionToExistingUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-users.sqlite")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	INSERT INTO users (id, email, password_hash, created_at) VALUES ('u1', 'legacy@example.test', 'hash', 'now');`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int64
	if err := db.QueryRow(`SELECT session_version FROM users WHERE id = 'u1'`).Scan(&version); err != nil {
		t.Fatalf("legacy user session version should be available after migration: %v", err)
	}
	if version != 1 {
		t.Fatalf("legacy user should start at session version 1, got %d", version)
	}
}

func TestMigrateRecordsVersionAndKeepsPreMigrationBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	INSERT INTO users (id, email, password_hash, created_at) VALUES ('u1', 'backup@example.test', 'hash', 'now');`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != latestMigrationVersion() {
		t.Fatalf("expected latest migration version %d, got %d", latestMigrationVersion(), version)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	backups, err := filepath.Glob(path + ".pre-migration-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one pre-migration backup, got %d", len(backups))
	}
	backup, err := sql.Open("sqlite", "file:"+filepath.ToSlash(backups[0])+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	var email string
	if err := backup.QueryRow(`SELECT email FROM users WHERE id = 'u1'`).Scan(&email); err != nil {
		t.Fatalf("backup should retain existing user data: %v", err)
	}
	if email != "backup@example.test" {
		t.Fatalf("backup user data changed: %q", email)
	}
	if exists, err := columnExists(backup, "users", "session_version"); err != nil || exists {
		t.Fatalf("backup should reflect the pre-migration schema: exists=%v err=%v", exists, err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
	backups, err = filepath.Glob(path + ".pre-migration-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("reopening an up-to-date database should not create another backup, got %d", len(backups))
	}
}

func TestOpenRejectsDatabaseFromNewerSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);
		INSERT INTO schema_migrations (version, name, applied_at) VALUES (999, 'future', 'now');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	opened, err := Open(path)
	if err == nil {
		opened.Close()
		t.Fatal("expected a newer database schema to be rejected")
	}
	if !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("expected a clear unsupported-schema error, got %v", err)
	}
	backups, globErr := filepath.Glob(path + ".pre-migration-*.sqlite")
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(backups) != 0 {
		t.Fatalf("unsupported schema should not create a backup, got %v", backups)
	}
}

func TestMigrateAddsChapterMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-memory.sqlite")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`
CREATE TABLE novels (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    genre TEXT NOT NULL DEFAULT '',
    planned_chapters INTEGER NOT NULL DEFAULT 0,
    approval_mode TEXT NOT NULL DEFAULT 'manual',
    writing_guide TEXT NOT NULL DEFAULT '',
    deleted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE chapters (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL,
    chapter_number INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'draft',
    goal TEXT NOT NULL DEFAULT '',
    key_events TEXT NOT NULL DEFAULT '',
    suspense_hook TEXT NOT NULL DEFAULT '',
    deleted_at TEXT,
    updated_at TEXT NOT NULL
);
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TEXT NOT NULL
);
INSERT INTO schema_migrations (version, name, applied_at) VALUES (1, 'legacy-columns', 'then');
INSERT INTO novels (id, workspace_id, title, writing_guide, created_at, updated_at) VALUES ('n1', 'w1', '旧作品', '旧指南', 'now', 'now');
INSERT INTO chapters (id, novel_id, chapter_number, title, goal, updated_at) VALUES ('c1', 'n1', 1, '旧章', '旧目标', 'now');
`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var title, goal, guide, appliedAt, memoryName string
	if err := db.QueryRow(`SELECT title, writing_guide FROM novels WHERE id = 'n1'`).Scan(&title, &guide); err != nil {
		t.Fatal(err)
	}
	var chapterTitle string
	if err := db.QueryRow(`SELECT title, goal FROM chapters WHERE id = 'c1'`).Scan(&chapterTitle, &goal); err != nil {
		t.Fatal(err)
	}
	if title != "旧作品" || guide != "旧指南" || chapterTitle != "旧章" || goal != "旧目标" {
		t.Fatalf("existing rows changed: novel=%q guide=%q chapter=%q goal=%q", title, guide, chapterTitle, goal)
	}
	if err := db.QueryRow(`SELECT applied_at FROM schema_migrations WHERE version = 1`).Scan(&appliedAt); err != nil {
		t.Fatal(err)
	}
	if appliedAt != "then" {
		t.Fatalf("already applied migration was rewritten: %q", appliedAt)
	}
	if err := db.QueryRow(`SELECT name FROM schema_migrations WHERE version = 2`).Scan(&memoryName); err != nil {
		t.Fatalf("chapter memory migration was not recorded: %v", err)
	}
	if memoryName != "chapter-memory" {
		t.Fatalf("unexpected migration name %q", memoryName)
	}
	var tableName string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'chapter_memories'`).Scan(&tableName); err != nil {
		t.Fatalf("chapter_memories table missing: %v", err)
	}
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'narrative_threads'`).Scan(&tableName); err != nil {
		t.Fatalf("narrative_threads table missing: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO chapter_memories (chapter_id, novel_id, chapter_number, summary, characters, new_settings, foreshadowing, updated_at) VALUES ('c1', 'n1', 1, '旧摘要', '旧人物', '旧设定', '', 't1')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopening an upgraded database should succeed: %v", err)
	}
	var versionRows int
	if err := reopened.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 2`).Scan(&versionRows); err != nil {
		t.Fatal(err)
	}
	if versionRows != 1 {
		t.Fatalf("second startup duplicated the migration, got %d version-2 rows", versionRows)
	}
	var memories int
	var summary string
	if err := reopened.QueryRow(`SELECT COUNT(*), MIN(summary) FROM chapter_memories`).Scan(&memories, &summary); err != nil {
		t.Fatal(err)
	}
	if memories != 1 || summary != "旧摘要" {
		t.Fatalf("second startup changed memory rows: count=%d summary=%q", memories, summary)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(path + ".pre-migration-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("second startup should not take another backup, got %d", len(backups))
	}

	futurePath := filepath.Join(t.TempDir(), "newer-than-memory.sqlite")
	future, err := sql.Open("sqlite", "file:"+filepath.ToSlash(futurePath))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := future.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);
		INSERT INTO schema_migrations (version, name, applied_at) VALUES (999, 'future', 'now');`); err != nil {
		t.Fatal(err)
	}
	if err := future.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(futurePath)
	if err == nil {
		opened.Close()
		t.Fatal("expected a newer database schema to be rejected")
	}
	if !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("expected a clear unsupported-schema error, got %v", err)
	}
}

func TestMigrateChapterMemorySourceRevisionMarksLegacyRowsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-memory-revision.sqlite")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);
CREATE TABLE novels (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '');
CREATE TABLE chapters (id TEXT PRIMARY KEY, novel_id TEXT NOT NULL, chapter_number INTEGER NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL, revision INTEGER NOT NULL, deleted_at TEXT);
CREATE TABLE chapter_memories (
    chapter_id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL,
    chapter_number INTEGER NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    characters TEXT NOT NULL DEFAULT '',
    new_settings TEXT NOT NULL DEFAULT '',
    foreshadowing TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
CREATE TABLE narrative_threads (
    id TEXT PRIMARY KEY,
    novel_id TEXT NOT NULL,
    name TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    subjects TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    opened_chapter_id TEXT NOT NULL,
    opened_chapter_number INTEGER NOT NULL,
    updated_chapter_id TEXT NOT NULL,
    updated_chapter_number INTEGER NOT NULL
);
INSERT INTO schema_migrations VALUES (1, 'legacy-columns', 'then');
INSERT INTO schema_migrations VALUES (2, 'chapter-memory', 'then');
INSERT INTO novels (id) VALUES ('n1');
INSERT INTO chapters VALUES ('c1', 'n1', 1, '第一章', '正文', 7, NULL);
INSERT INTO chapter_memories (chapter_id, novel_id, chapter_number, summary, updated_at) VALUES ('c1', 'n1', 1, '旧摘要', 'then');
INSERT INTO narrative_threads (id, novel_id, name, status, opened_chapter_id, opened_chapter_number, updated_chapter_id, updated_chapter_number) VALUES ('t1', 'n1', '旧伏笔', 'open', 'c1', 1, 'c1', 1);
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sourceRevision int
	if err := db.QueryRow(`SELECT source_revision FROM chapter_memories WHERE chapter_id = 'c1'`).Scan(&sourceRevision); err != nil {
		t.Fatal(err)
	}
	if sourceRevision != 0 {
		t.Fatalf("legacy chapter memory should remain stale until refreshed, got source revision %d", sourceRevision)
	}
	var threads int
	if err := db.QueryRow(`SELECT COUNT(*) FROM narrative_threads`).Scan(&threads); err != nil {
		t.Fatal(err)
	}
	if threads != 0 {
		t.Fatalf("legacy narrative projections should be discarded, got %d rows", threads)
	}
	var migrationName string
	if err := db.QueryRow(`SELECT name FROM schema_migrations WHERE version = 3`).Scan(&migrationName); err != nil {
		t.Fatal(err)
	}
	if migrationName != "chapter-memory-source-revision" {
		t.Fatalf("unexpected migration name %q", migrationName)
	}
}

func TestMigrateProposalDemoModeDefaultsLegacyProposalsToReal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-proposal-demo.sqlite")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);
CREATE TABLE proposals (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    novel_id TEXT NOT NULL,
    chapter_id TEXT NOT NULL,
    base_revision INTEGER NOT NULL,
    proposed_title TEXT NOT NULL,
    proposed_text TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    resolved_at TEXT
);
INSERT INTO schema_migrations VALUES (1, 'legacy-columns', 'then');
INSERT INTO schema_migrations VALUES (2, 'chapter-memory', 'then');
INSERT INTO schema_migrations VALUES (3, 'chapter-memory-source-revision', 'then');
INSERT INTO proposals (id, task_id, novel_id, chapter_id, base_revision, proposed_title, proposed_text, status, created_at)
VALUES ('p1', 'task1', 'novel1', 'chapter1', 1, '标题', '正文', 'pending', 'then');
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var demoMode int
	if err := db.QueryRow(`SELECT demo_mode FROM proposals WHERE id = 'p1'`).Scan(&demoMode); err != nil {
		t.Fatal(err)
	}
	if demoMode != 0 {
		t.Fatalf("legacy proposals should default to non-demo, got %d", demoMode)
	}
	var migrationName string
	if err := db.QueryRow(`SELECT name FROM schema_migrations WHERE version = 4`).Scan(&migrationName); err != nil {
		t.Fatal(err)
	}
	if migrationName != "proposal-demo-mode" {
		t.Fatalf("unexpected proposal migration name %q", migrationName)
	}
}
