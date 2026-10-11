package app

import (
	"time"
	"unicode"
)

type Project struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	Genre       string `json:"genre"`
	Premise     string `json:"premise"`
	Author      string `json:"author"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	WordTarget  int    `json:"word_target"`
	CurrentView string `json:"current_view"`
}

type ProjectSummary struct {
	WorkspaceID string  `json:"workspace_id"`
	Project     Project `json:"project"`
	Stats       Stats   `json:"stats"`
}

type ProjectInput struct {
	Title      string `json:"title"`
	Subtitle   string `json:"subtitle"`
	Genre      string `json:"genre"`
	Premise    string `json:"premise"`
	WordTarget int    `json:"word_target"`
}

type Chapter struct {
	ID           int64  `json:"id"`
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Content      string `json:"content"`
	Status       string `json:"status"`
	WordCount    int    `json:"word_count"`
	LastEditedAt string `json:"last_edited_at"`
}

type ChapterRevision struct {
	ID        int64  `json:"id"`
	ChapterID int64  `json:"chapter_id"`
	Label     string `json:"label"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	WordCount int    `json:"word_count"`
	CreatedAt string `json:"created_at"`
}

type ChapterRevisionSummary struct {
	ID        int64  `json:"id"`
	ChapterID int64  `json:"chapter_id"`
	Label     string `json:"label"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Status    string `json:"status"`
	WordCount int    `json:"word_count"`
	CreatedAt string `json:"created_at"`
}

type ChapterRevisionInput struct {
	Label string `json:"label"`
}

type Character struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Traits      []string `json:"traits"`
	Color       string   `json:"color"`
}

type Note struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Kind      string `json:"kind"`
	UpdatedAt string `json:"updated_at"`
}

type TimelineEvent struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	TimeLabel   string `json:"time_label"`
	Description string `json:"description"`
	Status      string `json:"status"`
	ChapterID   int64  `json:"chapter_id,omitempty"`
	UpdatedAt   string `json:"updated_at"`
}

type WorldAsset struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	Description     string  `json:"description"`
	Details         string  `json:"details"`
	ChapterIDs      []int64 `json:"chapter_ids,omitempty"`
	RelatedAssetIDs []int64 `json:"related_asset_ids,omitempty"`
	UpdatedAt       string  `json:"updated_at"`
}

type ChatMessage struct {
	ID                 int64  `json:"id"`
	Role               string `json:"role"`
	Content            string `json:"content"`
	CreatedAt          string `json:"created_at"`
	Mode               string `json:"mode,omitempty"`
	RequestID          string `json:"request_id,omitempty"`
	RequestFingerprint string `json:"-"`
}

type Settings struct {
	Provider    string  `json:"provider"`
	BaseURL     string  `json:"base_url"`
	APIKey      string  `json:"api_key,omitempty"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}

type SettingsInput struct {
	Provider                string  `json:"provider"`
	BaseURL                 string  `json:"base_url"`
	APIKey                  *string `json:"api_key"`
	Model                   string  `json:"model"`
	Temperature             float64 `json:"temperature"`
	ClearBaseURLCredentials bool    `json:"clear_base_url_credentials,omitempty"`
}

type SettingsView struct {
	Provider                     string  `json:"provider"`
	BaseURL                      string  `json:"base_url"`
	Model                        string  `json:"model"`
	Temperature                  float64 `json:"temperature"`
	APIKeyConfigured             bool    `json:"api_key_configured"`
	BaseURLCredentialsConfigured bool    `json:"base_url_credentials_configured"`
}

type Workspace struct {
	ID          string          `json:"workspace_id"`
	Project     Project         `json:"project"`
	Chapters    []Chapter       `json:"chapters"`
	Characters  []Character     `json:"characters"`
	Notes       []Note          `json:"notes"`
	Timeline    []TimelineEvent `json:"timeline"`
	WorldAssets []WorldAsset    `json:"world_assets"`
	Chat        []ChatMessage   `json:"chat"`
	Settings    Settings        `json:"settings"`
	UpdatedAt   string          `json:"updated_at"`
}

type WorkspaceView struct {
	ID          string           `json:"workspace_id"`
	Project     Project          `json:"project"`
	Chapters    []ChapterSummary `json:"chapters"`
	Characters  []Character      `json:"characters"`
	Notes       []Note           `json:"notes"`
	Timeline    []TimelineEvent  `json:"timeline"`
	WorldAssets []WorldAsset     `json:"world_assets"`
	Chat        []ChatMessage    `json:"chat"`
	Settings    SettingsView     `json:"settings"`
	UpdatedAt   string           `json:"updated_at"`
}

type ChapterSummary struct {
	ID           int64  `json:"id"`
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Status       string `json:"status"`
	WordCount    int    `json:"word_count"`
	LastEditedAt string `json:"last_edited_at"`
}

type ChapterSearchResult struct {
	ChapterSummary
	Excerpt string `json:"excerpt"`
}

type WorkspaceResponse struct {
	Workspace WorkspaceView `json:"workspace"`
	Stats     Stats         `json:"stats"`
}

const projectBackupFormat = "novelstudio-project-backup"
const projectBackupVersion = 1

type ProjectBackup struct {
	Format           string                `json:"format"`
	Version          int                   `json:"version"`
	ExportedAt       string                `json:"exported_at"`
	Project          Project               `json:"project"`
	Chapters         []Chapter             `json:"chapters"`
	ChapterRevisions []ChapterRevision     `json:"chapter_revisions,omitempty"`
	Characters       []Character           `json:"characters"`
	Notes            []Note                `json:"notes"`
	Timeline         []TimelineEvent       `json:"timeline,omitempty"`
	WorldAssets      []WorldAsset          `json:"world_assets,omitempty"`
	Chat             []ChatMessage         `json:"chat"`
	Settings         ProjectBackupSettings `json:"settings"`
}

type ProjectBackupSettings struct {
	Provider    string  `json:"provider"`
	BaseURL     string  `json:"base_url"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}

type Stats struct {
	WordCount      int     `json:"word_count"`
	WordTarget     int     `json:"word_target"`
	ChapterCount   int     `json:"chapter_count"`
	FinishedCount  int     `json:"finished_count"`
	CompletionRate float64 `json:"completion_rate"`
}

type ChatRequest struct {
	RequestID      string            `json:"request_id,omitempty"`
	Message        string            `json:"message"`
	Mode           string            `json:"mode"`
	ChapterID      int64             `json:"chapter_id"`
	ChapterTitle   *string           `json:"chapter_title,omitempty"`
	ChapterSummary *string           `json:"chapter_summary,omitempty"`
	Context        string            `json:"context"`
	Selection      string            `json:"selection,omitempty"`
	ContextItems   *ChatContextItems `json:"context_items,omitempty"`
}

// ChatContextItems identifies the project materials an author explicitly
// wants to include in an Agent request. A nil value preserves the legacy
// behavior for older clients; an allocated value with empty lists means that
// no optional project materials should be included.
type ChatContextItems struct {
	CharacterIDs     []int64 `json:"character_ids,omitempty"`
	NoteIDs          []int64 `json:"note_ids,omitempty"`
	TimelineEventIDs []int64 `json:"timeline_event_ids,omitempty"`
	WorldAssetIDs    []int64 `json:"world_asset_ids,omitempty"`
}

type ChapterGenerateRequest struct {
	Instruction    string            `json:"instruction"`
	ChapterTitle   *string           `json:"chapter_title,omitempty"`
	ChapterSummary *string           `json:"chapter_summary,omitempty"`
	Context        *string           `json:"context"`
	ContextItems   *ChatContextItems `json:"context_items,omitempty"`
}

type ChatResponse struct {
	UserMessage ChatMessage `json:"user_message"`
	Message     ChatMessage `json:"message"`
	Suggestions []string    `json:"suggestions"`
	Provider    string      `json:"provider"`
}

type ChapterInput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

type ChapterOrderInput struct {
	ChapterIDs []int64 `json:"chapter_ids"`
}

type CharacterInput struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Traits      []string `json:"traits"`
	Color       string   `json:"color"`
}

type NoteInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Kind  string `json:"kind"`
}

type TimelineEventInput struct {
	Title       string `json:"title"`
	TimeLabel   string `json:"time_label"`
	Description string `json:"description"`
	Status      string `json:"status"`
	ChapterID   int64  `json:"chapter_id"`
}

type WorldAssetInput struct {
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	Description     string  `json:"description"`
	Details         string  `json:"details"`
	ChapterIDs      []int64 `json:"chapter_ids"`
	RelatedAssetIDs []int64 `json:"related_asset_ids"`
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func countWords(text string) int {
	count := 0
	inWord := false
	flush := func() {
		if inWord {
			count++
			inWord = false
		}
	}
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			flush()
			count++
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' {
			inWord = true
		} else {
			flush()
		}
	}
	flush()
	return count
}
