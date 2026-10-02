package task

import (
	"context"
	"errors"
	"strings"
)

// ReviewFinding is one blueprint key event after a review task. Status is the
// stored checklist result. ModelJudgment is the model's own semantic slot and
// is not rewritten when the quotation fails the verbatim check. QuoteVerbatim
// is only that mechanical check.
type ReviewFinding struct {
	ChapterID     string
	Event         string
	Status        string
	Quote         string
	Location      string
	ModelJudgment string
	QuoteVerbatim bool
}

// ReviewOutcome is the checklist for one chapter. An empty Items list is a
// completed review of a chapter whose key events have no non-empty lines.
type ReviewOutcome struct {
	ChapterID string
	Items     []ReviewFinding
}

type parsedReviewBlock struct {
	Event       string
	Judgment    string
	Quote       string
	Location    string
	HasJudgment bool
}

// SplitKeyEvents treats each non-empty line of the blueprint field as one
// event. A line that joins several happenings with punctuation stays one event.
func SplitKeyEvents(text string) []string {
	events := make([]string, 0)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		events = append(events, line)
	}
	return events
}

// NormalizeReviewJudgment maps the model text onto one semantic slot.
// 「通过」is the same slot as「已完成」. 「不通过」is「未完成」, not a pass.
func NormalizeReviewJudgment(raw string) (string, bool) {
	text := strings.TrimSpace(raw)
	if index := strings.Index(text, "\n"); index >= 0 {
		text = strings.TrimSpace(text[:index])
	}
	switch {
	case strings.Contains(text, "未完成") || strings.Contains(text, "不通过"):
		return "未完成", true
	case strings.Contains(text, "待核实"):
		return "待核实", true
	case strings.Contains(text, "已完成") || strings.Contains(text, "通过"):
		return "已完成", true
	default:
		return "", false
	}
}

// QuoteIsVerbatim reports whether quote is a contiguous substring of body.
// A missing character, a rewording, or an empty quotation is not verbatim.
func QuoteIsVerbatim(body, quote string) bool {
	quote = strings.TrimSpace(quote)
	if quote == "" {
		return false
	}
	return strings.Contains(body, quote)
}

// ParseReviewModel reads labeled blocks. The first block for an event wins.
// Events the model does not mention are simply absent from the map.
func ParseReviewModel(raw string) map[string]parsedReviewBlock {
	found := make(map[string]parsedReviewBlock)
	var current *parsedReviewBlock
	flush := func() {
		if current == nil {
			return
		}
		event := strings.TrimSpace(current.Event)
		if event != "" {
			if _, exists := found[event]; !exists {
				found[event] = *current
			}
		}
		current = nil
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "关键事件："):
			flush()
			current = &parsedReviewBlock{Event: strings.TrimSpace(strings.TrimPrefix(line, "关键事件："))}
		case current == nil:
			continue
		case strings.HasPrefix(line, "判断："):
			current.Judgment = strings.TrimSpace(strings.TrimPrefix(line, "判断："))
			current.HasJudgment = true
		case strings.HasPrefix(line, "引文："):
			current.Quote = strings.TrimSpace(strings.TrimPrefix(line, "引文："))
		case strings.HasPrefix(line, "位置："):
			current.Location = strings.TrimSpace(strings.TrimPrefix(line, "位置："))
		}
	}
	flush()
	return found
}

// ScoreReviewChecklist builds one finding per blueprint line. A verbatim
// quotation does not by itself become「已完成」.「已完成」or「通过」with a
// quotation that is not a verbatim substring is stored as「待核实」.
func ScoreReviewChecklist(chapterID, body, keyEvents, modelText string) []ReviewFinding {
	events := SplitKeyEvents(keyEvents)
	parsed := ParseReviewModel(modelText)
	findings := make([]ReviewFinding, 0, len(events))
	for _, event := range events {
		block, mentioned := parsed[event]
		judgment := ""
		recognized := false
		if mentioned && block.HasJudgment {
			judgment, recognized = NormalizeReviewJudgment(block.Judgment)
		}
		quote := ""
		location := ""
		if mentioned {
			quote = strings.TrimSpace(block.Quote)
			location = strings.TrimSpace(block.Location)
		}
		verbatim := QuoteIsVerbatim(body, quote)
		status := "未完成"
		if recognized {
			status = judgment
			if judgment == "已完成" && !verbatim {
				status = "待核实"
			}
		} else {
			judgment = ""
		}
		findings = append(findings, ReviewFinding{
			ChapterID:     chapterID,
			Event:         event,
			Status:        status,
			Quote:         quote,
			Location:      location,
			ModelJudgment: judgment,
			QuoteVerbatim: verbatim,
		})
	}
	return findings
}

func buildReviewPrompt(request Request, chapter Chapter, events []string) string {
	var prompt strings.Builder
	prompt.WriteString("审稿任务：\n")
	prompt.WriteString(request.Instruction)
	prompt.WriteString("\n\n请只按下面的关键事件清单逐条审稿。不要输出章节正文，不要提出改写。")
	prompt.WriteString("\n每一条单独成段，关键事件一行必须与清单中的那一行完全相同。格式：\n")
	prompt.WriteString("关键事件：\n判断：已完成\n或\n判断：未完成\n或\n判断：待核实\n引文：\n位置：\n")
	prompt.WriteString("判断写成已完成或通过时，引文必须是当前正文里连续出现的原文。没有逐字引文时不要写成已完成。")
	prompt.WriteString("\n\n关键事件清单：\n")
	for _, event := range events {
		prompt.WriteString(event)
		prompt.WriteString("\n")
	}
	prompt.WriteString("\n当前正文：\n")
	prompt.WriteString(chapter.Body)
	return prompt.String()
}

func (m *Manager) runReviewChecklist(ctx context.Context, request Request) {
	failedChapters := 0
	stopMessage := ""
	for index, chapter := range request.Chapters {
		if err := ctx.Err(); err != nil {
			m.finish(request.ID, "canceled", "任务已取消")
			return
		}
		m.emit(request.ID, "chapter.started", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title})
		events := SplitKeyEvents(chapter.KeyEvents)
		if len(events) == 0 {
			m.storeReviewOutcome(&request, chapter, []ReviewFinding{})
			m.emitReviewChecklist(request.ID, chapter, nil)
			m.emit(request.ID, "chapter.completed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title})
			m.advanceReview(request.ID, index, len(request.Chapters), chapter)
			continue
		}
		prompt := buildReviewPrompt(request, chapter, events)
		tokenBatch := newTokenBatcher(m, request.ID, chapter.ID)
		text, err, _ := m.generate(ctx, request.ProviderID, prompt, tokenBatch.add)
		tokenBatch.flush()
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				m.emit(request.ID, "chapter.canceled", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number})
				m.finish(request.ID, "canceled", "任务已取消")
				return
			}
			message := generationFailureMessage(err)
			failedChapters++
			if stopMessage == "" {
				stopMessage = message
			}
			m.emit(request.ID, "chapter.failed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title, "message": message})
			m.advanceReview(request.ID, index, len(request.Chapters), chapter)
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			message := "模型没有返回可用内容"
			failedChapters++
			if stopMessage == "" {
				stopMessage = message
			}
			m.emit(request.ID, "chapter.failed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title, "message": message})
			m.advanceReview(request.ID, index, len(request.Chapters), chapter)
			continue
		}
		findings := ScoreReviewChecklist(chapter.ID, chapter.Body, chapter.KeyEvents, text)
		m.storeReviewOutcome(&request, chapter, findings)
		m.emitReviewChecklist(request.ID, chapter, findings)
		m.emit(request.ID, "chapter.completed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title})
		m.advanceReview(request.ID, index, len(request.Chapters), chapter)
	}
	if failedChapters > 0 {
		m.finish(request.ID, "failed", stopMessage)
		return
	}
	m.finish(request.ID, "completed", "")
}

func (m *Manager) storeReviewOutcome(request *Request, chapter Chapter, findings []ReviewFinding) {
	if findings == nil {
		findings = []ReviewFinding{}
	}
	request.ReviewOutcomes = append(request.ReviewOutcomes, ReviewOutcome{ChapterID: chapter.ID, Items: findings})
	m.persistRequest(*request)
}

func (m *Manager) emitReviewChecklist(taskID string, chapter Chapter, findings []ReviewFinding) {
	if findings == nil {
		findings = []ReviewFinding{}
	}
	items := make([]map[string]any, 0, len(findings))
	for _, finding := range findings {
		items = append(items, map[string]any{
			"event":         finding.Event,
			"status":        finding.Status,
			"quote":         finding.Quote,
			"location":      finding.Location,
			"modelJudgment": finding.ModelJudgment,
			"quoteVerbatim": finding.QuoteVerbatim,
		})
	}
	m.emit(taskID, "review.checklist", map[string]any{
		"chapterId":     chapter.ID,
		"chapterNumber": chapter.Number,
		"title":         chapter.Title,
		"items":         items,
	})
}

func (m *Manager) advanceReview(taskID string, index, total int, chapter Chapter) {
	progress := (index + 1) * 100 / total
	m.update(taskID, "running", progress, "")
	m.emit(taskID, "task.progress", map[string]any{"progress": progress, "chapterId": chapter.ID})
}
