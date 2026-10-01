package novel

import (
	"fmt"
	"net/http"
	"strings"
)

// exportNovel writes a stable, plain-text representation so users can keep a
// local backup without requiring a document conversion dependency.
func (s *Server) exportNovel(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "txt" && format != "md" {
		writeError(w, http.StatusBadRequest, "INVALID_FORMAT", "仅支持 txt 或 md 导出")
		return
	}
	var title string
	if err := s.db.QueryRowContext(r.Context(), `SELECT title FROM novels WHERE id = ? AND deleted_at IS NULL`, r.PathValue("novelID")).Scan(&title); err != nil {
		writeError(w, http.StatusNotFound, "NOVEL_NOT_FOUND", "小说不存在")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT chapter_number, title, body FROM chapters WHERE novel_id = ? AND deleted_at IS NULL ORDER BY chapter_number`, r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	defer rows.Close()
	var output strings.Builder
	if format == "md" {
		output.WriteString("# " + title + "\n\n")
	} else {
		output.WriteString(title + "\r\n\r\n")
	}
	for rows.Next() {
		var number int
		var chapterTitle, body string
		if err := rows.Scan(&number, &chapterTitle, &body); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
			return
		}
		if format == "md" {
			fmt.Fprintf(&output, "## 第%d章 %s\n\n%s\n\n", number, chapterTitle, body)
		} else {
			fmt.Fprintf(&output, "第%d章 %s\r\n\r\n%s\r\n\r\n", number, chapterTitle, body)
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	ext := "txt"
	contentType := "text/plain; charset=utf-8"
	if format == "md" {
		ext = "md"
		contentType = "text/markdown; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="novel-%s.%s"`, r.PathValue("novelID"), ext))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(output.String()))
}
