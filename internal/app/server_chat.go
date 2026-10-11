package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

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
