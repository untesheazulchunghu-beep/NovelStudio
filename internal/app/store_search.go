package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
