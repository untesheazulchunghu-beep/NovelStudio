package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

var ErrInvalidProjectBackup = errors.New("invalid project backup")

const maxProjectBackupEntries = 10000

func projectBackupFor(workspace Workspace) ProjectBackup {
	settings := workspace.Settings
	chat := append([]ChatMessage{}, workspace.Chat...)
	for index := range chat {
		chat[index].RequestID = ""
	}
	return ProjectBackup{
		Format:      projectBackupFormat,
		Version:     projectBackupVersion,
		ExportedAt:  now(),
		Project:     workspace.Project,
		Chapters:    append([]Chapter{}, workspace.Chapters...),
		Characters:  append([]Character{}, workspace.Characters...),
		Notes:       append([]Note{}, workspace.Notes...),
		Timeline:    append([]TimelineEvent{}, workspace.Timeline...),
		WorldAssets: cloneWorldAssets(workspace.WorldAssets),
		Chat:        chat,
		Settings: ProjectBackupSettings{
			Provider: settings.Provider, BaseURL: sanitizeBackupBaseURL(settings.BaseURL),
			Model: settings.Model, Temperature: settings.Temperature,
		},
	}
}

func (s *Store) CreateProjectBackup(workspaceID string) (ProjectBackup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshWorkspaceIfChangedLocked(workspaceID); err != nil {
		return ProjectBackup{}, err
	}
	_, exists := s.workspaces[workspaceID]
	if !exists {
		return ProjectBackup{}, os.ErrNotExist
	}
	workspace, err := loadWorkspace(s.db, workspaceID)
	if err != nil {
		return ProjectBackup{}, err
	}
	workspace.ID = workspaceID
	backup := projectBackupFor(workspace)
	revisions, err := loadChapterRevisions(s.db, workspaceID)
	if err != nil {
		return ProjectBackup{}, err
	}
	backup.ChapterRevisions = revisions
	return backup, nil
}

func sanitizeBackupBaseURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	parsed.User = nil
	parsed.Fragment = ""
	parsed.RawFragment = ""

	query := parsed.Query()
	for name := range query {
		if isCredentialQueryParameter(name) {
			query.Del(name)
		}
	}
	parsed.RawQuery = query.Encode()
	parsed.ForceQuery = false
	return parsed.String()
}

func baseURLHasCredentials(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if parsed.User != nil {
		return true
	}
	for name := range parsed.Query() {
		if isCredentialQueryParameter(name) {
			return true
		}
	}
	return false
}

func isCredentialQueryParameter(name string) bool {
	normalized := strings.Map(func(character rune) rune {
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			return character
		}
		return -1
	}, name)
	for _, marker := range []string{"secret", "password", "passwd", "credential", "signature", "token", "authorization", "authentication"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "auth" || strings.HasSuffix(normalized, "auth") || normalized == "key" || strings.HasSuffix(normalized, "key") || normalized == "sig" || strings.HasSuffix(normalized, "sig") || normalized == "bearer" || strings.HasSuffix(normalized, "jwt")
}

func (s *Store) ImportProjectBackup(backup ProjectBackup) (ProjectSummary, error) {
	if backup.Format != projectBackupFormat || backup.Version != projectBackupVersion {
		return ProjectSummary{}, invalidProjectBackup("不支持的备份格式或版本")
	}
	projectTitle := strings.TrimSpace(backup.Project.Title)
	if projectTitle == "" {
		return ProjectSummary{}, invalidProjectBackup("项目名称不能为空")
	}
	if backup.Project.WordTarget < 0 {
		return ProjectSummary{}, invalidProjectBackup("目标字数不能为负数")
	}
	if err := validateProjectTextFields(ProjectInput{Title: projectTitle, Subtitle: backup.Project.Subtitle, Genre: backup.Project.Genre, Premise: backup.Project.Premise}); err != nil {
		return ProjectSummary{}, invalidProjectBackup(err.Error())
	}
	if len(backup.Chapters) > maxProjectBackupEntries || len(backup.ChapterRevisions) > maxProjectBackupEntries*maxChapterRevisions || len(backup.Characters) > maxProjectBackupEntries || len(backup.Notes) > maxProjectBackupEntries || len(backup.Timeline) > maxProjectBackupEntries || len(backup.WorldAssets) > maxProjectBackupEntries || len(backup.Chat) > maxProjectBackupEntries {
		return ProjectSummary{}, invalidProjectBackup("备份中的项目条目过多")
	}
	for _, chapter := range backup.Chapters {
		if err := validateChapterTextFields(ChapterInput{Title: chapter.Title, Summary: chapter.Summary}); err != nil {
			return ProjectSummary{}, invalidProjectBackup(err.Error())
		}
	}
	for _, character := range backup.Characters {
		if strings.TrimSpace(character.Name) == "" {
			return ProjectSummary{}, invalidProjectBackup("角色名称不能为空")
		}
		if err := validateCharacterTextFields(CharacterInput{Name: character.Name, Role: character.Role, Description: character.Description, Traits: character.Traits}); err != nil {
			return ProjectSummary{}, invalidProjectBackup(err.Error())
		}
	}
	for _, note := range backup.Notes {
		if strings.TrimSpace(note.Title) == "" {
			return ProjectSummary{}, invalidProjectBackup("便笺标题不能为空")
		}
		if err := validateNoteTextFields(note.Title, note.Body); err != nil {
			return ProjectSummary{}, invalidProjectBackup(err.Error())
		}
	}
	for _, event := range backup.Timeline {
		if strings.TrimSpace(event.Title) == "" {
			return ProjectSummary{}, invalidProjectBackup("时间线事件标题不能为空")
		}
		if err := validateTimelineEventTextFields(TimelineEventInput{Title: event.Title, TimeLabel: event.TimeLabel, Description: event.Description}); err != nil {
			return ProjectSummary{}, invalidProjectBackup(err.Error())
		}
		if _, err := normalizeTimelineEventStatus(event.Status); err != nil {
			return ProjectSummary{}, invalidProjectBackup(err.Error())
		}
	}
	for _, asset := range backup.WorldAssets {
		if strings.TrimSpace(asset.Name) == "" {
			return ProjectSummary{}, invalidProjectBackup("world asset name is required")
		}
		if err := validateWorldAssetTextFields(WorldAssetInput{Name: asset.Name, Kind: asset.Kind, Description: asset.Description, Details: asset.Details}); err != nil {
			return ProjectSummary{}, invalidProjectBackup(err.Error())
		}
		if _, err := normalizeWorldAssetKind(asset.Kind); err != nil {
			return ProjectSummary{}, invalidProjectBackup(err.Error())
		}
	}
	for _, message := range backup.Chat {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "assistant" && role != "system" {
			return ProjectSummary{}, invalidProjectBackup("聊天记录包含未知角色")
		}
	}
	provider := strings.ToLower(strings.TrimSpace(backup.Settings.Provider))
	if provider != "" && provider != "local" && provider != "openai" && provider != "compatible" {
		return ProjectSummary{}, invalidProjectBackup("备份中的模型服务商不受支持")
	}

	chapters := append([]Chapter{}, backup.Chapters...)
	sort.SliceStable(chapters, func(i, j int) bool { return chapters[i].Number < chapters[j].Number })
	revisions := append([]ChapterRevision{}, backup.ChapterRevisions...)
	sort.SliceStable(revisions, func(i, j int) bool {
		if revisions[i].ChapterID != revisions[j].ChapterID {
			return revisions[i].ChapterID < revisions[j].ChapterID
		}
		if revisions[i].CreatedAt != revisions[j].CreatedAt {
			return revisions[i].CreatedAt < revisions[j].CreatedAt
		}
		return revisions[i].ID < revisions[j].ID
	})
	chat := backup.Chat
	if len(chat) > 60 {
		chat = chat[len(chat)-60:]
	}

	workspaceToken := make([]byte, 16)
	if _, err := rand.Read(workspaceToken); err != nil {
		return ProjectSummary{}, fmt.Errorf("generate restored workspace ID: %w", err)
	}
	workspaceID := "project-" + hex.EncodeToString(workspaceToken)
	created := now()

	s.mu.Lock()
	defer s.mu.Unlock()
	projectID := int64(1)
	for _, existing := range s.workspaces {
		if existing.Project.ID >= projectID {
			projectID = existing.Project.ID + 1
		}
	}
	entityID := s.nextIDLocked()
	project := backup.Project
	project.ID = projectID
	project.Title = importedProjectTitle(projectTitle, s.workspaces)
	project.Subtitle = strings.TrimSpace(project.Subtitle)
	project.Genre = strings.TrimSpace(project.Genre)
	project.Premise = strings.TrimSpace(project.Premise)
	if project.Author == "" {
		project.Author = "我的工作区"
	}
	if project.Status == "" {
		project.Status = "draft"
	}
	if project.WordTarget == 0 {
		project.WordTarget = 120000
	}
	project.CreatedAt = created
	project.UpdatedAt = created
	project.CurrentView = "editor"

	chapterIDMap := make(map[int64]int64, len(chapters))
	for index := range chapters {
		oldID := chapters[index].ID
		if oldID != 0 {
			if _, duplicate := chapterIDMap[oldID]; duplicate {
				return ProjectSummary{}, invalidProjectBackup("章节标识重复")
			}
			chapterIDMap[oldID] = entityID
		}
		chapters[index].ID = entityID
		entityID++
		chapters[index].Number = index + 1
		chapters[index].Title = strings.TrimSpace(chapters[index].Title)
		if chapters[index].Title == "" {
			chapters[index].Title = fmt.Sprintf("未命名章节 %02d", index+1)
		}
		chapters[index].WordCount = countWords(chapters[index].Content)
		chapters[index].Status = normalizeChapterStatus(chapters[index].Status)
		if strings.TrimSpace(chapters[index].LastEditedAt) == "" {
			chapters[index].LastEditedAt = created
		}
	}
	for index := range revisions {
		revision := &revisions[index]
		chapterID, exists := chapterIDMap[revision.ChapterID]
		if !exists {
			return ProjectSummary{}, invalidProjectBackup("章节版本引用了不存在的章节")
		}
		revision.ID = 0
		revision.ChapterID = chapterID
		revision.Label = strings.TrimSpace(revision.Label)
		if revision.Label == "" {
			revision.Label = "恢复的版本"
		}
		if err := validateTextLength("chapter revision label", revision.Label, maxChapterRevisionLabelLength); err != nil {
			return ProjectSummary{}, invalidProjectBackup("章节版本名称过长")
		}
		if err := validateTextLength("chapter revision title", revision.Title, maxChapterTitleLength); err != nil {
			return ProjectSummary{}, invalidProjectBackup("章节版本标题过长")
		}
		if err := validateTextLength("chapter revision summary", revision.Summary, maxChapterSummaryLength); err != nil {
			return ProjectSummary{}, invalidProjectBackup("章节版本目标过长")
		}
		if revision.Title == "" {
			revision.Title = "未命名章节"
		}
		revision.WordCount = countWords(revision.Content)
		revision.Status = normalizeChapterStatus(revision.Status)
		if strings.TrimSpace(revision.CreatedAt) == "" {
			revision.CreatedAt = created
		}
	}
	characters := append([]Character{}, backup.Characters...)
	for index := range characters {
		characters[index].ID = entityID
		entityID++
		characters[index].Name = strings.TrimSpace(characters[index].Name)
		characters[index].Role = strings.TrimSpace(characters[index].Role)
		characters[index].Description = strings.TrimSpace(characters[index].Description)
		characters[index].Traits = normalizeTraits(characters[index].Traits)
		if color, valid := normalizeCharacterColor(characters[index].Color); valid {
			characters[index].Color = color
		} else {
			characters[index].Color = defaultCharacterColor(index)
		}
	}
	notes := append([]Note{}, backup.Notes...)
	for index := range notes {
		notes[index].ID = entityID
		entityID++
		notes[index].Title = strings.TrimSpace(notes[index].Title)
		notes[index].Kind = strings.TrimSpace(notes[index].Kind)
		if notes[index].Kind == "" {
			notes[index].Kind = "idea"
		}
		if notes[index].UpdatedAt == "" {
			notes[index].UpdatedAt = created
		}
	}
	timeline := append([]TimelineEvent{}, backup.Timeline...)
	for index := range timeline {
		event := &timeline[index]
		event.ID = entityID
		entityID++
		event.Title = strings.TrimSpace(event.Title)
		event.TimeLabel = strings.TrimSpace(event.TimeLabel)
		event.Status, _ = normalizeTimelineEventStatus(event.Status)
		if event.ChapterID != 0 {
			chapterID, exists := chapterIDMap[event.ChapterID]
			if !exists {
				return ProjectSummary{}, invalidProjectBackup("时间线事件引用了不存在的章节")
			}
			event.ChapterID = chapterID
		}
		if strings.TrimSpace(event.UpdatedAt) == "" {
			event.UpdatedAt = created
		}
	}
	worldAssets := cloneWorldAssets(backup.WorldAssets)
	worldAssetIDMap := make(map[int64]int64, len(worldAssets))
	for index := range worldAssets {
		asset := &worldAssets[index]
		oldID := asset.ID
		if oldID != 0 {
			if _, duplicate := worldAssetIDMap[oldID]; duplicate {
				return ProjectSummary{}, invalidProjectBackup("world asset identifiers must be unique")
			}
			worldAssetIDMap[oldID] = entityID
		}
		asset.ID = entityID
		entityID++
		asset.Name = strings.TrimSpace(asset.Name)
		asset.Kind, _ = normalizeWorldAssetKind(asset.Kind)
		asset.Description = strings.TrimSpace(asset.Description)
		asset.Details = strings.TrimSpace(asset.Details)
		if strings.TrimSpace(asset.UpdatedAt) == "" {
			asset.UpdatedAt = created
		}
		chapterIDs, err := remapBackupChapterIDs(asset.ChapterIDs, chapterIDMap)
		if err != nil {
			return ProjectSummary{}, invalidProjectBackup(err.Error())
		}
		asset.ChapterIDs = chapterIDs
	}
	for index := range worldAssets {
		asset := &worldAssets[index]
		related := make([]int64, 0, len(asset.RelatedAssetIDs))
		seen := make(map[int64]struct{}, len(asset.RelatedAssetIDs))
		for _, oldID := range asset.RelatedAssetIDs {
			newID, exists := worldAssetIDMap[oldID]
			if !exists || newID == asset.ID {
				return ProjectSummary{}, invalidProjectBackup("world asset relation references an unknown or self asset")
			}
			if _, duplicate := seen[newID]; duplicate {
				continue
			}
			seen[newID] = struct{}{}
			related = append(related, newID)
		}
		sort.Slice(related, func(left, right int) bool { return related[left] < related[right] })
		asset.RelatedAssetIDs = related
	}
	messages := append([]ChatMessage{}, chat...)
	for index := range messages {
		messages[index].ID = entityID
		entityID++
		messages[index].Role = strings.ToLower(strings.TrimSpace(messages[index].Role))
		messages[index].Mode = normalizeWritingMode(messages[index].Mode)
		messages[index].RequestID = ""
		if messages[index].CreatedAt == "" {
			messages[index].CreatedAt = created
		}
	}
	settings := Settings{
		Provider: provider, BaseURL: sanitizeBackupBaseURL(backup.Settings.BaseURL),
		Model: backup.Settings.Model, Temperature: backup.Settings.Temperature,
	}
	normalizeSettings(&settings)
	workspace := Workspace{
		ID: workspaceID, Project: project, Chapters: chapters, Characters: characters,
		Notes: notes, Timeline: timeline, WorldAssets: worldAssets, Chat: messages, Settings: settings, UpdatedAt: created,
	}
	if err := persistWorkspaceWithChapterRevisions(s.db, workspaceID, workspace, revisions); err != nil {
		return ProjectSummary{}, err
	}
	s.workspaces[workspaceID] = workspaceMetadata(workspace)
	return ProjectSummary{WorkspaceID: workspaceID, Project: project, Stats: calculateStats(workspace)}, nil
}

func importedProjectTitle(title string, workspaces map[string]Workspace) string {
	used := func(candidate string) bool {
		for _, workspace := range workspaces {
			if strings.EqualFold(strings.TrimSpace(workspace.Project.Title), candidate) {
				return true
			}
		}
		return false
	}
	if !used(title) {
		return title
	}
	base := title + "（恢复副本）"
	if !used(base) {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s（恢复副本 %d）", title, suffix)
		if !used(candidate) {
			return candidate
		}
	}
}

func invalidProjectBackup(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidProjectBackup, message)
}

func remapBackupChapterIDs(ids []int64, chapterIDMap map[int64]int64) ([]int64, error) {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, oldID := range ids {
		newID, exists := chapterIDMap[oldID]
		if !exists {
			return nil, fmt.Errorf("world asset references an unknown chapter")
		}
		if _, duplicate := seen[newID]; duplicate {
			continue
		}
		seen[newID] = struct{}{}
		result = append(result, newID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result, nil
}
