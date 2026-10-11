package app

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func validBackupForImportTest() ProjectBackup {
	return ProjectBackup{
		Format:  projectBackupFormat,
		Version: projectBackupVersion,
		Project: Project{Title: "导入测试"},
	}
}

func TestImportProjectBackupAcceptsValidBackup(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	backup := validBackupForImportTest()
	backup.Chapters = []Chapter{{ID: 41, Title: "第一章", Summary: "目标", Content: "正文"}}
	backup.Timeline = []TimelineEvent{{Title: "确认线索", TimeLabel: "第三天", Description: "主角确认信件来源。", Status: "active", ChapterID: 41}}
	imported, err := store.ImportProjectBackup(backup)
	if err != nil {
		t.Fatalf("import valid backup: %v", err)
	}
	workspace, exists := store.Snapshot(imported.WorkspaceID)
	if !exists || len(workspace.Timeline) != 1 || workspace.Timeline[0].ChapterID != workspace.Chapters[0].ID {
		t.Fatalf("imported timeline = %+v, chapters = %+v", workspace.Timeline, workspace.Chapters)
	}
}

func TestProjectBackupSanitizesModelCredentials(t *testing.T) {
	backup := projectBackupFor(Workspace{
		Project:  Project{Title: "凭据清理"},
		Settings: Settings{Provider: "compatible", BaseURL: "https://writer:secret@example.com/v1?token=abc&stream=true&api_key=xyz#private", APIKey: "sk-local-secret"},
	})

	if backup.Settings.BaseURL != "https://example.com/v1?stream=true" {
		t.Fatalf("backup base URL = %q, want credentials removed", backup.Settings.BaseURL)
	}
	encoded, err := json.Marshal(backup)
	if err != nil {
		t.Fatalf("encode backup: %v", err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "abc") || strings.Contains(string(encoded), "xyz") {
		t.Fatalf("backup still contains model credentials: %s", encoded)
	}
}

// Backups are untrusted JSON. Import must enforce the same text limits as the
// create and update paths, otherwise a restored project can hold values the
// editor itself refuses to save.
func TestImportProjectBackupRejectsOversizedText(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	cases := map[string]func(*ProjectBackup){
		"project title": func(backup *ProjectBackup) {
			backup.Project.Title = strings.Repeat("题", maxProjectTitleLength+1)
		},
		"project premise": func(backup *ProjectBackup) {
			backup.Project.Premise = strings.Repeat("设", maxProjectPremiseLength+1)
		},
		"chapter title": func(backup *ProjectBackup) {
			backup.Chapters = []Chapter{{Title: strings.Repeat("章", maxChapterTitleLength+1)}}
		},
		"chapter summary": func(backup *ProjectBackup) {
			backup.Chapters = []Chapter{{Title: "第一章", Summary: strings.Repeat("摘", maxChapterSummaryLength+1)}}
		},
		"character name": func(backup *ProjectBackup) {
			backup.Characters = []Character{{Name: strings.Repeat("名", maxCharacterNameLength+1)}}
		},
		"character description": func(backup *ProjectBackup) {
			backup.Characters = []Character{{Name: "林澈", Description: strings.Repeat("述", maxCharacterDescriptionLength+1)}}
		},
		"note body": func(backup *ProjectBackup) {
			backup.Notes = []Note{{Title: "伏笔", Body: strings.Repeat("注", maxNoteBodyLength+1)}}
		},
		"timeline description": func(backup *ProjectBackup) {
			backup.Timeline = []TimelineEvent{{Title: "事件", Description: strings.Repeat("时", maxTimelineDescriptionLength+1)}}
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			backup := validBackupForImportTest()
			mutate(&backup)
			_, err := store.ImportProjectBackup(backup)
			if !errors.Is(err, ErrInvalidProjectBackup) {
				t.Fatalf("import error = %v, want ErrInvalidProjectBackup", err)
			}
		})
	}
}
