import { api } from '../api'
import type { Dispatch, SetStateAction } from 'react'
import type { WorldAsset } from '../types'
import type { WorldAssetDialogDraft } from '../hooks/useWorldAssetDialogState'
import type { WorkspaceResponse } from '../types'

type MutableRef<T> = { current: T }

interface WorldAssetActionsOptions {
  data: WorkspaceResponse | null
  setData: Dispatch<SetStateAction<WorkspaceResponse | null>>
  activeWorkspaceRef: MutableRef<string>
  projectActionBusy: boolean
  editingId: number | null
  setEditingId: Dispatch<SetStateAction<number | null>>
  draft: WorldAssetDialogDraft
  setDraft: Dispatch<SetStateAction<WorldAssetDialogDraft>>
  setOpen: Dispatch<SetStateAction<boolean>>
  setSaving: Dispatch<SetStateAction<boolean>>
  setDialogError: Dispatch<SetStateAction<string>>
  setError: Dispatch<SetStateAction<string>>
}

export function createWorldAssetActions(options: WorldAssetActionsOptions) {
  const { data, setData, activeWorkspaceRef, projectActionBusy, editingId, setEditingId, draft, setDraft, setOpen, setSaving, setDialogError, setError } = options
  const openWorldAssetCreate = () => {
    if (projectActionBusy) return
    setEditingId(null)
    setDraft({ name: '', kind: 'place', description: '', details: '', chapter_ids: [], related_asset_ids: [] })
    setDialogError('')
    setOpen(true)
  }
  const openWorldAssetEdit = (asset: WorldAsset) => {
    if (projectActionBusy) return
    setEditingId(asset.id)
    setDraft({ name: asset.name, kind: ['place', 'organization', 'item', 'concept'].includes(asset.kind) ? asset.kind as WorldAssetDialogDraft['kind'] : 'concept', description: asset.description, details: asset.details, chapter_ids: [...(asset.chapter_ids ?? [])], related_asset_ids: [...(asset.related_asset_ids ?? [])] })
    setDialogError('')
    setOpen(true)
  }
  const saveWorldAsset = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || !draft.name.trim() || projectActionBusy) return
    setSaving(true)
    setDialogError('')
    try {
      const input = { name: draft.name.trim(), kind: draft.kind, description: draft.description, details: draft.details, chapter_ids: draft.chapter_ids, related_asset_ids: draft.related_asset_ids }
      const asset = editingId === null ? await api.worldAsset.create(workspaceId, input) : await api.worldAsset.update(workspaceId, editingId, input)
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        const worldAssets = editingId === null ? [...current.workspace.world_assets, asset] : current.workspace.world_assets.map((item) => item.id === asset.id ? asset : item)
        return { ...current, workspace: { ...current.workspace, world_assets: worldAssets } }
      })
      if (activeWorkspaceRef.current === workspaceId) {
        setDraft({ name: '', kind: 'place', description: '', details: '', chapter_ids: [], related_asset_ids: [] })
        setEditingId(null)
        setOpen(false)
      }
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setDialogError(reason instanceof Error ? reason.message : '保存世界观资料失败')
    } finally { setSaving(false) }
  }
  const deleteWorldAsset = async (asset: WorldAsset) => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy || !window.confirm(`确定删除世界观资料“${asset.name}”吗？`)) return
    setSaving(true)
    try {
      await api.worldAsset.remove(workspaceId, asset.id)
      setData((current) => current && current.workspace.workspace_id === workspaceId ? { ...current, workspace: { ...current.workspace, world_assets: current.workspace.world_assets.filter((item) => item.id !== asset.id).map((item) => ({ ...item, related_asset_ids: (item.related_asset_ids ?? []).filter((id) => id !== asset.id) })) } } : current)
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setError(reason instanceof Error ? reason.message : '删除世界观资料失败')
    } finally { setSaving(false) }
  }
  return { openWorldAssetCreate, openWorldAssetEdit, saveWorldAsset, deleteWorldAsset }
}
