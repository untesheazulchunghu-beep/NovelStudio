package app

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestBuildDOCXPackagesEscapedTextAndChapterOrder(t *testing.T) {
	workspace := Workspace{
		Project: Project{Title: "甲 & 乙 <测试>", Subtitle: "副标题"},
		Chapters: []Chapter{
			{Number: 2, Title: "第二章", Content: "后来。"},
			{Number: 1, Title: "开端", Content: "起初 & 然后"},
		},
	}

	data, err := buildDOCX(workspace)
	if err != nil {
		t.Fatalf("buildDOCX returned error: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("DOCX is not a valid zip archive: %v", err)
	}

	parts := map[string]string{}
	for _, file := range reader.File {
		entry, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		body, err := io.ReadAll(entry)
		_ = entry.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		parts[file.Name] = string(body)
	}

	for _, name := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"word/document.xml",
		"word/styles.xml",
		"word/_rels/document.xml.rels",
		"docProps/core.xml",
		"docProps/app.xml",
	} {
		if _, ok := parts[name]; !ok {
			t.Fatalf("DOCX is missing %s", name)
		}
	}

	document := parts["word/document.xml"]
	decoder := xml.NewDecoder(strings.NewReader(document))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("document.xml is not well-formed XML: %v", err)
		}
	}
	if !strings.Contains(document, "甲 &amp; 乙 &lt;测试&gt;") {
		t.Fatalf("document does not escape the project title: %s", document)
	}
	if !strings.Contains(document, "起初 &amp; 然后") {
		t.Fatalf("document does not escape chapter text: %s", document)
	}
	first := strings.Index(document, "第 1 章 开端")
	second := strings.Index(document, "第 2 章 第二章")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("document chapters are not ordered: %s", document)
	}
}
