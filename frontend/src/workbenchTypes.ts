export type ActivityId = 'explorer' | 'chapters' | 'characters' | 'notes' | 'settings'

export type AssistantTab = 'chat' | 'outline' | 'context'

export type WritingMode = 'write' | 'plan' | 'review' | 'edit'

export type ChatRequestRetry = { signature: string; requestID: string; message: string; mode: WritingMode; safeToRetry: boolean; requiresReview: boolean; requiresFreshRequestID: boolean; needsReconcile: boolean }

export type PaletteResultKind = 'chapter' | 'character' | 'note' | 'command'

export interface EditorSelection {
  chapterId: number
  start: number
  end: number
}

export interface ChatApplyTarget extends EditorSelection {
  workspaceId: string
  messageId: number
  original: string
  replacement: string
  generatedReplacement: string
  context: string
}

export interface PaletteResult {
  id: string
  kind: PaletteResultKind
  title: string
  detail: string
  searchable: string
  entityId?: number
  disabled?: boolean
}
