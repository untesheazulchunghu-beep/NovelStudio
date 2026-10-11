package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func (s *Store) loadOrInitialize(legacyPath string) error {
	rows, err := s.db.Query(`SELECT workspace_id FROM workspaces ORDER BY updated_at DESC`)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read workspace id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate workspace ids: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close workspace query: %w", err)
	}
	if len(ids) > 0 {
		for _, id := range ids {
			workspace, err := loadWorkspaceMetadata(s.db, id)
			if err != nil {
				return err
			}
			workspace.ID = id
			normalizeWorkspaceMetadata(&workspace)
			s.workspaces[id] = workspace
		}
		return nil
	}

	workspace, err := readLegacyWorkspace(legacyPath)
	if err != nil {
		return err
	}
	if workspace == nil {
		seeded := seedWorkspace()
		workspace = &seeded
	}
	if workspace.ID == "" {
		workspace.ID = localWorkspaceID
	}
	normalizeWorkspace(workspace)
	if err := persistWorkspace(s.db, workspace.ID, *workspace); err != nil {
		return fmt.Errorf("initialize local workspace: %w", err)
	}
	if err := migrateChatRequestRecords(s.db); err != nil {
		return fmt.Errorf("migrate initialized chat request results: %w", err)
	}
	s.workspaces[workspace.ID] = workspaceMetadata(*workspace)
	return nil
}

func readLegacyWorkspace(path string) (*Workspace, error) {
	if path == "" {
		return nil, nil
	}
	if err := protectLocalFile(path, "legacy workspace file"); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read legacy workspace %s: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect legacy workspace %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("legacy workspace %s is not a regular file", path)
	}
	if info.Size() > maxLegacyWorkspaceBytes {
		_ = file.Close()
		return nil, fmt.Errorf("legacy workspace %s exceeds the %d MiB migration limit", path, maxLegacyWorkspaceBytes/(1<<20))
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxLegacyWorkspaceBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read legacy workspace %s: %w", path, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close legacy workspace %s: %w", path, closeErr)
	}
	if int64(len(data)) > maxLegacyWorkspaceBytes {
		return nil, fmt.Errorf("legacy workspace %s exceeds the %d MiB migration limit", path, maxLegacyWorkspaceBytes/(1<<20))
	}
	var workspace Workspace
	if err := json.Unmarshal(data, &workspace); err != nil {
		return nil, fmt.Errorf("decode legacy workspace %s: %w", path, err)
	}
	var legacySettings struct {
		Settings *struct {
			Temperature *float64 `json:"temperature"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(data, &legacySettings); err != nil {
		return nil, fmt.Errorf("decode legacy workspace settings %s: %w", path, err)
	}
	if legacySettings.Settings == nil || legacySettings.Settings.Temperature == nil {
		workspace.Settings.Temperature = 0.7
	}
	if workspace.Project.ID == 0 || strings.TrimSpace(workspace.Project.Title) == "" {
		return nil, fmt.Errorf("legacy workspace %s has no valid project", path)
	}
	return &workspace, nil
}

func persistWorkspace(db *sql.DB, workspaceID string, workspace Workspace) error {
	return persistWorkspaceWithChapterRevisions(db, workspaceID, workspace, nil)
}

func persistWorkspaceWithChapterRevisions(db *sql.DB, workspaceID string, workspace Workspace, revisions []ChapterRevision) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin workspace transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	project := workspace.Project
	_, err = tx.Exec(`INSERT INTO workspaces (
		workspace_id, owner_id, project_id, title, subtitle, genre, premise, author, status,
		created_at, updated_at, word_target, current_view
	) VALUES (?, 'local', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(workspace_id) DO UPDATE SET
		project_id=excluded.project_id, title=excluded.title, subtitle=excluded.subtitle,
		genre=excluded.genre, premise=excluded.premise, author=excluded.author, status=excluded.status,
		created_at=excluded.created_at, updated_at=excluded.updated_at,
		word_target=excluded.word_target, current_view=excluded.current_view`,
		workspaceID, project.ID, project.Title, project.Subtitle, project.Genre, project.Premise,
		project.Author, project.Status, project.CreatedAt, project.UpdatedAt, project.WordTarget, project.CurrentView)
	if err != nil {
		return fmt.Errorf("save workspace project: %w", err)
	}
	for _, table := range []string{"chapter_revisions", "chapters", "characters", "notes", "chat_messages"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE workspace_id = ?`, workspaceID); err != nil {
			return fmt.Errorf("replace workspace %s: %w", table, err)
		}
	}
	for _, chapter := range workspace.Chapters {
		if _, err := tx.Exec(`INSERT INTO chapters (id, workspace_id, number, title, summary, content, status, word_count, last_edited_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, chapter.ID, workspaceID, chapter.Number, chapter.Title,
			chapter.Summary, chapter.Content, chapter.Status, chapter.WordCount, chapter.LastEditedAt); err != nil {
			return fmt.Errorf("save chapter %d: %w", chapter.ID, err)
		}
	}
	revisionChapterIDs := make(map[int64]struct{})
	for _, revision := range revisions {
		if _, err := tx.Exec(`INSERT INTO chapter_revisions (workspace_id, chapter_id, label, title, summary, content, status, word_count, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, workspaceID, revision.ChapterID, revision.Label, revision.Title,
			revision.Summary, revision.Content, revision.Status, revision.WordCount, revision.CreatedAt); err != nil {
			return fmt.Errorf("save chapter revision %d: %w", revision.ID, err)
		}
		revisionChapterIDs[revision.ChapterID] = struct{}{}
	}
	for chapterID := range revisionChapterIDs {
		if err := pruneChapterRevisions(tx, workspaceID, chapterID); err != nil {
			return err
		}
	}
	for _, character := range workspace.Characters {
		traits, err := json.Marshal(character.Traits)
		if err != nil {
			return fmt.Errorf("encode character traits: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO characters (id, workspace_id, name, role, description, traits_json, color)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, character.ID, workspaceID, character.Name, character.Role,
			character.Description, string(traits), character.Color); err != nil {
			return fmt.Errorf("save character %d: %w", character.ID, err)
		}
	}
	for _, note := range workspace.Notes {
		if _, err := tx.Exec(`INSERT INTO notes (id, workspace_id, title, body, kind, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, note.ID, workspaceID, note.Title, note.Body, note.Kind, note.UpdatedAt); err != nil {
			return fmt.Errorf("save note %d: %w", note.ID, err)
		}
	}
	for _, message := range workspace.Chat {
		if _, err := tx.Exec(`INSERT INTO chat_messages (id, workspace_id, role, content, created_at, mode, request_id, request_fingerprint)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, message.ID, workspaceID, message.Role, message.Content, message.CreatedAt, message.Mode, message.RequestID, message.RequestFingerprint); err != nil {
			return fmt.Errorf("save chat message %d: %w", message.ID, err)
		}
	}
	settings := workspace.Settings
	if _, err := tx.Exec(`INSERT INTO settings (workspace_id, provider, base_url, api_key, model, temperature)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			provider=excluded.provider, base_url=excluded.base_url, api_key=excluded.api_key,
			model=excluded.model, temperature=excluded.temperature`,
		workspaceID, settings.Provider, settings.BaseURL, settings.APIKey, settings.Model, settings.Temperature); err != nil {
		return fmt.Errorf("save workspace settings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workspace transaction: %w", err)
	}
	return nil
}

func loadWorkspace(db *sql.DB, workspaceID string) (Workspace, error) {
	return loadWorkspaceWithChapterContent(db, workspaceID, true)
}

func loadWorkspaceMetadata(db *sql.DB, workspaceID string) (Workspace, error) {
	return loadWorkspaceWithChapterContent(db, workspaceID, false)
}

func loadWorkspaceWithChapterContent(db *sql.DB, workspaceID string, includeContent bool) (Workspace, error) {
	workspace := Workspace{ID: workspaceID, Chapters: []Chapter{}, Characters: []Character{}, Notes: []Note{}, Chat: []ChatMessage{}}
	project := &workspace.Project
	if err := db.QueryRow(`SELECT project_id, title, subtitle, genre, premise, author, status,
		created_at, updated_at, word_target, current_view FROM workspaces WHERE workspace_id = ?`, workspaceID).Scan(
		&project.ID, &project.Title, &project.Subtitle, &project.Genre, &project.Premise, &project.Author,
		&project.Status, &project.CreatedAt, &project.UpdatedAt, &project.WordTarget, &project.CurrentView); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Workspace{}, fmt.Errorf("load workspace project: %w", os.ErrNotExist)
		}
		return Workspace{}, fmt.Errorf("load workspace project: %w", err)
	}
	workspace.UpdatedAt = project.UpdatedAt

	chapterQuery := `SELECT id, number, title, summary, content, status, word_count, last_edited_at
		FROM chapters WHERE workspace_id = ? ORDER BY number, id`
	if !includeContent {
		chapterQuery = `SELECT id, number, title, summary, '' AS content, status, word_count, last_edited_at
			FROM chapters WHERE workspace_id = ? ORDER BY number, id`
	}
	chapterRows, err := db.Query(chapterQuery, workspaceID)
	if err != nil {
		return Workspace{}, fmt.Errorf("query chapters: %w", err)
	}
	for chapterRows.Next() {
		var chapter Chapter
		if err := chapterRows.Scan(&chapter.ID, &chapter.Number, &chapter.Title, &chapter.Summary,
			&chapter.Content, &chapter.Status, &chapter.WordCount, &chapter.LastEditedAt); err != nil {
			_ = chapterRows.Close()
			return Workspace{}, fmt.Errorf("read chapter: %w", err)
		}
		workspace.Chapters = append(workspace.Chapters, chapter)
	}
	if err := chapterRows.Err(); err != nil {
		_ = chapterRows.Close()
		return Workspace{}, fmt.Errorf("iterate chapters: %w", err)
	}
	if err := chapterRows.Close(); err != nil {
		return Workspace{}, fmt.Errorf("close chapter query: %w", err)
	}

	characterRows, err := db.Query(`SELECT id, name, role, description, traits_json, color
		FROM characters WHERE workspace_id = ? ORDER BY id`, workspaceID)
	if err != nil {
		return Workspace{}, fmt.Errorf("query characters: %w", err)
	}
	for characterRows.Next() {
		var character Character
		var traits string
		if err := characterRows.Scan(&character.ID, &character.Name, &character.Role, &character.Description, &traits, &character.Color); err != nil {
			_ = characterRows.Close()
			return Workspace{}, fmt.Errorf("read character: %w", err)
		}
		if err := json.Unmarshal([]byte(traits), &character.Traits); err != nil {
			_ = characterRows.Close()
			return Workspace{}, fmt.Errorf("decode character traits: %w", err)
		}
		workspace.Characters = append(workspace.Characters, character)
	}
	if err := characterRows.Err(); err != nil {
		_ = characterRows.Close()
		return Workspace{}, fmt.Errorf("iterate characters: %w", err)
	}
	if err := characterRows.Close(); err != nil {
		return Workspace{}, fmt.Errorf("close character query: %w", err)
	}

	noteRows, err := db.Query(`SELECT id, title, body, kind, updated_at FROM notes WHERE workspace_id = ? ORDER BY id`, workspaceID)
	if err != nil {
		return Workspace{}, fmt.Errorf("query notes: %w", err)
	}
	for noteRows.Next() {
		var note Note
		if err := noteRows.Scan(&note.ID, &note.Title, &note.Body, &note.Kind, &note.UpdatedAt); err != nil {
			_ = noteRows.Close()
			return Workspace{}, fmt.Errorf("read note: %w", err)
		}
		workspace.Notes = append(workspace.Notes, note)
	}
	if err := noteRows.Err(); err != nil {
		_ = noteRows.Close()
		return Workspace{}, fmt.Errorf("iterate notes: %w", err)
	}
	if err := noteRows.Close(); err != nil {
		return Workspace{}, fmt.Errorf("close note query: %w", err)
	}

	chatRows, err := db.Query(`SELECT id, role, content, created_at, mode, request_id, request_fingerprint FROM chat_messages
		WHERE workspace_id = ? ORDER BY id DESC LIMIT 60`, workspaceID)
	if err != nil {
		return Workspace{}, fmt.Errorf("query chat history: %w", err)
	}
	for chatRows.Next() {
		var message ChatMessage
		if err := chatRows.Scan(&message.ID, &message.Role, &message.Content, &message.CreatedAt, &message.Mode, &message.RequestID, &message.RequestFingerprint); err != nil {
			_ = chatRows.Close()
			return Workspace{}, fmt.Errorf("read chat message: %w", err)
		}
		workspace.Chat = append(workspace.Chat, message)
	}
	if err := chatRows.Err(); err != nil {
		_ = chatRows.Close()
		return Workspace{}, fmt.Errorf("iterate chat history: %w", err)
	}
	if err := chatRows.Close(); err != nil {
		return Workspace{}, fmt.Errorf("close chat query: %w", err)
	}
	for left, right := 0, len(workspace.Chat)-1; left < right; left, right = left+1, right-1 {
		workspace.Chat[left], workspace.Chat[right] = workspace.Chat[right], workspace.Chat[left]
	}

	workspace.Settings = Settings{Provider: "local", Model: "local-creative", Temperature: 0.7}
	settings := &workspace.Settings
	err = db.QueryRow(`SELECT provider, base_url, api_key, model, temperature FROM settings WHERE workspace_id = ?`, workspaceID).Scan(
		&settings.Provider, &settings.BaseURL, &settings.APIKey, &settings.Model, &settings.Temperature)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, fmt.Errorf("load workspace settings: %w", err)
	}
	return workspace, nil
}

func loadChapter(db *sql.DB, workspaceID string, chapterID int64) (Chapter, error) {
	var chapter Chapter
	err := db.QueryRow(`SELECT id, number, title, summary, content, status, word_count, last_edited_at
		FROM chapters WHERE workspace_id = ? AND id = ?`, workspaceID, chapterID).Scan(
		&chapter.ID, &chapter.Number, &chapter.Title, &chapter.Summary, &chapter.Content,
		&chapter.Status, &chapter.WordCount, &chapter.LastEditedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Chapter{}, os.ErrNotExist
	}
	if err != nil {
		return Chapter{}, fmt.Errorf("load chapter %d: %w", chapterID, err)
	}
	return chapter, nil
}
