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
