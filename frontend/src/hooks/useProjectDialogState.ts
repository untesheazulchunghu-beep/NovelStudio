import { useState } from 'react'

export interface ProjectDialogDraft {
  title: string
  subtitle: string
  genre: string
  premise: string
  word_target: number
}

export function useProjectDialogState() {
  const [projectDialogOpen, setProjectDialogOpen] = useState(false)
  const [projectTitle, setProjectTitle] = useState('')
  const [projectGenre, setProjectGenre] = useState('')
  const [projectPremise, setProjectPremise] = useState('')
  const [creatingProject, setCreatingProject] = useState(false)
  const [projectEditDialogOpen, setProjectEditDialogOpen] = useState(false)
  const [projectDraft, setProjectDraft] = useState<ProjectDialogDraft>({ title: '', subtitle: '', genre: '', premise: '', word_target: 120000 })
  const [savingProject, setSavingProject] = useState(false)
  const [projectEditError, setProjectEditError] = useState('')
  const [projectDeleteDialogOpen, setProjectDeleteDialogOpen] = useState(false)
  const [projectDeleteInput, setProjectDeleteInput] = useState('')
  const [projectDeleteError, setProjectDeleteError] = useState('')
  const [deletingProject, setDeletingProject] = useState(false)

  return {
    projectDialogOpen,
    setProjectDialogOpen,
    projectTitle,
    setProjectTitle,
    projectGenre,
    setProjectGenre,
    projectPremise,
    setProjectPremise,
    creatingProject,
    setCreatingProject,
    projectEditDialogOpen,
    setProjectEditDialogOpen,
    projectDraft,
    setProjectDraft,
    savingProject,
    setSavingProject,
    projectEditError,
    setProjectEditError,
    projectDeleteDialogOpen,
    setProjectDeleteDialogOpen,
    projectDeleteInput,
    setProjectDeleteInput,
    projectDeleteError,
    setProjectDeleteError,
    deletingProject,
    setDeletingProject,
  }
}
