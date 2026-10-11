package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func chatRequestMismatchError(existing ChatMessage, submittedUser, requestFingerprint string) error {
	if existing.RequestFingerprint == requestFingerprint {
		return nil
	}
	if existing.RequestFingerprint == "" && strings.TrimSpace(existing.Content) == strings.TrimSpace(submittedUser) {
		return ErrChatRequestUnverifiable
	}
	return ErrChatRequestConflict
}

func (s *Store) LookupChatRequest(workspaceID, requestID, user, requestFingerprint string) (ChatResponse, bool, error) {
	if requestID == "" {
		return ChatResponse{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	response, found, err := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint)
	if err == nil && found {
		if err := s.refreshWorkspaceLocked(workspaceID); err != nil {
			return ChatResponse{}, false, err
		}
	}
	return response, found, err
}

const chatRequestClaimLease = 2 * time.Minute

const chatRequestClaimPollInterval = 100 * time.Millisecond

func (s *Store) ClaimChatRequest(ctx context.Context, workspaceID, requestID, user, requestFingerprint string) (ChatResponse, string, bool, error) {
	if requestID == "" {
		return ChatResponse{}, "", false, nil
	}
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, "", false, err
	}
	ownerBytes := make([]byte, 16)
	if _, err := rand.Read(ownerBytes); err != nil {
		return ChatResponse{}, "", false, fmt.Errorf("create chat request claim token: %w", err)
	}
	ownerToken := hex.EncodeToString(ownerBytes)

	s.mu.Lock()
	_, err := s.db.Exec("DELETE FROM chat_request_claims WHERE lease_until <= ?", time.Now().UnixMilli())
	s.mu.Unlock()
	if err != nil {
		return ChatResponse{}, "", false, fmt.Errorf("remove expired chat request claims: %w", err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return ChatResponse{}, "", false, err
		}
		s.mu.Lock()
		response, found, err := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint)
		claimed := false
		if err == nil && !found {
			claimed, err = tryClaimChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint, ownerToken, time.Now())
		}
		if err == nil && found {
			err = s.refreshWorkspaceLocked(workspaceID)
		}
		s.mu.Unlock()
		if err != nil {
			return ChatResponse{}, "", false, err
		}
		if found {
			return response, "", false, nil
		}
		if claimed {
			s.mu.Lock()
			response, found, err = lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint)
			if err == nil && found {
				err = s.refreshWorkspaceLocked(workspaceID)
			}
			s.mu.Unlock()
			if err != nil || found {
				_ = s.ReleaseChatRequestClaim(workspaceID, requestID, ownerToken)
				return response, "", false, err
			}
			return ChatResponse{}, ownerToken, true, nil
		}

		timer := time.NewTimer(chatRequestClaimPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ChatResponse{}, "", false, ctx.Err()
		case <-timer.C:
		}
	}
}

func tryClaimChatRequestDB(db *sql.DB, workspaceID, requestID, user, requestFingerprint, ownerToken string, currentTime time.Time) (bool, error) {
	nowMillis := currentTime.UnixMilli()
	leaseUntil := currentTime.Add(chatRequestClaimLease).UnixMilli()
	result, err := db.Exec(`INSERT OR IGNORE INTO chat_request_claims
		(workspace_id, request_id, request_fingerprint, user_content, owner_token, lease_until)
		VALUES (?, ?, ?, ?, ?, ?)`, workspaceID, requestID, requestFingerprint, user, ownerToken, leaseUntil)
	if err != nil {
		return false, fmt.Errorf("claim chat request: %w", err)
	}
	if changed, err := result.RowsAffected(); err != nil {
		return false, fmt.Errorf("check chat request claim: %w", err)
	} else if changed == 1 {
		return true, nil
	}

	var storedFingerprint, storedUser string
	var storedLeaseUntil int64
	err = db.QueryRow(`SELECT request_fingerprint, user_content, lease_until FROM chat_request_claims
		WHERE workspace_id = ? AND request_id = ?`, workspaceID, requestID).Scan(&storedFingerprint, &storedUser, &storedLeaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read chat request claim: %w", err)
	}
	if err := chatRequestMismatchError(ChatMessage{Content: storedUser, RequestFingerprint: storedFingerprint}, user, requestFingerprint); err != nil {
		return false, err
	}
	if storedLeaseUntil > nowMillis {
		return false, nil
	}
	result, err = db.Exec(`UPDATE chat_request_claims SET owner_token = ?, lease_until = ?
		WHERE workspace_id = ? AND request_id = ? AND lease_until <= ?`, ownerToken, leaseUntil, workspaceID, requestID, nowMillis)
	if err != nil {
		return false, fmt.Errorf("take over expired chat request claim: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check expired chat request claim: %w", err)
	}
	return changed == 1, nil
}

func (s *Store) ReleaseChatRequestClaim(workspaceID, requestID, ownerToken string) error {
	if requestID == "" || ownerToken == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.db.Exec(`DELETE FROM chat_request_claims WHERE workspace_id = ? AND request_id = ? AND owner_token = ?`, workspaceID, requestID, ownerToken); err != nil {
		return fmt.Errorf("release chat request claim: %w", err)
	}
	return nil
}

func (s *Store) AppendChatRequest(ctx context.Context, workspaceID, requestID, user, assistant, mode, requestFingerprint, provider, ownerToken string) (ChatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}
	if requestID != "" {
		if response, found, err := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint); err != nil {
			return ChatResponse{}, err
		} else if found {
			if err := s.refreshWorkspaceLocked(workspaceID); err != nil {
				return ChatResponse{}, err
			}
			return response, nil
		}
	}
	if err := s.refreshWorkspaceLocked(workspaceID); err != nil {
		return ChatResponse{}, err
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return ChatResponse{}, os.ErrNotExist
	}
	for _, message := range workspace.Chat {
		if requestID == "" || message.Role != "user" || message.RequestID != requestID {
			continue
		}
		if err := chatRequestMismatchError(message, user, requestFingerprint); err != nil {
			return ChatResponse{}, err
		}
		if response, found := chatResponseForRequest(workspace.Chat, requestID, provider); found {
			if err := saveChatRequestRecord(s.db, newChatRequestRecord(workspaceID, requestID, requestFingerprint, user, response, ownerToken)); err != nil {
				if stored, found, lookupErr := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint); lookupErr != nil {
					return ChatResponse{}, lookupErr
				} else if found {
					return stored, nil
				}
				return ChatResponse{}, err
			}
			return response, nil
		}
		return ChatResponse{}, ErrChatRequestUnverifiable
	}

	userMessage := ChatMessage{Role: "user", Content: user, CreatedAt: now(), Mode: mode, RequestID: requestID, RequestFingerprint: requestFingerprint}
	assistantMessage := ChatMessage{Role: "assistant", Content: assistant, CreatedAt: now(), Mode: mode}
	response := ChatResponse{UserMessage: userMessage, Message: assistantMessage, Suggestions: suggestionsFor(mode), Provider: provider}
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}
	response, err := appendChatResponseDB(s.db, workspaceID, requestID, user, requestFingerprint, ownerToken, response)
	if err != nil {
		if requestID != "" {
			if stored, found, lookupErr := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint); lookupErr == nil && found {
				return stored, nil
			} else if lookupErr != nil {
				return ChatResponse{}, lookupErr
			}
		}
		return ChatResponse{}, err
	}
	if err := s.refreshWorkspaceLocked(workspaceID); err != nil {
		return ChatResponse{}, err
	}
	return response, nil
}

func (s *Store) AppendChat(ctx context.Context, workspaceID, requestID, user, assistant, mode, requestFingerprint string) ([]ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return nil, os.ErrNotExist
	}
	for _, message := range workspace.Chat {
		if requestID != "" && message.Role == "user" && message.RequestID == requestID {
			if message.RequestFingerprint != requestFingerprint {
				return nil, chatRequestMismatchError(message, user, requestFingerprint)
			}
			return append([]ChatMessage(nil), workspace.Chat...), nil
		}
	}
	workspace = cloneWorkspace(workspace)
	nextMessageID := s.nextIDLocked()
	if strings.TrimSpace(user) != "" {
		workspace.Chat = append(workspace.Chat, ChatMessage{ID: nextMessageID, Role: "user", Content: user, CreatedAt: now(), Mode: mode, RequestID: requestID, RequestFingerprint: requestFingerprint})
		nextMessageID++
	}
	if strings.TrimSpace(assistant) != "" {
		workspace.Chat = append(workspace.Chat, ChatMessage{ID: nextMessageID, Role: "assistant", Content: assistant, CreatedAt: now(), Mode: mode})
	}
	if len(workspace.Chat) > 60 {
		workspace.Chat = workspace.Chat[len(workspace.Chat)-60:]
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		if requestID != "" {
			persisted, loadErr := loadWorkspace(s.db, workspaceID)
			if loadErr == nil {
				for _, message := range persisted.Chat {
					if message.Role == "user" && message.RequestID == requestID {
						if message.RequestFingerprint != requestFingerprint {
							return nil, chatRequestMismatchError(message, user, requestFingerprint)
						}
						s.workspaces[workspaceID] = workspaceMetadata(persisted)
						return append([]ChatMessage(nil), persisted.Chat...), nil
					}
				}
			}
		}
		return nil, err
	}
	return append([]ChatMessage(nil), workspace.Chat...), nil
}
