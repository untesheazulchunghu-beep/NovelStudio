package app

import (
	"errors"
	"net/http"
	"os"
	"strings"
)

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
