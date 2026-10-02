package novel

import (
	"database/sql"
	"net/http"
)

type usageTotals struct {
	PromptTokens        int     `json:"promptTokens"`
	CompletionTokens    int     `json:"completionTokens"`
	TotalTokens         int     `json:"totalTokens"`
	EstimatedCostUSD    float64 `json:"estimatedCostUsd"`
	ReportedTokenCalls  int     `json:"reportedTokenCalls"`
	EstimatedTokenCalls int     `json:"estimatedTokenCalls"`
	CostKnownCalls      int     `json:"costKnownCalls"`
	CostUnknownCalls    int     `json:"costUnknownCalls"`
	CostKnown           bool    `json:"costKnown"`
}

type novelUsage struct {
	usageTotals
	NovelID    string `json:"novelId"`
	NovelTitle string `json:"novelTitle"`
}

type userUsage struct {
	usageTotals
	UserID    string `json:"userId"`
	UserEmail string `json:"userEmail"`
}

type taskUsage struct {
	usageTotals
	TaskID     string `json:"taskId"`
	NovelID    string `json:"novelId"`
	NovelTitle string `json:"novelTitle"`
	UserID     string `json:"userId"`
	UserEmail  string `json:"userEmail"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
}

type usageScanner interface {
	Scan(...any) error
}

func (s *Server) adminUsage(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireLocalSettingsAdmin(w, userID) {
		return
	}

	var summary usageTotals
	if err := scanUsageTotals(s.db.QueryRowContext(r.Context(), `SELECT
		COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(estimated_cost_usd), 0), COALESCE(SUM(reported_token_calls), 0),
		COALESCE(SUM(estimated_token_calls), 0), COALESCE(SUM(cost_known_calls), 0),
		COALESCE(SUM(cost_unknown_calls), 0) FROM tasks`), &summary); err != nil {
		writeError(w, http.StatusInternalServerError, "USAGE_READ_FAILED", "读取模型用量失败")
		return
	}
	setUsageCostKnown(&summary)

	novels, err := s.readNovelUsage(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "USAGE_READ_FAILED", "读取作品用量失败")
		return
	}
	users, err := s.readUserUsage(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "USAGE_READ_FAILED", "读取用户用量失败")
		return
	}
	tasks, err := s.readTaskUsage(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "USAGE_READ_FAILED", "读取任务用量失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary": summary,
		"novels":  novels,
		"users":   users,
		"tasks":   tasks,
	})
}

func (s *Server) readNovelUsage(r *http.Request) ([]novelUsage, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT n.id, n.title,
		COALESCE(SUM(t.prompt_tokens), 0), COALESCE(SUM(t.completion_tokens), 0), COALESCE(SUM(t.total_tokens), 0),
		COALESCE(SUM(t.estimated_cost_usd), 0), COALESCE(SUM(t.reported_token_calls), 0),
		COALESCE(SUM(t.estimated_token_calls), 0), COALESCE(SUM(t.cost_known_calls), 0),
		COALESCE(SUM(t.cost_unknown_calls), 0)
		FROM novels n JOIN tasks t ON t.novel_id = n.id
		GROUP BY n.id, n.title HAVING SUM(t.total_tokens) > 0
		ORDER BY SUM(t.total_tokens) DESC, n.title COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]novelUsage, 0)
	for rows.Next() {
		var item novelUsage
		if err := rows.Scan(&item.NovelID, &item.NovelTitle,
			&item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.EstimatedCostUSD,
			&item.ReportedTokenCalls, &item.EstimatedTokenCalls, &item.CostKnownCalls, &item.CostUnknownCalls); err != nil {
			return nil, err
		}
		setUsageCostKnown(&item.usageTotals)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) readUserUsage(r *http.Request) ([]userUsage, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT u.id, u.email,
		COALESCE(SUM(t.prompt_tokens), 0), COALESCE(SUM(t.completion_tokens), 0), COALESCE(SUM(t.total_tokens), 0),
		COALESCE(SUM(t.estimated_cost_usd), 0), COALESCE(SUM(t.reported_token_calls), 0),
		COALESCE(SUM(t.estimated_token_calls), 0), COALESCE(SUM(t.cost_known_calls), 0),
		COALESCE(SUM(t.cost_unknown_calls), 0)
		FROM users u JOIN tasks t ON t.created_by = u.id
		GROUP BY u.id, u.email HAVING SUM(t.total_tokens) > 0
		ORDER BY SUM(t.total_tokens) DESC, u.email COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]userUsage, 0)
	for rows.Next() {
		var item userUsage
		if err := rows.Scan(&item.UserID, &item.UserEmail,
			&item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.EstimatedCostUSD,
			&item.ReportedTokenCalls, &item.EstimatedTokenCalls, &item.CostKnownCalls, &item.CostUnknownCalls); err != nil {
			return nil, err
		}
		setUsageCostKnown(&item.usageTotals)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) readTaskUsage(r *http.Request) ([]taskUsage, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT t.id, n.id, n.title, u.id, u.email, t.kind, t.status, t.created_at,
		t.prompt_tokens, t.completion_tokens, t.total_tokens, t.estimated_cost_usd,
		t.reported_token_calls, t.estimated_token_calls, t.cost_known_calls, t.cost_unknown_calls
		FROM tasks t JOIN novels n ON n.id = t.novel_id JOIN users u ON u.id = t.created_by
		WHERE t.total_tokens > 0 ORDER BY t.created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]taskUsage, 0)
	for rows.Next() {
		var item taskUsage
		if err := rows.Scan(&item.TaskID, &item.NovelID, &item.NovelTitle, &item.UserID, &item.UserEmail,
			&item.Kind, &item.Status, &item.CreatedAt,
			&item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.EstimatedCostUSD,
			&item.ReportedTokenCalls, &item.EstimatedTokenCalls, &item.CostKnownCalls, &item.CostUnknownCalls); err != nil {
			return nil, err
		}
		setUsageCostKnown(&item.usageTotals)
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanUsageTotals(row usageScanner, totals *usageTotals) error {
	return row.Scan(&totals.PromptTokens, &totals.CompletionTokens, &totals.TotalTokens,
		&totals.EstimatedCostUSD, &totals.ReportedTokenCalls, &totals.EstimatedTokenCalls,
		&totals.CostKnownCalls, &totals.CostUnknownCalls)
}

func setUsageCostKnown(totals *usageTotals) {
	totals.CostKnown = totals.CostKnownCalls > 0 && totals.CostUnknownCalls == 0
}

var _ usageScanner = (*sql.Row)(nil)
