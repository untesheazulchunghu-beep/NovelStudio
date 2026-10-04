<script setup lang="ts">
import { ref } from 'vue'
import type { Chapter, ModelProvider, NovelTask, Workflow, WorkflowDefinition, WorkflowNode } from '../api'
import { acceptWorkflowEdge, workflowEdgePath } from '../workflow-connection'

const workflowName = defineModel<string>('workflowName', { required: true })
const workflowDefinition = defineModel<WorkflowDefinition>('workflowDefinition', { required: true })
const workflowChapterIds = defineModel<string[]>('workflowChapterIds', { required: true })
const runInstruction = defineModel<string>('runInstruction', { required: true })
const providerId = defineModel<string>('providerId', { required: true })

const props = defineProps<{
  workflows: Workflow[]
  activeWorkflowId: string
  chapters: Chapter[]
  canEdit: boolean
  tasks: NovelTask[]
  providers: ModelProvider[]
}>()

const emit = defineEmits<{
  select: [workflowId: string]
  create: []
  createTemplate: [template: string]
  save: []
  delete: []
  run: []
}>()

const workflowCanvas = ref<HTMLElement | null>(null)
const draggingNodeId = ref('')
const edgeSource = ref('')
const edgeTarget = ref('')

const nodeKinds: Record<WorkflowNode['kind'], string> = {
  context: '资料注入',
  generate: '创作生成',
  review: '一致性检查',
  output: '提案输出',
}

function onSelectWorkflow(event: Event) {
  emit('select', (event.target as HTMLSelectElement).value)
}

function addWorkflowNode(kind: WorkflowNode['kind']) {
  const previous = workflowDefinition.value.nodes.at(-1)
  const node: WorkflowNode = {
    id: crypto.randomUUID(),
    kind,
    title: nodeKinds[kind],
    prompt: kind === 'review' ? '检查内容是否符合前文设定。' : '补充此步骤的具体要求。',
    x: 60 + (workflowDefinition.value.nodes.length % 4) * 240,
    y: 250 + Math.floor(workflowDefinition.value.nodes.length / 4) * 200,
  }
  workflowDefinition.value.nodes.push(node)
  if (previous) workflowDefinition.value.edges.push({ id: crypto.randomUUID(), source: previous.id, target: node.id })
}

function beginNodeDrag(event: PointerEvent, id: string) {
  if (!props.canEdit) return
  draggingNodeId.value = id
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}

function moveNode(event: PointerEvent) {
  if (!draggingNodeId.value || !workflowCanvas.value) return
  const node = workflowDefinition.value.nodes.find((item) => item.id === draggingNodeId.value)
  if (!node) return
  const rect = workflowCanvas.value.getBoundingClientRect()
  node.x = Math.max(0, Math.min(rect.width - 208, event.clientX - rect.left - 92))
  node.y = Math.max(0, Math.min(rect.height - 145, event.clientY - rect.top - 18))
}

function finishNodeDrag() {
  draggingNodeId.value = ''
}

function connectWorkflowNodes() {
  const decision = acceptWorkflowEdge(workflowDefinition.value.edges, edgeSource.value, edgeTarget.value)
  if (!decision.ok) return
  workflowDefinition.value.edges.push({ id: crypto.randomUUID(), source: edgeSource.value, target: edgeTarget.value })
  edgeSource.value = ''
  edgeTarget.value = ''
}

function workflowLine(edge: WorkflowDefinition['edges'][number]) {
  const source = workflowDefinition.value.nodes.find((node) => node.id === edge.source)
  const target = workflowDefinition.value.nodes.find((node) => node.id === edge.target)
  return workflowEdgePath(source, target)
}

function removeWorkflowNode(id: string) {
  workflowDefinition.value.nodes = workflowDefinition.value.nodes.filter((item) => item.id !== id)
  workflowDefinition.value.edges = workflowDefinition.value.edges.filter(
    (edge) => edge.source !== id && edge.target !== id,
  )
}
</script>

<template>
  <section class="page workflow-page">
    <div class="page-heading workflow-heading">
      <div>
        <div class="eyebrow">VISUAL AUTHORING PIPELINE</div>
        <h1>把创作方法，变成流程。</h1>
        <p>拖动节点调整步骤，连接资料、生成与审核环节，复用你的创作节奏。</p>
      </div>
      <div class="workflow-actions">
        <select :value="activeWorkflowId" @change="onSelectWorkflow">
          <option value="">未保存流程</option>
          <option v-for="workflow in workflows" :key="workflow.id" :value="workflow.id">{{ workflow.name }}</option>
        </select>
        <button class="button secondary" :disabled="!canEdit" @click="emit('create')">＋ 新流程</button>
        <button class="button secondary" :disabled="!canEdit" @click="emit('createTemplate', '章节起草')">
          章节起草
        </button>
        <button class="button secondary" :disabled="!canEdit" @click="emit('createTemplate', '起草 → 审稿 → 修改')">
          起草 → 审稿 → 修改
        </button>
        <button class="button secondary" :disabled="!canEdit" @click="emit('createTemplate', '只润色')">只润色</button>
        <button class="button primary" :disabled="!canEdit" @click="emit('save')">保存流程</button>
      </div>
    </div>
    <div v-if="activeWorkflowId && canEdit" class="workflow-delete-action">
      <span>删除后可由历史任务保留的执行记录继续查看。</span>
      <button class="text-button" @click="emit('delete')">删除当前流程</button>
    </div>
    <div class="workflow-workspace">
      <div class="workflow-canvas-wrap">
        <div class="canvas-head">
          <input v-model="workflowName" :readonly="!canEdit" class="workflow-name-input" />
          <span class="canvas-hint"><i></i> 拖动节点调整位置</span>
        </div>
        <div
          ref="workflowCanvas"
          class="workflow-canvas"
          @pointermove="moveNode"
          @pointerup="finishNodeDrag"
          @pointercancel="finishNodeDrag"
        >
          <svg
            class="workflow-wires"
            :viewBox="`0 0 ${Math.max(1120, ...workflowDefinition.nodes.map((node) => node.x + 260))} ${Math.max(570, ...workflowDefinition.nodes.map((node) => node.y + 190))}`"
          >
            <defs>
              <marker id="arrow" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
                <path d="M0,0 L8,4 L0,8 Z" />
              </marker>
            </defs>
            <path
              v-for="edge in workflowDefinition.edges"
              :key="edge.id"
              :d="workflowLine(edge)"
              marker-end="url(#arrow)"
            />
          </svg>
          <article
            v-for="(node, index) in workflowDefinition.nodes"
            :key="node.id"
            class="workflow-node"
            :class="[`flow-${node.kind}`, { dragging: draggingNodeId === node.id }]"
            :style="{ left: `${node.x}px`, top: `${node.y}px` }"
            @pointerdown="beginNodeDrag($event, node.id)"
          >
            <div class="workflow-node-head">
              <span class="flow-index">{{ (index + 1).toString().padStart(2, '0') }}</span>
              <span>{{ nodeKinds[node.kind] }}</span>
              <button
                class="node-remove"
                :disabled="!canEdit"
                @pointerdown.stop
                @click.stop="removeWorkflowNode(node.id)"
              >
                ×
              </button>
            </div>
            <input v-model="node.title" :readonly="!canEdit" @pointerdown.stop />
            <textarea v-model="node.prompt" :readonly="!canEdit" rows="2" @pointerdown.stop></textarea>
            <select
              v-if="node.kind === 'generate' || node.kind === 'review'"
              v-model="node.providerId"
              :disabled="!canEdit"
              @pointerdown.stop
            >
              <option value="">沿用任务模型</option>
              <option v-for="provider in providers" :key="provider.id" :value="provider.id">{{ provider.name }}</option>
            </select>
          </article>
          <div v-if="!workflowDefinition.nodes.length" class="canvas-empty">添加一个节点，搭建你的创作流程</div>
        </div>
        <div class="canvas-bottom">
          <div class="node-palette">
            <span>添加步骤</span>
            <button @click="addWorkflowNode('context')">＋ 资料</button>
            <button @click="addWorkflowNode('generate')">＋ 生成</button>
            <button @click="addWorkflowNode('review')">＋ 审核</button>
            <button @click="addWorkflowNode('output')">＋ 输出</button>
          </div>
          <div class="connect-controls">
            <select v-model="edgeSource">
              <option value="">起始节点</option>
              <option v-for="node in workflowDefinition.nodes" :key="node.id" :value="node.id">{{ node.title }}</option>
            </select>
            <span>→</span>
            <select v-model="edgeTarget">
              <option value="">目标节点</option>
              <option v-for="node in workflowDefinition.nodes" :key="`target-${node.id}`" :value="node.id">
                {{ node.title }}
              </option>
            </select>
            <button class="text-button" @click="connectWorkflowNodes">连接</button>
          </div>
        </div>
      </div>
      <aside class="workflow-side">
        <div class="side-form-card run-card">
          <span class="eyebrow">RUN PIPELINE</span>
          <h2>执行创作流程</h2>
          <p>流程会逐章读取知识图谱与草稿，生成的内容按项目审批设置写入或进入待审提案。</p>
          <label
            >目标章节
            <select v-model="workflowChapterIds" multiple size="4">
              <option v-for="chapter in chapters" :key="chapter.id" :value="chapter.id">
                {{ chapter.number.toString().padStart(2, '0') }} · {{ chapter.title }}
              </option>
            </select>
          </label>
          <label
            >额外创作要求<textarea v-model="runInstruction" rows="3" placeholder="可选：本次任务的特别要求"></textarea>
          </label>
          <label
            >模型服务商
            <select v-model="providerId">
              <option v-for="provider in providers" :key="provider.id" :value="provider.id">
                {{ provider.name }}{{ provider.isDefault ? '（默认）' : '' }}
              </option>
            </select>
          </label>
          <button class="button primary wide" :disabled="!canEdit || !chapters.length" @click="emit('run')">
            运行工作流 <span>↗</span>
          </button>
          <div class="run-note"><span>●</span> 最多一次处理 20 个章节</div>
        </div>
        <div class="workflow-summary">
          <div>
            <span>流程步骤</span><strong>{{ workflowDefinition.nodes.length }}</strong>
          </div>
          <div>
            <span>节点连接</span><strong>{{ workflowDefinition.edges.length }}</strong>
          </div>
          <div>
            <span>最近执行</span
            ><strong>{{ tasks.filter((item) => item.kind === 'workflow').length ? '已运行' : '—' }}</strong>
          </div>
        </div>
      </aside>
    </div>
  </section>
</template>
