package novel

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"strings"

	"novelstudio/internal/audit"
)

// exportNovel writes stable manuscript exports without requiring an external
// document conversion dependency.
func (s *Server) exportNovel(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID := r.PathValue("novelID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "txt" && format != "md" && format != "epub" && format != "docx" {
		writeError(w, http.StatusBadRequest, "INVALID_FORMAT", "仅支持 txt、md、epub 或 docx 导出")
		return
	}
	var title string
	if err := s.db.QueryRowContext(r.Context(), `SELECT title FROM novels WHERE id = ? AND deleted_at IS NULL`, novelID).Scan(&title); err != nil {
		writeError(w, http.StatusNotFound, "NOVEL_NOT_FOUND", "小说不存在")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT chapter_number, title, body FROM chapters WHERE novel_id = ? AND deleted_at IS NULL ORDER BY chapter_number`, novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取章节失败")
		return
	}
	defer rows.Close()
	if format == "epub" {
		_ = writeEPUBExport(w, r, title, rows)
		_ = audit.Log(r.Context(), s.db, userID, &novelID, audit.ActionExportNovel, audit.TargetTypeNovel, novelID, map[string]interface{}{
			"format":      format,
			"novel_title": title,
		})
		return
	}
	if format == "docx" {
		_ = writeDOCXExport(w, r, title, rows)
		_ = audit.Log(r.Context(), s.db, userID, &novelID, audit.ActionExportNovel, audit.TargetTypeNovel, novelID, map[string]interface{}{
			"format":      format,
			"novel_title": title,
		})
		return
	}
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
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="novel-%s.%s"`, novelID, ext))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(output.String()))
	_ = audit.Log(r.Context(), s.db, userID, &novelID, audit.ActionExportNovel, audit.TargetTypeNovel, novelID, map[string]interface{}{
		"format":      format,
		"novel_title": title,
	})
}

type epubChapter struct {
	number      int
	title, body string
}

func writeEPUBExport(w http.ResponseWriter, r *http.Request, title string, rows *sql.Rows) error {
	chapters := make([]epubChapter, 0)
	for rows.Next() {
		var chapter epubChapter
		if err := rows.Scan(&chapter.number, &chapter.title, &chapter.body); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "failed to read chapters")
			return err
		}
		chapters = append(chapters, chapter)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "failed to read chapters")
		return err
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	header := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	entry, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	if _, err = entry.Write([]byte("application/epub+zip")); err != nil {
		return err
	}
	container := `<?xml version="1.0" encoding="UTF-8"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	if err = addZipText(archive, "META-INF/container.xml", container); err != nil {
		return err
	}
	manifest := []string{`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`}
	spine, nav := make([]string, 0, len(chapters)), make([]string, 0, len(chapters))
	for i, chapter := range chapters {
		id := fmt.Sprintf("chapter-%d", i+1)
		href := id + ".xhtml"
		manifest = append(manifest, fmt.Sprintf(`<item id="%s" href="%s" media-type="application/xhtml+xml"/>`, id, href))
		spine = append(spine, fmt.Sprintf(`<itemref idref="%s"/>`, id))
		nav = append(nav, fmt.Sprintf(`<li><a href="%s">%s</a></li>`, href, html.EscapeString(chapter.title)))
		xhtml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>%s</title></head><body><h1>%s</h1><pre>%s</pre></body></html>`, html.EscapeString(chapter.title), html.EscapeString(chapter.title), html.EscapeString(chapter.body))
		if err = addZipText(archive, "OEBPS/"+href, xhtml); err != nil {
			return err
		}
	}
	navDoc := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>%s</title></head><body><nav epub:type="toc"><ol>%s</ol></nav></body></html>`, html.EscapeString(title), strings.Join(nav, ""))
	if err = addZipText(archive, "OEBPS/nav.xhtml", navDoc); err != nil {
		return err
	}
	opf := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="book-id" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="book-id">novelstudio</dc:identifier><dc:title>%s</dc:title><dc:language>zh</dc:language></metadata><manifest>%s</manifest><spine>%s</spine></package>`, html.EscapeString(title), strings.Join(manifest, ""), strings.Join(spine, ""))
	if err = addZipText(archive, "OEBPS/content.opf", opf); err != nil {
		return err
	}
	if err = archive.Close(); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/epub+zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="novel-%s.epub"`, r.PathValue("novelID")))
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(buffer.Bytes())
	return err
}

func addZipText(archive *zip.Writer, name, content string) error {
	entry, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = entry.Write([]byte(content))
	return err
}

func writeDOCXExport(w http.ResponseWriter, r *http.Request, title string, rows *sql.Rows) error {
	var document strings.Builder
	document.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	document.WriteString(docxParagraph(title, true))
	for rows.Next() {
		var number int
		var chapterTitle, body string
		if err := rows.Scan(&number, &chapterTitle, &body); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "failed to read chapters")
			return err
		}
		document.WriteString(docxParagraph(fmt.Sprintf("第%d章 %s", number, chapterTitle), true))
		for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
			document.WriteString(docxParagraph(line, false))
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "failed to read chapters")
		return err
	}
	document.WriteString(`</w:body></w:document>`)
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   document.String(),
	}
	for name, content := range files {
		if err := addZipText(archive, name, content); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="novel-%s.docx"`, r.PathValue("novelID")))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(buffer.Bytes())
	return err
}

func docxParagraph(text string, heading bool) string {
	style := ""
	if heading {
		style = `<w:pPr><w:pStyle w:val="Heading1"/></w:pPr>`
	}
	return fmt.Sprintf(`<w:p>%s<w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, style, html.EscapeString(text))
}
