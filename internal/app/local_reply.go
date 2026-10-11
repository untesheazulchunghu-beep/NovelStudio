package app

import (
	"fmt"
	"strings"
)

func localReply(context localReplyContext) string {
	if context.chapterGeneration {
		return localChapterContinuation(context)
	}
	mode := normalizeWritingMode(context.mode)
	request := trimPromptText(context.requestText, 180)
	projectTitle := trimPromptText(context.projectTitle, 120)
	if projectTitle == "" {
		projectTitle = "当前项目"
	}
	chapterTitle := trimPromptText(context.chapterTitle, 120)
	if chapterTitle == "" {
		chapterTitle = "未命名章节"
	}
	chapterGoal := trimPromptText(context.chapterSummary, 180)
	if chapterGoal == "" {
		chapterGoal = "推动故事局势发生变化"
	}
	characterName := strings.TrimSpace(context.characterName)
	if characterName == "" {
		characterName = "主角"
	}
	scope := fmt.Sprintf("《%s》", projectTitle)
	if context.hasChapter {
		scope += fmt.Sprintf(" · 第%d章《%s》", context.chapterNumber, chapterTitle)
	}
	switch mode {
	case "edit":
		return "本地规则引擎不能直接改写选中的正文。请先在模型设置中配置兼容模型，再使用改写模式。"
	case "plan":
		premise := trimPromptText(context.projectPremise, 160)
		if premise == "" {
			premise = "尚未填写项目设定"
		}
		return fmt.Sprintf("规划模式 · %s\n项目设定：%s\n请求：%s\n本章目标：%s\n\n1. 目标：让%s采取一个能推进本章目标的行动。\n2. 阻力：从角色动机或项目设定中引出一个具体障碍。\n3. 选择：让%s作出有代价的决定，并改变下一场景的局面。\n\n下一步建议：把第三拍的结果写进章节目标。", scope, premise, request, chapterGoal, characterName, characterName)
	case "review":
		if !context.hasChapter && strings.TrimSpace(context.currentText) == "" {
			return fmt.Sprintf("审阅模式 · 《%s》\n当前没有章节正文可检查。请求：%s\n\n你可以先创建章节，或把要审阅的文本放进请求。", projectTitle, request)
		}
		return fmt.Sprintf("审阅模式 · %s\n审阅请求：%s\n\n本地规则引擎无法逐句评估正文，先提供结构检查清单：\n- 节奏：确认场景尽早出现行动、变化或悬念。\n- 动机：检查%s的选择是否由已建立的目标和顾虑推动。\n- 信息：把必要设定放进动作、对话或后果，减少连续解释。\n\n下一步建议：先核对与“%s”相关的角色选择。", scope, request, characterName, chapterGoal)
	}
	if strings.Contains(context.requestText, "续写") || strings.Contains(context.requestText, "继续") {
		opening := fmt.Sprintf("%s回到眼前的细节，重新检查刚才的判断。", characterName)
		if strings.TrimSpace(context.currentText) == "" {
			opening = fmt.Sprintf("%s从一个尚未解决的细节开始行动。", characterName)
		}
		return fmt.Sprintf("%s围绕“%s”采取了一个能验证线索的行动。结果没有立刻给出答案，反而让原来的解释出现缺口。为了继续推进，%s必须在当前风险和既定目标之间作出选择。\n\n下一步建议：让这个选择带来可见后果，而不只补充背景说明。", opening, chapterGoal, characterName)
	}
	if strings.Contains(context.requestText, "摘要") {
		if strings.TrimSpace(context.chapterSummary) != "" {
			return fmt.Sprintf("《%s》已保存的本章摘要：%s", chapterTitle, strings.TrimSpace(context.chapterSummary))
		}
		return fmt.Sprintf("《%s》还没有填写本章摘要。本地规则引擎不能可靠概括任意正文；请先填写本章目标，或配置兼容模型后再生成正文摘要。", chapterTitle)
	}
	if strings.Contains(context.requestText, "润色") || strings.Contains(context.requestText, "改写") {
		return "本地规则引擎无法逐句理解并改写正文。可以先试着删去直接说明情绪的句子，把变化落在角色的动作、感官细节或选择上；要逐句润色，请配置兼容模型。"
	}
	return fmt.Sprintf("针对%s的请求“%s”，先明确这一段要改变的东西：推进事件、暴露信息，或改变角色关系。\n\n下一步建议：让%s采取一个与本章目标“%s”相关的行动，并安排一个来自设定或动机的阻力。", scope, request, characterName, chapterGoal)
}

func localChapterContinuation(context localReplyContext) string {
	characterName := strings.TrimSpace(context.characterName)
	if characterName == "" {
		characterName = "主角"
	}
	chapterGoal := trimPromptText(context.chapterSummary, 180)
	if chapterGoal == "" {
		chapterGoal = "眼前尚未解决的问题"
	}
	if strings.TrimSpace(context.currentText) == "" {
		return fmt.Sprintf("%s从一个尚未解决的细节开始行动，试着围绕“%s”验证线索。结果没有立刻给出答案，反而让原来的解释出现缺口。%s必须在当前风险和既定目标之间作出选择。", characterName, chapterGoal, characterName)
	}
	return fmt.Sprintf("%s把注意力重新放回眼前的细节，围绕“%s”采取一个能验证线索的行动。结果没有立刻给出答案，反而让原来的解释出现缺口。%s必须在当前风险和既定目标之间作出选择。", characterName, chapterGoal, characterName)
}
