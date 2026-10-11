package app

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"
)

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
