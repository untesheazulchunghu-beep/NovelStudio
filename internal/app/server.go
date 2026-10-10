package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const maxJSONRequestBytes int64 = 16 << 20
const maxSmallJSONRequestBytes int64 = 256 << 10
const maxProjectBackupRequestBytes int64 = 256 << 20
const maxAgentRequestBytes int64 = 1 << 20
const maxModelResponseBytes int64 = 4 << 20
const maxAgentInputRunes = 8000
const maxAgentSelectionRunes = 1800

var ErrInvalidModelSettings = errors.New("invalid model settings")
var ErrEmptyJSONBody = errors.New("request body is empty")
var ErrRequestBodyTooLarge = errors.New("request body too large")

type chatRequestKey struct {
	workspaceID string
	requestID   string
}

type Server struct {
	store            *Store
	webDir           string
	client           *http.Client
	chatRequestLocks *chatRequestLockManager
}

func NewServer(store *Store, webDir string) *Server {
	requestLocks := newChatRequestLockManager()
	if store != nil && store.chatRequestLocks != nil {
		requestLocks = store.chatRequestLocks
	}
	return &Server{store: store, webDir: webDir, chatRequestLocks: requestLocks, client: &http.Client{
		Timeout:       90 * time.Second,
		CheckRedirect: checkModelRedirect,
	}}
}

type chatRequestLockManager struct {
	mu       sync.Mutex
	requests map[chatRequestKey]chan struct{}
	poolKey  string
}

type chatRequestLockPoolEntry struct {
	manager *chatRequestLockManager
	refs    int
}

var chatRequestLockPool = struct {
	sync.Mutex
	entries map[string]*chatRequestLockPoolEntry
}{entries: make(map[string]*chatRequestLockPoolEntry)}

func newChatRequestLockManager() *chatRequestLockManager {
	return &chatRequestLockManager{requests: make(map[chatRequestKey]chan struct{})}
}

func pooledChatRequestLockManager(databasePath string) (*chatRequestLockManager, func()) {
	if databasePath == ":memory:" {
		return newChatRequestLockManager(), func() {}
	}
	key := databasePath
	if !strings.HasPrefix(databasePath, "file:") {
		key = filepath.Clean(databasePath)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
	}
	chatRequestLockPool.Lock()
	entry := chatRequestLockPool.entries[key]
	if entry == nil {
		manager := newChatRequestLockManager()
		manager.poolKey = key
		entry = &chatRequestLockPoolEntry{manager: manager}
		chatRequestLockPool.entries[key] = entry
	}
	entry.refs++
	manager := entry.manager
	chatRequestLockPool.Unlock()
	var once sync.Once
	return manager, func() {
		once.Do(func() { releasePooledChatRequestLockManager(key, manager) })
	}
}

func releasePooledChatRequestLockManager(key string, manager *chatRequestLockManager) {
	if key == "" {
		return
	}
	chatRequestLockPool.Lock()
	if entry := chatRequestLockPool.entries[key]; entry != nil && entry.manager == manager {
		entry.refs--
		if entry.refs <= 0 {
			delete(chatRequestLockPool.entries, key)
		}
	}
	chatRequestLockPool.Unlock()
}

func (m *chatRequestLockManager) retain() func() {
	if m == nil || m.poolKey == "" {
		return func() {}
	}
	chatRequestLockPool.Lock()
	if entry := chatRequestLockPool.entries[m.poolKey]; entry != nil && entry.manager == m {
		entry.refs++
	}
	chatRequestLockPool.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() { releasePooledChatRequestLockManager(m.poolKey, m) })
	}
}

func (s *Server) acquireChatRequest(ctx context.Context, workspaceID, requestID string) (func(), error) {
	key := chatRequestKey{workspaceID: workspaceID, requestID: requestID}
	locks := s.chatRequestLocks
	if locks == nil {
		locks = newChatRequestLockManager()
		s.chatRequestLocks = locks
	}
	releasePoolRef := locks.retain()
	for {
		if err := ctx.Err(); err != nil {
			releasePoolRef()
			return nil, err
		}
		locks.mu.Lock()
		if locks.requests == nil {
			locks.requests = make(map[chatRequestKey]chan struct{})
		}
		if inFlight, exists := locks.requests[key]; exists {
			locks.mu.Unlock()
			select {
			case <-inFlight:
				continue
			case <-ctx.Done():
				releasePoolRef()
				return nil, ctx.Err()
			}
		}
		done := make(chan struct{})
		locks.requests[key] = done
		locks.mu.Unlock()
		var once sync.Once
		return func() {
			once.Do(func() {
				locks.mu.Lock()
				if locks.requests[key] == done {
					delete(locks.requests, key)
				}
				close(done)
				locks.mu.Unlock()
				releasePoolRef()
			})
		}, nil
	}
}

func checkModelRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}

	previousURL := via[len(via)-1].URL
	if !strings.EqualFold(request.URL.Host, previousURL.Host) ||
		(strings.EqualFold(previousURL.Scheme, "https") && !strings.EqualFold(request.URL.Scheme, "https")) {
		return http.ErrUseLastResponse
	}
	return nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", s.handleAPI)
	mux.HandleFunc("/", s.handleFrontend)
	return mux
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	if path == "" {
		writeJSON(w, http.StatusOK, map[string]any{"name": "NovelStudio API", "version": "0.1.0"})
		return
	}
	if path == "health" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": now()})
		return
	}
	parts := strings.Split(path, "/")
	switch parts[0] {
	case "projects":
		s.handleProjects(w, r, parts)
		return
	case "workspace":
		if len(parts) >= 2 && parts[1] == "export" {
			if len(parts) != 3 {
				writeError(w, http.StatusNotFound, "export endpoint not found")
				return
			}
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			switch parts[2] {
			case "markdown":
				s.handleWorkspaceMarkdownExport(w, r, requestWorkspaceID(r))
			case "docx":
				s.handleWorkspaceDocxExport(w, r, requestWorkspaceID(r))
			case "epub":
				s.handleWorkspaceEPUBExport(w, r, requestWorkspaceID(r))
			default:
				writeError(w, http.StatusNotFound, "export format not found")
			}
			return
		}
		if len(parts) == 2 && parts[1] == "backup" {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			backup, err := s.store.CreateProjectBackup(requestWorkspaceID(r))
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set("Content-Disposition", `attachment; filename="novelstudio-project-backup.json"`)
			writeJSON(w, http.StatusOK, backup)
			return
		}
		if r.Method == http.MethodGet && len(parts) == 1 {
			response, exists := s.store.Response(requestWorkspaceID(r))
			if !exists {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			writeJSON(w, http.StatusOK, response)
			return
		}
	case "stats":
		if r.Method == http.MethodGet && len(parts) == 1 {
			workspace, exists := s.store.MetadataSnapshot(requestWorkspaceID(r))
			if !exists {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			writeJSON(w, http.StatusOK, calculateStats(workspace))
			return
		}
	case "settings":
		s.handleSettings(w, r, parts, requestWorkspaceID(r))
		return
	case "chapters":
		s.handleChapters(w, r, parts, requestWorkspaceID(r))
		return
	case "characters":
		s.handleCharacters(w, r, parts, requestWorkspaceID(r))
		return
	case "notes":
		s.handleNotes(w, r, parts, requestWorkspaceID(r))
		return
	case "chat":
		if r.Method == http.MethodPost && len(parts) == 1 {
			s.handleChat(w, r, requestWorkspaceID(r))
			return
		}
	}
	writeError(w, http.StatusNotFound, "API endpoint not found")
}

func requestWorkspaceID(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get("X-Workspace-ID")); id != "" {
		return id
	}
	return localWorkspaceID
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 2 && parts[1] == "import" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var backup ProjectBackup
		if err := readJSONLimit(r, &backup, maxProjectBackupRequestBytes); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		project, err := s.store.ImportProjectBackup(backup)
		if errors.Is(err, ErrInvalidProjectBackup) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, project)
		return
	}
	if len(parts) == 2 {
		workspaceID := strings.TrimSpace(parts[1])
		if workspaceID == "" {
			writeError(w, http.StatusNotFound, "项目不存在")
			return
		}
		if r.Method == http.MethodDelete {
			if err := s.store.DeleteProject(workspaceID); errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "项目不存在")
				return
			} else if errors.Is(err, ErrLastProject) {
				writeError(w, http.StatusConflict, "至少保留一个项目")
				return
			} else if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "workspace_id": workspaceID})
			return
		}
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var input ProjectInput
		if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		if strings.TrimSpace(input.Title) == "" {
			writeError(w, http.StatusBadRequest, "project title is required")
			return
		}
		if input.WordTarget < 0 {
			writeError(w, http.StatusBadRequest, "project word target cannot be negative")
			return
		}
		project, err := s.store.UpdateProject(workspaceID, input)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		if errors.Is(err, ErrInvalidProjectInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, project)
		return
	}
	if len(parts) != 1 {
		writeError(w, http.StatusNotFound, "project endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"projects": s.store.ListProjects()})
	case http.MethodPost:
		var input ProjectInput
		if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		project, err := s.store.CreateProject(input)
		if errors.Is(err, ErrInvalidProjectInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, project)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request, parts []string, workspaceID string) {
	if len(parts) != 1 {
		writeError(w, http.StatusNotFound, "settings endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		workspace, exists := s.store.MetadataSnapshot(workspaceID)
		if !exists {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		writeJSON(w, http.StatusOK, settingsView(workspace.Settings))
	case http.MethodPut:
		var input SettingsInput
		if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		settings, err := s.store.UpdateSettings(workspaceID, input)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		if errors.Is(err, ErrInvalidModelSettings) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, settings)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleChapters(w http.ResponseWriter, r *http.Request, parts []string, workspaceID string) {
	if len(parts) == 2 && parts[1] == "search" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(query) > 512 {
			writeError(w, http.StatusBadRequest, "search query is too long")
			return
		}
		chapters, exists, err := s.store.SearchChapters(workspaceID, query)
		if !exists {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"chapters": chapters})
		return
	}
	if len(parts) == 2 && parts[1] == "order" {
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var input ChapterOrderInput
		if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		chapters, err := s.store.ReorderChapters(workspaceID, input.ChapterIDs)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		if errors.Is(err, ErrInvalidChapterOrder) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		orderedIDs := make([]int64, len(chapters))
		for index, chapter := range chapters {
			orderedIDs[index] = chapter.ID
		}
		writeJSON(w, http.StatusOK, map[string]any{"chapter_ids": orderedIDs})
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			workspace, exists := s.store.MetadataSnapshot(workspaceID)
			if !exists {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			chapters := make([]ChapterSummary, 0, len(workspace.Chapters))
			for _, chapter := range workspace.Chapters {
				chapters = append(chapters, chapterSummary(chapter))
			}
			writeJSON(w, http.StatusOK, map[string]any{"chapters": chapters})
		case http.MethodPost:
			var input ChapterInput
			if err := readJSON(r, &input); err != nil {
				writeJSONRequestError(w, err)
				return
			}
			chapter, err := s.store.CreateChapter(workspaceID, input)
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			if errors.Is(err, ErrInvalidChapterInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, chapter)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	id, err := parseID(parts[1])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chapter id")
		return
	}
	if len(parts) >= 3 && parts[2] == "revisions" {
		s.handleChapterRevisions(w, r, workspaceID, id, parts[3:])
		return
	}
	if len(parts) == 3 && parts[2] == "generate" && r.Method == http.MethodPost {
		s.handleChapterGenerate(w, r, workspaceID, id)
		return
	}
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "chapter endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		chapter, exists, err := s.store.GetChapter(workspaceID, id)
		if !exists {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "chapter not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, chapter)
	case http.MethodPut:
		var input ChapterInput
		if err := readJSON(r, &input); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		chapter, err := s.store.UpdateChapter(workspaceID, id, input)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "chapter not found")
			return
		}
		if errors.Is(err, ErrInvalidChapterInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, chapter)
	case http.MethodDelete:
		err := s.store.DeleteChapter(workspaceID, id)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "chapter not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleChapterRevisions(w http.ResponseWriter, r *http.Request, workspaceID string, chapterID int64, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			revisions, err := s.store.ListChapterRevisions(workspaceID, chapterID)
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "chapter not found")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"revisions": revisions})
		case http.MethodPost:
			var input ChapterRevisionInput
			if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
				writeJSONRequestError(w, err)
				return
			}
			revision, err := s.store.CreateChapterRevision(workspaceID, chapterID, input)
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "chapter not found")
				return
			}
			if errors.Is(err, ErrInvalidChapterRevisionInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, revision)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	revisionID, err := parseID(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chapter revision id")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		revision, err := s.store.GetChapterRevision(workspaceID, chapterID, revisionID)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "chapter revision not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, revision)
		return
	}
	if len(parts) == 2 && parts[1] == "restore" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		chapter, err := s.store.RestoreChapterRevision(workspaceID, chapterID, revisionID)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "chapter revision not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, chapter)
		return
	}
	writeError(w, http.StatusNotFound, "chapter revision endpoint not found")
}

func (s *Server) handleChapterGenerate(w http.ResponseWriter, r *http.Request, workspaceID string, id int64) {
	var request ChapterGenerateRequest
	if r.Body != nil {
		if err := readJSONLimit(r, &request, maxAgentRequestBytes); err != nil && !errors.Is(err, ErrEmptyJSONBody) {
			writeJSONRequestError(w, err)
			return
		}
	}
	workspace, exists := s.store.MetadataSnapshot(workspaceID)
	if !exists {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	chapter, workspaceExists, err := s.store.GetChapter(workspaceID, id)
	if !workspaceExists {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "chapter not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	applyChapterPromptOverrides(&chapter, request.ChapterTitle, request.ChapterSummary)
	instruction := strings.TrimSpace(request.Instruction)
	if instruction == "" {
		instruction = "继续当前场景，保持克制的悬疑感，并推进一个可见的线索。"
	}
	if utf8.RuneCountInString(instruction) > maxAgentInputRunes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("续写要求不能超过 %d 个字符", maxAgentInputRunes))
		return
	}
	currentText := chapter.Content
	if request.Context != nil {
		currentText = *request.Context
	}
	prompt := buildChapterGeneratePrompt(workspace, chapter, instruction, currentText)
	localContext := localReplyContextFor(workspace, chapter, "write", instruction, currentText)
	localContext.chapterGeneration = true
	reply, provider, err := s.generate(r.Context(), prompt, workspace.Settings, localContext)
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		writeModelError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": reply, "provider": provider, "chapter_id": id})
}

func (s *Server) handleCharacters(w http.ResponseWriter, r *http.Request, parts []string, workspaceID string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			workspace, exists := s.store.MetadataSnapshot(workspaceID)
			if !exists {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"characters": workspace.Characters})
		case http.MethodPost:
			var input CharacterInput
			if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
				writeJSONRequestError(w, err)
				return
			}
			character, err := s.store.CreateCharacter(workspaceID, input)
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			if errors.Is(err, ErrInvalidCharacterInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, character)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "character endpoint not found")
		return
	}
	id, err := parseID(parts[1])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid character id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		workspace, exists := s.store.MetadataSnapshot(workspaceID)
		if !exists {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		for _, character := range workspace.Characters {
			if character.ID == id {
				writeJSON(w, http.StatusOK, character)
				return
			}
		}
		writeError(w, http.StatusNotFound, "character not found")
	case http.MethodPut:
		var input CharacterInput
		if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		character, err := s.store.UpdateCharacter(workspaceID, id, input)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "character not found")
			return
		}
		if errors.Is(err, ErrInvalidCharacterInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, character)
	case http.MethodDelete:
		if err := s.store.DeleteCharacter(workspaceID, id); errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "character not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request, parts []string, workspaceID string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			workspace, exists := s.store.MetadataSnapshot(workspaceID)
			if !exists {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"notes": workspace.Notes})
		case http.MethodPost:
			var input NoteInput
			if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
				writeJSONRequestError(w, err)
				return
			}
			note, err := s.store.CreateNote(workspaceID, input)
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			if errors.Is(err, ErrInvalidNoteInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, note)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "notes endpoint not found")
		return
	}
	id, err := parseID(parts[1])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid note id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		workspace, exists := s.store.MetadataSnapshot(workspaceID)
		if !exists {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		for _, note := range workspace.Notes {
			if note.ID == id {
				writeJSON(w, http.StatusOK, note)
				return
			}
		}
		writeError(w, http.StatusNotFound, "note not found")
	case http.MethodPut:
		var input NoteInput
		if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		note, err := s.store.UpdateNote(workspaceID, id, input)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "note not found")
			return
		}
		if errors.Is(err, ErrInvalidNoteInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, note)
	case http.MethodDelete:
		if err := s.store.DeleteNote(workspaceID, id); errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "note not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request, workspaceID string) {
	streaming := strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream")
	var input ChatRequest
	if err := readJSONLimit(r, &input, maxAgentRequestBytes); err != nil {
		writeJSONRequestError(w, err)
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	input.RequestID = strings.TrimSpace(input.RequestID)
	if len(input.RequestID) > 128 {
		writeError(w, http.StatusBadRequest, "request_id is too long")
		return
	}
	for _, character := range input.RequestID {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_') {
			writeError(w, http.StatusBadRequest, "request_id contains invalid characters")
			return
		}
	}
	if input.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if utf8.RuneCountInString(input.Message) > maxAgentInputRunes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Agent 输入不能超过 %d 个字符", maxAgentInputRunes))
		return
	}
	if utf8.RuneCountInString(input.Selection) > maxAgentSelectionRunes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("选中正文不能超过 %d 个字符", maxAgentSelectionRunes))
		return
	}
	if strings.EqualFold(strings.TrimSpace(input.Mode), "edit") && strings.TrimSpace(input.Selection) == "" {
		writeError(w, http.StatusBadRequest, "改写模式需要先选中正文")
		return
	}
	if input.RequestID != "" {
		release, err := s.acquireChatRequest(r.Context(), workspaceID, input.RequestID)
		if err != nil {
			return
		}
		defer release()
	}
	workspace, exists := s.store.MetadataSnapshot(workspaceID)
	if !exists {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	input.Mode = normalizeWritingMode(input.Mode)
	providerLabel := chatProviderLabel(workspace.Settings)
	requestFingerprint, err := fingerprintChatRequest(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not fingerprint chat request")
		return
	}
	if input.RequestID != "" {
		response, found, err := s.store.LookupChatRequest(workspaceID, input.RequestID, input.Message, requestFingerprint)
		if err != nil {
			if errors.Is(err, ErrChatRequestConflict) || errors.Is(err, ErrChatRequestUnverifiable) {
				writeChatRequestError(w, err)
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		if found {
			if response.Provider == "" {
				response.Provider = providerLabel
			}
			if streaming {
				writeChatStream(w, response)
			} else {
				writeJSON(w, http.StatusOK, response)
			}
			return
		}
	}
	for _, message := range workspace.Chat {
		if input.RequestID == "" || message.Role != "user" || message.RequestID != input.RequestID {
			continue
		}
		if err := chatRequestMismatchError(message, input.Message, requestFingerprint); err != nil {
			writeChatRequestError(w, err)
			return
		}
		if response, found := chatResponseForRequest(workspace.Chat, input.RequestID, providerLabel); found {
			if streaming {
				writeChatStream(w, response)
			} else {
				writeJSON(w, http.StatusOK, response)
			}
			return
		}
		writeChatRequestError(w, ErrChatRequestUnverifiable)
		return
	}
	chapter, chapterExists := chapterFor(workspace.Chapters, input.ChapterID)
	if input.ChapterID != 0 && !chapterExists {
		writeError(w, http.StatusNotFound, "chapter not found")
		return
	}
	applyChapterPromptOverrides(&chapter, input.ChapterTitle, input.ChapterSummary)
	prompt := buildChatPrompt(workspace, chapter, input)
	localContext := localReplyContextFor(workspace, chapter, input.Mode, input.Message, input.Context)
	claimOwner := ""
	if input.RequestID != "" {
		response, ownerToken, claimed, err := s.store.ClaimChatRequest(r.Context(), workspaceID, input.RequestID, input.Message, requestFingerprint)
		if err != nil {
			if errors.Is(err, ErrChatRequestConflict) || errors.Is(err, ErrChatRequestUnverifiable) {
				writeChatRequestError(w, err)
			} else if r.Context().Err() == nil {
				writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		if !claimed {
			if streaming {
				writeChatStream(w, response)
			} else {
				writeJSON(w, http.StatusOK, response)
			}
			return
		}
		claimOwner = ownerToken
		defer func() { _ = s.store.ReleaseChatRequestClaim(workspaceID, input.RequestID, claimOwner) }()
	}
	var reply, provider string
	if streaming {
		modelProvider := strings.ToLower(strings.TrimSpace(workspace.Settings.Provider))
		if err := validateModelSettings(workspace.Settings, modelProvider); err != nil {
			writeModelError(w, err)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming is unavailable")
			return
		}
		beginChatStream(w, flusher)
		reply, provider, err = s.generateStreaming(r.Context(), prompt, workspace.Settings, localContext, func(text string) error {
			return writeChatStreamEvent(w, flusher, "delta", map[string]string{"text": text})
		})
	} else {
		reply, provider, err = s.generate(r.Context(), prompt, workspace.Settings, localContext)
	}
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		if streaming {
			writeChatStreamError(w, err)
		} else {
			writeModelError(w, err)
		}
		return
	}
	response, err := s.store.AppendChatRequest(r.Context(), workspaceID, input.RequestID, input.Message, reply, input.Mode, requestFingerprint, provider, claimOwner)
	if err != nil {
		if errors.Is(err, ErrChatRequestConflict) || errors.Is(err, ErrChatRequestUnverifiable) {
			if streaming {
				writeChatStreamError(w, err)
			} else {
				writeChatRequestError(w, err)
			}
		} else if errors.Is(err, ErrChatRequestClaimLost) {
			if streaming {
				writeChatStreamError(w, err)
			} else {
				writeError(w, http.StatusServiceUnavailable, err.Error())
			}
		} else {
			if streaming {
				writeChatStreamError(w, err)
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
			}
		}
		return
	}
	if streaming {
		_ = writeChatStreamEvent(w, w.(http.Flusher), "complete", response)
	} else {
		writeJSON(w, http.StatusOK, response)
	}
}

func beginChatStream(w http.ResponseWriter, flusher http.Flusher) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
}

func writeChatStream(w http.ResponseWriter, response ChatResponse) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusOK, response)
		return
	}
	beginChatStream(w, flusher)
	if response.Message.Content != "" {
		_ = writeChatStreamEvent(w, flusher, "delta", map[string]string{"text": response.Message.Content})
	}
	_ = writeChatStreamEvent(w, flusher, "complete", response)
}

func writeChatStreamEvent(w http.ResponseWriter, flusher http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeChatStreamError(w http.ResponseWriter, err error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeModelError(w, err)
		return
	}
	status := http.StatusBadGateway
	if errors.Is(err, ErrInvalidModelSettings) || errors.Is(err, ErrChatRequestConflict) || errors.Is(err, ErrChatRequestUnverifiable) {
		status = http.StatusBadRequest
	} else if errors.Is(err, ErrChatRequestClaimLost) {
		status = http.StatusServiceUnavailable
	}
	_ = writeChatStreamEvent(w, flusher, "error", map[string]any{"error": err.Error(), "status": status})
}

func fingerprintChatRequest(input ChatRequest) (string, error) {
	payload := struct {
		Message        string  `json:"message"`
		Mode           string  `json:"mode"`
		ChapterID      int64   `json:"chapter_id"`
		ChapterTitle   *string `json:"chapter_title,omitempty"`
		ChapterSummary *string `json:"chapter_summary,omitempty"`
		Context        string  `json:"context"`
		Selection      string  `json:"selection,omitempty"`
	}{
		Message: input.Message, Mode: input.Mode, ChapterID: input.ChapterID,
		ChapterTitle: input.ChapterTitle, ChapterSummary: input.ChapterSummary,
		Context: input.Context, Selection: input.Selection,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func chatProviderLabel(settings Settings) string {
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))
	if provider == "openai" || provider == "compatible" {
		return strings.TrimSpace(settings.Model)
	}
	return "local-creative"
}

func chatResponseForRequest(messages []ChatMessage, requestID, provider string) (ChatResponse, bool) {
	if requestID == "" {
		return ChatResponse{}, false
	}
	for index := 0; index+1 < len(messages); index++ {
		userMessage := messages[index]
		assistantMessage := messages[index+1]
		if userMessage.Role != "user" || userMessage.RequestID != requestID || assistantMessage.Role != "assistant" || assistantMessage.Mode != userMessage.Mode {
			continue
		}
		return ChatResponse{
			UserMessage: userMessage, Message: assistantMessage,
			Suggestions: suggestionsFor(userMessage.Mode), Provider: provider,
		}, true
	}
	return ChatResponse{}, false
}

func chapterFor(chapters []Chapter, id int64) (Chapter, bool) {
	for _, chapter := range chapters {
		if chapter.ID == id {
			return chapter, true
		}
	}
	if id == 0 && len(chapters) > 0 {
		return chapters[0], true
	}
	return Chapter{}, false
}

func applyChapterPromptOverrides(chapter *Chapter, title, summary *string) {
	if title != nil {
		if cleaned := strings.TrimSpace(*title); cleaned != "" {
			chapter.Title = cleaned
		}
	}
	if summary != nil {
		chapter.Summary = *summary
	}
}

func normalizeWritingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "edit":
		return "edit"
	case "plan":
		return "plan"
	case "review":
		return "review"
	default:
		return "write"
	}
}

func buildChatPrompt(workspace Workspace, chapter Chapter, input ChatRequest) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "你是小说工作室中的写作 Agent。项目：《%s》，类型：%s。\n", trimPromptText(workspace.Project.Title, 120), trimPromptText(workspace.Project.Genre, 120))
	fmt.Fprintf(&builder, "项目 premise：%s\n", trimPromptText(workspace.Project.Premise, 1600))
	fmt.Fprintf(&builder, "当前模式：%s。%s\n", input.Mode, writingModeInstruction(input.Mode))
	if chapter.ID == 0 {
		builder.WriteString("当前项目还没有章节，请依据项目设定和角色资料协助用户规划，不要假设已有正文。\n")
	} else {
		fmt.Fprintf(&builder, "当前章节：第%d章《%s》。章节摘要：%s。\n", chapter.Number, trimPromptText(chapter.Title, 120), trimPromptText(chapter.Summary, 1200))
	}
	appendWorkspaceReferences(&builder, workspace)
	if input.Context != "" {
		contextLabel := "当前章节正文片段（编辑器末尾）"
		if strings.TrimSpace(input.Selection) != "" {
			contextLabel = "当前章节选区附近的正文上下文（选区正文另附）"
		}
		fmt.Fprintf(&builder, "%s：%s\n", contextLabel, trimPromptText(input.Context, 1800))
	}
	appendRecentChat(&builder, workspace.Chat)
	if input.Selection != "" {
		fmt.Fprintf(&builder, "用户选中的正文（改写或审阅时优先围绕此段）：%s\n", trimPromptText(input.Selection, maxAgentSelectionRunes))
	}
	fmt.Fprintf(&builder, "用户请求：%s\n", input.Message)
	builder.WriteString("请用中文回答，不要编造与当前设定冲突的事实。")
	return builder.String()
}

func writingModeInstruction(mode string) string {
	switch normalizeWritingMode(mode) {
	case "edit":
		return "改写模式：只改写用户选中的正文，落实用户提出的要求，并保持事实设定、人物关系和叙述视角一致。只输出可以直接替换选区的正文，不要添加解释、标题、引号或 Markdown。"
	case "plan":
		return "规划模式：给出可执行的大纲方案，优先拆成 3 到 5 个有因果关系的节拍，并说明每拍的角色目标、阻力和结果；除非用户明确要求，不要把大纲扩写成完整正文。"
	case "review":
		return "审阅模式：围绕当前章节或用户提供的文本指出具体问题和依据，再给出可执行的修改建议；避免泛泛鼓励，也不要擅自重写整段，除非用户明确要求。"
	default:
		return "写作模式：优先直接回应创作请求；续写或润色时给出可直接使用的文本，用户要求分析时再提供分析。"
	}
}

func buildChapterGeneratePrompt(workspace Workspace, chapter Chapter, instruction, currentText string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "你是小说工作室中的写作 Agent。任务类型：正文续写。请为《%s》第%d章《%s》续写中文小说正文。\n", trimPromptText(workspace.Project.Title, 120), chapter.Number, trimPromptText(chapter.Title, 120))
	fmt.Fprintf(&builder, "项目类型：%s。项目设定：%s\n", trimPromptText(workspace.Project.Genre, 120), trimPromptText(workspace.Project.Premise, 1600))
	fmt.Fprintf(&builder, "本章目标：%s\n", trimPromptText(chapter.Summary, 1200))
	appendWorkspaceReferences(&builder, workspace)
	if text := strings.TrimSpace(currentText); text != "" {
		fmt.Fprintf(&builder, "当前章节正文（编辑器末尾片段）：%s\n", trimPromptText(text, 1800))
	}
	fmt.Fprintf(&builder, "续写要求：%s\n", instruction)
	builder.WriteString("直接输出可接在现有正文后的小说内容，只写正文，不要重复已有内容，不要添加标题、分析或下一步建议。保持人物、设定和叙述视角一致。")
	return builder.String()
}

func appendWorkspaceReferences(builder *strings.Builder, workspace Workspace) {
	if len(workspace.Characters) > 0 {
		builder.WriteString("角色档案：\n")
		limit := len(workspace.Characters)
		if limit > 16 {
			limit = 16
		}
		for _, character := range workspace.Characters[:limit] {
			fmt.Fprintf(builder, "- %s（%s）：%s。特征：%s。\n",
				trimPromptText(character.Name, 80), trimPromptText(character.Role, 120),
				trimPromptText(character.Description, 500), promptTraits(character.Traits))
		}
		if limit < len(workspace.Characters) {
			fmt.Fprintf(builder, "（另有 %d 份角色档案未列出。）\n", len(workspace.Characters)-limit)
		}
	}
	if len(workspace.Notes) > 0 {
		builder.WriteString("项目便笺：\n")
		limit := len(workspace.Notes)
		if limit > 20 {
			limit = 20
		}
		for _, note := range workspace.Notes[:limit] {
			fmt.Fprintf(builder, "- %s（%s）：%s\n",
				trimPromptText(note.Title, 120), trimPromptText(note.Kind, 80), trimPromptText(note.Body, 600))
		}
		if limit < len(workspace.Notes) {
			fmt.Fprintf(builder, "（另有 %d 条便笺未列出。）\n", len(workspace.Notes)-limit)
		}
	}
}

func appendRecentChat(builder *strings.Builder, messages []ChatMessage) {
	recent := make([]ChatMessage, 0, 8)
	for index := len(messages) - 1; index >= 0 && len(recent) < 8; index-- {
		if messages[index].Role == "system" {
			continue
		}
		recent = append(recent, messages[index])
	}
	if len(recent) == 0 {
		return
	}
	builder.WriteString("最近对话：\n")
	for left, right := 0, len(recent)-1; left < right; left, right = left+1, right-1 {
		recent[left], recent[right] = recent[right], recent[left]
	}
	for _, message := range recent {
		role := "Agent"
		if message.Role == "user" {
			role = "用户"
		}
		fmt.Fprintf(builder, "%s：%s\n", role, trimPromptText(message.Content, 600))
	}
}

func trimPromptText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return ""
	}
	runeCount := 0
	for index := range value {
		if runeCount == maxRunes {
			return value[:index] + "…"
		}
		runeCount++
	}
	return value
}

func promptTraits(traits []string) string {
	if len(traits) > 20 {
		traits = traits[:20]
	}
	cleaned := make([]string, 0, len(traits))
	for _, trait := range traits {
		if value := trimPromptText(trait, 80); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return trimPromptText(strings.Join(cleaned, "、"), 300)
}

func suggestionsFor(mode string) []string {
	switch mode {
	case "edit":
		return []string{"改得更克制一些", "增强画面感", "缩短这段文字"}
	case "plan":
		return []string{"把这个冲突拆成三个节拍", "为本章补一条伏笔", "检查角色动机"}
	case "review":
		return []string{"检查节奏和信息密度", "找出一个可删句子", "生成更有画面感的版本"}
	default:
		return []string{"续写下一段", "提炼本章摘要", "检查角色动机"}
	}
}

type localReplyContext struct {
	chapterGeneration bool
	mode              string
	requestText       string
	projectTitle      string
	projectPremise    string
	chapterNumber     int
	chapterTitle      string
	chapterSummary    string
	characterName     string
	currentText       string
	hasChapter        bool
}

func localReplyContextFor(workspace Workspace, chapter Chapter, mode, requestText, currentText string) localReplyContext {
	context := localReplyContext{
		mode: mode, requestText: requestText,
		projectTitle: workspace.Project.Title, projectPremise: workspace.Project.Premise,
		chapterNumber: chapter.Number, chapterTitle: chapter.Title, chapterSummary: chapter.Summary,
		currentText: currentText, hasChapter: chapter.ID != 0,
	}
	if len(workspace.Characters) > 0 {
		context.characterName = workspace.Characters[0].Name
	}
	return context
}

func (s *Server) generate(ctx context.Context, prompt string, settings Settings, localContext localReplyContext) (string, string, error) {
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))
	if err := validateModelSettings(settings, provider); err != nil {
		return "", "", err
	}
	if provider == "openai" || provider == "compatible" {
		text, err := s.callCompatible(ctx, prompt, settings)
		if err != nil {
			return "", "", err
		}
		return text, strings.TrimSpace(settings.Model), nil
	}
	return localReply(localContext), "local-creative", nil
}

func (s *Server) generateStreaming(ctx context.Context, prompt string, settings Settings, localContext localReplyContext, onDelta func(string) error) (string, string, error) {
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))
	if err := validateModelSettings(settings, provider); err != nil {
		return "", "", err
	}
	if provider == "openai" || provider == "compatible" {
		text, err := s.callCompatibleStreaming(ctx, prompt, settings, onDelta)
		if err != nil {
			return "", "", err
		}
		return text, strings.TrimSpace(settings.Model), nil
	}
	text := localReply(localContext)
	for start, runes := 0, []rune(text); start < len(runes); {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		end := start + 18
		if end > len(runes) {
			end = len(runes)
		}
		if err := onDelta(string(runes[start:end])); err != nil {
			return "", "", err
		}
		start = end
	}
	return text, "local-creative", nil
}

func validateModelSettings(settings Settings, provider string) error {
	switch provider {
	case "", "local":
		return nil
	case "openai", "compatible":
		if strings.TrimSpace(settings.Model) == "" {
			return fmt.Errorf("%w: 模型名称不能为空", ErrInvalidModelSettings)
		}
		if provider == "openai" && strings.TrimSpace(settings.APIKey) == "" {
			return fmt.Errorf("%w: 请填写 API Key", ErrInvalidModelSettings)
		}
		_, err := parseModelEndpoint(settings.BaseURL)
		return err
	default:
		return fmt.Errorf("%w: 不支持的服务商 %q", ErrInvalidModelSettings, settings.Provider)
	}
}

func parseModelEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("%w: 模型接口地址必须是有效的 HTTP 或 HTTPS URL", ErrInvalidModelSettings)
	}
	if endpoint.Scheme == "http" && !isLoopbackModelHost(endpoint.Hostname()) {
		return nil, fmt.Errorf("%w: 明文 HTTP 模型接口仅允许使用 localhost 或回环 IP 地址；网络服务请使用 HTTPS", ErrInvalidModelSettings)
	}
	return endpoint, nil
}

func isLoopbackModelHost(host string) bool {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.IsLoopback()
	}
	return ip.IsLoopback()
}

func writeModelError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, ErrInvalidModelSettings) {
		status = http.StatusBadRequest
	}
	writeError(w, status, err.Error())
}

func (s *Server) callCompatible(ctx context.Context, prompt string, settings Settings) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/"))
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1/chat/completions"
	} else if !strings.HasSuffix(path, "/chat/completions") {
		path += "/chat/completions"
	}
	parsed.Path = path
	body, _ := json.Marshal(map[string]any{"model": strings.TrimSpace(settings.Model), "temperature": settings.Temperature, "messages": []map[string]string{{"role": "system", "content": "你是专业的中文小说写作助手。"}, {"role": "user", "content": prompt}}})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	if apiKey := strings.TrimSpace(settings.APIKey); apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("调用模型失败: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取模型响应失败: %w", err)
	}
	responseTooLarge := int64(len(data)) > maxModelResponseBytes
	if responseTooLarge {
		data = data[:maxModelResponseBytes]
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := trimPromptText(string(data), 2000)
		if responseTooLarge {
			if detail == "" {
				detail = "错误响应超过 4 MiB，内容已截断"
			} else {
				detail += "（错误响应超过 4 MiB，内容已截断）"
			}
		}
		if detail == "" {
			return "", fmt.Errorf("模型返回 %s", response.Status)
		}
		return "", fmt.Errorf("模型返回 %s: %s", response.Status, detail)
	}
	if responseTooLarge {
		return "", fmt.Errorf("模型响应超过 4 MiB")
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("解析模型响应失败: %w", err)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("模型没有返回内容")
	}
	return strings.TrimSpace(decoded.Choices[0].Message.Content), nil
}

func (s *Server) callCompatibleStreaming(ctx context.Context, prompt string, settings Settings, onDelta func(string) error) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/"))
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1/chat/completions"
	} else if !strings.HasSuffix(path, "/chat/completions") {
		path += "/chat/completions"
	}
	parsed.Path = path
	body, err := json.Marshal(map[string]any{
		"model": strings.TrimSpace(settings.Model), "temperature": settings.Temperature, "stream": true,
		"messages": []map[string]string{{"role": "system", "content": "你是专业的中文小说写作助手。"}, {"role": "user", "content": prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("encode model request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	if apiKey := strings.TrimSpace(settings.APIKey); apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("调用模型失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, readErr := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
		if readErr != nil {
			return "", fmt.Errorf("读取模型响应失败: %w", readErr)
		}
		detail := trimPromptText(string(data), 2000)
		if detail == "" {
			return "", fmt.Errorf("模型返回 %s", response.Status)
		}
		return "", fmt.Errorf("模型返回 %s: %s", response.Status, detail)
	}

	reader := bufio.NewReader(response.Body)
	var reply strings.Builder
	var bytesRead int64
	var sawEvent bool
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		line, readErr := reader.ReadString('\n')
		bytesRead += int64(len(line))
		if bytesRead > maxModelResponseBytes {
			return "", fmt.Errorf("模型响应超过 4 MiB")
		}
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
		}
		if strings.HasPrefix(line, "data:") {
			sawEvent = true
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				if readErr != nil {
					break
				}
				continue
			}
			if payload == "[DONE]" {
				break
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				return "", fmt.Errorf("解析模型流响应失败: %w", err)
			}
			if len(chunk.Choices) > 0 {
				text := chunk.Choices[0].Delta.Content
				if text == "" {
					text = chunk.Choices[0].Message.Content
				}
				if text != "" {
					reply.WriteString(text)
					if err := onDelta(text); err != nil {
						return "", err
					}
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return "", fmt.Errorf("读取模型流响应失败: %w", readErr)
		}
	}
	if !sawEvent {
		return "", fmt.Errorf("模型没有返回流式内容")
	}
	if strings.TrimSpace(reply.String()) == "" {
		return "", fmt.Errorf("模型没有返回内容")
	}
	return strings.TrimSpace(reply.String()), nil
}

func localReply(context localReplyContext) string {
	if context.chapterGeneration {
		return localChapterContinuation(context)
	}
	mode := normalizeWritingMode(context.mode)
	request := trimPromptText(context.requestText, 180)
	projectTitle := trimPromptText(context.projectTitle, 120)
	if projectTitle == "" {
		projectTitle = "当前项目"
	}
	chapterTitle := trimPromptText(context.chapterTitle, 120)
	if chapterTitle == "" {
		chapterTitle = "未命名章节"
	}
	chapterGoal := trimPromptText(context.chapterSummary, 180)
	if chapterGoal == "" {
		chapterGoal = "推动故事局势发生变化"
	}
	characterName := strings.TrimSpace(context.characterName)
	if characterName == "" {
		characterName = "主角"
	}
	scope := fmt.Sprintf("《%s》", projectTitle)
	if context.hasChapter {
		scope += fmt.Sprintf(" · 第%d章《%s》", context.chapterNumber, chapterTitle)
	}
	switch mode {
	case "edit":
		return "本地规则引擎不能直接改写选中的正文。请先在模型设置中配置兼容模型，再使用改写模式。"
	case "plan":
		premise := trimPromptText(context.projectPremise, 160)
		if premise == "" {
			premise = "尚未填写项目设定"
		}
		return fmt.Sprintf("规划模式 · %s\n项目设定：%s\n请求：%s\n本章目标：%s\n\n1. 目标：让%s采取一个能推进本章目标的行动。\n2. 阻力：从角色动机或项目设定中引出一个具体障碍。\n3. 选择：让%s作出有代价的决定，并改变下一场景的局面。\n\n下一步建议：把第三拍的结果写进章节目标。", scope, premise, request, chapterGoal, characterName, characterName)
	case "review":
		if !context.hasChapter && strings.TrimSpace(context.currentText) == "" {
			return fmt.Sprintf("审阅模式 · 《%s》\n当前没有章节正文可检查。请求：%s\n\n你可以先创建章节，或把要审阅的文本放进请求。", projectTitle, request)
		}
		return fmt.Sprintf("审阅模式 · %s\n审阅请求：%s\n\n本地规则引擎无法逐句评估正文，先提供结构检查清单：\n- 节奏：确认场景尽早出现行动、变化或悬念。\n- 动机：检查%s的选择是否由已建立的目标和顾虑推动。\n- 信息：把必要设定放进动作、对话或后果，减少连续解释。\n\n下一步建议：先核对与“%s”相关的角色选择。", scope, request, characterName, chapterGoal)
	}
	if strings.Contains(context.requestText, "续写") || strings.Contains(context.requestText, "继续") {
		opening := fmt.Sprintf("%s回到眼前的细节，重新检查刚才的判断。", characterName)
		if strings.TrimSpace(context.currentText) == "" {
			opening = fmt.Sprintf("%s从一个尚未解决的细节开始行动。", characterName)
		}
		return fmt.Sprintf("%s围绕“%s”采取了一个能验证线索的行动。结果没有立刻给出答案，反而让原来的解释出现缺口。为了继续推进，%s必须在当前风险和既定目标之间作出选择。\n\n下一步建议：让这个选择带来可见后果，而不只补充背景说明。", opening, chapterGoal, characterName)
	}
	if strings.Contains(context.requestText, "摘要") {
		if strings.TrimSpace(context.chapterSummary) != "" {
			return fmt.Sprintf("《%s》已保存的本章摘要：%s", chapterTitle, strings.TrimSpace(context.chapterSummary))
		}
		return fmt.Sprintf("《%s》还没有填写本章摘要。本地规则引擎不能可靠概括任意正文；请先填写本章目标，或配置兼容模型后再生成正文摘要。", chapterTitle)
	}
	if strings.Contains(context.requestText, "润色") || strings.Contains(context.requestText, "改写") {
		return "本地规则引擎无法逐句理解并改写正文。可以先试着删去直接说明情绪的句子，把变化落在角色的动作、感官细节或选择上；要逐句润色，请配置兼容模型。"
	}
	return fmt.Sprintf("针对%s的请求“%s”，先明确这一段要改变的东西：推进事件、暴露信息，或改变角色关系。\n\n下一步建议：让%s采取一个与本章目标“%s”相关的行动，并安排一个来自设定或动机的阻力。", scope, request, characterName, chapterGoal)
}

func localChapterContinuation(context localReplyContext) string {
	characterName := strings.TrimSpace(context.characterName)
	if characterName == "" {
		characterName = "主角"
	}
	chapterGoal := trimPromptText(context.chapterSummary, 180)
	if chapterGoal == "" {
		chapterGoal = "眼前尚未解决的问题"
	}
	if strings.TrimSpace(context.currentText) == "" {
		return fmt.Sprintf("%s从一个尚未解决的细节开始行动，试着围绕“%s”验证线索。结果没有立刻给出答案，反而让原来的解释出现缺口。%s必须在当前风险和既定目标之间作出选择。", characterName, chapterGoal, characterName)
	}
	return fmt.Sprintf("%s把注意力重新放回眼前的细节，围绕“%s”采取一个能验证线索的行动。结果没有立刻给出答案，反而让原来的解释出现缺口。%s必须在当前风险和既定目标之间作出选择。", characterName, chapterGoal, characterName)
}

func (s *Server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	webRoot, rootErr := filepath.Abs(s.webDir)
	if rootErr != nil {
		writeError(w, http.StatusInternalServerError, "resolve frontend directory failed")
		return
	}
	requestedPath := strings.TrimPrefix(r.URL.Path, "/")
	requested := filepath.Join(webRoot, filepath.FromSlash(requestedPath))
	if relative, err := filepath.Rel(webRoot, requested); err == nil && isContainedPath(relative) {
		if info, err := os.Stat(requested); err == nil && !info.IsDir() {
			http.ServeFile(w, r, requested)
			return
		}
	}
	if filepath.Ext(requestedPath) != "" {
		http.NotFound(w, r)
		return
	}
	index := filepath.Join(webRoot, "index.html")
	if _, err := os.Stat(index); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "frontend build not found", "hint": "run npm install && npm run build in frontend"})
		return
	}
	http.ServeFile(w, r, index)
}

func isContainedPath(relative string) bool {
	if relative == "." || relative == "" {
		return true
	}
	if relative == ".." {
		return false
	}
	separator := string(filepath.Separator)
	return !strings.HasPrefix(relative, ".."+separator) && !filepath.IsAbs(relative)
}

func parseID(value string) (int64, error) { return strconv.ParseInt(value, 10, 64) }

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func writeChatRequestError(w http.ResponseWriter, err error) {
	code := "chat_request_conflict"
	if errors.Is(err, ErrChatRequestUnverifiable) {
		code = "chat_request_unverifiable"
	}
	writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": code})
}

func writeJSONRequestError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, ErrRequestBodyTooLarge) {
		status = http.StatusRequestEntityTooLarge
	}
	writeError(w, status, err.Error())
}

func readJSON(r *http.Request, target any) error {
	return readJSONLimit(r, target, maxJSONRequestBytes)
}

func readJSONLimit(r *http.Request, target any, maxBytes int64) error {
	if r.Body == nil {
		return ErrEmptyJSONBody
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return fmt.Errorf("%w: request body exceeds %d bytes", ErrRequestBodyTooLarge, maxBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return ErrEmptyJSONBody
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain only one JSON value")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
