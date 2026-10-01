export type ChapterDraft = {
  title: string
  body: string
  goal: string
  keyEvents: string
  suspenseHook: string
}

export type RemoteChapterSnapshot = ChapterDraft & { revision: number }

export type SaveConflictRecord = {
  remote: RemoteChapterSnapshot
  baseRevision: number
}

export type FailedSavePlan = {
  retainLocalDraft: true
  local: ChapterDraft
  recordConflict: boolean
  overwriteServer: false
  conflict?: SaveConflictRecord
}

export type ConflictResolution =
  { overwriteServer: false; local: ChapterDraft; remote: ChapterDraft } | { overwriteServer: true; local: ChapterDraft }

export function draftStorageKey(chapterId: string, revision: number) {
  return `novelstudio:draft:${chapterId}:${revision}`
}

export function conflictStorageKey(chapterId: string) {
  return `novelstudio:conflict:${chapterId}`
}

export function chapterDraftFrom(chapter: ChapterDraft): ChapterDraft {
  return {
    title: chapter.title,
    body: chapter.body,
    goal: chapter.goal,
    keyEvents: chapter.keyEvents,
    suspenseHook: chapter.suspenseHook,
  }
}

const staleChapterSaveMessages = [
  '章节已被其他人更新，请刷新后再保存',
  '章节蓝图或正文已被其他人更新，请比较后再保存',
  '章节已被其他人更新，请比较后再保存',
]

// Only a stale revision is a conflict. Messages that merely mention 版本,
// such as an invalid title or a failed revision insert, keep the local draft.
export function isStaleChapterSave(code: string, message: string) {
  return code === 'STALE_REVISION' || staleChapterSaveMessages.includes(message)
}

// Selecting a chapter loads the cache for that chapter id and revision.
// A different draft replaces the server text; a matching or unreadable cache leaves the server text.
export function restoreChapterDraft(
  server: ChapterDraft,
  cachedRaw: string | null,
): { draft: ChapterDraft; restored: boolean } {
  const draft = chapterDraftFrom(server)
  if (!cachedRaw) return { draft, restored: false }
  try {
    const saved = JSON.parse(cachedRaw) as Partial<ChapterDraft>
    const differs =
      saved.body !== server.body ||
      saved.title !== server.title ||
      saved.goal !== server.goal ||
      saved.keyEvents !== server.keyEvents ||
      saved.suspenseHook !== server.suspenseHook
    if (!differs) return { draft, restored: false }
    return {
      restored: true,
      draft: {
        title: saved.title ?? draft.title,
        body: saved.body ?? draft.body,
        goal: saved.goal ?? draft.goal,
        keyEvents: saved.keyEvents ?? draft.keyEvents,
        suspenseHook: saved.suspenseHook ?? draft.suspenseHook,
      },
    }
  } catch {
    return { draft, restored: false }
  }
}

// A failed save never writes the server chapter. Non-conflict failures keep the local draft.
// A revision conflict also keeps that draft and the remote snapshot for an explicit later choice.
export function planFailedChapterSave(
  local: ChapterDraft,
  message: string,
  remote: RemoteChapterSnapshot | null,
  baseRevision: number,
  code = '',
): FailedSavePlan {
  const recordConflict = isStaleChapterSave(code, message) && remote !== null
  const plan: FailedSavePlan = {
    retainLocalDraft: true,
    local,
    recordConflict,
    overwriteServer: false,
  }
  if (recordConflict && remote) plan.conflict = { remote, baseRevision }
  return plan
}

// The server chapter stays unchanged until the writer confirms the overwrite.
export function planConflictResolution(
  local: ChapterDraft,
  remote: ChapterDraft,
  confirmOverwrite: boolean,
): ConflictResolution {
  if (!confirmOverwrite) return { overwriteServer: false, local, remote }
  return { overwriteServer: true, local }
}
