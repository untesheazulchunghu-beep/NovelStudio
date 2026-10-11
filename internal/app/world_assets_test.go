package app

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportProjectBackupRemapsWorldAssetRelations(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	backup := validBackupForImportTest()
	backup.Chapters = []Chapter{{ID: 41, Title: "Chapter"}}
	backup.WorldAssets = []WorldAsset{
		{ID: 101, Name: "Harbor", Kind: "place", ChapterIDs: []int64{41}},
		{ID: 102, Name: "Guild", Kind: "organization", RelatedAssetIDs: []int64{101}},
	}
	imported, err := store.ImportProjectBackup(backup)
	if err != nil {
		t.Fatalf("import backup: %v", err)
	}
	workspace, exists := store.Snapshot(imported.WorkspaceID)
	if !exists || len(workspace.WorldAssets) != 2 {
		t.Fatalf("imported world assets = %+v", workspace.WorldAssets)
	}
	if len(workspace.WorldAssets[0].ChapterIDs) != 1 || workspace.WorldAssets[0].ChapterIDs[0] != workspace.Chapters[0].ID || len(workspace.WorldAssets[1].RelatedAssetIDs) != 1 || workspace.WorldAssets[1].RelatedAssetIDs[0] != workspace.WorldAssets[0].ID {
		t.Fatalf("remapped world asset links = %+v", workspace.WorldAssets)
	}
}

func TestImportProjectBackupRejectsOversizedWorldAsset(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	backup := validBackupForImportTest()
	backup.WorldAssets = []WorldAsset{{Name: "Harbor", Description: strings.Repeat("x", maxWorldAssetDescriptionLength+1)}}
	_, err = store.ImportProjectBackup(backup)
	if !errors.Is(err, ErrInvalidProjectBackup) {
		t.Fatalf("import error = %v, want invalid backup", err)
	}
}

func TestAPIWorldAssetLifecycle(t *testing.T) {
	f := newAPIFixture(t, "")
	chapter := decodeAPI[Chapter](t, f.expect(f.call(http.MethodPost, "/api/chapters", ChapterInput{Title: "Chapter"}), http.StatusCreated))
	place := decodeAPI[WorldAsset](t, f.expect(f.call(http.MethodPost, "/api/world-assets", WorldAssetInput{Name: "Harbor", Kind: "place", Description: "Foggy port", ChapterIDs: []int64{chapter.ID}}), http.StatusCreated))
	org := decodeAPI[WorldAsset](t, f.expect(f.call(http.MethodPost, "/api/world-assets", WorldAssetInput{Name: "Guild", Kind: "organization", RelatedAssetIDs: []int64{place.ID}}), http.StatusCreated))
	if org.Kind != "organization" || len(org.RelatedAssetIDs) != 1 || org.RelatedAssetIDs[0] != place.ID {
		t.Fatalf("created assets = %+v", []WorldAsset{place, org})
	}
	f.expect(f.call(http.MethodPost, "/api/world-assets", WorldAssetInput{Name: "Invalid", Kind: "planet"}), http.StatusBadRequest)
	updated := decodeAPI[WorldAsset](t, f.expect(f.call(http.MethodPut, "/api/world-assets/"+idPath(org.ID), WorldAssetInput{Name: "Guild", Kind: "organization", ChapterIDs: []int64{chapter.ID}, RelatedAssetIDs: []int64{place.ID}}), http.StatusOK))
	if len(updated.ChapterIDs) != 1 {
		t.Fatalf("updated asset chapters = %v", updated.ChapterIDs)
	}
	f.expect(f.call(http.MethodDelete, "/api/chapters/"+idPath(chapter.ID), nil), http.StatusOK)
	updated = decodeAPI[WorldAsset](t, f.expect(f.call(http.MethodGet, "/api/world-assets/"+idPath(org.ID), nil), http.StatusOK))
	if len(updated.ChapterIDs) != 0 {
		t.Fatalf("asset chapters after chapter deletion = %v", updated.ChapterIDs)
	}
	f.expect(f.call(http.MethodDelete, "/api/world-assets/"+idPath(place.ID), nil), http.StatusOK)
	updated = decodeAPI[WorldAsset](t, f.expect(f.call(http.MethodGet, "/api/world-assets/"+idPath(org.ID), nil), http.StatusOK))
	if len(updated.RelatedAssetIDs) != 0 {
		t.Fatalf("asset relations after target deletion = %v", updated.RelatedAssetIDs)
	}
	f.expect(f.call(http.MethodDelete, "/api/world-assets/"+idPath(org.ID), nil), http.StatusOK)
}
