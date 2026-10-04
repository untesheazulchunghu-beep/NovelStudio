package novel

import (
	"net/http"
	"strconv"

	"novelstudio/internal/audit"
)

func (s *Server) getAuditLogs(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}

	novelID := r.PathValue("novelID")
	if !s.requireNovelRole(w, novelID, userID, "owner", "editor", "reviewer", "viewer") {
		return
	}

	// Parse query parameters
	limit := 50
	offset := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	var actionFilter *audit.Action
	if actionStr := r.URL.Query().Get("action"); actionStr != "" {
		action := audit.Action(actionStr)
		actionFilter = &action
	}

	var userFilter *string
	if userStr := r.URL.Query().Get("user_id"); userStr != "" {
		userFilter = &userStr
	}

	// Query audit logs
	entries, err := audit.Query(r.Context(), s.db, userFilter, &novelID, actionFilter, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "查询审计日志失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"limit":   limit,
		"offset":  offset,
	})
}
