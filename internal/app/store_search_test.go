package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchChaptersFTSExcerptUsesMatchingField(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "novelstudio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	if _, err := store.CreateChapter(localWorkspaceID, ChapterInput{
		Title:   "标题命中测试",
		Summary: "摘要命中测试",
		Content: "正文命中测试，后面还有一段较长的正文。",
	}); err != nil {
		t.Fatalf("create chapter: %v", err)
	}

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{name: "title", query: "标题命中", want: "标题命中"},
		{name: "summary", query: "摘要命中", want: "摘要命中"},
		{name: "content", query: "正文命中", want: "正文命中"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results, exists, err := store.SearchChapters(localWorkspaceID, tc.query)
			if err != nil {
				t.Fatalf("search %q: %v", tc.query, err)
			}
			if !exists || len(results) != 1 {
				t.Fatalf("search %q = exists %t, results %+v", tc.query, exists, results)
			}
			if !strings.Contains(results[0].Excerpt, tc.want) {
				t.Fatalf("search %q excerpt = %q, want it to contain %q", tc.query, results[0].Excerpt, tc.want)
			}
		})
	}
}
