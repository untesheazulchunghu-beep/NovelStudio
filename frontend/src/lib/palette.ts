import type { PaletteResult } from '../workbenchTypes'
import { statusLabel } from './format'
import type { Chapter, ChapterSearchResult, Character, Note, TimelineEvent, WorldAsset } from '../types'

export const paletteCommands: PaletteResult[] = [
  { id: 'new-chapter', kind: 'command', title: '新建章节', detail: '在当前项目添加一章', searchable: '新建 添加 创建 章节 chapter add create' },
  { id: 'new-character', kind: 'command', title: '新建角色', detail: '添加角色档案', searchable: '新建 添加 创建 角色 人物 character add create' },
  { id: 'new-note', kind: 'command', title: '新建便笺', detail: '记录一条灵感或线索', searchable: '新建 添加 创建 便笺 灵感 note idea add create' },
  { id: 'new-timeline', kind: 'command', title: '新建时间线事件', detail: '记录故事中的关键变化', searchable: '新建 添加 创建 时间线 事件 故事 timeline event add create' },
  { id: 'edit-project', kind: 'command', title: '编辑项目资料', detail: '修改名称、设定和目标字数', searchable: '编辑 修改 项目 资料 名称 设定 字数 project edit' },
  { id: 'export-markdown', kind: 'command', title: '导出 Markdown', detail: '下载当前项目的 Markdown 文件', searchable: '导出 markdown 下载 export' },
  { id: 'export-docx', kind: 'command', title: '导出 Word 文档（DOCX）', detail: '下载包含当前章节编辑内容的 Word 文档', searchable: '导出 word docx 文档 下载 export' },
  { id: 'export-epub', kind: 'command', title: '导出 EPUB 电子书', detail: '下载包含目录和章节导航的电子书', searchable: '导出 epub 电子书 ebook 下载 export' },
  { id: 'export-backup', kind: 'command', title: '导出完整项目备份', detail: '下载可恢复的 JSON 项目备份', searchable: '导出 备份 完整项目 json backup export' },
  { id: 'import-backup', kind: 'command', title: '导入项目备份', detail: '将 JSON 备份恢复为新项目', searchable: '导入 恢复 备份 项目 json backup import restore' },
  { id: 'open-revisions', kind: 'command', title: '查看章节版本', detail: '保存检查点并恢复章节旧版本', searchable: '版本 历史 修订 章节 恢复 revision history restore' },
  { id: 'open-outline', kind: 'command', title: '打开章节大纲', detail: '切换 Agent 面板到章节路线', searchable: '打开 查看 章节 大纲 路线 outline' },
  { id: 'open-context', kind: 'command', title: '打开项目上下文', detail: '查看项目设定、角色和便笺', searchable: '打开 查看 项目 世界观 上下文 context setting world' },
  { id: 'open-settings', kind: 'command', title: '打开模型设置', detail: '配置写作 Agent 使用的模型', searchable: '打开 设置 模型 agent settings model' },
  { id: 'open-help', kind: 'command', title: '打开帮助与快捷键', detail: '查看工作台的键盘操作', searchable: '打开 帮助 快捷键 keyboard shortcuts help' },
  { id: 'open-characters', kind: 'command', title: '打开角色档案', detail: '浏览当前项目的角色', searchable: '打开 查看 角色 人物 档案 characters' },
  { id: 'open-notes', kind: 'command', title: '打开灵感便笺', detail: '浏览当前项目的便笺', searchable: '打开 查看 便笺 灵感 notes' },
]

paletteCommands.splice(4, 0, { id: 'new-world-asset', kind: 'command', title: '新建世界观资料', detail: '添加地点、组织、物件或概念', searchable: '新建 添加 创建 世界观 资料 地点 组织 物件 概念 world asset add create' })

export function paletteQueryTerms(query: string) {
  return query.trim().split(/\s+/).filter(Boolean).map((term) =>
    new RegExp(term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i'),
  )
}

export function matchesPaletteQuery(value: string | readonly string[], terms: RegExp[]) {
  if (terms.length === 0) return true
  const fields = typeof value === 'string' ? [value] : value
  return terms.every((term) => fields.some((field) => term.test(field)))
}

export function searchExcerpt(value: string, terms: RegExp[], maxLength = 92) {
  if (!value) return ''
  const matchIndex = terms
    .map((term) => term.exec(value)?.index)
    .find((index): index is number => index !== undefined) ?? -1
  if (matchIndex < 0) return ''
  const start = matchIndex > 28 ? matchIndex - 28 : 0
  const end = Math.min(value.length, start + maxLength)
  const compact = value.slice(start, end).replace(/\s+/g, ' ').trim()
  if (!compact) return ''
  return `${start > 0 ? '\u2026' : ''}${compact}${end < value.length ? '\u2026' : ''}`
}

export interface PaletteSearchInput {
  searchOpen: boolean
  hasData: boolean
  searchQuery: string
  currentChapter?: Chapter
  chapterBodyLoading: boolean
  projectActionBusy: boolean
  chapterSearchResults: ChapterSearchResult[]
  editorTitle: string
  editorSummary: string
  editorContent: string
  characters: Character[]
  notes: Note[]
  timeline: TimelineEvent[]
  worldAssets: WorldAsset[]
}

export function buildPaletteResults({ searchOpen, hasData, searchQuery, currentChapter, chapterBodyLoading, projectActionBusy, chapterSearchResults, editorTitle, editorSummary, editorContent, characters, notes, timeline, worldAssets }: PaletteSearchInput): PaletteResult[] {
  if (!searchOpen || !hasData) return []
  const query = searchQuery.trim()
  const queryTerms = paletteQueryTerms(query)
  const commandResults = paletteCommands
    .filter((command) => matchesPaletteQuery([command.title, command.searchable], queryTerms))
    .map((command) => ({
      ...command,
      disabled: command.id === 'open-revisions'
        ? !currentChapter || chapterBodyLoading || projectActionBusy
        : command.id === 'new-chapter'
        ? projectActionBusy
        : command.id === 'new-character' || command.id === 'new-note' || command.id === 'new-timeline' || command.id === 'new-world-asset'
          ? projectActionBusy
        : ['edit-project', 'export-markdown', 'export-backup', 'export-docx', 'export-epub', 'import-backup', 'open-context'].includes(command.id) && projectActionBusy,
    }))
  if (!query) return commandResults

  const chapterResults: PaletteResult[] = []
  for (const chapter of chapterSearchResults) {
    const isCurrent = chapter.id === currentChapter?.id
    const title = isCurrent ? editorTitle : chapter.title
    const summary = isCurrent ? editorSummary : chapter.summary
    const hasCurrentBody = isCurrent && currentChapter?.content !== undefined
    const searchableFields = [String(chapter.number), title, summary, ...(hasCurrentBody ? [editorContent] : [])]
    if (hasCurrentBody && !matchesPaletteQuery(searchableFields, queryTerms)) continue
    const excerpt = (hasCurrentBody ? searchExcerpt(summary, queryTerms, 78) || searchExcerpt(editorContent, queryTerms, 78) : '') || chapter.excerpt || statusLabel(chapter.status)
    chapterResults.push({
      id: `chapter:${chapter.id}`,
      kind: 'chapter',
      title: title || `第 ${chapter.number} 章（未命名）`,
      detail: `第 ${chapter.number} 章${excerpt ? ` · ${excerpt}` : ''}`,
      searchable: [String(chapter.number), title, summary].join(' '),
      entityId: chapter.id,
      disabled: projectActionBusy,
    })
  }
  const currentBodyMatches = Boolean(currentChapter?.content !== undefined && matchesPaletteQuery([String(currentChapter.number), editorTitle, editorSummary, editorContent], queryTerms))
  if (currentChapter && currentBodyMatches && !chapterResults.some((result) => result.id === `chapter:${currentChapter.id}`)) {
    const excerpt = searchExcerpt(editorSummary, queryTerms, 78) || searchExcerpt(editorContent, queryTerms, 78) || statusLabel(currentChapter.status)
    chapterResults.unshift({
      id: `chapter:${currentChapter.id}`,
      kind: 'chapter',
      title: editorTitle || `第 ${currentChapter.number} 章（未命名）`,
      detail: `第 ${currentChapter.number} 章${excerpt ? ` · ${excerpt}` : ''}`,
      searchable: [String(currentChapter.number), editorTitle, editorSummary].join(' '),
      entityId: currentChapter.id,
      disabled: projectActionBusy,
    })
  }
  const characterResults: PaletteResult[] = []
  for (const character of characters) {
    if (characterResults.length >= 10) break
    const searchableFields = [character.name, character.role, character.description, ...character.traits]
    if (!matchesPaletteQuery(searchableFields, queryTerms)) continue
    const detail = [character.role, searchExcerpt(character.description, queryTerms, 64), character.traits.slice(0, 2).join(' · ')].filter(Boolean).join(' · ')
    characterResults.push({ id: `character:${character.id}`, kind: 'character', title: character.name, detail, searchable: searchableFields.join(' '), entityId: character.id, disabled: projectActionBusy })
  }
  const noteResults: PaletteResult[] = []
  for (const note of notes) {
    if (noteResults.length >= 10) break
    const searchableFields = [note.title, note.kind, note.body]
    if (!matchesPaletteQuery(searchableFields, queryTerms)) continue
    const detail = searchExcerpt(note.body, queryTerms, 78) || '项目便笺'
    noteResults.push({ id: `note:${note.id}`, kind: 'note', title: note.title, detail, searchable: searchableFields.join(' '), entityId: note.id, disabled: projectActionBusy })
  }
  const timelineResults: PaletteResult[] = []
  for (const event of timeline) {
    if (timelineResults.length >= 10) break
    const searchableFields = [event.title, event.time_label, event.description, event.status]
    if (!matchesPaletteQuery(searchableFields, queryTerms)) continue
    const detail = [event.time_label, searchExcerpt(event.description, queryTerms, 78)].filter(Boolean).join(' · ') || '故事时间线'
    timelineResults.push({ id: `timeline:${event.id}`, kind: 'timeline', title: event.title, detail, searchable: searchableFields.join(' '), entityId: event.id, disabled: projectActionBusy })
  }
  const worldAssetResults: PaletteResult[] = []
  for (const asset of worldAssets) {
    if (worldAssetResults.length >= 10) break
    const searchableFields = [asset.name, asset.kind, asset.description, asset.details]
    if (!matchesPaletteQuery(searchableFields, queryTerms)) continue
    const detail = [asset.kind, searchExcerpt(asset.description, queryTerms, 78)].filter(Boolean).join(' · ') || '世界观资料'
    worldAssetResults.push({ id: `world-asset:${asset.id}`, kind: 'world-asset', title: asset.name, detail, searchable: searchableFields.join(' '), entityId: asset.id, disabled: projectActionBusy })
  }
  return [...commandResults, ...chapterResults, ...characterResults, ...noteResults, ...timelineResults, ...worldAssetResults]
}

paletteCommands.push(
  { id: 'open-timeline', kind: 'command', title: '\u6253\u5f00\u6545\u4e8b\u65f6\u95f4\u7ebf', detail: '\u6d4f\u89c8\u5173\u952e\u4e8b\u4ef6', searchable: '\u6253\u5f00 \u67e5\u770b \u65f6\u95f4\u7ebf \u6545\u4e8b \u4e8b\u4ef6 timeline' },
  { id: 'open-world-assets', kind: 'command', title: '\u6253\u5f00\u4e16\u754c\u89c2\u8d44\u6599', detail: '\u6d4f\u89c8\u5730\u70b9\u3001\u7ec4\u7ec7\u3001\u7269\u4ef6\u548c\u6982\u5ff5', searchable: '\u6253\u5f00 \u67e5\u770b \u4e16\u754c\u89c2 \u8d44\u6599 \u5730\u70b9 \u7ec4\u7ec7 \u7269\u4ef6 \u6982\u5ff5 world assets' },
)
