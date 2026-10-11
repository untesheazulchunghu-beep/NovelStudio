import { api } from './api'
import { ChatPanel, ContextPanel, OutlinePanel } from './components/assistant'
import { ChapterRevisionsDialog } from './components/chapterRevisionsDialog'
import { createAgentActions } from './lib/agentActions'
import { createContextItemActions } from './lib/contextItemActions'
import { createEditorActions } from './lib/editorActions'
import { createProjectActions } from './lib/projectActions'
import { useWorkbenchEffects } from './hooks/useWorkbenchEffects'
import { useChapterActions } from './lib/chapterActions'
import { CommandPalette } from './components/commandPalette'
import { CircleStatus, EmptyEditor, GitBranchIcon } from './components/editor'
import { ActivityButton, CharactersView, ExplorerView, NotesView, SettingsView } from './components/sidebar'
import { suggestionsForMode } from './lib/agent'
import { characterColors, maxAgentSelectionCharacters } from './lib/constants'
import { formatNumber, relativeTime, safeProjectFileBaseName, statsForChapters, statusClass } from './lib/format'
import { buildPaletteResults } from './lib/palette'
import { countTextWords, countUnicodeCharactersInRange, isIMEComposing, normalizeEditorLineEndings, normalizeUnicodeSelection, sliceUnicodeRange } from './lib/text'
import { readActiveWorkspacePreference, rememberActiveWorkspacePreference } from './lib/workspacePreference'
import { BookOpen, Bot, BrainCircuit, Check, ChevronDown, ChevronRight, CircleHelp, FileText, FolderOpen, History, LayoutPanelLeft, Lightbulb, ListTodo, MessageSquare, MoreHorizontal, PanelRightClose, PanelRightOpen, PenLine, Plus, RefreshCw, Save, Search, Settings2, Target, Users, WandSparkles, X, Zap } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Chapter, ChapterRevision, ChapterRevisionSummary, ChapterSearchResult, Character, Note, ProjectSummary, SettingsDraft, SettingsUpdate, WorkspaceResponse } from './types'
import type { ActivityId, AssistantTab, ChatApplyTarget, ChatRequestRetry, EditorSelection, PaletteResult, WritingMode } from './workbenchTypes'

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

  useWorkbenchEffects({
    saveErrorMessageRef,
    setEditorSelection,
    clearChatApplyTarget,
    setSaveRetryAttempt,
    currentChapterRef,
    currentChapter,
    editorContentRef,
    setEditorTitle,
    setEditorContent,
    setEditorSummary,
    setEditorStatus,
    setSaveState,
    assistantCollapsed,
    assistantTab,
    activeWorkspaceId,
    chatScrollRef,
    activeWorkspaceRef,
    data,
    chatIsVisibleAtLatest,
    setWorkspaceChatUnread,
    chatAutoScrollRef,
    chatScrollPositionsRef,
    handleChatScroll,
    chat,
    chatEndRef,
    sending,
    editorLocked,
    saveChapterRef,
    helpDialogOpen,
    projectDialogOpen,
    projectEditDialogOpen,
    projectDeleteDialogOpen,
    characterDialogOpen,
    noteDialogOpen,
    rewritePreviewOpen,
    revisionDialogOpen,
    searchOpen,
    chapterActionBusy,
    deletingProject,
    savingProject,
    creatingProject,
    savingContextItem,
    agentRequestRef,
    setCancellingRequest,
    setSearchQuery,
    setSelectedSearchIndex,
    setSearchOpen,
    lastOutsideModalFocusRef,
    modalWasOpenRef,
    modalOpen,
    setRewritePreviewOpen,
    setRevisionDialogOpen,
    setHelpDialogOpen,
    setProjectDeleteDialogOpen,
    setProjectEditDialogOpen,
    setProjectDialogOpen,
    setCharacterDialogOpen,
    setNoteDialogOpen,
  })

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

  const searchResults = useMemo<PaletteResult[]>(() => buildPaletteResults({
    searchOpen,
    hasData: Boolean(data),
    searchQuery,
    currentChapter,
    chapterBodyLoading,
    projectActionBusy,
    chapterSearchResults,
    editorTitle,
    editorSummary,
    editorContent,
    characters,
    notes,
  }), [chapterBodyLoading, chapterSearchResults, characters, currentChapter, data, editorContent, editorSummary, editorTitle, notes, projectActionBusy, searchOpen, searchQuery])
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

  const {
    saveChapter,
    loadChapterRevision,
    openChapterRevisions,
    saveChapterRevision,
    restoreChapterRevision,
    selectChapter,
    createChapter,
    moveChapter,
    deleteChapter,
  } = useChapterActions({
    data,
    setData,
    currentChapter,
    currentChapterRef,
    activeWorkspaceRef,
    chapterLoadRetry,
    setChapterLoadError,
    updateLocalChapter,
    editorRevisionRef,
    saveErrorMessageRef,
    saveChapterRef,
    editorTitle,
    editorSummary,
    editorContent,
    editorStatus,
    editorContentRef,
    setEditorTitle,
    setEditorSummary,
    setEditorContent,
    setEditorStatus,
    setEditorSelection,
    setSaveState,
    saveState,
    setSaveRetryAttempt,
    setError,
    revisionPreviewRequestRef,
    setRevisionPreviewLoading,
    setSelectedRevision,
    selectedRevision,
    setRevisionError,
    projectActionBusy,
    chapterActionBusy,
    setChapterActionBusy,
    sending,
    setRevisionCurrentSnapshot,
    setRevisionCompareOpen,
    setRevisionDialogOpen,
    setChapterRevisions,
    setRevisionLabel,
    revisionLabel,
    setRevisionListLoading,
    chatApplyTarget,
    hasEditedRewriteDraft,
    confirmDiscardEditedRewrite,
    setShowMobileSidebar,
    setActiveChapterId,
    activeChapterId,
    chapters,
    setActiveActivity,
  })
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


  const {
    openCharacterCreate,
    openCharacterEdit,
    saveCharacter,
    deleteCharacter,
    openNoteCreate,
    openNoteEdit,
    saveNote,
    deleteNote,
  } = createContextItemActions({
    data,
    setData,
    activeWorkspaceRef,
    projectActionBusy,
    characters,
    characterColors,
    characterEditingId,
    setCharacterEditingId,
    characterDraft,
    setCharacterDraft,
    setCharacterDialogOpen,
    noteEditingId,
    setNoteEditingId,
    noteDraft,
    setNoteDraft,
    setNoteDialogOpen,
    setSavingContextItem,
    setContextDialogError,
    setError,
  })
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

  const { updateEditor, markEditorDirty, captureEditorSelection, handleEditorKeyDown } = createEditorActions({
    editorRef,
    currentChapterRef,
    editorContentRef,
    editorRevisionRef,
    setEditorContent,
    setEditorSelection,
    setSaveRetryAttempt,
    setSaveState,
  })
  const {
    sendMessage,
    checkChatRecovery,
    retryAfterReview,
    applyChatRewrite,
    confirmRewritePreview,
    cancelAgentRequest,
    applyGeneratedText,
  } = createAgentActions({
    data,
    setData,
    projectActionBusy,
    chapterBodyLoading,
    settingsDirty,
    chatInput,
    setChatInput,
    updateChatInput,
    writingModeRef,
    editorContentRef,
    currentChapterRef,
    currentChapter,
    editorTitle,
    editorSummary,
    activeEditorSelection,
    activeWorkspaceRef,
    chatAutoScrollRef,
    chatAutoScrollByWorkspaceRef,
    chatInputRevisionRef,
    chatDraftsRef,
    chatRequestRetryRef,
    agentRequestRef,
    saveErrorMessageRef,
    editorRef,
    lastOutsideModalFocusRef,
    setEditorSelection,
    setChatDraftRevision,
    setSending,
    setCancellingRequest,
    setCheckingChatRecovery,
    checkingChatRecovery,
    sending,
    setError,
    setChatApplyTarget,
    chatApplyTarget,
    rewritePreviewOpen,
    setRewritePreviewOpen,
    setSuggestions,
    setWorkspaceChatUnread,
    chatIsVisibleAtLatest,
    confirmDiscardEditedRewrite,
    clearChatApplyTarget,
    updateEditor,
    activateAssistantTab,
  })
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

  const { createProject, openProjectEdit, saveProject, openProjectDelete, confirmProjectDelete } = createProjectActions({
    data,
    setData,
    project,
    projects,
    setProjects,
    activeWorkspaceRef,
    projectActionBusy,
    saveState,
    settingsDirty,
    sending,
    savingContextItem,
    savingSettings,
    chapterActionBusy,
    saveChapter,
    saveSettings,
    confirmDiscardEditedRewrite,
    setError,
    projectTitle,
    projectGenre,
    projectPremise,
    setProjectTitle,
    setProjectGenre,
    setProjectPremise,
    creatingProject,
    setCreatingProject,
    setProjectDialogOpen,
    projectDraft,
    setProjectDraft,
    savingProject,
    setSavingProject,
    setProjectEditError,
    setProjectEditDialogOpen,
    setProjectDeleteInput,
    projectDeleteInput,
    setProjectDeleteError,
    setProjectDeleteDialogOpen,
    deletingProject,
    setDeletingProject,
    selectWorkspace,
    chatDraftsRef,
    setChatDraftRevision,
    chatRequestRetryRef,
    unreadChatWorkspacesRef,
    chatAutoScrollByWorkspaceRef,
    chatScrollPositionsRef,
  })

  const updateSettingsDraft = (settings: SettingsDraft) => {
    setSettingsDraft(settings)
    setSettingsDirty(true)
    setSettingsSaved(false)
    setSettingsError('')
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
      {revisionDialogOpen && currentChapter && data && <ChapterRevisionsDialog
        revisionDialogOpen={revisionDialogOpen}
        setRevisionDialogOpen={setRevisionDialogOpen}
        currentChapter={currentChapter}
        workspaceID={data.workspace.workspace_id}
        chapterActionBusy={chapterActionBusy}
        revisionError={revisionError}
        revisionLabel={revisionLabel}
        setRevisionLabel={setRevisionLabel}
        saveChapterRevision={saveChapterRevision}
        revisionListLoading={revisionListLoading}
        chapterRevisions={chapterRevisions}
        selectedRevision={selectedRevision}
        onSelectRevision={(revision) => void loadChapterRevision(revision, data.workspace.workspace_id, currentChapter.id)}
        revisionPreviewLoading={revisionPreviewLoading}
        revisionCompareOpen={revisionCompareOpen}
        setRevisionCompareOpen={setRevisionCompareOpen}
        revisionCurrentSnapshot={revisionCurrentSnapshot}
        restoreChapterRevision={restoreChapterRevision}
        sending={sending}
      />}
      {projectDeleteDialogOpen && project && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !deletingProject) setProjectDeleteDialogOpen(false) }}><section className="project-dialog delete-project-dialog" role="dialog" aria-modal="true" aria-labelledby="project-delete-title" aria-describedby="project-delete-warning"><header><div><span className="dialog-kicker">DELETE WORKSPACE</span><h2 id="project-delete-title">删除小说项目</h2></div><button className="subtle-icon" onClick={() => setProjectDeleteDialogOpen(false)} aria-label="关闭删除确认" disabled={deletingProject}><X size={16} /></button></header><p className="delete-project-warning" id="project-delete-warning">项目 <strong>{project.title}</strong> 的章节、角色、便笺、聊天记录和模型设置都会被永久删除，当前未保存的编辑也会丢失。此操作无法撤销。</p><label>输入项目名称以确认<input autoFocus value={projectDeleteInput} onChange={(event) => setProjectDeleteInput(event.target.value)} onKeyDown={(event) => { if (!isIMEComposing(event.nativeEvent) && event.key === 'Enter' && projectDeleteInput === project.title && !projectActionBusy) void confirmProjectDelete() }} maxLength={80} /></label>{projectDeleteError && <p className="dialog-error" role="alert">{projectDeleteError}</p>}<div className="dialog-actions"><button className="toolbar-button" onClick={() => setProjectDeleteDialogOpen(false)} disabled={deletingProject}>取消</button><button className="danger-button" onClick={() => void confirmProjectDelete()} disabled={projectDeleteInput !== project.title || projectActionBusy || projects.length <= 1}>{deletingProject ? '正在删除…' : '永久删除项目'}</button></div></section></div>}
      {searchOpen && <CommandPalette
        query={searchQuery}
        results={searchResults}
        selectedIndex={activeSearchIndex}
        inputRef={searchInputRef}
        activeResultRef={searchActiveResultRef}
        onQueryChange={(query) => { setSearchQuery(query); setSelectedSearchIndex(0) }}
        onSelectIndex={setSelectedSearchIndex}
        onActivate={activateSearchResult}
        onClose={closeSearch}
      />}
      {helpDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) setHelpDialogOpen(false) }}><section className="project-dialog help-dialog" role="dialog" aria-modal="true" aria-labelledby="help-dialog-title"><header><div><span className="dialog-kicker">QUICK HELP</span><h2 id="help-dialog-title">帮助与快捷键</h2></div><button className="subtle-icon" onClick={() => setHelpDialogOpen(false)} aria-label="关闭帮助"><X size={16} /></button></header><p className="help-intro">使用快捷键更快地搜索项目、保存章节和操作写作 Agent。</p><div className="help-shortcuts"><div><kbd>Ctrl / ⌘ + K</kbd><span>搜索章节、角色、便笺和命令</span></div><div><kbd>Ctrl / ⌘ + S</kbd><span>保存当前章节（编辑器中）</span></div><div><kbd>Tab / Shift + Tab</kbd><span>选中行可缩进/反向缩进；无选区时 Tab 插入空格，Shift + Tab 减少当前行缩进</span></div><div><kbd>↑ / ↓</kbd><span>在搜索结果中移动</span></div><div><kbd>Enter</kbd><span>打开搜索结果或发送 Agent 消息</span></div><div><kbd>Shift + Enter</kbd><span>在 Agent 输入框中换行</span></div><div><kbd>Esc</kbd><span>关闭搜索/帮助；没有面板打开时取消 Agent 请求</span></div></div><div className="dialog-actions"><button className="primary-button" onClick={() => setHelpDialogOpen(false)}>完成</button></div></section></div>}
      {projectEditDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !savingProject) setProjectEditDialogOpen(false) }}><section className="project-dialog" role="dialog" aria-modal="true" aria-labelledby="project-edit-title"><header><div><span className="dialog-kicker">PROJECT DETAILS</span><h2 id="project-edit-title">编辑项目资料</h2></div><button className="subtle-icon" onClick={() => setProjectEditDialogOpen(false)} aria-label="关闭" disabled={savingProject}><X size={16} /></button></header><label>项目名称<input autoFocus value={projectDraft.title} onChange={(event) => setProjectDraft({ ...projectDraft, title: event.target.value })} maxLength={80} disabled={savingProject} /></label><label>副标题<input value={projectDraft.subtitle} onChange={(event) => setProjectDraft({ ...projectDraft, subtitle: event.target.value })} placeholder="可选" maxLength={120} disabled={savingProject} /></label><label>题材<input value={projectDraft.genre} onChange={(event) => setProjectDraft({ ...projectDraft, genre: event.target.value })} placeholder="例如：近未来 · 悬疑" maxLength={80} disabled={savingProject} /></label><label>故事设定<textarea value={projectDraft.premise} onChange={(event) => setProjectDraft({ ...projectDraft, premise: event.target.value })} rows={3} maxLength={500} disabled={savingProject} /></label><label>目标字数<input type="number" min={1} step={1} value={projectDraft.word_target} onChange={(event) => setProjectDraft({ ...projectDraft, word_target: Math.trunc(Number(event.target.value) || 0) })} disabled={savingProject} /></label>{projectEditError && <p className="dialog-error" role="alert">{projectEditError}</p>}<div className="dialog-actions"><button className="toolbar-button" onClick={() => setProjectEditDialogOpen(false)} disabled={savingProject}>取消</button><button className="primary-button" onClick={() => void saveProject()} disabled={!projectDraft.title.trim() || projectDraft.word_target < 1 || savingProject || sending || savingContextItem || savingSettings || chapterActionBusy || saveState === 'saving'}>{savingProject ? '正在保存…' : <><Check size={14} /> 保存资料</>}</button></div></section></div>}
      {projectDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !creatingProject) setProjectDialogOpen(false) }}><section className="project-dialog" role="dialog" aria-modal="true" aria-labelledby="project-dialog-title"><header><div><span className="dialog-kicker">NEW WORKSPACE</span><h2 id="project-dialog-title">创建小说项目</h2></div><button className="subtle-icon" onClick={() => setProjectDialogOpen(false)} aria-label="关闭" disabled={creatingProject}><X size={16} /></button></header><label>项目名称<input autoFocus value={projectTitle} onChange={(event) => setProjectTitle(event.target.value)} placeholder="例如：雾港来信" maxLength={80} disabled={creatingProject} /></label><label>题材<input value={projectGenre} onChange={(event) => setProjectGenre(event.target.value)} placeholder="近未来 · 悬疑" maxLength={80} disabled={creatingProject} /></label><label>故事设定<textarea value={projectPremise} onChange={(event) => setProjectPremise(event.target.value)} placeholder="用一两句话描述故事的核心设定。" rows={3} maxLength={500} disabled={creatingProject} /></label><div className="dialog-actions"><button className="toolbar-button" onClick={() => setProjectDialogOpen(false)} disabled={creatingProject}>取消</button><button className="primary-button" onClick={() => void createProject()} disabled={!projectTitle.trim() || creatingProject || sending || saveState === 'saving'}>{creatingProject ? '正在创建…' : <><Plus size={14} /> 创建项目</>}</button></div></section></div>}
      {characterDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !savingContextItem) setCharacterDialogOpen(false) }}><section className="project-dialog" role="dialog" aria-modal="true" aria-labelledby="character-dialog-title"><header><div><span className="dialog-kicker">PROJECT CHARACTER</span><h2 id="character-dialog-title">{characterEditingId === null ? '新建角色' : '编辑角色'}</h2></div><button className="subtle-icon" onClick={() => setCharacterDialogOpen(false)} aria-label="关闭" disabled={savingContextItem}><X size={16} /></button></header><label>角色名称<input autoFocus value={characterDraft.name} onChange={(event) => setCharacterDraft({ ...characterDraft, name: event.target.value })} placeholder="例如：林澈" maxLength={80} disabled={savingContextItem} /></label><label>身份<input value={characterDraft.role} onChange={(event) => setCharacterDraft({ ...characterDraft, role: event.target.value })} placeholder="主角、导师、对手……" maxLength={100} disabled={savingContextItem} /></label><label>角色描述<textarea value={characterDraft.description} onChange={(event) => setCharacterDraft({ ...characterDraft, description: event.target.value })} placeholder="角色的背景、目标或关键矛盾。" rows={3} maxLength={1000} disabled={savingContextItem} /></label><label>人物特质<input value={characterDraft.traits} onChange={(event) => setCharacterDraft({ ...characterDraft, traits: event.target.value })} placeholder="敏锐、克制、怕水（用逗号分隔）" maxLength={240} disabled={savingContextItem} /></label><label>角色颜色<input className="character-color-input" type="color" value={characterDraft.color || characterColors[0]} onChange={(event) => setCharacterDraft({ ...characterDraft, color: event.target.value })} disabled={savingContextItem} /></label>{contextDialogError && <p className="dialog-error" role="alert">{contextDialogError}</p>}<div className="dialog-actions"><button className="toolbar-button" onClick={() => setCharacterDialogOpen(false)} disabled={savingContextItem}>取消</button><button className="primary-button" onClick={() => void saveCharacter()} disabled={!characterDraft.name.trim() || projectActionBusy}>{savingContextItem ? '正在保存…' : <><Check size={14} /> {characterEditingId === null ? '添加角色' : '保存修改'}</>}</button></div></section></div>}
      {noteDialogOpen && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !savingContextItem) setNoteDialogOpen(false) }}><section className="project-dialog" role="dialog" aria-modal="true" aria-labelledby="note-dialog-title"><header><div><span className="dialog-kicker">PROJECT NOTE</span><h2 id="note-dialog-title">{noteEditingId === null ? '新建便笺' : '编辑便笺'}</h2></div><button className="subtle-icon" onClick={() => setNoteDialogOpen(false)} aria-label="关闭" disabled={savingContextItem}><X size={16} /></button></header><label>标题<input autoFocus value={noteDraft.title} onChange={(event) => setNoteDraft({ ...noteDraft, title: event.target.value })} placeholder="一句话概括这个想法" maxLength={120} disabled={savingContextItem} /></label><label>分类<select value={noteDraft.kind} onChange={(event) => setNoteDraft({ ...noteDraft, kind: event.target.value })} disabled={savingContextItem}>{!["idea", "mystery", "craft", "world"].includes(noteDraft.kind) && <option value={noteDraft.kind}>保留原分类：{noteDraft.kind}</option>}<option value="idea">灵感</option><option value="mystery">悬念</option><option value="craft">写作提醒</option><option value="world">世界观</option></select></label><label>内容<textarea value={noteDraft.body} onChange={(event) => setNoteDraft({ ...noteDraft, body: event.target.value })} placeholder="记录线索、设定或之后要处理的想法。" rows={4} maxLength={2000} disabled={savingContextItem} /></label>{contextDialogError && <p className="dialog-error" role="alert">{contextDialogError}</p>}<div className="dialog-actions"><button className="toolbar-button" onClick={() => setNoteDialogOpen(false)} disabled={savingContextItem}>取消</button><button className="primary-button" onClick={() => void saveNote()} disabled={!noteDraft.title.trim() || projectActionBusy}>{savingContextItem ? '正在保存…' : <><Check size={14} /> {noteEditingId === null ? '添加便笺' : '保存修改'}</>}</button></div></section></div>}
    </div>
  )
}
