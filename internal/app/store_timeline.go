package app

import (
	"fmt"
	"os"
	"strings"
)

func (s *Store) CreateTimelineEvent(workspaceID string, input TimelineEventInput) (TimelineEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return TimelineEvent{}, os.ErrNotExist
	}
	event, err := normalizeTimelineEventInput(workspace, input)
	if err != nil {
		return TimelineEvent{}, err
	}
	workspace = cloneWorkspace(workspace)
	event.ID = s.nextIDLocked()
	event.UpdatedAt = now()
	workspace.Timeline = append(workspace.Timeline, event)
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		return TimelineEvent{}, err
	}
	return event, nil
}

func (s *Store) UpdateTimelineEvent(workspaceID string, id int64, input TimelineEventInput) (TimelineEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return TimelineEvent{}, os.ErrNotExist
	}
	event, err := normalizeTimelineEventInput(workspace, input)
	if err != nil {
		return TimelineEvent{}, err
	}
	workspace = cloneWorkspace(workspace)
	for index := range workspace.Timeline {
		if workspace.Timeline[index].ID != id {
			continue
		}
		event.ID = id
		event.UpdatedAt = now()
		workspace.Timeline[index] = event
		touchProject(&workspace)
		if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
			return TimelineEvent{}, err
		}
		return event, nil
	}
	return TimelineEvent{}, os.ErrNotExist
}

func (s *Store) DeleteTimelineEvent(workspaceID string, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return os.ErrNotExist
	}
	workspace = cloneWorkspace(workspace)
	for index, event := range workspace.Timeline {
		if event.ID != id {
			continue
		}
		workspace.Timeline = append(workspace.Timeline[:index], workspace.Timeline[index+1:]...)
		touchProject(&workspace)
		return s.persistWorkspaceLocked(workspaceID, workspace)
	}
	return os.ErrNotExist
}

func normalizeTimelineEventInput(workspace Workspace, input TimelineEventInput) (TimelineEvent, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return TimelineEvent{}, fmt.Errorf("%w: timeline event title is required", ErrInvalidTimelineEventInput)
	}
	if err := validateTimelineEventTextFields(input); err != nil {
		return TimelineEvent{}, fmt.Errorf("%w: %v", ErrInvalidTimelineEventInput, err)
	}
	status, err := normalizeTimelineEventStatus(input.Status)
	if err != nil {
		return TimelineEvent{}, fmt.Errorf("%w: %v", ErrInvalidTimelineEventInput, err)
	}
	if input.ChapterID != 0 {
		if _, exists := findChapter(workspace, input.ChapterID); !exists {
			return TimelineEvent{}, fmt.Errorf("%w: chapter not found", ErrInvalidTimelineEventInput)
		}
	}
	return TimelineEvent{
		Title: title, TimeLabel: strings.TrimSpace(input.TimeLabel), Description: input.Description,
		Status: status, ChapterID: input.ChapterID,
	}, nil
}
