<script setup lang="ts">
import type { NovelTask } from '../api'

defineProps<{
  task: NovelTask | null
  demoMode: boolean
  progress: number
  chapterProgress: Array<{ chapterId: string; chapterNumber: number; title: string; status: string }>
  retrying: boolean
  canEdit: boolean
  streamText: string
  contextSources: string[]
  contextTruncation: { entities: number; relations: number }
}>()

const emit = defineEmits<{
  cancel: []
  retry: []
}>()
</script>

<template>
  <div v-if="task" class="task-panel">
    <p v-if="demoMode" class="assistant-hint">
      当前为演示模式，生成内容仅用于验证流程，不会自动写入正文；请勿将占位内容作为真实稿件。
    </p>
    <div class="task-panel-head">
      <div>
        <span class="eyebrow">LIVE GENERATION</span>
        <h3>
          {{
            task.status === 'completed'
              ? '生成任务已完成'
              : task.status === 'failed'
                ? '生成任务失败'
                : task.status === 'canceled'
                  ? '生成任务已取消'
                  : task.status === 'interrupted'
                    ? '生成任务已中断'
                    : '正在为你构建章节'
          }}
        </h3>
      </div>
      <div class="task-panel-actions">
        <span class="task-percent">{{ progress }}%</span>
        <button
          v-if="['failed', 'canceled', 'interrupted'].includes(task.status) && canEdit"
          class="button secondary"
          :disabled="retrying"
          @click="emit('retry')"
        >
          {{ retrying ? '准备重试…' : '重试未完成章节' }}
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
    <div v-if="chapterProgress.length" class="chapter-progress-list">
      <div v-for="chapter in chapterProgress" :key="chapter.chapterId" class="chapter-progress-item">
        <span>第 {{ chapter.chapterNumber }} 章 · {{ chapter.title }}</span>
        <small :class="chapter.status">{{
          chapter.status === 'completed'
            ? '已完成'
            : chapter.status === 'failed'
              ? '失败，可重试'
              : chapter.status === 'canceled'
                ? '已取消'
                : chapter.status === 'queued'
                  ? '等待中'
                  : '生成中'
        }}</small>
      </div>
    </div>
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
</template>
