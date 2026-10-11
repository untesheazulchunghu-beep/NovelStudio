import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ChatContextItems, Character, Note, TimelineEvent, WorldAsset } from '../types'

export type ContextKind = 'character_ids' | 'note_ids' | 'timeline_event_ids' | 'world_asset_ids'

interface StoredSelection {
  workspaceId: string
  items: ChatContextItems
}

function storageKey(workspaceId: string) {
  return `novelstudio.agentContext.${workspaceId}`
}

function defaultSelection(characters: Character[], notes: Note[]): ChatContextItems {
  return {
    character_ids: characters.map((item) => item.id),
    note_ids: notes.map((item) => item.id),
    timeline_event_ids: [],
    world_asset_ids: [],
  }
}

function readSelection(workspaceId: string, characters: Character[], notes: Note[], timeline: TimelineEvent[], worldAssets: WorldAsset[]): ChatContextItems {
  const fallback = defaultSelection(characters, notes)
  if (typeof window === 'undefined' || !workspaceId) return fallback
  try {
    const raw = window.localStorage.getItem(storageKey(workspaceId))
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as Partial<ChatContextItems>
    const available = {
      character_ids: new Set(characters.map((item) => item.id)),
      note_ids: new Set(notes.map((item) => item.id)),
      timeline_event_ids: new Set(timeline.map((item) => item.id)),
      world_asset_ids: new Set(worldAssets.map((item) => item.id)),
    }
    const normalize = (values: unknown, ids: Set<number>) => Array.isArray(values)
      ? values.filter((value): value is number => typeof value === 'number' && Number.isInteger(value) && ids.has(value))
      : []
    return {
      character_ids: normalize(parsed.character_ids, available.character_ids),
      note_ids: normalize(parsed.note_ids, available.note_ids),
      timeline_event_ids: normalize(parsed.timeline_event_ids, available.timeline_event_ids),
      world_asset_ids: normalize(parsed.world_asset_ids, available.world_asset_ids),
    }
  } catch {
    return fallback
  }
}

export function useAgentContextSelection(workspaceId: string, characters: Character[], notes: Note[], timeline: TimelineEvent[], worldAssets: WorldAsset[]) {
  const [stored, setStored] = useState<StoredSelection>({ workspaceId: '', items: defaultSelection([], []) })

  useEffect(() => {
    if (!workspaceId) return
    setStored({ workspaceId, items: readSelection(workspaceId, characters, notes, timeline, worldAssets) })
  }, [workspaceId])

  useEffect(() => {
    if (!stored.workspaceId || stored.workspaceId !== workspaceId || typeof window === 'undefined') return
    try {
      window.localStorage.setItem(storageKey(workspaceId), JSON.stringify(stored.items))
    } catch {
      // Preferences are optional when storage is unavailable.
    }
  }, [stored, workspaceId])

  const items = stored.workspaceId === workspaceId ? stored.items : defaultSelection(characters, notes)
  const selectedSets = useMemo(() => ({
    character_ids: new Set(items.character_ids),
    note_ids: new Set(items.note_ids),
    timeline_event_ids: new Set(items.timeline_event_ids),
    world_asset_ids: new Set(items.world_asset_ids),
  }), [items])

  const toggle = useCallback((kind: ContextKind, id: number) => {
    setStored((current) => {
      if (current.workspaceId !== workspaceId) return current
      const values = current.items[kind]
      const nextValues = values.includes(id) ? values.filter((value) => value !== id) : [...values, id]
      return { ...current, items: { ...current.items, [kind]: nextValues } }
    })
  }, [workspaceId])

  const setAll = useCallback((kind: ContextKind, enabled: boolean) => {
    setStored((current) => {
      if (current.workspaceId !== workspaceId) return current
      const source = kind === 'character_ids' ? characters : kind === 'note_ids' ? notes : kind === 'timeline_event_ids' ? timeline : worldAssets
      return { ...current, items: { ...current.items, [kind]: enabled ? source.map((item) => item.id) : [] } }
    })
  }, [characters, notes, timeline, worldAssets, workspaceId])

  return { items, selectedSets, toggle, setAll }
}
