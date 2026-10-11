import { api } from '../api'
import type { Character, Note, WorkspaceResponse } from '../types'
import type { Dispatch, SetStateAction } from 'react'

interface CharacterDraft {
  name: string
  role: string
  description: string
  traits: string
  color: string
}

interface NoteDraft {
  title: string
  body: string
  kind: string
}

type MutableRef<T> = { current: T }

interface ContextItemActionsOptions {
  data: WorkspaceResponse | null
  setData: Dispatch<SetStateAction<WorkspaceResponse | null>>
  activeWorkspaceRef: MutableRef<string>
  projectActionBusy: boolean
  characters: Character[]
  characterColors: readonly string[]
  characterEditingId: number | null
  setCharacterEditingId: Dispatch<SetStateAction<number | null>>
  characterDraft: CharacterDraft
  setCharacterDraft: Dispatch<SetStateAction<CharacterDraft>>
  setCharacterDialogOpen: Dispatch<SetStateAction<boolean>>
  noteEditingId: number | null
  setNoteEditingId: Dispatch<SetStateAction<number | null>>
  noteDraft: NoteDraft
  setNoteDraft: Dispatch<SetStateAction<NoteDraft>>
  setNoteDialogOpen: Dispatch<SetStateAction<boolean>>
  setSavingContextItem: Dispatch<SetStateAction<boolean>>
  setContextDialogError: Dispatch<SetStateAction<string>>
  setError: Dispatch<SetStateAction<string>>
}

export function createContextItemActions(options: ContextItemActionsOptions) {
  const {
    data,
    setData,
    activeWorkspaceRef,
    projectActionBusy,
    characters,
    characterColors,
    characterEditingId,
    setCharacterEditingId,
    characterDraft,
    setCharacterDraft,
    setCharacterDialogOpen,
    noteEditingId,
    setNoteEditingId,
    noteDraft,
    setNoteDraft,
    setNoteDialogOpen,
    setSavingContextItem,
    setContextDialogError,
    setError,
  } = options

  const openCharacterCreate = () => {
    if (projectActionBusy) return
    setCharacterEditingId(null)
    setCharacterDraft({ name: '', role: '', description: '', traits: '', color: characterColors[characters.length % characterColors.length] })
    setContextDialogError('')
    setCharacterDialogOpen(true)
  }

  const openCharacterEdit = (character: Character) => {
    if (projectActionBusy) return
    setCharacterEditingId(character.id)
    setCharacterDraft({ name: character.name, role: character.role, description: character.description, traits: character.traits.join('，'), color: character.color })
    setContextDialogError('')
    setCharacterDialogOpen(true)
  }

  const saveCharacter = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || !characterDraft.name.trim() || projectActionBusy) return
    const characterId = characterEditingId
    setSavingContextItem(true)
    setContextDialogError('')
    try {
      const input = {
        name: characterDraft.name.trim(),
        role: characterDraft.role.trim(),
        description: characterDraft.description.trim(),
        traits: characterDraft.traits.split(/[，,\n]/).map((trait) => trait.trim()).filter(Boolean),
        color: characterDraft.color,
      }
      const character = characterId === null
        ? await api.character.create(workspaceId, input)
        : await api.character.update(workspaceId, characterId, input)
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        const charactersNext = characterId === null
          ? [...current.workspace.characters, character]
          : current.workspace.characters.map((item) => item.id === character.id ? character : item)
        return { ...current, workspace: { ...current.workspace, characters: charactersNext } }
      })
      if (activeWorkspaceRef.current === workspaceId) {
        setCharacterDraft({ name: '', role: '', description: '', traits: '', color: '' })
        setCharacterEditingId(null)
        setCharacterDialogOpen(false)
      }
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setContextDialogError(reason instanceof Error ? reason.message : '保存角色失败')
    } finally {
      setSavingContextItem(false)
    }
  }

  const deleteCharacter = async (character: Character) => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy || !window.confirm(`确定删除角色“${character.name}”吗？`)) return
    setSavingContextItem(true)
    try {
      await api.character.remove(workspaceId, character.id)
      setData((current) => current && current.workspace.workspace_id === workspaceId
        ? { ...current, workspace: { ...current.workspace, characters: current.workspace.characters.filter((item) => item.id !== character.id) } }
        : current)
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setError(reason instanceof Error ? reason.message : '删除角色失败')
    } finally {
      setSavingContextItem(false)
    }
  }

  const openNoteCreate = () => {
    if (projectActionBusy) return
    setNoteEditingId(null)
    setNoteDraft({ title: '', body: '', kind: 'idea' })
    setContextDialogError('')
    setNoteDialogOpen(true)
  }

  const openNoteEdit = (note: Note) => {
    if (projectActionBusy) return
    setNoteEditingId(note.id)
    setNoteDraft({ title: note.title, body: note.body, kind: note.kind || 'idea' })
    setContextDialogError('')
    setNoteDialogOpen(true)
  }

  const saveNote = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || !noteDraft.title.trim() || projectActionBusy) return
    const noteId = noteEditingId
    setSavingContextItem(true)
    setContextDialogError('')
    try {
      const input = {
        title: noteDraft.title.trim(),
        body: noteDraft.body,
        kind: noteDraft.kind,
      }
      const note = noteId === null
        ? await api.note.create(workspaceId, input)
        : await api.note.update(workspaceId, noteId, input)
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        const notesNext = noteId === null
          ? [...current.workspace.notes, note]
          : current.workspace.notes.map((item) => item.id === note.id ? note : item)
        return { ...current, workspace: { ...current.workspace, notes: notesNext } }
      })
      if (activeWorkspaceRef.current === workspaceId) {
        setNoteDraft({ title: '', body: '', kind: 'idea' })
        setNoteEditingId(null)
        setNoteDialogOpen(false)
      }
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setContextDialogError(reason instanceof Error ? reason.message : '保存便笺失败')
    } finally {
      setSavingContextItem(false)
    }
  }

  const deleteNote = async (note: Note) => {
    const workspaceId = data?.workspace.workspace_id
    if (!workspaceId || projectActionBusy || !window.confirm(`确定删除便笺“${note.title}”吗？`)) return
    setSavingContextItem(true)
    try {
      await api.note.remove(workspaceId, note.id)
      setData((current) => current && current.workspace.workspace_id === workspaceId
        ? { ...current, workspace: { ...current.workspace, notes: current.workspace.notes.filter((item) => item.id !== note.id) } }
        : current)
    } catch (reason) {
      if (activeWorkspaceRef.current === workspaceId) setError(reason instanceof Error ? reason.message : '删除便笺失败')
    } finally {
      setSavingContextItem(false)
    }
  }

  return {
    openCharacterCreate,
    openCharacterEdit,
    saveCharacter,
    deleteCharacter,
    openNoteCreate,
    openNoteEdit,
    saveNote,
    deleteNote,
  }
}
