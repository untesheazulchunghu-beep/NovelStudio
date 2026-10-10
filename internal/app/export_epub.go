package app

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (s *Server) handleWorkspaceEPUBExport(w http.ResponseWriter, r *http.Request, workspaceID string) {
	workspace, ok := s.documentExportSnapshot(w, r, workspaceID)
	if !ok {
		return
	}
	document, err := buildEPUB(workspace)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/epub+zip")
	w.Header().Set("Content-Disposition", `attachment; filename="novelstudio.epub"`)
	w.Header().Set("Content-Length", fmt.Sprint(len(document)))
	w.WriteHeader(200)
	_, _ = w.Write(document)
}

func buildEPUB(workspace Workspace) ([]byte, error) {
	chapters := append([]Chapter(nil), workspace.Chapters...)
	sort.SliceStable(chapters, func(i, j int) bool { return chapters[i].Number < chapters[j].Number })
	chapterNames := make([]string, len(chapters))
	for i := range chapters {
		chapterNames[i] = fmt.Sprintf("chapter-%04d.xhtml", i+1)
	}

	titlePage, err := buildEPUBTitlePage(workspace.Project)
	if err != nil {
		return nil, fmt.Errorf("project title cannot be written to EPUB: %w", err)
	}
	navigation, err := buildEPUBNavigation(workspace.Project.Title, chapters, chapterNames)
	if err != nil {
		return nil, fmt.Errorf("project navigation cannot be written to EPUB: %w", err)
	}
	packageDocument, err := buildEPUBPackage(workspace, chapterNames)
	if err != nil {
		return nil, err
	}

	files := []epubPart{
		{name: "META-INF/container.xml", body: epubContainerXML},
		{name: "OEBPS/content.opf", body: packageDocument},
		{name: "OEBPS/nav.xhtml", body: navigation},
		{name: "OEBPS/styles.css", body: epubStyles},
		{name: "OEBPS/text/title.xhtml", body: titlePage},
	}
	for i, chapter := range chapters {
		chapterDocument, err := buildEPUBChapter(chapter)
		if err != nil {
			return nil, fmt.Errorf("chapter %d cannot be written to EPUB: %w", chapter.Number, err)
		}
		files = append(files, epubPart{
			name: "OEBPS/text/" + chapterNames[i],
			body: chapterDocument,
		})
	}

	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	mimetypeContent := []byte("application/epub+zip")
	mimetype, err := archive.CreateHeader(&zip.FileHeader{
		Name:               "mimetype",
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE(mimetypeContent),
		CompressedSize:     uint32(len(mimetypeContent)),
		UncompressedSize:   uint32(len(mimetypeContent)),
		CompressedSize64:   uint64(len(mimetypeContent)),
		UncompressedSize64: uint64(len(mimetypeContent)),
	})
	if err != nil {
		_ = archive.Close()
		return nil, fmt.Errorf("create EPUB mimetype: %w", err)
	}
	if _, err := mimetype.Write(mimetypeContent); err != nil {
		_ = archive.Close()
		return nil, fmt.Errorf("write EPUB mimetype: %w", err)
	}
	for _, file := range files {
		entry, err := archive.Create(file.name)
		if err != nil {
			_ = archive.Close()
			return nil, fmt.Errorf("create EPUB part %s: %w", file.name, err)
		}
		if _, err := io.WriteString(entry, file.body); err != nil {
			_ = archive.Close()
			return nil, fmt.Errorf("write EPUB part %s: %w", file.name, err)
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("finish EPUB archive: %w", err)
	}
	return output.Bytes(), nil
}

type epubPart struct {
	name string
	body string
}

func buildEPUBTitlePage(project Project) (string, error) {
	var page strings.Builder
	if err := writeEPUBPageStart(&page, project.Title, "../styles.css"); err != nil {
		return "", err
	}
	page.WriteString(`<body><section epub:type="titlepage"><h1>`)
	if err := xml.EscapeText(&page, []byte(project.Title)); err != nil {
		return "", err
	}
	page.WriteString(`</h1>`)
	if subtitle := strings.TrimSpace(project.Subtitle); subtitle != "" {
		page.WriteString(`<div class="subtitle">`)
		for _, paragraph := range docxParagraphs(subtitle) {
			page.WriteString("<p>")
			if err := xml.EscapeText(&page, []byte(paragraph)); err != nil {
				return "", err
			}
			page.WriteString("</p>")
		}
		page.WriteString("</div>")
	}
	page.WriteString(`</section></body></html>`)
	return page.String(), nil
}

func buildEPUBNavigation(title string, chapters []Chapter, chapterNames []string) (string, error) {
	var page strings.Builder
	if err := writeEPUBPageStart(&page, "目录", "styles.css"); err != nil {
		return "", err
	}
	page.WriteString(`<body><nav epub:type="toc" id="toc"><h1>目录</h1><ol><li><a href="text/title.xhtml">`)
	if err := xml.EscapeText(&page, []byte(title)); err != nil {
		return "", err
	}
	page.WriteString("</a></li>")
	for i, chapter := range chapters {
		page.WriteString(`<li><a href="text/` + chapterNames[i] + `">`)
		heading := fmt.Sprintf("第 %d 章 %s", chapter.Number, strings.NewReplacer("\r", " ", "\n", " ").Replace(chapter.Title))
		if err := xml.EscapeText(&page, []byte(heading)); err != nil {
			return "", err
		}
		page.WriteString("</a></li>")
	}
	page.WriteString(`</ol></nav></body></html>`)
	return page.String(), nil
}

func buildEPUBChapter(chapter Chapter) (string, error) {
	var page strings.Builder
	heading := fmt.Sprintf("第 %d 章 %s", chapter.Number, strings.NewReplacer("\r", " ", "\n", " ").Replace(chapter.Title))
	if err := writeEPUBPageStart(&page, heading, "../styles.css"); err != nil {
		return "", err
	}
	page.WriteString(`<body><section epub:type="chapter"><h1>`)
	if err := xml.EscapeText(&page, []byte(heading)); err != nil {
		return "", err
	}
	page.WriteString("</h1>")
	for _, paragraph := range docxParagraphs(chapter.Content) {
		page.WriteString("<p>")
		if err := xml.EscapeText(&page, []byte(paragraph)); err != nil {
			return "", err
		}
		page.WriteString("</p>")
	}
	page.WriteString(`</section></body></html>`)
	return page.String(), nil
}

func writeEPUBPageStart(page *strings.Builder, title, stylesheet string) error {
	page.WriteString(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="zh-CN" lang="zh-CN"><head><title>`)
	if err := xml.EscapeText(page, []byte(title)); err != nil {
		return err
	}
	page.WriteString(`</title><link rel="stylesheet" type="text/css" href="` + stylesheet + `"/></head>`)
	return nil
}

func buildEPUBPackage(workspace Workspace, chapterNames []string) (string, error) {
	identifier, err := epubIdentifier()
	if err != nil {
		return "", fmt.Errorf("create EPUB identifier: %w", err)
	}
	var packageDocument strings.Builder
	packageDocument.WriteString(`<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id" xml:lang="zh-CN"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="pub-id">`)
	packageDocument.WriteString(identifier)
	packageDocument.WriteString(`</dc:identifier><dc:title>`)
	if err := xml.EscapeText(&packageDocument, []byte(workspace.Project.Title)); err != nil {
		return "", fmt.Errorf("project title cannot be written to EPUB metadata: %w", err)
	}
	packageDocument.WriteString(`</dc:title><dc:language>zh-CN</dc:language><dc:creator>NovelStudio</dc:creator><meta property="dcterms:modified">`)
	packageDocument.WriteString(time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	packageDocument.WriteString(`</meta></metadata><manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="css" href="styles.css" media-type="text/css"/><item id="title-page" href="text/title.xhtml" media-type="application/xhtml+xml"/>`)
	for i, chapterName := range chapterNames {
		packageDocument.WriteString(fmt.Sprintf(`<item id="chapter-%04d" href="text/%s" media-type="application/xhtml+xml"/>`, i+1, chapterName))
	}
	packageDocument.WriteString(`</manifest><spine><itemref idref="title-page"/>`)
	for i := range chapterNames {
		packageDocument.WriteString(fmt.Sprintf(`<itemref idref="chapter-%04d"/>`, i+1))
	}
	packageDocument.WriteString(`</spine></package>`)
	return packageDocument.String(), nil
}

func epubIdentifier() (string, error) {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "", err
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

const epubContainerXML = `<?xml version="1.0" encoding="UTF-8"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`

const epubStyles = `body{font-family:serif;line-height:1.8;margin:1em}h1{text-align:center;line-height:1.4;margin:1.5em 0 1em}.subtitle{text-align:center;color:#555;margin-top:2em}p{white-space:pre-wrap;text-indent:2em;margin:.35em 0}`
