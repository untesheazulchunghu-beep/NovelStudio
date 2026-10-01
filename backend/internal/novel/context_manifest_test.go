package novel

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

func TestGenerationReportsKnowledgeContextAndTruncation(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "context.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	userID := database.NewID()
	workspaceID := database.NewID()
	novelID := database.NewID()
	chapterID := database.NewID()
	now := database.Now()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`, []any{userID, "context@example.test", "hash", now}},
		{`INSERT INTO workspaces (id, name, owner_id, created_at) VALUES (?, ?, ?, ?)`, []any{workspaceID, "Test", userID, now}},
		{`INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)`, []any{workspaceID, userID, now}},
		{`INSERT INTO novels (id, workspace_id, title, summary, genre, writing_guide, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, []any{novelID, workspaceID, "上下文测试", "雾港故事", "悬疑", "保持克制", now, now}},
		{`INSERT INTO chapters (id, novel_id, chapter_number, title, goal, updated_at) VALUES (?, ?, 1, ?, ?, ?)`, []any{chapterID, novelID, "第一章", "抵达雾港", now}},
	} {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	nodeIDs := make([]string, 81)
	for index := range nodeIDs {
		nodeIDs[index] = database.NewID()
		if _, err := db.Exec(`INSERT INTO graph_nodes (id, novel_id, kind, label, summary, created_at) VALUES (?, ?, 'character', ?, ?, ?)`,
			nodeIDs[index], novelID, fmt.Sprintf("人物%03d", index), "角色设定", now); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 161; index++ {
		source := index % len(nodeIDs)
		target := (source + 1) % len(nodeIDs)
		if _, err := db.Exec(`INSERT INTO graph_edges (id, novel_id, source_id, target_id, relation, summary, created_at) VALUES (?, ?, ?, ?, ?, '', ?)`,
			database.NewID(), novelID, nodeIDs[source], nodeIDs[target], fmt.Sprintf("关系%d", index), now); err != nil {
			t.Fatal(err)
		}
	}

	manager, err := task.NewManager(db, testGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	item, failure := server.startGeneration(context.Background(), novelID, userID, generationInput{
		ChapterIDs: []string{chapterID}, Instruction: "生成本章",
	}, "batch", "", "")
	if failure != nil {
		t.Fatalf("startGeneration failed: %s", failure.message)
	}

	var requestJSON string
	if err := db.QueryRow(`SELECT request_json FROM tasks WHERE id = ?`, item.ID).Scan(&requestJSON); err != nil {
		t.Fatal(err)
	}
	var savedRequest map[string]any
	if err := json.Unmarshal([]byte(requestJSON), &savedRequest); err != nil {
		t.Fatal(err)
	}
	manifest, ok := savedRequest["contextManifest"].(map[string]any)
	if !ok || manifest["truncatedEntities"] != float64(1) || manifest["truncatedRelations"] != float64(1) {
		t.Fatalf("expected one omitted entity and relation, got %#v", savedRequest["contextManifest"])
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		events, err := manager.EventsAfter(item.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Type != "task.started" {
				continue
			}
			if event.Data["truncatedEntities"] != float64(1) || event.Data["truncatedRelations"] != float64(1) {
				t.Fatalf("task.started should expose truncation counts, got %#v", event.Data)
			}
			sources, ok := event.Data["contextSources"].([]any)
			if !ok || len(sources) != 243 {
				t.Fatalf("expected 243 included context sources, got %#v", event.Data["contextSources"])
			}
			savedSources, ok := manifest["sources"].([]any)
			if !ok || !reflect.DeepEqual(sources, savedSources) {
				t.Fatalf("event context sources should match the persisted request, event=%#v snapshot=%#v", sources, manifest["sources"])
			}
			if events[len(events)-1].Type == "task.completed" {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("generation task did not emit context details and complete")
}
