package task

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"strings"

	"novelstudio/internal/database"
)

const (
	// RecentSummaryWindow is the number of earlier chapter summaries a
	// generation may see. It is wider than the 600-rune previous-chapter tail
	// and still bounded for a novel of 50 or more chapters.
	RecentSummaryWindow = 8
	// DefaultContextBudgetRunes is the injected-context cap used when the
	// process has not configured CONTEXT_BUDGET_RUNES.
	DefaultContextBudgetRunes = 12000
)

// ChapterMemory is the structured record stored for one finalized chapter.
type ChapterMemory struct {
	Summary       string
	Characters    string
	NewSettings   string
	Foreshadowing string
}

// StoredMemory is one current chapter memory joined with the live chapter title.
type StoredMemory struct {
	ChapterID     string
	ChapterNumber int
	Title         string
	Summary       string
	Characters    string
	NewSettings   string
	Foreshadowing string
}

// StoredThread is one foreshadowing item projected from chapter memories.
type StoredThread struct {
	Name                string
	Detail              string
	Subjects            string
	Status              string
	OpenedChapterNumber int
}

// StoredEntity is a knowledge-graph node considered for prompt injection.
type StoredEntity struct {
	Kind    string
	Label   string
	Summary string
}

// StoredRelation is a knowledge-graph edge considered for prompt injection.
type StoredRelation struct {
	Source   string
	Relation string
	Target   string
	Summary  string
}

// AssembledContext is the budgeted prompt block for one chapter, plus the
// names of what was injected and what was left out.
type AssembledContext struct {
	Text               string
	Included           []string
	Dropped            []string
	TruncatedEntities  int
	TruncatedRelations int
}

type contextPiece struct {
	Label string
	Body  string
	Kind  string
}

type foreshadowChange struct {
	Recover  bool
	Name     string
	Subjects string
	Detail   string
}

type queryContext interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// MemoryPrompt asks the process generator for the four memory fields.
func MemoryPrompt(chapter Chapter, body string) string {
	var prompt strings.Builder
	prompt.WriteString("请只输出章节记忆，不要输出正文。\n第")
	prompt.WriteString(strconv.Itoa(chapter.Number))
	prompt.WriteString("章《")
	prompt.WriteString(chapter.Title)
	prompt.WriteString("》\n")
	prompt.WriteString(body)
	prompt.WriteString("\n\n按以下格式输出，不要添加其他说明：\n摘要：\n出场人物：\n新增设定：\n伏笔变化：\n+名称|相关人物或地点|细节\n-名称\n")
	return prompt.String()
}

// ParseChapterMemory reads the four fields. Prose that does not use the
// labels is kept as the summary so a finalize still stores a current row.
func ParseChapterMemory(raw string) ChapterMemory {
	raw = strings.TrimSpace(raw)
	labels := []string{"摘要：", "出场人物：", "新增设定：", "伏笔变化："}
	found := map[string]string{}
	var current string
	var buf strings.Builder
	recognized := false
	flush := func() {
		if current == "" {
			return
		}
		found[current] = strings.TrimSpace(buf.String())
		buf.Reset()
	}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		matched := ""
		for _, label := range labels {
			if strings.HasPrefix(trimmed, label) {
				matched = label
				break
			}
		}
		if matched != "" {
			recognized = true
			flush()
			current = matched
			buf.WriteString(strings.TrimSpace(strings.TrimPrefix(trimmed, matched)))
			continue
		}
		if current == "" || trimmed == "" {
			continue
		}
		if buf.Len() > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(trimmed)
	}
	flush()
	if !recognized {
		return ChapterMemory{Summary: raw}
	}
	return ChapterMemory{
		Summary:       found["摘要："],
		Characters:    found["出场人物："],
		NewSettings:   found["新增设定："],
		Foreshadowing: found["伏笔变化："],
	}
}

// ValidChapterMemoryOutput reports whether the model returned the structured
// fields requested for a finalized chapter. A body or refusal must not be
// mistaken for a chapter summary and injected into future prompts.
func ValidChapterMemoryOutput(raw string) bool {
	seen := make(map[string]bool, 4)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		for _, label := range []string{"摘要：", "出场人物：", "新增设定：", "伏笔变化："} {
			if strings.HasPrefix(line, label) {
				seen[label] = true
			}
		}
	}
	return len(seen) == 4
}

// SaveChapterMemory replaces the current memory for a chapter and rebuilds
// foreshadowing state from every current memory of the novel.
func SaveChapterMemory(ctx context.Context, tx *sql.Tx, novelID, chapterID string, chapterNumber, sourceRevision int, memory ChapterMemory) error {
	now := database.Now()
	result, err := tx.ExecContext(ctx, `UPDATE chapter_memories
		SET novel_id = ?, chapter_number = ?, summary = ?, characters = ?, new_settings = ?, foreshadowing = ?, source_revision = ?, updated_at = ?
		WHERE chapter_id = ?`,
		novelID, chapterNumber, memory.Summary, memory.Characters, memory.NewSettings, memory.Foreshadowing, sourceRevision, now, chapterID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO chapter_memories (
			chapter_id, novel_id, chapter_number, summary, characters, new_settings, foreshadowing, source_revision, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			chapterID, novelID, chapterNumber, memory.Summary, memory.Characters, memory.NewSettings, memory.Foreshadowing, sourceRevision, now); err != nil {
			return err
		}
	}
	return rebuildNarrativeThreads(ctx, tx, novelID)
}

// RebuildNarrativeThreads refreshes the novel projection after a chapter body
// changes, excluding memories whose source revision no longer matches.
func RebuildNarrativeThreads(ctx context.Context, tx *sql.Tx, novelID string) error {
	return rebuildNarrativeThreads(ctx, tx, novelID)
}

func rebuildNarrativeThreads(ctx context.Context, tx *sql.Tx, novelID string) error {
	rows, err := tx.QueryContext(ctx, `SELECT m.chapter_id, c.chapter_number, m.foreshadowing
		FROM chapter_memories m
		JOIN chapters c ON c.id = m.chapter_id AND c.novel_id = m.novel_id
		WHERE m.novel_id = ? AND c.deleted_at IS NULL AND m.source_revision = c.revision
		ORDER BY c.chapter_number, m.chapter_id`, novelID)
	if err != nil {
		return err
	}
	type memoryForeshadowing struct {
		chapterID     string
		chapterNumber int
		text          string
	}
	memories := make([]memoryForeshadowing, 0)
	for rows.Next() {
		var item memoryForeshadowing
		if err := rows.Scan(&item.chapterID, &item.chapterNumber, &item.text); err != nil {
			rows.Close()
			return err
		}
		memories = append(memories, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	type thread struct {
		name                 string
		detail               string
		subjects             string
		status               string
		openedChapterID      string
		openedChapterNumber  int
		updatedChapterID     string
		updatedChapterNumber int
	}
	threads := make(map[string]*thread)
	order := make([]string, 0)
	for _, memory := range memories {
		for _, change := range parseForeshadowingChanges(memory.text) {
			current := threads[change.Name]
			if current == nil {
				current = &thread{
					name:                change.Name,
					openedChapterID:     memory.chapterID,
					openedChapterNumber: memory.chapterNumber,
				}
				threads[change.Name] = current
				order = append(order, change.Name)
			}
			current.updatedChapterID = memory.chapterID
			current.updatedChapterNumber = memory.chapterNumber
			if change.Recover {
				current.status = "recovered"
				continue
			}
			current.status = "open"
			current.detail = change.Detail
			current.subjects = change.Subjects
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM narrative_threads WHERE novel_id = ?`, novelID); err != nil {
		return err
	}
	for _, name := range order {
		item := threads[name]
		if item.status == "" {
			item.status = "open"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO narrative_threads (
			id, novel_id, name, detail, subjects, status, opened_chapter_id, opened_chapter_number, updated_chapter_id, updated_chapter_number
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			database.NewID(), novelID, item.name, item.detail, item.subjects, item.status,
			item.openedChapterID, item.openedChapterNumber, item.updatedChapterID, item.updatedChapterNumber); err != nil {
			return err
		}
	}
	return nil
}

func parseForeshadowingChanges(text string) []foreshadowChange {
	changes := make([]foreshadowChange, 0)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || (line[0] != '+' && line[0] != '-') {
			continue
		}
		recover := line[0] == '-'
		rest := strings.TrimSpace(line[1:])
		parts := strings.Split(rest, "|")
		if len(parts) == 1 {
			parts = strings.Split(rest, "｜")
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		change := foreshadowChange{Recover: recover, Name: name}
		if len(parts) > 1 {
			change.Subjects = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			change.Detail = strings.TrimSpace(strings.Join(parts[2:], "|"))
		}
		changes = append(changes, change)
	}
	return changes
}

// LoadChapterMemories returns current memories in chapter order.
func LoadChapterMemories(ctx context.Context, db queryContext, novelID string) ([]StoredMemory, error) {
	rows, err := db.QueryContext(ctx, `SELECT m.chapter_id, c.chapter_number, c.title, m.summary, m.characters, m.new_settings, m.foreshadowing
		FROM chapter_memories m
		JOIN chapters c ON c.id = m.chapter_id AND c.novel_id = m.novel_id
		WHERE m.novel_id = ? AND c.deleted_at IS NULL AND m.source_revision = c.revision
		ORDER BY c.chapter_number, m.chapter_id`, novelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memories := make([]StoredMemory, 0)
	for rows.Next() {
		var item StoredMemory
		if err := rows.Scan(&item.ChapterID, &item.ChapterNumber, &item.Title, &item.Summary, &item.Characters, &item.NewSettings, &item.Foreshadowing); err != nil {
			return nil, err
		}
		memories = append(memories, item)
	}
	return memories, rows.Err()
}

// LoadNarrativeThreads returns foreshadowing projected from current memories.
func LoadNarrativeThreads(ctx context.Context, db queryContext, novelID string) ([]StoredThread, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, detail, subjects, status, opened_chapter_number
		FROM narrative_threads WHERE novel_id = ? ORDER BY opened_chapter_number, name`, novelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	threads := make([]StoredThread, 0)
	for rows.Next() {
		var item StoredThread
		if err := rows.Scan(&item.Name, &item.Detail, &item.Subjects, &item.Status, &item.OpenedChapterNumber); err != nil {
			return nil, err
		}
		threads = append(threads, item)
	}
	return threads, rows.Err()
}

// AssembleChapterContext selects the recent summary window, blueprint-matched
// people and places, and unrecovered foreshadowing, then slices each section
// to its own share of budget so one section cannot consume the others.
func AssembleChapterContext(budget int, chapterNumber int, blueprint, header string, memories []StoredMemory, entities []StoredEntity, relations []StoredRelation, threads []StoredThread) AssembledContext {
	if budget < 0 {
		budget = 0
	}
	header = strings.TrimSpace(header)
	if header != "" {
		header += "\n"
	}
	headerRunes := len([]rune(header))
	if headerRunes > budget {
		header = string([]rune(header)[:budget])
		headerRunes = budget
	}
	shares := splitBudget(budget-headerRunes, 3)
	matched := matchedPeopleAndPlaces(blueprint, entities)

	dropped := make([]string, 0)
	summaryPieces := make([]contextPiece, 0)
	earlier := make([]StoredMemory, 0)
	for _, memory := range memories {
		if memory.ChapterNumber >= chapterNumber || strings.TrimSpace(memory.Summary) == "" {
			continue
		}
		earlier = append(earlier, memory)
	}
	sort.Slice(earlier, func(i, j int) bool {
		if earlier[i].ChapterNumber != earlier[j].ChapterNumber {
			return earlier[i].ChapterNumber > earlier[j].ChapterNumber
		}
		return earlier[i].ChapterID > earlier[j].ChapterID
	})
	if len(earlier) > RecentSummaryWindow {
		for _, memory := range earlier[RecentSummaryWindow:] {
			dropped = append(dropped, dropLabel(summaryLabel(memory), "超出近章窗口"))
		}
		earlier = earlier[:RecentSummaryWindow]
	}
	for _, memory := range earlier {
		summaryPieces = append(summaryPieces, contextPiece{
			Label: summaryLabel(memory),
			Body:  "第" + strconv.Itoa(memory.ChapterNumber) + "章《" + memory.Title + "》：" + strings.TrimSpace(memory.Summary) + "\n",
			Kind:  "summary",
		})
	}

	matchedSet := make(map[string]bool, len(matched))
	for _, label := range matched {
		matchedSet[label] = true
	}
	graphPieces := make([]contextPiece, 0)
	for _, entity := range entities {
		label := strings.TrimSpace(entity.Label)
		manifest := kindLabel(entity.Kind) + "：" + label
		if label != "" && matchedSet[label] && (entity.Kind == "character" || entity.Kind == "place") {
			graphPieces = append(graphPieces, contextPiece{
				Label: manifest,
				Body:  entity.Kind + "：" + label + "。" + entity.Summary + "\n",
				Kind:  "entity",
			})
			continue
		}
		dropped = append(dropped, dropLabel(manifest, "与本章蓝图无关"))
	}
	for _, relation := range relations {
		manifest := "关系：" + relation.Source + " —" + relation.Relation + "→ " + relation.Target
		if matchedSet[strings.TrimSpace(relation.Source)] || matchedSet[strings.TrimSpace(relation.Target)] {
			graphPieces = append(graphPieces, contextPiece{
				Label: manifest,
				Body:  relation.Source + " —" + relation.Relation + "→ " + relation.Target + "。" + relation.Summary + "\n",
				Kind:  "relation",
			})
			continue
		}
		dropped = append(dropped, dropLabel(manifest, "与本章蓝图无关"))
	}

	threadPieces := make([]contextPiece, 0)
	relevantThreads := make([]StoredThread, 0)
	for _, thread := range threads {
		manifest := "伏笔：" + thread.Name
		if thread.Status == "recovered" {
			dropped = append(dropped, dropLabel(manifest, "已回收"))
			continue
		}
		if !mentionsAny(thread, matched) {
			dropped = append(dropped, dropLabel(manifest, "与本章蓝图无关"))
			continue
		}
		relevantThreads = append(relevantThreads, thread)
	}
	sort.Slice(relevantThreads, func(i, j int) bool {
		if relevantThreads[i].OpenedChapterNumber != relevantThreads[j].OpenedChapterNumber {
			return relevantThreads[i].OpenedChapterNumber < relevantThreads[j].OpenedChapterNumber
		}
		return relevantThreads[i].Name < relevantThreads[j].Name
	})
	for _, thread := range relevantThreads {
		detail := strings.TrimSpace(thread.Detail)
		body := "伏笔：" + thread.Name
		if detail != "" {
			body += "。" + detail
		}
		body += "\n"
		threadPieces = append(threadPieces, contextPiece{Label: "伏笔：" + thread.Name, Body: body, Kind: "thread"})
	}

	summaryText, summaryKept, summaryDropped := fitSection("近期章节摘要：", summaryPieces, shares[0])
	graphText, graphKept, graphDropped := fitSection("相关知识图谱：", graphPieces, shares[1])
	threadText, threadKept, threadDropped := fitSection("未回收伏笔：", threadPieces, shares[2])
	for _, piece := range summaryDropped {
		dropped = append(dropped, dropLabel(piece.Label, "超出上下文预算"))
	}
	truncatedEntities := 0
	truncatedRelations := 0
	for _, piece := range graphDropped {
		dropped = append(dropped, dropLabel(piece.Label, "超出上下文预算"))
		switch piece.Kind {
		case "entity":
			truncatedEntities++
		case "relation":
			truncatedRelations++
		}
	}
	for _, piece := range threadDropped {
		dropped = append(dropped, dropLabel(piece.Label, "超出上下文预算"))
	}

	included := make([]string, 0, len(summaryKept)+len(graphKept)+len(threadKept))
	for _, piece := range summaryKept {
		included = append(included, piece.Label)
	}
	for _, piece := range graphKept {
		included = append(included, piece.Label)
	}
	for _, piece := range threadKept {
		included = append(included, piece.Label)
	}
	text := header + summaryText + graphText + threadText
	if len([]rune(text)) > budget {
		text = string([]rune(text)[:budget])
	}
	return AssembledContext{
		Text:               text,
		Included:           included,
		Dropped:            dropped,
		TruncatedEntities:  truncatedEntities,
		TruncatedRelations: truncatedRelations,
	}
}

// AssembleContinuityContext selects current graph facts and finalized chapter
// memories using the chapter text itself as the relevance query. Unlike the
// generation context, it includes the structured character and setting fields
// from memories so the reviewer can compare those details against the draft.
func AssembleContinuityContext(budget, chapterNumber int, chapterText, header string, memories []StoredMemory, entities []StoredEntity, relations []StoredRelation, threads []StoredThread) AssembledContext {
	if budget < 0 {
		budget = 0
	}
	header = strings.TrimSpace(header)
	if header != "" {
		header += "\n"
	}
	headerRunes := len([]rune(header))
	if headerRunes > budget {
		header = string([]rune(header)[:budget])
		headerRunes = budget
	}
	shares := splitBudget(budget-headerRunes, 2)

	matchedLabels := make([]string, 0)
	matchedSet := make(map[string]bool)
	graphPieces := make([]contextPiece, 0)
	for _, entity := range entities {
		label := strings.TrimSpace(entity.Label)
		if label == "" || !strings.Contains(chapterText, label) {
			continue
		}
		if !matchedSet[label] {
			matchedSet[label] = true
			matchedLabels = append(matchedLabels, label)
		}
		manifest := "图谱" + kindLabel(entity.Kind) + "设定：" + label
		graphPieces = append(graphPieces, contextPiece{
			Label: manifest,
			Body:  manifest + "\n事实：" + strings.TrimSpace(entity.Summary) + "\n",
			Kind:  "entity",
		})
	}
	for _, relation := range relations {
		if !matchedSet[strings.TrimSpace(relation.Source)] && !matchedSet[strings.TrimSpace(relation.Target)] {
			continue
		}
		manifest := "图谱关系：" + relation.Source + " —" + relation.Relation + "→ " + relation.Target
		graphPieces = append(graphPieces, contextPiece{
			Label: manifest,
			Body:  manifest + "\n事实：" + strings.TrimSpace(relation.Summary) + "\n",
			Kind:  "relation",
		})
	}
	for _, thread := range threads {
		if thread.Status == "recovered" || strings.TrimSpace(thread.Name) == "" {
			continue
		}
		relevant := strings.Contains(chapterText, thread.Name)
		for _, label := range matchedLabels {
			if strings.Contains(thread.Name+"\n"+thread.Subjects+"\n"+thread.Detail, label) {
				relevant = true
				break
			}
		}
		if !relevant {
			continue
		}
		manifest := "未回收伏笔：" + thread.Name
		body := manifest
		if detail := strings.TrimSpace(thread.Detail); detail != "" {
			body += "\n细节：" + detail
		}
		if subjects := strings.TrimSpace(thread.Subjects); subjects != "" {
			body += "\n相关人物或地点：" + subjects
		}
		graphPieces = append(graphPieces, contextPiece{Label: manifest, Body: body + "\n", Kind: "thread"})
	}

	type rankedMemory struct {
		memory StoredMemory
		score  int
	}
	prior := make([]rankedMemory, 0, len(memories))
	for _, memory := range memories {
		if memory.ChapterNumber >= chapterNumber {
			continue
		}
		score := 0
		for _, label := range matchedLabels {
			if strings.Contains(memory.Characters, label) {
				score += 4
			}
			if strings.Contains(memory.Summary, label) {
				score += 2
			}
			if strings.Contains(memory.NewSettings+"\n"+memory.Foreshadowing, label) {
				score++
			}
		}
		prior = append(prior, rankedMemory{memory: memory, score: score})
	}
	sort.Slice(prior, func(i, j int) bool {
		if prior[i].score != prior[j].score {
			return prior[i].score > prior[j].score
		}
		if prior[i].memory.ChapterNumber != prior[j].memory.ChapterNumber {
			return prior[i].memory.ChapterNumber > prior[j].memory.ChapterNumber
		}
		return prior[i].memory.ChapterID > prior[j].memory.ChapterID
	})
	if len(matchedLabels) == 0 {
		if len(prior) > RecentSummaryWindow {
			prior = prior[:RecentSummaryWindow]
		}
	} else {
		selected := make([]rankedMemory, 0, len(prior))
		seen := make(map[string]bool)
		for _, item := range prior {
			if item.score > 0 {
				selected = append(selected, item)
				seen[item.memory.ChapterID] = true
			}
		}
		fallbacks := 0
		for _, item := range prior {
			if seen[item.memory.ChapterID] || fallbacks >= 4 {
				continue
			}
			selected = append(selected, item)
			seen[item.memory.ChapterID] = true
			fallbacks++
		}
		prior = selected
	}
	memoryPieces := make([]contextPiece, 0, len(prior))
	for _, item := range prior {
		memory := item.memory
		manifest := "第" + strconv.Itoa(memory.ChapterNumber) + "章《" + memory.Title + "》定稿记忆"
		body := manifest + "\n"
		if memory.Summary != "" {
			body += "摘要：" + memory.Summary + "\n"
		}
		if memory.Characters != "" {
			body += "出场人物：" + memory.Characters + "\n"
		}
		if memory.NewSettings != "" {
			body += "新增设定：" + memory.NewSettings + "\n"
		}
		if memory.Foreshadowing != "" {
			body += "伏笔变化：" + memory.Foreshadowing + "\n"
		}
		memoryPieces = append(memoryPieces, contextPiece{Label: manifest, Body: body, Kind: "memory"})
	}

	memoryText, memoryKept, memoryDropped := fitSection("相关定稿章节记忆：", memoryPieces, shares[0])
	graphText, graphKept, graphDropped := fitSection("相关图谱事实与伏笔：", graphPieces, shares[1])
	dropped := make([]string, 0, len(memoryDropped)+len(graphDropped))
	for _, piece := range memoryDropped {
		dropped = append(dropped, dropLabel(piece.Label, "超出上下文预算"))
	}
	truncatedEntities := 0
	truncatedRelations := 0
	for _, piece := range graphDropped {
		dropped = append(dropped, dropLabel(piece.Label, "超出上下文预算"))
		switch piece.Kind {
		case "entity":
			truncatedEntities++
		case "relation":
			truncatedRelations++
		}
	}
	included := make([]string, 0, len(memoryKept)+len(graphKept))
	for _, piece := range memoryKept {
		included = append(included, piece.Label)
	}
	for _, piece := range graphKept {
		included = append(included, piece.Label)
	}
	text := header + memoryText + graphText
	if len([]rune(text)) > budget {
		text = string([]rune(text)[:budget])
	}
	return AssembledContext{
		Text:               text,
		Included:           included,
		Dropped:            dropped,
		TruncatedEntities:  truncatedEntities,
		TruncatedRelations: truncatedRelations,
	}
}

func splitBudget(total, parts int) []int {
	shares := make([]int, parts)
	if total <= 0 || parts <= 0 {
		return shares
	}
	base := total / parts
	remainder := total % parts
	for index := range shares {
		shares[index] = base
		if index < remainder {
			shares[index]++
		}
	}
	return shares
}

func fitSection(title string, pieces []contextPiece, share int) (string, []contextPiece, []contextPiece) {
	if len(pieces) == 0 || share <= 0 {
		return "", nil, pieces
	}
	header := title
	if !strings.HasSuffix(header, "\n") {
		header += "\n"
	}
	headerLen := len([]rune(header))
	if headerLen >= share {
		return "", nil, pieces
	}
	kept, dropped, body := takePieces(pieces, share-headerLen)
	if body == "" {
		return "", nil, pieces
	}
	return header + body, kept, dropped
}

func takePieces(pieces []contextPiece, share int) ([]contextPiece, []contextPiece, string) {
	if share <= 0 {
		return nil, pieces, ""
	}
	var builder strings.Builder
	used := 0
	kept := make([]contextPiece, 0)
	for index, piece := range pieces {
		block := piece.Body
		if strings.TrimSpace(block) == "" {
			continue
		}
		if !strings.HasSuffix(block, "\n") {
			block += "\n"
		}
		need := len([]rune(block))
		if used+need <= share {
			builder.WriteString(block)
			used += need
			kept = append(kept, piece)
			continue
		}
		if len(kept) == 0 {
			room := share - used
			if room > 0 {
				builder.WriteString(string([]rune(block)[:room]))
				kept = append(kept, piece)
			}
			return kept, pieces[index+1:], builder.String()
		}
		return kept, pieces[index:], builder.String()
	}
	return kept, nil, builder.String()
}

func matchedPeopleAndPlaces(blueprint string, entities []StoredEntity) []string {
	labels := make([]string, 0)
	seen := make(map[string]bool)
	for _, entity := range entities {
		if entity.Kind != "character" && entity.Kind != "place" {
			continue
		}
		label := strings.TrimSpace(entity.Label)
		if label == "" || seen[label] || !strings.Contains(blueprint, label) {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	return labels
}

func mentionsAny(thread StoredThread, labels []string) bool {
	haystack := thread.Name + "\n" + thread.Subjects + "\n" + thread.Detail
	for _, label := range labels {
		if label != "" && strings.Contains(haystack, label) {
			return true
		}
	}
	return false
}

func summaryLabel(memory StoredMemory) string {
	return "摘要：第" + strconv.Itoa(memory.ChapterNumber) + "章《" + memory.Title + "》"
}

func dropLabel(label, reason string) string {
	return label + "（" + reason + "）"
}

func kindLabel(kind string) string {
	switch kind {
	case "character":
		return "人物"
	case "place":
		return "地点"
	case "faction":
		return "组织"
	case "item":
		return "物品"
	case "concept":
		return "设定"
	default:
		return "实体"
	}
}
