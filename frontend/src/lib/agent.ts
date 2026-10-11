import { ListTodo, PenLine, Sparkles, WandSparkles } from 'lucide-react'
import type { WritingMode } from '../workbenchTypes'

export const writingModes: Array<{ id: WritingMode; label: string; icon: typeof PenLine }> = [
  { id: 'write', label: '写作', icon: PenLine },
  { id: 'plan', label: '规划', icon: ListTodo },
  { id: 'review', label: '审阅', icon: Sparkles },
  { id: 'edit', label: '改写', icon: WandSparkles },
]

export const defaultSuggestions: Record<WritingMode, string[]> = {
  write: ['续写下一段', '提炼本章摘要', '检查角色动机'],
  plan: ['把这个冲突拆成三个节拍', '为本章补一条伏笔', '检查角色动机'],
  review: ['检查节奏和信息密度', '找出一个可删句子', '生成更有画面感的版本'],
  edit: ['改得更克制一些', '增强画面感', '缩短这段文字'],
}

export function suggestionsForMode(mode: WritingMode) {
  return [...defaultSuggestions[mode]]
}

export function writingModeLabel(mode?: string) {
  return writingModes.find((item) => item.id === mode)?.label
}
