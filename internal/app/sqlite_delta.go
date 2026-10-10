package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// persistWorkspaceDelta keeps ordinary saves proportional to changed records
// instead of replacing every child table on each autosave.
func persistWorkspaceDelta(db *sql.DB, workspaceID string, previous, workspace Workspace) error {
	return persistWorkspaceDeltaWithChatRequest(db, workspaceID, previous, workspace, nil)
}

func persistWorkspaceDeltaWithChatRequest(db *sql.DB, workspaceID string, previous, workspace Workspace, request *chatRequestRecord) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin workspace update: %w", err)
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
	if err := persistChapterDelta(tx, workspaceID, previous.Chapters, workspace.Chapters); err != nil {
		return err
	}
	if err := persistCharacterDelta(tx, workspaceID, previous.Characters, workspace.Characters); err != nil {
		return err
	}
	if err := persistNoteDelta(tx, workspaceID, previous.Notes, workspace.Notes); err != nil {
		return err
	}
	if err := persistChatDelta(tx, workspaceID, previous.Chat, workspace.Chat); err != nil {
		return err
	}
	if request != nil {
		request.WorkspaceID = workspaceID
		if err := insertChatRequestRecord(tx, *request); err != nil {
			return err
		}
	}
	if previous.Settings != workspace.Settings {
		settings := workspace.Settings
		if _, err := tx.Exec(`INSERT INTO settings (workspace_id, provider, base_url, api_key, model, temperature)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(workspace_id) DO UPDATE SET
				provider=excluded.provider, base_url=excluded.base_url, api_key=excluded.api_key,
				model=excluded.model, temperature=excluded.temperature`,
			workspaceID, settings.Provider, settings.BaseURL, settings.APIKey, settings.Model, settings.Temperature); err != nil {
			return fmt.Errorf("save workspace settings: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workspace update: %w", err)
	}
	return nil
}

func persistChapterDelta(tx *sql.Tx, workspaceID string, previous, current []Chapter) error {
	previousByID := make(map[int64]Chapter, len(previous))
	currentByID := make(map[int64]Chapter, len(current))
	for _, chapter := range previous {
		previousByID[chapter.ID] = chapter
	}
	for _, chapter := range current {
		currentByID[chapter.ID] = chapter
	}
	if err := deleteRemovedRows(tx, "chapters", workspaceID, previous, current, func(chapter Chapter) int64 { return chapter.ID }); err != nil {
		return err
	}

	orderChanged := false
	for id, old := range previousByID {
		if next, exists := currentByID[id]; exists && old.Number != next.Number {
			orderChanged = true
			break
		}
	}
	if orderChanged {
		// Move existing numbers to values unused by either order before assigning
		// the new sequence, avoiding UNIQUE(workspace_id, number) conflicts.
		usedNumbers := make(map[int64]struct{}, len(previous)+len(current)*2)
		for _, chapter := range previous {
			if _, remains := currentByID[chapter.ID]; remains {
				usedNumbers[int64(chapter.Number)] = struct{}{}
			}
		}
		for _, chapter := range current {
			usedNumbers[int64(chapter.Number)] = struct{}{}
		}
		temporaryNumbers := make(map[int64]int64, len(previous))
		nextTemporaryNumber := int64(-1 << 63)
		for _, chapter := range previous {
			if _, remains := currentByID[chapter.ID]; !remains {
				continue
			}
			for {
				if _, occupied := usedNumbers[nextTemporaryNumber]; !occupied {
					break
				}
				nextTemporaryNumber++
			}
			temporaryNumbers[chapter.ID] = nextTemporaryNumber
			usedNumbers[nextTemporaryNumber] = struct{}{}
			nextTemporaryNumber++
		}
		for _, chapter := range previous {
			temporaryNumber, remains := temporaryNumbers[chapter.ID]
			if !remains {
				continue
			}
			if _, err := tx.Exec(`UPDATE chapters SET number = ? WHERE workspace_id = ? AND id = ?`, temporaryNumber, workspaceID, chapter.ID); err != nil {
				return fmt.Errorf("stage chapter order for chapter %d: %w", chapter.ID, err)
			}
		}
	}

	for _, chapter := range current {
		old, exists := previousByID[chapter.ID]
		if exists && !orderChanged && old == chapter {
			continue
		}
		if exists {
			if old.Title == chapter.Title && old.Summary == chapter.Summary &&
				old.Content == chapter.Content && old.Status == chapter.Status && old.WordCount == chapter.WordCount &&
				old.LastEditedAt == chapter.LastEditedAt {
				if _, err := tx.Exec(`UPDATE chapters SET number = ? WHERE workspace_id = ? AND id = ?`, chapter.Number, workspaceID, chapter.ID); err != nil {
					return fmt.Errorf("update chapter order for chapter %d: %w", chapter.ID, err)
				}
				continue
			}
			if _, err := tx.Exec(`UPDATE chapters SET number = ?, title = ?, summary = ?, content = ?, status = ?, word_count = ?, last_edited_at = ?
				WHERE workspace_id = ? AND id = ?`, chapter.Number, chapter.Title, chapter.Summary, chapter.Content,
				chapter.Status, chapter.WordCount, chapter.LastEditedAt, workspaceID, chapter.ID); err != nil {
				return fmt.Errorf("update chapter %d: %w", chapter.ID, err)
			}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO chapters (id, workspace_id, number, title, summary, content, status, word_count, last_edited_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, chapter.ID, workspaceID, chapter.Number, chapter.Title,
			chapter.Summary, chapter.Content, chapter.Status, chapter.WordCount, chapter.LastEditedAt); err != nil {
			return fmt.Errorf("insert chapter %d: %w", chapter.ID, err)
		}
	}
	return nil
}

func persistCharacterDelta(tx *sql.Tx, workspaceID string, previous, current []Character) error {
	previousByID := make(map[int64]Character, len(previous))
	for _, character := range previous {
		previousByID[character.ID] = character
	}
	if err := deleteRemovedRows(tx, "characters", workspaceID, previous, current, func(character Character) int64 { return character.ID }); err != nil {
		return err
	}
	for _, character := range current {
		old, exists := previousByID[character.ID]
		if exists && sameCharacter(old, character) {
			continue
		}
		traits, err := json.Marshal(character.Traits)
		if err != nil {
			return fmt.Errorf("encode character %d traits: %w", character.ID, err)
		}
		if exists {
			if _, err := tx.Exec(`UPDATE characters SET name = ?, role = ?, description = ?, traits_json = ?, color = ?
				WHERE workspace_id = ? AND id = ?`, character.Name, character.Role, character.Description,
				string(traits), character.Color, workspaceID, character.ID); err != nil {
				return fmt.Errorf("update character %d: %w", character.ID, err)
			}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO characters (id, workspace_id, name, role, description, traits_json, color)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, character.ID, workspaceID, character.Name, character.Role,
			character.Description, string(traits), character.Color); err != nil {
			return fmt.Errorf("insert character %d: %w", character.ID, err)
		}
	}
	return nil
}

func sameCharacter(left, right Character) bool {
	if left.ID != right.ID || left.Name != right.Name || left.Role != right.Role || left.Description != right.Description || left.Color != right.Color || len(left.Traits) != len(right.Traits) {
		return false
	}
	for index := range left.Traits {
		if left.Traits[index] != right.Traits[index] {
			return false
		}
	}
	return true
}

func persistNoteDelta(tx *sql.Tx, workspaceID string, previous, current []Note) error {
	previousByID := make(map[int64]Note, len(previous))
	for _, note := range previous {
		previousByID[note.ID] = note
	}
	if err := deleteRemovedRows(tx, "notes", workspaceID, previous, current, func(note Note) int64 { return note.ID }); err != nil {
		return err
	}
	for _, note := range current {
		old, exists := previousByID[note.ID]
		if exists && old == note {
			continue
		}
		if exists {
			if _, err := tx.Exec(`UPDATE notes SET title = ?, body = ?, kind = ?, updated_at = ? WHERE workspace_id = ? AND id = ?`,
				note.Title, note.Body, note.Kind, note.UpdatedAt, workspaceID, note.ID); err != nil {
				return fmt.Errorf("update note %d: %w", note.ID, err)
			}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO notes (id, workspace_id, title, body, kind, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			note.ID, workspaceID, note.Title, note.Body, note.Kind, note.UpdatedAt); err != nil {
			return fmt.Errorf("insert note %d: %w", note.ID, err)
		}
	}
	return nil
}

func persistChatDelta(tx *sql.Tx, workspaceID string, previous, current []ChatMessage) error {
	previousByID := make(map[int64]ChatMessage, len(previous))
	for _, message := range previous {
		previousByID[message.ID] = message
	}
	if err := deleteRemovedRows(tx, "chat_messages", workspaceID, previous, current, func(message ChatMessage) int64 { return message.ID }); err != nil {
		return err
	}
	for _, message := range current {
		old, exists := previousByID[message.ID]
		if exists && old == message {
			continue
		}
		if exists {
			if _, err := tx.Exec(`UPDATE chat_messages SET role = ?, content = ?, created_at = ?, mode = ?, request_id = ?, request_fingerprint = ? WHERE workspace_id = ? AND id = ?`,
				message.Role, message.Content, message.CreatedAt, message.Mode, message.RequestID, message.RequestFingerprint, workspaceID, message.ID); err != nil {
				return fmt.Errorf("update chat message %d: %w", message.ID, err)
			}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO chat_messages (id, workspace_id, role, content, created_at, mode, request_id, request_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			message.ID, workspaceID, message.Role, message.Content, message.CreatedAt, message.Mode, message.RequestID, message.RequestFingerprint); err != nil {
			return fmt.Errorf("insert chat message %d: %w", message.ID, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM chat_messages WHERE workspace_id = ? AND id NOT IN (
		SELECT id FROM chat_messages WHERE workspace_id = ? ORDER BY id DESC LIMIT 60
	)`, workspaceID, workspaceID); err != nil {
		return fmt.Errorf("prune chat history: %w", err)
	}
	return nil
}

func deleteRemovedRows[T any](tx *sql.Tx, table, workspaceID string, previous, current []T, idOf func(T) int64) error {
	currentIDs := make(map[int64]struct{}, len(current))
	for _, item := range current {
		currentIDs[idOf(item)] = struct{}{}
	}
	for _, item := range previous {
		id := idOf(item)
		if _, remains := currentIDs[id]; remains {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE workspace_id = ? AND id = ?`, workspaceID, id); err != nil {
			return fmt.Errorf("delete %s row %d: %w", table, id, err)
		}
	}
	return nil
}
