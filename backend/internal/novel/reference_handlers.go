package novel

import (
	"net/http"
	"strings"

	"novelstudio/internal/audit"
	"novelstudio/internal/database"
)

type referenceDocument struct {
	ID         string `json:"id"`
	NovelID    string `json:"novelId"`
	Title      string `json:"title"`
	ChunkCount int    `json:"chunkCount"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

func (s *Server) listReferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT d.id, d.novel_id, d.title, COUNT(c.id), d.created_at, d.updated_at FROM reference_documents d LEFT JOIN reference_chunks c ON c.document_id = d.id WHERE d.novel_id = ? GROUP BY d.id ORDER BY d.updated_at DESC`, r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取参考资料失败")
		return
	}
	defer rows.Close()
	items := make([]referenceDocument, 0)
	for rows.Next() {
		var item referenceDocument
		if err := rows.Scan(&item.ID, &item.NovelID, &item.Title, &item.ChunkCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeError(w, 500, "DATABASE_ERROR", "读取参考资料失败")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取参考资料失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": items})
}

func (s *Server) createReference(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	var input struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	if input.Title == "" || input.Content == "" || len([]rune(input.Title)) > 160 || len([]rune(input.Content)) > 500000 {
		writeError(w, http.StatusBadRequest, "INVALID_REFERENCE", "参考资料标题或正文无效")
		return
	}
	id, now := database.NewID(), database.Now()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "保存参考资料失败")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO reference_documents (id, novel_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, id, novelID, input.Title, now, now); err == nil {
		runes := []rune(input.Content)
		chunkCount := 0
		for offset, number := 0, 1; offset < len(runes); number++ {
			end := offset + 1200
			if end > len(runes) {
				end = len(runes)
			}
			if _, err = tx.ExecContext(r.Context(), `INSERT INTO reference_chunks (id, document_id, chunk_number, content, created_at) VALUES (?, ?, ?, ?, ?)`, database.NewID(), id, number, string(runes[offset:end]), now); err != nil {
				break
			}
			chunkCount++
			offset = end
		}
		if err == nil {
			err = audit.LogTx(r.Context(), tx, userID, &novelID, audit.ActionImportReference, audit.TargetTypeReference, id, map[string]interface{}{
				"document_title": input.Title,
				"chunk_count":    chunkCount,
			})
		}
	}
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "保存参考资料失败")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, "DATABASE_ERROR", "保存参考资料失败")
		return
	}
	writeJSON(w, http.StatusCreated, referenceDocument{ID: id, NovelID: novelID, Title: input.Title, ChunkCount: (len([]rune(input.Content)) + 1199) / 1200, CreatedAt: now, UpdatedAt: now})
}

func (s *Server) searchReferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < 2 || len([]rune(query)) > 100 {
		writeError(w, 400, "INVALID_QUERY", "搜索词长度必须为 2 到 100 个字符")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT c.id, c.document_id, d.title, c.chunk_number, c.content FROM reference_chunks_fts f JOIN reference_chunks c ON c.rowid = f.rowid JOIN reference_documents d ON d.id = c.document_id WHERE d.novel_id = ? AND reference_chunks_fts MATCH ? ORDER BY bm25(reference_chunks_fts) LIMIT 20`, r.PathValue("novelID"), quoteFTSQuery(query))
	if err != nil {
		pattern := "%" + escapeLike(query) + "%"
		rows, err = s.db.QueryContext(r.Context(), `SELECT c.id, c.document_id, d.title, c.chunk_number, c.content FROM reference_chunks c JOIN reference_documents d ON d.id = c.document_id WHERE d.novel_id = ? AND c.content LIKE ? ESCAPE '\' ORDER BY c.chunk_number LIMIT 20`, r.PathValue("novelID"), pattern)
	}
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "搜索参考资料失败")
		return
	}
	defer rows.Close()
	results := make([]map[string]any, 0)
	for rows.Next() {
		var id, documentID, title, content string
		var number int
		if err := rows.Scan(&id, &documentID, &title, &number, &content); err != nil {
			writeError(w, 500, "DATABASE_ERROR", "搜索参考资料失败")
			return
		}
		results = append(results, map[string]any{"id": id, "documentId": documentID, "title": title, "chunkNumber": number, "content": content})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "搜索参考资料失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": query, "results": results})
}

func quoteFTSQuery(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func (s *Server) deleteReference(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	documentID := r.PathValue("documentID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "删除参考资料失败")
		return
	}
	defer tx.Rollback()
	var title string
	err = tx.QueryRowContext(r.Context(), `SELECT title FROM reference_documents WHERE id = ? AND novel_id = ?`, documentID, novelID).Scan(&title)
	if err != nil {
		writeError(w, http.StatusNotFound, "REFERENCE_NOT_FOUND", "参考资料不存在")
		return
	}
	result, err := tx.ExecContext(r.Context(), `DELETE FROM reference_documents WHERE id = ? AND novel_id = ?`, documentID, novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "删除参考资料失败")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeError(w, http.StatusNotFound, "REFERENCE_NOT_FOUND", "参考资料不存在")
		return
	}
	if err := audit.LogTx(r.Context(), tx, userID, &novelID, audit.ActionDeleteReference, audit.TargetTypeReference, documentID, map[string]interface{}{
		"document_title": title,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "记录审计日志失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "删除参考资料失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
