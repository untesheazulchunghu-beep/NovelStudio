package app

import (
	"database/sql"
	"errors"
	"fmt"
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

const sqliteSchemaVersion = 9

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
	protectedDatabasePath, databaseExisted, err := existingSQLiteDatabase(databasePath)
	if err != nil {
		return nil, "", err
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
	if databaseExisted {
		if err := backupSQLiteBeforeMigration(db, protectedDatabasePath); err != nil {
			_ = db.Close()
			return nil, "", err
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

func existingSQLiteDatabase(databasePath string) (string, bool, error) {
	protectedPath, shouldProtect := sqliteProtectionPath(databasePath)
	if !shouldProtect {
		return "", false, nil
	}
	info, err := os.Stat(protectedPath)
	if errors.Is(err, os.ErrNotExist) {
		return protectedPath, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect SQLite database %s: %w", protectedPath, err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return protectedPath, false, nil
	}
	return protectedPath, true, nil
}

func backupSQLiteBeforeMigration(db *sql.DB, databasePath string) error {
	var currentVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&currentVersion); err != nil {
		return fmt.Errorf("read SQLite schema version before backup: %w", err)
	}
	if currentVersion >= sqliteSchemaVersion {
		return nil
	}
	backupPath := fmt.Sprintf("%s.pre-migration-v%d.bak", databasePath, currentVersion)
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect SQLite migration backup %s: %w", backupPath, err)
	}
	temporaryPath := fmt.Sprintf("%s.tmp-%d", backupPath, time.Now().UnixNano())
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := db.Exec("VACUUM INTO ?", temporaryPath); err != nil {
		return fmt.Errorf("create SQLite migration backup %s: %w", backupPath, err)
	}
	if err := os.Rename(temporaryPath, backupPath); err != nil {
		return fmt.Errorf("store SQLite migration backup %s: %w", backupPath, err)
	}
	if err := protectLocalFile(backupPath, "SQLite migration backup"); err != nil {
		return err
	}
	return nil
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
