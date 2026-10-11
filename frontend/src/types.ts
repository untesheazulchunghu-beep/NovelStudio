export type ChapterStatus = 'outline' | 'draft' | 'review' | 'done'

export interface Project {
  id: number
  title: string
  subtitle: string
  genre: string
  premise: string
  author: string
  status: string
  word_target: number
  current_view: string
  created_at: string
  updated_at: string
}

export interface ProjectSummary {
  workspace_id: string
  project: Project
  stats: Stats
}

export interface ProjectInput {
  title: string
  subtitle?: string
  genre?: string
  premise?: string
  word_target?: number
}

export interface Chapter {
  id: number
  number: number
  title: string
  summary: string
  content?: string
  status: ChapterStatus | string
  word_count: number
  last_edited_at: string
}

export interface ChapterSearchResult extends Omit<Chapter, 'content'> {
  excerpt: string
}

export interface ChapterRevisionSummary {
  id: number
  chapter_id: number
  label: string
  title: string
  summary: string
  status: ChapterStatus | string
  word_count: number
  created_at: string
}

export interface ChapterRevision extends ChapterRevisionSummary {
  content: string
}

export interface Character {
  id: number
  name: string
  role: string
  description: string
  traits: string[]
  color: string
}

export interface Note {
  id: number
  title: string
  body: string
  kind: string
  updated_at: string
}

export type TimelineEventStatus = 'planned' | 'active' | 'done'

export interface TimelineEvent {
  id: number
  title: string
  time_label: string
  description: string
  status: TimelineEventStatus | string
  chapter_id?: number
  updated_at: string
}

export type WorldAssetKind = 'place' | 'organization' | 'item' | 'concept'

export interface WorldAsset {
  id: number
  name: string
  kind: WorldAssetKind | string
  description: string
  details: string
  chapter_ids?: number[]
  related_asset_ids?: number[]
  updated_at: string
}

export interface ChatContextItems {
  character_ids: number[]
  note_ids: number[]
  timeline_event_ids: number[]
  world_asset_ids: number[]
}

export interface ChatMessage {
  id?: number
  request_id?: string
  role: 'user' | 'assistant' | 'system'
  content: string
  created_at?: string
  mode?: string
}

export interface Settings {
  provider: string
  base_url: string
  base_url_credentials_configured: boolean
  model: string
  temperature: number
  api_key_configured: boolean
}

export interface SettingsDraft extends Settings {
  api_key?: string
  clear_api_key?: boolean
  clear_base_url_credentials?: boolean
}

export interface SettingsUpdate {
  provider: string
  base_url: string
  api_key?: string
  clear_base_url_credentials?: boolean
  model: string
  temperature: number
}

export interface Stats {
  word_count: number
  word_target: number
  chapter_count: number
  finished_count: number
  completion_rate: number
}

export interface Workspace {
  workspace_id: string
  project: Project
  chapters: Chapter[]
  characters: Character[]
  notes: Note[]
  timeline: TimelineEvent[]
  world_assets: WorldAsset[]
  chat: ChatMessage[]
  settings: Settings
  updated_at: string
}

export interface WorkspaceResponse {
  workspace: Workspace
  stats: Stats
}

export interface ChatResponse {
  user_message?: ChatMessage
  message: ChatMessage
  suggestions: string[]
  provider: string
}
