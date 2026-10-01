package novel

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"novelstudio/internal/auth"
	"novelstudio/internal/llm"
	"novelstudio/internal/ratelimit"
	"novelstudio/internal/task"
)

const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; " +
	"font-src 'self' data:; script-src 'self'; connect-src 'self' ws: wss:; " +
	"base-uri 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'"

type Server struct {
	db          *sql.DB
	sessions    *auth.Sessions
	tasks       *task.Manager
	llmRuntime  *llm.Runtime
	origins     []string
	staticDir   string
	static      http.Handler
	authLimiter *ratelimit.Limiter
}

func NewServer(db *sql.DB, sessions *auth.Sessions, tasks *task.Manager, origins []string, staticDir string) *Server {
	server := &Server{
		db:          db,
		sessions:    sessions,
		tasks:       tasks,
		origins:     origins,
		staticDir:   staticDir,
		authLimiter: ratelimit.New(20, time.Minute),
	}
	if staticDir != "" {
		server.static = http.FileServer(http.Dir(staticDir))
	}
	return server
}

func (s *Server) SetLLMRuntime(runtime *llm.Runtime) {
	s.llmRuntime = runtime
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/settings/llm", s.getLLMSettings)
	mux.HandleFunc("PUT /api/v1/settings/llm", s.updateLLMSettings)
	mux.HandleFunc("POST /api/v1/settings/llm/test", s.testLLMConnection)
	mux.HandleFunc("POST /api/v1/auth/register", s.rateLimit(s.register))
	mux.HandleFunc("POST /api/v1/auth/login", s.rateLimit(s.login))
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/me", s.me)
	mux.HandleFunc("GET /api/v1/workspaces", s.workspaces)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/members", s.listMembers)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/members", s.addMember)
	mux.HandleFunc("PUT /api/v1/workspaces/{workspaceID}/members/{userID}", s.updateMember)
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/members/{userID}", s.removeMember)
	mux.HandleFunc("GET /api/v1/novels", s.listNovels)
	mux.HandleFunc("GET /api/v1/novels/trash", s.listDeletedNovels)
	mux.HandleFunc("POST /api/v1/novels", s.createNovel)
	mux.HandleFunc("GET /api/v1/novels/{novelID}", s.getNovel)
	mux.HandleFunc("PUT /api/v1/novels/{novelID}", s.updateNovel)
	mux.HandleFunc("DELETE /api/v1/novels/{novelID}", s.deleteNovel)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/restore", s.restoreNovel)
	mux.HandleFunc("GET /api/v1/novels/{novelID}/export", s.exportNovel)
	mux.HandleFunc("GET /api/v1/novels/{novelID}/chapters", s.listChapters)
	mux.HandleFunc("GET /api/v1/novels/{novelID}/chapters/trash", s.listDeletedChapters)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/chapters", s.createChapter)
	mux.HandleFunc("PUT /api/v1/novels/{novelID}/chapters/order", s.reorderChapters)
	mux.HandleFunc("DELETE /api/v1/novels/{novelID}/chapters/{chapterID}", s.deleteChapter)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/chapters/{chapterID}/restore", s.restoreChapter)
	mux.HandleFunc("PUT /api/v1/novels/{novelID}/chapters/{chapterID}", s.updateChapter)
	mux.HandleFunc("GET /api/v1/novels/{novelID}/chapters/{chapterID}/revisions", s.listChapterRevisions)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/chapters/{chapterID}/revisions/{revision}/restore", s.restoreChapterRevision)
	mux.HandleFunc("GET /api/v1/novels/{novelID}/graph", s.getGraph)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/graph/nodes", s.createGraphNode)
	mux.HandleFunc("PUT /api/v1/novels/{novelID}/graph/nodes/{nodeID}", s.updateGraphNode)
	mux.HandleFunc("DELETE /api/v1/novels/{novelID}/graph/nodes/{nodeID}", s.deleteGraphNode)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/graph/edges", s.createGraphEdge)
	mux.HandleFunc("PUT /api/v1/novels/{novelID}/graph/edges/{edgeID}", s.updateGraphEdge)
	mux.HandleFunc("DELETE /api/v1/novels/{novelID}/graph/edges/{edgeID}", s.deleteGraphEdge)
	mux.HandleFunc("GET /api/v1/novels/{novelID}/workflows", s.listWorkflows)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/workflows", s.createWorkflow)
	mux.HandleFunc("PUT /api/v1/novels/{novelID}/workflows/{workflowID}", s.updateWorkflow)
	mux.HandleFunc("DELETE /api/v1/novels/{novelID}/workflows/{workflowID}", s.deleteWorkflow)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/workflows/{workflowID}/run", s.runWorkflow)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/tasks/batch", s.createBatchTask)
	mux.HandleFunc("GET /api/v1/novels/{novelID}/tasks", s.listTasks)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}", s.getTask)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}/events", s.getTaskEvents)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}/ws", s.taskWebSocket)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/cancel", s.cancelTask)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/retry", s.retryTask)
	mux.HandleFunc("GET /api/v1/novels/{novelID}/proposals", s.listProposals)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/proposals/{proposalID}/apply", s.applyProposal)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/proposals/{proposalID}/reject", s.rejectProposal)
	mux.HandleFunc("POST /api/v1/novels/{novelID}/proposals/{proposalID}/regenerate", s.regenerateStaleProposal)
	mux.Handle("/", s.staticHandler())
	return s.withSecurityHeaders(s.withCORS(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "数据库暂时不可用")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// staticHandler serves the built SPA when a static directory is configured and
// falls back to index.html so client-side routes keep working on refresh.
func (s *Server) staticHandler() http.Handler {
	if s.static == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
		})
	}
	indexFile := filepath.Join(s.staticDir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
			return
		}
		clean := path.Clean("/" + r.URL.Path)
		if file := filepath.Join(s.staticDir, filepath.FromSlash(clean)); isRegularFile(file) {
			if strings.HasPrefix(clean, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			s.static.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, indexFile)
	})
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Add("Vary", "Origin")
			if s.originAllowed(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
		}
		if r.Method == http.MethodOptions && strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	for _, allowed := range s.origins {
		if allowed = strings.TrimSpace(allowed); allowed != "" && strings.EqualFold(allowed, parsed.Host) {
			return true
		}
	}
	return false
}

func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		header.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			header.Set("Content-Security-Policy", contentSecurityPolicy)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authLimiter.Allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "尝试过于频繁，请稍后再试")
			return
		}
		next(w, r)
	}
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, version, ok := s.sessions.UserIDVersion(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "请先登录")
		return "", false
	}
	var current int64
	if err := s.db.QueryRowContext(r.Context(), `SELECT session_version FROM users WHERE id = ?`, userID).Scan(&current); err != nil || current != version {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "会话已失效，请重新登录")
		return "", false
	}
	return userID, true
}

func (s *Server) requireNovelRole(w http.ResponseWriter, novelID, userID string, accepted ...string) bool {
	var role string
	err := s.db.QueryRow(`SELECT wm.role FROM novels n JOIN workspace_members wm ON wm.workspace_id = n.workspace_id WHERE n.id = ? AND n.deleted_at IS NULL AND wm.user_id = ?`, novelID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOVEL_NOT_FOUND", "小说不存在或无权访问")
		return false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取项目权限失败")
		return false
	}
	for _, allowed := range accepted {
		if role == allowed {
			return true
		}
	}
	writeError(w, http.StatusForbidden, "FORBIDDEN", "当前角色没有执行此操作的权限")
	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求内容格式无效")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
