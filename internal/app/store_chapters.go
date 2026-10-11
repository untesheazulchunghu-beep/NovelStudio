package app

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

func chapterSummary(chapter Chapter) ChapterSummary {
	return ChapterSummary{
		ID: chapter.ID, Number: chapter.Number, Title: chapter.Title, Summary: chapter.Summary,
		Status: chapter.Status, WordCount: chapter.WordCount, LastEditedAt: chapter.LastEditedAt,
	}
}

func (s *Store) GetChapter(workspaceID string, chapterID int64) (Chapter, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); errors.Is(err, os.ErrNotExist) {
		return Chapter{}, false, nil
	} else if err != nil {
		return Chapter{}, false, err
	}
	_, exists := s.workspaces[workspaceID]
	if !exists {
		return Chapter{}, false, nil
	}
	chapter, err := loadChapter(s.db, workspaceID, chapterID)
	if errors.Is(err, os.ErrNotExist) {
		return Chapter{}, true, os.ErrNotExist
	}
	return chapter, true, err
}

func (s *Store) UpdateChapter(workspaceID string, id int64, input ChapterInput) (Chapter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return Chapter{}, os.ErrNotExist
	}
	workspace = cloneWorkspace(workspace)
	for index := range workspace.Chapters {
		if workspace.Chapters[index].ID != id {
			continue
		}
		chapter := &workspace.Chapters[index]
		if title := strings.TrimSpace(input.Title); title != "" && title != strings.TrimSpace(chapter.Title) {
			if err := validateTextLength("chapter title", title, maxChapterTitleLength); err != nil {
				return Chapter{}, fmt.Errorf("%w: %v", ErrInvalidChapterInput, err)
			}
		}
		if input.Summary != chapter.Summary {
			if err := validateTextLength("chapter summary", input.Summary, maxChapterSummaryLength); err != nil {
				return Chapter{}, fmt.Errorf("%w: %v", ErrInvalidChapterInput, err)
			}
		}
		if strings.TrimSpace(input.Title) != "" {
			chapter.Title = strings.TrimSpace(input.Title)
		}
		chapter.Summary, chapter.Content = input.Summary, input.Content
		if input.Status != "" {
			chapter.Status = normalizeChapterStatus(input.Status)
		}
		chapter.WordCount, chapter.LastEditedAt = countWords(chapter.Content), now()
		touchProject(&workspace)
		if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
			return Chapter{}, err
		}
		return chapterCopy(*chapter), nil
	}
	return Chapter{}, os.ErrNotExist
}

func (s *Store) CreateChapter(workspaceID string, input ChapterInput) (Chapter, error) {
	if err := validateChapterTextFields(input); err != nil {
		return Chapter{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return Chapter{}, os.ErrNotExist
	}
	workspace = cloneWorkspace(workspace)
	number := len(workspace.Chapters) + 1
	for _, chapter := range workspace.Chapters {
		if chapter.Number >= number {
			number = chapter.Number + 1
		}
	}
	status := "outline"
	if strings.TrimSpace(input.Status) != "" {
		status = normalizeChapterStatus(input.Status)
	}
	chapter := Chapter{ID: s.nextIDLocked(), Number: number, Title: strings.TrimSpace(input.Title), Summary: input.Summary, Content: input.Content, Status: status, WordCount: countWords(input.Content), LastEditedAt: now()}
	if chapter.Title == "" {
		chapter.Title = fmt.Sprintf("未命名章节 %02d", number)
	}
	workspace.Chapters = append(workspace.Chapters, chapter)
	sort.SliceStable(workspace.Chapters, func(i, j int) bool { return workspace.Chapters[i].Number < workspace.Chapters[j].Number })
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		return Chapter{}, err
	}
	return chapterCopy(chapter), nil
}

func (s *Store) DeleteChapter(workspaceID string, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return os.ErrNotExist
	}
	workspace = cloneWorkspace(workspace)
	for index, chapter := range workspace.Chapters {
		if chapter.ID != id {
			continue
		}
		workspace.Chapters = append(workspace.Chapters[:index], workspace.Chapters[index+1:]...)
		for chapterIndex := range workspace.Chapters {
			workspace.Chapters[chapterIndex].Number = chapterIndex + 1
		}
		touchProject(&workspace)
		return s.persistWorkspaceLocked(workspaceID, workspace)
	}
	return os.ErrNotExist
}

func (s *Store) ReorderChapters(workspaceID string, chapterIDs []int64) ([]Chapter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return nil, os.ErrNotExist
	}
	if len(chapterIDs) != len(workspace.Chapters) {
		return nil, fmt.Errorf("%w: chapter order must include every chapter exactly once", ErrInvalidChapterOrder)
	}

	chaptersByID := make(map[int64]Chapter, len(workspace.Chapters))
	for _, chapter := range workspace.Chapters {
		chaptersByID[chapter.ID] = chapter
	}
	ordered := make([]Chapter, len(chapterIDs))
	seen := make(map[int64]struct{}, len(chapterIDs))
	for index, id := range chapterIDs {
		chapter, found := chaptersByID[id]
		if !found {
			return nil, fmt.Errorf("%w: chapter order contains an unknown or duplicate chapter", ErrInvalidChapterOrder)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("%w: chapter order contains an unknown or duplicate chapter", ErrInvalidChapterOrder)
		}
		seen[id] = struct{}{}
		chapter.Number = index + 1
		ordered[index] = chapter
	}

	workspace = cloneWorkspace(workspace)
	workspace.Chapters = ordered
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		return nil, err
	}
	return append([]Chapter(nil), ordered...), nil
}
