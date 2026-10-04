package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"

	"novelstudio/internal/database"
)

const (
	// MaxReviewReturns is how many times a failing review may send execution
	// back to the preceding generate node. The next failure stops the run.
	MaxReviewReturns = 2
	// ReviewLimitMessage is stored on the task when that limit is exceeded.
	ReviewLimitMessage = "审稿未通过，已达到返回上限，工作流已停止"
)

type WorkflowNode struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Prompt     string `json:"prompt"`
	ProviderID string `json:"providerId,omitempty"`
}

// NodeResult is a node that already succeeded for one chapter. Retry reads
// these from the task snapshot and does not run them again.
type NodeResult struct {
	ChapterID string
	NodeID    string
	Status    string
	Output    string
	Demo      bool
}

const (
	workflowOK = iota
	workflowFailed
	workflowCanceled
)

func (m *Manager) runWorkflowGraph(ctx context.Context, request Request) {
	var previousGenerated Chapter
	hasPreviousGenerated := false
	failedChapters := 0
	stopMessage := ""
	for index, chapter := range request.Chapters {
		if err := ctx.Err(); err != nil {
			m.finish(request.ID, "canceled", "任务已取消")
			return
		}
		m.emit(request.ID, "chapter.started", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title})
		promptChapter := chapter
		if hasPreviousGenerated && previousGenerated.Number+1 == chapter.Number {
			promptChapter.PreviousTail = tailRunes(previousGenerated.Body, 600)
		}
		text, message, stop := m.executeWorkflowNodes(ctx, &request, promptChapter)
		switch stop {
		case workflowCanceled:
			m.emit(request.ID, "chapter.canceled", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number})
			m.finish(request.ID, "canceled", "任务已取消")
			return
		case workflowFailed:
			failedChapters++
			if message == "" {
				message = "工作流节点失败"
			}
			if stopMessage == "" {
				stopMessage = message
			}
			m.emit(request.ID, "chapter.failed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title, "message": message})
			progress := (index + 1) * 100 / len(request.Chapters)
			m.update(request.ID, "running", progress, "")
			m.emit(request.ID, "task.progress", map[string]any{"progress": progress, "chapterId": chapter.ID})
			m.finish(request.ID, "failed", stopMessage)
			return
		default:
			m.emit(request.ID, "chapter.completed", map[string]any{"chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title})
			if strings.TrimSpace(text) != "" {
				previousGenerated = Chapter{Number: chapter.Number, Body: text}
				hasPreviousGenerated = true
			}
			progress := (index + 1) * 100 / len(request.Chapters)
			m.update(request.ID, "running", progress, "")
			m.emit(request.ID, "task.progress", map[string]any{"progress": progress, "chapterId": chapter.ID})
		}
	}
	if failedChapters > 0 {
		m.finish(request.ID, "failed", stopMessage)
		return
	}
	m.finish(request.ID, "completed", "")
}

// executeWorkflowNodes runs the ordered nodes for one chapter. A review fail
// jumps back to the preceding generate node; that return is not a graph edge.
func (m *Manager) executeWorkflowNodes(ctx context.Context, request *Request, chapter Chapter) (string, string, int) {
	outputs := make(map[string]NodeResult, len(request.Nodes))
	skip := make(map[string]bool, len(request.Nodes))
	for _, item := range request.NodeResults {
		if item.ChapterID == chapter.ID && item.Status == "succeeded" {
			outputs[item.NodeID] = item
			skip[item.NodeID] = true
		}
	}
	returns := 0
	lastText := ""
	lastDemo := m.demoModeFor(request.ProviderID)
	lastProvider := request.ProviderID
	stored := false
	for index := 0; index < len(request.Nodes); {
		if err := ctx.Err(); err != nil {
			return "", "任务已取消", workflowCanceled
		}
		node := request.Nodes[index]
		nodeProvider := request.ProviderID
		if node.ProviderID != "" {
			nodeProvider = node.ProviderID
		}
		if skip[node.ID] {
			saved := outputs[node.ID]
			if node.Kind == "generate" || node.Kind == "output" {
				lastText = saved.Output
				lastDemo = saved.Demo
				if node.Kind == "generate" {
					lastProvider = nodeProvider
				}
			}
			index++
			continue
		}
		m.emitNode(request.ID, "node.started", chapter, node, "", "")
		switch node.Kind {
		case "context":
			output := contextNodeText(node, chapter, *request)
			m.emitNode(request.ID, "node.finished", chapter, node, "succeeded", "")
			m.rememberNode(request, chapter.ID, node.ID, output, false)
			outputs[node.ID] = NodeResult{ChapterID: chapter.ID, NodeID: node.ID, Status: "succeeded", Output: output}
			index++
		case "generate":
			upstream := upstreamText(request.Nodes[:index], outputs)
			reviewNote := laterReviewOutput(request.Nodes, index, outputs)
			prompt := buildNodePrompt(*request, chapter, node, upstream, reviewNote)
			tokenBatch := newTokenBatcher(m, request.ID, chapter.ID)
			text, usage, err, demo := m.generate(ctx, nodeProvider, prompt, tokenBatch.add)
			tokenBatch.flush()
			m.recordUsage(request.ID, usage, map[string]any{
				"chapterId": chapter.ID, "chapterNumber": chapter.Number,
				"kind": node.Kind, "nodeId": node.ID, "nodeTitle": node.Title,
			})
			if err != nil {
				if errors.Is(ctx.Err(), context.Canceled) {
					m.emitNode(request.ID, "node.finished", chapter, node, "canceled", "任务已取消")
					return "", "任务已取消", workflowCanceled
				}
				message := generationFailureMessage(err)
				m.emitNode(request.ID, "node.finished", chapter, node, "failed", message)
				return "", message, workflowFailed
			}
			text = strings.TrimSpace(text)
			if text == "" {
				m.emitNode(request.ID, "node.finished", chapter, node, "failed", "模型没有返回可用内容")
				return "", "模型没有返回可用内容", workflowFailed
			}
			lastText = text
			lastDemo = demo
			lastProvider = nodeProvider
			m.emitNode(request.ID, "node.finished", chapter, node, "succeeded", "")
			m.rememberNode(request, chapter.ID, node.ID, text, demo)
			outputs[node.ID] = NodeResult{ChapterID: chapter.ID, NodeID: node.ID, Status: "succeeded", Output: text, Demo: demo}
			index++
		case "review":
			upstream := upstreamText(request.Nodes[:index], outputs)
			prompt := buildNodePrompt(*request, chapter, node, upstream, "")
			tokenBatch := newTokenBatcher(m, request.ID, chapter.ID)
			text, usage, err, _ := m.generate(ctx, nodeProvider, prompt, tokenBatch.add)
			tokenBatch.flush()
			m.recordUsage(request.ID, usage, map[string]any{
				"chapterId": chapter.ID, "chapterNumber": chapter.Number,
				"kind": node.Kind, "nodeId": node.ID, "nodeTitle": node.Title,
			})
			if err != nil {
				if errors.Is(ctx.Err(), context.Canceled) {
					m.emitNode(request.ID, "node.finished", chapter, node, "canceled", "任务已取消")
					return "", "任务已取消", workflowCanceled
				}
				message := generationFailureMessage(err)
				m.emitNode(request.ID, "node.finished", chapter, node, "failed", message)
				return "", message, workflowFailed
			}
			pass, ok := ParseReviewVerdict(text)
			if !ok {
				message := "审稿没有返回通过或不通过"
				m.emitNode(request.ID, "node.finished", chapter, node, "failed", message)
				return "", message, workflowFailed
			}
			if pass {
				m.emitNode(request.ID, "node.finished", chapter, node, "succeeded", "")
				m.rememberNode(request, chapter.ID, node.ID, text, false)
				outputs[node.ID] = NodeResult{ChapterID: chapter.ID, NodeID: node.ID, Status: "succeeded", Output: text}
				index++
				continue
			}
			generateIndex := previousGenerateIndex(request.Nodes, index)
			if generateIndex < 0 || returns >= MaxReviewReturns {
				message := ReviewLimitMessage
				if generateIndex < 0 {
					message = "审稿未通过，且前面没有可返回的生成节点"
				}
				m.forgetNode(request, chapter.ID, node.ID)
				delete(outputs, node.ID)
				m.emitNode(request.ID, "node.finished", chapter, node, "failed", message)
				return "", message, workflowFailed
			}
			returns++
			m.emitNode(request.ID, "node.finished", chapter, node, "succeeded", "")
			// The fail text is only an in-run note for the rewrite prompt.
			// Leaving it as a succeeded snapshot would make a later review
			// error or cancel skip this node and store the unreviewed rewrite.
			m.forgetNode(request, chapter.ID, node.ID)
			outputs[node.ID] = NodeResult{ChapterID: chapter.ID, NodeID: node.ID, Status: "failed", Output: text}
			skip[node.ID] = false
			generateID := request.Nodes[generateIndex].ID
			m.forgetNode(request, chapter.ID, generateID)
			delete(outputs, generateID)
			skip[generateID] = false
			index = generateIndex
		case "output":
			if strings.TrimSpace(lastText) == "" {
				message := "没有可保存的章节正文"
				m.emitNode(request.ID, "node.finished", chapter, node, "failed", message)
				return "", message, workflowFailed
			}
			if !stored {
				outputRequest := *request
				outputRequest.ProviderID = lastProvider
				proposalID, applied, err := m.persistChapterOutput(ctx, outputRequest, chapter, lastText, lastDemo)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						m.emitNode(request.ID, "node.finished", chapter, node, "canceled", "任务已取消")
						return "", "任务已取消", workflowCanceled
					}
					m.emitNode(request.ID, "node.finished", chapter, node, "failed", err.Error())
					return "", err.Error(), workflowFailed
				}
				if applied {
					m.emit(request.ID, "proposal.applied", map[string]any{"proposalId": proposalID, "chapterId": chapter.ID, "title": chapter.Title, "text": lastText})
				} else {
					m.emit(request.ID, "proposal.created", map[string]any{"proposalId": proposalID, "chapterId": chapter.ID, "chapterNumber": chapter.Number, "title": chapter.Title, "text": lastText, "baseRevision": chapter.Revision, "demoMode": lastDemo})
				}
				stored = true
			}
			m.emitNode(request.ID, "node.finished", chapter, node, "succeeded", "")
			m.rememberNode(request, chapter.ID, node.ID, lastText, lastDemo)
			outputs[node.ID] = NodeResult{ChapterID: chapter.ID, NodeID: node.ID, Status: "succeeded", Output: lastText, Demo: lastDemo}
			index++
		default:
			message := "未知的工作流节点"
			m.emitNode(request.ID, "node.finished", chapter, node, "failed", message)
			return "", message, workflowFailed
		}
	}
	return lastText, "", workflowOK
}

func (m *Manager) emitNode(taskID, eventType string, chapter Chapter, node WorkflowNode, status, message string) {
	data := map[string]any{
		"nodeId": node.ID, "kind": node.Kind, "title": node.Title,
		"chapterId": chapter.ID, "chapterNumber": chapter.Number,
	}
	if node.ProviderID != "" {
		data["providerId"] = node.ProviderID
	}
	if status != "" {
		data["status"] = status
	}
	if message != "" {
		data["message"] = message
	}
	m.emit(taskID, eventType, data)
}

func (m *Manager) rememberNode(request *Request, chapterID, nodeID, output string, demo bool) {
	for index := range request.NodeResults {
		item := &request.NodeResults[index]
		if item.ChapterID == chapterID && item.NodeID == nodeID {
			item.Status = "succeeded"
			item.Output = output
			item.Demo = demo
			m.persistRequest(*request)
			return
		}
	}
	request.NodeResults = append(request.NodeResults, NodeResult{
		ChapterID: chapterID, NodeID: nodeID, Status: "succeeded", Output: output, Demo: demo,
	})
	m.persistRequest(*request)
}

func (m *Manager) forgetNode(request *Request, chapterID, nodeID string) {
	kept := make([]NodeResult, 0, len(request.NodeResults))
	for _, item := range request.NodeResults {
		if item.ChapterID == chapterID && item.NodeID == nodeID {
			continue
		}
		kept = append(kept, item)
	}
	request.NodeResults = kept
	m.persistRequest(*request)
}

func (m *Manager) persistChapterOutput(ctx context.Context, request Request, chapter Chapter, text string, demoMode bool) (string, bool, error) {
	applyAutomatically := request.ApprovalMode == "auto" && !demoMode
	var chapterMemory ChapterMemory
	memoryAvailable := false
	if applyAutomatically {
		rawMemory, memoryUsage, memoryErr, memoryDemo := m.generate(ctx, request.ProviderID, MemoryPrompt(chapter, text), func(string) {})
		m.recordUsage(request.ID, memoryUsage, map[string]any{
			"chapterId": chapter.ID, "chapterNumber": chapter.Number, "kind": "memory",
		})
		if memoryErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", false, ctxErr
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return "", false, context.Canceled
			}
			log.Printf("task %s chapter %d memory generation failed: %v", request.ID, chapter.Number, memoryErr)
		} else if !memoryDemo && ValidChapterMemoryOutput(rawMemory) {
			chapterMemory = ParseChapterMemory(rawMemory)
			memoryAvailable = true
		} else {
			log.Printf("task %s chapter %d memory output was not structured; chapter memory remains missing", request.ID, chapter.Number)
		}
	}
	proposalID := database.NewID()
	status := "pending"
	var resolvedAt any
	if applyAutomatically {
		status = "applied"
		resolvedAt = database.Now()
	}
	tx, err := m.db.Begin()
	if err != nil {
		return "", false, errors.New("保存提案失败")
	}
	_, err = tx.Exec(`INSERT INTO proposals (id, task_id, novel_id, chapter_id, base_revision, proposed_title, proposed_text, status, demo_mode, created_at, resolved_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		proposalID, request.ID, request.NovelID, chapter.ID, chapter.Revision, chapter.Title, text, status, demoModeInt(demoMode), database.Now(), resolvedAt)
	if err == nil && applyAutomatically {
		_, err = tx.Exec(`INSERT INTO chapter_revisions (id, novel_id, chapter_id, revision, title, body, source, edited_by, created_at) VALUES (?, ?, ?, ?, ?, ?, 'agent-auto', ?, ?)`, database.NewID(), request.NovelID, chapter.ID, chapter.Revision, chapter.Title, chapter.Body, request.UserID, database.Now())
		if err == nil {
			var result sql.Result
			result, err = tx.Exec(`UPDATE chapters SET title = ?, body = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ?`, chapter.Title, text, database.Now(), chapter.ID, chapter.Revision)
			if err == nil {
				var count int64
				count, err = result.RowsAffected()
				if err == nil && count != 1 {
					err = errors.New("chapter revision changed during generation")
				}
			}
		}
		if err == nil && memoryAvailable {
			err = SaveChapterMemory(ctx, tx, request.NovelID, chapter.ID, chapter.Number, chapter.Revision+1, chapterMemory)
		}
	}
	if err != nil {
		tx.Rollback()
		log.Printf("task %s chapter %d proposal persistence failed: %v", request.ID, chapter.Number, err)
		return "", false, errors.New("保存提案失败，章节可能已被其他编辑更新")
	}
	if err := tx.Commit(); err != nil {
		log.Printf("task %s chapter %d proposal commit failed: %v", request.ID, chapter.Number, err)
		return "", false, errors.New("保存提案失败")
	}
	return proposalID, applyAutomatically, nil
}

func demoModeInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (m *Manager) persistRequest(request Request) {
	encoded, err := json.Marshal(request)
	if err != nil {
		log.Printf("task %s request snapshot encoding failed: %v", request.ID, err)
		return
	}
	if _, err := m.db.Exec(`UPDATE tasks SET request_json = ?, updated_at = ? WHERE id = ?`, string(encoded), database.Now(), request.ID); err != nil {
		log.Printf("task %s request snapshot update failed: %v", request.ID, err)
	}
}

func contextNodeText(node WorkflowNode, chapter Chapter, request Request) string {
	var builder strings.Builder
	if prompt := strings.TrimSpace(node.Prompt); prompt != "" {
		builder.WriteString(prompt)
		builder.WriteString("\n")
	}
	contextText := chapter.InjectedContext
	if strings.TrimSpace(contextText) == "" {
		contextText = request.Context
	}
	builder.WriteString(contextText)
	return strings.TrimSpace(builder.String())
}

func upstreamText(nodes []WorkflowNode, results map[string]NodeResult) string {
	var builder strings.Builder
	for _, node := range nodes {
		result, ok := results[node.ID]
		if !ok || strings.TrimSpace(result.Output) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(node.Title)
		builder.WriteString("：\n")
		builder.WriteString(result.Output)
	}
	return builder.String()
}

func laterReviewOutput(nodes []WorkflowNode, generateIndex int, results map[string]NodeResult) string {
	for _, node := range nodes[generateIndex+1:] {
		if node.Kind != "review" {
			continue
		}
		if result, ok := results[node.ID]; ok && strings.TrimSpace(result.Output) != "" {
			return result.Output
		}
	}
	return ""
}

func previousGenerateIndex(nodes []WorkflowNode, reviewIndex int) int {
	for index := reviewIndex - 1; index >= 0; index-- {
		if nodes[index].Kind == "generate" {
			return index
		}
	}
	return -1
}

// ParseReviewVerdict reads a structured pass or fail. A missing verdict is not
// a generator error; the caller stops that node instead of branching.
func ParseReviewVerdict(text string) (pass bool, ok bool) {
	if strings.Contains(text, "审稿结论：不通过") {
		return false, true
	}
	if strings.Contains(text, "审稿结论：通过") {
		return true, true
	}
	return false, false
}

func buildNodePrompt(request Request, chapter Chapter, node WorkflowNode, upstream, reviewNote string) string {
	var prompt strings.Builder
	prompt.WriteString("创作任务：\n")
	prompt.WriteString(request.Instruction)
	if guide := strings.TrimSpace(request.WritingGuide); guide != "" {
		prompt.WriteString("\n\n写作指南（必须遵守，冲突时优先于其他资料）：\n")
		prompt.WriteString(guide)
	}
	prompt.WriteString("\n\n小说资料与知识图谱：\n")
	contextText := chapter.InjectedContext
	if strings.TrimSpace(contextText) == "" {
		contextText = request.Context
	}
	prompt.WriteString(contextText)
	if request.WorkflowName != "" {
		prompt.WriteString("\n\n工作流：")
		prompt.WriteString(request.WorkflowName)
	}
	prompt.WriteString("\n\n工作流节点（")
	prompt.WriteString(node.Kind)
	prompt.WriteString("）：")
	prompt.WriteString(node.Title)
	if nodePrompt := strings.TrimSpace(node.Prompt); nodePrompt != "" {
		prompt.WriteString("\n节点要求：\n")
		prompt.WriteString(nodePrompt)
	}
	if upstream = strings.TrimSpace(upstream); upstream != "" {
		prompt.WriteString("\n\n上游节点结果：\n")
		prompt.WriteString(upstream)
	}
	if reviewNote = strings.TrimSpace(reviewNote); reviewNote != "" {
		prompt.WriteString("\n\n审稿意见（必须据此修改）：\n")
		prompt.WriteString(reviewNote)
	}
	prompt.WriteString("\n\n目标章节：第")
	prompt.WriteString(strconv.Itoa(chapter.Number))
	prompt.WriteString("章《")
	prompt.WriteString(chapter.Title)
	prompt.WriteString("》")
	writePromptField(&prompt, "本章目标", chapter.Goal)
	writePromptField(&prompt, "本章关键事件", chapter.KeyEvents)
	writePromptField(&prompt, "章末钩子", chapter.SuspenseHook)
	if tail := strings.TrimSpace(chapter.PreviousTail); tail != "" {
		prompt.WriteString("\n上一章结尾（仅用于承接，不要重复或改写）：\n")
		prompt.WriteString(tail)
	}
	prompt.WriteString("\n当前草稿：\n")
	prompt.WriteString(chapter.Body)
	if node.Kind == "review" {
		prompt.WriteString("\n\n请只输出审稿结论，格式为「审稿结论：通过」或「审稿结论：不通过」，并附一句理由。不要输出章节正文。")
		return prompt.String()
	}
	prompt.WriteString("\n\n请根据任务要求产出完整章节正文。只生成提案，不要改动未提供的设定。")
	return prompt.String()
}
