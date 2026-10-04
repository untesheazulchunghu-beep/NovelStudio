package novel

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

type Invitation struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Role        string `json:"role"`
	CreatedBy   string `json:"created_by"`
	ExpiresAt   string `json:"expires_at"`
	UsedBy      string `json:"used_by,omitempty"`
	UsedAt      string `json:"used_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

func generateInvitationCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) createInvitation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int64)
	workspaceID := r.PathValue("workspaceID")

	var req struct {
		Role      string `json:"role"`
		ExpiresIn int    `json:"expires_in"` // hours
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Role != "editor" && req.Role != "reviewer" && req.Role != "viewer" {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	if req.ExpiresIn <= 0 || req.ExpiresIn > 168 {
		req.ExpiresIn = 168 // default 7 days
	}

	var role string
	err := s.db.QueryRow(`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`,
		workspaceID, userID).Scan(&role)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if role != "owner" {
		http.Error(w, "only owners can create invitations", http.StatusForbidden)
		return
	}

	code, err := generateInvitationCode()
	if err != nil {
		http.Error(w, "failed to generate code", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	expiresAt := time.Now().UTC().Add(time.Duration(req.ExpiresIn) * time.Hour).Format(time.RFC3339)

	_, err = s.db.Exec(`
		INSERT INTO workspace_invitations (id, workspace_id, role, created_by, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, code, workspaceID, req.Role, userID, expiresAt, now)
	if err != nil {
		http.Error(w, "failed to create invitation", http.StatusInternalServerError)
		return
	}

	inv := Invitation{
		ID:          code,
		WorkspaceID: workspaceID,
		Role:        req.Role,
		CreatedBy:   string(rune(userID)),
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int64)
	workspaceID := r.PathValue("workspaceID")

	var role string
	err := s.db.QueryRow(`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`,
		workspaceID, userID).Scan(&role)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if role != "owner" {
		http.Error(w, "only owners can list invitations", http.StatusForbidden)
		return
	}

	rows, err := s.db.Query(`
		SELECT id, workspace_id, role, created_by, expires_at, used_by, used_at, created_at
		FROM workspace_invitations
		WHERE workspace_id = ?
		ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var invitations []Invitation
	for rows.Next() {
		var inv Invitation
		var usedBy, usedAt sql.NullString
		if err := rows.Scan(&inv.ID, &inv.WorkspaceID, &inv.Role, &inv.CreatedBy, &inv.ExpiresAt,
			&usedBy, &usedAt, &inv.CreatedAt); err != nil {
			http.Error(w, "scan failed", http.StatusInternalServerError)
			return
		}
		if usedBy.Valid {
			inv.UsedBy = usedBy.String
		}
		if usedAt.Valid {
			inv.UsedAt = usedAt.String
		}
		invitations = append(invitations, inv)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invitations)
}

func (s *Server) deleteInvitation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int64)
	workspaceID := r.PathValue("workspaceID")
	invitationID := r.PathValue("invitationID")

	var role string
	err := s.db.QueryRow(`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`,
		workspaceID, userID).Scan(&role)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if role != "owner" {
		http.Error(w, "only owners can delete invitations", http.StatusForbidden)
		return
	}

	_, err = s.db.Exec(`DELETE FROM workspace_invitations WHERE id = ? AND workspace_id = ?`,
		invitationID, workspaceID)
	if err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int64)
	invitationID := r.PathValue("invitationID")

	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, "tx failed", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var inv Invitation
	var usedBy sql.NullString
	err = tx.QueryRow(`
		SELECT id, workspace_id, role, expires_at, used_by
		FROM workspace_invitations
		WHERE id = ?
	`, invitationID).Scan(&inv.ID, &inv.WorkspaceID, &inv.Role, &inv.ExpiresAt, &usedBy)
	if err != nil {
		http.Error(w, "invitation not found", http.StatusNotFound)
		return
	}

	if usedBy.Valid {
		http.Error(w, "invitation already used", http.StatusBadRequest)
		return
	}

	expiresAt, _ := time.Parse(time.RFC3339, inv.ExpiresAt)
	if time.Now().UTC().After(expiresAt) {
		http.Error(w, "invitation expired", http.StatusBadRequest)
		return
	}

	var exists int
	tx.QueryRow(`SELECT 1 FROM workspace_members WHERE workspace_id = ? AND user_id = ?`,
		inv.WorkspaceID, userID).Scan(&exists)
	if exists == 1 {
		http.Error(w, "already a member", http.StatusBadRequest)
		return
	}

	_, err = tx.Exec(`
		INSERT INTO workspace_members (workspace_id, user_id, role, joined_at)
		VALUES (?, ?, ?, ?)
	`, inv.WorkspaceID, userID, inv.Role, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		http.Error(w, "failed to add member", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(`UPDATE workspace_invitations SET used_by = ?, used_at = ? WHERE id = ?`,
		userID, time.Now().UTC().Format(time.RFC3339), invitationID)
	if err != nil {
		http.Error(w, "failed to mark invitation as used", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "commit failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"workspace_id": inv.WorkspaceID})
}
