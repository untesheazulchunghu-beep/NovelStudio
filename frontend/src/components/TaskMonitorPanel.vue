<script setup lang="ts">
import type { ContinuityFinding, NovelTask } from '../api'

defineProps<{
  task: NovelTask | null
  demoMode: boolean
  progress: number
  chapterProgress: Array<{ chapterId: string; chapterNumber: number; title: string; status: string }>
  nodeProgress: Array<{ nodeId: string; title: string; status: string }>
  reviewItems: Array<{
    event: string
    status: string
    quote: string
    location: string
    modelJudgment: string
    quoteVerbatim: boolean
  }>
  continuityFindings: ContinuityFinding[]
  retrying: boolean
  canEdit: boolean
  streamText: string
  contextSources: string[]
  droppedContext: string[]
  contextTruncation: { entities: number; relations: number }
}>()

const emit = defineEmits<{
  cancel: []
  retry: []
}>()

function taskTitle(task: NovelTask | null) {
  if (!task) return ''
  const titles: Record<string, Record<string, string>> = {
    continuity: {
      completed: '连续性检查已完成',
      failed: '连续性检查失败',
      canceled: '连续性检查已取消',
      interrupted: '连续性检查已中断',
      running: '正在检查连续性',
      queued: '正在检查连续性',
    },
    review: {
      completed: '审稿清单已完成',
      failed: '审稿清单失败',
      canceled: '审稿清单已取消',
      interrupted: '审稿清单已中断',
      running: '正在核对关键事件',
      queued: '正在核对关键事件',
    },
  }
  const knownTitle = titles[task.kind]?.[task.status]
  if (knownTitle) return knownTitle
  if (task.status === 'completed') return '生成任务已完成'
  if (task.status === 'failed') return '生成任务失败'
  if (task.status === 'canceled') return '生成任务已取消'
  if (task.status === 'interrupted') return '生成任务已中断'
  return '正在为你构建章节'
}

function retryLabel(task: NovelTask) {
  if (task.kind === 'workflow') return '重试未完成节点'
  if (task.kind === 'review') return '重试审稿'
  if (task.kind === 'continuity') return '重试连续性检查'
  return '重试未完成章节'
}

function chapterStatusLabel(task: NovelTask, status: string) {
  if (status === 'completed') return '已完成'
  if (status === 'failed') return '失败，可重试'
  if (status === 'canceled') return '已取消'
  if (status === 'queued') return '等待中'
  if (task.kind === 'continuity') return '检查中'
  if (task.kind === 'review') return '审稿中'
  return '生成中'
}
</script>

<template>
  <div v-if="task" class="task-panel">
    <p v-if="demoMode" class="assistant-hint">
      当前为演示模式，生成内容仅用于验证流程，不会自动写入正文；请勿将占位内容作为真实稿件。
    </p>
    <div class="task-panel-head">
      <div>
        <span class="eyebrow">
          {{
            task.kind === 'continuity'
              ? 'CONTINUITY REVIEW'
              : task.kind === 'review'
                ? 'KEY EVENT REVIEW'
                : 'LIVE GENERATION'
          }}
        </span>
        <h3>{{ taskTitle(task) }}</h3>
      </div>
      <div class="task-panel-actions">
        <span class="task-percent">{{ progress }}%</span>
        <button
          v-if="['failed', 'canceled', 'interrupted'].includes(task.status) && canEdit"
          class="button secondary"
          :disabled="retrying"
          @click="emit('retry')"
        >
          {{ retrying ? '准备重试…' : retryLabel(task) }}
        </button>
        <button
          v-if="['queued', 'running'].includes(task.status)"
          class="button cancel-task-button"
          @click="emit('cancel')"
        >
          取消任务
        </button>
      </div>
    </div>
    <div class="progress-track"><span :style="{ width: `${progress}%` }"></span></div>
    <div v-if="(task.totalTokens ?? 0) > 0" class="task-usage-breakdown">
      <strong>本任务 {{ (task.totalTokens ?? 0).toLocaleString() }} Token</strong>
      <span>输入 {{ (task.promptTokens ?? 0).toLocaleString() }} · 输出 {{ (task.completionTokens ?? 0).toLocaleString() }}</span>
      <small>
        {{ (task.reportedTokenCalls ?? 0).toLocaleString() }} 次实际用量
        <template v-if="task.estimatedTokenCalls">· {{ (task.estimatedTokenCalls ?? 0).toLocaleString() }} 次估算用量</template>
      </small>
      <small>费用：{{ task.costKnown ? `$${(task.estimatedCostUsd ?? 0).toFixed(8)} USD` : '未配置费率' }}</small>
    </div>
    <div v-if="chapterProgress.length" class="chapter-progress-list">
      <div v-for="chapter in chapterProgress" :key="chapter.chapterId" class="chapter-progress-item">
        <span>第 {{ chapter.chapterNumber }} 章 · {{ chapter.title }}</span>
        <small :class="chapter.status">{{ chapterStatusLabel(task, chapter.status) }}</small>
      </div>
    </div>
    <div v-if="nodeProgress.length" class="chapter-progress-list">
      <div v-for="(node, index) in nodeProgress" :key="`${index}:${node.nodeId}`" class="chapter-progress-item">
        <span>{{ node.title }}</span>
        <small :class="node.status === 'succeeded' ? 'completed' : node.status">{{
          node.status === 'succeeded'
            ? '已完成'
            : node.status === 'failed'
              ? '失败'
              : node.status === 'canceled'
                ? '已取消'
                : '执行中'
        }}</small>
      </div>
    </div>
    <p v-if="task.kind === 'continuity'" class="assistant-hint">
      这些是模型给出的参考提示；只有正文引文和来源引文都能逐字核对时才标为可核对。任务不会修改正文。
    </p>
    <div v-if="task.kind === 'continuity' && continuityFindings.length" class="continuity-finding-list">
      <article
        v-for="(item, index) in continuityFindings"
        :key="`${item.chapterId}:${index}`"
        class="continuity-finding"
      >
        <div class="continuity-finding-head">
          <strong>第 {{ item.chapterNumber }} 章 · {{ item.title }} · {{ item.category }}</strong>
          <small :class="item.status === '引文可核对' ? 'completed' : 'queued'">{{ item.status }}</small>
        </div>
        <p v-if="item.subject" class="continuity-subject">{{ item.subject }}</p>
        <p>{{ item.issue }}</p>
        <small v-if="item.currentQuote">正文引文：{{ item.currentQuote }}</small>
        <small v-if="item.referenceSource || item.referenceQuote">
          对照依据{{ item.referenceSource ? `（${item.referenceSource}）` : '' }}：{{ item.referenceQuote || '未提供引文' }}
        </small>
        <small>
          模型把握 {{ item.confidence }} · 正文引文{{ item.currentQuoteVerbatim ? '逐字相符' : '待核实' }} ·
          依据引文{{ item.referenceQuoteVerbatim ? '逐字相符' : '待核实' }}
        </small>
      </article>
    </div>
    <p v-else-if="task.kind === 'continuity' && task.status === 'completed'" class="assistant-hint">
      没有提示出明显的连续性问题；正文未改动。
    </p>
    <div v-if="task.kind === 'review' && reviewItems.length" class="chapter-progress-list">
      <div v-for="(item, index) in reviewItems" :key="`${index}:${item.event}`" class="chapter-progress-item">
        <span>{{ item.event }} · {{ item.status }}</span>
        <small :class="item.status === '已完成' ? 'completed' : item.status === '未完成' ? 'failed' : 'queued'">
          模型判断 {{ item.modelJudgment || '未给出' }} · 引文{{ item.quoteVerbatim ? '逐字相符' : '不是逐字子串' }}
        </small>
        <small v-if="item.quote"
          >引文：{{ item.quote }}<template v-if="item.location"> · {{ item.location }}</template></small
        >
      </div>
    </div>
    <p v-else-if="task.kind === 'review' && task.status === 'completed'" class="assistant-hint">
      这一章的关键事件没有非空行，审稿清单为空，没有编造事件。
    </p>
    <div class="stream-preview">
      {{ streamText || '正在连接实时任务流…'
      }}<span v-if="['queued', 'running'].includes(task.status)" class="stream-cursor"></span>
    </div>
  </div>
  <div
    v-if="task && (contextTruncation.entities || contextTruncation.relations)"
    class="context-truncation"
    role="status"
  >
    本次知识图谱上下文已截断 {{ contextTruncation.entities }} 个实体、{{ contextTruncation.relations }} 条关系。
  </div>
  <details v-if="task && contextSources.length" class="task-context">
    <summary>本次注入资料 · {{ contextSources.length }} 项</summary>
    <ul>
      <li v-for="(source, index) in contextSources" :key="`${index}:${source}`">{{ source }}</li>
    </ul>
  </details>
  <details v-if="task && droppedContext.length" class="task-context">
    <summary>本次未注入 · {{ droppedContext.length }} 项</summary>
    <ul>
      <li v-for="(source, index) in droppedContext" :key="`${index}:${source}`">{{ source }}</li>
    </ul>
  </details>
</template>
