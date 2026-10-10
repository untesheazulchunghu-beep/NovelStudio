import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  Activity,
  Archive,
  ArrowDownToLine,
  BookOpen,
  Bot,
  BrainCircuit,
  Check,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  CircleHelp,
  Command,
  FilePlus2,
  FileText,
  FolderOpen,
  History,
  LayoutPanelLeft,
  Lightbulb,
  ListTodo,
  MessageSquare,
  MoreHorizontal,
  PanelRightClose,
  PanelRightOpen,
  PenLine,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Search,
  Send,
  Settings2,
  Sparkles,
  Target,
  Trash2,
  Upload,
  Users,
  WandSparkles,
  X,
  Zap,
} from 'lucide-react'
import { ApiError, api } from './api'
import type { Chapter, ChapterRevision, ChapterRevisionSummary, ChapterSearchResult, ChatMessage, Character, Note, ProjectSummary, SettingsDraft, SettingsUpdate, WorkspaceResponse } from './types'

type ActivityId = 'explorer' | 'chapters' | 'characters' | 'notes' | 'settings'
type AssistantTab = 'chat' | 'outline' | 'context'
type WritingMode = 'write' | 'plan' | 'review' | 'edit'
type ChatRequestRetry = { signature: string; requestID: string; message: string; mode: WritingMode; safeToRetry: boolean; requiresReview: boolean; requiresFreshRequestID: boolean; needsReconcile: boolean }
type PaletteResultKind = 'chapter' | 'character' | 'note' | 'command'

const characterColors = ['#f0a46b', '#8f9df4', '#58c5a3', '#d890b8'] as const
const maxAgentInputCharacters = 8000
const maxAgentSelectionCharacters = 1800
const hanScriptPattern = /\p{Script=Han}/u
const wordCharacterPattern = /[\p{L}\p{M}\p{N}_]/u

function normalizeEditorLineEndings(text: string) {
  return text.replace(/\r\n?/g, '\n')
}

function countUnicodeCharacters(text: string) {
  return Array.from(text).length
}

function countUnicodeCharactersInRange(text: string, start: number, end: number, limit: number) {
  let count = 0
  for (let index = start; index < end;) {
    const codePoint = text.codePointAt(index)
    if (codePoint === undefined) break
    index += codePoint > 0xffff ? 2 : 1
    count += 1
    if (count > limit) return count
  }
  return count
}

function sliceUnicodeRange(text: string, start: number, end: number, limit: number) {
  let index = start
  let count = 0
  while (index < end && count < limit) {
    const codePoint = text.codePointAt(index)
    if (codePoint === undefined) break
    index += codePoint > 0xffff ? 2 : 1
    count += 1
  }
  return text.slice(start, Math.min(index, end))
}

function sliceUnicodeTail(text: string, limit: number) {
  let start = text.length
  let count = 0
  while (start > 0 && count < limit) {
    start -= 1
    const current = text.charCodeAt(start)
    if (current >= 0xdc00 && current <= 0xdfff && start > 0) {
      const previous = text.charCodeAt(start - 1)
      if (previous >= 0xd800 && previous <= 0xdbff) start -= 1
    }
    count += 1
  }
  return text.slice(start)
}

function sliceUnicodeHead(text: string, limit: number) {
  let end = 0
  let count = 0
  for (const character of text) {
    if (count >= limit) break
    end += character.length
    count += 1
  }
  return text.slice(0, end)
}

function normalizeUnicodeSelection(text: string, start: number, end: number) {
  let safeStart = start
  let safeEnd = end
  if (safeStart > 0 && safeStart < text.length) {
    const previous = text.charCodeAt(safeStart - 1)
    const current = text.charCodeAt(safeStart)
    if (previous >= 0xd800 && previous <= 0xdbff && current >= 0xdc00 && current <= 0xdfff) safeStart -= 1
  }
  if (safeEnd > 0 && safeEnd < text.length) {
    const previous = text.charCodeAt(safeEnd - 1)
    const current = text.charCodeAt(safeEnd)
    if (previous >= 0xd800 && previous <= 0xdbff && current >= 0xdc00 && current <= 0xdfff) safeEnd += 1
  }
  return { start: safeStart, end: safeEnd }
}

function contextAroundSelection(text: string, start: number, end: number, limit = 400) {
  if (start < 0 || start >= end || end > text.length) return ""
  let beforeStart = start
  let beforeCount = 0
  while (beforeStart > 0 && beforeCount < limit) {
    const currentIndex = beforeStart - 1
    const current = text.charCodeAt(currentIndex)
    if (current >= 0xdc00 && current <= 0xdfff && currentIndex > 0) {
      const previous = text.charCodeAt(currentIndex - 1)
      beforeStart = previous >= 0xd800 && previous <= 0xdbff ? currentIndex - 1 : currentIndex
    } else {
      beforeStart = currentIndex
    }
    beforeCount += 1
  }
  let afterEnd = end
  let afterCount = 0
  while (afterEnd < text.length && afterCount < limit) {
    const codePoint = text.codePointAt(afterEnd)
    if (codePoint === undefined) break
    afterEnd += codePoint > 0xffff ? 2 : 1
    afterCount += 1
  }
  const before = sliceUnicodeRange(text, beforeStart, start, limit)
  const after = sliceUnicodeRange(text, end, afterEnd, limit)
  const parts = []
  if (before) parts.push("选区前文：" + (beforeStart > 0 ? "…" : "") + before)
  if (after) parts.push("选区后文：" + after + (afterEnd < text.length ? "…" : ""))
  return parts.join("\n")
}

function isIMEComposing(event: KeyboardEvent) {
  return event.isComposing || event.keyCode === 229
}

function countTextWords(text: string) {
  let count = 0
  let inWord = false
  const flush = () => {
    if (!inWord) return
    count += 1
    inWord = false
  }
  for (const character of text) {
    if (hanScriptPattern.test(character)) {
      flush()
      count += 1
    } else if (wordCharacterPattern.test(character)) {
      inWord = true
    } else {
      flush()
    }
  }
  flush()
  return count
}

interface EditorSelection {
  chapterId: number
  start: number
  end: number
}

interface ChatApplyTarget extends EditorSelection {
  workspaceId: string
  messageId: number
  original: string
  replacement: string
  generatedReplacement: string
  context: string
}

interface PaletteResult {
  id: string
  kind: PaletteResultKind
  title: string
  detail: string
  searchable: string
  entityId?: number
  disabled?: boolean
}

const writingModes: Array<{ id: WritingMode; label: string; icon: typeof PenLine }> = [
  { id: 'write', label: '写作', icon: PenLine },
  { id: 'plan', label: '规划', icon: ListTodo },
  { id: 'review', label: '审阅', icon: Sparkles },
  { id: 'edit', label: '改写', icon: WandSparkles },
]

const defaultSuggestions: Record<WritingMode, string[]> = {
  write: ['续写下一段', '提炼本章摘要', '检查角色动机'],
  plan: ['把这个冲突拆成三个节拍', '为本章补一条伏笔', '检查角色动机'],
  review: ['检查节奏和信息密度', '找出一个可删句子', '生成更有画面感的版本'],
  edit: ['改得更克制一些', '增强画面感', '缩短这段文字'],
}

function suggestionsForMode(mode: WritingMode) {
  return [...defaultSuggestions[mode]]
}

function writingModeLabel(mode?: string) {
  return writingModes.find((item) => item.id === mode)?.label
}

const paletteCommands: PaletteResult[] = [
  { id: 'new-chapter', kind: 'command', title: '新建章节', detail: '在当前项目添加一章', searchable: '新建 添加 创建 章节 chapter add create' },
  { id: 'new-character', kind: 'command', title: '新建角色', detail: '添加角色档案', searchable: '新建 添加 创建 角色 人物 character add create' },
  { id: 'new-note', kind: 'command', title: '新建便笺', detail: '记录一条灵感或线索', searchable: '新建 添加 创建 便笺 灵感 note idea add create' },
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

function paletteQueryTerms(query: string) {
  return query.trim().split(/\s+/).filter(Boolean).map((term) =>
    new RegExp(term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i'),
  )
}

function matchesPaletteQuery(value: string | readonly string[], terms: RegExp[]) {
  if (terms.length === 0) return true
  const fields = typeof value === 'string' ? [value] : value
  return terms.every((term) => fields.some((field) => term.test(field)))
}

function searchExcerpt(value: string, terms: RegExp[], maxLength = 92) {
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

function formatNumber(value: number) {
  return new Intl.NumberFormat('zh-CN').format(value)
}

function safeProjectFileBaseName(title: string) {
  const normalized = title
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_')
    .replace(/[. ]+$/g, '')
    .trim() || 'NovelStudio'
  return /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\..*)?$/i.test(normalized)
    ? `_${normalized}`
    : normalized
}

function readActiveWorkspacePreference() {
  try {
    return typeof window === 'undefined' ? null : window.localStorage.getItem('novelstudio.activeWorkspace')
  } catch {
    return null
  }
}

function rememberActiveWorkspacePreference(workspaceId: string) {
  try {
    window.localStorage.setItem('novelstudio.activeWorkspace', workspaceId)
  } catch {
    // Remembering the selection is optional; the workspace itself is stored by the backend.
  }
}

function statsForChapters(chapters: Chapter[], currentStats: WorkspaceResponse['stats']) {
  const wordCount = chapters.reduce((sum, chapter) => sum + chapter.word_count, 0)
  return {
    ...currentStats,
    chapter_count: chapters.length,
    word_count: wordCount,
    finished_count: chapters.filter((chapter) => chapter.status === 'done').length,
    completion_rate: currentStats.word_target > 0 ? Math.min(1, wordCount / currentStats.word_target) : 0,
  }
}

function relativeTime(value?: string) {
  if (!value) return '刚刚'
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return '刚刚'
  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000))
  if (seconds < 60) return '刚刚'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`
  return `${Math.floor(seconds / 3600)} 小时前`
}

function formatTimestamp(value: string) {
  const timestamp = new Date(value)
  return Number.isFinite(timestamp.getTime()) ? timestamp.toLocaleString() : '时间未知'
}

function statusLabel(status: string) {
  if (status === 'done') return '已完成'
  if (status === 'review') return '审阅中'
  if (status === 'outline') return '大纲'
  return '草稿'
}

function statusClass(status: string) {
  return status === 'done' ? 'done' : status === 'review' ? 'review' : status === 'outline' ? 'outline' : 'draft'
}

export default function App() {
  const [data, setData] = useState<WorkspaceResponse | null>(null)
  const [projects, setProjects] = useState<ProjectSummary[]>([])
  const [activeWorkspaceId, setActiveWorkspaceId] = useState(() => readActiveWorkspacePreference() ?? 'default')
  const [switchingWorkspace, setSwitchingWorkspace] = useState(false)
  const [projectDialogOpen, setProjectDialogOpen] = useState(false)
  const [projectTitle, setProjectTitle] = useState('')
  const [projectGenre, setProjectGenre] = useState('')
  const [projectPremise, setProjectPremise] = useState('')
  const [creatingProject, setCreatingProject] = useState(false)
  const [projectEditDialogOpen, setProjectEditDialogOpen] = useState(false)
  const [projectDraft, setProjectDraft] = useState({ title: '', subtitle: '', genre: '', premise: '', word_target: 120000 })
  const [savingProject, setSavingProject] = useState(false)
  const [projectEditError, setProjectEditError] = useState('')
  const [projectDeleteDialogOpen, setProjectDeleteDialogOpen] = useState(false)
  const [projectDeleteInput, setProjectDeleteInput] = useState('')
  const [projectDeleteError, setProjectDeleteError] = useState('')
  const [deletingProject, setDeletingProject] = useState(false)
  const [characterDialogOpen, setCharacterDialogOpen] = useState(false)
  const [characterEditingId, setCharacterEditingId] = useState<number | null>(null)
  const [characterDraft, setCharacterDraft] = useState({ name: '', role: '', description: '', traits: '', color: '' })
  const [noteDialogOpen, setNoteDialogOpen] = useState(false)
  const [noteEditingId, setNoteEditingId] = useState<number | null>(null)
  const [noteDraft, setNoteDraft] = useState({ title: '', body: '', kind: 'idea' })
  const [savingContextItem, setSavingContextItem] = useState(false)
  const [contextDialogError, setContextDialogError] = useState('')
  const [chapterActionBusy, setChapterActionBusy] = useState(false)
  const [revisionDialogOpen, setRevisionDialogOpen] = useState(false)
  const [chapterRevisions, setChapterRevisions] = useState<ChapterRevisionSummary[]>([])
  const [selectedRevision, setSelectedRevision] = useState<ChapterRevision | null>(null)
  const [revisionCompareOpen, setRevisionCompareOpen] = useState(false)
  const [revisionCurrentSnapshot, setRevisionCurrentSnapshot] = useState<Pick<Chapter, 'title' | 'summary' | 'content' | 'status' | 'word_count'> | null>(null)
  const [revisionListLoading, setRevisionListLoading] = useState(false)
  const [revisionPreviewLoading, setRevisionPreviewLoading] = useState(false)
  const [revisionLabel, setRevisionLabel] = useState('手动版本')
  const [revisionError, setRevisionError] = useState('')
  const [error, setError] = useState('')
  const [activeChapterId, setActiveChapterId] = useState<number | null>(null)
  const [activeActivity, setActiveActivity] = useState<ActivityId>('explorer')
  const [assistantTab, setAssistantTab] = useState<AssistantTab>('chat')
  const [chatUnread, setChatUnread] = useState(false)
  const unreadChatWorkspacesRef = useRef(new Set<string>())
  const chatAutoScrollByWorkspaceRef = useRef(new Map<string, boolean>())
  const chatAutoScrollRef = useRef(true)
  const assistantTabRef = useRef<AssistantTab>('chat')
  const assistantCollapsedRef = useRef(false)
  const assistantToggleRef = useRef<HTMLButtonElement>(null)
  const activateAssistantTab = (tab: AssistantTab) => {
    assistantTabRef.current = tab
    setAssistantTab(tab)
    if (tab === 'chat' && chatIsVisibleAtLatest()) setWorkspaceChatUnread(activeWorkspaceRef.current, false)
  }
  const [writingMode, setWritingMode] = useState<WritingMode>('write')
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [assistantCollapsed, setAssistantCollapsed] = useState(false)
  const updateAssistantCollapsed = (collapsed: boolean, markChatRead = false) => {
    if (collapsed) {
      const node = chatScrollRef.current
      const workspaceId = activeWorkspaceRef.current
      if (workspaceId) chatAutoScrollByWorkspaceRef.current.set(workspaceId, chatAutoScrollRef.current)
      if (node && workspaceId) chatScrollPositionsRef.current.set(workspaceId, node.scrollTop)
    }
    assistantCollapsedRef.current = collapsed
    setAssistantCollapsed(collapsed)
    if (markChatRead && !collapsed && chatIsVisibleAtLatest()) setWorkspaceChatUnread(activeWorkspaceRef.current, false)
  }
  const collapseAssistantPanel = () => {
    updateAssistantCollapsed(true)
    window.requestAnimationFrame(() => assistantToggleRef.current?.focus({ preventScroll: true }))
  }
  const chatIsVisibleAtLatest = (workspaceId = activeWorkspaceRef.current) => Boolean(workspaceId && activeWorkspaceRef.current === workspaceId && data?.workspace.workspace_id === workspaceId && !assistantCollapsedRef.current && assistantTabRef.current === 'chat' && chatAutoScrollRef.current && document.visibilityState === 'visible' && document.hasFocus())

  const handleChatScroll = () => {
    const node = chatScrollRef.current
    const workspaceId = activeWorkspaceRef.current
    if (!node || !workspaceId || data?.workspace.workspace_id !== workspaceId) return
    const atBottom = node.scrollHeight - node.scrollTop - node.clientHeight <= 80
    chatAutoScrollRef.current = atBottom
    chatAutoScrollByWorkspaceRef.current.set(workspaceId, atBottom)
    chatScrollPositionsRef.current.set(workspaceId, node.scrollTop)
    if (atBottom && chatIsVisibleAtLatest(workspaceId)) setWorkspaceChatUnread(workspaceId, false)
  }
  const jumpToLatestChat = () => {
    chatAutoScrollRef.current = true
    const workspaceId = activeWorkspaceRef.current
    if (workspaceId) chatAutoScrollByWorkspaceRef.current.set(workspaceId, true)
    setWorkspaceChatUnread(workspaceId, false)
    const behavior = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
    chatEndRef.current?.scrollIntoView({ behavior })
  }
  const [editorTitle, setEditorTitle] = useState('')
  const [editorContent, setEditorContent] = useState('')
  const [editorSelection, setEditorSelection] = useState<EditorSelection | null>(null)
  const [chatApplyTarget, setChatApplyTarget] = useState<ChatApplyTarget | null>(null)
  const [rewritePreviewOpen, setRewritePreviewOpen] = useState(false)
  const hasEditedRewriteDraft = Boolean(chatApplyTarget && chatApplyTarget.replacement !== chatApplyTarget.generatedReplacement)
  const confirmDiscardEditedRewrite = (workspaceID?: string, chapterID?: number) => {
    if (!hasEditedRewriteDraft || !chatApplyTarget) return true
    if (workspaceID && chatApplyTarget.workspaceId !== workspaceID) return true
    if (chapterID !== undefined && chatApplyTarget.chapterId !== chapterID) return true
    return window.confirm('当前有尚未应用的改写编辑。继续此操作会丢弃它；取消以保留。继续？')
  }
  const clearChatApplyTarget = () => {
    setChatApplyTarget(null)
    setRewritePreviewOpen(false)
  }
  const [editorSummary, setEditorSummary] = useState('')
  const [editorStatus, setEditorStatus] = useState<Chapter['status']>('outline')
  const [saveState, setSaveState] = useState<'saved' | 'saving' | 'dirty' | 'error'>('saved')
  const [saveRetryAttempt, setSaveRetryAttempt] = useState(0)
  const [chatInput, setChatInput] = useState('')
  const [chatDraftRevision, setChatDraftRevision] = useState(0)
  const [sending, setSending] = useState(false)
  const [cancellingRequest, setCancellingRequest] = useState(false)
  const [checkingChatRecovery, setCheckingChatRecovery] = useState(false)
  const [suggestions, setSuggestions] = useState<string[]>(() => suggestionsForMode('write'))
  const [settingsDraft, setSettingsDraft] = useState<SettingsDraft | null>(null)
  const [settingsSaved, setSettingsSaved] = useState(false)
  const [settingsDirty, setSettingsDirty] = useState(false)
  const [settingsError, setSettingsError] = useState('')
  const [savingSettings, setSavingSettings] = useState(false)
  const [creatingBackup, setCreatingBackup] = useState(false)
  const [exportingDocument, setExportingDocument] = useState(false)
  const [importingBackup, setImportingBackup] = useState(false)
  const [showMobileSidebar, setShowMobileSidebar] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [chapterSearchResults, setChapterSearchResults] = useState<ChapterSearchResult[]>([])
  const [chapterLoadRetry, setChapterLoadRetry] = useState(0)
  const [chapterLoadError, setChapterLoadError] = useState('')
  const [selectedSearchIndex, setSelectedSearchIndex] = useState(0)
  const [helpDialogOpen, setHelpDialogOpen] = useState(false)
  const editorRef = useRef<HTMLTextAreaElement>(null)
  const backupInputRef = useRef<HTMLInputElement>(null)
  const saveChapterRef = useRef<() => Promise<boolean>>(async () => true)
  const chatEndRef = useRef<HTMLDivElement>(null)
  const chatScrollRef = useRef<HTMLDivElement>(null)
  const chatScrollPositionsRef = useRef(new Map<string, number>())
  const agentRequestRef = useRef<AbortController | null>(null)
  const revisionPreviewRequestRef = useRef(0)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const searchActiveResultRef = useRef<HTMLButtonElement>(null)
  const activeWorkspaceRef = useRef(activeWorkspaceId)
  const setWorkspaceChatUnread = (workspaceId: string | undefined, unread: boolean) => {
    if (!workspaceId) return
    if (unread) unreadChatWorkspacesRef.current.add(workspaceId)
    else unreadChatWorkspacesRef.current.delete(workspaceId)
    if (activeWorkspaceRef.current === workspaceId) setChatUnread(unread)
  }
  const workspaceRequestRef = useRef(0)
  const lastOutsideModalFocusRef = useRef<HTMLElement | null>(null)
  const modalWasOpenRef = useRef(false)
  const editorRevisionRef = useRef(0)
  const chatInputRevisionRef = useRef(0)
  const chatDraftsRef = useRef(new Map<string, string>())
  const chatRequestRetryRef = useRef(new Map<string, ChatRequestRetry>())
  const saveErrorMessageRef = useRef('')
  const currentChapterRef = useRef<Chapter | undefined>(undefined)
  const editorContentRef = useRef('')
  const writingModeRef = useRef<WritingMode>('write')

  const updateChatInput = (value: string) => {
    chatInputRevisionRef.current += 1
    const workspaceId = activeWorkspaceRef.current
    if (workspaceId) {
      if (value) chatDraftsRef.current.set(workspaceId, value)
      else chatDraftsRef.current.delete(workspaceId)
      setChatDraftRevision((revision) => revision + 1)
    }
    setChatInput(value)
  }

  const changeWritingMode = (mode: WritingMode) => {
    writingModeRef.current = mode
    setWritingMode(mode)
    setSuggestions(suggestionsForMode(mode))
  }

  const handleAssistantTabKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>, currentTab: AssistantTab) => {
    const tabs: AssistantTab[] = ['chat', 'outline', 'context']
    let nextIndex = tabs.indexOf(currentTab)
    if (event.key === 'ArrowRight') nextIndex = (nextIndex + 1) % tabs.length
    else if (event.key === 'ArrowLeft') nextIndex = (nextIndex - 1 + tabs.length) % tabs.length
    else if (event.key === 'Home') nextIndex = 0
    else if (event.key === 'End') nextIndex = tabs.length - 1
    else return

    event.preventDefault()
    const nextTab = tabs[nextIndex]
    activateAssistantTab(nextTab)
    document.getElementById('assistant-tab-' + nextTab)?.focus()
  }

  const openSearch = () => {
    if (!data) return
    setSearchQuery('')
    setSelectedSearchIndex(0)
    setSearchOpen(true)
  }

  const closeSearch = () => {
    setSearchOpen(false)
    setSearchQuery('')
    setSelectedSearchIndex(0)
  }

  const openCharactersView = () => {
    setActiveActivity('characters')
    setShowMobileSidebar(true)
  }

  const openNotesView = () => {
    setActiveActivity('notes')
    setShowMobileSidebar(true)
  }

  const openProjectContextView = () => {
    if (projectActionBusy) return
    updateAssistantCollapsed(false)
    activateAssistantTab('context')
  }

  const loadWorkspace = useCallback(async (requestedWorkspaceId?: string) => {
    if (requestedWorkspaceId) rememberActiveWorkspacePreference(requestedWorkspaceId)
    const requestID = ++workspaceRequestRef.current
    try {
      setError('')
      const response = await api.projects.list()
      if (workspaceRequestRef.current !== requestID) return
      setProjects(response.projects)
      const remembered = requestedWorkspaceId ?? readActiveWorkspacePreference() ?? activeWorkspaceRef.current
      const workspaceId = response.projects.some((item) => item.workspace_id === remembered)
        ? remembered
        : response.projects[0]?.workspace_id
      if (!workspaceId) {
        setError('还没有小说项目，请先创建一个项目。')
        return
      }
      activeWorkspaceRef.current = workspaceId
      setChatUnread(unreadChatWorkspacesRef.current.has(workspaceId))
      chatAutoScrollRef.current = chatAutoScrollByWorkspaceRef.current.get(workspaceId) ?? true
      setActiveWorkspaceId(workspaceId)
      rememberActiveWorkspacePreference(workspaceId)
      const next = await api.workspace(workspaceId)
      if (workspaceRequestRef.current !== requestID || activeWorkspaceRef.current !== workspaceId) return
      setData(next)
      setActiveChapterId(next.workspace.chapters[0]?.id ?? null)
      setSettingsDraft(next.workspace.settings)
      setSettingsDirty(false)
      setSettingsSaved(false)
      setSettingsError('')
      setChatInput(chatDraftsRef.current.get(workspaceId) ?? '')
      setSuggestions(suggestionsForMode(writingModeRef.current))
    } catch (reason) {
      if (workspaceRequestRef.current === requestID) {
        setError(reason instanceof Error ? reason.message : '无法连接到 NovelStudio 后端')
      }
    }
  }, [])

  useEffect(() => {
    void loadWorkspace()
  }, [loadWorkspace])

  const chapters = data?.workspace.chapters ?? []
  const currentChapter = chapters.find((chapter) => chapter.id === activeChapterId) ?? chapters[0]
  const activeEditorSelection = editorSelection?.chapterId === currentChapter?.id ? editorSelection : null
  const selectedEditorCharacterCount = activeEditorSelection ? countUnicodeCharactersInRange(editorContent, activeEditorSelection.start, activeEditorSelection.end, maxAgentSelectionCharacters) : 0
  const activeEditorSelectionHasText = Boolean(activeEditorSelection && activeEditorSelection.start >= 0 && activeEditorSelection.start < activeEditorSelection.end && activeEditorSelection.end <= editorContent.length && selectedEditorCharacterCount <= maxAgentSelectionCharacters && sliceUnicodeRange(editorContent, activeEditorSelection.start, activeEditorSelection.end, maxAgentSelectionCharacters).trim())
  const characters = data?.workspace.characters ?? []
  const notes = data?.workspace.notes ?? []
  const chat = data?.workspace.chat ?? []
  const chatRetry = data ? chatRequestRetryRef.current.get(data.workspace.workspace_id) : undefined
  const project = data?.workspace.project
  const stats = data?.stats
  const chapterBodyLoading = Boolean(currentChapter && currentChapter.content === undefined)

  useEffect(() => {
    if (activeChapterId === null) return
    setData((current) => {
      if (!current || current.workspace.workspace_id !== activeWorkspaceRef.current) return current
      let changed = false
      const chaptersNext = current.workspace.chapters.map((chapter) => {
        if (chapter.id === activeChapterId || chapter.content === undefined) return chapter
        changed = true
        return { ...chapter, content: undefined }
      })
      return changed ? { ...current, workspace: { ...current.workspace, chapters: chaptersNext } } : current
    })
  }, [activeChapterId])

  const saveStatusLabel = !data
    ? switchingWorkspace ? '正在切换项目' : '正在读取工作区'
    : saveState === 'saving' ? '保存中' : saveState === 'dirty' ? '未保存' : saveState === 'error' ? '保存失败' : '已保存'
  const assistantProviderLabel = !settingsDraft
    ? '读取模型配置…'
    : settingsDraft.provider === 'local' || !settingsDraft.provider
      ? '本地规则引擎'
      : `${settingsDraft.provider === 'openai' ? 'OpenAI 兼容' : '兼容接口'} · ${settingsDraft.model.trim() || '未填写模型'}`
  const sendLocked = creatingProject || deletingProject || creatingBackup || exportingDocument || importingBackup || switchingWorkspace || checkingChatRecovery || savingContextItem || savingProject || savingSettings || chapterActionBusy || saveState === 'saving'
  const projectActionBusy = sending || sendLocked || exportingDocument
  const editorLocked = creatingProject || deletingProject || creatingBackup || importingBackup || switchingWorkspace || chapterActionBusy || chapterBodyLoading || saveState === 'saving'
  const modalOpen = projectDialogOpen || projectEditDialogOpen || projectDeleteDialogOpen || characterDialogOpen || noteDialogOpen || searchOpen || helpDialogOpen || rewritePreviewOpen || revisionDialogOpen

  useLayoutEffect(() => {
    saveErrorMessageRef.current = ''
    setEditorSelection(null)
    clearChatApplyTarget()
    setSaveRetryAttempt(0)
    currentChapterRef.current = currentChapter
    if (!currentChapter) {
      editorContentRef.current = ''
      setEditorTitle('')
      setEditorContent('')
      setEditorSummary('')
      setEditorStatus('outline')
      setSaveState('saved')
      return
    }
    const chapterContent = normalizeEditorLineEndings(currentChapter.content ?? '')
    editorContentRef.current = chapterContent
    setEditorTitle(currentChapter.title)
    setEditorContent(chapterContent)
    setEditorSummary(currentChapter.summary)
    setEditorStatus(currentChapter.status || 'outline')
    setSaveState('saved')
  }, [currentChapter?.id, data?.workspace.workspace_id])

  useLayoutEffect(() => {
    if (assistantCollapsed || assistantTab !== 'chat') return
    const node = chatScrollRef.current
    const workspaceId = activeWorkspaceRef.current
    if (!node || !workspaceId || data?.workspace.workspace_id !== workspaceId) return
    if (chatIsVisibleAtLatest()) setWorkspaceChatUnread(workspaceId, false)
    if (chatAutoScrollRef.current) {
      node.scrollTop = node.scrollHeight
      return
    }
    const savedScrollTop = chatScrollPositionsRef.current.get(workspaceId)
    if (savedScrollTop === undefined) return
    node.scrollTop = savedScrollTop
    handleChatScroll()
  }, [assistantCollapsed, assistantTab, activeWorkspaceId, data?.workspace.workspace_id, chat.length])

  useEffect(() => {
    const markLatestChatRead = () => {
      if (chatIsVisibleAtLatest()) setWorkspaceChatUnread(activeWorkspaceRef.current, false)
    }
    window.addEventListener('focus', markLatestChatRead)
    document.addEventListener('visibilitychange', markLatestChatRead)
    return () => {
      window.removeEventListener('focus', markLatestChatRead)
      document.removeEventListener('visibilitychange', markLatestChatRead)
    }
  }, [data?.workspace.workspace_id])

  useEffect(() => {
    if (assistantCollapsed || assistantTab !== 'chat' || !chatAutoScrollRef.current) return
    const behavior = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
    chatEndRef.current?.scrollIntoView({ behavior })
  }, [assistantCollapsed, assistantTab, chat.length, sending])

  useEffect(() => {
    const handleGlobalShortcuts = (event: KeyboardEvent) => {
      if (isIMEComposing(event)) return
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
        const target = event.target
        if (!(target instanceof HTMLElement) || !target.closest('.editor-pane') || !data || !currentChapter) return
        event.preventDefault()
        if (!editorLocked) void saveChapterRef.current()
        return
      }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        if (helpDialogOpen || projectDialogOpen || projectEditDialogOpen || projectDeleteDialogOpen || characterDialogOpen || noteDialogOpen || rewritePreviewOpen || revisionDialogOpen) {
          event.preventDefault()
          return
        }
        if (!data) return
        event.preventDefault()
        setSearchQuery('')
        setSelectedSearchIndex(0)
        setSearchOpen(true)
      } else if (event.key === 'Escape') {
        if (rewritePreviewOpen) {
          event.preventDefault()
          setRewritePreviewOpen(false)
        } else if (revisionDialogOpen) {
          event.preventDefault()
          if (!chapterActionBusy) setRevisionDialogOpen(false)
        } else if (searchOpen) {
          event.preventDefault()
          setSearchOpen(false)
        } else if (helpDialogOpen) {
          event.preventDefault()
          setHelpDialogOpen(false)
        } else if (projectDeleteDialogOpen) {
          event.preventDefault()
          if (!deletingProject) setProjectDeleteDialogOpen(false)
        } else if (projectEditDialogOpen) {
          event.preventDefault()
          if (!savingProject) setProjectEditDialogOpen(false)
        } else if (projectDialogOpen) {
          event.preventDefault()
          if (!creatingProject) setProjectDialogOpen(false)
        } else if (characterDialogOpen || noteDialogOpen) {
          event.preventDefault()
          if (!savingContextItem) {
            setCharacterDialogOpen(false)
            setNoteDialogOpen(false)
          }
        } else if (agentRequestRef.current) {
          event.preventDefault()
          setCancellingRequest(true)
          agentRequestRef.current.abort()
        }
      }
    }
    window.addEventListener('keydown', handleGlobalShortcuts)
    return () => window.removeEventListener('keydown', handleGlobalShortcuts)
  }, [chapterActionBusy, characterDialogOpen, creatingProject, currentChapter?.id, data, deletingProject, editorLocked, helpDialogOpen, noteDialogOpen, projectDeleteDialogOpen, projectDialogOpen, projectEditDialogOpen, revisionDialogOpen, rewritePreviewOpen, savingContextItem, savingProject, searchOpen])

  useEffect(() => {
    const rememberOutsideModalFocus = (event: FocusEvent) => {
      const target = event.target
      if (modalWasOpenRef.current || !(target instanceof HTMLElement) || target.closest('[role="dialog"][aria-modal="true"]')) return
      lastOutsideModalFocusRef.current = target
    }
    document.addEventListener('focusin', rememberOutsideModalFocus)
    return () => document.removeEventListener('focusin', rememberOutsideModalFocus)
  }, [])

  useEffect(() => {
    if (modalOpen) {
      modalWasOpenRef.current = true
      return
    }
    if (!modalWasOpenRef.current) return
    modalWasOpenRef.current = false
    const target = lastOutsideModalFocusRef.current
    lastOutsideModalFocusRef.current = null
    if (target?.isConnected && !target.matches(':disabled')) target.focus({ preventScroll: true })
  }, [modalOpen])

  useEffect(() => {
    if (!modalOpen) return
    const topDialog = () => {
      const dialogs = document.querySelectorAll<HTMLElement>('[role="dialog"][aria-modal="true"]')
      return dialogs[dialogs.length - 1]
    }
    const focusableElements = (dialog: HTMLElement) => Array.from(dialog.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    )).filter((element) => element.getClientRects().length > 0 && element.getAttribute('aria-hidden') !== 'true')

    const dialog = topDialog()
    if (dialog && !dialog.contains(document.activeElement)) {
      const firstFocusable = focusableElements(dialog)[0]
      if (firstFocusable) firstFocusable.focus({ preventScroll: true })
      else {
        dialog.tabIndex = -1
        dialog.focus({ preventScroll: true })
      }
    }

    const handleModalTab = (event: KeyboardEvent) => {
      if (isIMEComposing(event)) return
      if (event.key !== 'Tab') return
      const dialog = topDialog()
      if (!dialog) return
      const focusable = focusableElements(dialog)
      if (focusable.length === 0) {
        event.preventDefault()
        return
      }
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      const activeElement = document.activeElement
      if (event.shiftKey && (activeElement === first || !dialog.contains(activeElement))) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && (activeElement === last || !dialog.contains(activeElement))) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', handleModalTab)
    return () => document.removeEventListener('keydown', handleModalTab)
  }, [characterDialogOpen, helpDialogOpen, modalOpen, noteDialogOpen, projectDeleteDialogOpen, projectDialogOpen, projectEditDialogOpen, rewritePreviewOpen, searchOpen])

  const lineCount = Math.max(1, editorContent.split('\n').length)
  const editorWordCount = useMemo(() => countTextWords(editorContent), [editorContent])

  useEffect(() => {
    const workspaceID = data?.workspace.workspace_id
    const query = searchQuery.trim()
    if (!searchOpen || !workspaceID || !query) {
      setChapterSearchResults([])
      return
    }
    const controller = new AbortController()
    setChapterSearchResults([])
    const timer = window.setTimeout(() => {
      void api.chapter.search(workspaceID, query, controller.signal)
        .then((response) => {
          if (!controller.signal.aborted && activeWorkspaceRef.current === workspaceID) setChapterSearchResults(response.chapters)
        })
        .catch((reason) => {
          if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : '章节搜索失败')
        })
    }, 120)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [data?.workspace.workspace_id, searchOpen, searchQuery])

  const searchResults = useMemo<PaletteResult[]>(() => {
    if (!searchOpen || !data) return []
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
          : command.id === 'new-character' || command.id === 'new-note'
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
    return [...commandResults, ...chapterResults, ...characterResults, ...noteResults]
  }, [chapterSearchResults, characters, currentChapter, data, editorContent, editorSummary, editorTitle, notes, projectActionBusy, searchOpen, searchQuery])
  const activeSearchIndex = Math.min(selectedSearchIndex, Math.max(0, searchResults.length - 1))

  useEffect(() => {
    if (searchOpen) searchActiveResultRef.current?.scrollIntoView({ block: 'nearest' })
  }, [activeSearchIndex, searchOpen])

  const exportProjectMarkdown = async () => {
    const workspaceID = data?.workspace.workspace_id
    const projectTitle = data?.workspace.project.title
    if (!workspaceID || !projectTitle || exportingDocument || chapterBodyLoading) return
    setExportingDocument(true)
    try {
      const chapter = currentChapter
        ? { id: currentChapter.id, title: editorTitle.trim() || '未命名章节', content: editorContent }
        : undefined
      const file = await api.workspace.markdown(workspaceID, chapter)
      const fileURL = URL.createObjectURL(file)
      const link = document.createElement('a')
      link.href = fileURL
      link.download = `${safeProjectFileBaseName(projectTitle)}.md`
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.setTimeout(() => URL.revokeObjectURL(fileURL), 1000)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Markdown 导出失败')
    } finally {
      setExportingDocument(false)
    }
  }

  const exportProjectDocument = async (format: 'docx' | 'epub') => {
    const workspaceID = data?.workspace.workspace_id
    const projectTitle = data?.workspace.project.title
    if (!workspaceID || !projectTitle || projectActionBusy || chapterBodyLoading) return
    setExportingDocument(true)
    try {
      const chapter = currentChapter
        ? { id: currentChapter.id, title: editorTitle.trim() || '未命名章节', content: editorContent }
        : undefined
      const blob = format === 'docx'
        ? await api.workspace.docx(workspaceID, chapter)
        : await api.workspace.epub(workspaceID, chapter)
      const fileURL = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = fileURL
      link.download = `${safeProjectFileBaseName(projectTitle)}.${format}`
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.setTimeout(() => URL.revokeObjectURL(fileURL), 1000)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : `导出 ${format.toUpperCase()} 失败`)
    } finally {
      setExportingDocument(false)
    }
  }

  const updateLocalChapter = useCallback((workspaceId: string, chapter: Chapter) => {
    setData((current) => {
      if (!current || current.workspace.workspace_id !== workspaceId) return current
      const chaptersNext = current.workspace.chapters.map((item) => (item.id === chapter.id
        ? { ...chapter, number: item.number, content: chapter.content ?? item.content ?? '' }
        : item))
      return {
        ...current,
        workspace: { ...current.workspace, chapters: chaptersNext },
        stats: statsForChapters(chaptersNext, current.stats),
      }
    })
  }, [])

  useEffect(() => {
    const workspaceID = data?.workspace.workspace_id
    const chapter = currentChapter
    if (!workspaceID || !chapter || chapter.content !== undefined) {
      setChapterLoadError('')
      return
    }
    const controller = new AbortController()
    setChapterLoadError('')
    void api.chapter.get(workspaceID, chapter.id, controller.signal)
      .then((loaded) => {
        if (controller.signal.aborted || activeWorkspaceRef.current !== workspaceID || currentChapterRef.current?.id !== chapter.id) return
        const content = normalizeEditorLineEndings(loaded.content ?? '')
        updateLocalChapter(workspaceID, { ...loaded, content })
        editorContentRef.current = content
        setEditorTitle(loaded.title)
        setEditorSummary(loaded.summary)
        setEditorContent(content)
        setEditorStatus(loaded.status || 'outline')
        setSaveState('saved')
      })
      .catch((reason) => {
        if (!controller.signal.aborted && activeWorkspaceRef.current === workspaceID && currentChapterRef.current?.id === chapter.id) {
          setChapterLoadError(reason instanceof Error ? reason.message : '无法读取章节正文')
        }
      })
    return () => controller.abort()
  }, [currentChapter, data?.workspace.workspace_id, chapterLoadRetry, updateLocalChapter])

  const saveChapter = useCallback(async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!currentChapter || !workspaceId) return true
    if (currentChapter.content === undefined) return false
    const savedRevision = editorRevisionRef.current
    setSaveState('saving')
    try {
      const saved = await api.chapter.update(workspaceId, currentChapter.id, {
        title: editorTitle.trim() || '未命名章节',
        summary: editorSummary,
        content: editorContent,
        status: editorStatus,
      })
      updateLocalChapter(workspaceId, saved)
      const unchanged = editorRevisionRef.current === savedRevision
      if (unchanged && activeWorkspaceRef.current === workspaceId && currentChapterRef.current?.id === saved.id) {
        setEditorTitle(saved.title)
        setEditorStatus(saved.status)
      }
      if (activeWorkspaceRef.current === workspaceId) {
        setSaveRetryAttempt(0)
        setSaveState(unchanged ? 'saved' : 'dirty')
        if (saveErrorMessageRef.current) {
          const saveErrorMessage = saveErrorMessageRef.current
          saveErrorMessageRef.current = ''
          setError((current) => current === saveErrorMessage ? '' : current)
        }
      }
      return unchanged
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) {
        const message = reason instanceof Error ? reason.message : '章节保存失败'
        saveErrorMessageRef.current = message
        setSaveRetryAttempt((attempt) => Math.min(attempt + 1, 6))
        setSaveState('error')
        setError(message)
      }
      return false
    }
  }, [currentChapter, data?.workspace.workspace_id, editorContent, editorStatus, editorSummary, editorTitle, updateLocalChapter])

  saveChapterRef.current = saveChapter

  const loadChapterRevision = async (revisionSummary: ChapterRevisionSummary, workspaceID: string, chapterID: number) => {
    const requestID = ++revisionPreviewRequestRef.current
    setRevisionPreviewLoading(true)
    try {
      const revision = await api.chapter.revision(workspaceID, chapterID, revisionSummary.id)
      if (requestID === revisionPreviewRequestRef.current && activeWorkspaceRef.current === workspaceID && currentChapterRef.current?.id === chapterID) {
        setSelectedRevision(revision)
        setRevisionError('')
      }
    } catch (reason) {
      if (requestID === revisionPreviewRequestRef.current && activeWorkspaceRef.current === workspaceID) setRevisionError(reason instanceof Error ? reason.message : '无法读取章节版本')
    } finally {
      if (requestID === revisionPreviewRequestRef.current) setRevisionPreviewLoading(false)
    }
  }

  const openChapterRevisions = async () => {
    const workspaceID = data?.workspace.workspace_id
    const chapter = currentChapter
    if (!workspaceID || !chapter || chapter.content === undefined || projectActionBusy) return
    if ((saveState === 'dirty' || saveState === 'error') && !(await saveChapter())) return
    setRevisionCurrentSnapshot({
      title: editorTitle.trim() || '未命名章节',
      summary: editorSummary,
      content: editorContent,
      status: editorStatus,
      word_count: countTextWords(editorContent),
    })
    setRevisionCompareOpen(false)
    setRevisionDialogOpen(true)
    revisionPreviewRequestRef.current += 1
    setChapterRevisions([])
    setSelectedRevision(null)
    setRevisionPreviewLoading(false)
    setRevisionLabel('手动版本')
    setRevisionListLoading(true)
    setRevisionError('')
    try {
      const result = await api.chapter.revisions(workspaceID, chapter.id)
      if (activeWorkspaceRef.current !== workspaceID || currentChapterRef.current?.id !== chapter.id) return
      setChapterRevisions(result.revisions)
      if (result.revisions[0]) void loadChapterRevision(result.revisions[0], workspaceID, chapter.id)
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceID) setRevisionError(reason instanceof Error ? reason.message : '无法读取章节版本')
    } finally {
      setRevisionListLoading(false)
    }
  }

  const saveChapterRevision = async () => {
    const workspaceID = data?.workspace.workspace_id
    const chapter = currentChapter
    if (!workspaceID || !chapter || chapterActionBusy || sending) return
    setChapterActionBusy(true)
    setRevisionError('')
    try {
      if ((saveState === 'dirty' || saveState === 'error') && !(await saveChapter())) {
        setRevisionError(saveErrorMessageRef.current || '章节未能保存，未创建版本。')
        return
      }
      const revision = await api.chapter.createRevision(workspaceID, chapter.id, revisionLabel)
      const result = await api.chapter.revisions(workspaceID, chapter.id)
      if (activeWorkspaceRef.current !== workspaceID || currentChapterRef.current?.id !== chapter.id) return
      setChapterRevisions(result.revisions)
      setSelectedRevision(revision)
      setRevisionLabel('手动版本')
      setRevisionError('')
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceID) setRevisionError(reason instanceof Error ? reason.message : '保存章节版本失败')
    } finally {
      setChapterActionBusy(false)
    }
  }

  const restoreChapterRevision = async () => {
    const workspaceID = data?.workspace.workspace_id
    const chapter = currentChapter
    const revision = selectedRevision
    if (!workspaceID || !chapter || !revision || chapterActionBusy || sending) return
    if (!window.confirm(`恢复到“${revision.label}”（${new Date(revision.created_at).toLocaleString()}）？当前正文会先自动保存为恢复前版本。`)) return
    setChapterActionBusy(true)
    setRevisionError('')
    try {
      if ((saveState === 'dirty' || saveState === 'error') && !(await saveChapter())) {
        setRevisionError(saveErrorMessageRef.current || '章节未能保存，未恢复版本。')
        return
      }
      const restored = await api.chapter.restoreRevision(workspaceID, chapter.id, revision.id)
      if (activeWorkspaceRef.current !== workspaceID || currentChapterRef.current?.id !== chapter.id) return
      const content = normalizeEditorLineEndings(restored.content ?? '')
      updateLocalChapter(workspaceID, { ...restored, content })
      editorContentRef.current = content
      setEditorTitle(restored.title)
      setEditorSummary(restored.summary)
      setEditorContent(content)
      setEditorStatus(restored.status)
      setEditorSelection(null)
      editorRevisionRef.current += 1
      setSaveRetryAttempt(0)
      setSaveState('saved')
      setRevisionDialogOpen(false)
      setSelectedRevision(null)
      setRevisionCompareOpen(false)
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceID) setRevisionError(reason instanceof Error ? reason.message : '恢复章节版本失败')
    } finally {
      setChapterActionBusy(false)
    }
  }

  const selectChapter = async (chapterId: number, closeSidebar = false) => {
    if (closeSidebar) setShowMobileSidebar(false)
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || chapterId === currentChapter?.id || projectActionBusy) return
    if (chatApplyTarget?.workspaceId === workspaceId && chatApplyTarget.chapterId !== chapterId && !confirmDiscardEditedRewrite(workspaceId)) return
    if ((saveState === 'dirty' || saveState === 'error') && !(await saveChapter())) return
    if (activeWorkspaceRef.current === workspaceId) setActiveChapterId(chapterId)
  }

  const selectWorkspace = async (workspaceId: string, chapterAlreadySaved = false, allowDuringProjectDelete = false, rewriteDiscardApproved = false) => {
    if (!workspaceId || workspaceId === activeWorkspaceRef.current || switchingWorkspace || sending || checkingChatRecovery || savingContextItem || savingProject || savingSettings || chapterActionBusy || saveState === 'saving' || (deletingProject && !allowDuringProjectDelete)) return
    if (!rewriteDiscardApproved && !confirmDiscardEditedRewrite(activeWorkspaceRef.current)) return
    const requestID = ++workspaceRequestRef.current
    setSwitchingWorkspace(true)
    if (!chapterAlreadySaved && (saveState === 'dirty' || saveState === 'error') && !(await saveChapter())) {
      setSwitchingWorkspace(false)
      return
    }
    if (settingsDirty && !allowDuringProjectDelete && !(await saveSettings())) {
      setSwitchingWorkspace(false)
      return
    }
    const previousWorkspaceId = activeWorkspaceRef.current
    const previousChatNode = chatScrollRef.current
    if (previousWorkspaceId) {
      chatAutoScrollByWorkspaceRef.current.set(previousWorkspaceId, chatAutoScrollRef.current)
      if (previousChatNode) chatScrollPositionsRef.current.set(previousWorkspaceId, previousChatNode.scrollTop)
    }
    setData(null)
    setChatUnread(unreadChatWorkspacesRef.current.has(workspaceId))
    chatAutoScrollRef.current = chatAutoScrollByWorkspaceRef.current.get(workspaceId) ?? true
    setActiveChapterId(null)
    setChatInput('')
    setSuggestions(suggestionsForMode(writingModeRef.current))
    setSettingsDraft(null)
    setSettingsDirty(false)
    setSettingsSaved(false)
    setSettingsError('')
    setCharacterDialogOpen(false)
    setNoteDialogOpen(false)
    setSaveState('saved')
    setError('')
    activeWorkspaceRef.current = workspaceId
    setActiveWorkspaceId(workspaceId)
    rememberActiveWorkspacePreference(workspaceId)
    try {
      const [next, projectList] = await Promise.all([api.workspace(workspaceId), api.projects.list()])
      if (workspaceRequestRef.current === requestID && activeWorkspaceRef.current === workspaceId) {
        setProjects(projectList.projects)
        setData(next)
        setActiveChapterId(next.workspace.chapters[0]?.id ?? null)
        setSettingsDraft(next.workspace.settings)
        setSettingsDirty(false)
        setSettingsSaved(false)
        setSettingsError('')
        setChatInput(chatDraftsRef.current.get(workspaceId) ?? '')
        setSuggestions(suggestionsForMode(writingModeRef.current))
      }
    } catch (reason) {
      if (workspaceRequestRef.current === requestID) {
        setError(reason instanceof Error ? reason.message : '无法打开这个小说项目')
      }
    }
    setSwitchingWorkspace(false)
  }

  const createProject = async () => {
    if (!projectTitle.trim() || creatingProject || sending || savingContextItem || savingSettings || chapterActionBusy || saveState === 'saving') return
    if (!confirmDiscardEditedRewrite(activeWorkspaceRef.current)) return
    setCreatingProject(true)
    try {
      if ((saveState === 'dirty' || saveState === 'error') && !(await saveChapter())) return
      if (settingsDirty && !(await saveSettings())) return
      const created = await api.projects.create({ title: projectTitle.trim(), genre: projectGenre.trim(), premise: projectPremise.trim() })
      setProjects((current) => [created, ...current])
      setProjectDialogOpen(false)
      setProjectTitle('')
      setProjectGenre('')
      setProjectPremise('')
      await selectWorkspace(created.workspace_id, true, false, true)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '创建项目失败')
    } finally {
      setCreatingProject(false)
    }
  }

  const openProjectEdit = () => {
    if (!project || projectActionBusy) return
    setProjectDraft({
      title: project.title,
      subtitle: project.subtitle,
      genre: project.genre,
      premise: project.premise,
      word_target: project.word_target,
    })
    setProjectEditError('')
    setProjectEditDialogOpen(true)
  }

  const saveProject = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || !projectDraft.title.trim() || projectDraft.word_target < 1 || savingProject || sending || savingContextItem || savingSettings || chapterActionBusy || saveState === 'saving') return
    setSavingProject(true)
    setProjectEditError('')
    try {
      const updated = await api.projects.update(workspaceId, {
        ...projectDraft,
        title: projectDraft.title.trim(),
        subtitle: projectDraft.subtitle.trim(),
        genre: projectDraft.genre.trim(),
        premise: projectDraft.premise.trim(),
        word_target: Math.trunc(projectDraft.word_target),
      })
      setProjects((current) => [updated, ...current.filter((item) => item.workspace_id !== workspaceId)])
      setData((current) => current && current.workspace.workspace_id === workspaceId
        ? { ...current, workspace: { ...current.workspace, project: updated.project, updated_at: updated.project.updated_at }, stats: updated.stats }
        : current)
      setProjectEditDialogOpen(false)
    } catch (reason) {
      setProjectEditError(reason instanceof Error ? reason.message : '保存项目资料失败')
    } finally {
      setSavingProject(false)
    }
  }

  const openProjectDelete = () => {
    if (!project || projects.length <= 1 || projectActionBusy) return
    setProjectDeleteInput('')
    setProjectDeleteError('')
    setProjectDeleteDialogOpen(true)
  }

  const confirmProjectDelete = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || !project || projects.length <= 1 || projectDeleteInput !== project.title || projectActionBusy) return
    if (!confirmDiscardEditedRewrite(workspaceId)) return
    setDeletingProject(true)
    setProjectDeleteError('')
    try {
      const result = await api.projects.remove(workspaceId)
      if (!result.deleted) throw new Error('项目未能删除')
      if (chatDraftsRef.current.delete(workspaceId)) setChatDraftRevision((revision) => revision + 1)
      chatRequestRetryRef.current.delete(workspaceId)
      unreadChatWorkspacesRef.current.delete(workspaceId)
      chatAutoScrollByWorkspaceRef.current.delete(workspaceId)
      chatScrollPositionsRef.current.delete(workspaceId)
      const remainingProjects = projects.filter((item) => item.workspace_id !== workspaceId)
      if (remainingProjects.length === 0) throw new Error('请至少保留一个小说项目')
      setProjects(remainingProjects)
      setProjectDeleteDialogOpen(false)
      setProjectDeleteInput('')
      await selectWorkspace(remainingProjects[0].workspace_id, true, true, true)
    } catch (reason) {
      setProjectDeleteError(reason instanceof Error ? reason.message : '删除项目失败')
    } finally {
      setDeletingProject(false)
    }
  }

  useEffect(() => {
    if (!currentChapter || chapterActionBusy || deletingProject) return
    let delay: number
    if (saveState === 'dirty') {
      delay = 1000
    } else if (saveState === 'error' && saveRetryAttempt > 0 && saveRetryAttempt <= 5) {
      delay = Math.min(5000 * 2 ** (saveRetryAttempt - 1), 30000)
    } else {
      return
    }
    const timer = window.setTimeout(() => void saveChapter(), delay)
    return () => window.clearTimeout(timer)
  }, [chapterActionBusy, currentChapter, deletingProject, saveChapter, saveRetryAttempt, saveState])

  useEffect(() => {
    const chapterHasUnsavedChanges = saveState === 'dirty' || saveState === 'saving' || saveState === 'error'
    if (!chapterHasUnsavedChanges && !settingsDirty && !sending && !hasEditedRewriteDraft && chatDraftsRef.current.size === 0) return
    const warnBeforeLeaving = (event: BeforeUnloadEvent) => {
      if (!chapterHasUnsavedChanges && !settingsDirty && !sending && !hasEditedRewriteDraft && chatDraftsRef.current.size === 0) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warnBeforeLeaving)
    return () => window.removeEventListener('beforeunload', warnBeforeLeaving)
  }, [chatDraftRevision, hasEditedRewriteDraft, saveState, sending, settingsDirty])

  const updateEditor = (next: string) => {
    const content = normalizeEditorLineEndings(next)
    editorContentRef.current = content
    setEditorContent(content)
    setEditorSelection(null)
    editorRevisionRef.current += 1
    setSaveRetryAttempt(0)
    setSaveState('dirty')
  }

  const markEditorDirty = () => {
    editorRevisionRef.current += 1
    setSaveRetryAttempt(0)
    setSaveState('dirty')
  }

  const captureEditorSelection = () => {
    const editor = editorRef.current
    const chapterId = currentChapterRef.current?.id
    if (!editor || !chapterId || editor.selectionStart === editor.selectionEnd) {
      setEditorSelection(null)
      return
    }
    const selection = normalizeUnicodeSelection(editor.value, editor.selectionStart, editor.selectionEnd)
    if (selection.start !== editor.selectionStart || selection.end !== editor.selectionEnd) {
      editor.setSelectionRange(selection.start, selection.end, editor.selectionDirection)
    }
    setEditorSelection({ chapterId, ...selection })
  }

  const createChapter = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy) return
    if (chatApplyTarget?.workspaceId === workspaceId && !confirmDiscardEditedRewrite(workspaceId)) return
    setChapterActionBusy(true)
    try {
      if ((saveState === 'dirty' || saveState === 'error') && !(await saveChapter())) return
      const chapter = await api.chapter.create(workspaceId, { title: `未命名章节 ${String(chapters.length + 1).padStart(2, '0')}`, summary: '', content: '', status: 'outline' })
      if (activeWorkspaceRef.current !== workspaceId) return
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        const chaptersNext = [...current.workspace.chapters, chapter]
        return {
          ...current,
          workspace: { ...current.workspace, chapters: chaptersNext },
          stats: statsForChapters(chaptersNext, current.stats),
        }
      })
      setActiveChapterId(chapter.id)
      setActiveActivity('explorer')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '创建章节失败')
    } finally {
      setChapterActionBusy(false)
    }
  }

  const moveChapter = async (chapterId: number, direction: -1 | 1) => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy) return
    const sourceIndex = chapters.findIndex((chapter) => chapter.id === chapterId)
    const destinationIndex = sourceIndex + direction
    if (sourceIndex < 0 || destinationIndex < 0 || destinationIndex >= chapters.length) return
    const ordered = [...chapters]
    const [moved] = ordered.splice(sourceIndex, 1)
    ordered.splice(destinationIndex, 0, moved)
    const chapterIDs = ordered.map((chapter) => chapter.id)

    setChapterActionBusy(true)
    try {
      const response = await api.chapter.reorder(workspaceId, chapterIDs)
      if (activeWorkspaceRef.current !== workspaceId) return
      const numbers = new Map<number, number>()
      response.chapter_ids.forEach((id, index) => numbers.set(id, index + 1))
      setData((current) => current && current.workspace.workspace_id === workspaceId
        ? {
          ...current,
          workspace: {
            ...current.workspace,
            chapters: current.workspace.chapters
              .map((chapter) => ({ ...chapter, number: numbers.get(chapter.id) ?? chapter.number }))
              .sort((left, right) => left.number - right.number),
          },
        }
        : current)
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setError(reason instanceof Error ? reason.message : '调整章节顺序失败')
    } finally {
      setChapterActionBusy(false)
    }
  }

  const deleteChapter = async (chapter: Chapter) => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy) return
    const hasUnsavedContent = chapter.id === currentChapter?.id && (saveState === 'dirty' || saveState === 'error')
    const warning = hasUnsavedContent ? '\n此章的未保存编辑也会被删除。' : ''
    const rewriteWarning = hasEditedRewriteDraft && chatApplyTarget?.workspaceId === workspaceId && chatApplyTarget.chapterId === chapter.id ? '\n当前有尚未应用的改写编辑；继续删除会丢弃它。' : ''
    if (!window.confirm(`确定永久删除《${chapter.title}》吗？${warning}${rewriteWarning}`)) return
    setChapterActionBusy(true)
    try {
      await api.chapter.remove(workspaceId, chapter.id)
      if (activeWorkspaceRef.current !== workspaceId) return
      const deletedIndex = chapters.findIndex((item) => item.id === chapter.id)
      const chaptersNext = chapters.filter((item) => item.id !== chapter.id)
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        const persistedChapters = current.workspace.chapters
          .filter((item) => item.id !== chapter.id)
          .map((item, index) => ({ ...item, number: index + 1 }))
        return {
          ...current,
          workspace: { ...current.workspace, chapters: persistedChapters },
          stats: statsForChapters(persistedChapters, current.stats),
        }
      })
      if (activeChapterId === chapter.id) {
        const nextIndex = Math.min(Math.max(deletedIndex, 0), chaptersNext.length - 1)
        setActiveChapterId(chaptersNext[nextIndex]?.id ?? null)
        setSaveState('saved')
      }
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setError(reason instanceof Error ? reason.message : '删除章节失败')
    } finally {
      setChapterActionBusy(false)
    }
  }

  const openCharacterCreate = () => {
    if (projectActionBusy) return
    setCharacterEditingId(null)
    setCharacterDraft({ name: '', role: '', description: '', traits: '', color: characterColors[characters.length % characterColors.length] })
    setContextDialogError('')
    setCharacterDialogOpen(true)
  }

  const openCharacterEdit = (character: Character) => {
    if (projectActionBusy) return
    setCharacterEditingId(character.id)
    setCharacterDraft({ name: character.name, role: character.role, description: character.description, traits: character.traits.join('，'), color: character.color })
    setContextDialogError('')
    setCharacterDialogOpen(true)
  }

  const saveCharacter = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || !characterDraft.name.trim() || projectActionBusy) return
    const characterId = characterEditingId
    setSavingContextItem(true)
    setContextDialogError('')
    try {
      const input = {
        name: characterDraft.name.trim(),
        role: characterDraft.role.trim(),
        description: characterDraft.description.trim(),
        traits: characterDraft.traits.split(/[，,\n]/).map((trait) => trait.trim()).filter(Boolean),
        color: characterDraft.color,
      }
      const character = characterId === null
        ? await api.character.create(workspaceId, input)
        : await api.character.update(workspaceId, characterId, input)
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        const charactersNext = characterId === null
          ? [...current.workspace.characters, character]
          : current.workspace.characters.map((item) => item.id === character.id ? character : item)
        return { ...current, workspace: { ...current.workspace, characters: charactersNext } }
      })
      if (activeWorkspaceRef.current === workspaceId) {
        setCharacterDraft({ name: '', role: '', description: '', traits: '', color: '' })
        setCharacterEditingId(null)
        setCharacterDialogOpen(false)
      }
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setContextDialogError(reason instanceof Error ? reason.message : '保存角色失败')
    } finally {
      setSavingContextItem(false)
    }
  }

  const deleteCharacter = async (character: Character) => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy || !window.confirm(`确定删除角色“${character.name}”吗？`)) return
    setSavingContextItem(true)
    try {
      await api.character.remove(workspaceId, character.id)
      setData((current) => current && current.workspace.workspace_id === workspaceId
        ? { ...current, workspace: { ...current.workspace, characters: current.workspace.characters.filter((item) => item.id !== character.id) } }
        : current)
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setError(reason instanceof Error ? reason.message : '删除角色失败')
    } finally {
      setSavingContextItem(false)
    }
  }

  const openNoteCreate = () => {
    if (projectActionBusy) return
    setNoteEditingId(null)
    setNoteDraft({ title: '', body: '', kind: 'idea' })
    setContextDialogError('')
    setNoteDialogOpen(true)
  }

  const openNoteEdit = (note: Note) => {
    if (projectActionBusy) return
    setNoteEditingId(note.id)
    setNoteDraft({ title: note.title, body: note.body, kind: note.kind || 'idea' })
    setContextDialogError('')
    setNoteDialogOpen(true)
  }

  const saveNote = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || !noteDraft.title.trim() || projectActionBusy) return
    const noteId = noteEditingId
    setSavingContextItem(true)
    setContextDialogError('')
    try {
      const input = {
        title: noteDraft.title.trim(),
        body: noteDraft.body,
        kind: noteDraft.kind,
      }
      const note = noteId === null
        ? await api.note.create(workspaceId, input)
        : await api.note.update(workspaceId, noteId, input)
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        const notesNext = noteId === null
          ? [...current.workspace.notes, note]
          : current.workspace.notes.map((item) => item.id === note.id ? note : item)
        return { ...current, workspace: { ...current.workspace, notes: notesNext } }
      })
      if (activeWorkspaceRef.current === workspaceId) {
        setNoteDraft({ title: '', body: '', kind: 'idea' })
        setNoteEditingId(null)
        setNoteDialogOpen(false)
      }
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setContextDialogError(reason instanceof Error ? reason.message : '保存便笺失败')
    } finally {
      setSavingContextItem(false)
    }
  }

  const deleteNote = async (note: Note) => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy || !window.confirm(`确定删除便笺“${note.title}”吗？`)) return
    setSavingContextItem(true)
    try {
      await api.note.remove(workspaceId, note.id)
      setData((current) => current && current.workspace.workspace_id === workspaceId
        ? { ...current, workspace: { ...current.workspace, notes: current.workspace.notes.filter((item) => item.id !== note.id) } }
        : current)
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setError(reason instanceof Error ? reason.message : '删除便笺失败')
    } finally {
      setSavingContextItem(false)
    }
  }

  const activateSearchResult = (result: PaletteResult) => {
    if (result.disabled) return
    closeSearch()
    if (result.kind === 'chapter' && result.entityId !== undefined) {
      setActiveActivity('explorer')
      void selectChapter(result.entityId, true)
      return
    }
    if (result.kind === 'character' && result.entityId !== undefined) {
      const character = characters.find((item) => item.id === result.entityId)
      if (!character) return
      openCharactersView()
      openCharacterEdit(character)
      return
    }
    if (result.kind === 'note' && result.entityId !== undefined) {
      const note = notes.find((item) => item.id === result.entityId)
      if (!note) return
      openNotesView()
      openNoteEdit(note)
      return
    }
    switch (result.id) {
      case 'new-chapter':
        setActiveActivity('explorer')
        setShowMobileSidebar(false)
        void createChapter()
        break
      case 'new-character':
        openCharactersView()
        openCharacterCreate()
        break
      case 'new-note':
        openNotesView()
        openNoteCreate()
        break
      case 'edit-project':
        openProjectEdit()
        break
      case 'export-markdown':
        void exportProjectMarkdown()
        break
      case 'export-docx':
        void exportProjectDocument('docx')
        break
      case 'export-epub':
        void exportProjectDocument('epub')
        break
      case 'export-backup':
        void exportProjectBackup()
        break
      case 'import-backup':
        backupInputRef.current?.click()
        break
      case 'open-outline':
        updateAssistantCollapsed(false)
        activateAssistantTab('outline')
        break
      case 'open-context':
        openProjectContextView()
        break
      case 'open-revisions':
        void openChapterRevisions()
        break
      case 'open-settings':
        setActiveActivity('settings')
        setShowMobileSidebar(true)
        break
      case 'open-help':
        setHelpDialogOpen(true)
        break
      case 'open-characters':
        openCharactersView()
        break
      case 'open-notes':
        openNotesView()
        break
    }
  }

  const sendMessage = async (preset?: string, rewriteDiscardApproved = false) => {
    const submittedInput = preset === undefined ? chatInput : null
    const message = (preset ?? chatInput).trim()
    const inputAtSend = chatInput
    const workspaceId = data?.workspace.workspace_id
    if (!message || projectActionBusy || chapterBodyLoading || !workspaceId || agentRequestRef.current) return
    if (settingsDirty) {
      setError('请先保存模型设置，再发送 Agent 请求')
      return
    }
    if (countUnicodeCharacters(message) > maxAgentInputCharacters) {
      setError(`Agent 输入不能超过 ${maxAgentInputCharacters} 个字符`)
      return
    }
    const requestMode = writingModeRef.current
    const editorContentAtSend = editorContentRef.current
    const selectionRange = activeEditorSelection
      ? { ...activeEditorSelection, ...normalizeUnicodeSelection(editorContentAtSend, activeEditorSelection.start, activeEditorSelection.end) }
      : null
    if (selectionRange && (selectionRange.start !== activeEditorSelection?.start || selectionRange.end !== activeEditorSelection?.end)) {
      setEditorSelection(selectionRange)
    }
    const selectionRangeIsValid = selectionRange !== null && selectionRange.start >= 0 && selectionRange.start < selectionRange.end && selectionRange.end <= editorContentAtSend.length
    const selectedCharacterCount = selectionRangeIsValid && selectionRange ? countUnicodeCharactersInRange(editorContentAtSend, selectionRange.start, selectionRange.end, maxAgentSelectionCharacters) : 0
    if (requestMode === 'edit' && selectedCharacterCount === 0) {
      setError('请先在编辑器正文中选中需要改写的内容')
      return
    }
    if (requestMode === 'edit' && selectedCharacterCount > maxAgentSelectionCharacters) {
      setError(`改写选区不能超过 ${maxAgentSelectionCharacters} 个字符，请缩小选区后重试`)
      return
    }
    const selectedText = selectionRangeIsValid && selectionRange ? sliceUnicodeRange(editorContentAtSend, selectionRange.start, selectionRange.end, maxAgentSelectionCharacters) : ''
    if (requestMode === 'edit' && selectedText.trim() === '') {
      setError('改写选区不能只包含空白字符，请选中正文内容')
      return
    }
    const selectionForRequest = selectedText
    const canApplySelection = requestMode === 'edit' && selectionRange !== null && selectionRangeIsValid && selectedText.trim() !== '' && selectedCharacterCount <= maxAgentSelectionCharacters && (data?.workspace.settings.provider === 'openai' || data?.workspace.settings.provider === 'compatible')
    const inputRevision = chatInputRevisionRef.current
    const chapterIDForRequest = currentChapter?.id ?? 0
    const contextForRequest = currentChapter
      ? selectionRangeIsValid && selectionRange
        ? contextAroundSelection(editorContentAtSend, selectionRange.start, selectionRange.end)
        : sliceUnicodeTail(editorContentAtSend, 1800)
      : ''
    const chapterTitleForRequest = currentChapter ? editorTitle.trim() || currentChapter.title : ''
    const chapterSummaryForRequest = currentChapter ? editorSummary : ''
    const requestContextSignature = JSON.stringify([
      [data?.workspace.project.title, data?.workspace.project.genre, data?.workspace.project.premise],
      data?.workspace.characters.slice(0, 16).map((character) => [character.name, character.role, character.description, character.traits.slice(0, 20)]),
      data?.workspace.notes.slice(0, 20).map((note) => [note.title, note.kind, note.body]),
      [data?.workspace.settings.provider, data?.workspace.settings.model, data?.workspace.settings.base_url, data?.workspace.settings.temperature],
      data?.workspace.chat.filter((item) => item.role !== 'system').slice(-8).map((item) => [item.role, item.content]),
      currentChapter?.number,
    ])
    const requestSignature = JSON.stringify([message, requestMode, chapterIDForRequest, contextForRequest, selectionForRequest, chapterTitleForRequest, chapterSummaryForRequest, requestContextSignature])
    const pendingRequest = chatRequestRetryRef.current.get(workspaceId)
    if (pendingRequest?.needsReconcile) {
      setError('请先检查最新聊天记录，确认上次请求的结果后再发送。')
      return
    }
    if (pendingRequest?.requiresReview) {
      setError('请先检查聊天记录，再使用下方的“已检查，生成新请求 ID 重发”操作。')
      return
    }
    if (pendingRequest && pendingRequest.signature !== requestSignature) chatRequestRetryRef.current.delete(workspaceId)
    const requestID = pendingRequest?.signature === requestSignature ? pendingRequest.requestID : crypto.randomUUID()
    if (!rewriteDiscardApproved && !confirmDiscardEditedRewrite(workspaceId)) return
    setError((current) => current === saveErrorMessageRef.current ? current : '')
    clearChatApplyTarget()
    const optimisticMessageID = -Date.now()
    const streamingMessageID = optimisticMessageID - 1
    const controller = new AbortController()
    agentRequestRef.current = controller
    chatAutoScrollRef.current = true
    chatAutoScrollByWorkspaceRef.current.set(workspaceId, true)
    setWorkspaceChatUnread(workspaceId, false)
    if (submittedInput !== null) setChatInput('')
    const optimisticMessage: ChatMessage = {
      id: optimisticMessageID,
      role: 'user',
      content: message,
      created_at: new Date().toISOString(),
      mode: requestMode,
      request_id: requestID,
    }
    setData((current) => current && current.workspace.workspace_id === workspaceId
      ? { ...current, workspace: { ...current.workspace, chat: [...current.workspace.chat, optimisticMessage].slice(-60) } }
      : current)
    setSending(true)
    try {
      let streamedContent = ''
      const response = await api.chatStream(
        workspaceId,
        requestID,
        message,
        requestMode,
        chapterIDForRequest,
        contextForRequest,
        selectionForRequest,
        chapterTitleForRequest,
        chapterSummaryForRequest,
        {
          signal: controller.signal,
          onDelta: (text) => {
            streamedContent += text
            setData((current) => {
              if (!current || current.workspace.workspace_id !== workspaceId) return current
              const assistant: ChatMessage = {
                id: streamingMessageID,
                role: 'assistant',
                content: streamedContent,
                created_at: new Date().toISOString(),
                mode: requestMode,
              }
              const withoutStream = current.workspace.chat.filter((item) => item.id !== streamingMessageID)
              return { ...current, workspace: { ...current.workspace, chat: [...withoutStream, assistant].slice(-60) } }
            })
          },
        },
      )
      setError((current) => current === saveErrorMessageRef.current ? current : '')
      if (chatRequestRetryRef.current.get(workspaceId)?.requestID === requestID) chatRequestRetryRef.current.delete(workspaceId)
      setWorkspaceChatUnread(workspaceId, !chatIsVisibleAtLatest(workspaceId))
      if (canApplySelection && selectionRange && response.message.id !== undefined) {
        const latestContent = editorContentRef.current
        const selectionStillCurrent = activeWorkspaceRef.current === workspaceId &&
          currentChapterRef.current?.id === selectionRange.chapterId &&
          latestContent.slice(selectionRange.start, selectionRange.end) === selectedText &&
          contextAroundSelection(latestContent, selectionRange.start, selectionRange.end) === contextForRequest
        if (selectionStillCurrent) {
          setChatApplyTarget({
            workspaceId,
            chapterId: selectionRange.chapterId,
            start: selectionRange.start,
            end: selectionRange.end,
            messageId: response.message.id,
            original: selectedText,
            replacement: normalizeEditorLineEndings(response.message.content),
            generatedReplacement: normalizeEditorLineEndings(response.message.content),
            context: contextForRequest,
          })
        } else {
          setError('生成期间选区内容或邻近正文已变化，本次回复不能直接替换。')
        }
      }
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        return {
          ...current,
          workspace: {
            ...current.workspace,
            chat: (() => {
              const currentChat = current.workspace.chat.filter((item) => item.id !== optimisticMessageID && item.id !== streamingMessageID)
              const responseUserMessage = response.user_message ?? optimisticMessage
              if (typeof responseUserMessage.id !== 'number' || typeof response.message.id !== 'number') {
                return [...currentChat, responseUserMessage, response.message].slice(-60)
              }
              const knownMessages = currentChat.filter((item): item is ChatMessage & { id: number } => typeof item.id === 'number')
              const latestMessageID = knownMessages.reduce((latest, item) => Math.max(latest, item.id), 0)
              const messagesByID = new Map<number, ChatMessage>()
              knownMessages.forEach((item) => messagesByID.set(item.id, item))
              messagesByID.set(responseUserMessage.id, responseUserMessage)
              messagesByID.set(response.message.id as number, response.message)
              const mergedChat = [
                ...currentChat.filter((item) => typeof item.id !== 'number'),
                ...[...messagesByID.values()].sort((left, right) => (left.id ?? 0) - (right.id ?? 0)),
              ]
              return responseUserMessage.id < latestMessageID ? mergedChat : mergedChat.slice(-60)
            })(),
          },
        }
      })
      if (submittedInput !== null && chatInputRevisionRef.current === inputRevision) {
        if (chatDraftsRef.current.delete(workspaceId)) setChatDraftRevision((revision) => revision + 1)
      }
      if (activeWorkspaceRef.current === workspaceId && writingModeRef.current === requestMode) {
        const nextSuggestions = Array.isArray(response.suggestions) && response.suggestions.length > 0
          ? response.suggestions.filter((suggestion): suggestion is string => typeof suggestion === 'string' && suggestion.trim().length > 0)
          : []
        setSuggestions(nextSuggestions.length > 0 ? nextSuggestions : suggestionsForMode(requestMode))
      }
    } catch (reason) {
      const requestIDConflict = reason instanceof ApiError && reason.status === 409
      const knownRequestIDConflict = requestIDConflict && reason instanceof ApiError && reason.code === 'chat_request_conflict'
      const requiresReview = requestIDConflict && !knownRequestIDConflict
      let reconciled = false
      let submittedMessageWasPersisted = false
      let assistantReplyWasPersisted = false
      try {
        const refreshed = await api.workspace(workspaceId, AbortSignal.timeout(5000))
        if (activeWorkspaceRef.current === workspaceId) {
          const persistedUserMessageIndex = requestIDConflict ? -1 : refreshed.workspace.chat.findIndex((chatMessage) =>
            chatMessage.role === 'user' && chatMessage.request_id === requestID,
          )
          submittedMessageWasPersisted = persistedUserMessageIndex >= 0
          assistantReplyWasPersisted = persistedUserMessageIndex >= 0 &&
            refreshed.workspace.chat[persistedUserMessageIndex + 1]?.role === 'assistant' &&
            refreshed.workspace.chat[persistedUserMessageIndex + 1]?.mode === requestMode
          if (assistantReplyWasPersisted && !chatIsVisibleAtLatest(workspaceId)) setWorkspaceChatUnread(workspaceId, true)
          setData((current) => current && current.workspace.workspace_id === workspaceId
            ? { ...current, workspace: { ...current.workspace, chat: refreshed.workspace.chat } }
            : current)
          reconciled = true
        }
      } catch {
        // Keep the optimistic rollback path when the server cannot be reached.
      }
      if (!reconciled) {
        setData((current) => current && current.workspace.workspace_id === workspaceId
          ? { ...current, workspace: { ...current.workspace, chat: current.workspace.chat.filter((item) => item.id !== optimisticMessageID && item.id !== streamingMessageID) } }
          : current)
      }
      if (submittedMessageWasPersisted && assistantReplyWasPersisted) {
        if (chatRequestRetryRef.current.get(workspaceId)?.requestID === requestID) chatRequestRetryRef.current.delete(workspaceId)
      } else {
        chatRequestRetryRef.current.set(workspaceId, {
          signature: requestSignature,
          requestID: knownRequestIDConflict ? crypto.randomUUID() : requestID,
          message,
          mode: requestMode,
          safeToRetry: knownRequestIDConflict || (!requestIDConflict && reconciled && !submittedMessageWasPersisted),
          requiresReview: requiresReview || (submittedMessageWasPersisted && !assistantReplyWasPersisted),
          requiresFreshRequestID: requiresReview,
          needsReconcile: !reconciled,
        })
      }
      if (submittedInput !== null && chatInputRevisionRef.current === inputRevision) {
        if (submittedMessageWasPersisted) {
          if (chatDraftsRef.current.delete(workspaceId)) setChatDraftRevision((revision) => revision + 1)
        } else {
          setChatInput(submittedInput)
        }
      } else if (submittedInput === null && !inputAtSend.trim() && chatInputRevisionRef.current === inputRevision && !submittedMessageWasPersisted) {
        updateChatInput(message)
      }
      if (!reconciled) {
        setError('暂时无法确认这条消息是否已保存。恢复连接后请先检查聊天记录，再决定是否重新发送。')
      } else if (requestIDConflict && !knownRequestIDConflict) {
        setError('这条请求与旧聊天记录无法安全匹配。聊天记录已更新，输入已保留；请先检查记录再决定是否重新发送。')
      } else if (!controller.signal.aborted) {
        setError(reason instanceof Error ? reason.message : 'Agent 暂时无法响应')
      }
    } finally {
      if (agentRequestRef.current === controller) agentRequestRef.current = null
      setCancellingRequest(false)
      setSending(false)
    }
  }

  const checkChatRecovery = async () => {
    const workspaceId = data?.workspace.workspace_id
    const retry = workspaceId ? chatRequestRetryRef.current.get(workspaceId) : undefined
    if (!workspaceId || !retry?.needsReconcile || checkingChatRecovery || sending) return
    setCheckingChatRecovery(true)
    try {
      const refreshed = await api.workspace(workspaceId, AbortSignal.timeout(5000))
      if (activeWorkspaceRef.current !== workspaceId) return
      const persistedUserMessageIndex = refreshed.workspace.chat.findIndex((chatMessage) =>
        chatMessage.role === 'user' && chatMessage.request_id === retry.requestID,
      )
      const assistantReplyWasPersisted = persistedUserMessageIndex >= 0 &&
        refreshed.workspace.chat[persistedUserMessageIndex + 1]?.role === 'assistant' &&
        refreshed.workspace.chat[persistedUserMessageIndex + 1]?.mode === retry.mode
      setData((current) => current && current.workspace.workspace_id === workspaceId
        ? { ...current, workspace: { ...current.workspace, chat: refreshed.workspace.chat } }
        : current)
      if (assistantReplyWasPersisted && !retry.requiresFreshRequestID) {
        chatRequestRetryRef.current.delete(workspaceId)
        setWorkspaceChatUnread(workspaceId, !chatIsVisibleAtLatest(workspaceId))
        setError((current) => current.startsWith('暂时无法确认这条消息是否已保存') || current.startsWith('仍无法检查聊天记录') ? '' : current)
      } else if (persistedUserMessageIndex >= 0 || retry.requiresReview) {
        chatRequestRetryRef.current.set(workspaceId, { ...retry, safeToRetry: false, requiresReview: true, needsReconcile: false })
        setError('聊天记录已同步，但仍需检查这条请求是否已有回复，再决定是否重发。')
      } else {
        chatRequestRetryRef.current.set(workspaceId, { ...retry, safeToRetry: true, needsReconcile: false })
        setError((current) => current.startsWith('暂时无法确认这条消息是否已保存') || current.startsWith('仍无法检查聊天记录') ? '' : current)
      }
    } catch (reason) {
      setError(reason instanceof Error ? '仍无法检查聊天记录：' + reason.message : '仍无法检查聊天记录，请恢复连接后重试。')
    } finally {
      setCheckingChatRecovery(false)
    }
  }

  const retryAfterReview = () => {
    const workspaceId = data?.workspace.workspace_id
    const retry = workspaceId ? chatRequestRetryRef.current.get(workspaceId) : undefined
    if (!workspaceId || !retry?.requiresReview || retry.needsReconcile || sending) return
    if (!confirmDiscardEditedRewrite(workspaceId)) return
    chatRequestRetryRef.current.delete(workspaceId)
    void sendMessage(chatInput.trim() === retry.message ? undefined : retry.message, true)
  }

  const applyChatRewrite = (target: ChatApplyTarget) => {
    if (rewritePreviewOpen) lastOutsideModalFocusRef.current = editorRef.current
    const currentContent = editorContentRef.current
    if (activeWorkspaceRef.current !== target.workspaceId || currentChapterRef.current?.id !== target.chapterId) {
      setError('当前章节已切换，请在对应章节重新生成改写。')
      clearChatApplyTarget()
      return
    }
    if (target.start < 0 || target.start >= target.end || target.end > currentContent.length || currentContent.slice(target.start, target.end) !== target.original) {
      setError('选区内容已变化，未应用改写。请重新选择正文并发送请求。')
      clearChatApplyTarget()
      return
    }
    if (contextAroundSelection(currentContent, target.start, target.end) !== target.context) {
      setError('选区附近的正文已变化，未应用改写。请检查上下文后重新生成。')
      clearChatApplyTarget()
      return
    }
    const updatedContent = currentContent.slice(0, target.start) + target.replacement + currentContent.slice(target.end)
    updateEditor(updatedContent)
    setEditorSelection({ chapterId: target.chapterId, start: target.start, end: target.start + target.replacement.length })
    clearChatApplyTarget()
    setError('')
    window.requestAnimationFrame(() => {
      const editor = editorRef.current
      if (!editor) return
      editor.focus()
      editor.setSelectionRange(target.start, target.start + target.replacement.length)
    })
  }

  const confirmRewritePreview = () => {
    if (chatApplyTarget) applyChatRewrite(chatApplyTarget)
  }
  const cancelAgentRequest = () => {
    const controller = agentRequestRef.current
    if (!controller || controller.signal.aborted) return
    setCancellingRequest(true)
    controller.abort()
  }

  const applyGeneratedText = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!currentChapter || currentChapter.content === undefined || !workspaceId || projectActionBusy || agentRequestRef.current) return
    if (!confirmDiscardEditedRewrite(workspaceId, currentChapter.id)) return
    if (settingsDirty) {
      setError('请先保存模型设置，再发送 Agent 请求')
      return
    }
    const instruction = chatInput.trim() || '继续当前场景，保持人物动机一致。'
    if (countUnicodeCharacters(instruction) > maxAgentInputCharacters) {
      setError(`续写要求不能超过 ${maxAgentInputCharacters} 个字符`)
      return
    }
    const chapterId = currentChapter.id
    const controller = new AbortController()
    agentRequestRef.current = controller
    setSending(true)
    try {
      const generated = await api.chapter.generate(
        workspaceId,
        chapterId,
        instruction,
        sliceUnicodeTail(editorContentRef.current, 1800),
        editorTitle.trim() || currentChapter.title,
        editorSummary,
        controller.signal,
      )
      if (activeWorkspaceRef.current !== workspaceId || currentChapterRef.current?.id !== chapterId) return
      setError((current) => current === saveErrorMessageRef.current ? current : '')
      const latestContent = editorContentRef.current
      const separator = latestContent.trim() ? '\n\n' : ''
      clearChatApplyTarget()
      updateEditor(`${latestContent}${separator}${generated.text}`)
      activateAssistantTab('chat')
    } catch (reason) {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : '生成正文失败')
    } finally {
      if (agentRequestRef.current === controller) agentRequestRef.current = null
      setCancellingRequest(false)
      setSending(false)
    }
  }

  const saveSettings = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!settingsDraft || !workspaceId || savingSettings) return false
    setSavingSettings(true)
    try {
      const { api_key, api_key_configured: _keyConfigured, base_url_credentials_configured: _urlCredentialsConfigured, clear_api_key, clear_base_url_credentials, ...settingsFields } = settingsDraft
      const settingsInput: SettingsUpdate = { ...settingsFields }
      if (clear_api_key) settingsInput.api_key = ''
      else if (api_key?.trim()) settingsInput.api_key = api_key
      if (clear_base_url_credentials) settingsInput.clear_base_url_credentials = true
      const saved = await api.settings.update(workspaceId, settingsInput)
      chatRequestRetryRef.current.delete(workspaceId)
      setData((current) => current && current.workspace.workspace_id === workspaceId ? { ...current, workspace: { ...current.workspace, settings: saved } } : current)
      if (activeWorkspaceRef.current === workspaceId) {
        setSettingsDraft(saved)
        setSettingsDirty(false)
        setSettingsSaved(true)
        setSettingsError('')
        window.setTimeout(() => {
          if (activeWorkspaceRef.current === workspaceId) setSettingsSaved(false)
        }, 1800)
      }
      return true
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : 'settings save failed'
      setSettingsError(message)
      setError(message)
      return false
    } finally {
      setSavingSettings(false)
    }
  }

  const exportProjectBackup = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy) return
    setCreatingBackup(true)
    try {
      if ((saveState === 'dirty' || saveState === 'error') && !(await saveChapter())) return
      if (settingsDirty && !(await saveSettings())) return
      const blob = await api.workspace.backup(workspaceId)
      const fileURL = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = fileURL
      link.download = `${safeProjectFileBaseName(data!.workspace.project.title)}.novelstudio.json`
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.setTimeout(() => URL.revokeObjectURL(fileURL), 1000)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '导出项目备份失败')
    } finally {
      setCreatingBackup(false)
    }
  }

  const importProjectBackup = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    if (!file || projectActionBusy) return
    if (file.size > 256 * 1024 * 1024) {
      setError('备份文件超过 256 MiB，无法导入。')
      return
    }
    const rewriteWarning = hasEditedRewriteDraft ? '\n当前有尚未应用的改写编辑；导入后切换项目会丢弃它。' : ''
    if (!window.confirm('备份将作为新项目导入；同名项目会标记为“恢复副本”。最近 60 条对话会恢复。未发送的 Agent 输入不会带入新项目。API Key 不包含在备份中，接口地址中的凭据参数也会移除；导入后请检查模型设置。' + rewriteWarning + '\n继续？')) return

    setImportingBackup(true)
    setError('')
    try {
      if ((saveState === 'dirty' || saveState === 'error') && !(await saveChapter())) return
      if (settingsDirty && !(await saveSettings())) return
      const imported = await api.projects.importBackup(file)
      setChatUnread(unreadChatWorkspacesRef.current.has(imported.workspace_id))
      chatAutoScrollRef.current = chatAutoScrollByWorkspaceRef.current.get(imported.workspace_id) ?? true
      setData(null)
      setChatInput('')
      setActiveChapterId(null)
      setSettingsDraft(null)
      setSettingsDirty(false)
      setSettingsSaved(false)
      setSaveState('saved')
      setActiveActivity('explorer')
      await loadWorkspace(imported.workspace_id)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '导入项目备份失败')
    } finally {
      setImportingBackup(false)
    }
  }

  const updateSettingsDraft = (settings: SettingsDraft) => {
    setSettingsDraft(settings)
    setSettingsDirty(true)
    setSettingsSaved(false)
    setSettingsError('')
  }

  const handleEditorKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (isIMEComposing(event.nativeEvent)) return
    if (event.key !== 'Tab') return
    event.preventDefault()
    const target = event.currentTarget
    const start = target.selectionStart
    const end = target.selectionEnd
    const content = target.value
    const selectionDirection = target.selectionDirection
    let next: string
    let nextSelectionStart: number
    let nextSelectionEnd: number

    if (start === end && event.shiftKey) {
      const lineStart = content.lastIndexOf('\n', start - 1) + 1
      const line = content.slice(lineStart)
      const removeCount = line.startsWith('\t') ? 1 : line.match(/^ {1,2}/)?.[0].length ?? 0
      if (removeCount === 0) return
      next = `${content.slice(0, lineStart)}${line.slice(removeCount)}`
      nextSelectionStart = Math.max(lineStart, start - removeCount)
      nextSelectionEnd = nextSelectionStart
    } else if (start === end) {
      next = `${content.slice(0, start)}  ${content.slice(end)}`
      nextSelectionStart = start + 2
      nextSelectionEnd = start + 2
    } else {
      const lineStart = content.lastIndexOf('\n', start - 1) + 1
      const endsAtNextLineStart = content[end - 1] === '\n'
      const blockSearchStart = endsAtNextLineStart ? end - 1 : end
      const nextLineStart = content.indexOf('\n', blockSearchStart)
      const blockEnd = nextLineStart < 0 ? content.length : nextLineStart
      const lines = content.slice(lineStart, blockEnd).split('\n')

      if (event.shiftKey) {
        let lineOffset = lineStart
        let removedBeforeStart = 0
        let removedBeforeEnd = 0
        let totalRemoved = 0
        const outdentedLines = lines.map((line) => {
          const removeCount = line.startsWith('\t') ? 1 : line.startsWith('  ') ? 2 : line.startsWith(' ') ? 1 : 0
          removedBeforeStart += Math.min(removeCount, Math.max(0, start - lineOffset))
          removedBeforeEnd += Math.min(removeCount, Math.max(0, end - lineOffset))
          totalRemoved += removeCount
          lineOffset += line.length + 1
          return line.slice(removeCount)
        })
        if (totalRemoved === 0) return
        next = `${content.slice(0, lineStart)}${outdentedLines.join('\n')}${content.slice(blockEnd)}`
        nextSelectionStart = start - removedBeforeStart
        nextSelectionEnd = Math.max(nextSelectionStart, end - removedBeforeEnd)
      } else {
        const indentedBlock = lines.map((line) => `  ${line}`).join('\n')
        next = `${content.slice(0, lineStart)}${indentedBlock}${content.slice(blockEnd)}`
        nextSelectionStart = start + 2
        nextSelectionEnd = end + lines.length * 2
      }
    }

    updateEditor(next)
    requestAnimationFrame(() => {
      target.setSelectionRange(nextSelectionStart, nextSelectionEnd, selectionDirection)
      if (editorRef.current === target) captureEditorSelection()
    })
  }

  if (error && !data) {
    return (
      <div className="connection-screen">
        <div className="connection-card">
          <div className="brand-mark large"><PenLine size={24} /></div>
          <p className="eyebrow">NOVELSTUDIO / LOCAL WORKSPACE</p>
          <h1>连接本地工作区</h1>
          <p>Go 后端尚未响应。启动服务后刷新页面即可继续写作。</p>
          <code>go run ./cmd/novelstudio</code>
          <button className="primary-button" onClick={() => void loadWorkspace()}><RefreshCw size={15} /> 重试连接</button>
          <span className="connection-error" role="alert">{error}</span>
        </div>
      </div>
    )
  }

  return (
    <div className="app-shell">
      <input ref={backupInputRef} type="file" accept="application/json,.json" hidden onChange={(event) => void importProjectBackup(event)} />
      <span className="sr-only" role="status" aria-live="polite" aria-atomic="true">{chatUnread && !chatIsVisibleAtLatest() ? '写作 Agent 已回复，对话中有未读消息。' : ''}</span>
      <header className="topbar">
        <div className="topbar-brand">
          <div className="brand-mark"><PenLine size={16} strokeWidth={2.5} /></div>
          <span className="brand-name">Novel<span>Studio</span></span>
          <span className="brand-divider" />
          <div className="workspace-picker">
            <select aria-label="切换小说项目" value={activeWorkspaceId} onChange={(event) => void selectWorkspace(event.target.value)} disabled={projects.length === 0 || projectActionBusy}>
              {projects.map((item) => <option key={item.workspace_id} value={item.workspace_id}>{item.project.title}</option>)}
            </select>
            <ChevronDown size={12} className="workspace-picker-chevron" />
          </div>
            <button className="create-project-button" title="新建小说项目" aria-label="新建小说项目" onClick={() => setProjectDialogOpen(true)} disabled={!data || projectActionBusy}><Plus size={14} /></button>
        </div>
        <div className="topbar-center">
          <button className="command-search" onClick={openSearch} title="搜索（Ctrl/⌘ + K）" aria-label="搜索章节、角色、便笺或命令" disabled={!data}><Search size={14} /><span>搜索项目内容或命令</span><kbd>⌘/Ctrl K</kbd></button>
        </div>
        <div className="topbar-actions">
          <div className="save-indicator" aria-hidden="true"><span className={`status-dot ${saveState === 'saved' ? 'green' : saveState === 'saving' ? 'amber' : 'red'}`} />{saveStatusLabel}</div>
          <span className="sr-only" role="status" aria-live="polite" aria-atomic="true">{saveStatusLabel}</span>
          <button className="icon-button mobile-search" onClick={openSearch} title="搜索（Ctrl/⌘ + K）" aria-label="搜索当前项目" disabled={!data}><Search size={16} /></button>
          <button className="icon-button" onClick={() => setHelpDialogOpen(true)} title="帮助与快捷键" aria-label="帮助与快捷键"><CircleHelp size={16} /></button>
          <div className="avatar">W</div>
        </div>
      </header>

      <div className="shell-body">
        <nav className="activitybar">
          <div className="activity-group">
            <ActivityButton icon={<FolderOpen size={19} />} active={activeActivity === 'explorer'} label="资源管理器" onClick={() => { setActiveActivity('explorer'); setShowMobileSidebar(true) }} />
            <ActivityButton icon={<BookOpen size={19} />} active={activeActivity === 'chapters'} label="章节" badge={chapters.length} onClick={() => { setActiveActivity('chapters'); setShowMobileSidebar(true) }} />
            <ActivityButton icon={<Users size={19} />} active={activeActivity === 'characters'} label="角色" onClick={() => { setActiveActivity('characters'); setShowMobileSidebar(true) }} />
            <ActivityButton icon={<Lightbulb size={19} />} active={activeActivity === 'notes'} label="灵感便笺" onClick={() => { setActiveActivity('notes'); setShowMobileSidebar(true) }} />
          </div>
          <div className="activity-group bottom">
            <ActivityButton icon={<Settings2 size={19} />} active={activeActivity === 'settings'} label="设置" onClick={() => { setActiveActivity('settings'); setShowMobileSidebar(true) }} />
            <ActivityButton icon={<PanelRightOpen size={19} />} active={!assistantCollapsed} expanded={!assistantCollapsed} label={assistantCollapsed ? '展开 Agent 面板' : '收起 Agent 面板'} unread={assistantCollapsed && chatUnread} buttonRef={assistantToggleRef} onClick={() => updateAssistantCollapsed(!assistantCollapsedRef.current, true)} />
          </div>
        </nav>

        <aside className={`sidebar ${sidebarCollapsed ? 'collapsed' : ''} ${showMobileSidebar ? 'mobile-open' : ''}`}>
          <div className="sidebar-header">
            <span>{activeActivity === 'settings' ? '设置' : activeActivity === 'characters' ? '角色档案' : activeActivity === 'notes' ? '灵感便笺' : '资源管理器'}</span>
            <div className="sidebar-header-actions">
              <button className="subtle-icon" onClick={() => setSidebarCollapsed(true)} title="收起侧栏" aria-label="收起侧栏"><LayoutPanelLeft size={15} /></button>
              <button className="subtle-icon mobile-close" onClick={() => setShowMobileSidebar(false)} title="关闭" aria-label="关闭侧栏"><X size={15} /></button>
            </div>
          </div>
          {activeActivity === 'settings' && <SettingsView settings={settingsDraft} setSettings={updateSettingsDraft} onSave={() => void saveSettings()} saved={settingsSaved} saving={savingSettings} locked={projectActionBusy} dirty={settingsDirty} error={settingsError} />}
          {activeActivity === 'characters' && <CharactersView characters={characters} busy={projectActionBusy} onCreate={openCharacterCreate} onEdit={openCharacterEdit} onDelete={(character) => void deleteCharacter(character)} />}
          {activeActivity === 'notes' && <NotesView notes={notes} busy={projectActionBusy} onCreate={openNoteCreate} onEdit={openNoteEdit} onDelete={(note) => void deleteNote(note)} />}
          {activeActivity !== 'settings' && activeActivity !== 'characters' && activeActivity !== 'notes' && (
            <ExplorerView project={project} chapters={chapters} activeChapterId={activeChapterId} setActiveChapterId={(id) => void selectChapter(id, true)} onCreate={createChapter} onExport={exportProjectMarkdown} onExportDocx={() => void exportProjectDocument('docx')} onBackup={exportProjectBackup} onRestore={() => backupInputRef.current?.click()} onEditProject={openProjectEdit} onDeleteProject={openProjectDelete} canDeleteProject={projects.length > 1} projectBusy={projectActionBusy} onOpenWorld={openProjectContextView} onOpenCharacters={openCharactersView} onOpenNotes={openNotesView} onMove={(id, direction) => void moveChapter(id, direction)} onDelete={(chapter) => void deleteChapter(chapter)} chapterBusy={projectActionBusy} activeActivity={activeActivity} />
          )}
        </aside>

        {sidebarCollapsed && <button className="sidebar-reopen" onClick={() => setSidebarCollapsed(false)} title="展开资源管理器" aria-label="展开资源管理器"><PanelRightOpen size={16} /></button>}

        <main className="main-workspace">
          {!data ? (
            <section className="connection-screen workspace-load-state" role={error ? 'alert' : 'status'} aria-live="polite" aria-atomic="true" aria-busy={!error}>
              <div className="connection-card">
                <div className="brand-mark"><PenLine size={16} strokeWidth={2.5} /></div>
                <p className="eyebrow">NOVELSTUDIO WORKSPACE</p>
                <h1>{error ? '无法打开工作区' : switchingWorkspace ? '正在切换项目' : '正在读取工作区'}</h1>
                <p>{error || '正在连接 NovelStudio 后端并读取项目数据。'}</p>
                {error && projects.length > 0 && <p className="workspace-switch-hint">也可以从左上角的项目选择器打开其他项目。</p>}
                {error && <div className="workspace-load-actions">
                  <button className="primary-button" onClick={() => void loadWorkspace(activeWorkspaceRef.current)}>重试</button>
                  {error === '还没有小说项目，请先创建一个项目。' && <button className="toolbar-button" onClick={() => setProjectDialogOpen(true)}>新建项目</button>}
                </div>}
              </div>
            </section>
          ) : (
          <>
          <div className="editor-tabs">
          <div className="editor-tab active"><FileText size={14} /><span>{currentChapter ? `第 ${currentChapter.number} 章 · ${editorTitle || currentChapter.title}` : project?.title ?? '欢迎页'}</span><span className="tab-close">×</span></div>
            <button className="new-tab" onClick={createChapter} title="新建章节" aria-label="新建章节" disabled={projectActionBusy}><Plus size={15} /></button>
            <div className="editor-tab-actions"><button className="subtle-icon" onClick={openSearch} title="打开命令面板" aria-label="打开命令面板" disabled={!data}><MoreHorizontal size={16} /></button></div>
          </div>

          <div className="workbench">
            <section className="editor-pane">
              {currentChapter ? (
                <>
                  <div className="editor-toolbar">
                    <div className="breadcrumbs"><span className="crumb-muted">{project?.title}</span><ChevronRight size={13} /><span>章节</span><ChevronRight size={13} /><span className="crumb-current">第 {currentChapter.number} 章</span></div>
                    <div className="editor-actions"><button className="toolbar-button" onClick={() => void saveChapter()} disabled={editorLocked}><Save size={14} /> 保存</button><button className="toolbar-button" onClick={() => void openChapterRevisions()} disabled={editorLocked} title="查看和恢复章节版本"><History size={14} /> 版本</button><button className="toolbar-button primary-soft" onClick={() => void applyGeneratedText()} disabled={editorLocked || projectActionBusy || settingsDirty}><WandSparkles size={14} /> {sending ? '生成中' : 'AI 续写'}</button></div>
                  </div>
                  <div className="editor-scroll">
                    {chapterBodyLoading && !chapterLoadError && <div className="revision-empty" role="status">正在读取章节正文…</div>}
                    {chapterLoadError && <div className="revision-empty" role="alert">{chapterLoadError} <button className="toolbar-button" onClick={() => setChapterLoadRetry((retry) => retry + 1)}>重试</button></div>}
                    <div className="chapter-heading">
                      <input className="chapter-title-input" value={editorTitle} onChange={(event) => { setEditorTitle(event.target.value); markEditorDirty() }} maxLength={120} disabled={editorLocked} aria-label="章节标题" />
                      <div className="chapter-meta"><label className={`chapter-status ${statusClass(editorStatus)}`}><span /><select value={editorStatus} onChange={(event) => { setEditorStatus(event.target.value); markEditorDirty() }} disabled={editorLocked} aria-label="章节状态"><option value="outline">大纲</option><option value="draft">草稿</option><option value="review">审阅中</option><option value="done">已完成</option></select></label><span className="meta-separator" /><span>{formatNumber(editorWordCount)} 字</span><span className="meta-separator" /><span>最后编辑于 {relativeTime(currentChapter.last_edited_at)}</span></div>
                    </div>
                    <div className="summary-box"><div className="summary-label"><Target size={13} /> 本章目标</div><textarea value={editorSummary} onChange={(event) => { setEditorSummary(event.target.value); markEditorDirty() }} maxLength={1200} disabled={editorLocked} placeholder="写下这一章要改变什么，Agent 会将它作为写作锚点。" rows={2} /></div>
                    <div className="editor-area">
                      <div className="line-numbers" aria-hidden="true">{Array.from({ length: lineCount }, (_, index) => <span key={index}>{index + 1}</span>)}</div>
                      <textarea ref={editorRef} className="chapter-editor" value={editorContent} onChange={(event) => updateEditor(event.target.value)} onSelect={captureEditorSelection} onKeyDown={handleEditorKeyDown} disabled={editorLocked} spellCheck={false} placeholder={chapterBodyLoading ? '正在读取章节正文…' : '从这里开始写下你的故事……'} aria-label="章节正文" />
                    </div>
                  </div>
                  <div className="editor-footer"><span><Zap size={12} /> 编辑器已就绪</span><span>Markdown 支持</span><span>⌘ S 保存</span></div>
                </>
              ) : <EmptyEditor onCreate={createChapter} disabled={!project || projectActionBusy} />}
            </section>

            {!assistantCollapsed && <aside className="assistant-pane">
              <div className="assistant-header"><div className="assistant-title"><div className="agent-avatar"><Bot size={16} /></div><div><strong>写作 Agent</strong><span role="status" aria-live="polite" aria-atomic="true"><span className={`status-dot ${sending ? 'amber' : 'green'}`} /> {cancellingRequest ? '正在确认取消…' : sending ? '正在请求模型…' : assistantProviderLabel}</span></div></div>{sending && <button className="subtle-icon" onClick={cancelAgentRequest} title={cancellingRequest ? '正在确认取消' : '取消当前请求'} aria-label={cancellingRequest ? '正在确认取消 Agent 请求' : '取消当前 Agent 请求'} disabled={cancellingRequest}><X size={14} /></button>}<button className="subtle-icon" onClick={collapseAssistantPanel} title="收起 Agent" aria-label="收起 Agent 面板"><PanelRightClose size={15} /></button></div>
              <div className="assistant-tabs" role="tablist" aria-label="写作 Agent 面板">
                <button id="assistant-tab-chat" role="tab" aria-label={chatUnread ? '对话（有未读回复）' : '对话'} aria-selected={assistantTab === 'chat'} aria-controls="assistant-panel-chat" tabIndex={assistantTab === 'chat' ? 0 : -1} className={assistantTab === 'chat' ? 'active' : ''} onClick={() => activateAssistantTab('chat')} onKeyDown={(event) => handleAssistantTabKeyDown(event, 'chat')}><MessageSquare size={13} /> 对话{chatUnread && <span className="assistant-unread-dot" aria-hidden="true" />}</button>
                <button id="assistant-tab-outline" role="tab" aria-selected={assistantTab === 'outline'} aria-controls="assistant-panel-outline" tabIndex={assistantTab === 'outline' ? 0 : -1} className={assistantTab === 'outline' ? 'active' : ''} onClick={() => activateAssistantTab('outline')} onKeyDown={(event) => handleAssistantTabKeyDown(event, 'outline')}><ListTodo size={13} /> 大纲</button>
                <button id="assistant-tab-context" role="tab" aria-selected={assistantTab === 'context'} aria-controls="assistant-panel-context" tabIndex={assistantTab === 'context' ? 0 : -1} className={assistantTab === 'context' ? 'active' : ''} onClick={() => activateAssistantTab('context')} onKeyDown={(event) => handleAssistantTabKeyDown(event, 'context')}><BrainCircuit size={13} /> 上下文</button>
              </div>
              <ChatPanel chat={chat} input={chatInput} setInput={updateChatInput} send={sendMessage} cancel={cancelAgentRequest} retryPrompt={chatRetry?.safeToRetry && !chatRetry.needsReconcile ? chatRetry.message : undefined} retryNeedsReconcile={Boolean(chatRetry?.needsReconcile)} retryNeedsReview={Boolean(chatRetry?.requiresReview && !chatRetry.needsReconcile)} checkChatRecovery={checkChatRecovery} checkingChatRecovery={checkingChatRecovery} retryAfterReview={retryAfterReview} sending={sending} cancelling={cancellingRequest} hasUnread={chatUnread} onJumpToLatest={jumpToLatestChat} onChatScroll={handleChatScroll} chatScrollRef={chatScrollRef} locked={sendLocked || chapterBodyLoading} settingsUnsaved={settingsDirty} hasChapter={Boolean(currentChapter)} suggestions={suggestions} mode={writingMode} setMode={changeWritingMode} chatEndRef={chatEndRef} hidden={assistantTab !== 'chat'} selectionCharacterCount={selectedEditorCharacterCount} selectionHasText={activeEditorSelectionHasText} clearSelection={() => setEditorSelection(null)} applyTarget={chatApplyTarget} onPreviewRewrite={() => setRewritePreviewOpen(true)} />
              <OutlinePanel chapters={chapters} activeChapterId={activeChapterId} setActiveChapterId={(id) => void selectChapter(id)} onCreate={createChapter} busy={projectActionBusy} hidden={assistantTab !== 'outline'} />
              <ContextPanel characters={characters} notes={notes} project={project} projectBusy={projectActionBusy} onEditProject={openProjectEdit} onEditCharacter={(character) => { openCharactersView(); openCharacterEdit(character) }} onEditNote={(note) => { openNotesView(); openNoteEdit(note) }} hidden={assistantTab !== 'context'} />
            </aside>}
          </div>
          <footer className="statusbar"><div className="status-left"><span><GitBranchIcon /> main</span><span><CircleStatus /> 单用户模式</span><span className="status-mobile-hide">{project?.genre ?? '创作工作区'}</span></div><div className="status-right"><span>{stats ? `${formatNumber(stats.word_count)} / ${formatNumber(stats.word_target)} 字` : '—'}</span><span className="status-progress"><span style={{ width: `${Math.min(100, (stats?.completion_rate ?? 0) * 100)}%` }} /></span><span className="status-mobile-hide">UTF-8</span><span>Go + React</span></div></footer>
          </>
          )}
        </main>
      </div>
      {error && data && <div className="toast error-toast" role="alert" aria-atomic="true"><span>{error}</span><button onClick={() => setError('')} aria-label="关闭错误提示"><X size={13} /></button></div>}
      {rewritePreviewOpen && chatApplyTarget && (
        <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) setRewritePreviewOpen(false) }}>
          <section className="project-dialog rewrite-preview-dialog" role="dialog" aria-modal="true" aria-labelledby="rewrite-preview-title" aria-describedby="rewrite-preview-description">
            <header><div><span className="dialog-kicker">REWRITE PREVIEW</span><h2 id="rewrite-preview-title">检查改写内容</h2></div><button className="subtle-icon" onClick={() => setRewritePreviewOpen(false)} aria-label="关闭改写预览"><X size={16} /></button></header>
            <p className="rewrite-preview-description" id="rewrite-preview-description">确认或编辑 Agent 生成文本；应用后只替换当前选区。</p>
            <div className="rewrite-preview-original"><span>原文选区</span><pre>{chatApplyTarget.original}</pre></div>
            <label>替换正文<textarea autoFocus rows={9} value={chatApplyTarget.replacement} onChange={(event) => setChatApplyTarget((current) => current ? { ...current, replacement: normalizeEditorLineEndings(event.target.value) } : current)} /></label>
            <div className="dialog-actions"><button className="toolbar-button" onClick={() => setRewritePreviewOpen(false)}>返回对话</button><button className="primary-button" onClick={confirmRewritePreview} disabled={sendLocked || sending}><Check size={14} /> 应用到正文</button></div>
          </section>
        </div>
      )}
      {revisionDialogOpen && currentChapter && data && (
        <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !chapterActionBusy) setRevisionDialogOpen(false) }}>
          <section className="project-dialog chapter-revisions-dialog" role="dialog" aria-modal="true" aria-labelledby="chapter-revisions-title" aria-describedby="chapter-revisions-description">
            <header><div><span className="dialog-kicker">CHAPTER HISTORY</span><h2 id="chapter-revisions-title">章节版本</h2></div><button className="subtle-icon" onClick={() => setRevisionDialogOpen(false)} aria-label="关闭章节版本" disabled={chapterActionBusy}><X size={16} /></button></header>
            <p className="revision-dialog-description" id="chapter-revisions-description">为当前章节保存检查点。恢复旧版本时，当前内容会自动保存为一个可恢复的版本。</p>
            {revisionError && <p className="dialog-error" role="alert">{revisionError}</p>}
            <div className="revision-save-row"><label>版本名称<input value={revisionLabel} onChange={(event) => setRevisionLabel(event.target.value)} maxLength={80} disabled={chapterActionBusy} /></label><button className="primary-button" onClick={() => void saveChapterRevision()} disabled={chapterActionBusy || sending}><Save size={14} /> 保存当前版本</button></div>
            <div className="revision-columns">
              <div className="revision-list" aria-label="版本列表">
                {revisionListLoading && <div className="revision-empty">正在读取版本…</div>}
                {!revisionListLoading && chapterRevisions.length === 0 && <div className="revision-empty">还没有保存的版本。保存后，可以比较和恢复章节正文、标题与目标。</div>}
                {!revisionListLoading && chapterRevisions.map((revision) => <button className={`revision-item ${selectedRevision?.id === revision.id ? 'selected' : ''}`} key={revision.id} onClick={() => void loadChapterRevision(revision, data.workspace.workspace_id, currentChapter.id)} aria-pressed={selectedRevision?.id === revision.id} disabled={chapterActionBusy || revisionPreviewLoading && selectedRevision?.id === revision.id}>
                  <span className="revision-item-top"><strong>{revision.label}</strong><small>{formatTimestamp(revision.created_at)}</small></span><span className="revision-item-title">{revision.title || '未命名章节'}</span><span className="revision-item-meta">{statusLabel(revision.status)} · {formatNumber(revision.word_count)} 字</span>{revision.summary && <span className="revision-item-summary">{revision.summary}</span>}
                </button>)}
              </div>
              <div className="revision-preview">
                {revisionPreviewLoading && <div className="revision-empty">正在读取版本正文…</div>}
                {!revisionPreviewLoading && !selectedRevision && <div className="revision-empty">选择左侧版本查看内容。</div>}
                {!revisionPreviewLoading && selectedRevision && revisionCompareOpen && revisionCurrentSnapshot && <>
                  <div className="revision-compare-toolbar">
                    <span>对照保存版本与打开此窗口时的当前稿</span>
                    <button className="toolbar-button" onClick={() => setRevisionCompareOpen(false)}>返回版本预览</button>
                  </div>
                  <div className="revision-compare-columns">
                    <article className="revision-compare-pane">
                      <div className="revision-compare-heading"><span>保存版本</span><strong>{selectedRevision.title || '未命名章节'}</strong><small>{formatTimestamp(selectedRevision.created_at)} · {formatNumber(selectedRevision.word_count)} 字 · {statusLabel(selectedRevision.status)}</small>{selectedRevision.summary && <p>{selectedRevision.summary}</p>}</div>
                      <pre>{sliceUnicodeHead(selectedRevision.content, 100000) || '（此版本正文为空）'}</pre>
                      {countUnicodeCharacters(selectedRevision.content) > 100000 && <small className="revision-truncated">对照仅显示前 100,000 个字符。</small>}
                    </article>
                    <article className="revision-compare-pane">
                      <div className="revision-compare-heading"><span>当前稿</span><strong>{revisionCurrentSnapshot.title}</strong><small>{formatNumber(revisionCurrentSnapshot.word_count)} 字 · {statusLabel(revisionCurrentSnapshot.status)}</small>{revisionCurrentSnapshot.summary && <p>{revisionCurrentSnapshot.summary}</p>}</div>
                      <pre>{sliceUnicodeHead(revisionCurrentSnapshot.content ?? '', 100000) || '（当前稿正文为空）'}</pre>
                      {countUnicodeCharacters(revisionCurrentSnapshot.content ?? '') > 100000 && <small className="revision-truncated">对照仅显示前 100,000 个字符。</small>}
                    </article>
                  </div>
                  <button className="primary-button revision-restore-button" onClick={() => void restoreChapterRevision()} disabled={chapterActionBusy || sending}><RotateCcw size={14} /> 恢复此版本</button>
                </>}
                {!revisionPreviewLoading && selectedRevision && !revisionCompareOpen && <>
                  <div className="revision-preview-heading"><div><strong>{selectedRevision.label}</strong><small>{formatTimestamp(selectedRevision.created_at)} · {formatNumber(selectedRevision.word_count)} 字</small></div><span className={`chapter-status-text ${statusClass(selectedRevision.status)}`}>{statusLabel(selectedRevision.status)}</span></div>
                  <h3>{selectedRevision.title}</h3>
                  {selectedRevision.summary && <p className="revision-preview-summary">{selectedRevision.summary}</p>}
                  <pre>{sliceUnicodeHead(selectedRevision.content, 2400) || '（此版本正文为空）'}</pre>
                  {countUnicodeCharacters(selectedRevision.content) > 2400 && <small className="revision-truncated">预览已截取前 2,400 个字符。</small>}
                  <div className="revision-preview-actions"><button className="toolbar-button" onClick={() => setRevisionCompareOpen(true)} disabled={!revisionCurrentSnapshot}><History size={13} /> 与当前稿对照</button><button className="primary-button" onClick={() => void restoreChapterRevision()} disabled={chapterActionBusy || sending}><RotateCcw size={14} /> 恢复此版本</button></div>
                </>}
              </div>
            </div>
            <div className="dialog-actions"><button className="toolbar-button" onClick={() => setRevisionDialogOpen(false)} disabled={chapterActionBusy}>关闭</button></div>
          </section>
        </div>
      )}
      {projectDeleteDialogOpen && project && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !deletingProject) setProjectDeleteDialogOpen(false) }}><section className="project-dialog delete-project-dialog" role="dialog" aria-modal="true" aria-labelledby="project-delete-title" aria-describedby="project-delete-warning"><header><div><span className="dialog-kicker">DELETE WORKSPACE</span><h2 id="project-delete-title">删除小说项目</h2></div><button className="subtle-icon" onClick={() => setProjectDeleteDialogOpen(false)} aria-label="关闭删除确认" disabled={deletingProject}><X size={16} /></button></header><p className="delete-project-warning" id="project-delete-warning">项目 <strong>{project.title}</strong> 的章节、角色、便笺、聊天记录和模型设置都会被永久删除，当前未保存的编辑也会丢失。此操作无法撤销。</p><label>输入项目名称以确认<input autoFocus value={projectDeleteInput} onChange={(event) => setProjectDeleteInput(event.target.value)} onKeyDown={(event) => { if (!isIMEComposing(event.nativeEvent) && event.key === 'Enter' && projectDeleteInput === project.title && !projectActionBusy) void confirmProjectDelete() }} maxLength={80} /></label>{projectDeleteError && <p className="dialog-error" role="alert">{projectDeleteError}</p>}<div className="dialog-actions"><button className="toolbar-button" onClick={() => setProjectDeleteDialogOpen(false)} disabled={deletingProject}>取消</button><button className="danger-button" onClick={() => void confirmProjectDelete()} disabled={projectDeleteInput !== project.title || projectActionBusy || projects.length <= 1}>{deletingProject ? '正在删除…' : '永久删除项目'}</button></div></section></div>}
      {searchOpen && <div className="modal-backdrop search-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) closeSearch() }}><section className="command-palette" role="dialog" aria-modal="true" aria-labelledby="command-palette-title"><div className="command-palette-input-row"><Search size={17} /><input ref={searchInputRef} autoFocus role="combobox" aria-expanded="true" aria-controls="command-palette-results" aria-activedescendant={searchResults[activeSearchIndex] ? `palette-${searchResults[activeSearchIndex].id}` : undefined} aria-autocomplete="list" aria-label="搜索当前项目" placeholder="搜索章节、角色、便笺或命令…" value={searchQuery} onChange={(event) => { setSearchQuery(event.target.value); setSelectedSearchIndex(0) }} onKeyDown={(event) => {
        if (isIMEComposing(event.nativeEvent)) return
        if (event.key === 'ArrowDown' && searchResults.length > 0) { event.preventDefault(); setSelectedSearchIndex((index) => Math.min(index + 1, searchResults.length - 1)) }
        else if (event.key === 'ArrowUp' && searchResults.length > 0) { event.preventDefault(); setSelectedSearchIndex((index) => Math.max(index - 1, 0)) }
        else if (event.key === 'Enter' && searchResults[activeSearchIndex] && !searchResults[activeSearchIndex].disabled) { event.preventDefault(); activateSearchResult(searchResults[activeSearchIndex]) }
        else if (event.key === 'Escape') { event.preventDefault(); closeSearch() }
      }} /><kbd>ESC</kbd><button className="subtle-icon" onClick={closeSearch} aria-label="关闭搜索"><X size={15} /></button></div><h2 id="command-palette-title" className="sr-only">搜索当前项目</h2><div className="command-palette-results" id="command-palette-results" role="listbox" aria-label="搜索结果">{searchResults.map((result, index) => {
        const ResultIcon = result.kind === 'chapter' ? FileText : result.kind === 'character' ? Users : result.kind === 'note' ? Lightbulb : Command
        const kindLabel = result.kind === 'chapter' ? '章节' : result.kind === 'character' ? '角色' : result.kind === 'note' ? '便笺' : '命令'
        return <button key={result.id} id={`palette-${result.id}`} ref={index === activeSearchIndex ? searchActiveResultRef : undefined} className={`palette-result ${index === activeSearchIndex ? 'active' : ''}`} role="option" aria-selected={index === activeSearchIndex} disabled={result.disabled} onMouseEnter={() => setSelectedSearchIndex(index)} onClick={() => activateSearchResult(result)}><span className={`palette-result-icon ${result.kind}`}><ResultIcon size={14} /></span><span className="palette-result-copy"><strong>{result.title}</strong><small>{result.detail}</small></span><span className="palette-result-kind">{kindLabel}</span></button>
      })}{searchResults.length === 0 && <div className="palette-empty"><Search size={16} /><span>{searchQuery.trim() ? '没有找到匹配内容' : '当前项目还没有可搜索的内容'}</span></div>}</div><footer className="palette-footer"><span><kbd>↑</kbd><kbd>↓</kbd> 选择</span><span><kbd>Enter</kbd> 打开</span><span><kbd>Esc</kbd> 关闭</span></footer></section></div>}
      {helpDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) setHelpDialogOpen(false) }}><section className="project-dialog help-dialog" role="dialog" aria-modal="true" aria-labelledby="help-dialog-title"><header><div><span className="dialog-kicker">QUICK HELP</span><h2 id="help-dialog-title">帮助与快捷键</h2></div><button className="subtle-icon" onClick={() => setHelpDialogOpen(false)} aria-label="关闭帮助"><X size={16} /></button></header><p className="help-intro">使用快捷键更快地搜索项目、保存章节和操作写作 Agent。</p><div className="help-shortcuts"><div><kbd>Ctrl / ⌘ + K</kbd><span>搜索章节、角色、便笺和命令</span></div><div><kbd>Ctrl / ⌘ + S</kbd><span>保存当前章节（编辑器中）</span></div><div><kbd>Tab / Shift + Tab</kbd><span>选中行可缩进/反向缩进；无选区时 Tab 插入空格，Shift + Tab 减少当前行缩进</span></div><div><kbd>↑ / ↓</kbd><span>在搜索结果中移动</span></div><div><kbd>Enter</kbd><span>打开搜索结果或发送 Agent 消息</span></div><div><kbd>Shift + Enter</kbd><span>在 Agent 输入框中换行</span></div><div><kbd>Esc</kbd><span>关闭搜索/帮助；没有面板打开时取消 Agent 请求</span></div></div><div className="dialog-actions"><button className="primary-button" onClick={() => setHelpDialogOpen(false)}>完成</button></div></section></div>}
      {projectEditDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !savingProject) setProjectEditDialogOpen(false) }}><section className="project-dialog" role="dialog" aria-modal="true" aria-labelledby="project-edit-title"><header><div><span className="dialog-kicker">PROJECT DETAILS</span><h2 id="project-edit-title">编辑项目资料</h2></div><button className="subtle-icon" onClick={() => setProjectEditDialogOpen(false)} aria-label="关闭" disabled={savingProject}><X size={16} /></button></header><label>项目名称<input autoFocus value={projectDraft.title} onChange={(event) => setProjectDraft({ ...projectDraft, title: event.target.value })} maxLength={80} disabled={savingProject} /></label><label>副标题<input value={projectDraft.subtitle} onChange={(event) => setProjectDraft({ ...projectDraft, subtitle: event.target.value })} placeholder="可选" maxLength={120} disabled={savingProject} /></label><label>题材<input value={projectDraft.genre} onChange={(event) => setProjectDraft({ ...projectDraft, genre: event.target.value })} placeholder="例如：近未来 · 悬疑" maxLength={80} disabled={savingProject} /></label><label>故事设定<textarea value={projectDraft.premise} onChange={(event) => setProjectDraft({ ...projectDraft, premise: event.target.value })} rows={3} maxLength={500} disabled={savingProject} /></label><label>目标字数<input type="number" min={1} step={1} value={projectDraft.word_target} onChange={(event) => setProjectDraft({ ...projectDraft, word_target: Math.trunc(Number(event.target.value) || 0) })} disabled={savingProject} /></label>{projectEditError && <p className="dialog-error" role="alert">{projectEditError}</p>}<div className="dialog-actions"><button className="toolbar-button" onClick={() => setProjectEditDialogOpen(false)} disabled={savingProject}>取消</button><button className="primary-button" onClick={() => void saveProject()} disabled={!projectDraft.title.trim() || projectDraft.word_target < 1 || savingProject || sending || savingContextItem || savingSettings || chapterActionBusy || saveState === 'saving'}>{savingProject ? '正在保存…' : <><Check size={14} /> 保存资料</>}</button></div></section></div>}
      {projectDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !creatingProject) setProjectDialogOpen(false) }}><section className="project-dialog" role="dialog" aria-modal="true" aria-labelledby="project-dialog-title"><header><div><span className="dialog-kicker">NEW WORKSPACE</span><h2 id="project-dialog-title">创建小说项目</h2></div><button className="subtle-icon" onClick={() => setProjectDialogOpen(false)} aria-label="关闭" disabled={creatingProject}><X size={16} /></button></header><label>项目名称<input autoFocus value={projectTitle} onChange={(event) => setProjectTitle(event.target.value)} placeholder="例如：雾港来信" maxLength={80} disabled={creatingProject} /></label><label>题材<input value={projectGenre} onChange={(event) => setProjectGenre(event.target.value)} placeholder="近未来 · 悬疑" maxLength={80} disabled={creatingProject} /></label><label>故事设定<textarea value={projectPremise} onChange={(event) => setProjectPremise(event.target.value)} placeholder="用一两句话描述故事的核心设定。" rows={3} maxLength={500} disabled={creatingProject} /></label><div className="dialog-actions"><button className="toolbar-button" onClick={() => setProjectDialogOpen(false)} disabled={creatingProject}>取消</button><button className="primary-button" onClick={() => void createProject()} disabled={!projectTitle.trim() || creatingProject || sending || saveState === 'saving'}>{creatingProject ? '正在创建…' : <><Plus size={14} /> 创建项目</>}</button></div></section></div>}
      {characterDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !savingContextItem) setCharacterDialogOpen(false) }}><section className="project-dialog" role="dialog" aria-modal="true" aria-labelledby="character-dialog-title"><header><div><span className="dialog-kicker">PROJECT CHARACTER</span><h2 id="character-dialog-title">{characterEditingId === null ? '新建角色' : '编辑角色'}</h2></div><button className="subtle-icon" onClick={() => setCharacterDialogOpen(false)} aria-label="关闭" disabled={savingContextItem}><X size={16} /></button></header><label>角色名称<input autoFocus value={characterDraft.name} onChange={(event) => setCharacterDraft({ ...characterDraft, name: event.target.value })} placeholder="例如：林澈" maxLength={80} disabled={savingContextItem} /></label><label>身份<input value={characterDraft.role} onChange={(event) => setCharacterDraft({ ...characterDraft, role: event.target.value })} placeholder="主角、导师、对手……" maxLength={100} disabled={savingContextItem} /></label><label>角色描述<textarea value={characterDraft.description} onChange={(event) => setCharacterDraft({ ...characterDraft, description: event.target.value })} placeholder="角色的背景、目标或关键矛盾。" rows={3} maxLength={1000} disabled={savingContextItem} /></label><label>人物特质<input value={characterDraft.traits} onChange={(event) => setCharacterDraft({ ...characterDraft, traits: event.target.value })} placeholder="敏锐、克制、怕水（用逗号分隔）" maxLength={240} disabled={savingContextItem} /></label><label>角色颜色<input className="character-color-input" type="color" value={characterDraft.color || characterColors[0]} onChange={(event) => setCharacterDraft({ ...characterDraft, color: event.target.value })} disabled={savingContextItem} /></label>{contextDialogError && <p className="dialog-error" role="alert">{contextDialogError}</p>}<div className="dialog-actions"><button className="toolbar-button" onClick={() => setCharacterDialogOpen(false)} disabled={savingContextItem}>取消</button><button className="primary-button" onClick={() => void saveCharacter()} disabled={!characterDraft.name.trim() || projectActionBusy}>{savingContextItem ? '正在保存…' : <><Check size={14} /> {characterEditingId === null ? '添加角色' : '保存修改'}</>}</button></div></section></div>}
      {noteDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !savingContextItem) setNoteDialogOpen(false) }}><section className="project-dialog" role="dialog" aria-modal="true" aria-labelledby="note-dialog-title"><header><div><span className="dialog-kicker">PROJECT NOTE</span><h2 id="note-dialog-title">{noteEditingId === null ? '新建便笺' : '编辑便笺'}</h2></div><button className="subtle-icon" onClick={() => setNoteDialogOpen(false)} aria-label="关闭" disabled={savingContextItem}><X size={16} /></button></header><label>标题<input autoFocus value={noteDraft.title} onChange={(event) => setNoteDraft({ ...noteDraft, title: event.target.value })} placeholder="一句话概括这个想法" maxLength={120} disabled={savingContextItem} /></label><label>分类<select value={noteDraft.kind} onChange={(event) => setNoteDraft({ ...noteDraft, kind: event.target.value })} disabled={savingContextItem}>{!["idea", "mystery", "craft", "world"].includes(noteDraft.kind) && <option value={noteDraft.kind}>保留原分类：{noteDraft.kind}</option>}<option value="idea">灵感</option><option value="mystery">悬念</option><option value="craft">写作提醒</option><option value="world">世界观</option></select></label><label>内容<textarea value={noteDraft.body} onChange={(event) => setNoteDraft({ ...noteDraft, body: event.target.value })} placeholder="记录线索、设定或之后要处理的想法。" rows={4} maxLength={2000} disabled={savingContextItem} /></label>{contextDialogError && <p className="dialog-error" role="alert">{contextDialogError}</p>}<div className="dialog-actions"><button className="toolbar-button" onClick={() => setNoteDialogOpen(false)} disabled={savingContextItem}>取消</button><button className="primary-button" onClick={() => void saveNote()} disabled={!noteDraft.title.trim() || projectActionBusy}>{savingContextItem ? '正在保存…' : <><Check size={14} /> {noteEditingId === null ? '添加便笺' : '保存修改'}</>}</button></div></section></div>}
    </div>
  )
}

function ActivityButton({ icon, active, expanded, label, badge, unread, buttonRef, onClick }: { icon: React.ReactNode; active: boolean; expanded?: boolean; label: string; badge?: number; unread?: boolean; buttonRef?: React.Ref<HTMLButtonElement>; onClick: () => void }) {
  const accessibleLabel = badge === undefined ? (unread ? `${label}（有未读 Agent 回复）` : label) : `${label}（${badge}）${unread ? '，有未读 Agent 回复' : ''}`
  return <button ref={buttonRef} className={`activity-button ${active ? 'active' : ''}`} onClick={onClick} title={accessibleLabel} aria-label={accessibleLabel} aria-pressed={expanded === undefined ? active : undefined} aria-expanded={expanded}>{icon}{unread && <span className="activity-unread-dot" aria-hidden="true" />}{badge !== undefined && <span className="activity-badge">{badge}</span>}</button>
}

function ExplorerView({ project, chapters, activeChapterId, setActiveChapterId, onCreate, onExport, onExportDocx, onBackup, onRestore, onEditProject, onDeleteProject, canDeleteProject, projectBusy, onOpenWorld, onOpenCharacters, onOpenNotes, onMove, onDelete, chapterBusy, activeActivity }: { project?: WorkspaceResponse['workspace']['project']; chapters: Chapter[]; activeChapterId: number | null; setActiveChapterId: (id: number) => void; onCreate: () => void; onExport: () => void; onExportDocx: () => void; onBackup: () => void; onRestore: () => void; onEditProject: () => void; onDeleteProject: () => void; canDeleteProject: boolean; projectBusy: boolean; onOpenWorld: () => void; onOpenCharacters: () => void; onOpenNotes: () => void; onMove: (chapterID: number, direction: -1 | 1) => void; onDelete: (chapter: Chapter) => void; chapterBusy: boolean; activeActivity: ActivityId }) {
  return <div className="explorer-body">
    <div className="project-summary"><div className="project-cover"><BookOpen size={18} /></div><div className="project-summary-copy"><strong>{project?.title ?? '加载中…'}</strong><span>{project?.genre ?? '小说项目'}</span></div><button className="subtle-icon" onClick={onEditProject} title="编辑项目资料" aria-label="编辑项目资料" disabled={!project || projectBusy}><Pencil size={14} /></button><button className="subtle-icon danger" onClick={onDeleteProject} title={canDeleteProject ? '删除当前项目' : '至少保留一个项目'} aria-label={`删除项目 ${project?.title ?? ''}`} disabled={!project || !canDeleteProject || projectBusy}><Trash2 size={14} /></button></div>
    <div className="tree-toolbar"><span>项目文件</span><div><button className="subtle-icon" onClick={onCreate} title="新建章节" aria-label="新建章节" disabled={chapterBusy}><FilePlus2 size={14} /></button><button className="subtle-icon" onClick={onExport} title="导出 Markdown" aria-label={`导出${project?.title ?? '当前项目'}为 Markdown`} disabled={!project || projectBusy}><ArrowDownToLine size={14} /></button><button className="subtle-icon" onClick={onExportDocx} title="导出 Word 文档（DOCX）" aria-label={`导出${project?.title ?? '当前项目'}为 Word 文档`} disabled={!project || projectBusy}><FileText size={14} /></button><button className="subtle-icon" onClick={onBackup} title="导出完整项目备份" aria-label={`导出${project?.title ?? '当前项目'}的完整备份`} disabled={!project || projectBusy}><Archive size={14} /></button><button className="subtle-icon" onClick={onRestore} title="导入项目备份" aria-label="导入项目备份" disabled={!project || projectBusy}><Upload size={14} /></button></div></div>
    <div className="file-tree">
      <div className="tree-folder open"><ChevronDown size={13} /><FolderOpen size={15} className="folder-icon" /><span>{project?.title ?? '未命名项目'}</span></div>
      <div className="tree-folder open nested"><ChevronDown size={13} /><FolderOpen size={15} className="folder-icon" /><span>章节</span><span className="tree-count">{chapters.length}</span></div>
      {chapters.map((chapter, index) => <div key={chapter.id} className={`tree-file nested-2 chapter-tree-row ${activeChapterId === chapter.id ? 'active' : ''}`}>
        <button className="chapter-tree-select" onClick={() => setActiveChapterId(chapter.id)} disabled={chapterBusy}>
          <FileText size={14} className="file-icon" /><span className="tree-chapter-number">{String(chapter.number).padStart(2, '0')}</span><span className="tree-file-name">{chapter.title}</span><span className={`tree-status ${statusClass(chapter.status)}`} />
        </button>
        <div className="chapter-tree-actions">
          <button className="chapter-action-button" onClick={() => onMove(chapter.id, -1)} title="上移章节" aria-label={`上移第 ${chapter.number} 章`} disabled={chapterBusy || index === 0}><ChevronUp size={12} /></button>
          <button className="chapter-action-button" onClick={() => onMove(chapter.id, 1)} title="下移章节" aria-label={`下移第 ${chapter.number} 章`} disabled={chapterBusy || index === chapters.length - 1}><ChevronDown size={12} /></button>
          <button className="chapter-action-button danger" onClick={() => onDelete(chapter)} title="删除章节" aria-label={`删除章节 ${chapter.title}`} disabled={chapterBusy}><Trash2 size={12} /></button>
        </div>
      </div>)}
      <button className="tree-file nested" onClick={onOpenWorld} disabled={!project || projectBusy} aria-label="查看世界观设定"><Archive size={14} className="file-icon muted-icon" /><span className="tree-file-name muted-text">世界观设定</span></button>
      <button className="tree-file nested" onClick={onOpenCharacters} disabled={!project || projectBusy}><Users size={14} className="file-icon muted-icon" /><span className="tree-file-name muted-text">角色档案</span></button>
      <button className="tree-file nested" onClick={onOpenNotes} disabled={!project || projectBusy}><Lightbulb size={14} className="file-icon muted-icon" /><span className="tree-file-name muted-text">灵感便笺</span></button>
    </div>
    {activeActivity === 'chapters' && <div className="sidebar-helper"><div className="helper-icon"><ListTodo size={15} /></div><div><strong>章节视图</strong><p>使用章节右侧的箭头调整顺序。</p></div></div>}
  </div>
}

function ChatPanel({ chat, input, setInput, send, cancel, retryPrompt, retryNeedsReconcile, retryNeedsReview, checkChatRecovery, checkingChatRecovery, retryAfterReview, sending, cancelling, hasUnread, onJumpToLatest, onChatScroll, chatScrollRef, locked, settingsUnsaved, hasChapter, suggestions, mode, setMode, chatEndRef, hidden, selectionCharacterCount, selectionHasText, clearSelection, applyTarget, onPreviewRewrite }: { chat: ChatMessage[]; input: string; setInput: (value: string) => void; send: (preset?: string) => void; cancel: () => void; retryPrompt?: string; retryNeedsReconcile: boolean; retryNeedsReview: boolean; checkChatRecovery: () => void; checkingChatRecovery: boolean; retryAfterReview: () => void; sending: boolean; cancelling: boolean; hasUnread: boolean; onJumpToLatest: () => void; onChatScroll: () => void; chatScrollRef: React.RefObject<HTMLDivElement | null>; locked: boolean; settingsUnsaved: boolean; hasChapter: boolean; suggestions: string[]; mode: WritingMode; setMode: (mode: WritingMode) => void; chatEndRef: React.RefObject<HTMLDivElement | null>; hidden: boolean; selectionCharacterCount: number; selectionHasText: boolean; clearSelection: () => void; applyTarget: ChatApplyTarget | null; onPreviewRewrite: (target: ChatApplyTarget) => void }) {
  const inputCharacterCount = countUnicodeCharacters(input.trim())
  const editSelectionInvalid = mode === 'edit' && (selectionCharacterCount === 0 || !selectionHasText || selectionCharacterCount > maxAgentSelectionCharacters)
  const retryBlocked = retryNeedsReconcile || retryNeedsReview
  return (
    <div className="chat-panel" role="tabpanel" id="assistant-panel-chat" aria-labelledby="assistant-tab-chat" tabIndex={0} hidden={hidden} style={hidden ? { display: 'none' } : undefined}>
      <div ref={chatScrollRef} className="chat-scroll" role="log" aria-label="Agent 对话记录" aria-live="polite" aria-relevant="additions text" aria-atomic="false" onScroll={onChatScroll}>
        {chat.length === 0 && <div className="agent-welcome"><div className="welcome-orbit"><Sparkles size={19} /></div><h3>一起把故事写下去</h3><p>{hasChapter ? '我会记住当前章节、角色和项目设定。试着告诉我你想推进的一个小目标。' : '项目还没有章节。你可以先和我讨论设定、角色或整体大纲。'}</p><div className="welcome-chips"><button onClick={() => send(hasChapter ? '帮我为本章设计三个节拍' : '帮我为这个项目设计一个开篇')} disabled={locked || sending || settingsUnsaved || editSelectionInvalid || retryBlocked}><Target size={12} /> {hasChapter ? '设计本章节拍' : '规划故事开篇'}</button><button onClick={() => send(hasChapter ? '检查当前章节的悬念是否足够' : '检查项目的核心悬念是否足够')} disabled={locked || sending || settingsUnsaved || editSelectionInvalid || retryBlocked}><Search size={12} /> 检查悬念</button></div></div>}
        {chat.map((message, index) => <div className={`chat-message ${message.role}`} key={message.id !== undefined ? `id:${message.id}` : message.request_id ? `request:${message.request_id}` : `local:${index}`}><div className="message-avatar">{message.role === 'assistant' ? <Bot size={13} /> : message.role === 'system' ? <Command size={13} /> : 'W'}</div><div className="message-body"><div className="message-meta"><span>{message.role === 'assistant' ? '写作 Agent' : message.role === 'system' ? '系统' : '我'}</span>{message.role === 'assistant' && writingModeLabel(message.mode) && <span className="message-mode-tag">{writingModeLabel(message.mode)}</span>}<span className="message-time">{message.created_at ? relativeTime(message.created_at) : '刚刚'}</span></div><div className="message-content">{message.content.split('\n').map((line, lineIndex) => <p key={lineIndex}>{line || <>&nbsp;</>}</p>)}</div>{message.role === 'assistant' && applyTarget && applyTarget.messageId === message.id && <button type="button" className="apply-rewrite-button" onClick={() => onPreviewRewrite(applyTarget)} disabled={locked || sending} aria-label={applyTarget.replacement !== applyTarget.generatedReplacement ? '继续编辑改写' : '预览并编辑 Agent 改写'}><Check size={12} /> {applyTarget.replacement !== applyTarget.generatedReplacement ? '继续编辑' : '检查并编辑'}</button>}</div></div>)}
        {sending && <div className="chat-message assistant"><div className="message-avatar"><Bot size={13} /></div><div className="message-body"><div className="message-meta"><span>写作 Agent</span><span>{cancelling ? '正在确认取消…' : '思考中'}</span></div>{cancelling ? <div role="status" className="selection-context empty">正在结束当前 Agent 请求，请稍候。</div> : <div className="typing"><i /><i /><i /></div>}</div></div>}
        <div ref={chatEndRef} />
      </div>
      {hasUnread && <button type="button" className="chat-new-messages" onClick={onJumpToLatest}>查看最新回复</button>}
      <div className="suggestions"><span>试试</span>{suggestions.slice(0, 3).map((suggestion) => <button key={suggestion} onClick={() => send(suggestion)} disabled={locked || sending || settingsUnsaved || editSelectionInvalid || retryBlocked}>{suggestion}</button>)}</div>
      {retryPrompt && <div className="selection-context empty" role="status"><span title={retryPrompt}>上次未保存的问题：{Array.from(retryPrompt).slice(0, 72).join('')}{Array.from(retryPrompt).length > 72 ? '…' : ''}</span><button type="button" onClick={() => send(retryPrompt === input.trim() ? undefined : retryPrompt)} disabled={locked || sending} aria-label="重试上次未保存的问题" title="重试上次未保存的问题">重试</button></div>}
      {retryNeedsReconcile && <div className="selection-context empty" role="status"><span>暂时无法确认上次请求是否已保存。先同步最新聊天记录，避免重复生成。</span><button type="button" onClick={checkChatRecovery} disabled={locked || sending || checkingChatRecovery} aria-label="检查最新聊天记录">{checkingChatRecovery ? '正在检查…' : '检查聊天记录'}</button></div>}
      {retryNeedsReview && <div className="selection-context empty" role="status"><span>这条请求无法与旧聊天记录安全匹配。请先检查上方记录，再决定是否以新请求 ID 重发。</span><button type="button" onClick={retryAfterReview} disabled={locked || sending || settingsUnsaved || inputCharacterCount > maxAgentInputCharacters || editSelectionInvalid} aria-label="已检查聊天记录，以新请求 ID 重发" title="以新请求 ID 重新发送">已检查，生成新请求 ID 重发</button></div>}
      <div className="composer">
        {settingsUnsaved && <div className="selection-context empty" role="status">先保存模型设置，再发送 Agent 请求。</div>}
        <div className="mode-switcher" role="group" aria-label="Agent 工作模式">{writingModes.map(({ id, label, icon: Icon }) => <button key={id} className={mode === id ? 'active' : ''} aria-pressed={mode === id} onClick={() => setMode(id)} disabled={locked}><Icon size={12} /> {label}</button>)}</div>
        {selectionCharacterCount > 0 ? <div className="selection-context" role="status"><span><FileText size={12} /> {mode === 'edit' && !selectionHasText && selectionCharacterCount <= maxAgentSelectionCharacters ? '改写选区（只有空白字符）' : mode === 'edit' ? '改写选区' : '选区上下文'} · {selectionCharacterCount > maxAgentSelectionCharacters ? '超过 ' + formatNumber(maxAgentSelectionCharacters) + ' 字' : formatNumber(selectionCharacterCount) + ' 字'}{selectionCharacterCount > maxAgentSelectionCharacters ? (mode === 'edit' ? '（请缩小到 1800 字以内才能改写）' : '（对话只带入前 1800 字）') : ''}</span><button type="button" onClick={clearSelection} aria-label="移除选中正文上下文"><X size={12} /></button></div> : mode === 'edit' ? <div className="selection-context empty" role="note">先在编辑器正文中选中需要改写的段落。</div> : null}
        <textarea value={input} onChange={(event) => setInput(event.target.value)} onKeyDown={(event) => { if (!isIMEComposing(event.nativeEvent) && event.key === 'Enter' && !event.shiftKey && !retryBlocked) { event.preventDefault(); send() } }} placeholder="告诉 Agent 你想写什么…" rows={3} disabled={locked || settingsUnsaved} />
        <div className="composer-footer"><span>Shift + Enter 换行 · {inputCharacterCount}/{maxAgentInputCharacters}</span>{sending ? <button className="send-button cancel-button" onClick={cancel} aria-label={cancelling ? "正在确认取消 Agent 请求" : "取消当前 Agent 请求"} title={cancelling ? "正在确认取消" : "取消当前请求"} disabled={cancelling}><X size={14} /></button> : <button className="send-button" onClick={() => send()} aria-label="发送消息" disabled={!input.trim() || inputCharacterCount > maxAgentInputCharacters || locked || settingsUnsaved || editSelectionInvalid || retryBlocked}><Send size={14} /></button>}</div>
      </div>
    </div>
  )
}

function OutlinePanel({ chapters, activeChapterId, setActiveChapterId, onCreate, busy, hidden }: { chapters: Chapter[]; activeChapterId: number | null; setActiveChapterId: (id: number) => void; onCreate: () => void; busy: boolean; hidden: boolean }) {
  return <div className="outline-panel" role="tabpanel" id="assistant-panel-outline" aria-labelledby="assistant-tab-outline" tabIndex={0} hidden={hidden} style={hidden ? { display: 'none' } : undefined}><div className="panel-intro"><div className="panel-intro-icon"><ListTodo size={16} /></div><div><strong>章节路线</strong><p>把每章的变化写清楚，正文会更轻松。</p></div></div><div className="outline-list">{chapters.map((chapter) => <button key={chapter.id} className={`outline-item ${activeChapterId === chapter.id ? 'active' : ''}`} onClick={() => setActiveChapterId(chapter.id)} disabled={busy}><span className={`outline-marker ${statusClass(chapter.status)}`} /> <span className="outline-number">{String(chapter.number).padStart(2, '0')}</span><span className="outline-title"><strong>{chapter.title}</strong><small>{chapter.summary || '还没有章节摘要'}</small></span><ChevronRight size={14} /></button>)}</div><button className="outline-add" onClick={onCreate} disabled={busy}><Plus size={14} /> 添加章节</button></div>
}

function ContextPanel({ characters, notes, project, projectBusy, onEditProject, onEditCharacter, onEditNote, hidden }: { characters: Character[]; notes: Note[]; project?: WorkspaceResponse['workspace']['project']; projectBusy: boolean; onEditProject: () => void; onEditCharacter: (character: Character) => void; onEditNote: (note: Note) => void; hidden: boolean }) {
  return <div className="context-panel" role="tabpanel" id="assistant-panel-context" aria-labelledby="assistant-tab-context" tabIndex={0} hidden={hidden} style={hidden ? { display: 'none' } : undefined}><div className="context-card"><div className="context-card-header"><span><Archive size={14} /> 项目设定</span><button className="subtle-icon" onClick={onEditProject} title="编辑项目资料" aria-label="编辑项目资料" disabled={!project || projectBusy}><Pencil size={13} /></button></div><p>{project?.premise ?? '加载项目设定…'}</p><div className="context-tags"><span>{project?.genre}</span><span>当前项目</span></div></div><div className="context-section"><div className="context-section-title"><span>角色</span><span>{characters.length}</span></div>{characters.map((character) => <button className="context-character" key={character.id} onClick={() => onEditCharacter(character)} aria-label={`编辑角色 ${character.name}`} disabled={projectBusy}><span className="character-dot" style={{ background: character.color }} /> <div><strong>{character.name}</strong><small>{character.role}</small></div></button>)}</div><div className="context-section"><div className="context-section-title"><span>已固定便笺</span><span>{notes.length}</span></div>{notes.map((note) => <button className="context-note" key={note.id} onClick={() => onEditNote(note)} aria-label={`编辑便笺 ${note.title}`} disabled={projectBusy}><Lightbulb size={13} /><span>{note.title}</span></button>)}</div><div className="context-footnote"><BrainCircuit size={13} /> 这些内容会随每次对话自动带入</div></div>
}

function CharactersView({ characters, busy, onCreate, onEdit, onDelete }: { characters: Character[]; busy: boolean; onCreate: () => void; onEdit: (character: Character) => void; onDelete: (character: Character) => void }) {
  return <div className="collection-view"><div className="collection-intro"><div className="collection-icon"><Users size={16} /></div><div><strong>角色档案</strong><p>让每个角色在长篇里保持自己的声音。</p></div></div>{characters.map((character) => <div className="character-card" key={character.id}><div className="character-avatar" style={{ background: `${character.color}22`, color: character.color }}>{character.name.slice(0, 1)}</div><div className="character-info"><strong>{character.name}</strong><span>{character.role}</span><p>{character.description}</p><div className="character-traits">{character.traits.map((trait) => <i key={trait}>{trait}</i>)}</div></div><div className="collection-actions"><button className="subtle-icon" onClick={() => onEdit(character)} aria-label={`编辑角色 ${character.name}`} title="编辑" disabled={busy}><Pencil size={13} /></button><button className="subtle-icon danger" onClick={() => onDelete(character)} aria-label={`删除角色 ${character.name}`} title="删除" disabled={busy}><Trash2 size={13} /></button></div></div>)}{characters.length === 0 && <div className="collection-empty">还没有角色档案，添加角色后 Agent 会把资料带入对话。</div>}<button className="dashed-add" onClick={onCreate} disabled={busy}><Plus size={14} /> 新建角色</button></div>
}

function NotesView({ notes, busy, onCreate, onEdit, onDelete }: { notes: Note[]; busy: boolean; onCreate: () => void; onEdit: (note: Note) => void; onDelete: (note: Note) => void }) {
  return <div className="collection-view"><div className="collection-intro"><div className="collection-icon warm"><Lightbulb size={16} /></div><div><strong>灵感便笺</strong><p>把还没成熟的想法先放在这里。</p></div></div>{notes.map((note) => <div className="note-card" key={note.id}><div className="note-card-heading"><Lightbulb size={13} /><strong>{note.title}</strong><span>{note.kind}</span><div className="collection-actions"><button className="subtle-icon" onClick={() => onEdit(note)} aria-label={`编辑便笺 ${note.title}`} title="编辑" disabled={busy}><Pencil size={13} /></button><button className="subtle-icon danger" onClick={() => onDelete(note)} aria-label={`删除便笺 ${note.title}`} title="删除" disabled={busy}><Trash2 size={13} /></button></div></div><p>{note.body}</p></div>)}{notes.length === 0 && <div className="collection-empty">还没有便笺，记录下线索、设定或之后要处理的想法。</div>}<button className="dashed-add" onClick={onCreate} disabled={busy}><Plus size={14} /> 新建便笺</button></div>
}

function SettingsView({ settings, setSettings, onSave, saved, saving, locked, dirty, error }: { settings: SettingsDraft | null; setSettings: (settings: SettingsDraft) => void; onSave: () => void; saved: boolean; saving: boolean; locked: boolean; dirty: boolean; error: string }) {
  if (!settings) return <div className="loading-side">加载设置…</div>
  const apiKeyPlaceholder = settings.api_key_configured
    ? '留空以保留已保存的密钥'
    : settings.provider === 'compatible'
      ? '按接口要求填写；本机免鉴权接口可留空'
      : settings.provider === 'openai'
        ? '此服务商需要 API Key，仅保存在本机'
        : '仅保存在本机'
  return (
    <div className="settings-view">
      {error && <p className="dialog-error settings-error" role="alert">{error}</p>}
      <div className="settings-intro"><div className="collection-icon"><Settings2 size={16} /></div><div><strong>模型设置</strong><p>单用户配置保存在本地工作区。</p></div></div>
      <label>服务商<select disabled={saving || locked} value={settings.provider} onChange={(event) => setSettings({ ...settings, provider: event.target.value })}><option value="local">本地创作引擎</option><option value="openai">OpenAI 兼容接口</option><option value="compatible">其他兼容接口</option></select></label>
      <label>模型名称<input disabled={saving || locked} value={settings.model} onChange={(event) => setSettings({ ...settings, model: event.target.value })} placeholder="local-creative" /></label>
      <label>接口地址<input disabled={saving || locked} value={settings.base_url} onChange={(event) => setSettings({ ...settings, base_url: event.target.value })} placeholder="https://api.example.com/v1" /></label><p className="settings-note"><CircleHelp size={13} /> HTTP 仅用于本机回环地址；其他模型服务请使用 HTTPS。</p>
      {settings.base_url_credentials_configured && <button className="settings-key-clear" type="button" disabled={saving || locked} onClick={() => setSettings({ ...settings, clear_base_url_credentials: !settings.clear_base_url_credentials })}>{settings.clear_base_url_credentials ? '撤销清除地址凭据' : '清除接口地址中隐藏的凭据'}</button>}
      {settings.clear_base_url_credentials && <p className="settings-url-warning">保存后将从本地配置中移除 URL 凭据。</p>}
      <label>API Key<input disabled={saving || locked} type="password" autoComplete="new-password" value={settings.api_key ?? ''} onChange={(event) => setSettings({ ...settings, api_key: event.target.value, clear_api_key: false })} placeholder={apiKeyPlaceholder} /></label>
      {settings.api_key_configured && <button className="settings-key-clear" type="button" disabled={saving || locked} onClick={() => setSettings({ ...settings, api_key: '', clear_api_key: !settings.clear_api_key })}>{settings.clear_api_key ? '撤销清除密钥' : '清除已保存的密钥'}</button>}
      {settings.clear_api_key && <p className="settings-key-warning">保存后将删除本地保存的 API Key。</p>}
      <label>创造性 <div className="range-row"><input disabled={saving || locked} type="range" min="0" max="2" step="0.1" value={settings.temperature} onChange={(event) => setSettings({ ...settings, temperature: Number(event.target.value) })} /><span>{settings.temperature.toFixed(1)}</span></div></label>
      <button className="primary-button settings-save" onClick={onSave} disabled={saving || locked || !dirty}>{saving ? '正在保存…' : saved ? <><Check size={14} /> 已保存</> : <><Save size={14} /> 保存设置</>}</button>
      <div className="settings-note"><CircleHelp size={13} /> 密钥只发送到本机服务，不会在设置读取或工作区响应中返回。</div>
    </div>
  )
}

function EmptyEditor({ onCreate, disabled }: { onCreate: () => void; disabled: boolean }) {
  return <div className="empty-editor"><div className="empty-orb"><PenLine size={23} /></div><h2>准备好写下一章了吗？</h2><p>从一个章节开始，Agent 会帮你管理线索、角色与节奏。</p><button className="primary-button" onClick={onCreate} disabled={disabled}><Plus size={15} /> 新建第一章</button></div>
}

function GitBranchIcon() { return <span className="status-glyph">⑂</span> }
function CircleStatus() { return <span className="status-glyph green-glyph">●</span> }
