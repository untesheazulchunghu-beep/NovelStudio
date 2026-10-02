package novel

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"novelstudio/internal/auth"
	"novelstudio/internal/database"
	"novelstudio/internal/task"
)

func TestNovelAndChapterCanBeSoftDeletedAndRestored(t *testing.T) {
	db, server := newTrashServer(t)
	client := newClient(t, server)
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "trash-owner@example.test", "password": "safe-trash-password",
	}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	createdNovel := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "回收站测试", "plannedChapters": 2,
	}, http.StatusCreated)
	novelID := createdNovel["id"].(string)
	createdChapter := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/chapters", map[string]string{
		"title": "第二章",
	}, http.StatusCreated)
	chapterID := createdChapter["id"].(string)
	storeTestChapterMemory(t, db, novelID, createdChapter, "+灯塔|甲|灯塔仍未点亮")

	requestJSON(t, client, http.MethodDelete, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID, nil, http.StatusOK)
	threads, err := task.LoadNarrativeThreads(context.Background(), db, novelID)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 0 {
		t.Fatalf("deleting a chapter should remove its foreshadowing from the active projection, got %#v", threads)
	}
	activeChapters := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)
	if len(activeChapters) != 1 {
		t.Fatalf("deleted chapter should not appear in the active list, got %d chapters", len(activeChapters))
	}
	deletedChapters := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/chapters/trash", nil, http.StatusOK)["chapters"].([]any)
	if len(deletedChapters) != 1 || deletedChapters[0].(map[string]any)["id"] != chapterID {
		t.Fatalf("deleted chapter should be recoverable, got %#v", deletedChapters)
	}
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID+"/restore", map[string]any{}, http.StatusOK)
	threads, err = task.LoadNarrativeThreads(context.Background(), db, novelID)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].Name != "灯塔" || threads[0].Status != "open" {
		t.Fatalf("restoring a chapter should restore its current foreshadowing, got %#v", threads)
	}
	activeChapters = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)
	if len(activeChapters) != 2 {
		t.Fatalf("restored chapter should return to the active list, got %d chapters", len(activeChapters))
	}

	requestJSON(t, client, http.MethodDelete, server.URL+"/api/v1/novels/"+novelID, nil, http.StatusOK)
	activeNovels := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels", nil, http.StatusOK)["novels"].([]any)
	if len(activeNovels) != 0 {
		t.Fatalf("deleted novel should not appear in the active list, got %#v", activeNovels)
	}
	requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID, nil, http.StatusNotFound)
	deletedNovels := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/trash", nil, http.StatusOK)["novels"].([]any)
	if len(deletedNovels) != 1 || deletedNovels[0].(map[string]any)["id"] != novelID {
		t.Fatalf("deleted novel should be recoverable, got %#v", deletedNovels)
	}
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/restore", map[string]any{}, http.StatusOK)
	activeNovels = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels", nil, http.StatusOK)["novels"].([]any)
	if len(activeNovels) != 1 || activeNovels[0].(map[string]any)["id"] != novelID {
		t.Fatalf("restored novel should return to the active list, got %#v", activeNovels)
	}
}

func TestViewerCannotDeleteOrRestoreNovelContent(t *testing.T) {
	server := newInfraServer(t, "", nil)
	owner := newClient(t, server)
	requestJSON(t, owner, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "trash-writer@example.test", "password": "safe-trash-password",
	}, http.StatusCreated)
	profile := requestJSON(t, owner, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	createdNovel := requestJSON(t, owner, http.MethodPost, server.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "权限测试",
	}, http.StatusCreated)
	novelID := createdNovel["id"].(string)
	createdChapter := requestJSON(t, owner, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/chapters", map[string]string{
		"title": "第二章",
	}, http.StatusCreated)
	chapterID := createdChapter["id"].(string)

	viewer := newClient(t, server)
	requestJSON(t, viewer, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "trash-viewer@example.test", "password": "safe-trash-password",
	}, http.StatusCreated)
	requestJSON(t, owner, http.MethodPost, server.URL+"/api/v1/workspaces/"+workspaceID+"/members", map[string]string{
		"email": "trash-viewer@example.test", "role": "viewer",
	}, http.StatusCreated)
	requestJSON(t, viewer, http.MethodDelete, server.URL+"/api/v1/novels/"+novelID, nil, http.StatusForbidden)
	requestJSON(t, viewer, http.MethodDelete, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID, nil, http.StatusForbidden)
	requestJSON(t, owner, http.MethodDelete, server.URL+"/api/v1/novels/"+novelID, nil, http.StatusOK)
	requestJSON(t, viewer, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/restore", map[string]any{}, http.StatusForbidden)
	requestJSON(t, owner, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/restore", map[string]any{}, http.StatusOK)
	requestJSON(t, owner, http.MethodDelete, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID, nil, http.StatusOK)
	requestJSON(t, viewer, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID+"/restore", map[string]any{}, http.StatusForbidden)
	requestJSON(t, owner, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID+"/restore", map[string]any{}, http.StatusOK)
}

func TestChapterReorderPreservesDeletedSlotsAndReferences(t *testing.T) {
	db, server := newTrashServer(t)
	client := newClient(t, server)
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "reorder-owner@example.test", "password": "safe-reorder-password",
	}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	createdNovel := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "章节排序测试", "plannedChapters": 3,
	}, http.StatusCreated)
	novelID := createdNovel["id"].(string)
	firstChapter := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)[0].(map[string]any)
	firstChapterID := firstChapter["id"].(string)
	firstChapter = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+firstChapterID, map[string]any{
		"expectedRevision": 1, "title": "第一章", "body": "保留版本的正文", "goal": "", "keyEvents": "", "suspenseHook": "",
	}, http.StatusOK)
	secondChapter := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/chapters", map[string]string{"title": "第二章"}, http.StatusCreated)
	thirdChapter := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/chapters", map[string]string{"title": "第三章"}, http.StatusCreated)
	secondChapterID := secondChapter["id"].(string)
	thirdChapterID := thirdChapter["id"].(string)
	storeTestChapterMemory(t, db, novelID, firstChapter, "+第一章伏笔|甲|初始")
	storeTestChapterMemory(t, db, novelID, thirdChapter, "+第三章伏笔|乙|初始")
	requestJSON(t, client, http.MethodDelete, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+secondChapterID, nil, http.StatusOK)

	taskResponse := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{
		"chapterIds": []string{firstChapterID}, "instruction": "为第一章生成一个提案。",
	}, http.StatusAccepted)
	waitForTaskStatus(t, client, server.URL, taskResponse["id"].(string), "completed")
	proposal := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/proposals", nil, http.StatusOK)["proposals"].([]any)[0].(map[string]any)

	reordered := requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/novels/"+novelID+"/chapters/order", map[string][]string{
		"chapterIds": {thirdChapterID, firstChapterID},
	}, http.StatusOK)["chapters"].([]any)
	if len(reordered) != 2 {
		t.Fatalf("expected both active chapters after reorder, got %#v", reordered)
	}
	firstOrdered := reordered[0].(map[string]any)
	secondOrdered := reordered[1].(map[string]any)
	if firstOrdered["id"] != thirdChapterID || firstOrdered["number"] != float64(1) ||
		secondOrdered["id"] != firstChapterID || secondOrdered["number"] != float64(3) {
		t.Fatalf("reorder should use the active chapter number slots around the deleted chapter, got %#v", reordered)
	}
	threads, err := task.LoadNarrativeThreads(context.Background(), db, novelID)
	if err != nil {
		t.Fatal(err)
	}
	threadNumbers := make(map[string]int)
	for _, thread := range threads {
		threadNumbers[thread.Name] = thread.OpenedChapterNumber
	}
	if threadNumbers["第一章伏笔"] != 3 || threadNumbers["第三章伏笔"] != 1 {
		t.Fatalf("reordering chapters should update foreshadowing chapter numbers, got %#v", threadNumbers)
	}
	revisions := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+firstChapterID+"/revisions", nil, http.StatusOK)["revisions"].([]any)
	if len(revisions) != 1 {
		t.Fatalf("reordering should preserve chapter revision history, got %#v", revisions)
	}
	proposals := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/proposals", nil, http.StatusOK)["proposals"].([]any)
	if len(proposals) != 1 || proposals[0].(map[string]any)["chapterId"] != firstChapterID || proposal["chapterId"] != firstChapterID {
		t.Fatalf("reordering should keep the proposal attached to its chapter ID, got %#v", proposals)
	}
}

func newTrashServer(t *testing.T) (*sql.DB, *httptest.Server) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "trash.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	manager, err := task.NewManager(db, testGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)
	return db, server
}

func storeTestChapterMemory(t *testing.T, db *sql.DB, novelID string, chapter map[string]any, foreshadowing string) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err = task.SaveChapterMemory(context.Background(), tx, novelID, chapter["id"].(string),
		int(chapter["number"].(float64)), int(chapter["revision"].(float64)), task.ChapterMemory{
			Summary: "章节摘要", Foreshadowing: foreshadowing,
		})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestStaleProposalRegenerationKeepsInstructionAndUsesLatestChapter(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "proposal-regeneration.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := task.NewManager(db, testGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	application := NewServer(db, auth.NewSessions([]byte("test-session-secret-that-is-long-enough"), false), manager, nil, "")
	server := newTLSServer(t, application.Handler())
	defer server.Close()
	client := newClient(t, server)
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "stale-proposal@example.test", "password": "safe-stale-password",
	}, http.StatusCreated)
	profile := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/me", nil, http.StatusOK)
	workspaceID := profile["workspaces"].([]any)[0].(map[string]any)["id"].(string)
	createdNovel := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels", map[string]any{
		"workspaceId": workspaceID, "title": "过期提案测试",
	}, http.StatusCreated)
	novelID := createdNovel["id"].(string)
	chapter := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)[0].(map[string]any)
	chapterID := chapter["id"].(string)
	instruction := "沿用原任务说明，补写渡口相遇。"
	originalTask := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/tasks/batch", map[string]any{
		"chapterIds": []string{chapterID}, "instruction": instruction,
	}, http.StatusAccepted)
	waitForTaskStatus(t, client, server.URL, originalTask["id"].(string), "completed")
	proposal := requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/proposals", nil, http.StatusOK)["proposals"].([]any)[0].(map[string]any)

	chapter = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/novels/"+novelID+"/chapters", nil, http.StatusOK)["chapters"].([]any)[0].(map[string]any)
	requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/novels/"+novelID+"/chapters/"+chapterID, map[string]any{
		"expectedRevision": chapter["revision"], "expectedUpdatedAt": chapter["updatedAt"],
		"title": chapter["title"], "body": "编辑者刚刚修改的正文",
		"goal": chapter["goal"], "keyEvents": chapter["keyEvents"], "suspenseHook": chapter["suspenseHook"],
	}, http.StatusOK)

	reviewer := newClient(t, server)
	requestJSON(t, reviewer, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]string{
		"email": "stale-reviewer@example.test", "password": "safe-stale-password",
	}, http.StatusCreated)
	requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/workspaces/"+workspaceID+"/members", map[string]string{
		"email": "stale-reviewer@example.test", "role": "reviewer",
	}, http.StatusCreated)
	requestJSON(t, reviewer, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/proposals/"+proposal["id"].(string)+"/regenerate", map[string]any{}, http.StatusForbidden)

	retryTask := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/novels/"+novelID+"/proposals/"+proposal["id"].(string)+"/regenerate", map[string]any{}, http.StatusAccepted)
	retryTaskID := retryTask["id"].(string)
	var requestSnapshot string
	if err := db.QueryRow(`SELECT request_json FROM tasks WHERE id = ?`, retryTaskID).Scan(&requestSnapshot); err != nil {
		t.Fatal(err)
	}
	var savedRequest task.Request
	if err := json.Unmarshal([]byte(requestSnapshot), &savedRequest); err != nil {
		t.Fatal(err)
	}
	if savedRequest.Instruction != instruction || len(savedRequest.Chapters) != 1 ||
		savedRequest.Chapters[0].ID != chapterID || savedRequest.Chapters[0].Revision != 2 ||
		savedRequest.Chapters[0].Body != "编辑者刚刚修改的正文" {
		t.Fatalf("regeneration should preserve the task instruction and use the current chapter snapshot: %#v", savedRequest)
	}
	waitForTaskStatus(t, client, server.URL, retryTaskID, "completed")
}
