import { isIMEComposing, normalizeEditorLineEndings, normalizeUnicodeSelection } from './text'
import type { Chapter } from '../types'
import type { Dispatch, KeyboardEvent, SetStateAction } from 'react'
import type { EditorSelection } from '../workbenchTypes'

type MutableRef<T> = { current: T }
type SaveState = 'saved' | 'saving' | 'dirty' | 'error'

interface EditorActionsOptions {
  editorRef: MutableRef<HTMLTextAreaElement | null>
  currentChapterRef: MutableRef<Chapter | undefined>
  editorContentRef: MutableRef<string>
  editorRevisionRef: MutableRef<number>
  setEditorContent: Dispatch<SetStateAction<string>>
  setEditorSelection: Dispatch<SetStateAction<EditorSelection | null>>
  setSaveRetryAttempt: Dispatch<SetStateAction<number>>
  setSaveState: Dispatch<SetStateAction<SaveState>>
}

export function createEditorActions(options: EditorActionsOptions) {
  const {
    editorRef,
    currentChapterRef,
    editorContentRef,
    editorRevisionRef,
    setEditorContent,
    setEditorSelection,
    setSaveRetryAttempt,
    setSaveState,
  } = options

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


  const handleEditorKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
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

  return { updateEditor, markEditorDirty, captureEditorSelection, handleEditorKeyDown }
}
