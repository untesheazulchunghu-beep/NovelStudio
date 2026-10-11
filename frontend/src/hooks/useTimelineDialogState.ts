import { useState } from 'react'
import type { TimelineEventStatus } from '../types'

export interface TimelineDialogDraft {
  title: string
  time_label: string
  description: string
  status: TimelineEventStatus
  chapter_id: number
}

export const emptyTimelineDialogDraft: TimelineDialogDraft = {
  title: '',
  time_label: '',
  description: '',
  status: 'planned',
  chapter_id: 0,
}

export function useTimelineDialogState() {
  const [timelineDialogOpen, setTimelineDialogOpen] = useState(false)
  const [timelineEditingId, setTimelineEditingId] = useState<number | null>(null)
  const [timelineDraft, setTimelineDraft] = useState<TimelineDialogDraft>(emptyTimelineDialogDraft)
  const [savingTimeline, setSavingTimeline] = useState(false)
  const [timelineDialogError, setTimelineDialogError] = useState('')

  return {
    timelineDialogOpen,
    setTimelineDialogOpen,
    timelineEditingId,
    setTimelineEditingId,
    timelineDraft,
    setTimelineDraft,
    savingTimeline,
    setSavingTimeline,
    timelineDialogError,
    setTimelineDialogError,
  }
}
