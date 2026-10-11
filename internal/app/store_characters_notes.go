package app

import (
	"fmt"
	"os"
	"strings"
)

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
