package app

import (
	"errors"
	"net/http"
	"os"
)

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
