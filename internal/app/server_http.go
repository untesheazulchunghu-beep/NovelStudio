package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

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
