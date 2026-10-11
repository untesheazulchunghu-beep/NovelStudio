package app

import (
	"errors"
	"net/http"
	"os"
)

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request, parts []string, workspaceID string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			workspace, exists := s.store.MetadataSnapshot(workspaceID)
			if !exists {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"timeline": workspace.Timeline})
		case http.MethodPost:
			var input TimelineEventInput
			if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
				writeJSONRequestError(w, err)
				return
			}
			event, err := s.store.CreateTimelineEvent(workspaceID, input)
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			if errors.Is(err, ErrInvalidTimelineEventInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, event)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "timeline endpoint not found")
		return
	}
	id, err := parseID(parts[1])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid timeline event id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		workspace, exists := s.store.MetadataSnapshot(workspaceID)
		if !exists {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		for _, event := range workspace.Timeline {
			if event.ID == id {
				writeJSON(w, http.StatusOK, event)
				return
			}
		}
		writeError(w, http.StatusNotFound, "timeline event not found")
	case http.MethodPut:
		var input TimelineEventInput
		if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		event, err := s.store.UpdateTimelineEvent(workspaceID, id, input)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "timeline event not found")
			return
		}
		if errors.Is(err, ErrInvalidTimelineEventInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, event)
	case http.MethodDelete:
		if err := s.store.DeleteTimelineEvent(workspaceID, id); errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "timeline event not found")
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
