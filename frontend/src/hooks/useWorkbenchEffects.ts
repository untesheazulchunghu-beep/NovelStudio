import { useEffect, useLayoutEffect } from 'react'
import { isIMEComposing, normalizeEditorLineEndings } from '../lib/text'
import type { Chapter, ChatMessage, WorkspaceResponse } from '../types'
import type { AssistantTab, EditorSelection } from '../workbenchTypes'
import type { Dispatch, SetStateAction } from 'react'

type MutableRef<T> = { current: T }
type SaveState = 'saved' | 'saving' | 'dirty' | 'error'

interface WorkbenchEffectsOptions {
  saveErrorMessageRef: MutableRef<string>
  setEditorSelection: Dispatch<SetStateAction<EditorSelection | null>>
  clearChatApplyTarget: () => void
  setSaveRetryAttempt: Dispatch<SetStateAction<number>>
  currentChapterRef: MutableRef<Chapter | undefined>
  currentChapter: Chapter | undefined
  editorContentRef: MutableRef<string>
  setEditorTitle: Dispatch<SetStateAction<string>>
  setEditorContent: Dispatch<SetStateAction<string>>
  setEditorSummary: Dispatch<SetStateAction<string>>
  setEditorStatus: Dispatch<SetStateAction<Chapter['status']>>
  setSaveState: Dispatch<SetStateAction<SaveState>>
  assistantCollapsed: boolean
  assistantTab: AssistantTab
  activeWorkspaceId: string
  chatScrollRef: MutableRef<HTMLDivElement | null>
  activeWorkspaceRef: MutableRef<string>
  data: WorkspaceResponse | null
  chatIsVisibleAtLatest: (workspaceID?: string) => boolean
  setWorkspaceChatUnread: (workspaceID: string | undefined, unread: boolean) => void
  chatAutoScrollRef: MutableRef<boolean>
  chatScrollPositionsRef: MutableRef<Map<string, number>>
  handleChatScroll: () => void
  chat: ChatMessage[]
  chatEndRef: MutableRef<HTMLDivElement | null>
  sending: boolean
  helpDialogOpen: boolean
  projectDialogOpen: boolean
  projectEditDialogOpen: boolean
  projectDeleteDialogOpen: boolean
  characterDialogOpen: boolean
  noteDialogOpen: boolean
  timelineDialogOpen: boolean
  worldAssetDialogOpen: boolean
  rewritePreviewOpen: boolean
  revisionDialogOpen: boolean
  searchOpen: boolean
  lastOutsideModalFocusRef: MutableRef<HTMLElement | null>
  modalWasOpenRef: MutableRef<boolean>
  modalOpen: boolean
}

export function useWorkbenchEffects(options: WorkbenchEffectsOptions) {
  const {
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
    helpDialogOpen,
    projectDialogOpen,
    projectEditDialogOpen,
    projectDeleteDialogOpen,
    characterDialogOpen,
    noteDialogOpen,
    timelineDialogOpen,
    worldAssetDialogOpen,
    rewritePreviewOpen,
    revisionDialogOpen,
    searchOpen,
    lastOutsideModalFocusRef,
    modalWasOpenRef,
    modalOpen,
  } = options

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
  }, [characterDialogOpen, helpDialogOpen, modalOpen, noteDialogOpen, projectDeleteDialogOpen, projectDialogOpen, projectEditDialogOpen, rewritePreviewOpen, searchOpen, timelineDialogOpen])


}
