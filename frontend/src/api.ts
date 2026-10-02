export type User = { id: string; email: string }
export type Workspace = { id: string; name: string; role: string }
export type Novel = {
  id: string
  workspaceId: string
  title: string
  summary: string
  genre: string
  plannedChapters: number
  approvalMode: 'manual' | 'auto'
  writingGuide: string
  chapterCount: number
  deletedAt?: string
}
export type Chapter = {
  id: string
  novelId: string
  number: number
  title: string
  body: string
  revision: number
  status: string
  goal: string
  keyEvents: string
  suspenseHook: string
  deletedAt?: string
  updatedAt: string
  memoryStatus?: 'missing' | 'stale' | 'current'
}
export type ChapterRevision = { revision: number; title: string; body: string; source: string; createdAt: string }
export type GraphNode = {
  id: string
  novelId: string
  kind: 'character' | 'place' | 'faction' | 'item' | 'concept'
  label: string
  summary: string
  x: number
  y: number
}
export type GraphEdge = {
  id: string
  novelId: string
  sourceId: string
  targetId: string
  relation: string
  summary: string
}
export type WorkflowNode = {
  id: string
  kind: 'context' | 'generate' | 'review' | 'output'
  title: string
  prompt: string
  x: number
  y: number
}
export type WorkflowEdge = { id: string; source: string; target: string }
export type WorkflowDefinition = { nodes: WorkflowNode[]; edges: WorkflowEdge[] }
export type Workflow = { id: string; novelId: string; name: string; definition: WorkflowDefinition }
export type Member = { id: string; email: string; role: string }
export type ModelSettings = {
  baseUrl: string
  model: string
  temperature: number
  maxTokens: number
  timeoutSec: number
  apiKeyConfigured: boolean
}
export type ModelSettingsInput = Omit<ModelSettings, 'apiKeyConfigured'> & {
  apiKey: string
  clearApiKey: boolean
}
export type ModelSettingsResponse = { settings: ModelSettings; canManage: boolean }
export type NovelTask = { id: string; novelId: string; kind: string; status: string; progress: number; error?: string }
export type ContinuityFinding = {
  chapterId: string
  chapterNumber: number
  title: string
  category: string
  subject: string
  issue: string
  currentQuote: string
  referenceSource: string
  referenceQuote: string
  confidence: string
  status: string
  currentQuoteVerbatim: boolean
  referenceQuoteVerbatim: boolean
}
export type Proposal = {
  id: string
  taskId: string
  novelId: string
  chapterId: string
  chapterNumber: number
  baseRevision: number
  title: string
  text: string
  status: string
  demoMode?: boolean
  createdAt: string
}
export type ProposalSelection = { selectedHunks: number[]; applyTitle: boolean }
export type ProposalMergePreview = {
  proposalId: string
  chapterId: string
  baseRevision: number
  currentRevision: number
  baseTitle: string
  baseText: string
  currentTitle: string
  currentText: string
  proposalTitle: string
  proposalText: string
}
export type ProposalMergeInput = { expectedRevision: number; title: string; text: string }
export type TaskEvent = {
  taskId: string
  sequence: number
  type: string
  data: Record<string, unknown>
  createdAt: string
}

export class ApiError extends Error {
  code: string

  constructor(message: string, code: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    credentials: 'same-origin',
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  const value = await response.json().catch(() => ({}))
  if (!response.ok) {
    const payload = value?.error ?? {}
    const message = typeof payload.message === 'string' ? payload.message : `请求失败 (${response.status})`
    const code = typeof payload.code === 'string' ? payload.code : ''
    throw new ApiError(message, code)
  }
  return value as T
}

const json = (body: unknown): RequestInit => ({ method: 'POST', body: JSON.stringify(body) })
const put = (body: unknown): RequestInit => ({ method: 'PUT', body: JSON.stringify(body) })

export const api = {
  me: () => request<{ user: User; workspaces: Workspace[] }>('/me'),
  login: (email: string, password: string) => request<{ user: User }>('/auth/login', json({ email, password })),
  register: (email: string, password: string) => request<{ user: User }>('/auth/register', json({ email, password })),
  logout: () => request<{ ok: boolean }>('/auth/logout', json({})),
  modelSettings: () => request<ModelSettingsResponse>('/settings/llm'),
  updateModelSettings: (input: ModelSettingsInput) => request<ModelSettings>('/settings/llm', put(input)),
  testModelSettings: (input: ModelSettingsInput) =>
    request<{ ok: boolean; message: string }>('/settings/llm/test', json(input)),
  novels: () => request<{ novels: Novel[] }>('/novels'),
  deletedNovels: () => request<{ novels: Novel[] }>('/novels/trash'),
  deleteNovel: (id: string) => request<{ ok: boolean }>(`/novels/${id}`, { method: 'DELETE' }),
  restoreNovel: (id: string) => request<{ ok: boolean }>(`/novels/${id}/restore`, json({})),
  createNovel: (
    title: string,
    summary: string,
    genre: string,
    plannedChapters: number,
    workspaceId: string,
    writingGuide = '',
  ) =>
    request<Novel>(
      '/novels',
      json({ title, summary, genre, plannedChapters, workspaceId, approvalMode: 'manual', writingGuide }),
    ),
  updateNovel: (
    id: string,
    input: Pick<Novel, 'title' | 'summary' | 'genre' | 'plannedChapters' | 'approvalMode' | 'writingGuide'>,
  ) => request<{ ok: boolean }>(`/novels/${id}`, put(input)),
  exportNovel: (id: string, format: 'txt' | 'md') => `/api/v1/novels/${id}/export?format=${format}`,
  chapters: (novelId: string) => request<{ chapters: Chapter[] }>(`/novels/${novelId}/chapters`),
  deletedChapters: (novelId: string) => request<{ chapters: Chapter[] }>(`/novels/${novelId}/chapters/trash`),
  deleteChapter: (novelId: string, chapterId: string) =>
    request<{ ok: boolean }>(`/novels/${novelId}/chapters/${chapterId}`, { method: 'DELETE' }),
  restoreChapter: (novelId: string, chapterId: string) =>
    request<{ ok: boolean }>(`/novels/${novelId}/chapters/${chapterId}/restore`, json({})),
  reorderChapters: (novelId: string, chapterIds: string[]) =>
    request<{ chapters: Chapter[] }>(`/novels/${novelId}/chapters/order`, put({ chapterIds })),
  createChapter: (novelId: string, title: string) => request<Chapter>(`/novels/${novelId}/chapters`, json({ title })),
  saveChapter: (chapter: Chapter) =>
    request<Chapter>(
      `/novels/${chapter.novelId}/chapters/${chapter.id}`,
      put({
        expectedRevision: chapter.revision,
        expectedUpdatedAt: chapter.updatedAt,
        title: chapter.title,
        body: chapter.body,
        goal: chapter.goal,
        keyEvents: chapter.keyEvents,
        suspenseHook: chapter.suspenseHook,
      }),
    ),
  refreshChapterMemory: (novelId: string, chapterId: string) =>
    request<{ memoryStatus: 'current' }>(`/novels/${novelId}/chapters/${chapterId}/memory/refresh`, json({})),
  chapterRevisions: (novelId: string, chapterId: string) =>
    request<{ revisions: ChapterRevision[] }>(`/novels/${novelId}/chapters/${chapterId}/revisions`),
  restoreChapterRevision: (novelId: string, chapterId: string, revision: number) =>
    request<Chapter>(`/novels/${novelId}/chapters/${chapterId}/revisions/${revision}/restore`, json({})),

  graph: (novelId: string) => request<{ nodes: GraphNode[]; edges: GraphEdge[] }>(`/novels/${novelId}/graph`),
  createGraphNode: (novelId: string, input: Pick<GraphNode, 'kind' | 'label' | 'summary' | 'x' | 'y'>) =>
    request<GraphNode>(`/novels/${novelId}/graph/nodes`, json(input)),
  createGraphEdge: (novelId: string, input: Pick<GraphEdge, 'sourceId' | 'targetId' | 'relation' | 'summary'>) =>
    request<GraphEdge>(`/novels/${novelId}/graph/edges`, json(input)),
  updateGraphNode: (
    novelId: string,
    nodeId: string,
    input: Pick<GraphNode, 'kind' | 'label' | 'summary' | 'x' | 'y'>,
  ) => request<GraphNode>(`/novels/${novelId}/graph/nodes/${nodeId}`, put(input)),
  deleteGraphNode: (novelId: string, nodeId: string) =>
    request<{ ok: boolean }>(`/novels/${novelId}/graph/nodes/${nodeId}`, { method: 'DELETE' }),
  updateGraphEdge: (
    novelId: string,
    edgeId: string,
    input: Pick<GraphEdge, 'sourceId' | 'targetId' | 'relation' | 'summary'>,
  ) => request<{ ok: boolean }>(`/novels/${novelId}/graph/edges/${edgeId}`, put(input)),
  deleteGraphEdge: (novelId: string, edgeId: string) =>
    request<{ ok: boolean }>(`/novels/${novelId}/graph/edges/${edgeId}`, { method: 'DELETE' }),
  workflows: (novelId: string) => request<{ workflows: Workflow[] }>(`/novels/${novelId}/workflows`),
  createWorkflow: (novelId: string, name: string, definition: Workflow['definition']) =>
    request<Workflow>(`/novels/${novelId}/workflows`, json({ name, definition })),
  createWorkflowTemplate: (novelId: string, template: string) =>
    request<Workflow>(`/novels/${novelId}/workflows`, json({ template })),
  updateWorkflow: (novelId: string, workflowId: string, name: string, definition: Workflow['definition']) =>
    request<{ ok: boolean }>(`/novels/${novelId}/workflows/${workflowId}`, put({ name, definition })),
  deleteWorkflow: (novelId: string, workflowId: string) =>
    request<{ ok: boolean }>(`/novels/${novelId}/workflows/${workflowId}`, { method: 'DELETE' }),
  runWorkflow: (novelId: string, workflowId: string, chapterIds: string[], instruction: string) =>
    request<NovelTask>(`/novels/${novelId}/workflows/${workflowId}/run`, json({ chapterIds, instruction })),
  createBatchTask: (novelId: string, chapterIds: string[], instruction: string) =>
    request<NovelTask>(`/novels/${novelId}/tasks/batch`, json({ chapterIds, instruction })),
  createReviewTask: (novelId: string, chapterIds: string[], instruction: string) =>
    request<NovelTask>(`/novels/${novelId}/tasks/batch`, json({ chapterIds, instruction, kind: 'review' })),
  createContinuityTask: (novelId: string, chapterIds: string[], instruction: string) =>
    request<NovelTask>(`/novels/${novelId}/tasks/batch`, json({ chapterIds, instruction, kind: 'continuity' })),
  tasks: (novelId: string) => request<{ tasks: NovelTask[] }>(`/novels/${novelId}/tasks`),
  task: (taskId: string) => request<NovelTask>(`/tasks/${taskId}`),
  cancelTask: (taskId: string) => request<{ ok: boolean }>(`/tasks/${taskId}/cancel`, json({})),
  retryTask: (taskId: string) => request<NovelTask>(`/tasks/${taskId}/retry`, json({})),

  proposals: (novelId: string) => request<{ proposals: Proposal[] }>(`/novels/${novelId}/proposals`),
  applyProposal: (novelId: string, proposalId: string, selection?: ProposalSelection) =>
    request<{ ok: boolean; appliedHunks?: number; appliedTitle?: boolean }>(
      `/novels/${novelId}/proposals/${proposalId}/apply`,
      json(selection ?? {}),
    ),
  proposalMergePreview: (novelId: string, proposalId: string) =>
    request<ProposalMergePreview>(`/novels/${novelId}/proposals/${proposalId}/merge-preview`),
  applyProposalMerge: (novelId: string, proposalId: string, input: ProposalMergeInput) =>
    request<{ ok: boolean; revision: number }>(`/novels/${novelId}/proposals/${proposalId}/merge`, json(input)),
  rejectProposal: (novelId: string, proposalId: string) =>
    request<{ ok: boolean }>(`/novels/${novelId}/proposals/${proposalId}/reject`, json({})),
  regenerateStaleProposal: (novelId: string, proposalId: string) =>
    request<NovelTask>(`/novels/${novelId}/proposals/${proposalId}/regenerate`, json({})),
  members: (workspaceId: string) => request<{ members: Member[] }>(`/workspaces/${workspaceId}/members`),
  addMember: (workspaceId: string, email: string, role: string) =>
    request<Member>(`/workspaces/${workspaceId}/members`, json({ email, role })),
  updateMember: (workspaceId: string, userId: string, role: string) =>
    request<{ ok: boolean }>(`/workspaces/${workspaceId}/members/${userId}`, put({ role })),
  removeMember: (workspaceId: string, userId: string) =>
    request<{ ok: boolean }>(`/workspaces/${workspaceId}/members/${userId}`, { method: 'DELETE' }),
}

export function connectTask(taskId: string, after: number, onEvent: (event: TaskEvent) => void, onClose: () => void) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/tasks/${taskId}/ws?after=${after}`)
  socket.onmessage = (message) => {
    try {
      onEvent(JSON.parse(message.data) as TaskEvent)
    } catch {
      onClose()
    }
  }
  socket.onclose = onClose
  socket.onerror = onClose
  return socket
}
