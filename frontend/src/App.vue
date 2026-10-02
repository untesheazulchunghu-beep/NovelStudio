<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  ApiError,
  api,
  connectTask,
  type Chapter,
  type ChapterRevision,
  type GraphEdge,
  type GraphNode,
  type Member,
  type Novel,
  type NovelTask,
  type Proposal,
  type ProposalSelection,
  type TaskEvent,
  type User,
  type Workflow,
  type WorkflowDefinition,
  type Workspace,
} from './api'
import ChapterEditorPage from './components/ChapterEditorPage.vue'
import KnowledgeGraphPage from './components/KnowledgeGraphPage.vue'
import LibraryPage from './components/LibraryPage.vue'
import LoginPage from './components/LoginPage.vue'
import MembersPage from './components/MembersPage.vue'
import ModelSettingsPage from './components/ModelSettingsPage.vue'
import ProposalReviewPanel from './components/ProposalReviewPanel.vue'
import TaskMonitorPanel from './components/TaskMonitorPanel.vue'
import TrashPage from './components/TrashPage.vue'
import WorkflowPage from './components/WorkflowPage.vue'
import {
  chapterDraftFrom,
  conflictStorageKey,
  draftStorageKey,
  isStaleChapterSave,
  planConflictResolution,
  planFailedChapterSave,
  restoreChapterDraft,
  type ChapterDraft,
} from './draft'
import { countManuscriptCharacters } from './word-count'

const user = ref<User | null>(null)
const workspaces = ref<Workspace[]>([])
const novels = ref<Novel[]>([])
const deletedNovels = ref<Novel[]>([])
const activeNovel = ref<Novel | null>(null)
const chapters = ref<Chapter[]>([])
const deletedChapters = ref<Chapter[]>([])
const activeChapterId = ref('')
const section = ref<'library' | 'write' | 'knowledge' | 'workflow' | 'team' | 'trash' | 'settings'>('library')
const loading = ref(false)
const message = ref('')
const errorMessage = ref('')
const authMode = ref<'login' | 'register'>('register')
const authEmail = ref('')
const authPassword = ref('')
const authBusy = ref(false)
const showCreate = ref(false)
const novelTitle = ref('')
const novelGenre = ref('')
const novelSummary = ref('')
const plannedChapters = ref(24)
const draftTitle = ref('')
const draftBody = ref('')
const draftGoal = ref('')
const draftKeyEvents = ref('')
const draftHook = ref('')
type ChapterSaveConflict = { remote: Chapter; baseRevision: number }
const saveConflicts = ref<Record<string, ChapterSaveConflict>>({})
const autoSaveState = ref<'saved' | 'pending' | 'saving' | 'error'>('saved')
const showBlueprint = ref(false)
const guideDraft = ref('')
const savingGuide = ref(false)
const savingChapter = ref(false)
const refreshingMemory = ref(false)
const chapterRevisions = ref<ChapterRevision[]>([])
const showHistory = ref(false)
const loadingHistory = ref(false)
const nodes = ref<GraphNode[]>([])
const edges = ref<GraphEdge[]>([])
const graphForm = ref({ kind: 'character' as GraphNode['kind'], label: '', summary: '' })
const editingNodeId = ref('')
const edgeForm = ref({ sourceId: '', targetId: '', relation: '', summary: '' })
const editingEdgeId = ref('')
const workflows = ref<Workflow[]>([])
const activeWorkflowId = ref('')
const workflowName = ref('')
const workflowDefinition = ref<WorkflowDefinition>({ nodes: [], edges: [] })
const workflowChapterIds = ref<string[]>([])
const runInstruction = ref('')
const members = ref<Member[]>([])
const memberEmail = ref('')
const memberRole = ref('editor')
const selectedChapterIds = ref<string[]>([])
const proposals = ref<Proposal[]>([])
const tasks = ref<NovelTask[]>([])
const activeTask = ref<NovelTask | null>(null)
const taskProgress = ref(0)
const chapterStates = ref<Record<string, { chapterId: string; chapterNumber: number; title: string; status: string }>>(
  {},
)
const chapterProgress = computed(() =>
  Object.values(chapterStates.value).sort((a, b) => a.chapterNumber - b.chapterNumber),
)
const nodeProgress = ref<Array<{ nodeId: string; title: string; status: string }>>([])
const reviewItems = ref<
  Array<{
    event: string
    status: string
    quote: string
    location: string
    modelJudgment: string
    quoteVerbatim: boolean
  }>
>([])
const retryingTask = ref(false)
const demoMode = ref(false)
const streamText = ref('')
const taskContextSources = ref<string[]>([])
const taskDroppedContext = ref<string[]>([])
const contextTruncation = ref({ entities: 0, relations: 0 })
const lastSequence = ref(0)
let taskSocket: WebSocket | null = null
let reconnectTimer = 0
let draftSaveTimer = 0
let autoSaveTimer = 0
const beforeUnloadHandler = (event: BeforeUnloadEvent) => {
  if (hasUnsavedChanges()) {
    cacheDraft()
    event.preventDefault()
    event.returnValue = ''
  }
}

const activeChapter = computed(() => chapters.value.find((chapter) => chapter.id === activeChapterId.value) ?? null)
const activeSaveConflict = computed(() =>
  activeChapter.value ? (saveConflicts.value[activeChapter.value.id] ?? null) : null,
)
const canEdit = computed(() => activeWorkspace.value?.role === 'owner' || activeWorkspace.value?.role === 'editor')
const canReview = computed(() => canEdit.value || activeWorkspace.value?.role === 'reviewer')
const activeWorkspace = computed(
  () =>
    workspaces.value.find((workspace) => workspace.id === activeNovel.value?.workspaceId) ??
    workspaces.value[0] ??
    null,
)
const canManageNovel = (novel: Novel) =>
  ['owner', 'editor'].includes(workspaces.value.find((workspace) => workspace.id === novel.workspaceId)?.role ?? '')
const pendingProposals = computed(() => proposals.value.filter((proposal) => proposal.status === 'pending'))
const stats = computed(() => ({
  chapterCount: chapters.value.length,
  words: chapters.value.reduce((total, chapter) => total + countManuscriptCharacters(chapter.body), 0),
  entities: nodes.value.length,
  pending: pendingProposals.value.length,
}))
const roleLabels: Record<string, string> = { owner: '所有者', editor: '编辑', reviewer: '审阅者', viewer: '只读' }

function notify(text: string, isError = false) {
  message.value = isError ? '' : text
  errorMessage.value = isError ? text : ''
  window.setTimeout(() => {
    if (message.value === text) message.value = ''
    if (errorMessage.value === text) errorMessage.value = ''
  }, 4200)
}

async function loadSession() {
  try {
    const result = await api.me()
    user.value = result.user
    workspaces.value = result.workspaces
    await loadNovels()
  } catch {
    user.value = null
  }
}

async function authenticate() {
  authBusy.value = true
  errorMessage.value = ''
  try {
    const action = authMode.value === 'register' ? api.register : api.login
    const result = await action(authEmail.value, authPassword.value)
    user.value = result.user
    await loadSession()
  } catch (error) {
    notify(error instanceof Error ? error.message : '登录失败', true)
  } finally {
    authBusy.value = false
  }
}

async function signOut() {
  await api.logout()
  user.value = null
  workspaces.value = []
  novels.value = []
  deletedNovels.value = []
  activeNovel.value = null
  deletedChapters.value = []
  closeTaskSocket()
}

async function loadNovels() {
  const [result, deletedResult] = await Promise.all([api.novels(), api.deletedNovels()])
  novels.value = result.novels
  deletedNovels.value = deletedResult.novels
  if (activeNovel.value) {
    activeNovel.value = novels.value.find((novel) => novel.id === activeNovel.value?.id) ?? null
  }
}

async function openNovel(novel: Novel, nextSection: typeof section.value = 'write') {
  loading.value = true
  try {
    if (activeNovel.value?.id !== novel.id && hasUnsavedChanges()) cacheDraft()
    window.clearTimeout(draftSaveTimer)
    window.clearTimeout(autoSaveTimer)
    cancelGraphNodeEdit()
    cancelGraphEdgeEdit()
    activeNovel.value = novel
    guideDraft.value = novel.writingGuide ?? ''
    section.value = nextSection
    const chapterTrashRequest = canEdit.value
      ? api.deletedChapters(novel.id)
      : Promise.resolve({ chapters: [] as Chapter[] })
    const [chapterResult, chapterTrashResult, graphResult, workflowResult, proposalResult, taskResult] =
      await Promise.all([
        api.chapters(novel.id),
        chapterTrashRequest,
        api.graph(novel.id),
        api.workflows(novel.id),
        api.proposals(novel.id),
        api.tasks(novel.id),
      ])
    chapters.value = chapterResult.chapters
    deletedChapters.value = chapterTrashResult.chapters
    nodes.value = graphResult.nodes
    edges.value = graphResult.edges
    workflows.value = workflowResult.workflows
    proposals.value = proposalResult.proposals
    tasks.value = taskResult.tasks
    if (!chapters.value.some((chapter) => chapter.id === activeChapterId.value)) {
      activeChapterId.value = chapters.value[0]?.id ?? ''
    }
    syncDraft()
    if (workflows.value.length) {
      selectWorkflow(workflows.value[0])
    } else {
      activeWorkflowId.value = ''
      workflowName.value = '章节创作流程'
      workflowDefinition.value = defaultWorkflow()
    }
    workflowChapterIds.value = chapters.value.slice(0, 1).map((chapter) => chapter.id)
    selectedChapterIds.value = workflowChapterIds.value
    await loadMembers()
  } catch (error) {
    notify(error instanceof Error ? error.message : '读取小说失败', true)
  } finally {
    loading.value = false
  }
}

async function loadMembers() {
  if (!activeWorkspace.value) return
  try {
    members.value = (await api.members(activeWorkspace.value.id)).members
  } catch {
    members.value = []
  }
}

function syncDraft() {
  window.clearTimeout(autoSaveTimer)
  autoSaveTimer = 0
  const chapter = activeChapter.value
  const serverDraft = chapter
    ? chapterDraftFrom(chapter)
    : { title: '', body: '', goal: '', keyEvents: '', suspenseHook: '' }
  let restored = { draft: serverDraft, restored: false }
  if (chapter) {
    try {
      restored = restoreChapterDraft(serverDraft, localStorage.getItem(draftStorageKey(chapter.id, chapter.revision)))
    } catch {
      /* ignore storage that cannot be read */
    }
  }
  draftTitle.value = restored.draft.title
  draftBody.value = restored.draft.body
  draftGoal.value = restored.draft.goal
  draftKeyEvents.value = restored.draft.keyEvents
  draftHook.value = restored.draft.suspenseHook
  if (chapter && restored.restored) notify('已恢复本地草稿')
  if (chapter) {
    const conflictKey = conflictStorageKey(chapter.id)
    try {
      const conflictMarker = localStorage.getItem(conflictKey)
      if (conflictMarker) {
        const marker = JSON.parse(conflictMarker) as { baseRevision?: number }
        if (typeof marker.baseRevision === 'number') {
          saveConflicts.value = {
            ...saveConflicts.value,
            [chapter.id]: { remote: chapter, baseRevision: marker.baseRevision },
          }
        }
      }
    } catch {
      /* keep loading even when browser storage is unavailable */
    }
    autoSaveState.value = saveConflicts.value[chapter.id] ? 'error' : hasUnsavedChanges() ? 'pending' : 'saved'
  } else {
    autoSaveState.value = 'saved'
  }
}

function selectChapter(chapterId: string) {
  if (chapterId === activeChapterId.value) return
  if (hasUnsavedChanges()) cacheDraft()
  window.clearTimeout(draftSaveTimer)
  window.clearTimeout(autoSaveTimer)
  activeChapterId.value = chapterId
}

function currentDraft(): ChapterDraft {
  return {
    title: draftTitle.value,
    body: draftBody.value,
    goal: draftGoal.value,
    keyEvents: draftKeyEvents.value,
    suspenseHook: draftHook.value,
  }
}

function draftMatches(snapshot: ChapterDraft) {
  return (
    draftTitle.value === snapshot.title &&
    draftBody.value === snapshot.body &&
    draftGoal.value === snapshot.goal &&
    draftKeyEvents.value === snapshot.keyEvents &&
    draftHook.value === snapshot.suspenseHook
  )
}

function removeSaveConflict(chapterId: string) {
  const next = { ...saveConflicts.value }
  delete next[chapterId]
  saveConflicts.value = next
  localStorage.removeItem(conflictStorageKey(chapterId))
}

function refreshSaveConflictSnapshots(latest: Chapter[]) {
  const next = { ...saveConflicts.value }
  for (const chapterId of Object.keys(next)) {
    const remote = latest.find((chapter) => chapter.id === chapterId)
    if (remote) next[chapterId] = { ...next[chapterId], remote }
  }
  saveConflicts.value = next
}

function clearDraftCache(chapterId: string, ...revisions: number[]) {
  for (const revision of new Set(revisions)) {
    localStorage.removeItem(draftStorageKey(chapterId, revision))
  }
}

function updateDraftCacheAfterSave(
  chapterId: string,
  baseRevision: number,
  savedRevision: number,
  snapshot: ChapterDraft,
  preserveLocalDraft: boolean,
) {
  const oldKey = draftStorageKey(chapterId, baseRevision)
  const cached = localStorage.getItem(oldKey)
  if (!cached) return
  try {
    const local = JSON.parse(cached) as Partial<ChapterDraft>
    const unchanged =
      local.title === snapshot.title &&
      local.body === snapshot.body &&
      local.goal === snapshot.goal &&
      local.keyEvents === snapshot.keyEvents &&
      local.suspenseHook === snapshot.suspenseHook
    if (unchanged || !preserveLocalDraft) {
      localStorage.removeItem(oldKey)
    } else if (savedRevision !== baseRevision) {
      localStorage.setItem(draftStorageKey(chapterId, savedRevision), cached)
      localStorage.removeItem(oldKey)
    }
  } catch {
    localStorage.removeItem(oldKey)
  }
}

function discardDraftConflict() {
  const chapter = activeChapter.value
  const conflict = activeSaveConflict.value
  if (!chapter || !conflict) return
  removeSaveConflict(chapter.id)
  clearDraftCache(chapter.id, conflict.baseRevision, conflict.remote.revision)
  draftTitle.value = conflict.remote.title
  draftBody.value = conflict.remote.body
  draftGoal.value = conflict.remote.goal
  draftKeyEvents.value = conflict.remote.keyEvents
  draftHook.value = conflict.remote.suspenseHook
  autoSaveState.value = 'saved'
}

async function saveDraftAfterConflict() {
  const chapter = activeChapter.value
  const conflict = activeSaveConflict.value
  if (!chapter || !conflict || !canEdit.value) return
  const confirmed = window.confirm(
    `服务器版本已更新到 v${conflict.remote.revision}。确认使用当前本地草稿覆盖该版本？建议先对照两栏并手动合并。`,
  )
  const plan = planConflictResolution(currentDraft(), chapterDraftFrom(conflict.remote), confirmed)
  if (!plan.overwriteServer) return
  removeSaveConflict(chapter.id)
  await persistChapterDraft(true)
}

function cacheDraft() {
  const chapter = activeChapter.value
  if (!chapter || !hasUnsavedChanges()) return
  localStorage.setItem(draftStorageKey(chapter.id, chapter.revision), JSON.stringify(currentDraft()))
}

function scheduleDraftCache() {
  window.clearTimeout(draftSaveTimer)
  draftSaveTimer = window.setTimeout(cacheDraft, 2000)
}

function scheduleChapterAutoSave() {
  window.clearTimeout(autoSaveTimer)
  autoSaveTimer = 0
  if (!activeChapter.value || !canEdit.value || !hasUnsavedChanges()) {
    if (!activeSaveConflict.value) autoSaveState.value = 'saved'
    return
  }
  if (activeSaveConflict.value) {
    autoSaveState.value = 'error'
    return
  }
  autoSaveState.value = 'pending'
  autoSaveTimer = window.setTimeout(() => {
    void persistChapterDraft(false)
  }, 2500)
}

function hasUnsavedChanges() {
  const chapter = activeChapter.value
  return (
    !!chapter &&
    (chapter.title !== draftTitle.value ||
      chapter.body !== draftBody.value ||
      chapter.goal !== draftGoal.value ||
      chapter.keyEvents !== draftKeyEvents.value ||
      chapter.suspenseHook !== draftHook.value)
  )
}

function isRevisionConflict(error: unknown) {
  if (!(error instanceof Error)) return false
  const code = error instanceof ApiError ? error.code : ''
  return isStaleChapterSave(code, error.message)
}

async function persistChapterDraft(showSuccess: boolean) {
  const chapter = activeChapter.value
  const novel = activeNovel.value
  if (!chapter || !novel || !canEdit.value || !hasUnsavedChanges()) return
  if (savingChapter.value) {
    scheduleChapterAutoSave()
    return
  }
  if (saveConflicts.value[chapter.id]) {
    autoSaveState.value = 'error'
    return
  }
  window.clearTimeout(autoSaveTimer)
  autoSaveTimer = 0
  const chapterId = chapter.id
  const novelId = novel.id
  const baseRevision = chapter.revision
  const snapshot = currentDraft()
  savingChapter.value = true
  autoSaveState.value = 'saving'
  try {
    const saved = await api.saveChapter({ ...chapter, ...snapshot })
    const index = chapters.value.findIndex((item) => item.id === saved.id)
    if (index !== -1) chapters.value[index] = saved
    const currentDraftIsSnapshot = activeChapterId.value === chapterId && draftMatches(snapshot)
    updateDraftCacheAfterSave(chapterId, baseRevision, saved.revision, snapshot, !currentDraftIsSnapshot)
    if (currentDraftIsSnapshot) {
      syncDraft()
      clearDraftCache(chapterId, saved.revision)
      autoSaveState.value = 'saved'
    } else if (activeChapterId.value === chapterId) {
      autoSaveState.value = 'pending'
    }
    if (showSuccess) notify('章节已保存')
    if (showHistory.value && activeChapterId.value === chapterId) await loadChapterRevisions()
  } catch (error) {
    const failureMessage = error instanceof Error ? error.message : ''
    const failureCode = error instanceof ApiError ? error.code : ''
    if (isRevisionConflict(error)) {
      try {
        const latest = await api.chapters(novelId)
        chapters.value = latest.chapters
        const remoteChapter = latest.chapters.find((item) => item.id === chapterId)
        const remote = remoteChapter ? { ...chapterDraftFrom(remoteChapter), revision: remoteChapter.revision } : null
        const plan = planFailedChapterSave(snapshot, failureMessage, remote, baseRevision, failureCode)
        if (plan.recordConflict && plan.conflict && remoteChapter && !plan.overwriteServer) {
          saveConflicts.value = {
            ...saveConflicts.value,
            [chapterId]: { remote: remoteChapter, baseRevision: plan.conflict.baseRevision },
          }
          localStorage.setItem(
            conflictStorageKey(chapterId),
            JSON.stringify({ baseRevision: plan.conflict.baseRevision }),
          )
          if (activeChapterId.value === chapterId && plan.retainLocalDraft) {
            cacheDraft()
            autoSaveState.value = 'error'
          }
          notify('检测到其他版本已更新，请比较差异后选择如何处理。', true)
        } else if (!plan.overwriteServer) {
          if (plan.retainLocalDraft) cacheDraft()
          notify('该章节已被移入回收站，本地草稿仍保留在浏览器中。', true)
        }
      } catch {
        const plan = planFailedChapterSave(snapshot, '保存冲突后无法读取服务器版本', null, baseRevision)
        if (plan.retainLocalDraft && !plan.overwriteServer) cacheDraft()
        notify('保存冲突后无法读取服务器版本，本地草稿已保留。', true)
      }
    } else {
      const plan = planFailedChapterSave(snapshot, failureMessage, null, baseRevision, failureCode)
      autoSaveState.value = 'error'
      if (plan.retainLocalDraft && !plan.overwriteServer) cacheDraft()
      notify(error instanceof Error ? error.message : '章节保存失败，本地草稿已保留。', true)
    }
  } finally {
    savingChapter.value = false
    if (activeChapterId.value === chapterId && hasUnsavedChanges() && !saveConflicts.value[chapterId]) {
      scheduleChapterAutoSave()
    }
  }
}

watch(activeChapterId, () => {
  syncDraft()
  chapterRevisions.value = []
  if (showHistory.value) void loadChapterRevisions()
})

async function createNovel() {
  const workspace = workspaces.value.find((item) => item.role === 'owner' || item.role === 'editor')
  if (!workspace || !novelTitle.value.trim()) {
    notify('先填写小说名称', true)
    return
  }
  try {
    const created = await api.createNovel(
      novelTitle.value.trim(),
      novelSummary.value,
      novelGenre.value,
      plannedChapters.value,
      workspace.id,
    )
    novelTitle.value = ''
    novelSummary.value = ''
    novelGenre.value = ''
    showCreate.value = false
    await loadNovels()
    await openNovel(created)
  } catch (error) {
    notify(error instanceof Error ? error.message : '创建失败', true)
  }
}

async function deleteActiveNovel() {
  const novel = activeNovel.value
  if (
    !novel ||
    !canEdit.value ||
    savingChapter.value ||
    !window.confirm(`将《${novel.title}》移入回收站？作品数据可在回收站恢复。`)
  )
    return
  if (hasUnsavedChanges()) cacheDraft()
  try {
    await api.deleteNovel(novel.id)
    closeTaskSocket()
    activeNovel.value = null
    chapters.value = []
    deletedChapters.value = []
    activeChapterId.value = ''
    section.value = 'trash'
    await loadNovels()
    notify('作品已移入回收站')
  } catch (error) {
    notify(error instanceof Error ? error.message : '移入回收站失败', true)
  }
}

async function restoreNovel(novel: Novel) {
  if (!canManageNovel(novel)) return
  try {
    await api.restoreNovel(novel.id)
    await loadNovels()
    notify(`《${novel.title}》已恢复`)
  } catch (error) {
    notify(error instanceof Error ? error.message : '恢复作品失败', true)
  }
}

async function addChapter() {
  if (!activeNovel.value || !canEdit.value) return
  if (hasUnsavedChanges()) cacheDraft()
  try {
    const chapter = await api.createChapter(activeNovel.value.id, '')
    chapters.value.push(chapter)
    window.clearTimeout(draftSaveTimer)
    draftSaveTimer = 0
    window.clearTimeout(autoSaveTimer)
    autoSaveTimer = 0
    activeChapterId.value = chapter.id
    notify('新章节已创建')
  } catch (error) {
    notify(error instanceof Error ? error.message : '创建章节失败', true)
  }
}

async function moveChapter(chapterId: string, offset: -1 | 1) {
  const novel = activeNovel.value
  if (!novel || !canEdit.value) return
  const currentIndex = chapters.value.findIndex((chapter) => chapter.id === chapterId)
  const targetIndex = currentIndex + offset
  if (currentIndex < 0 || targetIndex < 0 || targetIndex >= chapters.value.length) return
  const reordered = [...chapters.value]
  const currentChapter = reordered[currentIndex]
  reordered[currentIndex] = reordered[targetIndex]
  reordered[targetIndex] = currentChapter
  try {
    const result = await api.reorderChapters(
      novel.id,
      reordered.map((chapter) => chapter.id),
    )
    if (activeNovel.value?.id === novel.id) chapters.value = result.chapters
  } catch (error) {
    notify(error instanceof Error ? error.message : '调整章节顺序失败', true)
  }
}

async function deleteActiveChapter() {
  const novel = activeNovel.value
  const chapter = activeChapter.value
  if (
    !novel ||
    !chapter ||
    !canEdit.value ||
    savingChapter.value ||
    !window.confirm(`将第 ${chapter.number} 章《${chapter.title}》移入回收站？`)
  )
    return
  if (hasUnsavedChanges()) cacheDraft()
  try {
    await api.deleteChapter(novel.id, chapter.id)
    chapters.value = chapters.value.filter((item) => item.id !== chapter.id)
    deletedChapters.value = (await api.deletedChapters(novel.id)).chapters
    selectedChapterIds.value = selectedChapterIds.value.filter((id) => id !== chapter.id)
    workflowChapterIds.value = workflowChapterIds.value.filter((id) => id !== chapter.id)
    if (activeChapterId.value === chapter.id) {
      window.clearTimeout(draftSaveTimer)
      draftSaveTimer = 0
      window.clearTimeout(autoSaveTimer)
      autoSaveTimer = 0
      activeChapterId.value = chapters.value[0]?.id ?? ''
    }
    chapterRevisions.value = []
    syncDraft()
    notify('章节已移入回收站')
  } catch (error) {
    notify(error instanceof Error ? error.message : '移入回收站失败', true)
  }
}

async function restoreChapter(chapter: Chapter) {
  if (!activeNovel.value || !canEdit.value || savingChapter.value) return
  if (hasUnsavedChanges()) cacheDraft()
  try {
    await api.restoreChapter(activeNovel.value.id, chapter.id)
    const [activeResult, deletedResult] = await Promise.all([
      api.chapters(activeNovel.value.id),
      api.deletedChapters(activeNovel.value.id),
    ])
    chapters.value = activeResult.chapters
    deletedChapters.value = deletedResult.chapters
    window.clearTimeout(draftSaveTimer)
    draftSaveTimer = 0
    window.clearTimeout(autoSaveTimer)
    autoSaveTimer = 0
    activeChapterId.value = chapter.id
    notify(`第 ${chapter.number} 章已恢复`)
  } catch (error) {
    notify(error instanceof Error ? error.message : '恢复章节失败', true)
  }
}

async function saveChapter() {
  if (!activeChapter.value || !canEdit.value) return
  if (activeSaveConflict.value) {
    notify('请先对比冲突版本，再选择使用服务器稿或保存本地稿。', true)
    return
  }
  await persistChapterDraft(true)
}

async function refreshChapterMemory() {
  if (!activeNovel.value || !activeChapter.value || !canEdit.value || refreshingMemory.value) return
  if (
    hasUnsavedChanges() ||
    activeSaveConflict.value ||
    (activeTask.value && ['queued', 'running'].includes(activeTask.value.status))
  ) {
    notify('请先保存当前正文并等待生成任务结束。', true)
    return
  }
  const novelId = activeNovel.value.id
  const chapterId = activeChapter.value.id
  refreshingMemory.value = true
  try {
    const result = await api.refreshChapterMemory(novelId, chapterId)
    const chapter = chapters.value.find((item) => item.id === chapterId)
    if (chapter) chapter.memoryStatus = result.memoryStatus
    notify('章节记忆已更新')
  } catch (error) {
    notify(error instanceof Error ? error.message : '刷新章节记忆失败', true)
  } finally {
    refreshingMemory.value = false
  }
}

async function loadChapterRevisions() {
  if (!activeNovel.value || !activeChapter.value) return
  loadingHistory.value = true
  try {
    chapterRevisions.value = (await api.chapterRevisions(activeNovel.value.id, activeChapter.value.id)).revisions
  } catch (error) {
    notify(error instanceof Error ? error.message : '读取版本记录失败', true)
  } finally {
    loadingHistory.value = false
  }
}

async function toggleHistory() {
  showHistory.value = !showHistory.value
  if (showHistory.value) await loadChapterRevisions()
}

async function restoreChapterRevision(revision: ChapterRevision) {
  if (!activeNovel.value || !activeChapter.value || !canEdit.value || savingChapter.value) return
  if (activeSaveConflict.value) {
    notify('请先解决本地草稿与服务器版本的冲突。', true)
    return
  }
  const hasDraft = hasUnsavedChanges()
  const confirmText = hasDraft
    ? `恢复到 v${revision.revision}？当前本地草稿会另存到浏览器，但恢复内容会取代编辑器内容。`
    : `恢复到 v${revision.revision}？当前正文会保留为一个新版本。`
  if (!window.confirm(confirmText)) return
  if (hasDraft) cacheDraft()
  window.clearTimeout(draftSaveTimer)
  draftSaveTimer = 0
  window.clearTimeout(autoSaveTimer)
  autoSaveTimer = 0
  savingChapter.value = true
  try {
    const restored = await api.restoreChapterRevision(activeNovel.value.id, activeChapter.value.id, revision.revision)
    const index = chapters.value.findIndex((chapter) => chapter.id === restored.id)
    if (index !== -1) chapters.value[index] = restored
    syncDraft()
    await loadChapterRevisions()
    notify(`已恢复到 v${revision.revision}`)
  } catch (error) {
    notify(error instanceof Error ? error.message : '恢复版本失败', true)
  } finally {
    savingChapter.value = false
    if (hasUnsavedChanges() && !activeSaveConflict.value) scheduleChapterAutoSave()
  }
}

async function updateApprovalMode(event: Event) {
  if (!activeNovel.value || !canEdit.value) return
  const mode = (event.target as HTMLSelectElement).value as Novel['approvalMode']
  const novel = activeNovel.value
  try {
    await api.updateNovel(novel.id, {
      title: novel.title,
      summary: novel.summary,
      genre: novel.genre,
      plannedChapters: novel.plannedChapters,
      approvalMode: mode,
      writingGuide: novel.writingGuide,
    })
    novel.approvalMode = mode
    notify(mode === 'auto' ? '后续生成将自动写入章节' : '后续生成需要人工审核提案')
  } catch (error) {
    notify(error instanceof Error ? error.message : '更新审批设置失败', true)
  }
}

async function saveWritingGuide() {
  if (!activeNovel.value || !canEdit.value) return
  const novel = activeNovel.value
  savingGuide.value = true
  try {
    await api.updateNovel(novel.id, {
      title: novel.title,
      summary: novel.summary,
      genre: novel.genre,
      plannedChapters: novel.plannedChapters,
      approvalMode: novel.approvalMode,
      writingGuide: guideDraft.value,
    })
    novel.writingGuide = guideDraft.value
    notify('写作指南已保存，将注入后续生成')
  } catch (error) {
    notify(error instanceof Error ? error.message : '保存写作指南失败', true)
  } finally {
    savingGuide.value = false
  }
}

async function addGraphNode() {
  if (!activeNovel.value || !canEdit.value || !graphForm.value.label.trim()) return
  const novelId = activeNovel.value.id
  try {
    const index = nodes.value.length
    const node = await api.createGraphNode(novelId, {
      ...graphForm.value,
      label: graphForm.value.label.trim(),
      x: 80 + (index % 3) * 250,
      y: 80 + Math.floor(index / 3) * 150,
    })
    if (activeNovel.value?.id !== novelId) return
    nodes.value.push(node)
    graphForm.value.label = ''
    graphForm.value.summary = ''
    notify('设定实体已加入图谱')
  } catch (error) {
    notify(error instanceof Error ? error.message : '添加实体失败', true)
  }
}

function editGraphNode(node: GraphNode) {
  if (!canEdit.value) return
  editingNodeId.value = node.id
  graphForm.value = { kind: node.kind, label: node.label, summary: node.summary }
}

function selectGraphNodeForEdit(nodeId: string) {
  const node = nodes.value.find((item) => item.id === nodeId)
  if (node) editGraphNode(node)
  else cancelGraphNodeEdit()
}

function cancelGraphNodeEdit() {
  editingNodeId.value = ''
  graphForm.value = { kind: 'character', label: '', summary: '' }
}

async function saveGraphNode() {
  if (!activeNovel.value || !canEdit.value || !editingNodeId.value || !graphForm.value.label.trim()) return
  const novelId = activeNovel.value.id
  const editingId = editingNodeId.value
  const current = nodes.value.find((node) => node.id === editingId)
  if (!current) return cancelGraphNodeEdit()
  try {
    const updated = await api.updateGraphNode(novelId, editingId, {
      ...graphForm.value,
      label: graphForm.value.label.trim(),
      x: current.x,
      y: current.y,
    })
    if (activeNovel.value?.id !== novelId) return
    nodes.value = nodes.value.map((node) => (node.id === updated.id ? updated : node))
    if (editingNodeId.value === editingId) cancelGraphNodeEdit()
    notify('设定实体已更新')
  } catch (error) {
    notify(error instanceof Error ? error.message : '更新实体失败', true)
  }
}

async function addGraphEdge() {
  if (
    !activeNovel.value ||
    !canEdit.value ||
    !edgeForm.value.sourceId ||
    !edgeForm.value.targetId ||
    edgeForm.value.sourceId === edgeForm.value.targetId ||
    !edgeForm.value.relation.trim()
  )
    return
  const novelId = activeNovel.value.id
  try {
    const edge = await api.createGraphEdge(novelId, { ...edgeForm.value, relation: edgeForm.value.relation.trim() })
    if (activeNovel.value?.id !== novelId) return
    edges.value.push(edge)
    cancelGraphEdgeEdit()
    notify('关系已建立')
  } catch (error) {
    notify(error instanceof Error ? error.message : '添加关系失败', true)
  }
}

function editGraphEdge(edge: GraphEdge) {
  if (!canEdit.value) return
  editingEdgeId.value = edge.id
  edgeForm.value = {
    sourceId: edge.sourceId,
    targetId: edge.targetId,
    relation: edge.relation,
    summary: edge.summary,
  }
}

function cancelGraphEdgeEdit() {
  editingEdgeId.value = ''
  edgeForm.value = { sourceId: '', targetId: '', relation: '', summary: '' }
}

async function saveGraphEdge() {
  if (
    !activeNovel.value ||
    !canEdit.value ||
    !editingEdgeId.value ||
    !edgeForm.value.sourceId ||
    !edgeForm.value.targetId ||
    edgeForm.value.sourceId === edgeForm.value.targetId ||
    !edgeForm.value.relation.trim()
  )
    return
  const novelId = activeNovel.value.id
  const editingId = editingEdgeId.value
  const input = { ...edgeForm.value, relation: edgeForm.value.relation.trim() }
  try {
    await api.updateGraphEdge(novelId, editingId, input)
    if (activeNovel.value?.id !== novelId) return
    edges.value = edges.value.map((edge) => (edge.id === editingId ? { ...edge, ...input } : edge))
    if (editingEdgeId.value === editingId) cancelGraphEdgeEdit()
    notify('关系已更新')
  } catch (error) {
    notify(error instanceof Error ? error.message : '更新关系失败', true)
  }
}

async function removeGraphNode(node: GraphNode) {
  if (!activeNovel.value || !canEdit.value || !window.confirm(`删除实体“${node.label}”？相关关系也会被删除。`)) return
  const novelId = activeNovel.value.id
  try {
    await api.deleteGraphNode(novelId, node.id)
    if (activeNovel.value?.id !== novelId) return
    nodes.value = nodes.value.filter((item) => item.id !== node.id)
    edges.value = edges.value.filter((item) => item.sourceId !== node.id && item.targetId !== node.id)
    if (editingNodeId.value === node.id) cancelGraphNodeEdit()
    if (editingEdgeId.value && edges.value.every((edge) => edge.id !== editingEdgeId.value)) cancelGraphEdgeEdit()
    notify('实体已删除')
  } catch (error) {
    notify(error instanceof Error ? error.message : '删除实体失败', true)
  }
}

async function removeGraphEdge(edge: GraphEdge) {
  if (!activeNovel.value || !canEdit.value || !window.confirm('删除这条关系？')) return
  const novelId = activeNovel.value.id
  try {
    await api.deleteGraphEdge(novelId, edge.id)
    if (activeNovel.value?.id !== novelId) return
    edges.value = edges.value.filter((item) => item.id !== edge.id)
    if (editingEdgeId.value === edge.id) cancelGraphEdgeEdit()
    notify('关系已删除')
  } catch (error) {
    notify(error instanceof Error ? error.message : '删除关系失败', true)
  }
}

function defaultWorkflow(): WorkflowDefinition {
  const contextId = crypto.randomUUID()
  const generateId = crypto.randomUUID()
  const reviewId = crypto.randomUUID()
  const outputId = crypto.randomUUID()
  return {
    nodes: [
      {
        id: contextId,
        kind: 'context',
        title: '读取设定',
        prompt: '使用知识图谱和章节已有内容作为上下文。',
        x: 36,
        y: 70,
      },
      { id: generateId, kind: 'generate', title: '章节创作', prompt: '按照当前章节目标完成创作。', x: 318, y: 70 },
      {
        id: reviewId,
        kind: 'review',
        title: '一致性检查',
        prompt: '检查角色动机、设定冲突和情节因果。',
        x: 600,
        y: 70,
      },
      {
        id: outputId,
        kind: 'output',
        title: '提交提案',
        prompt: '输出完整章节修改提案，等待用户审核。',
        x: 882,
        y: 70,
      },
    ],
    edges: [
      { id: crypto.randomUUID(), source: contextId, target: generateId },
      { id: crypto.randomUUID(), source: generateId, target: reviewId },
      { id: crypto.randomUUID(), source: reviewId, target: outputId },
    ],
  }
}

function selectWorkflow(workflow: Workflow) {
  activeWorkflowId.value = workflow.id
  workflowName.value = workflow.name
  workflowDefinition.value = JSON.parse(JSON.stringify(workflow.definition)) as WorkflowDefinition
}

function selectWorkflowById(workflowId: string) {
  const item = workflows.value.find((workflow) => workflow.id === workflowId)
  if (item) selectWorkflow(item)
}

async function createWorkflow() {
  if (!activeNovel.value || !canEdit.value) return
  try {
    const definition = defaultWorkflow()
    const workflow = await api.createWorkflow(
      activeNovel.value.id,
      `新工作流 ${workflows.value.length + 1}`,
      definition,
    )
    workflows.value.unshift(workflow)
    selectWorkflow(workflow)
    notify('工作流已创建')
  } catch (error) {
    notify(error instanceof Error ? error.message : '创建工作流失败', true)
  }
}

async function createWorkflowTemplate(template: string) {
  if (!activeNovel.value || !canEdit.value) return
  try {
    const workflow = await api.createWorkflowTemplate(activeNovel.value.id, template)
    workflows.value.unshift(workflow)
    selectWorkflow(workflow)
    notify(`已创建「${template}」`)
  } catch (error) {
    notify(error instanceof Error ? error.message : '创建工作流失败', true)
  }
}

async function saveWorkflow() {
  if (!activeNovel.value || !canEdit.value) return
  try {
    if (!activeWorkflowId.value) {
      const workflow = await api.createWorkflow(activeNovel.value.id, workflowName.value, workflowDefinition.value)
      workflows.value.unshift(workflow)
      selectWorkflow(workflow)
    } else {
      await api.updateWorkflow(
        activeNovel.value.id,
        activeWorkflowId.value,
        workflowName.value,
        workflowDefinition.value,
      )
      const item = workflows.value.find((workflow) => workflow.id === activeWorkflowId.value)
      if (item) {
        item.name = workflowName.value
        item.definition = JSON.parse(JSON.stringify(workflowDefinition.value)) as WorkflowDefinition
      }
    }
    notify('工作流已保存')
  } catch (error) {
    notify(error instanceof Error ? error.message : '保存工作流失败', true)
  }
}

async function deleteActiveWorkflow() {
  const novel = activeNovel.value
  const workflowId = activeWorkflowId.value
  const workflow = workflows.value.find((item) => item.id === workflowId)
  if (!novel || !canEdit.value || !workflow || !window.confirm(`永久删除工作流“${workflow.name}”？`)) return
  try {
    await api.deleteWorkflow(novel.id, workflowId)
    if (activeNovel.value?.id !== novel.id) return
    workflows.value = workflows.value.filter((item) => item.id !== workflowId)
    if (workflows.value.length) {
      selectWorkflow(workflows.value[0])
    } else {
      activeWorkflowId.value = ''
      workflowName.value = '章节创作流程'
      workflowDefinition.value = defaultWorkflow()
    }
    notify('工作流已删除')
  } catch (error) {
    notify(error instanceof Error ? error.message : '删除工作流失败', true)
  }
}

async function addMember() {
  if (!activeWorkspace.value || activeWorkspace.value.role !== 'owner' || !memberEmail.value.trim()) return
  try {
    await api.addMember(activeWorkspace.value.id, memberEmail.value.trim(), memberRole.value)
    memberEmail.value = ''
    await loadMembers()
    notify('成员已加入工作区')
  } catch (error) {
    notify(error instanceof Error ? error.message : '添加成员失败', true)
  }
}

async function changeMemberRole(member: Member, event: Event) {
  if (!activeWorkspace.value) return
  const role = (event.target as HTMLSelectElement).value
  try {
    await api.updateMember(activeWorkspace.value.id, member.id, role)
    member.role = role
    notify('成员权限已更新')
  } catch (error) {
    notify(error instanceof Error ? error.message : '更新权限失败', true)
    await loadMembers()
  }
}

async function removeMember(member: Member) {
  if (!activeWorkspace.value || !window.confirm(`确定从工作区移除 ${member.email}？`)) return
  try {
    await api.removeMember(activeWorkspace.value.id, member.id)
    members.value = members.value.filter((item) => item.id !== member.id)
    notify('成员已移除')
  } catch (error) {
    notify(error instanceof Error ? error.message : '移除成员失败', true)
  }
}

async function startBatch() {
  if (!activeNovel.value || !canEdit.value) return
  const ids = selectedChapterIds.value.length
    ? selectedChapterIds.value
    : activeChapter.value
      ? [activeChapter.value.id]
      : []
  if (!ids.length || !runInstruction.value.trim()) {
    notify('选择至少一个章节并填写创作要求', true)
    return
  }
  try {
    const task = await api.createBatchTask(activeNovel.value.id, ids, runInstruction.value.trim())
    await startTask(task)
  } catch (error) {
    notify(error instanceof Error ? error.message : '创建任务失败', true)
  }
}

async function startReview() {
  if (!activeNovel.value || !canEdit.value) return
  const ids = selectedChapterIds.value.length
    ? selectedChapterIds.value
    : activeChapter.value
      ? [activeChapter.value.id]
      : []
  if (!ids.length || !runInstruction.value.trim()) {
    notify('选择至少一个章节并填写审稿说明', true)
    return
  }
  try {
    const task = await api.createReviewTask(activeNovel.value.id, ids, runInstruction.value.trim())
    await startTask(task)
  } catch (error) {
    notify(error instanceof Error ? error.message : '创建审稿任务失败', true)
  }
}

async function runWorkflow() {
  if (!activeNovel.value || !canEdit.value) return
  const ids = workflowChapterIds.value.length
    ? workflowChapterIds.value
    : activeChapter.value
      ? [activeChapter.value.id]
      : []
  if (!ids.length) {
    notify('先选择需要运行的章节', true)
    return
  }
  try {
    await saveWorkflow()
    const workflowID = activeWorkflowId.value || workflows.value[0]?.id
    if (!workflowID) return
    const task = await api.runWorkflow(
      activeNovel.value.id,
      workflowID,
      ids,
      runInstruction.value.trim() || '根据工作流完成章节创作并生成修改提案。',
    )
    await startTask(task)
  } catch (error) {
    notify(error instanceof Error ? error.message : '启动工作流失败', true)
  }
}

async function startTask(task: NovelTask) {
  activeTask.value = task
  taskProgress.value = 0
  chapterStates.value = {}
  nodeProgress.value = []
  reviewItems.value = []
  retryingTask.value = false
  demoMode.value = false
  streamText.value = ''
  taskContextSources.value = []
  taskDroppedContext.value = []
  contextTruncation.value = { entities: 0, relations: 0 }
  lastSequence.value = 0
  tasks.value.unshift(task)
  openTaskSocket()
  section.value = 'write'
  notify('创作任务已排队，实时进度正在连接')
}

async function resumeTask(task: NovelTask) {
  if (!activeNovel.value) return
  try {
    const latest = await api.task(task.id)
    activeTask.value = latest
    taskProgress.value = latest.progress
    chapterStates.value = {}
    nodeProgress.value = []
    reviewItems.value = []
    retryingTask.value = false
    demoMode.value = false
    streamText.value = ''
    taskContextSources.value = []
    taskDroppedContext.value = []
    contextTruncation.value = { entities: 0, relations: 0 }
    lastSequence.value = 0
    section.value = 'write'
    openTaskSocket()
  } catch (error) {
    notify(error instanceof Error ? error.message : '恢复任务事件失败', true)
  }
}

async function cancelActiveTask() {
  if (!activeTask.value || !['queued', 'running'].includes(activeTask.value.status)) return
  try {
    await api.cancelTask(activeTask.value.id)
    if (activeTask.value && ['queued', 'running'].includes(activeTask.value.status)) notify('正在取消任务…')
  } catch (error) {
    notify(error instanceof Error ? error.message : '取消任务失败', true)
  }
}

async function retryActiveTask() {
  if (!activeTask.value || !canEdit.value || retryingTask.value) return
  retryingTask.value = true
  try {
    const task = await api.retryTask(activeTask.value.id)
    await startTask(task)
  } catch (error) {
    notify(error instanceof Error ? error.message : '重试任务失败', true)
  } finally {
    retryingTask.value = false
  }
}

function openTaskSocket() {
  if (!activeTask.value) return
  closeTaskSocket()
  taskSocket = connectTask(
    activeTask.value.id,
    lastSequence.value,
    (event) => {
      if (event.sequence <= lastSequence.value) return
      lastSequence.value = event.sequence
      handleTaskEvent(event)
    },
    () => {
      if (!activeTask.value || ['completed', 'failed', 'canceled', 'interrupted'].includes(activeTask.value.status))
        return
      reconnectTimer = window.setTimeout(openTaskSocket, 1400)
    },
  )
}

function closeTaskSocket() {
  if (reconnectTimer) window.clearTimeout(reconnectTimer)
  reconnectTimer = 0
  if (taskSocket) {
    taskSocket.onclose = null
    taskSocket.onerror = null
    taskSocket.close()
    taskSocket = null
  }
}

function handleTaskEvent(event: TaskEvent) {
  const data = event.data
  if (event.type === 'task.started' && data.demoMode === true) demoMode.value = true
  if (event.type === 'task.started') {
    taskContextSources.value = Array.isArray(data.contextSources)
      ? data.contextSources.filter((source): source is string => typeof source === 'string')
      : []
    taskDroppedContext.value = Array.isArray(data.droppedContext)
      ? data.droppedContext.filter((source): source is string => typeof source === 'string')
      : []
    contextTruncation.value = {
      entities: typeof data.truncatedEntities === 'number' ? data.truncatedEntities : 0,
      relations: typeof data.truncatedRelations === 'number' ? data.truncatedRelations : 0,
    }
  }
  const chapterStatusByEvent: Record<string, string> = {
    'chapter.queued': 'queued',
    'chapter.started': 'running',
    'chapter.completed': 'completed',
    'chapter.failed': 'failed',
    'chapter.canceled': 'canceled',
  }
  const chapterStatus = chapterStatusByEvent[event.type]
  if (chapterStatus && typeof data.chapterId === 'string') {
    const previous = chapterStates.value[data.chapterId]
    chapterStates.value[data.chapterId] = {
      chapterId: data.chapterId,
      chapterNumber: typeof data.chapterNumber === 'number' ? data.chapterNumber : (previous?.chapterNumber ?? 0),
      title: typeof data.title === 'string' ? data.title : (previous?.title ?? ''),
      status: chapterStatus,
    }
  }
  if (event.type === 'node.started' && typeof data.nodeId === 'string') {
    nodeProgress.value.push({
      nodeId: data.nodeId,
      title: typeof data.title === 'string' && data.title ? data.title : data.nodeId,
      status: 'running',
    })
  }
  if (event.type === 'review.checklist' && Array.isArray(data.items)) {
    for (const item of data.items) {
      if (!item || typeof item !== 'object') continue
      const row = item as Record<string, unknown>
      if (typeof row.event !== 'string' || typeof row.status !== 'string') continue
      reviewItems.value.push({
        event: row.event,
        status: row.status,
        quote: typeof row.quote === 'string' ? row.quote : '',
        location: typeof row.location === 'string' ? row.location : '',
        modelJudgment: typeof row.modelJudgment === 'string' ? row.modelJudgment : '',
        quoteVerbatim: row.quoteVerbatim === true,
      })
    }
  }
  if (event.type === 'node.finished' && typeof data.nodeId === 'string') {
    const status = typeof data.status === 'string' ? data.status : 'succeeded'
    for (let index = nodeProgress.value.length - 1; index >= 0; index -= 1) {
      if (nodeProgress.value[index].nodeId === data.nodeId && nodeProgress.value[index].status === 'running') {
        nodeProgress.value[index] = { ...nodeProgress.value[index], status }
        break
      }
    }
  }
  if (event.type === 'task.token' && typeof data.text === 'string') streamText.value += data.text
  if (event.type === 'task.progress' && typeof data.progress === 'number') {
    taskProgress.value = data.progress
    const listedTask = tasks.value.find((item) => item.id === activeTask.value?.id)
    if (listedTask) listedTask.progress = data.progress
  }
  const statusByEvent: Record<string, string> = {
    'task.started': 'running',
    'task.completed': 'completed',
    'task.failed': 'failed',
    'task.canceled': 'canceled',
    'task.interrupted': 'interrupted',
  }
  const status = statusByEvent[event.type]
  if (status) {
    if (activeTask.value) activeTask.value.status = status
    const listedTask = tasks.value.find((item) => item.id === activeTask.value?.id)
    if (listedTask) listedTask.status = status
    if (['completed', 'failed', 'canceled', 'interrupted'].includes(status)) {
      taskProgress.value = status === 'completed' ? 100 : taskProgress.value
      void refreshNovelData()
      if (status === 'completed')
        notify(
          activeTask.value?.kind === 'review'
            ? '审稿清单已完成，正文未改动'
            : demoMode.value
              ? '演示任务完成，生成内容未自动写入'
              : activeNovel.value?.approvalMode === 'auto'
                ? '生成完成，章节已自动写入'
                : '生成完成，提案已进入审核区',
        )
      else if (status === 'failed') notify(typeof data.message === 'string' ? data.message : '生成失败', true)
      else if (status === 'canceled') notify('创作任务已取消')
      else if (status === 'interrupted') notify('服务器重启后该任务已中断，可重新发起生成', true)
      closeTaskSocket()
    }
  }
  if (event.type === 'proposal.created') {
    void refreshNovelData()
    notify('收到新的章节提案，可在下方审核')
  }
  if (event.type === 'proposal.applied') {
    void refreshNovelData()
    notify('提案已自动写入章节')
  }
}

async function refreshNovelData() {
  if (!activeNovel.value) return
  const keepDraft = hasUnsavedChanges()
  const [chapterResult, proposalResult, taskResult] = await Promise.all([
    api.chapters(activeNovel.value.id),
    api.proposals(activeNovel.value.id),
    api.tasks(activeNovel.value.id),
  ])
  chapters.value = chapterResult.chapters
  refreshSaveConflictSnapshots(chapterResult.chapters)
  proposals.value = proposalResult.proposals
  tasks.value = taskResult.tasks
  if (!keepDraft) syncDraft()
}

async function applyProposal(proposal: Proposal, selection?: ProposalSelection) {
  if (!activeNovel.value) return
  if (proposal.chapterId === activeChapterId.value && hasUnsavedChanges()) {
    notify('请先保存或放弃当前章节草稿，再应用提案。', true)
    return
  }
  try {
    await api.applyProposal(activeNovel.value.id, proposal.id, selection)
    await refreshNovelData()
    if (selection) {
      const applied = [
        selection.selectedHunks.length ? `${selection.selectedHunks.length} 组段落修改` : '',
        selection.applyTitle ? '提案标题' : '',
      ].filter(Boolean)
      notify(`已采纳${applied.join('和')}；提案已处理，章节版本已更新。`)
    } else {
      notify('提案已应用，章节版本已更新')
    }
  } catch (error) {
    if (error instanceof Error && /更新|版本/.test(error.message)) {
      try {
        await refreshNovelData()
      } catch {
        /* preserve the original stale-proposal message */
      }
    }
    notify(error instanceof Error ? error.message : '应用提案失败', true)
  }
}

async function regenerateStaleProposal(proposal: Proposal) {
  if (!activeNovel.value || !canEdit.value) return
  if (hasUnsavedChanges() || activeSaveConflict.value) {
    notify('请先保存或处理当前章节草稿，再重新生成过期提案。', true)
    return
  }
  if (activeTask.value && ['queued', 'running'].includes(activeTask.value.status)) {
    notify('请等待当前生成任务完成后再重新生成。', true)
    return
  }
  try {
    const task = await api.regenerateStaleProposal(activeNovel.value.id, proposal.id)
    await startTask(task)
  } catch (error) {
    notify(error instanceof Error ? error.message : '重新生成提案失败', true)
  }
}

async function rejectProposal(proposal: Proposal) {
  if (!activeNovel.value) return
  try {
    await api.rejectProposal(activeNovel.value.id, proposal.id)
    await refreshNovelData()
    notify('提案已拒绝')
  } catch (error) {
    notify(error instanceof Error ? error.message : '拒绝提案失败', true)
  }
}

watch([draftTitle, draftBody, draftGoal, draftKeyEvents, draftHook], () => {
  scheduleDraftCache()
  scheduleChapterAutoSave()
})
onMounted(() => {
  void loadSession()
  window.addEventListener('beforeunload', beforeUnloadHandler)
})
onBeforeUnmount(() => {
  window.clearTimeout(draftSaveTimer)
  window.clearTimeout(autoSaveTimer)
  window.removeEventListener('beforeunload', beforeUnloadHandler)
})
onBeforeUnmount(closeTaskSocket)
</script>

<template>
  <LoginPage
    v-if="!user"
    v-model:auth-mode="authMode"
    v-model:email="authEmail"
    v-model:password="authPassword"
    :busy="authBusy"
    :error-message="errorMessage"
    @submit="authenticate"
  />

  <div v-else class="app-shell">
    <aside class="sidebar">
      <div class="brand-row">
        <div class="brand-mark">卷</div>
        <div><strong>拾卷</strong><span>STORY STUDIO</span></div>
      </div>
      <div class="workspace-switch">
        <span class="workspace-glyph">{{ activeWorkspace?.name.slice(0, 1) ?? '创' }}</span
        ><span class="workspace-name">{{ activeWorkspace?.name ?? '个人工作区' }}</span
        ><span class="chevron">⌄</span>
      </div>
      <div class="side-label">工作台</div>
      <nav class="main-nav primary-nav">
        <button :class="{ active: section === 'library' }" @click="section = 'library'">
          <span class="nav-icon">▦</span>作品库
        </button>
        <button :disabled="!activeNovel" :class="{ active: section === 'write' }" @click="section = 'write'">
          <span class="nav-icon">▤</span>章节编辑
        </button>
        <button :disabled="!activeNovel" :class="{ active: section === 'knowledge' }" @click="section = 'knowledge'">
          <span class="nav-icon">⌘</span>知识图谱
        </button>
        <button :disabled="!activeNovel" :class="{ active: section === 'workflow' }" @click="section = 'workflow'">
          <span class="nav-icon">⌁</span>创作工作流
        </button>
        <button :class="{ active: section === 'trash' }" @click="section = 'trash'">
          <span class="nav-icon">⌫</span>回收站
          <span v-if="deletedNovels.length" class="nav-count">{{ deletedNovels.length }}</span>
        </button>
      </nav>
      <div class="side-label team-label">协作与设置</div>
      <nav class="main-nav manage-nav">
        <button :disabled="!activeNovel" :class="{ active: section === 'team' }" @click="section = 'team'">
          <span class="nav-icon">◉</span>成员与权限
        </button>
        <button :class="{ active: section === 'settings' }" @click="section = 'settings'">
          <span class="nav-icon">⚙</span>API 设置
        </button>
      </nav>
      <div class="sidebar-bottom">
        <div class="usage-card">
          <div class="usage-top"><span>创作概览</span><span class="usage-mark">LIVE</span></div>
          <div class="usage-number">{{ stats.words.toLocaleString() }}<small> 字</small></div>
          <div class="usage-caption">当前作品累计正文</div>
          <div class="usage-line"><span :style="{ width: `${Math.min(100, stats.chapterCount * 4)}%` }"></span></div>
          <div class="usage-bottom">
            <span>{{ stats.chapterCount }} 个章节</span><span>{{ stats.entities }} 个设定</span>
          </div>
        </div>
        <div class="profile-row">
          <div class="avatar">{{ user.email.slice(0, 1).toUpperCase() }}</div>
          <div class="profile-text">
            <strong>{{ user.email.split('@')[0] }}</strong
            ><span>{{ user.email }}</span>
          </div>
          <button class="icon-button signout" title="退出登录" @click="signOut">↗</button>
        </div>
      </div>
    </aside>

    <main class="main-area">
      <header class="topbar">
        <div class="breadcrumbs">
          <span>工作台</span><span class="crumb-sep">/</span
          ><strong>{{
            section === 'library'
              ? '作品库'
              : section === 'trash'
                ? '回收站'
                : section === 'settings'
                  ? 'API 设置'
                  : (activeNovel?.title ?? '选择作品')
          }}</strong
          ><template v-if="!['library', 'trash', 'settings'].includes(section)"
            ><span class="crumb-sep">/</span
            ><span>{{
              section === 'write'
                ? '章节编辑'
                : section === 'knowledge'
                  ? '知识图谱'
                  : section === 'workflow'
                    ? '创作工作流'
                    : '成员与权限'
            }}</span></template
          >
        </div>
        <div class="topbar-actions">
          <span class="connection"><i></i> 已安全连接</span>
          <div class="avatar small-avatar">{{ user.email.slice(0, 1).toUpperCase() }}</div>
        </div>
      </header>

      <div v-if="message || errorMessage" class="toast" :class="{ danger: !!errorMessage }">
        {{ errorMessage || message }}
      </div>

      <LibraryPage
        v-if="section === 'library'"
        v-model:show-create="showCreate"
        v-model:novel-title="novelTitle"
        v-model:novel-genre="novelGenre"
        v-model:novel-summary="novelSummary"
        v-model:planned-chapters="plannedChapters"
        :novels="novels"
        :stats="stats"
        :can-create="canEdit || !novels.length"
        @create="createNovel"
        @open="openNovel"
      />
      <TrashPage
        v-else-if="section === 'trash'"
        :novels="deletedNovels"
        :can-manage-novel="canManageNovel"
        @restore="restoreNovel"
      />
      <ModelSettingsPage v-else-if="section === 'settings'" />
      <ChapterEditorPage
        v-else-if="section === 'write' && activeNovel"
        v-model:draft-title="draftTitle"
        v-model:draft-body="draftBody"
        v-model:draft-goal="draftGoal"
        v-model:draft-key-events="draftKeyEvents"
        v-model:draft-hook="draftHook"
        v-model:guide-draft="guideDraft"
        v-model:run-instruction="runInstruction"
        v-model:selected-chapter-ids="selectedChapterIds"
        v-model:show-blueprint="showBlueprint"
        v-model:show-history="showHistory"
        :novel="activeNovel"
        :chapters="chapters"
        :deleted-chapters="deletedChapters"
        :active-chapter-id="activeChapterId"
        :active-chapter="activeChapter"
        :word-total="stats.words"
        :role-label="roleLabels[activeWorkspace?.role ?? 'viewer']"
        :can-edit="canEdit"
        :saving-chapter="savingChapter"
        :refreshing-memory="refreshingMemory"
        :unsaved="hasUnsavedChanges()"
        :auto-save-state="autoSaveState"
        :save-conflict="activeSaveConflict"
        :chapter-revisions="chapterRevisions"
        :loading-history="loadingHistory"
        :edges="edges"
        :entity-count="stats.entities"
        :tasks="tasks"
        :saving-guide="savingGuide"
        :markdown-href="api.exportNovel(activeNovel.id, 'md')"
        :text-href="api.exportNovel(activeNovel.id, 'txt')"
        :active-task-running="!!activeTask && ['queued', 'running'].includes(activeTask.status)"
        @update-approval="updateApprovalMode"
        @add-chapter="addChapter"
        @save-chapter="saveChapter"
        @refresh-memory="refreshChapterMemory"
        @delete-novel="deleteActiveNovel"
        @select-chapter="selectChapter"
        @move-chapter="moveChapter"
        @delete-chapter="deleteActiveChapter"
        @restore-chapter="restoreChapter"
        @toggle-history="toggleHistory"
        @restore-revision="restoreChapterRevision"
        @discard-conflict="discardDraftConflict"
        @save-conflict="saveDraftAfterConflict"
        @save-guide="saveWritingGuide"
        @start-batch="startBatch"
        @start-review="startReview"
        @resume-task="resumeTask"
        @open-knowledge="section = 'knowledge'"
      >
        <template #generation>
          <TaskMonitorPanel
            :task="activeTask"
            :demo-mode="demoMode"
            :progress="taskProgress"
            :chapter-progress="chapterProgress"
            :node-progress="nodeProgress"
            :review-items="reviewItems"
            :retrying="retryingTask"
            :can-edit="canEdit"
            :stream-text="streamText"
            :context-sources="taskContextSources"
            :dropped-context="taskDroppedContext"
            :context-truncation="contextTruncation"
            @cancel="cancelActiveTask"
            @retry="retryActiveTask"
          />
          <ProposalReviewPanel
            :proposals="proposals"
            :chapters="chapters"
            :active-chapter-id="activeChapterId"
            :can-review="canReview"
            :can-edit="canEdit"
            :has-unsaved-changes="hasUnsavedChanges()"
            :saving-chapter="savingChapter"
            :active-task-running="!!activeTask && ['queued', 'running'].includes(activeTask.status)"
            @apply="applyProposal"
            @apply-selected="applyProposal"
            @reject="rejectProposal"
            @regenerate="regenerateStaleProposal"
          />
        </template>
      </ChapterEditorPage>
      <KnowledgeGraphPage
        v-else-if="section === 'knowledge' && activeNovel"
        v-model:graph-form="graphForm"
        v-model:editing-node-id="editingNodeId"
        v-model:edge-form="edgeForm"
        v-model:editing-edge-id="editingEdgeId"
        :novel="activeNovel"
        :nodes="nodes"
        :edges="edges"
        :can-edit="canEdit"
        @select-node="selectGraphNodeForEdit"
        @add-node="addGraphNode"
        @save-node="saveGraphNode"
        @cancel-node="cancelGraphNodeEdit"
        @edit-node="editGraphNode"
        @remove-node="removeGraphNode"
        @add-edge="addGraphEdge"
        @save-edge="saveGraphEdge"
        @cancel-edge="cancelGraphEdgeEdit"
        @edit-edge="editGraphEdge"
        @remove-edge="removeGraphEdge"
      />
      <WorkflowPage
        v-else-if="section === 'workflow' && activeNovel"
        v-model:workflow-name="workflowName"
        v-model:workflow-definition="workflowDefinition"
        v-model:workflow-chapter-ids="workflowChapterIds"
        v-model:run-instruction="runInstruction"
        :workflows="workflows"
        :active-workflow-id="activeWorkflowId"
        :chapters="chapters"
        :can-edit="canEdit"
        :tasks="tasks"
        @select="selectWorkflowById"
        @create="createWorkflow"
        @create-template="createWorkflowTemplate"
        @save="saveWorkflow"
        @delete="deleteActiveWorkflow"
        @run="runWorkflow"
      />
      <MembersPage
        v-else-if="section === 'team' && activeNovel && user"
        v-model:member-email="memberEmail"
        v-model:member-role="memberRole"
        :members="members"
        :current-user-id="user.id"
        :workspace-role="activeWorkspace?.role ?? 'viewer'"
        :role-label="roleLabels[activeWorkspace?.role ?? 'viewer']"
        @add="addMember"
        @change-role="changeMemberRole"
        @remove="removeMember"
      />

      <div v-if="loading" class="loading-overlay">
        <span class="loading-spinner"></span><span>正在整理创作资料…</span>
      </div>
    </main>
  </div>
</template>
