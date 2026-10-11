package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewStoreMigratesLegacyWorkspaceAndKeepsJSONBackup(t *testing.T) {
	dataDir := t.TempDir()
	legacyPath := filepath.Join(dataDir, "workspace.json")
	legacy := Workspace{
		ID:       "legacy-workspace",
		Project:  Project{ID: 42, Title: "旧版工作区", Genre: "悬疑", WordTarget: 80000},
		Chapters: []Chapter{{ID: 43, Number: 1, Title: "旧章", Content: "旧正文", Status: "draft"}},
		Timeline: []TimelineEvent{{ID: 44, Title: "旧记录", TimeLabel: "第一天", Status: "planned", ChapterID: 43}},
		Settings: Settings{Provider: "local", Model: "local-creative", Temperature: 0.7},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("encode legacy workspace: %v", err)
	}
	if err := os.WriteFile(legacyPath, raw, 0o600); err != nil {
		t.Fatalf("write legacy workspace: %v", err)
	}

	store, err := NewStore(filepath.Join(dataDir, "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store and migrate legacy workspace: %v", err)
	}
	defer func() { _ = store.Close() }()

	workspace, exists := store.Snapshot("legacy-workspace")
	if !exists || workspace.Project.Title != "旧版工作区" || len(workspace.Chapters) != 1 {
		t.Fatalf("migrated workspace = exists:%t value:%+v", exists, workspace)
	}
	chapter, exists, err := store.GetChapter("legacy-workspace", 43)
	if err != nil || !exists || chapter.Content != "旧正文" {
		t.Fatalf("migrated chapter = exists:%t err:%v value:%+v", exists, err, chapter)
	}
	workspace, _ = store.Snapshot("legacy-workspace")
	if len(workspace.Timeline) != 1 || workspace.Timeline[0].ChapterID != 43 {
		t.Fatalf("migrated timeline = %+v", workspace.Timeline)
	}
	after, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read retained legacy workspace: %v", err)
	}
	if !bytes.Equal(after, raw) {
		t.Fatal("legacy workspace JSON was changed during migration")
	}
}
