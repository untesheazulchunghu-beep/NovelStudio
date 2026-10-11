import { useState } from 'react'
import type { WorldAssetKind } from '../types'

export interface WorldAssetDialogDraft {
  name: string
  kind: WorldAssetKind
  description: string
  details: string
  chapter_ids: number[]
  related_asset_ids: number[]
}

export const emptyWorldAssetDialogDraft: WorldAssetDialogDraft = {
  name: '', kind: 'place', description: '', details: '', chapter_ids: [], related_asset_ids: [],
}

export function useWorldAssetDialogState() {
  const [worldAssetDialogOpen, setWorldAssetDialogOpen] = useState(false)
  const [worldAssetEditingId, setWorldAssetEditingId] = useState<number | null>(null)
  const [worldAssetDraft, setWorldAssetDraft] = useState<WorldAssetDialogDraft>(emptyWorldAssetDialogDraft)
  const [savingWorldAsset, setSavingWorldAsset] = useState(false)
  const [worldAssetDialogError, setWorldAssetDialogError] = useState('')
  return {
    worldAssetDialogOpen, setWorldAssetDialogOpen, worldAssetEditingId, setWorldAssetEditingId,
    worldAssetDraft, setWorldAssetDraft, savingWorldAsset, setSavingWorldAsset,
    worldAssetDialogError, setWorldAssetDialogError,
  }
}
