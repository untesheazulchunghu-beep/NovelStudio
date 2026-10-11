import type { Chapter, WorkspaceResponse } from '../types'

export function formatNumber(value: number) {
  return new Intl.NumberFormat('zh-CN').format(value)
}

export function safeProjectFileBaseName(title: string) {
  const normalized = title
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_')
    .replace(/[. ]+$/g, '')
    .trim() || 'NovelStudio'
  return /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\..*)?$/i.test(normalized)
    ? `_${normalized}`
    : normalized
}

export function statsForChapters(chapters: Chapter[], currentStats: WorkspaceResponse['stats']) {
  const wordCount = chapters.reduce((sum, chapter) => sum + chapter.word_count, 0)
  return {
    ...currentStats,
    chapter_count: chapters.length,
    word_count: wordCount,
    finished_count: chapters.filter((chapter) => chapter.status === 'done').length,
    completion_rate: currentStats.word_target > 0 ? Math.min(1, wordCount / currentStats.word_target) : 0,
  }
}

export function relativeTime(value?: string) {
  if (!value) return '刚刚'
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return '刚刚'
  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000))
  if (seconds < 60) return '刚刚'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`
  return `${Math.floor(seconds / 3600)} 小时前`
}

export function formatTimestamp(value: string) {
  const timestamp = new Date(value)
  return Number.isFinite(timestamp.getTime()) ? timestamp.toLocaleString() : '时间未知'
}

export function statusLabel(status: string) {
  if (status === 'done') return '已完成'
  if (status === 'review') return '审阅中'
  if (status === 'outline') return '大纲'
  return '草稿'
}

export function statusClass(status: string) {
  return status === 'done' ? 'done' : status === 'review' ? 'review' : status === 'outline' ? 'outline' : 'draft'
}
