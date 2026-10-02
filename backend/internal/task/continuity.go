package task

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// ContinuityFinding is a model-suggested inconsistency with separately
// recorded literal checks for the current passage and its supporting source.
type ContinuityFinding struct {
	Category               string `json:"category"`
	Subject                string `json:"subject"`
	Issue                  string `json:"issue"`
	CurrentQuote           string `json:"currentQuote"`
	ReferenceSource        string `json:"referenceSource"`
	ReferenceQuote         string `json:"referenceQuote"`
	Confidence             string `json:"confidence"`
	Status                 string `json:"status"`
	CurrentQuoteVerbatim   bool   `json:"currentQuoteVerbatim"`
	ReferenceQuoteVerbatim bool   `json:"referenceQuoteVerbatim"`
}

// ContinuityOutcome stores one chapter's read-only continuity review.
type ContinuityOutcome struct {
	ChapterID string              `json:"chapterId"`
	Items     []ContinuityFinding `json:"items"`
}

type parsedContinuityBlock struct {
	Category        string
	Subject         string
	Issue           string
	CurrentQuote    string
	ReferenceSource string
	ReferenceQuote  string
	Confidence      string
}

// ParseContinuityModel accepts labeled blocks and an explicit empty result.
// Unknown prose is not treated as a successful clean review.
func ParseContinuityModel(raw string) ([]parsedContinuityBlock, bool) {
	blocks := make([]parsedContinuityBlock, 0)
	var current *parsedContinuityBlock
	flush := func() {
		if current == nil {
			return
		}
		if strings.TrimSpace(current.Issue) != "" {
			blocks = append(blocks, *current)
		}
		current = nil
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "可能问题："):
			flush()
			current = &parsedContinuityBlock{Issue: strings.TrimSpace(strings.TrimPrefix(line, "可能问题："))}
		case current == nil:
			continue
		case strings.HasPrefix(line, "类别："):
			current.Category = strings.TrimSpace(strings.TrimPrefix(line, "类别："))
		case strings.HasPrefix(line, "涉及对象："):
			current.Subject = strings.TrimSpace(strings.TrimPrefix(line, "涉及对象："))
		case strings.HasPrefix(line, "问题说明："):
			current.Issue = strings.TrimSpace(strings.TrimPrefix(line, "问题说明："))
		case strings.HasPrefix(line, "正文引文："):
			current.CurrentQuote = strings.TrimSpace(strings.TrimPrefix(line, "正文引文："))
		case strings.HasPrefix(line, "依据来源："):
			current.ReferenceSource = strings.TrimSpace(strings.TrimPrefix(line, "依据来源："))
		case strings.HasPrefix(line, "依据引文："):
			current.ReferenceQuote = strings.TrimSpace(strings.TrimPrefix(line, "依据引文："))
		case strings.HasPrefix(line, "把握程度："):
			current.Confidence = strings.TrimSpace(strings.TrimPrefix(line, "把握程度："))
		}
	}
	flush()
	if len(blocks) > 0 {
		return blocks, true
	}
	noIssues := strings.Contains(raw, "未发现明显连续性问题") || strings.Contains(raw, "没有发现明显的连续性问题")
	return nil, noIssues
}

func buildContinuityPrompt(request Request, chapter Chapter) string {
	var prompt strings.Builder
	prompt.WriteString("连续性审查任务：\n")
	prompt.WriteString(request.Instruction)
	prompt.WriteString("\n\n只检查当前章节与已定稿章节记忆、相关人物/地点/设定图谱及伏笔之间的时间线、人物状态和设定冲突。所有结果只是供作者核对的提示；不要改写、续写或输出正文，也不要把猜测写成事实。\n")
	prompt.WriteString("每条问题按以下格式输出。正文引文必须是当前正文中的连续原文；依据来源必须是给定资料里可见的标题，依据引文必须是该来源中的连续原文。若冲突来自当前章节内部，可将依据来源写为“当前章节正文”，并引用另一处原文。证据缺失或无法逐字引用时仍可报告，但把握程度应为低，并留空无法提供的引文。类别和把握程度都只能各选一项。\n")
	prompt.WriteString("可能问题：简短标题\n类别：时间线 / 人物状态 / 设定冲突 / 其他（仅选一项）\n涉及对象：人物、地点或设定\n问题说明：可能的矛盾及原因\n正文引文：\n依据来源：\n依据引文：\n把握程度：高 / 中 / 低（仅选一项）\n")
	prompt.WriteString("若没有发现值得提醒的问题，只输出：未发现明显连续性问题。不要为了填满格式而制造问题。\n\n相关资料：\n")
	contextText := chapter.InjectedContext
	if strings.TrimSpace(contextText) == "" {
		contextText = request.Context
	}
	prompt.WriteString(contextText)
	prompt.WriteString("\n\n当前章节：第")
	prompt.WriteString(strconv.Itoa(chapter.Number))
	prompt.WriteString("章《")
	prompt.WriteString(chapter.Title)
	prompt.WriteString("》\n")
	prompt.WriteString("本章目标：")
	prompt.WriteString(chapter.Goal)
	prompt.WriteString("\n本章关键事件：")
	prompt.WriteString(chapter.KeyEvents)
	prompt.WriteString("\n当前正文：\n")
	prompt.WriteString(chapter.Body)
	return prompt.String()
}

// ScoreContinuityFindings keeps the model's suggestion visible while marking
// it as unverified unless both literal quotes can be found in their sources.
func ScoreContinuityFindings(chapter Chapter, contextText, modelText string) ([]ContinuityFinding, bool) {
	blocks, valid := ParseContinuityModel(modelText)
	if !valid {
		return nil, false
	}
	findings := make([]ContinuityFinding, 0, len(blocks))
	for _, block := range blocks {
		category := normalizeContinuityCategory(block.Category)
		confidence := normalizeContinuityConfidence(block.Confidence)
		currentVerbatim := QuoteIsVerbatim(chapter.Body, block.CurrentQuote)
		referenceVerbatim := false
		if strings.TrimSpace(block.ReferenceSource) == "当前章节正文" {
			referenceVerbatim = QuoteIsVerbatim(chapter.Body, block.ReferenceQuote)
		} else if block.ReferenceSource != "" && strings.Contains(contextText, block.ReferenceSource) {
			referenceVerbatim = QuoteIsVerbatim(contextText, block.ReferenceQuote)
		}
		status := "待核实"
		if currentVerbatim && referenceVerbatim {
			status = "引文可核对"
		}
		findings = append(findings, ContinuityFinding{
			Category:               category,
			Subject:                block.Subject,
			Issue:                  block.Issue,
			CurrentQuote:           block.CurrentQuote,
			ReferenceSource:        block.ReferenceSource,
			ReferenceQuote:         block.ReferenceQuote,
			Confidence:             confidence,
			Status:                 status,
			CurrentQuoteVerbatim:   currentVerbatim,
			ReferenceQuoteVerbatim: referenceVerbatim,
		})
	}
	return findings, true
}

func normalizeContinuityCategory(raw string) string {
	text := strings.TrimSpace(raw)
	switch {
	case strings.Contains(text, "时间线"), strings.Contains(text, "时间"):
		return "时间线"
	case strings.Contains(text, "人物状态"), strings.Contains(text, "人物"):
		return "人物状态"
	case strings.Contains(text, "设定冲突"), strings.Contains(text, "设定"):
		return "设定冲突"
	case strings.Contains(text, "其他"):
		return "其他"
	default:
		return "未分类"
	}
}

func normalizeContinuityConfidence(raw string) string {
	text := strings.TrimSpace(raw)
	switch {
	case strings.Contains(text, "高"):
		return "高"
	case strings.Contains(text, "中"):
		return "中"
	case strings.Contains(text, "低"):
		return "低"
	default:
		return "未说明"
	}
}

func (m *Manager) runContinuityCheck(ctx context.Context, request Request) {
	failedChapters := 0
	stopMessage := ""
	for index, chapter := range request.Chapters {
		if err := ctx.Err(); err != nil {
			m.finish(request.ID, "canceled", "任务已取消")
			return
		}
		m.emit(request.ID, "chapter.started", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title})
		prompt := buildContinuityPrompt(request, chapter)
		tokenBatch := newTokenBatcher(m, request.ID, chapter.ID)
		text, err, _ := m.generate(ctx, prompt, tokenBatch.add)
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
		contextText := chapter.InjectedContext
		if strings.TrimSpace(contextText) == "" {
			contextText = request.Context
		}
		findings, valid := ScoreContinuityFindings(chapter, contextText, strings.TrimSpace(text))
		if !valid {
			message := "模型返回的连续性审查格式无法识别，请重试"
			failedChapters++
			if stopMessage == "" {
				stopMessage = message
			}
			m.emit(request.ID, "chapter.failed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title, "message": message})
			m.advanceReview(request.ID, index, len(request.Chapters), chapter)
			continue
		}
		m.storeContinuityOutcome(&request, chapter, findings)
		m.emitContinuityFindings(request.ID, chapter, findings)
		m.emit(request.ID, "chapter.completed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title})
		m.advanceReview(request.ID, index, len(request.Chapters), chapter)
	}
	if failedChapters > 0 {
		m.finish(request.ID, "failed", stopMessage)
		return
	}
	m.finish(request.ID, "completed", "")
}

func (m *Manager) storeContinuityOutcome(request *Request, chapter Chapter, findings []ContinuityFinding) {
	if findings == nil {
		findings = []ContinuityFinding{}
	}
	request.ContinuityOutcomes = append(request.ContinuityOutcomes, ContinuityOutcome{ChapterID: chapter.ID, Items: findings})
	m.persistRequest(*request)
}

func (m *Manager) emitContinuityFindings(taskID string, chapter Chapter, findings []ContinuityFinding) {
	if findings == nil {
		findings = []ContinuityFinding{}
	}
	m.emit(taskID, "continuity.findings", map[string]any{
		"chapterId":     chapter.ID,
		"chapterNumber": chapter.Number,
		"title":         chapter.Title,
		"items":         findings,
	})
}
