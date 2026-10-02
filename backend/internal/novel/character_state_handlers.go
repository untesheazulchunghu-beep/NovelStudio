package novel

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"novelstudio/internal/database"
)

type CharacterState struct {
	ID             string `json:"id"`
	NovelID        string `json:"novelId"`
	CharacterID    string `json:"characterId"`
	CharacterLabel string `json:"characterLabel"`
	ChapterID      string `json:"chapterId"`
	ChapterNumber  int    `json:"chapterNumber"`
	ChapterTitle   string `json:"chapterTitle"`
	Appellation    string `json:"appellation"`
	Age            string `json:"age"`
	Abilities      string `json:"abilities"`
	Stance         string `json:"stance"`
	Notes          string `json:"notes"`
	UpdatedAt      string `json:"updatedAt"`
}

type characterStateInput struct {
	ChapterID   string `json:"chapterId"`
	Appellation string `json:"appellation"`
	Age         string `json:"age"`
	Abilities   string `json:"abilities"`
	Stance      string `json:"stance"`
	Notes       string `json:"notes"`
}

func loadCharacterStates(r *http.Request, db *sql.DB, novelID string) ([]CharacterState, error) {
	rows, err := db.QueryContext(r.Context(), `SELECT state.id, state.novel_id, state.character_id, node.label,
		state.chapter_id, chapter.chapter_number, chapter.title, state.appellation, state.age,
		state.abilities, state.stance, state.notes, state.updated_at
		FROM character_states state
		JOIN graph_nodes node ON node.id = state.character_id AND node.novel_id = state.novel_id AND node.kind = 'character'
		JOIN chapters chapter ON chapter.id = state.chapter_id AND chapter.novel_id = state.novel_id
		WHERE state.novel_id = ? AND chapter.deleted_at IS NULL
		ORDER BY node.label, chapter.chapter_number, state.id`, novelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make([]CharacterState, 0)
	for rows.Next() {
		var state CharacterState
		if err := rows.Scan(&state.ID, &state.NovelID, &state.CharacterID, &state.CharacterLabel,
			&state.ChapterID, &state.ChapterNumber, &state.ChapterTitle, &state.Appellation, &state.Age,
			&state.Abilities, &state.Stance, &state.Notes, &state.UpdatedAt); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (s *Server) upsertCharacterState(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	characterID := r.PathValue("nodeID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	var input characterStateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ChapterID = strings.TrimSpace(input.ChapterID)
	input.Appellation = strings.TrimSpace(input.Appellation)
	input.Age = strings.TrimSpace(input.Age)
	input.Abilities = strings.TrimSpace(input.Abilities)
	input.Stance = strings.TrimSpace(input.Stance)
	input.Notes = strings.TrimSpace(input.Notes)
	if input.ChapterID == "" || tooLong(input.Appellation, 160) || tooLong(input.Age, 80) ||
		tooLong(input.Abilities, 1200) || tooLong(input.Stance, 1200) || tooLong(input.Notes, 2000) {
		writeError(w, http.StatusBadRequest, "INVALID_CHARACTER_STATE", "请选择章节，并将称谓、年龄、能力、立场和补充说明控制在长度限制内")
		return
	}
	if input.Appellation == "" && input.Age == "" && input.Abilities == "" && input.Stance == "" && input.Notes == "" {
		writeError(w, http.StatusBadRequest, "INVALID_CHARACTER_STATE", "至少填写一项人物状态")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存人物状态失败")
		return
	}
	defer tx.Rollback()
	var kind string
	err = tx.QueryRowContext(r.Context(), `SELECT kind FROM graph_nodes WHERE id = ? AND novel_id = ?`, characterID, novelID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NODE_NOT_FOUND", "人物实体不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取人物实体失败")
		return
	}
	if kind != "character" {
		writeError(w, http.StatusBadRequest, "NOT_A_CHARACTER", "人物状态只能关联人物实体")
		return
	}
	var chapterNumber int
	err = tx.QueryRowContext(r.Context(), `SELECT chapter_number FROM chapters WHERE id = ? AND novel_id = ? AND deleted_at IS NULL`, input.ChapterID, novelID).Scan(&chapterNumber)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "INVALID_CHAPTER", "章节不存在或不属于当前小说")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取人物状态章节失败")
		return
	}
	now := database.Now()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO character_states (
		id, novel_id, character_id, chapter_id, appellation, age, abilities, stance, notes, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(character_id, chapter_id) DO UPDATE SET
		appellation = excluded.appellation, age = excluded.age, abilities = excluded.abilities,
		stance = excluded.stance, notes = excluded.notes, updated_at = excluded.updated_at`,
		database.NewID(), novelID, characterID, input.ChapterID, input.Appellation, input.Age,
		input.Abilities, input.Stance, input.Notes, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存人物状态失败")
		return
	}
	var state CharacterState
	err = tx.QueryRowContext(r.Context(), `SELECT state.id, state.novel_id, state.character_id, node.label,
		state.chapter_id, chapter.chapter_number, chapter.title, state.appellation, state.age,
		state.abilities, state.stance, state.notes, state.updated_at
		FROM character_states state
		JOIN graph_nodes node ON node.id = state.character_id AND node.novel_id = state.novel_id AND node.kind = 'character'
		JOIN chapters chapter ON chapter.id = state.chapter_id AND chapter.novel_id = state.novel_id AND chapter.deleted_at IS NULL
		WHERE state.character_id = ? AND state.chapter_id = ? AND state.novel_id = ?`, characterID, input.ChapterID, novelID).
		Scan(&state.ID, &state.NovelID, &state.CharacterID, &state.CharacterLabel,
			&state.ChapterID, &state.ChapterNumber, &state.ChapterTitle, &state.Appellation, &state.Age,
			&state.Abilities, &state.Stance, &state.Notes, &state.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取已保存的人物状态失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存人物状态失败")
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) deleteCharacterState(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	characterID := r.PathValue("nodeID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM character_states WHERE id = ? AND novel_id = ? AND character_id = ?`,
		r.PathValue("stateID"), novelID, characterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "删除人物状态失败")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, http.StatusNotFound, "CHARACTER_STATE_NOT_FOUND", "人物状态记录不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func tooLong(value string, max int) bool {
	return len([]rune(value)) > max
}
