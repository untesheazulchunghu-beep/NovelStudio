import { api } from '../api'
import type { Project, ProjectSummary, WorkspaceResponse } from '../types'
import type { ChatRequestRetry } from '../workbenchTypes'
import type { Dispatch, SetStateAction } from 'react'

interface ProjectDraft {
  title: string
  subtitle: string
  genre: string
  premise: string
  word_target: number
}

type MutableRef<T> = { current: T }
type SaveState = 'saved' | 'saving' | 'dirty' | 'error'
type SelectWorkspace = (workspaceId: string, chapterAlreadySaved?: boolean, allowDuringProjectDelete?: boolean, rewriteDiscardApproved?: boolean) => Promise<void>

interface ProjectActionsOptions {
  data: WorkspaceResponse | null
  setData: Dispatch<SetStateAction<WorkspaceResponse | null>>
  project: Project | undefined
  projects: ProjectSummary[]
  setProjects: Dispatch<SetStateAction<ProjectSummary[]>>
  activeWorkspaceRef: MutableRef<string>
  projectActionBusy: boolean
  saveState: SaveState
  settingsDirty: boolean
  sending: boolean
  savingContextItem: boolean
  savingSettings: boolean
  chapterActionBusy: boolean
  saveChapter: () => Promise<boolean>
  saveSettings: () => Promise<boolean>
  confirmDiscardEditedRewrite: (workspaceID?: string, chapterID?: number) => boolean
  setError: Dispatch<SetStateAction<string>>
  projectTitle: string
  projectGenre: string
  projectPremise: string
  setProjectTitle: Dispatch<SetStateAction<string>>
  setProjectGenre: Dispatch<SetStateAction<string>>
  setProjectPremise: Dispatch<SetStateAction<string>>
  creatingProject: boolean
  setCreatingProject: Dispatch<SetStateAction<boolean>>
  setProjectDialogOpen: Dispatch<SetStateAction<boolean>>
  projectDraft: ProjectDraft
  setProjectDraft: Dispatch<SetStateAction<ProjectDraft>>
  savingProject: boolean
  setSavingProject: Dispatch<SetStateAction<boolean>>
  setProjectEditError: Dispatch<SetStateAction<string>>
  setProjectEditDialogOpen: Dispatch<SetStateAction<boolean>>
  setProjectDeleteInput: Dispatch<SetStateAction<string>>
  projectDeleteInput: string
  setProjectDeleteError: Dispatch<SetStateAction<string>>
  setProjectDeleteDialogOpen: Dispatch<SetStateAction<boolean>>
  deletingProject: boolean
  setDeletingProject: Dispatch<SetStateAction<boolean>>
  selectWorkspace: SelectWorkspace
  chatDraftsRef: MutableRef<Map<string, string>>
  setChatDraftRevision: Dispatch<SetStateAction<number>>
  chatRequestRetryRef: MutableRef<Map<string, ChatRequestRetry>>
  unreadChatWorkspacesRef: MutableRef<Set<string>>
  chatAutoScrollByWorkspaceRef: MutableRef<Map<string, boolean>>
  chatScrollPositionsRef: MutableRef<Map<string, number>>
}

export function createProjectActions(options: ProjectActionsOptions) {
  const {
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
  } = options

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

  return { createProject, openProjectEdit, saveProject, openProjectDelete, confirmProjectDelete }
}
