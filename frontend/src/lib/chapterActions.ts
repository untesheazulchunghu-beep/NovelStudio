import { useCallback, useEffect } from 'react'
import { api } from '../api'
import { statsForChapters } from './format'
import { countTextWords, normalizeEditorLineEndings } from './text'
import type { Chapter, ChapterRevision, ChapterRevisionSummary, WorkspaceResponse } from '../types'
import type { ActivityId, ChatApplyTarget, EditorSelection } from '../workbenchTypes'
import type { Dispatch, SetStateAction } from 'react'

type MutableRef<T> = { current: T }
type SaveState = 'saved' | 'saving' | 'dirty' | 'error'

interface ChapterActionsOptions {
  data: WorkspaceResponse | null
  setData: Dispatch<SetStateAction<WorkspaceResponse | null>>
  currentChapter: Chapter | undefined
  currentChapterRef: MutableRef<Chapter | undefined>
  activeWorkspaceRef: MutableRef<string>
  chapterLoadRetry: number
  setChapterLoadError: Dispatch<SetStateAction<string>>
  updateLocalChapter: (workspaceId: string, chapter: Chapter) => void
  editorRevisionRef: MutableRef<number>
  saveErrorMessageRef: MutableRef<string>
  saveChapterRef: MutableRef<() => Promise<boolean>>
  editorTitle: string
  editorSummary: string
  editorContent: string
  editorStatus: Chapter['status']
  editorContentRef: MutableRef<string>
  setEditorTitle: Dispatch<SetStateAction<string>>
  setEditorSummary: Dispatch<SetStateAction<string>>
  setEditorContent: Dispatch<SetStateAction<string>>
  setEditorStatus: Dispatch<SetStateAction<Chapter['status']>>
  setEditorSelection: Dispatch<SetStateAction<EditorSelection | null>>
  setSaveState: Dispatch<SetStateAction<SaveState>>
  saveState: SaveState
  setSaveRetryAttempt: Dispatch<SetStateAction<number>>
  setError: Dispatch<SetStateAction<string>>
  revisionPreviewRequestRef: MutableRef<number>
  setRevisionPreviewLoading: Dispatch<SetStateAction<boolean>>
  setSelectedRevision: Dispatch<SetStateAction<ChapterRevision | null>>
  selectedRevision: ChapterRevision | null
  setRevisionError: Dispatch<SetStateAction<string>>
  projectActionBusy: boolean
  chapterActionBusy: boolean
  setChapterActionBusy: Dispatch<SetStateAction<boolean>>
  sending: boolean
  setRevisionCurrentSnapshot: Dispatch<SetStateAction<Pick<Chapter, 'title' | 'summary' | 'content' | 'status' | 'word_count'> | null>>
  setRevisionCompareOpen: Dispatch<SetStateAction<boolean>>
  setRevisionDialogOpen: Dispatch<SetStateAction<boolean>>
  setChapterRevisions: Dispatch<SetStateAction<ChapterRevisionSummary[]>>
  setRevisionLabel: Dispatch<SetStateAction<string>>
  revisionLabel: string
  setRevisionListLoading: Dispatch<SetStateAction<boolean>>
  chatApplyTarget: ChatApplyTarget | null
  hasEditedRewriteDraft: boolean
  confirmDiscardEditedRewrite: (workspaceID?: string, chapterID?: number) => boolean
  setShowMobileSidebar: Dispatch<SetStateAction<boolean>>
  setActiveChapterId: Dispatch<SetStateAction<number | null>>
  activeChapterId: number | null
  chapters: Chapter[]
  setActiveActivity: Dispatch<SetStateAction<ActivityId>>
}

export function useChapterActions(options: ChapterActionsOptions) {
  const {
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
  } = options

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


  return {
    saveChapter,
    loadChapterRevision,
    openChapterRevisions,
    saveChapterRevision,
    restoreChapterRevision,
    selectChapter,
    createChapter,
    moveChapter,
    deleteChapter,
  }
}
