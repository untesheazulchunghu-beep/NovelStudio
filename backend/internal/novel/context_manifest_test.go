package novel

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
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
		{`INSERT INTO chapters (id, novel_id, chapter_number, title, goal, updated_at) VALUES (?, ?, 1, ?, ?, ?)`, []any{chapterID, novelID, "第一章", "鹤归抵达雾隐港", now}},
	} {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	nodeIDs := make([]string, 80)
	for index := range nodeIDs {
		nodeIDs[index] = database.NewID()
		if _, err := db.Exec(`INSERT INTO graph_nodes (id, novel_id, kind, label, summary, created_at) VALUES (?, ?, 'character', ?, ?, ?)`,
			nodeIDs[index], novelID, fmt.Sprintf("人物%03d", index), "角色设定", now); err != nil {
			t.Fatal(err)
		}
	}
	heroID := database.NewID()
	placeID := database.NewID()
	if _, err := db.Exec(`INSERT INTO graph_nodes (id, novel_id, kind, label, summary, created_at) VALUES (?, ?, 'character', '鹤归', '章外人物', ?)`, heroID, novelID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO graph_nodes (id, novel_id, kind, label, summary, created_at) VALUES (?, ?, 'place', '雾隐港', '章外地点', ?)`, placeID, novelID, now); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 160; index++ {
		source := index % len(nodeIDs)
		target := (source + 1) % len(nodeIDs)
		if _, err := db.Exec(`INSERT INTO graph_edges (id, novel_id, source_id, target_id, relation, summary, created_at) VALUES (?, ?, ?, ?, '旧前缀关系', '', ?)`,
			database.NewID(), novelID, nodeIDs[source], nodeIDs[target], fmt.Sprintf("t%04d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO graph_edges (id, novel_id, source_id, target_id, relation, summary, created_at) VALUES (?, ?, ?, ?, '驻留', '', ?)`,
		database.NewID(), novelID, heroID, placeID, "t9999"); err != nil {
		t.Fatal(err)
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
	if !ok {
		t.Fatalf("missing context manifest: %#v", savedRequest["contextManifest"])
	}
	if truncationCount(manifest["truncatedEntities"]) != 0 || truncationCount(manifest["truncatedRelations"]) != 0 {
		t.Fatalf("relevant facts fit the budget, truncation should be empty, got %#v", manifest)
	}
	sources, _ := manifest["sources"].([]any)
	dropped, _ := manifest["dropped"].([]any)
	if !manifestNames(sources, "人物：鹤归") || !manifestNames(sources, "地点：雾隐港") || !manifestNames(sources, "关系：鹤归 —驻留→ 雾隐港") {
		t.Fatalf("blueprint entities outside the old prefix should be included, got %#v", sources)
	}
	if manifestNames(sources, "人物：人物000") || manifestNames(sources, "旧前缀关系") {
		t.Fatalf("unrelated prefix facts should be omitted, got %#v", sources)
	}
	if !manifestNames(dropped, "人物：人物000") || !manifestNames(dropped, "旧前缀关系") {
		t.Fatalf("omitted prefix facts should be named, got %#v", dropped)
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
			if truncationCount(event.Data["truncatedEntities"]) != 0 || truncationCount(event.Data["truncatedRelations"]) != 0 {
				t.Fatalf("task.started should not report budget truncation when every relevant fact fits, got %#v", event.Data)
			}
			eventSources, ok := event.Data["contextSources"].([]any)
			if !ok || !reflect.DeepEqual(eventSources, sources) {
				t.Fatalf("event context sources should match the persisted request, event=%#v snapshot=%#v", eventSources, sources)
			}
			eventDropped, _ := event.Data["droppedContext"].([]any)
			if !reflect.DeepEqual(eventDropped, dropped) {
				t.Fatalf("event dropped context should match the persisted request, event=%#v snapshot=%#v", eventDropped, dropped)
			}
			if events[len(events)-1].Type == "task.completed" {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("generation task did not emit context details and complete")
}

func manifestNames(items []any, needle string) bool {
	for _, item := range items {
		text, ok := item.(string)
		if ok && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func truncationCount(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}
