<script setup lang="ts">
import { computed } from 'vue'
import type { CharacterState, CharacterStateInput, Chapter, GraphEdge, GraphNode, Novel } from '../api'

const graphForm = defineModel<{ kind: GraphNode['kind']; label: string; summary: string }>('graphForm', {
  required: true,
})
const editingNodeId = defineModel<string>('editingNodeId', { required: true })
const characterStateForm = defineModel<CharacterStateInput>('characterStateForm', { required: true })
const editingCharacterStateId = defineModel<string>('editingCharacterStateId', { required: true })
const edgeForm = defineModel<{ sourceId: string; targetId: string; relation: string; summary: string }>('edgeForm', {
  required: true,
})
const editingEdgeId = defineModel<string>('editingEdgeId', { required: true })

const props = defineProps<{
  novel: Novel
  nodes: GraphNode[]
  edges: GraphEdge[]
  chapters: Chapter[]
  characterStates: CharacterState[]
  canEdit: boolean
}>()

const emit = defineEmits<{
  selectNode: [nodeId: string]
  addNode: []
  saveNode: []
  cancelNode: []
  editNode: [node: GraphNode]
  removeNode: [node: GraphNode]
  saveCharacterState: []
  cancelCharacterState: []
  editCharacterState: [state: CharacterState]
  removeCharacterState: [state: CharacterState]
  addEdge: []
  saveEdge: []
  cancelEdge: []
  editEdge: [edge: GraphEdge]
  removeEdge: [edge: GraphEdge]
}>()

const graphKindLabels: Record<GraphNode['kind'], string> = {
  character: '角色',
  place: '地点',
  faction: '组织',
  item: '物品',
  concept: '设定',
}

const selectedCharacter = computed(() =>
  props.nodes.find((node) => node.id === editingNodeId.value && node.kind === 'character') ?? null,
)
const selectedCharacterStates = computed(() =>
  props.characterStates
    .filter((state) => state.characterId === editingNodeId.value)
    .sort((a, b) => a.chapterNumber - b.chapterNumber),
)
const stateForSelectedChapter = computed(() =>
  selectedCharacterStates.value.find((state) => state.chapterId === characterStateForm.value.chapterId) ?? null,
)

function graphEdgeLabel(edge: GraphEdge) {
  const source = props.nodes.find((node) => node.id === edge.sourceId)?.label ?? '未知实体'
  const target = props.nodes.find((node) => node.id === edge.targetId)?.label ?? '未知实体'
  return `${source} —${edge.relation}→ ${target}`
}

function nodePosition(nodeId: string) {
  const index = props.nodes.findIndex((node) => node.id === nodeId)
  return {
    x: (index % 3) * 250 + 170,
    y: Math.floor(index / 3) * 150 + 118,
  }
}

function onSelectNode(event: Event) {
  emit('selectNode', (event.target as HTMLSelectElement).value)
}
</script>

<template>
  <section class="page knowledge-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">WORLD BIBLE / KNOWLEDGE GRAPH</div>
        <h1>让设定彼此相连。</h1>
        <p>人物、地点、组织与关键物件构成故事的长期记忆。</p>
      </div>
      <div class="graph-count">
        <strong>{{ nodes.length.toString().padStart(2, '0') }}</strong
        ><span>实体节点</span><i></i> <strong>{{ edges.length.toString().padStart(2, '0') }}</strong
        ><span>条关系</span>
      </div>
    </div>
    <div class="knowledge-layout">
      <div class="graph-panel">
        <div class="graph-toolbar">
          <div>
            <span class="eyebrow">RELATION MAP</span><strong>{{ novel.title }}</strong>
          </div>
          <div class="graph-legend">
            <span><i class="legend-person"></i>人物</span><span><i class="legend-place"></i>地点</span
            ><span><i class="legend-thing"></i>其他</span>
          </div>
        </div>
        <div class="graph-stage">
          <svg class="graph-lines" viewBox="0 0 900 550" preserveAspectRatio="none">
            <line
              v-for="edge in edges"
              :key="edge.id"
              :x1="nodePosition(edge.sourceId).x"
              :y1="nodePosition(edge.sourceId).y"
              :x2="nodePosition(edge.targetId).x"
              :y2="nodePosition(edge.targetId).y"
            />
            <text
              v-for="edge in edges"
              :key="`${edge.id}-label`"
              :x="(nodePosition(edge.sourceId).x + nodePosition(edge.targetId).x) / 2"
              :y="(nodePosition(edge.sourceId).y + nodePosition(edge.targetId).y) / 2 - 7"
            >
              {{ edge.relation }}
            </text>
          </svg>
          <div
            v-for="(node, index) in nodes"
            :key="node.id"
            class="graph-node"
            :class="`node-${node.kind}`"
            :style="{ left: `${80 + (index % 3) * 250}px`, top: `${55 + Math.floor(index / 3) * 150}px` }"
          >
            <span class="node-kind">{{ graphKindLabels[node.kind] }}</span>
            <strong>{{ node.label }}</strong>
            <small>{{ node.summary || '尚未补充描述' }}</small>
          </div>
          <div v-if="!nodes.length" class="graph-empty">
            <span>⌘</span>
            <strong>故事的世界还等待第一个锚点</strong>
            <small>添加一位人物或一个地点，开始建立设定关系。</small>
          </div>
        </div>
        <div class="graph-foot">
          <span>拖动设定卡片调整布局暂不可用</span><span>知识图谱会作为 Agent 创作上下文</span>
        </div>
      </div>
      <aside class="graph-side">
        <div class="side-form-card">
          <span class="eyebrow">{{ editingNodeId ? (canEdit ? 'EDIT ENTITY' : 'VIEW ENTITY') : 'ADD ENTITY' }}</span>
          <h2>{{ editingNodeId ? (canEdit ? '编辑设定实体' : '查看设定实体') : '添加设定实体' }}</h2>
          <label v-if="nodes.length"
            >选择实体查看或编辑
            <select :value="editingNodeId" @change="onSelectNode">
              <option value="">新增实体</option>
              <option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.label }}</option>
            </select>
          </label>
          <label
            >类型
            <select v-model="graphForm.kind" :disabled="!canEdit">
              <option value="character">人物角色</option>
              <option value="place">故事地点</option>
              <option value="faction">组织势力</option>
              <option value="item">关键物件</option>
              <option value="concept">世界设定</option>
            </select>
          </label>
          <label
            >名称<input v-model="graphForm.label" placeholder="例如：沈知遥" maxlength="80" :readonly="!canEdit"
          /></label>
          <label
            >描述<textarea
              v-model="graphForm.summary"
              rows="3"
              placeholder="身份、动机或与故事相关的事实"
              :readonly="!canEdit"
            ></textarea>
          </label>
          <div class="graph-form-actions">
            <button
              v-if="canEdit"
              class="button primary wide"
              :disabled="!canEdit || !graphForm.label.trim()"
              @click="editingNodeId ? emit('saveNode') : emit('addNode')"
            >
              {{ editingNodeId ? '保存实体修改' : '加入图谱' }} <span>{{ editingNodeId ? '✓' : '＋' }}</span>
            </button>
            <button v-if="editingNodeId && canEdit" class="text-button" @click="emit('cancelNode')">取消</button>
          </div>
          <div v-if="nodes.length" class="graph-item-list">
            <div v-for="node in nodes" :key="node.id" class="graph-item-row">
              <div>
                <strong>{{ node.label }}</strong
                ><small>{{ graphKindLabels[node.kind] }}</small>
              </div>
              <div class="graph-item-actions">
                <button :disabled="!canEdit" @click="emit('editNode', node)">编辑</button>
                <button :disabled="!canEdit" @click="emit('removeNode', node)">删除</button>
              </div>
            </div>
          </div>
        </div>
        <div v-if="selectedCharacter" class="side-form-card character-state-card">
          <span class="eyebrow">CHARACTER TIMELINE</span>
          <h2>{{ selectedCharacter.label }} · 章节状态</h2>
          <p class="side-form-hint">记录人物在不同章节的称谓、年龄、能力与立场；最近一条记录会用于后续章节上下文。</p>
          <div v-if="canEdit" class="character-state-form">
            <label
              >生效章节
              <select v-model="characterStateForm.chapterId" :disabled="Boolean(editingCharacterStateId)">
                <option value="">选择章节</option>
                <option v-for="chapter in chapters" :key="chapter.id" :value="chapter.id">
                  第 {{ chapter.number }} 章 · {{ chapter.title }}
                </option>
              </select>
            </label>
            <label
              >称谓<input v-model="characterStateForm.appellation" maxlength="160" placeholder="例如：巡夜人、沈先生"
            /></label>
            <label
              >年龄<input v-model="characterStateForm.age" maxlength="80" placeholder="例如：二十七岁、外表约四十岁"
            /></label>
            <label
              >能力<textarea v-model="characterStateForm.abilities" rows="2" maxlength="1200" placeholder="能力与限制"></textarea
            ></label>
            <label
              >立场<textarea v-model="characterStateForm.stance" rows="2" maxlength="1200" placeholder="当前目标、阵营或态度"></textarea
            ></label>
            <label
              >补充说明<textarea v-model="characterStateForm.notes" rows="2" maxlength="2000" placeholder="变化依据或其他状态"></textarea
            ></label>
            <p v-if="stateForSelectedChapter && !editingCharacterStateId" class="character-state-existing-hint">
              该章节已有记录，请在下方选择“编辑”。
            </p>
            <div class="graph-form-actions">
              <button
                class="button secondary wide"
                :disabled="!characterStateForm.chapterId || (stateForSelectedChapter && !editingCharacterStateId)"
                @click="emit('saveCharacterState')"
              >
                {{ editingCharacterStateId ? '更新章节状态' : '保存章节状态' }}
              </button>
              <button v-if="editingCharacterStateId" class="text-button" @click="emit('cancelCharacterState')">取消编辑</button>
            </div>
          </div>
          <div v-if="selectedCharacterStates.length" class="graph-item-list character-state-list">
            <div v-for="state in selectedCharacterStates" :key="state.id" class="graph-item-row character-state-row">
              <div>
                <strong>第 {{ state.chapterNumber }} 章 · {{ state.chapterTitle }}</strong>
                <small>{{ [state.appellation, state.age, state.abilities, state.stance, state.notes].filter(Boolean).join(' · ') || '未填写状态' }}</small>
              </div>
              <div v-if="canEdit" class="graph-item-actions">
                <button :disabled="!canEdit" @click="emit('editCharacterState', state)">编辑</button>
                <button :disabled="!canEdit" @click="emit('removeCharacterState', state)">删除</button>
              </div>
            </div>
          </div>
          <p v-else class="character-state-empty">尚未记录章节状态。</p>
        </div>
        <div class="side-form-card">
          <span class="eyebrow">{{ editingEdgeId ? 'EDIT RELATION' : 'DEFINE RELATION' }}</span>
          <h2>{{ editingEdgeId ? '编辑关系' : '连接两个实体' }}</h2>
          <label
            >起点
            <select v-model="edgeForm.sourceId" :disabled="!canEdit">
              <option value="">选择实体</option>
              <option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.label }}</option>
            </select>
          </label>
          <label
            >关系名称<input v-model="edgeForm.relation" placeholder="例如：旧识、隶属、持有" :readonly="!canEdit"
          /></label>
          <label
            >终点
            <select v-model="edgeForm.targetId" :disabled="!canEdit">
              <option value="">选择实体</option>
              <option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.label }}</option>
            </select>
          </label>
          <label
            >补充说明<input v-model="edgeForm.summary" placeholder="关系背景或限制（可选）" :readonly="!canEdit"
          /></label>
          <div class="graph-form-actions">
            <button
              class="button secondary wide"
              :disabled="
                !canEdit ||
                nodes.length < 2 ||
                !edgeForm.sourceId ||
                !edgeForm.targetId ||
                edgeForm.sourceId === edgeForm.targetId ||
                !edgeForm.relation.trim()
              "
              @click="editingEdgeId ? emit('saveEdge') : emit('addEdge')"
            >
              {{ editingEdgeId ? '保存关系修改' : '建立关系' }} <span>↗</span>
            </button>
            <button v-if="editingEdgeId" class="text-button" @click="emit('cancelEdge')">取消</button>
          </div>
          <div v-if="edges.length" class="graph-item-list">
            <div v-for="edge in edges" :key="edge.id" class="graph-item-row">
              <strong>{{ graphEdgeLabel(edge) }}</strong>
              <div class="graph-item-actions">
                <button :disabled="!canEdit" @click="emit('editEdge', edge)">编辑</button>
                <button :disabled="!canEdit" @click="emit('removeEdge', edge)">删除</button>
              </div>
            </div>
          </div>
        </div>
      </aside>
    </div>
  </section>
</template>
