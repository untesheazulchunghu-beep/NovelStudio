package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
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

const invalidCharacterColorError = "character color must be a 6-digit hex color"
const maxProjectTitleLength = 80
const maxProjectSubtitleLength = 120
const maxProjectGenreLength = 80
const maxProjectPremiseLength = 500
const maxCharacterNameLength = 80
const maxCharacterRoleLength = 100
const maxCharacterDescriptionLength = 1000
const maxCharacterTraitsLength = 240
const maxNoteTitleLength = 120
const maxNoteBodyLength = 2000
const maxChapterTitleLength = 120
const maxChapterSummaryLength = 1200
const maxChapterRevisionLabelLength = 80
const maxChapterRevisions = 20

var characterColorPalette = [...]string{"#f0a46b", "#8f9df4", "#58c5a3", "#d890b8"}

func normalizeCharacterColor(color string) (string, bool) {
	color = strings.TrimSpace(color)
	if len(color) != 7 || color[0] != '#' {
		return "", false
	}
	for _, digit := range color[1:] {
		if !((digit >= '0' && digit <= '9') || (digit >= 'a' && digit <= 'f') || (digit >= 'A' && digit <= 'F')) {
			return "", false
		}
	}
	return strings.ToLower(color), true
}

func defaultCharacterColor(index int) string {
	return characterColorPalette[index%len(characterColorPalette)]
}

func validateTextLength(field, value string, maxLength int) error {
	if utf16Length(value) > maxLength {
		return fmt.Errorf("%s exceeds %d characters", field, maxLength)
	}
	return nil
}

func utf16Length(value string) int {
	length := 0
	for _, character := range value {
		length++
		if character > 0xffff {
			length++
		}
	}
	return length
}

func validateProjectTextFields(input ProjectInput) error {
	fields := []struct {
		name  string
		value string
		limit int
	}{
		{"project title", strings.TrimSpace(input.Title), maxProjectTitleLength},
		{"project subtitle", strings.TrimSpace(input.Subtitle), maxProjectSubtitleLength},
		{"project genre", strings.TrimSpace(input.Genre), maxProjectGenreLength},
		{"project premise", strings.TrimSpace(input.Premise), maxProjectPremiseLength},
	}
	for _, field := range fields {
		if err := validateTextLength(field.name, field.value, field.limit); err != nil {
			return err
		}
	}
	return nil
}

func validateCharacterTextFields(input CharacterInput) error {
	fields := []struct {
		name  string
		value string
		limit int
	}{
		{"character name", strings.TrimSpace(input.Name), maxCharacterNameLength},
		{"character role", strings.TrimSpace(input.Role), maxCharacterRoleLength},
		{"character description", strings.TrimSpace(input.Description), maxCharacterDescriptionLength},
	}
	for _, field := range fields {
		if err := validateTextLength(field.name, field.value, field.limit); err != nil {
			return err
		}
	}
	traits := strings.Join(normalizeTraits(input.Traits), ",")
	return validateTextLength("character traits", traits, maxCharacterTraitsLength)
}

func validateNoteTextFields(title, body string) error {
	if err := validateTextLength("note title", strings.TrimSpace(title), maxNoteTitleLength); err != nil {
		return err
	}
	return validateTextLength("note body", body, maxNoteBodyLength)
}

func validateChapterTextFields(input ChapterInput) error {
	if err := validateTextLength("chapter title", strings.TrimSpace(input.Title), maxChapterTitleLength); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidChapterInput, err)
	}
	if err := validateTextLength("chapter summary", input.Summary, maxChapterSummaryLength); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidChapterInput, err)
	}
	return nil
}

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

func chapterSummary(chapter Chapter) ChapterSummary {
	return ChapterSummary{
		ID: chapter.ID, Number: chapter.Number, Title: chapter.Title, Summary: chapter.Summary,
		Status: chapter.Status, WordCount: chapter.WordCount, LastEditedAt: chapter.LastEditedAt,
	}
}

func (s *Store) SearchChapters(workspaceID, query string) ([]ChapterSearchResult, bool, error) {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	_, exists := s.workspaces[workspaceID]
	if !exists {
		return nil, false, nil
	}
	if len(terms) == 0 {
		return []ChapterSearchResult{}, true, nil
	}
	if ftsQuery, supported := chapterSearchFTSQuery(terms); supported {
		results, err := s.searchChapterFTSLocked(workspaceID, ftsQuery)
		if err != nil {
			return nil, true, fmt.Errorf("search chapter index: %w", err)
		}
		return results, true, nil
	}
	results, err := s.searchChapterRowsLocked(workspaceID, terms)
	if err != nil {
		return nil, true, err
	}
	return results, true, nil
}

func (s *Store) searchChapterRowsLocked(workspaceID string, terms []string) ([]ChapterSearchResult, error) {
	if !searchTermsNeedCaseFold(terms) {
		return s.searchChapterRowsSQLLocked(workspaceID, terms)
	}
	return s.searchChapterRowsInGoLocked(workspaceID, terms)
}

func (s *Store) searchChapterRowsInGoLocked(workspaceID string, terms []string) ([]ChapterSearchResult, error) {
	rows, err := s.db.Query(`SELECT id, number, title, summary, content, status, word_count, last_edited_at
		FROM chapters WHERE workspace_id = ? ORDER BY number, id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results := make([]ChapterSearchResult, 0, 15)
	for rows.Next() {
		var chapter Chapter
		if err := rows.Scan(&chapter.ID, &chapter.Number, &chapter.Title, &chapter.Summary, &chapter.Content,
			&chapter.Status, &chapter.WordCount, &chapter.LastEditedAt); err != nil {
			return nil, err
		}
		if chapterMatchesSearchTerms(chapter, terms) {
			results = append(results, chapterSearchResult(chapter, terms))
			if len(results) == 15 {
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Store) searchChapterRowsSQLLocked(workspaceID string, terms []string) ([]ChapterSearchResult, error) {
	whereTerms := make([]string, 0, len(terms))
	args := make([]any, 0, len(terms)*8)
	for _, term := range terms {
		whereTerms = append(whereTerms, `(instr(CAST(number AS TEXT), ?) > 0 OR instr(title, ?) > 0 OR instr(summary, ?) > 0 OR instr(content, ?) > 0)`)
		args = append(args, term, term, term, term)
	}
	args = append(args, workspaceID)
	summaryIndexSQL, summaryIndexArgs := firstSQLiteMatchIndex("summary", terms)
	contentIndexSQL, contentIndexArgs := firstSQLiteMatchIndex("content", terms)
	queryArgs := make([]any, 0, len(summaryIndexArgs)+len(contentIndexArgs)+len(args))
	queryArgs = append(queryArgs, summaryIndexArgs...)
	queryArgs = append(queryArgs, contentIndexArgs...)
	queryArgs = append(queryArgs, args[len(terms)*4:]...)
	queryArgs = append(queryArgs, args[:len(terms)*4]...)
	query := `WITH matched AS (
		SELECT id, number, title, summary, status, word_count, last_edited_at,
			` + summaryIndexSQL + ` AS summary_index,
			` + contentIndexSQL + ` AS content_index
		FROM chapters
		WHERE workspace_id = ? AND ` + strings.Join(whereTerms, " AND ") + `
		ORDER BY number, id
		LIMIT 15
	)
	SELECT matched.id, matched.number, matched.title, matched.summary, matched.status,
		matched.word_count, matched.last_edited_at,
		CASE
			WHEN matched.summary_index > 0 THEN
				(CASE WHEN matched.summary_index > 29 THEN '…' ELSE '' END) ||
				substr(matched.summary, max(matched.summary_index - 28, 1), 78) ||
				(CASE WHEN length(matched.summary) > max(matched.summary_index - 28, 1) + 77 THEN '…' ELSE '' END)
			WHEN matched.content_index > 0 THEN
				(CASE WHEN matched.content_index > 29 THEN '…' ELSE '' END) ||
				substr(chapter.content, max(matched.content_index - 28, 1), 78) ||
				(CASE WHEN length(chapter.content) > max(matched.content_index - 28, 1) + 77 THEN '…' ELSE '' END)
			ELSE ''
		END AS excerpt
	FROM matched
	JOIN chapters AS chapter ON chapter.id = matched.id
	ORDER BY matched.number, matched.id`
	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results := make([]ChapterSearchResult, 0, 15)
	for rows.Next() {
		var chapter Chapter
		var excerpt string
		if err := rows.Scan(&chapter.ID, &chapter.Number, &chapter.Title, &chapter.Summary,
			&chapter.Status, &chapter.WordCount, &chapter.LastEditedAt, &excerpt); err != nil {
			return nil, err
		}
		results = append(results, ChapterSearchResult{
			ChapterSummary: chapterSummary(chapter),
			Excerpt:        compactChapterSearchExcerpt(excerpt, 78),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func firstSQLiteMatchIndex(column string, terms []string) (string, []any) {
	branches := make([]string, 0, len(terms))
	args := make([]any, 0, len(terms)*2)
	for _, term := range terms {
		branches = append(branches, `WHEN instr(`+column+`, ?) > 0 THEN instr(`+column+`, ?)`)
		args = append(args, term, term)
	}
	return `CASE ` + strings.Join(branches, " ") + ` ELSE 0 END`, args
}

func chapterSearchFTSQuery(terms []string) (string, bool) {
	quotedTerms := make([]string, 0, len(terms))
	for _, term := range terms {
		if utf8.RuneCountInString(term) < 3 {
			return "", false
		}
		asciiDigitsOnly := true
		for _, character := range term {
			if character < '0' || character > '9' {
				asciiDigitsOnly = false
			}
			if !unicode.IsLetter(character) && !unicode.IsNumber(character) && !unicode.IsMark(character) {
				return "", false
			}
		}
		if asciiDigitsOnly {
			return "", false
		}
		quotedTerms = append(quotedTerms, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	return strings.Join(quotedTerms, " AND "), len(quotedTerms) > 0
}

func (s *Store) searchChapterFTSLocked(workspaceID, ftsQuery string) ([]ChapterSearchResult, error) {
	rows, err := s.db.Query(`SELECT chapter.id, chapter.number, chapter.title, chapter.summary,
		chapter.status, chapter.word_count, chapter.last_edited_at,
		COALESCE(NULLIF(snippet(chapters_fts, 1, '', '', '…', 80), ''),
			NULLIF(snippet(chapters_fts, 2, '', '', '…', 80), ''), '')
		FROM chapters_fts
	JOIN chapters AS chapter ON chapter.id = chapters_fts.rowid
	WHERE chapters_fts MATCH ? AND chapter.workspace_id = ?
	ORDER BY chapter.number, chapter.id
	LIMIT 15`, ftsQuery, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results := make([]ChapterSearchResult, 0, 15)
	for rows.Next() {
		var chapter Chapter
		var excerpt string
		if err := rows.Scan(&chapter.ID, &chapter.Number, &chapter.Title, &chapter.Summary,
			&chapter.Status, &chapter.WordCount, &chapter.LastEditedAt, &excerpt); err != nil {
			return nil, err
		}
		results = append(results, ChapterSearchResult{
			ChapterSummary: chapterSummary(chapter),
			Excerpt:        compactChapterSearchExcerpt(excerpt, 78),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func compactChapterSearchExcerpt(value string, maxLength int) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	runes := []rune(value)
	if len(runes) > maxLength {
		return string(runes[:maxLength]) + "…"
	}
	return value
}

func chapterMatchesSearchTerms(chapter Chapter, terms []string) bool {
	fields := []string{strconv.Itoa(chapter.Number), chapter.Title, chapter.Summary, chapter.Content}
	for _, term := range terms {
		matched := false
		needsCaseFold := searchTermNeedsCaseFold(term)
		for _, field := range fields {
			if strings.Contains(field, term) || needsCaseFold && strings.Contains(strings.ToLower(field), term) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func searchTermNeedsCaseFold(term string) bool {
	for _, character := range term {
		if unicode.SimpleFold(character) != character {
			return true
		}
	}
	return false
}

func chapterSearchResult(chapter Chapter, terms []string) ChapterSearchResult {
	excerpt := chapterSearchExcerpt(chapter.Summary, terms, 78)
	if excerpt == "" {
		excerpt = chapterSearchExcerpt(chapter.Content, terms, 78)
	}
	return ChapterSearchResult{ChapterSummary: chapterSummary(chapter), Excerpt: excerpt}
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

func chapterSearchExcerpt(value string, terms []string, maxLength int) string {
	searchable := value
	if searchTermsNeedCaseFold(terms) {
		searchable = strings.ToLower(value)
	}
	matchIndex := -1
	for _, term := range terms {
		if index := strings.Index(searchable, term); index >= 0 {
			matchIndex = index
			break
		}
	}
	if matchIndex < 0 {
		return ""
	}
	startRune := utf8.RuneCountInString(searchable[:matchIndex])
	runes := []rune(value)
	start := 0
	if startRune > 28 {
		start = startRune - 28
	}
	end := start + maxLength
	if end > len(runes) {
		end = len(runes)
	}
	compact := strings.TrimSpace(strings.Join(strings.Fields(string(runes[start:end])), " "))
	if compact == "" {
		return ""
	}
	if start > 0 {
		compact = "…" + compact
	}
	if end < len(runes) {
		compact += "…"
	}
	return compact
}

func searchTermsNeedCaseFold(terms []string) bool {
	for _, term := range terms {
		if searchTermNeedsCaseFold(term) {
			return true
		}
	}
	return false
}

func settingsView(settings Settings) SettingsView {
	return SettingsView{
		Provider: settings.Provider, BaseURL: sanitizeBackupBaseURL(settings.BaseURL), Model: settings.Model,
		Temperature: settings.Temperature, APIKeyConfigured: strings.TrimSpace(settings.APIKey) != "",
		BaseURLCredentialsConfigured: baseURLHasCredentials(settings.BaseURL),
	}
}

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

func (s *Store) CreateCharacter(workspaceID string, input CharacterInput) (Character, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return Character{}, os.ErrNotExist
	}
	color := strings.TrimSpace(input.Color)
	if color != "" {
		var valid bool
		color, valid = normalizeCharacterColor(color)
		if !valid {
			return Character{}, fmt.Errorf("%w: %s", ErrInvalidCharacterInput, invalidCharacterColorError)
		}
	}
	if err := validateCharacterTextFields(input); err != nil {
		return Character{}, fmt.Errorf("%w: %v", ErrInvalidCharacterInput, err)
	}
	workspace = cloneWorkspace(workspace)
	character := Character{ID: s.nextIDLocked(), Name: strings.TrimSpace(input.Name), Role: strings.TrimSpace(input.Role), Description: strings.TrimSpace(input.Description), Traits: normalizeTraits(input.Traits), Color: color}
	if character.Name == "" {
		return Character{}, fmt.Errorf("%w: character name is required", ErrInvalidCharacterInput)
	}
	if character.Color == "" {
		character.Color = defaultCharacterColor(len(workspace.Characters))
	}
	workspace.Characters = append(workspace.Characters, character)
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		return Character{}, err
	}
	return characterCopy(character), nil
}

func (s *Store) UpdateCharacter(workspaceID string, id int64, input CharacterInput) (Character, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return Character{}, os.ErrNotExist
	}
	color := strings.TrimSpace(input.Color)
	if color != "" {
		var valid bool
		color, valid = normalizeCharacterColor(color)
		if !valid {
			return Character{}, fmt.Errorf("%w: %s", ErrInvalidCharacterInput, invalidCharacterColorError)
		}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Character{}, fmt.Errorf("%w: character name is required", ErrInvalidCharacterInput)
	}
	if err := validateCharacterTextFields(input); err != nil {
		return Character{}, fmt.Errorf("%w: %v", ErrInvalidCharacterInput, err)
	}
	workspace = cloneWorkspace(workspace)
	for index := range workspace.Characters {
		if workspace.Characters[index].ID != id {
			continue
		}
		character := &workspace.Characters[index]
		character.Name = name
		character.Role = strings.TrimSpace(input.Role)
		character.Description = strings.TrimSpace(input.Description)
		character.Traits = normalizeTraits(input.Traits)
		if color != "" {
			character.Color = color
		}
		touchProject(&workspace)
		if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
			return Character{}, err
		}
		return characterCopy(*character), nil
	}
	return Character{}, os.ErrNotExist
}

func (s *Store) DeleteCharacter(workspaceID string, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return os.ErrNotExist
	}
	workspace = cloneWorkspace(workspace)
	for index, character := range workspace.Characters {
		if character.ID != id {
			continue
		}
		workspace.Characters = append(workspace.Characters[:index], workspace.Characters[index+1:]...)
		touchProject(&workspace)
		return s.persistWorkspaceLocked(workspaceID, workspace)
	}
	return os.ErrNotExist
}

func (s *Store) CreateNote(workspaceID string, input NoteInput) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return Note{}, os.ErrNotExist
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return Note{}, fmt.Errorf("%w: note title is required", ErrInvalidNoteInput)
	}
	if err := validateNoteTextFields(title, input.Body); err != nil {
		return Note{}, fmt.Errorf("%w: %v", ErrInvalidNoteInput, err)
	}
	kind, err := normalizeNoteKind(input.Kind)
	if err != nil {
		return Note{}, fmt.Errorf("%w: %v", ErrInvalidNoteInput, err)
	}
	workspace = cloneWorkspace(workspace)
	note := Note{ID: s.nextIDLocked(), Title: title, Body: input.Body, Kind: kind, UpdatedAt: now()}
	workspace.Notes = append(workspace.Notes, note)
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		return Note{}, err
	}
	return note, nil
}

func (s *Store) UpdateNote(workspaceID string, id int64, input NoteInput) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return Note{}, os.ErrNotExist
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return Note{}, fmt.Errorf("%w: note title is required", ErrInvalidNoteInput)
	}
	if err := validateNoteTextFields(title, input.Body); err != nil {
		return Note{}, fmt.Errorf("%w: %v", ErrInvalidNoteInput, err)
	}
	kind, kindErr := normalizeNoteKind(input.Kind)
	workspace = cloneWorkspace(workspace)
	for index := range workspace.Notes {
		if workspace.Notes[index].ID != id {
			continue
		}
		note := &workspace.Notes[index]
		if kindErr != nil {
			if input.Kind != note.Kind {
				return Note{}, fmt.Errorf("%w: %v", ErrInvalidNoteInput, kindErr)
			}
			kind = note.Kind
		}
		note.Title = title
		note.Body = input.Body
		note.Kind = kind
		note.UpdatedAt = now()
		touchProject(&workspace)
		if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
			return Note{}, err
		}
		return *note, nil
	}
	return Note{}, os.ErrNotExist
}

func (s *Store) DeleteNote(workspaceID string, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return os.ErrNotExist
	}
	workspace = cloneWorkspace(workspace)
	for index, note := range workspace.Notes {
		if note.ID != id {
			continue
		}
		workspace.Notes = append(workspace.Notes[:index], workspace.Notes[index+1:]...)
		touchProject(&workspace)
		return s.persistWorkspaceLocked(workspaceID, workspace)
	}
	return os.ErrNotExist
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

func chatRequestMismatchError(existing ChatMessage, submittedUser, requestFingerprint string) error {
	if existing.RequestFingerprint == requestFingerprint {
		return nil
	}
	if existing.RequestFingerprint == "" && strings.TrimSpace(existing.Content) == strings.TrimSpace(submittedUser) {
		return ErrChatRequestUnverifiable
	}
	return ErrChatRequestConflict
}

func (s *Store) LookupChatRequest(workspaceID, requestID, user, requestFingerprint string) (ChatResponse, bool, error) {
	if requestID == "" {
		return ChatResponse{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	response, found, err := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint)
	if err == nil && found {
		if err := s.refreshWorkspaceLocked(workspaceID); err != nil {
			return ChatResponse{}, false, err
		}
	}
	return response, found, err
}

func (s *Store) refreshWorkspaceIfChangedLocked(workspaceID string) error {
	var updatedAt string
	err := s.db.QueryRow(`SELECT updated_at FROM workspaces WHERE workspace_id = ?`, workspaceID).Scan(&updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		delete(s.workspaces, workspaceID)
		return os.ErrNotExist
	}
	if err != nil {
		return fmt.Errorf("check workspace freshness %s: %w", workspaceID, err)
	}
	if workspace, exists := s.workspaces[workspaceID]; exists && workspace.Project.UpdatedAt == updatedAt {
		return nil
	}
	return s.refreshWorkspaceLocked(workspaceID)
}

func (s *Store) refreshWorkspaceLocked(workspaceID string) error {
	workspace, err := loadWorkspaceMetadata(s.db, workspaceID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			delete(s.workspaces, workspaceID)
			return os.ErrNotExist
		}
		return fmt.Errorf("refresh workspace %s: %w", workspaceID, err)
	}
	workspace.ID = workspaceID
	normalizeWorkspaceMetadata(&workspace)
	s.workspaces[workspaceID] = workspace
	return nil
}

const chatRequestClaimLease = 2 * time.Minute
const chatRequestClaimPollInterval = 100 * time.Millisecond

func (s *Store) ClaimChatRequest(ctx context.Context, workspaceID, requestID, user, requestFingerprint string) (ChatResponse, string, bool, error) {
	if requestID == "" {
		return ChatResponse{}, "", false, nil
	}
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, "", false, err
	}
	ownerBytes := make([]byte, 16)
	if _, err := rand.Read(ownerBytes); err != nil {
		return ChatResponse{}, "", false, fmt.Errorf("create chat request claim token: %w", err)
	}
	ownerToken := hex.EncodeToString(ownerBytes)

	s.mu.Lock()
	_, err := s.db.Exec("DELETE FROM chat_request_claims WHERE lease_until <= ?", time.Now().UnixMilli())
	s.mu.Unlock()
	if err != nil {
		return ChatResponse{}, "", false, fmt.Errorf("remove expired chat request claims: %w", err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return ChatResponse{}, "", false, err
		}
		s.mu.Lock()
		response, found, err := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint)
		claimed := false
		if err == nil && !found {
			claimed, err = tryClaimChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint, ownerToken, time.Now())
		}
		if err == nil && found {
			err = s.refreshWorkspaceLocked(workspaceID)
		}
		s.mu.Unlock()
		if err != nil {
			return ChatResponse{}, "", false, err
		}
		if found {
			return response, "", false, nil
		}
		if claimed {
			s.mu.Lock()
			response, found, err = lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint)
			if err == nil && found {
				err = s.refreshWorkspaceLocked(workspaceID)
			}
			s.mu.Unlock()
			if err != nil || found {
				_ = s.ReleaseChatRequestClaim(workspaceID, requestID, ownerToken)
				return response, "", false, err
			}
			return ChatResponse{}, ownerToken, true, nil
		}

		timer := time.NewTimer(chatRequestClaimPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ChatResponse{}, "", false, ctx.Err()
		case <-timer.C:
		}
	}
}

func tryClaimChatRequestDB(db *sql.DB, workspaceID, requestID, user, requestFingerprint, ownerToken string, currentTime time.Time) (bool, error) {
	nowMillis := currentTime.UnixMilli()
	leaseUntil := currentTime.Add(chatRequestClaimLease).UnixMilli()
	result, err := db.Exec(`INSERT OR IGNORE INTO chat_request_claims
		(workspace_id, request_id, request_fingerprint, user_content, owner_token, lease_until)
		VALUES (?, ?, ?, ?, ?, ?)`, workspaceID, requestID, requestFingerprint, user, ownerToken, leaseUntil)
	if err != nil {
		return false, fmt.Errorf("claim chat request: %w", err)
	}
	if changed, err := result.RowsAffected(); err != nil {
		return false, fmt.Errorf("check chat request claim: %w", err)
	} else if changed == 1 {
		return true, nil
	}

	var storedFingerprint, storedUser string
	var storedLeaseUntil int64
	err = db.QueryRow(`SELECT request_fingerprint, user_content, lease_until FROM chat_request_claims
		WHERE workspace_id = ? AND request_id = ?`, workspaceID, requestID).Scan(&storedFingerprint, &storedUser, &storedLeaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read chat request claim: %w", err)
	}
	if err := chatRequestMismatchError(ChatMessage{Content: storedUser, RequestFingerprint: storedFingerprint}, user, requestFingerprint); err != nil {
		return false, err
	}
	if storedLeaseUntil > nowMillis {
		return false, nil
	}
	result, err = db.Exec(`UPDATE chat_request_claims SET owner_token = ?, lease_until = ?
		WHERE workspace_id = ? AND request_id = ? AND lease_until <= ?`, ownerToken, leaseUntil, workspaceID, requestID, nowMillis)
	if err != nil {
		return false, fmt.Errorf("take over expired chat request claim: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check expired chat request claim: %w", err)
	}
	return changed == 1, nil
}

func (s *Store) ReleaseChatRequestClaim(workspaceID, requestID, ownerToken string) error {
	if requestID == "" || ownerToken == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.db.Exec(`DELETE FROM chat_request_claims WHERE workspace_id = ? AND request_id = ? AND owner_token = ?`, workspaceID, requestID, ownerToken); err != nil {
		return fmt.Errorf("release chat request claim: %w", err)
	}
	return nil
}

func (s *Store) AppendChatRequest(ctx context.Context, workspaceID, requestID, user, assistant, mode, requestFingerprint, provider, ownerToken string) (ChatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}
	if requestID != "" {
		if response, found, err := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint); err != nil {
			return ChatResponse{}, err
		} else if found {
			if err := s.refreshWorkspaceLocked(workspaceID); err != nil {
				return ChatResponse{}, err
			}
			return response, nil
		}
	}
	if err := s.refreshWorkspaceLocked(workspaceID); err != nil {
		return ChatResponse{}, err
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return ChatResponse{}, os.ErrNotExist
	}
	for _, message := range workspace.Chat {
		if requestID == "" || message.Role != "user" || message.RequestID != requestID {
			continue
		}
		if err := chatRequestMismatchError(message, user, requestFingerprint); err != nil {
			return ChatResponse{}, err
		}
		if response, found := chatResponseForRequest(workspace.Chat, requestID, provider); found {
			if err := saveChatRequestRecord(s.db, newChatRequestRecord(workspaceID, requestID, requestFingerprint, user, response, ownerToken)); err != nil {
				if stored, found, lookupErr := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint); lookupErr != nil {
					return ChatResponse{}, lookupErr
				} else if found {
					return stored, nil
				}
				return ChatResponse{}, err
			}
			return response, nil
		}
		return ChatResponse{}, ErrChatRequestUnverifiable
	}

	userMessage := ChatMessage{Role: "user", Content: user, CreatedAt: now(), Mode: mode, RequestID: requestID, RequestFingerprint: requestFingerprint}
	assistantMessage := ChatMessage{Role: "assistant", Content: assistant, CreatedAt: now(), Mode: mode}
	response := ChatResponse{UserMessage: userMessage, Message: assistantMessage, Suggestions: suggestionsFor(mode), Provider: provider}
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}
	response, err := appendChatResponseDB(s.db, workspaceID, requestID, user, requestFingerprint, ownerToken, response)
	if err != nil {
		if requestID != "" {
			if stored, found, lookupErr := lookupChatRequestDB(s.db, workspaceID, requestID, user, requestFingerprint); lookupErr == nil && found {
				return stored, nil
			} else if lookupErr != nil {
				return ChatResponse{}, lookupErr
			}
		}
		return ChatResponse{}, err
	}
	if err := s.refreshWorkspaceLocked(workspaceID); err != nil {
		return ChatResponse{}, err
	}
	return response, nil
}

func (s *Store) AppendChat(ctx context.Context, workspaceID, requestID, user, assistant, mode, requestFingerprint string) ([]ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return nil, os.ErrNotExist
	}
	for _, message := range workspace.Chat {
		if requestID != "" && message.Role == "user" && message.RequestID == requestID {
			if message.RequestFingerprint != requestFingerprint {
				return nil, chatRequestMismatchError(message, user, requestFingerprint)
			}
			return append([]ChatMessage(nil), workspace.Chat...), nil
		}
	}
	workspace = cloneWorkspace(workspace)
	nextMessageID := s.nextIDLocked()
	if strings.TrimSpace(user) != "" {
		workspace.Chat = append(workspace.Chat, ChatMessage{ID: nextMessageID, Role: "user", Content: user, CreatedAt: now(), Mode: mode, RequestID: requestID, RequestFingerprint: requestFingerprint})
		nextMessageID++
	}
	if strings.TrimSpace(assistant) != "" {
		workspace.Chat = append(workspace.Chat, ChatMessage{ID: nextMessageID, Role: "assistant", Content: assistant, CreatedAt: now(), Mode: mode})
	}
	if len(workspace.Chat) > 60 {
		workspace.Chat = workspace.Chat[len(workspace.Chat)-60:]
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		if requestID != "" {
			persisted, loadErr := loadWorkspace(s.db, workspaceID)
			if loadErr == nil {
				for _, message := range persisted.Chat {
					if message.Role == "user" && message.RequestID == requestID {
						if message.RequestFingerprint != requestFingerprint {
							return nil, chatRequestMismatchError(message, user, requestFingerprint)
						}
						s.workspaces[workspaceID] = workspaceMetadata(persisted)
						return append([]ChatMessage(nil), persisted.Chat...), nil
					}
				}
			}
		}
		return nil, err
	}
	return append([]ChatMessage(nil), workspace.Chat...), nil
}

func (s *Store) nextIDLocked() int64 {
	max := int64(0)
	for _, workspace := range s.workspaces {
		workspaceMax := maxID(workspace)
		if workspaceMax > max {
			max = workspaceMax
		}
	}
	return max + 1
}

func maxID(workspace Workspace) int64 {
	max := workspace.Project.ID
	for _, chapter := range workspace.Chapters {
		if chapter.ID > max {
			max = chapter.ID
		}
	}
	for _, character := range workspace.Characters {
		if character.ID > max {
			max = character.ID
		}
	}
	for _, note := range workspace.Notes {
		if note.ID > max {
			max = note.ID
		}
	}
	for _, message := range workspace.Chat {
		if message.ID > max {
			max = message.ID
		}
	}
	return max
}

func touchProject(workspace *Workspace) {
	workspace.Project.UpdatedAt = now()
	workspace.UpdatedAt = workspace.Project.UpdatedAt
}

func (s *Store) persistWorkspaceLocked(workspaceID string, workspace Workspace) error {
	workspace.ID = workspaceID
	workspace.UpdatedAt = now()
	workspace.Project.UpdatedAt = workspace.UpdatedAt
	var err error
	if previous, exists := s.workspaces[workspaceID]; exists {
		err = persistWorkspaceDelta(s.db, workspaceID, previous, workspace)
	} else {
		err = persistWorkspace(s.db, workspaceID, workspace)
	}
	if err != nil {
		return err
	}
	s.workspaces[workspaceID] = workspaceMetadata(workspace)
	return nil
}

func workspaceMetadata(workspace Workspace) Workspace {
	metadata := cloneWorkspace(workspace)
	for index := range metadata.Chapters {
		metadata.Chapters[index].Content = ""
	}
	return metadata
}

func normalizeWorkspace(workspace *Workspace) {
	if workspace.Chapters == nil {
		workspace.Chapters = []Chapter{}
	}
	if workspace.Characters == nil {
		workspace.Characters = []Character{}
	}
	if workspace.Notes == nil {
		workspace.Notes = []Note{}
	}
	if workspace.Chat == nil {
		workspace.Chat = []ChatMessage{}
	}
	normalizeSettings(&workspace.Settings)
	createdAt := strings.TrimSpace(workspace.Project.CreatedAt)
	updatedAt := strings.TrimSpace(workspace.Project.UpdatedAt)
	if updatedAt == "" {
		updatedAt = strings.TrimSpace(workspace.UpdatedAt)
	}
	if updatedAt == "" {
		updatedAt = now()
	}
	if createdAt == "" {
		createdAt = updatedAt
	}
	workspace.Project.CreatedAt = createdAt
	workspace.Project.UpdatedAt = updatedAt
	workspace.UpdatedAt = updatedAt
	for index := range workspace.Chapters {
		workspace.Chapters[index].WordCount = countWords(workspace.Chapters[index].Content)
		workspace.Chapters[index].Status = normalizeChapterStatus(workspace.Chapters[index].Status)
		if strings.TrimSpace(workspace.Chapters[index].LastEditedAt) == "" {
			workspace.Chapters[index].LastEditedAt = workspace.Project.UpdatedAt
		}
	}
	for index := range workspace.Characters {
		workspace.Characters[index].Traits = normalizeTraits(workspace.Characters[index].Traits)
		if color, valid := normalizeCharacterColor(workspace.Characters[index].Color); valid {
			workspace.Characters[index].Color = color
		} else {
			workspace.Characters[index].Color = defaultCharacterColor(index)
		}
	}
	for index := range workspace.Notes {
		workspace.Notes[index].Title = strings.TrimSpace(workspace.Notes[index].Title)
		workspace.Notes[index].Kind = strings.TrimSpace(workspace.Notes[index].Kind)
		if workspace.Notes[index].Kind == "" {
			workspace.Notes[index].Kind = "idea"
		}
	}
	if workspace.Project.WordTarget <= 0 {
		workspace.Project.WordTarget = 120000
	}
}

func normalizeWorkspaceMetadata(workspace *Workspace) {
	wordCounts := make([]int, len(workspace.Chapters))
	for index := range workspace.Chapters {
		wordCounts[index] = workspace.Chapters[index].WordCount
	}
	normalizeWorkspace(workspace)
	for index := range workspace.Chapters {
		workspace.Chapters[index].WordCount = wordCounts[index]
	}
}

func normalizeChapterStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "outline":
		return "outline"
	case "draft":
		return "draft"
	case "review":
		return "review"
	case "done":
		return "done"
	default:
		return "draft"
	}
}

func normalizeNoteKind(kind string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return "idea", nil
	}
	switch kind {
	case "idea", "mystery", "craft", "world":
		return kind, nil
	default:
		return "", fmt.Errorf("note kind must be idea, mystery, craft, or world")
	}
}

func normalizeSettings(settings *Settings) {
	settings.Provider = strings.ToLower(strings.TrimSpace(settings.Provider))
	settings.BaseURL = strings.TrimSpace(settings.BaseURL)
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	settings.Model = strings.TrimSpace(settings.Model)
	if settings.Provider == "" {
		settings.Provider = "local"
	}
	if settings.Model == "" && settings.Provider == "local" {
		settings.Model = "local-creative"
	}
	if math.IsNaN(settings.Temperature) || math.IsInf(settings.Temperature, 0) || settings.Temperature < 0 || settings.Temperature > 2 {
		settings.Temperature = 0.7
	}
}

func normalizeTraits(traits []string) []string {
	cleaned := make([]string, 0, len(traits))
	for _, trait := range traits {
		if value := strings.TrimSpace(trait); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func seedWorkspace() Workspace {
	created := now()
	return Workspace{
		ID:      localWorkspaceID,
		Project: Project{ID: 1, Title: "星火回廊", Subtitle: "一封来自未来的信", Genre: "近未来 · 悬疑", Premise: "在记忆可以被备份的雾港，失踪的姐姐寄来一封尚未发生的信。", Author: "我的工作区", Status: "draft", CreatedAt: created, UpdatedAt: created, WordTarget: 120000, CurrentView: "editor"},
		Chapters: []Chapter{
			{ID: 2, Number: 1, Title: "雾港来信", Summary: "林澈在凌晨收到姐姐寄来的信，信上写着一场尚未发生的事故。", Content: "凌晨四点十七分，雾港的潮声像一台没有关掉的旧收音机。\n\n林澈在门缝里发现那只信封时，先看见的是邮戳。日期是三天以后。\n\n她把信翻过来，封口处没有胶水，只有一枚暗银色的回廊印记。那是姐姐林栀失踪前，留在桌上的最后一样东西。\n\n“不要让钟楼在第七声之后响。”\n\n纸上的字迹很轻，像写信的人正站在很远的地方。", Status: "draft", LastEditedAt: created},
			{ID: 3, Number: 2, Title: "第七声钟响", Summary: "林澈追着信中的线索来到旧钟楼，发现时间记录被人为改写。", Content: "旧钟楼的门没有锁。\n\n林澈推开门，灰尘从门楣落下，露出墙上密密麻麻的刻痕。每一道刻痕旁都写着一个时间，唯独七点整的位置被人刮去了。", Status: "outline", LastEditedAt: created},
			{ID: 4, Number: 3, Title: "回廊尽头", Summary: "一条只在潮汐最低时出现的走廊，通向姐姐留下的真相。", Content: "", Status: "outline", LastEditedAt: created},
		},
		Characters: []Character{
			{ID: 5, Name: "林澈", Role: "主角 · 记忆修复师", Description: "习惯先修复别人的记忆，再处理自己的情绪。", Traits: []string{"敏锐", "克制", "怕水"}, Color: "#f0a46b"},
			{ID: 6, Name: "林栀", Role: "失踪的姐姐", Description: "留下预言般书信的量子考古学家。", Traits: []string{"理性", "冒险", "执拗"}, Color: "#8f9df4"},
			{ID: 7, Name: "周既明", Role: "钟楼管理员", Description: "知道雾港每一座钟的秘密，却从不谈论第七声。", Traits: []string{"温和", "多疑", "守旧"}, Color: "#58c5a3"},
		},
		Notes: []Note{
			{ID: 8, Title: "核心悬念", Body: "姐姐寄来的信来自三天以后；第七声钟响会让一段记忆被覆盖。", Kind: "mystery", UpdatedAt: created},
			{ID: 9, Title: "写作提醒", Body: "让每章都出现一种与潮汐有关的感官意象。", Kind: "craft", UpdatedAt: created},
		},
		Settings:  Settings{Provider: "local", Model: "local-creative", Temperature: 0.7},
		UpdatedAt: created,
	}
}

func calculateStats(workspace Workspace) Stats {
	stats := Stats{WordTarget: workspace.Project.WordTarget, ChapterCount: len(workspace.Chapters)}
	for _, chapter := range workspace.Chapters {
		stats.WordCount += chapter.WordCount
		if chapter.Status == "done" {
			stats.FinishedCount++
		}
	}
	if stats.WordTarget > 0 {
		stats.CompletionRate = float64(stats.WordCount) / float64(stats.WordTarget)
	}
	if stats.CompletionRate > 1 {
		stats.CompletionRate = 1
	}
	return stats
}

func cloneWorkspace(input Workspace) Workspace {
	output := input
	output.Chapters = append([]Chapter{}, input.Chapters...)
	output.Characters = make([]Character, len(input.Characters))
	for index, character := range input.Characters {
		output.Characters[index] = characterCopy(character)
	}
	output.Notes = append([]Note{}, input.Notes...)
	output.Chat = append([]ChatMessage{}, input.Chat...)
	output.Settings = input.Settings
	return output
}

func chapterCopy(chapter Chapter) Chapter { return chapter }
func characterCopy(character Character) Character {
	character.Traits = append([]string{}, character.Traits...)
	return character
}
