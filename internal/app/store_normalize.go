package app

import (
	"fmt"
	"math"
	"strings"
)

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
