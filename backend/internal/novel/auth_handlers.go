package novel

import (
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"novelstudio/internal/database"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userSummary struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = normalizeEmail(input.Email)
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Password) < 10 || len(input.Password) > 72 {
		writeError(w, http.StatusBadRequest, "INVALID_CREDENTIALS", "请输入有效邮箱和 10–72 位密码")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUTH_ERROR", "创建账户失败")
		return
	}
	userID := database.NewID()
	workspaceID := database.NewID()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建账户失败")
		return
	}
	createdAt := database.Now()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`, userID, input.Email, string(passwordHash), createdAt)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO workspaces (id, name, owner_id, created_at) VALUES (?, ?, ?, ?)`, workspaceID, "我的创作空间", userID, createdAt)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)`, workspaceID, userID, createdAt)
	}
	if err != nil {
		tx.Rollback()
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "EMAIL_EXISTS", "该邮箱已注册")
			return
		}
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建账户失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建账户失败")
		return
	}
	s.sessions.SetVersion(w, userID, 1)
	writeJSON(w, http.StatusCreated, map[string]any{"user": userSummary{ID: userID, Email: input.Email}})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = normalizeEmail(input.Email)
	var user userSummary
	var passwordHash string
	err := s.db.QueryRowContext(r.Context(), `SELECT id, email, password_hash FROM users WHERE email = ?`, input.Email).Scan(&user.ID, &user.Email, &passwordHash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "INVALID_LOGIN", "邮箱或密码不正确")
		return
	}
	var version int64
	_ = s.db.QueryRowContext(r.Context(), `SELECT session_version FROM users WHERE id = ?`, user.ID).Scan(&version)
	s.sessions.SetVersion(w, user.ID, version)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if userID, _, ok := s.sessions.UserIDVersion(r); ok {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE users SET session_version = session_version + 1 WHERE id = ?`, userID)
	}
	s.sessions.Clear(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	var user userSummary
	if err := s.db.QueryRowContext(r.Context(), `SELECT id, email FROM users WHERE id = ?`, userID).Scan(&user.ID, &user.Email); err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "账户不存在")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT w.id, w.name, wm.role FROM workspaces w JOIN workspace_members wm ON wm.workspace_id = w.id WHERE wm.user_id = ? ORDER BY w.created_at`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取工作区失败")
		return
	}
	defer rows.Close()
	workspaces := make([]map[string]string, 0)
	for rows.Next() {
		var workspace map[string]string
		var id, name, role string
		if err := rows.Scan(&id, &name, &role); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取工作区失败")
			return
		}
		workspace = map[string]string{"id": id, "name": name, "role": role}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取工作区失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "workspaces": workspaces})
}

func (s *Server) workspaces(w http.ResponseWriter, r *http.Request) {
	s.me(w, r)
}

type memberInput struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	workspaceID := r.PathValue("workspaceID")
	var role string
	if err := s.db.QueryRowContext(r.Context(), `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&role); err != nil {
		writeError(w, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "工作区不存在或无权访问")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT u.id, u.email, wm.role FROM workspace_members wm JOIN users u ON u.id = wm.user_id WHERE wm.workspace_id = ? ORDER BY CASE wm.role WHEN 'owner' THEN 0 ELSE 1 END, u.email`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取成员失败")
		return
	}
	defer rows.Close()
	members := make([]map[string]string, 0)
	for rows.Next() {
		var member map[string]string
		var id, email, memberRole string
		if err := rows.Scan(&id, &email, &memberRole); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取成员失败")
			return
		}
		member = map[string]string{"id": id, "email": email, "role": memberRole}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取成员失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	workspaceID := r.PathValue("workspaceID")
	if !s.requireWorkspaceOwner(w, r, workspaceID, userID) {
		return
	}
	var input memberInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = normalizeEmail(input.Email)
	if !validAssignableRole(input.Role) {
		writeError(w, http.StatusBadRequest, "INVALID_ROLE", "请选择 editor、reviewer 或 viewer 角色")
		return
	}
	var memberID string
	if err := s.db.QueryRowContext(r.Context(), `SELECT id FROM users WHERE email = ?`, input.Email).Scan(&memberID); err != nil {
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", "请先让该成员注册账户，再添加到工作区")
		return
	}
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, ?, ?) ON CONFLICT(workspace_id, user_id) DO UPDATE SET role = CASE WHEN workspace_members.role = 'owner' THEN 'owner' ELSE excluded.role END`, workspaceID, memberID, input.Role, database.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "添加成员失败")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"userId": memberID, "email": input.Email, "role": input.Role})
}

func (s *Server) updateMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	workspaceID := r.PathValue("workspaceID")
	if !s.requireWorkspaceOwner(w, r, workspaceID, userID) {
		return
	}
	var input struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validAssignableRole(input.Role) {
		writeError(w, http.StatusBadRequest, "INVALID_ROLE", "请选择 editor、reviewer 或 viewer 角色")
		return
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE workspace_members SET role = ? WHERE workspace_id = ? AND user_id = ? AND role <> 'owner'`, input.Role, workspaceID, r.PathValue("userID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "更新成员角色失败")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeError(w, http.StatusNotFound, "MEMBER_NOT_FOUND", "成员不存在或不能修改所有者角色")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	workspaceID := r.PathValue("workspaceID")
	if !s.requireWorkspaceOwner(w, r, workspaceID, userID) {
		return
	}
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ? AND role <> 'owner'`, workspaceID, r.PathValue("userID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "移除成员失败")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeError(w, http.StatusNotFound, "MEMBER_NOT_FOUND", "成员不存在或不能移除所有者")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) requireWorkspaceOwner(w http.ResponseWriter, r *http.Request, workspaceID, userID string) bool {
	var role string
	err := s.db.QueryRowContext(r.Context(), `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "工作区不存在或无权访问")
		return false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取工作区权限失败")
		return false
	}
	if role != "owner" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "只有工作区所有者可以管理成员")
		return false
	}
	return true
}

func validAssignableRole(role string) bool {
	return role == "editor" || role == "reviewer" || role == "viewer"
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	var input struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.NewPassword) < 10 || len(input.NewPassword) > 72 {
		writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", "新密码长度必须为 10–72 位")
		return
	}
	var currentHash string
	var sessionVersion int64
	if err := s.db.QueryRowContext(r.Context(), `SELECT password_hash, session_version FROM users WHERE id = ?`, userID).Scan(&currentHash, &sessionVersion); err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "账户不存在")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(input.OldPassword)); err != nil {
		writeError(w, http.StatusUnauthorized, "INVALID_PASSWORD", "当前密码不正确")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUTH_ERROR", "修改密码失败")
		return
	}
	sessionVersion++
	_, err = s.db.ExecContext(r.Context(), `UPDATE users SET password_hash = ?, session_version = ? WHERE id = ?`, string(newHash), sessionVersion, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "修改密码失败")
		return
	}
	s.sessions.SetVersion(w, userID, sessionVersion)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
