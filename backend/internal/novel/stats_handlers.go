package novel

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type writingStat struct {
	Date         string `json:"date"`
	WordsAdded   int    `json:"wordsAdded"`
	WordsDeleted int    `json:"wordsDeleted"`
	NetWords     int    `json:"netWords"`
}

type writingGoal struct {
	DailyWordGoal int `json:"dailyWordGoal"`
}

// GET /api/v1/novels/{id}/stats?days=30
func (s *Server) getWritingStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	novelID := r.PathValue("novelID")

	// Check permission - any role can view stats
	if !s.requireNovelRole(w, novelID, userID, "owner", "editor", "reviewer", "viewer") {
		return
	}

	// Parse days parameter
	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	// Get stats for the last N days
	rows, err := s.db.Query(`
		SELECT date, words_added, words_deleted, net_words
		FROM writing_stats
		WHERE novel_id = ? AND date >= date('now', '-' || ? || ' days')
		ORDER BY date DESC
	`, novelID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取统计数据失败")
		return
	}
	defer rows.Close()

	var stats []writingStat
	for rows.Next() {
		var stat writingStat
		if err := rows.Scan(&stat.Date, &stat.WordsAdded, &stat.WordsDeleted, &stat.NetWords); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取统计数据失败")
			return
		}
		stats = append(stats, stat)
	}

	if stats == nil {
		stats = []writingStat{}
	}

	writeJSON(w, http.StatusOK, stats)
}

// GET /api/v1/novels/{id}/goal
func (s *Server) getWritingGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	novelID := r.PathValue("novelID")

	// Check permission - any role can view goal
	if !s.requireNovelRole(w, novelID, userID, "owner", "editor", "reviewer", "viewer") {
		return
	}

	var goal writingGoal
	err := s.db.QueryRow(`
		SELECT daily_word_goal FROM novels WHERE id = ? AND deleted_at IS NULL
	`, novelID).Scan(&goal.DailyWordGoal)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOVEL_NOT_FOUND", "小说不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取写作目标失败")
		return
	}

	writeJSON(w, http.StatusOK, goal)
}

// PUT /api/v1/novels/{id}/goal
func (s *Server) updateWritingGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	novelID := r.PathValue("novelID")

	// Check permission (owner or editor)
	if !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}

	var goal writingGoal
	if !decodeJSON(w, r, &goal) {
		return
	}

	if goal.DailyWordGoal < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_GOAL", "每日字数目标必须为非负数")
		return
	}

	_, err := s.db.Exec(`
		UPDATE novels SET daily_word_goal = ? WHERE id = ? AND deleted_at IS NULL
	`, goal.DailyWordGoal, novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "更新写作目标失败")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// updateWritingStats is called after saving chapter content
func (s *Server) updateWritingStats(tx *sql.Tx, novelID string, oldWordCount, newWordCount int) error {
	today := time.Now().UTC().Format("2006-01-02")

	wordsAdded := 0
	wordsDeleted := 0
	netWords := newWordCount - oldWordCount

	if netWords > 0 {
		wordsAdded = netWords
	} else {
		wordsDeleted = -netWords
	}

	_, err := tx.Exec(`
		INSERT INTO writing_stats (novel_id, date, words_added, words_deleted, net_words)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(novel_id, date) DO UPDATE SET
			words_added = words_added + excluded.words_added,
			words_deleted = words_deleted + excluded.words_deleted,
			net_words = net_words + excluded.net_words
	`, novelID, today, wordsAdded, wordsDeleted, netWords)

	return err
}

// countWords counts non-whitespace characters in text (simplified word count for Chinese/Japanese)
func countWords(text string) int {
	count := 0
	for _, r := range text {
		if r > ' ' && r != '\t' && r != '\n' && r != '\r' {
			count++
		}
	}
	return count
}
