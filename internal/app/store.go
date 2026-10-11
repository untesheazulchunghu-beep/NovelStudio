package app

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

var ErrLastProject = errors.New("cannot delete the last project")

var ErrInvalidChapterInput = errors.New("invalid chapter input")

var ErrInvalidProjectInput = errors.New("invalid project input")

var ErrInvalidChapterOrder = errors.New("invalid chapter order")

var ErrChatRequestConflict = errors.New("这条 Agent 请求的标识已用于不同内容。")

var ErrChatRequestUnverifiable = errors.New("这条 Agent 请求对应旧聊天记录，无法验证是否为同一请求。请先检查聊天记录，再决定是否重新发送。")

var ErrChatRequestClaimLost = errors.New("Agent 请求的处理租约已失效，请检查聊天记录后使用同一请求重试。")

var ErrInvalidCharacterInput = errors.New("invalid character input")

var ErrInvalidNoteInput = errors.New("invalid note input")

type Store struct {
	mu                      sync.RWMutex
	db                      *sql.DB
	databasePath            string
	workspaces              map[string]Workspace
	chatRequestLocks        *chatRequestLockManager
	releaseChatRequestLocks func()
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("workspace path is empty")
	}
	store, legacyPath, err := openSQLiteStore(path)
	if err != nil {
		return nil, err
	}
	store.workspaces = make(map[string]Workspace)
	store.chatRequestLocks, store.releaseChatRequestLocks = pooledChatRequestLockManager(store.databasePath)
	if err := store.loadOrInitialize(legacyPath); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	if s.releaseChatRequestLocks != nil {
		s.releaseChatRequestLocks()
	}
	return err
}

func (s *Store) DatabasePath() string { return s.databasePath }

func (s *Store) Snapshot(workspaceID string) (Workspace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); errors.Is(err, os.ErrNotExist) {
		return Workspace{}, false
	}
	if _, exists := s.workspaces[workspaceID]; !exists {
		return Workspace{}, false
	}
	workspace, err := loadWorkspace(s.db, workspaceID)
	if err != nil {
		return Workspace{}, false
	}
	workspace.ID = workspaceID
	normalizeWorkspaceMetadata(&workspace)
	return workspace, true
}

func (s *Store) MetadataSnapshot(workspaceID string) (Workspace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); errors.Is(err, os.ErrNotExist) {
		return Workspace{}, false
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return Workspace{}, false
	}
	return cloneWorkspace(workspace), true
}

func (s *Store) Response(workspaceID string) (WorkspaceResponse, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); err != nil {
		return WorkspaceResponse{}, false
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return WorkspaceResponse{}, false
	}
	chapters := make([]ChapterSummary, 0, len(workspace.Chapters))
	for _, chapter := range workspace.Chapters {
		chapters = append(chapters, chapterSummary(chapter))
	}
	characters := append([]Character{}, workspace.Characters...)
	for index := range characters {
		characters[index].Traits = append([]string{}, characters[index].Traits...)
	}
	return WorkspaceResponse{
		Workspace: WorkspaceView{
			ID: workspace.ID, Project: workspace.Project, Chapters: chapters,
			Characters: characters, Notes: append([]Note{}, workspace.Notes...), Chat: append([]ChatMessage{}, workspace.Chat...),
			Settings: settingsView(workspace.Settings), UpdatedAt: workspace.UpdatedAt,
		},
		Stats: calculateStats(workspace),
	}, true
}

func settingsView(settings Settings) SettingsView {
	return SettingsView{
		Provider: settings.Provider, BaseURL: sanitizeBackupBaseURL(settings.BaseURL), Model: settings.Model,
		Temperature: settings.Temperature, APIKeyConfigured: strings.TrimSpace(settings.APIKey) != "",
		BaseURLCredentialsConfigured: baseURLHasCredentials(settings.BaseURL),
	}
}
