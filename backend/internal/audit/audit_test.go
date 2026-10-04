package audit

import (
	"context"
	"path/filepath"
	"testing"

	"novelstudio/internal/database"
)

func TestAuditLog(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create test user
	userID := database.NewID()
	_, err = db.Exec(`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, datetime('now'))`,
		userID, "test@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create test workspace
	workspaceID := database.NewID()
	_, err = db.Exec(`INSERT INTO workspaces (id, name, owner_id, created_at) VALUES (?, ?, ?, datetime('now'))`,
		workspaceID, "Test Workspace", userID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// Create test novel
	novelID := database.NewID()
	_, err = db.Exec(`INSERT INTO novels (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		novelID, workspaceID, "Test Novel")
	if err != nil {
		t.Fatalf("create novel: %v", err)
	}

	ctx := context.Background()

	// Test logging an action
	chapterID := database.NewID()
	metadata := map[string]interface{}{
		"chapter_title": "Chapter 1",
		"word_count":    1500,
	}

	err = Log(ctx, db, userID, &novelID, ActionDeleteChapter, TargetTypeChapter, chapterID, metadata)
	if err != nil {
		t.Fatalf("log audit entry: %v", err)
	}

	// Query back the entry
	entries, err := Query(ctx, db, &userID, nil, nil, 10, 0)
	if err != nil {
		t.Fatalf("query audit entries: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.UserID != userID {
		t.Errorf("expected user_id %s, got %s", userID, entry.UserID)
	}
	if entry.NovelID == nil || *entry.NovelID != novelID {
		t.Errorf("expected novel_id %s, got %v", novelID, entry.NovelID)
	}
	if entry.Action != ActionDeleteChapter {
		t.Errorf("expected action %s, got %s", ActionDeleteChapter, entry.Action)
	}
	if entry.TargetType != TargetTypeChapter {
		t.Errorf("expected target_type %s, got %s", TargetTypeChapter, entry.TargetType)
	}
	if entry.TargetID != chapterID {
		t.Errorf("expected target_id %s, got %s", chapterID, entry.TargetID)
	}
	if entry.Metadata["chapter_title"] != "Chapter 1" {
		t.Errorf("expected metadata chapter_title 'Chapter 1', got %v", entry.Metadata["chapter_title"])
	}
	if entry.Metadata["word_count"] != float64(1500) { // JSON numbers are float64
		t.Errorf("expected metadata word_count 1500, got %v", entry.Metadata["word_count"])
	}
}

func TestAuditLogFilters(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// Create test users
	user1ID := database.NewID()
	user2ID := database.NewID()
	for _, uid := range []string{user1ID, user2ID} {
		_, err = db.Exec(`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, datetime('now'))`,
			uid, uid+"@example.com", "hash")
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
	}

	// Create test workspace
	workspaceID := database.NewID()
	_, err = db.Exec(`INSERT INTO workspaces (id, name, owner_id, created_at) VALUES (?, ?, ?, datetime('now'))`,
		workspaceID, "Test Workspace", user1ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// Create test novel
	novelID := database.NewID()
	_, err = db.Exec(`INSERT INTO novels (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		novelID, workspaceID, "Test Novel")
	if err != nil {
		t.Fatalf("create novel: %v", err)
	}

	ctx := context.Background()

	// Log multiple actions
	actions := []struct {
		userID string
		action Action
	}{
		{user1ID, ActionApplyProposal},
		{user1ID, ActionDeleteChapter},
		{user2ID, ActionApplyProposal},
	}

	for _, a := range actions {
		err = Log(ctx, db, a.userID, &novelID, a.action, TargetTypeChapter, database.NewID(), nil)
		if err != nil {
			t.Fatalf("log audit entry: %v", err)
		}
	}

	// Query by user
	entries, err := Query(ctx, db, &user1ID, nil, nil, 10, 0)
	if err != nil {
		t.Fatalf("query by user: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for user1, got %d", len(entries))
	}

	// Query by action
	applyAction := ActionApplyProposal
	entries, err = Query(ctx, db, nil, nil, &applyAction, 10, 0)
	if err != nil {
		t.Fatalf("query by action: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 apply_proposal entries, got %d", len(entries))
	}

	// Query by novel
	entries, err = Query(ctx, db, nil, &novelID, nil, 10, 0)
	if err != nil {
		t.Fatalf("query by novel: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries for novel, got %d", len(entries))
	}
}
