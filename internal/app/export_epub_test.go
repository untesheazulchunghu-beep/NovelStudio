package app

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestBuildEPUBPackagesChaptersInNumberOrderWithEscapedText(t *testing.T) {
	workspace := Workspace{
		Project: Project{Title: "甲 & 乙 <测试>"},
		Chapters: []Chapter{
			{Number: 2, Title: "第二章", Content: "后来。"},
			{Number: 1, Title: "开端", Content: "起初 & 然后"},
		},
	}

	data, err := buildEPUB(workspace)
	if err != nil {
		t.Fatalf("buildEPUB returned error: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("EPUB is not a valid zip archive: %v", err)
	}

	parts := map[string]string{}
	for index, file := range reader.File {
		if index == 0 && file.Name != "mimetype" {
			t.Fatalf("first archive entry = %q, want mimetype", file.Name)
		}
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

	if parts["mimetype"] != "application/epub+zip" {
		t.Fatalf("mimetype = %q", parts["mimetype"])
	}
	for _, name := range []string{
		"META-INF/container.xml",
		"OEBPS/content.opf",
		"OEBPS/nav.xhtml",
		"OEBPS/styles.css",
		"OEBPS/text/title.xhtml",
		"OEBPS/text/chapter-0001.xhtml",
		"OEBPS/text/chapter-0002.xhtml",
	} {
		if _, ok := parts[name]; !ok {
			t.Fatalf("EPUB is missing %s", name)
		}
	}

	if got := parts["OEBPS/text/title.xhtml"]; !strings.Contains(got, "甲 &amp; 乙 &lt;测试&gt;") {
		t.Fatalf("title page does not escape the project title: %s", got)
	}
	if got := parts["OEBPS/text/chapter-0001.xhtml"]; !strings.Contains(got, "第 1 章 开端") || !strings.Contains(got, "起初 &amp; 然后") {
		t.Fatalf("first chapter file is not the number-1 chapter: %s", got)
	}
	opf := parts["OEBPS/content.opf"]
	first := strings.Index(opf, `<itemref idref="chapter-0001"/>`)
	second := strings.Index(opf, `<itemref idref="chapter-0002"/>`)
	if first < 0 || second < 0 || first > second {
		t.Fatalf("spine does not list chapters in order: %s", opf)
	}
	if !strings.Contains(opf, `<item id="chapter-0002" href="text/chapter-0002.xhtml"`) {
		t.Fatalf("manifest does not reference chapter-0002.xhtml: %s", opf)
	}
}
