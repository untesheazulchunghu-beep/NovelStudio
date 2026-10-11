package app

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	case "timeline":
		s.handleTimeline(w, r, parts, requestWorkspaceID(r))
		return
	case "world-assets":
		s.handleWorldAssets(w, r, parts, requestWorkspaceID(r))
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
