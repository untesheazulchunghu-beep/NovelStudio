package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CreateBackup writes a consistent SQLite snapshot, including committed WAL
// data, without requiring the application to stop.
func CreateBackup(databasePath, outputPath string) (string, error) {
	source, err := filepath.Abs(databasePath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(source); err != nil {
		return "", fmt.Errorf("inspect source database: %w", err)
	}

	if outputPath == "" {
		outputPath = fmt.Sprintf("%s.backup-%s.sqlite", source, time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	destination, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	samePath := filepath.Clean(source) == filepath.Clean(destination)
	if runtime.GOOS == "windows" {
		samePath = strings.EqualFold(filepath.Clean(source), filepath.Clean(destination))
	}
	if samePath {
		return "", fmt.Errorf("backup destination must differ from the source database")
	}
	if _, err := os.Stat(destination); err == nil {
		return "", fmt.Errorf("backup destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect backup destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}

	dsn := "file:" + filepath.ToSlash(source) + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return "", err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		return "", fmt.Errorf("open source database: %w", err)
	}
	if _, err := db.Exec(`VACUUM INTO ?`, destination); err != nil {
		return "", fmt.Errorf("create sqlite backup: %w", err)
	}
	return destination, nil
}
