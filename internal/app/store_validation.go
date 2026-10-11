package app

import (
	"fmt"
	"strings"
)

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
