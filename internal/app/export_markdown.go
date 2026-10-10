package app

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func (s *Server) handleWorkspaceMarkdownExport(w http.ResponseWriter, r *http.Request, workspaceID string) {
	workspace, ok := s.documentExportSnapshot(w, r, workspaceID)
	if !ok {
		return
	}
	markdown := buildMarkdown(workspace)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="novelstudio.md"`)
	w.Header().Set("Content-Length", fmt.Sprint(len(markdown)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(markdown))
}

func buildMarkdown(workspace Workspace) string {
	chapters := append([]Chapter(nil), workspace.Chapters...)
	sort.SliceStable(chapters, func(i, j int) bool { return chapters[i].Number < chapters[j].Number })
	sections := []string{"# " + workspace.Project.Title}
	if subtitle := strings.TrimSpace(workspace.Project.Subtitle); subtitle != "" {
		sections = append(sections, subtitle)
	}
	for _, chapter := range chapters {
		title := strings.TrimSpace(chapter.Title)
		if title == "" {
			title = "未命名章节"
		}
		sections = append(sections, fmt.Sprintf("## 第 %d 章 %s\n\n%s", chapter.Number, title, chapter.Content))
	}
	return strings.Join(sections, "\n\n") + "\n"
}
