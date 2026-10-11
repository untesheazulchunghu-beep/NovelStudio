import { useState } from 'react'
import type { Chapter, ChapterRevision, ChapterRevisionSummary } from '../types'

export function useRevisionDialogState() {
  const [revisionDialogOpen, setRevisionDialogOpen] = useState(false)
  const [chapterRevisions, setChapterRevisions] = useState<ChapterRevisionSummary[]>([])
  const [selectedRevision, setSelectedRevision] = useState<ChapterRevision | null>(null)
  const [revisionCompareOpen, setRevisionCompareOpen] = useState(false)
  const [revisionCurrentSnapshot, setRevisionCurrentSnapshot] = useState<Pick<Chapter, 'title' | 'summary' | 'content' | 'status' | 'word_count'> | null>(null)
  const [revisionListLoading, setRevisionListLoading] = useState(false)
  const [revisionPreviewLoading, setRevisionPreviewLoading] = useState(false)
  const [revisionLabel, setRevisionLabel] = useState('手动版本')
  const [revisionError, setRevisionError] = useState('')

  return {
    revisionDialogOpen,
    setRevisionDialogOpen,
    chapterRevisions,
    setChapterRevisions,
    selectedRevision,
    setSelectedRevision,
    revisionCompareOpen,
    setRevisionCompareOpen,
    revisionCurrentSnapshot,
    setRevisionCurrentSnapshot,
    revisionListLoading,
    setRevisionListLoading,
    revisionPreviewLoading,
    setRevisionPreviewLoading,
    revisionLabel,
    setRevisionLabel,
    revisionError,
    setRevisionError,
  }
}
