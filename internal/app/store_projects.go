package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func (s *Store) ListProjects() []ProjectSummary {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT workspace_id FROM workspaces`)
	if err == nil {
		workspaceIDs := make([]string, 0)
		for rows.Next() {
			var workspaceID string
			if scanErr := rows.Scan(&workspaceID); scanErr != nil {
				err = scanErr
				break
			}
			workspaceIDs = append(workspaceIDs, workspaceID)
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			err = rowsErr
		}
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err == nil {
			knownWorkspaces := make(map[string]struct{}, len(workspaceIDs))
			for _, workspaceID := range workspaceIDs {
				if refreshErr := s.refreshWorkspaceLocked(workspaceID); errors.Is(refreshErr, os.ErrNotExist) {
					continue
				}
				knownWorkspaces[workspaceID] = struct{}{}
			}
			for workspaceID := range s.workspaces {
				if _, exists := knownWorkspaces[workspaceID]; !exists {
					delete(s.workspaces, workspaceID)
				}
			}
		}
	}

	projects := make([]ProjectSummary, 0, len(s.workspaces))
	for workspaceID, workspace := range s.workspaces {
		projects = append(projects, ProjectSummary{WorkspaceID: workspaceID, Project: workspace.Project, Stats: calculateStats(workspace)})
	}
	sort.Slice(projects, func(i, j int) bool {
		leftUpdated, leftErr := time.Parse(time.RFC3339Nano, projects[i].Project.UpdatedAt)
		rightUpdated, rightErr := time.Parse(time.RFC3339Nano, projects[j].Project.UpdatedAt)
		if leftErr == nil && rightErr == nil && !leftUpdated.Equal(rightUpdated) {
			return leftUpdated.After(rightUpdated)
		}
		if (leftErr == nil) != (rightErr == nil) {
			return leftErr == nil
		}
		if projects[i].Project.UpdatedAt != projects[j].Project.UpdatedAt {
			return projects[i].Project.UpdatedAt > projects[j].Project.UpdatedAt
		}
		if projects[i].Project.ID != projects[j].Project.ID {
			return projects[i].Project.ID > projects[j].Project.ID
		}
		return projects[i].WorkspaceID < projects[j].WorkspaceID
	})
	return projects
}

func (s *Store) CreateProject(input ProjectInput) (ProjectSummary, error) {
	input.Title = strings.TrimSpace(input.Title)
	if err := validateProjectTextFields(input); err != nil {
		return ProjectSummary{}, fmt.Errorf("%w: %v", ErrInvalidProjectInput, err)
	}
	if input.Title == "" {
		return ProjectSummary{}, fmt.Errorf("%w: project title is required", ErrInvalidProjectInput)
	}
	if input.WordTarget < 0 {
		return ProjectSummary{}, fmt.Errorf("%w: project word target cannot be negative", ErrInvalidProjectInput)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var nextID int64 = 1
	for _, workspace := range s.workspaces {
		if workspace.Project.ID >= nextID {
			nextID = workspace.Project.ID + 1
		}
	}
	workspaceToken := make([]byte, 16)
	if _, err := rand.Read(workspaceToken); err != nil {
		return ProjectSummary{}, fmt.Errorf("generate project workspace ID: %w", err)
	}
	workspaceID := "project-" + hex.EncodeToString(workspaceToken)
	created := now()
	wordTarget := input.WordTarget
	if wordTarget <= 0 {
		wordTarget = 120000
	}
	workspace := Workspace{
		ID: workspaceID,
		Project: Project{
			ID: nextID, Title: input.Title, Subtitle: strings.TrimSpace(input.Subtitle),
			Genre: strings.TrimSpace(input.Genre), Premise: strings.TrimSpace(input.Premise),
			Author: "我的工作区", Status: "draft", CreatedAt: created, UpdatedAt: created,
			WordTarget: wordTarget, CurrentView: "editor",
		},
		Chapters: []Chapter{}, Characters: []Character{}, Notes: []Note{}, Chat: []ChatMessage{},
		Settings:  Settings{Provider: "local", Model: "local-creative", Temperature: 0.7},
		UpdatedAt: created,
	}
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		return ProjectSummary{}, err
	}
	return ProjectSummary{WorkspaceID: workspaceID, Project: workspace.Project, Stats: calculateStats(workspace)}, nil
}

func (s *Store) UpdateProject(workspaceID string, input ProjectInput) (ProjectSummary, error) {
	input.Title = strings.TrimSpace(input.Title)
	if err := validateProjectTextFields(input); err != nil {
		return ProjectSummary{}, fmt.Errorf("%w: %v", ErrInvalidProjectInput, err)
	}
	if input.Title == "" {
		return ProjectSummary{}, fmt.Errorf("%w: project title is required", ErrInvalidProjectInput)
	}
	if input.WordTarget < 0 {
		return ProjectSummary{}, fmt.Errorf("%w: project word target cannot be negative", ErrInvalidProjectInput)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return ProjectSummary{}, os.ErrNotExist
	}
	workspace = cloneWorkspace(workspace)
	workspace.Project.Title = input.Title
	workspace.Project.Subtitle = strings.TrimSpace(input.Subtitle)
	workspace.Project.Genre = strings.TrimSpace(input.Genre)
	workspace.Project.Premise = strings.TrimSpace(input.Premise)
	if input.WordTarget > 0 {
		workspace.Project.WordTarget = input.WordTarget
	}
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		return ProjectSummary{}, err
	}
	return ProjectSummary{WorkspaceID: workspaceID, Project: workspace.Project, Stats: calculateStats(workspace)}, nil
}

func (s *Store) DeleteProject(workspaceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin project deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize deletions before counting rows so separate processes cannot
	// both remove a different project based on stale workspace caches.
	locked, err := tx.Exec(`UPDATE workspaces SET updated_at = updated_at WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return fmt.Errorf("lock project for deletion: %w", err)
	}
	if affected, err := locked.RowsAffected(); err != nil {
		return fmt.Errorf("check project before deletion: %w", err)
	} else if affected == 0 {
		return os.ErrNotExist
	}
	var projectCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM workspaces`).Scan(&projectCount); err != nil {
		return fmt.Errorf("count projects before deletion: %w", err)
	}
	if projectCount <= 1 {
		return ErrLastProject
	}
	result, err := tx.Exec(`DELETE FROM workspaces WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check project deletion: %w", err)
	}
	if deleted == 0 {
		return os.ErrNotExist
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit project deletion: %w", err)
	}
	delete(s.workspaces, workspaceID)
	return nil
}

func (s *Store) UpdateSettings(workspaceID string, input SettingsInput) (SettingsView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return SettingsView{}, os.ErrNotExist
	}
	workspace = cloneWorkspace(workspace)
	baseURL := input.BaseURL
	if input.ClearBaseURLCredentials {
		baseURL = sanitizeBackupBaseURL(baseURL)
	} else if strings.TrimSpace(baseURL) == sanitizeBackupBaseURL(workspace.Settings.BaseURL) {
		baseURL = workspace.Settings.BaseURL
	}
	settings := Settings{
		Provider: input.Provider, BaseURL: baseURL,
		Model: input.Model, Temperature: input.Temperature,
	}
	if input.APIKey == nil {
		settings.APIKey = workspace.Settings.APIKey
	} else {
		settings.APIKey = *input.APIKey
	}
	normalizeSettings(&settings)
	// Reject incomplete remote settings while they are being saved. Without
	// this check the settings page could report success and the next Agent
	// request would fail much later with the same configuration error.
	if settings.Provider != "local" && settings.Provider != "openai" && settings.Provider != "compatible" {
		return SettingsView{}, fmt.Errorf("%w: unsupported provider %q", ErrInvalidModelSettings, settings.Provider)
	}
	if settings.Provider == "openai" || settings.Provider == "compatible" {
		if err := validateModelSettings(settings, settings.Provider); err != nil {
			return SettingsView{}, err
		}
	}
	workspace.Settings = settings
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		return SettingsView{}, err
	}
	return settingsView(settings), nil
}
