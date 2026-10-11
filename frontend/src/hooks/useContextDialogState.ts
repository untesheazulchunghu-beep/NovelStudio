import { useState } from 'react'

export interface CharacterDialogDraft {
  name: string
  role: string
  description: string
  traits: string
  color: string
}

export interface NoteDialogDraft {
  title: string
  body: string
  kind: string
}

export function useContextDialogState() {
  const [characterDialogOpen, setCharacterDialogOpen] = useState(false)
  const [characterEditingId, setCharacterEditingId] = useState<number | null>(null)
  const [characterDraft, setCharacterDraft] = useState<CharacterDialogDraft>({ name: '', role: '', description: '', traits: '', color: '' })
  const [noteDialogOpen, setNoteDialogOpen] = useState(false)
  const [noteEditingId, setNoteEditingId] = useState<number | null>(null)
  const [noteDraft, setNoteDraft] = useState<NoteDialogDraft>({ title: '', body: '', kind: 'idea' })
  const [savingContextItem, setSavingContextItem] = useState(false)
  const [contextDialogError, setContextDialogError] = useState('')

  return {
    characterDialogOpen,
    setCharacterDialogOpen,
    characterEditingId,
    setCharacterEditingId,
    characterDraft,
    setCharacterDraft,
    noteDialogOpen,
    setNoteDialogOpen,
    noteEditingId,
    setNoteEditingId,
    noteDraft,
    setNoteDraft,
    savingContextItem,
    setSavingContextItem,
    contextDialogError,
    setContextDialogError,
  }
}
