package app

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestNewStoreSupportsSQLiteMemoryPath(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	defer func() { _ = store.Close() }()
	if store.DatabasePath() != ":memory:" {
		t.Fatalf("database path = %q, want :memory:", store.DatabasePath())
	}
	if _, exists := store.Snapshot(localWorkspaceID); !exists {
		t.Fatal("in-memory store did not initialize its default workspace")
	}
}

func TestConcurrentProjectDeletionKeepsLastProject(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "novelstudio.db")
	firstStore, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	defer func() { _ = firstStore.Close() }()
	created, err := firstStore.CreateProject(ProjectInput{Title: "Second novel"})
	if err != nil {
		t.Fatalf("create second project: %v", err)
	}
	secondStore, err := NewStore(databasePath)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	defer func() { _ = secondStore.Close() }()
	current, exists := firstStore.Snapshot(localWorkspaceID)
	if !exists {
		t.Fatal("default workspace disappeared before project update")
	}
	if _, err := firstStore.UpdateProject(localWorkspaceID, ProjectInput{Title: "Updated by first Store", Subtitle: current.Project.Subtitle, Genre: current.Project.Genre, Premise: current.Project.Premise, WordTarget: current.Project.WordTarget}); err != nil {
		t.Fatalf("update project through first Store: %v", err)
	}
	refreshed, exists := secondStore.Snapshot(localWorkspaceID)
	if !exists || refreshed.Project.Title != "Updated by first Store" {
		t.Fatalf("second Store did not refresh project metadata: %+v", refreshed.Project)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; results <- firstStore.DeleteProject(localWorkspaceID) }()
	go func() { <-start; results <- secondStore.DeleteProject(created.WorkspaceID) }()
	close(start)
	firstErr := <-results
	secondErr := <-results
	if (firstErr == nil) == (secondErr == nil) {
		t.Fatalf("deletion results = (%v, %v), want exactly one successful deletion", firstErr, secondErr)
	}
	loserErr := firstErr
	if loserErr == nil {
		loserErr = secondErr
	}
	if !errors.Is(loserErr, ErrLastProject) {
		t.Fatalf("remaining deletion error = %v, want ErrLastProject", loserErr)
	}
	var projectCount int
	if err := firstStore.db.QueryRow("SELECT COUNT(*) FROM workspaces").Scan(&projectCount); err != nil {
		t.Fatalf("count remaining projects: %v", err)
	}
	if projectCount != 1 {
		t.Fatalf("database has %d projects after concurrent deletion, want exactly 1", projectCount)
	}
	var remainingWorkspaceID string
	if err := firstStore.db.QueryRow("SELECT workspace_id FROM workspaces").Scan(&remainingWorkspaceID); err != nil {
		t.Fatalf("read remaining project ID: %v", err)
	}
	deletedWorkspaceID := localWorkspaceID
	if remainingWorkspaceID == localWorkspaceID {
		deletedWorkspaceID = created.WorkspaceID
	}
	for index, store := range []*Store{firstStore, secondStore} {
		if _, exists := store.Snapshot(deletedWorkspaceID); exists {
			t.Fatalf("Store %d retained deleted workspace %q in its snapshot", index, deletedWorkspaceID)
		}
		if projects := store.ListProjects(); len(projects) != 1 {
			t.Fatalf("Store %d lists %d projects after external deletion, want 1", index, len(projects))
		}
	}
}
