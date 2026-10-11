package app

import (
	"errors"
	"net/http"
	"os"
)

func (s *Server) handleWorldAssets(w http.ResponseWriter, r *http.Request, parts []string, workspaceID string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			workspace, exists := s.store.MetadataSnapshot(workspaceID)
			if !exists {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"world_assets": workspace.WorldAssets})
		case http.MethodPost:
			var input WorldAssetInput
			if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
				writeJSONRequestError(w, err)
				return
			}
			asset, err := s.store.CreateWorldAsset(workspaceID, input)
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			if errors.Is(err, ErrInvalidWorldAssetInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, asset)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "world asset endpoint not found")
		return
	}
	id, err := parseID(parts[1])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world asset id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		workspace, exists := s.store.MetadataSnapshot(workspaceID)
		if !exists {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		for _, asset := range workspace.WorldAssets {
			if asset.ID == id {
				writeJSON(w, http.StatusOK, asset)
				return
			}
		}
		writeError(w, http.StatusNotFound, "world asset not found")
	case http.MethodPut:
		var input WorldAssetInput
		if err := readJSONLimit(r, &input, maxSmallJSONRequestBytes); err != nil {
			writeJSONRequestError(w, err)
			return
		}
		asset, err := s.store.UpdateWorldAsset(workspaceID, id, input)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "world asset not found")
			return
		}
		if errors.Is(err, ErrInvalidWorldAssetInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, asset)
	case http.MethodDelete:
		if err := s.store.DeleteWorldAsset(workspaceID, id); errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "world asset not found")
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
