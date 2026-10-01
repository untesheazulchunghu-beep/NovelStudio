package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCreateBackupIncludesCommittedWALData(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.sqlite")
	source, err := sql.Open("sqlite", "file:"+filepath.ToSlash(sourcePath)+"?_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err := source.Exec(`CREATE TABLE notes (body TEXT NOT NULL);
		INSERT INTO notes (body) VALUES ('committed in wal');`); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "snapshot.sqlite")
	gotPath, err := CreateBackup(sourcePath, destination)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != destination {
		t.Fatalf("expected backup path %q, got %q", destination, gotPath)
	}

	snapshot, err := sql.Open("sqlite", "file:"+filepath.ToSlash(destination)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	var body string
	if err := snapshot.QueryRow(`SELECT body FROM notes`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body != "committed in wal" {
		t.Fatalf("backup lost committed data, got %q", body)
	}
}

func TestCreateBackupDoesNotOverwriteSourceOrExistingDestination(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.sqlite")
	source, err := sql.Open("sqlite", "file:"+filepath.ToSlash(sourcePath))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`CREATE TABLE notes (body TEXT NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := CreateBackup(sourcePath, sourcePath); err == nil {
		t.Fatal("expected source path to be rejected as the backup destination")
	}

	destination := filepath.Join(t.TempDir(), "existing.sqlite")
	original := []byte("do not overwrite")
	if err := os.WriteFile(destination, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateBackup(sourcePath, destination); err == nil {
		t.Fatal("expected an existing backup destination to be rejected")
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("existing destination was changed: %q", got)
	}
}

func TestCreateBackupChoosesTimestampedPath(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.sqlite")
	source, err := sql.Open("sqlite", "file:"+filepath.ToSlash(sourcePath))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`CREATE TABLE notes (body TEXT NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	source.Close()

	backupPath, err := CreateBackup(sourcePath, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("timestamped backup file was not created: %v", err)
	}
}
