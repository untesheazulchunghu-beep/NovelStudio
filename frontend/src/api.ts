import type { Chapter, ChapterRevision, ChapterRevisionSummary, ChapterSearchResult, ChatContextItems, ChatResponse, Character, Note, ProjectInput, ProjectSummary, Settings, SettingsUpdate, TimelineEvent, WorldAsset, WorkspaceResponse } from './types'

export class ApiError extends Error {
  constructor(readonly status: number, message: string, readonly code?: string) {
    super(message)
    this.name = 'ApiError'
  }
}

type ChatStreamOptions = {
  signal?: AbortSignal
  onDelta?: (text: string) => void
}

function chatBody(requestID: string, message: string, mode: string, chapterId: number, context: string, selection: string, chapterTitle: string, chapterSummary: string, contextItems: ChatContextItems) {
  return JSON.stringify({ request_id: requestID, message, mode, chapter_id: chapterId, context, selection, chapter_title: chapterTitle, chapter_summary: chapterSummary, context_items: contextItems })
}

async function request<T>(path: string, init: RequestInit = {}, workspaceID?: string): Promise<T> {
  const headers = new Headers(init.headers)
  if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (workspaceID) headers.set('X-Workspace-ID', workspaceID)
  const response = await fetch(`/api/${path}`, {
    ...init,
    headers,
  })
  const payload = await response.json().catch((reason) => {
    if (init.signal?.aborted) throw reason
    if (response.ok) throw new Error('Invalid JSON response from server')
    return {}
  })
  if (!response.ok) {
    throw new ApiError(response.status, payload.error || `请求失败（${response.status}）`, typeof payload.code === 'string' ? payload.code : undefined)
  }
  return payload as T
}

async function requestBlob(path: string, workspaceID?: string, init: RequestInit = {}): Promise<Blob> {
  const headers = new Headers(init.headers)
  if (workspaceID) headers.set('X-Workspace-ID', workspaceID)
  if (init.body !== undefined && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(`/api/${path}`, { ...init, headers })
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}))
    throw new Error(payload.error || `Request failed (${response.status})`)
  }
  return response.blob()
}

export const api = {
  projects: {
    list: () => request<{ projects: ProjectSummary[] }>('projects'),
    create: (input: ProjectInput) => request<ProjectSummary>('projects', { method: 'POST', body: JSON.stringify(input) }),
    importBackup: async (file: File) => {
      try {
        return await request<ProjectSummary>('projects/import', { method: 'POST', body: file })
      } catch (reason) {
        if (reason instanceof Error && (
          reason.message.startsWith('invalid JSON:') ||
          reason.message === 'request body is empty' ||
          reason.message === 'request body must contain only one JSON value'
        )) {
          throw new Error('备份文件无法解析，请选择有效的 NovelStudio 项目 JSON 备份。')
        }
        throw reason
      }
    },
    update: (workspaceID: string, input: ProjectInput) => request<ProjectSummary>(`projects/${encodeURIComponent(workspaceID)}`, { method: 'PUT', body: JSON.stringify(input) }),
    remove: (workspaceID: string) => request<{ deleted: boolean; workspace_id: string }>(`projects/${encodeURIComponent(workspaceID)}`, { method: 'DELETE' }),
  },
  workspace: Object.assign(
    (workspaceID: string, signal?: AbortSignal) => request<WorkspaceResponse>('workspace', signal ? { signal } : undefined, workspaceID),
    {
      backup: (workspaceID: string) => requestBlob('workspace/backup', workspaceID),
      markdown: (workspaceID: string, chapter?: Pick<Chapter, 'id' | 'title' | 'content'>) => requestBlob('workspace/export/markdown', workspaceID, {
        method: 'POST',
        body: JSON.stringify({
          chapter_id: chapter?.id ?? 0,
          chapter_title: chapter?.title ?? '',
          chapter_content: chapter?.content ?? '',
        }),
      }),
      docx: (workspaceID: string, chapter?: Pick<Chapter, 'id' | 'title' | 'content'>) => requestBlob('workspace/export/docx', workspaceID, {
        method: 'POST',
        body: JSON.stringify({
          chapter_id: chapter?.id ?? 0,
          chapter_title: chapter?.title ?? '',
          chapter_content: chapter?.content ?? '',
        }),
      }),
      epub: (workspaceID: string, chapter?: Pick<Chapter, 'id' | 'title' | 'content'>) => requestBlob('workspace/export/epub', workspaceID, {
        method: 'POST',
        body: JSON.stringify({
          chapter_id: chapter?.id ?? 0,
          chapter_title: chapter?.title ?? '',
          chapter_content: chapter?.content ?? '',
        }),
      }),
    },
  ),
  chapter: {
    get: (workspaceID: string, id: number, signal?: AbortSignal) =>
      request<Chapter>(`chapters/${id}`, signal ? { signal } : undefined, workspaceID),
    search: (workspaceID: string, query: string, signal?: AbortSignal) =>
      request<{ chapters: ChapterSearchResult[] }>(`chapters/search?q=${encodeURIComponent(query)}`, signal ? { signal } : undefined, workspaceID),
    create: (workspaceID: string, input: Pick<Chapter, 'title' | 'summary' | 'content' | 'status'>) =>
      request<Chapter>('chapters', { method: 'POST', body: JSON.stringify(input) }, workspaceID),
    update: (workspaceID: string, id: number, input: Pick<Chapter, 'title' | 'summary' | 'content' | 'status'>) =>
      request<Chapter>(`chapters/${id}`, { method: 'PUT', body: JSON.stringify(input) }, workspaceID),
    remove: (workspaceID: string, id: number) => request<{ deleted: boolean }>(`chapters/${id}`, { method: 'DELETE' }, workspaceID),
    revisions: (workspaceID: string, id: number) =>
      request<{ revisions: ChapterRevisionSummary[] }>(`chapters/${id}/revisions`, undefined, workspaceID),
    revision: (workspaceID: string, chapterID: number, revisionID: number) =>
      request<ChapterRevision>(`chapters/${chapterID}/revisions/${revisionID}`, undefined, workspaceID),
    createRevision: (workspaceID: string, id: number, label: string) =>
      request<ChapterRevision>(`chapters/${id}/revisions`, { method: 'POST', body: JSON.stringify({ label }) }, workspaceID),
    restoreRevision: (workspaceID: string, chapterID: number, revisionID: number) =>
      request<Chapter>(`chapters/${chapterID}/revisions/${revisionID}/restore`, { method: 'POST', body: '{}' }, workspaceID),
    reorder: (workspaceID: string, chapterIDs: number[]) =>
      request<{ chapter_ids: number[] }>('chapters/order', { method: 'PUT', body: JSON.stringify({ chapter_ids: chapterIDs }) }, workspaceID),
  generate: (workspaceID: string, id: number, instruction: string, context: string, chapterTitle: string, chapterSummary: string, contextItems: ChatContextItems, signal?: AbortSignal) =>
    request<{ text: string; provider: string }>(`chapters/${id}/generate`, {
      method: 'POST',
      body: JSON.stringify({ instruction, context, chapter_title: chapterTitle, chapter_summary: chapterSummary, context_items: contextItems }),
      signal,
    }, workspaceID),
  },
  chat: (workspaceID: string, requestID: string, message: string, mode: string, chapterId: number, context: string, selection: string, chapterTitle: string, chapterSummary: string, contextItems: ChatContextItems, signal?: AbortSignal) =>
    request<ChatResponse>('chat', {
      method: 'POST',
      body: chatBody(requestID, message, mode, chapterId, context, selection, chapterTitle, chapterSummary, contextItems),
      signal,
    }, workspaceID),
  chatStream: async (workspaceID: string, requestID: string, message: string, mode: string, chapterId: number, context: string, selection: string, chapterTitle: string, chapterSummary: string, contextItems: ChatContextItems, options: ChatStreamOptions = {}) => {
    const headers = new Headers({ 'Content-Type': 'application/json', Accept: 'text/event-stream' })
    if (workspaceID) headers.set('X-Workspace-ID', workspaceID)
    const response = await fetch('/api/chat', {
      method: 'POST',
      headers,
      body: chatBody(requestID, message, mode, chapterId, context, selection, chapterTitle, chapterSummary, contextItems),
      signal: options.signal,
    })
    if (!response.ok) {
      const payload = await response.json().catch(() => ({})) as { error?: unknown; code?: unknown }
      throw new ApiError(response.status, typeof payload.error === 'string' ? payload.error : `请求失败（${response.status}）`, typeof payload.code === 'string' ? payload.code : undefined)
    }
    if (!response.body) throw new Error('Agent 没有返回流式内容')

    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let eventName = ''
    let dataLines: string[] = []
    let completed: ChatResponse | undefined
    let streamError: ApiError | undefined

    const processEvent = () => {
      if (!eventName && dataLines.length === 0) return
      const event = eventName || 'message'
      const rawData = dataLines.join('\n')
      eventName = ''
      dataLines = []
      if (!rawData) return
      if (event === 'delta') {
        let payload: { text?: unknown }
        try {
          payload = JSON.parse(rawData) as { text?: unknown }
        } catch {
          throw new Error('Agent 流式响应格式无效')
        }
        if (typeof payload.text === 'string' && payload.text) options.onDelta?.(payload.text)
        return
      }
      if (event === 'complete') {
        try {
          completed = JSON.parse(rawData) as ChatResponse
        } catch {
          throw new Error('Agent 完成响应格式无效')
        }
        return
      }
      if (event === 'error') {
        let payload: { error?: unknown; status?: unknown; code?: unknown }
        try {
          payload = JSON.parse(rawData) as { error?: unknown; status?: unknown; code?: unknown }
        } catch {
          throw new Error('Agent 错误响应格式无效')
        }
        const status = typeof payload.status === 'number' ? payload.status : 502
        streamError = new ApiError(status, typeof payload.error === 'string' ? payload.error : 'Agent 暂时无法响应', typeof payload.code === 'string' ? payload.code : undefined)
      }
    }

    try {
      while (true) {
        const result = await reader.read()
        buffer += decoder.decode(result.value ?? new Uint8Array(), { stream: !result.done })
        let newlineIndex = buffer.indexOf('\n')
        while (newlineIndex >= 0) {
          const line = buffer.slice(0, newlineIndex).replace(/\r$/, '')
          buffer = buffer.slice(newlineIndex + 1)
          if (line === '') processEvent()
          else if (line.startsWith('event:')) eventName = line.slice(6).trim()
          else if (line.startsWith('data:')) dataLines.push(line.slice(5).trimStart())
          newlineIndex = buffer.indexOf('\n')
        }
        if (result.done) break
      }
      if (buffer.trim() !== '') {
        if (buffer.startsWith('data:')) dataLines.push(buffer.slice(5).trimStart())
        processEvent()
      } else if (dataLines.length > 0 || eventName) {
        processEvent()
      }
    } finally {
      reader.releaseLock()
    }
    if (streamError) throw streamError
    if (!completed) throw new Error('Agent 流式响应未正常结束')
    return completed
  },
  settings: {
    update: (workspaceID: string, settings: SettingsUpdate) => request<Settings>('settings', { method: 'PUT', body: JSON.stringify(settings) }, workspaceID),
  },
  character: {
    create: (workspaceID: string, input: Pick<Character, 'name' | 'role' | 'description' | 'traits' | 'color'>) =>
      request<Character>('characters', { method: 'POST', body: JSON.stringify(input) }, workspaceID),
    update: (workspaceID: string, id: number, input: Pick<Character, 'name' | 'role' | 'description' | 'traits' | 'color'>) =>
      request<Character>(`characters/${id}`, { method: 'PUT', body: JSON.stringify(input) }, workspaceID),
    remove: (workspaceID: string, id: number) => request<{ deleted: boolean }>(`characters/${id}`, { method: 'DELETE' }, workspaceID),
  },
  note: {
    create: (workspaceID: string, input: Pick<Note, 'title' | 'body' | 'kind'>) =>
      request<Note>('notes', { method: 'POST', body: JSON.stringify(input) }, workspaceID),
    update: (workspaceID: string, id: number, input: Pick<Note, 'title' | 'body' | 'kind'>) =>
      request<Note>(`notes/${id}`, { method: 'PUT', body: JSON.stringify(input) }, workspaceID),
    remove: (workspaceID: string, id: number) => request<{ deleted: boolean }>(`notes/${id}`, { method: 'DELETE' }, workspaceID),
  },
  timeline: {
    create: (workspaceID: string, input: Pick<TimelineEvent, 'title' | 'time_label' | 'description' | 'status' | 'chapter_id'>) =>
      request<TimelineEvent>('timeline', { method: 'POST', body: JSON.stringify(input) }, workspaceID),
    update: (workspaceID: string, id: number, input: Pick<TimelineEvent, 'title' | 'time_label' | 'description' | 'status' | 'chapter_id'>) =>
      request<TimelineEvent>(`timeline/${id}`, { method: 'PUT', body: JSON.stringify(input) }, workspaceID),
    remove: (workspaceID: string, id: number) => request<{ deleted: boolean }>(`timeline/${id}`, { method: 'DELETE' }, workspaceID),
  },
  worldAsset: {
    create: (workspaceID: string, input: Pick<WorldAsset, 'name' | 'kind' | 'description' | 'details' | 'chapter_ids' | 'related_asset_ids'>) =>
      request<WorldAsset>('world-assets', { method: 'POST', body: JSON.stringify(input) }, workspaceID),
    update: (workspaceID: string, id: number, input: Pick<WorldAsset, 'name' | 'kind' | 'description' | 'details' | 'chapter_ids' | 'related_asset_ids'>) =>
      request<WorldAsset>(`world-assets/${id}`, { method: 'PUT', body: JSON.stringify(input) }, workspaceID),
    remove: (workspaceID: string, id: number) => request<{ deleted: boolean }>(`world-assets/${id}`, { method: 'DELETE' }, workspaceID),
  },
}
