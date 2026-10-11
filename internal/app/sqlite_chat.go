package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

type chatRequestRecord struct {
	WorkspaceID        string
	RequestID          string
	RequestFingerprint string
	UserContent        string
	OwnerToken         string
	Response           ChatResponse
	CreatedAt          string
}

func newChatRequestRecord(workspaceID, requestID, requestFingerprint, user string, response ChatResponse, ownerToken string) chatRequestRecord {
	return chatRequestRecord{
		WorkspaceID: workspaceID, RequestID: requestID, RequestFingerprint: requestFingerprint,
		UserContent: user, OwnerToken: ownerToken, Response: response, CreatedAt: now(),
	}
}

type chatRequestQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func lookupChatRequestDB(db chatRequestQueryer, workspaceID, requestID, user, requestFingerprint string) (ChatResponse, bool, error) {
	var storedFingerprint, storedUser, responseJSON string
	err := db.QueryRow(`SELECT request_fingerprint, user_content, response_json FROM chat_requests
		WHERE workspace_id = ? AND request_id = ?`, workspaceID, requestID).Scan(&storedFingerprint, &storedUser, &responseJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ChatResponse{}, false, nil
	}
	if err != nil {
		return ChatResponse{}, false, fmt.Errorf("load chat request result: %w", err)
	}
	existing := ChatMessage{Content: storedUser, RequestFingerprint: storedFingerprint}
	if err := chatRequestMismatchError(existing, user, requestFingerprint); err != nil {
		return ChatResponse{}, false, err
	}
	if responseJSON == "" {
		return ChatResponse{}, false, ErrChatRequestUnverifiable
	}
	var response ChatResponse
	if err := json.Unmarshal([]byte(responseJSON), &response); err != nil {
		return ChatResponse{}, false, fmt.Errorf("decode chat request result: %w", err)
	}
	return response, true, nil
}

func appendChatResponseDB(db *sql.DB, workspaceID, requestID, user, requestFingerprint, ownerToken string, response ChatResponse) (ChatResponse, error) {
	tx, err := db.Begin()
	if err != nil {
		return ChatResponse{}, fmt.Errorf("begin chat append transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Acquire SQLite's write lock before reading the request ledger or allocating
	// message IDs so independent Store instances cannot append from the same cache.
	if _, err := tx.Exec("UPDATE workspaces SET updated_at = updated_at WHERE workspace_id = ?", workspaceID); err != nil {
		return ChatResponse{}, fmt.Errorf("lock workspace for chat append: %w", err)
	}
	var exists int
	if err := tx.QueryRow("SELECT 1 FROM workspaces WHERE workspace_id = ?", workspaceID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ChatResponse{}, os.ErrNotExist
		}
		return ChatResponse{}, fmt.Errorf("check workspace for chat append: %w", err)
	}

	if requestID != "" {
		if stored, found, err := lookupChatRequestDB(tx, workspaceID, requestID, user, requestFingerprint); err != nil {
			return ChatResponse{}, err
		} else if found {
			if ownerToken != "" {
				if _, err := tx.Exec("DELETE FROM chat_request_claims WHERE workspace_id = ? AND request_id = ? AND owner_token = ?", workspaceID, requestID, ownerToken); err != nil {
					return ChatResponse{}, fmt.Errorf("release completed chat request claim: %w", err)
				}
			}
			if err := tx.Commit(); err != nil {
				return ChatResponse{}, fmt.Errorf("commit chat request replay: %w", err)
			}
			return stored, nil
		}

		var storedUser, storedFingerprint string
		err := tx.QueryRow("SELECT content, request_fingerprint FROM chat_messages WHERE workspace_id = ? AND request_id = ? AND role = 'user'", workspaceID, requestID).Scan(&storedUser, &storedFingerprint)
		if err == nil {
			if err := chatRequestMismatchError(ChatMessage{Content: storedUser, RequestFingerprint: storedFingerprint}, user, requestFingerprint); err != nil {
				return ChatResponse{}, err
			}
			return ChatResponse{}, ErrChatRequestUnverifiable
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return ChatResponse{}, fmt.Errorf("check existing chat message: %w", err)
		}
	}

	var maxID int64
	if err := tx.QueryRow("SELECT COALESCE(MAX(id), 0) FROM (SELECT project_id AS id FROM workspaces UNION ALL SELECT id FROM chapters UNION ALL SELECT id FROM characters UNION ALL SELECT id FROM notes UNION ALL SELECT id FROM chat_messages)").Scan(&maxID); err != nil {
		return ChatResponse{}, fmt.Errorf("allocate chat message IDs: %w", err)
	}
	response.UserMessage.ID = maxID + 1
	response.Message.ID = maxID + 2
	response.UserMessage.RequestID = requestID
	response.UserMessage.RequestFingerprint = requestFingerprint

	updatedAt := now()
	if _, err := tx.Exec("UPDATE workspaces SET updated_at = ? WHERE workspace_id = ?", updatedAt, workspaceID); err != nil {
		return ChatResponse{}, fmt.Errorf("update workspace activity: %w", err)
	}
	for _, message := range []ChatMessage{response.UserMessage, response.Message} {
		if _, err := tx.Exec("INSERT INTO chat_messages (id, workspace_id, role, content, created_at, mode, request_id, request_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", message.ID, workspaceID, message.Role, message.Content, message.CreatedAt, message.Mode, message.RequestID, message.RequestFingerprint); err != nil {
			return ChatResponse{}, fmt.Errorf("save chat message %d: %w", message.ID, err)
		}
	}
	if _, err := tx.Exec("DELETE FROM chat_messages WHERE workspace_id = ? AND id NOT IN (SELECT id FROM chat_messages WHERE workspace_id = ? ORDER BY id DESC LIMIT 60)", workspaceID, workspaceID); err != nil {
		return ChatResponse{}, fmt.Errorf("prune chat history: %w", err)
	}
	if requestID != "" {
		record := newChatRequestRecord(workspaceID, requestID, requestFingerprint, user, response, ownerToken)
		if err := insertChatRequestRecord(tx, record); err != nil {
			return ChatResponse{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ChatResponse{}, fmt.Errorf("commit chat append: %w", err)
	}
	return response, nil
}

func saveChatRequestRecord(db *sql.DB, record chatRequestRecord) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin chat request result transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertChatRequestRecord(tx, record); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit chat request result: %w", err)
	}
	return nil
}

func insertChatRequestRecord(tx *sql.Tx, record chatRequestRecord) error {
	if record.OwnerToken == "" {
		return ErrChatRequestClaimLost
	}
	var currentOwner string
	var leaseUntil int64
	if err := tx.QueryRow(`SELECT owner_token, lease_until FROM chat_request_claims
		WHERE workspace_id = ? AND request_id = ?`, record.WorkspaceID, record.RequestID).Scan(&currentOwner, &leaseUntil); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatRequestClaimLost
		}
		return fmt.Errorf("verify chat request claim: %w", err)
	}
	if currentOwner != record.OwnerToken || leaseUntil <= time.Now().UnixMilli() {
		return ErrChatRequestClaimLost
	}
	responseJSON, err := json.Marshal(record.Response)
	if err != nil {
		return fmt.Errorf("encode chat request result: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO chat_requests (workspace_id, request_id, request_fingerprint, user_content, response_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, record.WorkspaceID, record.RequestID, record.RequestFingerprint, record.UserContent, string(responseJSON), record.CreatedAt); err != nil {
		return fmt.Errorf("save chat request result: %w", err)
	}
	if record.OwnerToken != "" {
		if _, err := tx.Exec(`DELETE FROM chat_request_claims WHERE workspace_id = ? AND request_id = ? AND owner_token = ?`, record.WorkspaceID, record.RequestID, record.OwnerToken); err != nil {
			return fmt.Errorf("complete chat request claim: %w", err)
		}
	}
	return nil
}
