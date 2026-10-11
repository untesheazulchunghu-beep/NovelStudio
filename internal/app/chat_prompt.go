package app

import (
	"fmt"
	"strings"
)

func chapterFor(chapters []Chapter, id int64) (Chapter, bool) {
	for _, chapter := range chapters {
		if chapter.ID == id {
			return chapter, true
		}
	}
	if id == 0 && len(chapters) > 0 {
		return chapters[0], true
	}
	return Chapter{}, false
}

func applyChapterPromptOverrides(chapter *Chapter, title, summary *string) {
	if title != nil {
		if cleaned := strings.TrimSpace(*title); cleaned != "" {
			chapter.Title = cleaned
		}
	}
	if summary != nil {
		chapter.Summary = *summary
	}
}

func normalizeWritingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "edit":
		return "edit"
	case "plan":
		return "plan"
	case "review":
		return "review"
	default:
		return "write"
	}
}

func buildChatPrompt(workspace Workspace, chapter Chapter, input ChatRequest) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "你是小说工作室中的写作 Agent。项目：《%s》，类型：%s。\n", trimPromptText(workspace.Project.Title, 120), trimPromptText(workspace.Project.Genre, 120))
	fmt.Fprintf(&builder, "项目 premise：%s\n", trimPromptText(workspace.Project.Premise, 1600))
	fmt.Fprintf(&builder, "当前模式：%s。%s\n", input.Mode, writingModeInstruction(input.Mode))
	if chapter.ID == 0 {
		builder.WriteString("当前项目还没有章节，请依据项目设定和角色资料协助用户规划，不要假设已有正文。\n")
	} else {
		fmt.Fprintf(&builder, "当前章节：第%d章《%s》。章节摘要：%s。\n", chapter.Number, trimPromptText(chapter.Title, 120), trimPromptText(chapter.Summary, 1200))
	}
	appendWorkspaceReferences(&builder, workspace)
	if input.Context != "" {
		contextLabel := "当前章节正文片段（编辑器末尾）"
		if strings.TrimSpace(input.Selection) != "" {
			contextLabel = "当前章节选区附近的正文上下文（选区正文另附）"
		}
		fmt.Fprintf(&builder, "%s：%s\n", contextLabel, trimPromptText(input.Context, 1800))
	}
	appendRecentChat(&builder, workspace.Chat)
	if input.Selection != "" {
		fmt.Fprintf(&builder, "用户选中的正文（改写或审阅时优先围绕此段）：%s\n", trimPromptText(input.Selection, maxAgentSelectionRunes))
	}
	fmt.Fprintf(&builder, "用户请求：%s\n", input.Message)
	builder.WriteString("请用中文回答，不要编造与当前设定冲突的事实。")
	return builder.String()
}

func writingModeInstruction(mode string) string {
	switch normalizeWritingMode(mode) {
	case "edit":
		return "改写模式：只改写用户选中的正文，落实用户提出的要求，并保持事实设定、人物关系和叙述视角一致。只输出可以直接替换选区的正文，不要添加解释、标题、引号或 Markdown。"
	case "plan":
		return "规划模式：给出可执行的大纲方案，优先拆成 3 到 5 个有因果关系的节拍，并说明每拍的角色目标、阻力和结果；除非用户明确要求，不要把大纲扩写成完整正文。"
	case "review":
		return "审阅模式：围绕当前章节或用户提供的文本指出具体问题和依据，再给出可执行的修改建议；避免泛泛鼓励，也不要擅自重写整段，除非用户明确要求。"
	default:
		return "写作模式：优先直接回应创作请求；续写或润色时给出可直接使用的文本，用户要求分析时再提供分析。"
	}
}

func buildChapterGeneratePrompt(workspace Workspace, chapter Chapter, instruction, currentText string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "你是小说工作室中的写作 Agent。任务类型：正文续写。请为《%s》第%d章《%s》续写中文小说正文。\n", trimPromptText(workspace.Project.Title, 120), chapter.Number, trimPromptText(chapter.Title, 120))
	fmt.Fprintf(&builder, "项目类型：%s。项目设定：%s\n", trimPromptText(workspace.Project.Genre, 120), trimPromptText(workspace.Project.Premise, 1600))
	fmt.Fprintf(&builder, "本章目标：%s\n", trimPromptText(chapter.Summary, 1200))
	appendWorkspaceReferences(&builder, workspace)
	if text := strings.TrimSpace(currentText); text != "" {
		fmt.Fprintf(&builder, "当前章节正文（编辑器末尾片段）：%s\n", trimPromptText(text, 1800))
	}
	fmt.Fprintf(&builder, "续写要求：%s\n", instruction)
	builder.WriteString("直接输出可接在现有正文后的小说内容，只写正文，不要重复已有内容，不要添加标题、分析或下一步建议。保持人物、设定和叙述视角一致。")
	return builder.String()
}

func appendWorkspaceReferences(builder *strings.Builder, workspace Workspace) {
	if len(workspace.Characters) > 0 {
		builder.WriteString("角色档案：\n")
		limit := len(workspace.Characters)
		if limit > 16 {
			limit = 16
		}
		for _, character := range workspace.Characters[:limit] {
			fmt.Fprintf(builder, "- %s（%s）：%s。特征：%s。\n",
				trimPromptText(character.Name, 80), trimPromptText(character.Role, 120),
				trimPromptText(character.Description, 500), promptTraits(character.Traits))
		}
		if limit < len(workspace.Characters) {
			fmt.Fprintf(builder, "（另有 %d 份角色档案未列出。）\n", len(workspace.Characters)-limit)
		}
	}
	if len(workspace.Notes) > 0 {
		builder.WriteString("项目便笺：\n")
		limit := len(workspace.Notes)
		if limit > 20 {
			limit = 20
		}
		for _, note := range workspace.Notes[:limit] {
			fmt.Fprintf(builder, "- %s（%s）：%s\n",
				trimPromptText(note.Title, 120), trimPromptText(note.Kind, 80), trimPromptText(note.Body, 600))
		}
		if limit < len(workspace.Notes) {
			fmt.Fprintf(builder, "（另有 %d 条便笺未列出。）\n", len(workspace.Notes)-limit)
		}
	}
}

func appendRecentChat(builder *strings.Builder, messages []ChatMessage) {
	recent := make([]ChatMessage, 0, 8)
	for index := len(messages) - 1; index >= 0 && len(recent) < 8; index-- {
		if messages[index].Role == "system" {
			continue
		}
		recent = append(recent, messages[index])
	}
	if len(recent) == 0 {
		return
	}
	builder.WriteString("最近对话：\n")
	for left, right := 0, len(recent)-1; left < right; left, right = left+1, right-1 {
		recent[left], recent[right] = recent[right], recent[left]
	}
	for _, message := range recent {
		role := "Agent"
		if message.Role == "user" {
			role = "用户"
		}
		fmt.Fprintf(builder, "%s：%s\n", role, trimPromptText(message.Content, 600))
	}
}

func trimPromptText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return ""
	}
	runeCount := 0
	for index := range value {
		if runeCount == maxRunes {
			return value[:index] + "…"
		}
		runeCount++
	}
	return value
}

func promptTraits(traits []string) string {
	if len(traits) > 20 {
		traits = traits[:20]
	}
	cleaned := make([]string, 0, len(traits))
	for _, trait := range traits {
		if value := trimPromptText(trait, 80); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return trimPromptText(strings.Join(cleaned, "、"), 300)
}

func suggestionsFor(mode string) []string {
	switch mode {
	case "edit":
		return []string{"改得更克制一些", "增强画面感", "缩短这段文字"}
	case "plan":
		return []string{"把这个冲突拆成三个节拍", "为本章补一条伏笔", "检查角色动机"}
	case "review":
		return []string{"检查节奏和信息密度", "找出一个可删句子", "生成更有画面感的版本"}
	default:
		return []string{"续写下一段", "提炼本章摘要", "检查角色动机"}
	}
}

type localReplyContext struct {
	chapterGeneration bool
	mode              string
	requestText       string
	projectTitle      string
	projectPremise    string
	chapterNumber     int
	chapterTitle      string
	chapterSummary    string
	characterName     string
	currentText       string
	hasChapter        bool
}

func localReplyContextFor(workspace Workspace, chapter Chapter, mode, requestText, currentText string) localReplyContext {
	context := localReplyContext{
		mode: mode, requestText: requestText,
		projectTitle: workspace.Project.Title, projectPremise: workspace.Project.Premise,
		chapterNumber: chapter.Number, chapterTitle: chapter.Title, chapterSummary: chapter.Summary,
		currentText: currentText, hasChapter: chapter.ID != 0,
	}
	if len(workspace.Characters) > 0 {
		context.characterName = workspace.Characters[0].Name
	}
	return context
}
