<script setup lang="ts">
import { computed } from 'vue'
import type { Chapter, Proposal } from '../api'
import { diffParagraphs } from '../proposal-diff'

const props = defineProps<{
  proposals: Proposal[]
  chapters: Chapter[]
  activeChapterId: string
  canReview: boolean
  canEdit: boolean
  hasUnsavedChanges: boolean
  savingChapter: boolean
  activeTaskRunning: boolean
}>()

const emit = defineEmits<{
  apply: [proposal: Proposal]
  reject: [proposal: Proposal]
  regenerate: [proposal: Proposal]
}>()

const reviews = computed(() =>
  props.proposals
    .filter((proposal) => proposal.status === 'pending')
    .map((proposal) => {
      const chapter = props.chapters.find((item) => item.id === proposal.chapterId)
      return {
        proposal,
        stale: !chapter || chapter.revision !== proposal.baseRevision,
        currentRevision: chapter?.revision ?? null,
        localDraftPending: proposal.chapterId === props.activeChapterId && props.hasUnsavedChanges,
        diff: diffParagraphs(chapter?.body ?? '', proposal.text),
      }
    }),
)
</script>

<template>
  <div v-if="reviews.length" class="proposal-section">
    <div class="section-heading">
      <div>
        <span class="eyebrow">REVIEW QUEUE</span>
        <h2>
          待审核提案 <span class="count-pill">{{ reviews.length }}</span>
        </h2>
      </div>
    </div>
    <article v-for="review in reviews" :key="review.proposal.id" class="proposal-card">
      <div class="proposal-card-head">
        <div>
          <span class="proposal-label">第 {{ review.proposal.chapterNumber }} 章 · 修改提案</span>
          <h3>{{ review.proposal.title }}</h3>
        </div>
        <span class="revision-chip" :class="{ stale: review.stale }">{{
          review.stale ? `已过期 · 基于 v${review.proposal.baseRevision}` : `基于 v${review.proposal.baseRevision}`
        }}</span>
      </div>
      <p v-if="review.stale" class="stale-proposal-hint">
        当前章节版本为 v{{ review.currentRevision ?? '—' }}。差异视图将当前正文与提案比较。
      </p>
      <p v-if="review.proposal.demoMode" class="stale-proposal-hint">
        这是演示模式内容。应用后会更新正文，但不会生成章节记忆；配置真实模型后可手动刷新记忆。
      </p>
      <p v-if="review.localDraftPending" class="stale-proposal-hint">
        该章节有未保存的本地草稿；先保存或放弃草稿，再应用提案。
      </p>
      <details class="proposal-diff-toggle">
        <summary>查看当前正文与提案的段落差异</summary>
        <div class="proposal-diff-list">
          <p
            v-for="(paragraph, index) in review.diff"
            :key="`${index}:${paragraph.kind}`"
            :class="`diff-${paragraph.kind}`"
          >
            <span>{{ paragraph.kind === 'added' ? '+' : paragraph.kind === 'removed' ? '−' : ' ' }}</span
            >{{ paragraph.text }}
          </p>
          <p v-if="!review.diff.length" class="diff-empty">正文与提案内容相同。</p>
        </div>
      </details>
      <p class="proposal-text">{{ review.proposal.text }}</p>
      <div class="proposal-actions">
        <button class="button text-button" :disabled="!canReview" @click="emit('reject', review.proposal)">拒绝</button>
        <button
          v-if="review.stale"
          class="button secondary"
          :disabled="!canEdit || savingChapter || review.localDraftPending || activeTaskRunning"
          @click="emit('regenerate', review.proposal)"
        >
          基于最新版重新生成
        </button>
        <button
          class="button primary"
          :disabled="!canReview || review.stale || review.localDraftPending"
          @click="emit('apply', review.proposal)"
        >
          {{ review.stale ? '版本已过期' : review.localDraftPending ? '请先处理草稿' : '应用到正文' }}
        </button>
      </div>
    </article>
  </div>
</template>
