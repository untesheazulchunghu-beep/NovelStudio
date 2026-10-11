import { api } from '../api'
import type { Dispatch, SetStateAction } from 'react'
import type { TimelineDialogDraft } from '../hooks/useTimelineDialogState'
import type { TimelineEvent, WorkspaceResponse } from '../types'

type MutableRef<T> = { current: T }

interface TimelineActionsOptions {
  data: WorkspaceResponse | null
  setData: Dispatch<SetStateAction<WorkspaceResponse | null>>
  activeWorkspaceRef: MutableRef<string>
  projectActionBusy: boolean
  timelineEditingId: number | null
  setTimelineEditingId: Dispatch<SetStateAction<number | null>>
  timelineDraft: TimelineDialogDraft
  setTimelineDraft: Dispatch<SetStateAction<TimelineDialogDraft>>
  setTimelineDialogOpen: Dispatch<SetStateAction<boolean>>
  setSavingTimeline: Dispatch<SetStateAction<boolean>>
  setTimelineDialogError: Dispatch<SetStateAction<string>>
  setError: Dispatch<SetStateAction<string>>
}

export function createTimelineActions(options: TimelineActionsOptions) {
  const {
    data,
    setData,
    activeWorkspaceRef,
    projectActionBusy,
    timelineEditingId,
    setTimelineEditingId,
    timelineDraft,
    setTimelineDraft,
    setTimelineDialogOpen,
    setSavingTimeline,
    setTimelineDialogError,
    setError,
  } = options

  const openTimelineCreate = () => {
    if (projectActionBusy) return
    setTimelineEditingId(null)
    setTimelineDraft({ title: '', time_label: '', description: '', status: 'planned', chapter_id: 0 })
    setTimelineDialogError('')
    setTimelineDialogOpen(true)
  }

  const openTimelineEdit = (event: TimelineEvent) => {
    if (projectActionBusy) return
    setTimelineEditingId(event.id)
    setTimelineDraft({
      title: event.title,
      time_label: event.time_label,
      description: event.description,
      status: event.status === 'active' || event.status === 'done' ? event.status : 'planned',
      chapter_id: event.chapter_id ?? 0,
    })
    setTimelineDialogError('')
    setTimelineDialogOpen(true)
  }

  const saveTimelineEvent = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || !timelineDraft.title.trim() || projectActionBusy) return
    const eventId = timelineEditingId
    setSavingTimeline(true)
    setTimelineDialogError('')
    try {
      const input = {
        title: timelineDraft.title.trim(),
        time_label: timelineDraft.time_label.trim(),
        description: timelineDraft.description,
        status: timelineDraft.status,
        chapter_id: timelineDraft.chapter_id || undefined,
      }
      const event = eventId === null
        ? await api.timeline.create(workspaceId, input)
        : await api.timeline.update(workspaceId, eventId, input)
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        const timelineNext = eventId === null
          ? [...current.workspace.timeline, event]
          : current.workspace.timeline.map((item) => item.id === event.id ? event : item)
        return { ...current, workspace: { ...current.workspace, timeline: timelineNext } }
      })
      if (activeWorkspaceRef.current === workspaceId) {
        setTimelineDraft({ title: '', time_label: '', description: '', status: 'planned', chapter_id: 0 })
        setTimelineEditingId(null)
        setTimelineDialogOpen(false)
      }
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setTimelineDialogError(reason instanceof Error ? reason.message : '保存时间线事件失败')
    } finally {
      setSavingTimeline(false)
    }
  }

  const deleteTimelineEvent = async (event: TimelineEvent) => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy || !window.confirm(`确定删除时间线事件“${event.title}”吗？`)) return
    setSavingTimeline(true)
    try {
      await api.timeline.remove(workspaceId, event.id)
      setData((current) => current && current.workspace.workspace_id === workspaceId
        ? { ...current, workspace: { ...current.workspace, timeline: current.workspace.timeline.filter((item) => item.id !== event.id) } }
        : current)
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setError(reason instanceof Error ? reason.message : '删除时间线事件失败')
    } finally {
      setSavingTimeline(false)
    }
  }

  return { openTimelineCreate, openTimelineEdit, saveTimelineEvent, deleteTimelineEvent }
}
