package app

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

const docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

type documentExportRequest struct {
	ChapterID      int64  `json:"chapter_id"`
	ChapterTitle   string `json:"chapter_title"`
	ChapterContent string `json:"chapter_content"`
}

func (s *Server) documentExportSnapshot(w http.ResponseWriter, r *http.Request, workspaceID string) (Workspace, bool) {
	var input documentExportRequest
	if err := readJSONLimit(r, &input, maxJSONRequestBytes); err != nil {
		writeJSONRequestError(w, err)
		return Workspace{}, false
	}
	if input.ChapterID < 0 || input.ChapterID == 0 && (input.ChapterTitle != "" || input.ChapterContent != "") {
		writeError(w, http.StatusBadRequest, "invalid current chapter")
		return Workspace{}, false
	}

	workspace, exists := s.store.Snapshot(workspaceID)
	if !exists {
		writeError(w, http.StatusNotFound, "workspace not found")
		return Workspace{}, false
	}
	if input.ChapterID > 0 {
		found := false
		for index := range workspace.Chapters {
			if workspace.Chapters[index].ID != input.ChapterID {
				continue
			}
			workspace.Chapters[index].Title = input.ChapterTitle
			workspace.Chapters[index].Content = input.ChapterContent
			found = true
			break
		}
		if !found {
			writeError(w, http.StatusNotFound, "chapter not found")
			return Workspace{}, false
		}
	}
	return workspace, true
}

func (s *Server) handleWorkspaceDocxExport(w http.ResponseWriter, r *http.Request, workspaceID string) {
	workspace, ok := s.documentExportSnapshot(w, r, workspaceID)
	if !ok {
		return
	}
	document, err := buildDOCX(workspace)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", docxContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="novelstudio.docx"`)
	w.Header().Set("Content-Length", fmt.Sprint(len(document)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

func buildDOCX(workspace Workspace) ([]byte, error) {
	var document strings.Builder
	document.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	if err := appendDocxTextBlock(&document, workspace.Project.Title, "Title"); err != nil {
		return nil, fmt.Errorf("project title cannot be written to DOCX: %w", err)
	}
	if subtitle := strings.TrimSpace(workspace.Project.Subtitle); subtitle != "" {
		if err := appendDocxTextBlock(&document, subtitle, "Subtitle"); err != nil {
			return nil, fmt.Errorf("project subtitle cannot be written to DOCX: %w", err)
		}
	}

	chapters := append([]Chapter(nil), workspace.Chapters...)
	sort.SliceStable(chapters, func(i, j int) bool { return chapters[i].Number < chapters[j].Number })
	for _, chapter := range chapters {
		title := strings.NewReplacer("\r", " ", "\n", " ").Replace(chapter.Title)
		heading := fmt.Sprintf("第 %d 章 %s", chapter.Number, title)
		if err := appendDocxParagraph(&document, heading, "Heading1"); err != nil {
			return nil, fmt.Errorf("chapter %d title cannot be written to DOCX: %w", chapter.Number, err)
		}
		for _, paragraph := range docxParagraphs(chapter.Content) {
			if err := appendDocxParagraph(&document, paragraph, "Normal"); err != nil {
				return nil, fmt.Errorf("chapter %d contains text that cannot be written to DOCX: %w", chapter.Number, err)
			}
		}
	}
	document.WriteString(`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr></w:body></w:document>`)

	coreProperties := strings.Builder{}
	coreProperties.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>`)
	if err := xml.EscapeText(&coreProperties, []byte(workspace.Project.Title)); err != nil {
		return nil, fmt.Errorf("project title cannot be written to DOCX metadata: %w", err)
	}
	coreProperties.WriteString(`</dc:title><dc:creator>NovelStudio</dc:creator></cp:coreProperties>`)

	files := []struct {
		name string
		body string
	}{
		{"[Content_Types].xml", docxContentTypes},
		{"_rels/.rels", docxPackageRelationships},
		{"word/document.xml", document.String()},
		{"word/styles.xml", docxStyles},
		{"word/_rels/document.xml.rels", docxDocumentRelationships},
		{"docProps/core.xml", coreProperties.String()},
		{"docProps/app.xml", docxAppProperties},
	}

	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, file := range files {
		entry, err := archive.Create(file.name)
		if err != nil {
			_ = archive.Close()
			return nil, fmt.Errorf("create DOCX part %s: %w", file.name, err)
		}
		if _, err := io.WriteString(entry, file.body); err != nil {
			_ = archive.Close()
			return nil, fmt.Errorf("write DOCX part %s: %w", file.name, err)
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("finish DOCX archive: %w", err)
	}
	return output.Bytes(), nil
}

func docxParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

func appendDocxTextBlock(document *strings.Builder, text, style string) error {
	for _, paragraph := range docxParagraphs(text) {
		if err := appendDocxParagraph(document, paragraph, style); err != nil {
			return err
		}
	}
	return nil
}

func appendDocxParagraph(document *strings.Builder, text, style string) error {
	document.WriteString(`<w:p><w:pPr><w:pStyle w:val="`)
	document.WriteString(style)
	document.WriteString(`"/></w:pPr><w:r><w:t xml:space="preserve">`)
	if err := xml.EscapeText(document, []byte(text)); err != nil {
		return err
	}
	document.WriteString(`</w:t></w:r></w:p>`)
	return nil
}

const docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`

const docxPackageRelationships = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`

const docxDocumentRelationships = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`

const docxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Aptos" w:eastAsia="宋体" w:hAnsi="Aptos" w:cs="Aptos"/><w:lang w:val="zh-CN" w:eastAsia="zh-CN"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="160" w:line="360" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:jc w:val="center"/><w:spacing w:after="280"/></w:pPr><w:rPr><w:b/><w:sz w:val="36"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Subtitle"><w:name w:val="Subtitle"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:spacing w:after="480"/></w:pPr><w:rPr><w:color w:val="666666"/><w:sz w:val="24"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="360" w:after="160"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:color w:val="334E68"/><w:sz w:val="28"/></w:rPr></w:style></w:styles>`

const docxAppProperties = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>NovelStudio</Application></Properties>`
