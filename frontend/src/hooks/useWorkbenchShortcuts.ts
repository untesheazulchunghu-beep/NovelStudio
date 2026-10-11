import { useEffect } from 'react'
import { isIMEComposing } from '../lib/text'
import type { Dispatch, SetStateAction } from 'react'

type MutableRef<T> = { current: T }

interface WorkbenchShortcutOptions {
  hasData: boolean
  hasCurrentChapter: boolean
  editorLocked: boolean
  saveChapterRef: MutableRef<() => Promise<boolean>>
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
  chapterActionBusy: boolean
  deletingProject: boolean
  savingProject: boolean
  creatingProject: boolean
  savingContextItem: boolean
  savingTimeline: boolean
  savingWorldAsset: boolean
  agentRequestRef: MutableRef<AbortController | null>
  setCancellingRequest: Dispatch<SetStateAction<boolean>>
  setSearchQuery: Dispatch<SetStateAction<string>>
  setSelectedSearchIndex: Dispatch<SetStateAction<number>>
  setSearchOpen: Dispatch<SetStateAction<boolean>>
  setRewritePreviewOpen: Dispatch<SetStateAction<boolean>>
  setRevisionDialogOpen: Dispatch<SetStateAction<boolean>>
  setHelpDialogOpen: Dispatch<SetStateAction<boolean>>
  setProjectDeleteDialogOpen: Dispatch<SetStateAction<boolean>>
  setProjectEditDialogOpen: Dispatch<SetStateAction<boolean>>
  setProjectDialogOpen: Dispatch<SetStateAction<boolean>>
  setCharacterDialogOpen: Dispatch<SetStateAction<boolean>>
  setNoteDialogOpen: Dispatch<SetStateAction<boolean>>
  setTimelineDialogOpen: Dispatch<SetStateAction<boolean>>
  setWorldAssetDialogOpen: Dispatch<SetStateAction<boolean>>
}

export function useWorkbenchShortcuts(options: WorkbenchShortcutOptions) {
  const {
    hasData,
    hasCurrentChapter,
    editorLocked,
    saveChapterRef,
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
    chapterActionBusy,
    deletingProject,
    savingProject,
    creatingProject,
    savingContextItem,
    savingTimeline,
    savingWorldAsset,
    agentRequestRef,
    setCancellingRequest,
    setSearchQuery,
    setSelectedSearchIndex,
    setSearchOpen,
    setRewritePreviewOpen,
    setRevisionDialogOpen,
    setHelpDialogOpen,
    setProjectDeleteDialogOpen,
    setProjectEditDialogOpen,
    setProjectDialogOpen,
    setCharacterDialogOpen,
    setNoteDialogOpen,
    setTimelineDialogOpen,
    setWorldAssetDialogOpen,
  } = options

  useEffect(() => {
    const handleGlobalShortcuts = (event: KeyboardEvent) => {
      if (isIMEComposing(event)) return
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
        const target = event.target
        if (!(target instanceof HTMLElement) || !target.closest('.editor-pane') || !hasData || !hasCurrentChapter) return
        event.preventDefault()
        if (!editorLocked) void saveChapterRef.current()
        return
      }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        if (helpDialogOpen || projectDialogOpen || projectEditDialogOpen || projectDeleteDialogOpen || characterDialogOpen || noteDialogOpen || timelineDialogOpen || worldAssetDialogOpen || rewritePreviewOpen || revisionDialogOpen) {
          event.preventDefault()
          return
        }
        if (!hasData) return
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
        } else if (timelineDialogOpen) {
          event.preventDefault()
          if (!savingTimeline) setTimelineDialogOpen(false)
        } else if (worldAssetDialogOpen) {
          event.preventDefault()
          if (!savingWorldAsset) setWorldAssetDialogOpen(false)
        } else if (agentRequestRef.current) {
          event.preventDefault()
          setCancellingRequest(true)
          agentRequestRef.current.abort()
        }
      }
    }
    window.addEventListener('keydown', handleGlobalShortcuts)
    return () => window.removeEventListener('keydown', handleGlobalShortcuts)
  }, [agentRequestRef, chapterActionBusy, characterDialogOpen, creatingProject, deletingProject, editorLocked, hasCurrentChapter, hasData, helpDialogOpen, noteDialogOpen, projectDeleteDialogOpen, projectDialogOpen, projectEditDialogOpen, revisionDialogOpen, rewritePreviewOpen, savingContextItem, savingProject, savingTimeline, savingWorldAsset, searchOpen, setCancellingRequest, setCharacterDialogOpen, setHelpDialogOpen, setNoteDialogOpen, setProjectDeleteDialogOpen, setProjectDialogOpen, setProjectEditDialogOpen, setRewritePreviewOpen, setRevisionDialogOpen, setSearchOpen, setSearchQuery, setSelectedSearchIndex, setTimelineDialogOpen, setWorldAssetDialogOpen, timelineDialogOpen, worldAssetDialogOpen])
}
