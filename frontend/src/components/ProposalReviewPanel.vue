<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Chapter, Proposal, ProposalSelection } from '../api'
import { diffParagraphs, type ParagraphDiff } from '../proposal-diff'

type DiffBlock =
  | { kind: 'same'; id: number; paragraphs: ParagraphDiff[] }
  | { kind: 'change'; hunk: number; paragraphs: ParagraphDiff[] }

function groupDiffBlocks(diff: ParagraphDiff[]): DiffBlock[] {
  const blocks: DiffBlock[] = []
  for (const paragraph of diff) {
    const kind = paragraph.kind === 'same' ? 'same' : 'change'
    const previous = blocks.at(-1)
    if (previous?.kind === kind) {
      previous.paragraphs.push(paragraph)
      continue
    }
    blocks.push(
      kind === 'same'
        ? { kind, id: blocks.length, paragraphs: [paragraph] }
        : { kind, hunk: blocks.filter((block) => block.kind === 'change').length, paragraphs: [paragraph] },
    )
  }
  return blocks
}

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
  applySelected: [proposal: Proposal, selection: ProposalSelection]
  reject: [proposal: Proposal]
  regenerate: [proposal: Proposal]
}>()

const selectedHunks = ref<Record<string, number[]>>({})
const selectedTitles = ref<Record<string, boolean>>({})

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
        blocks: groupDiffBlocks(diffParagraphs(chapter?.body ?? '', proposal.text)),
        titleChanged: !!chapter && chapter.title !== proposal.title,
      }
    }),
)

function isHunkSelected(proposalId: string, hunk: number) {
  return selectedHunks.value[proposalId]?.includes(hunk) ?? false
}

function changeHunkSelected(proposalId: string, hunk: number, event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const selected = new Set(selectedHunks.value[proposalId] ?? [])
  if (checked) selected.add(hunk)
  else selected.delete(hunk)
  selectedHunks.value[proposalId] = [...selected].sort((left, right) => left - right)
}

function changeTitleSelected(proposalId: string, event: Event) {
  selectedTitles.value[proposalId] = (event.target as HTMLInputElement).checked
}

function hasSelectedChanges(proposal: Proposal) {
  return (selectedHunks.value[proposal.id]?.length ?? 0) > 0 || !!selectedTitles.value[proposal.id]
}

function applySelected(review: (typeof reviews.value)[number]) {
  emit('applySelected', review.proposal, {
    selectedHunks: [...(selectedHunks.value[review.proposal.id] ?? [])],
    applyTitle: !!selectedTitles.value[review.proposal.id],
  })
}
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
        <summary>查看差异并选择要采纳的修改</summary>
        <div class="proposal-diff-list">
          <template
            v-for="block in review.blocks"
            :key="block.kind === 'change' ? `hunk:${block.hunk}` : `same:${block.id}`"
          >
            <div v-if="block.kind === 'change'" class="proposal-diff-hunk">
              <label class="proposal-hunk-control">
                <input
                  type="checkbox"
                  :checked="isHunkSelected(review.proposal.id, block.hunk)"
                  :disabled="!canReview || review.stale || review.localDraftPending || savingChapter"
                  @change="changeHunkSelected(review.proposal.id, block.hunk, $event)"
                />
                采纳这组段落修改
              </label>
              <p
                v-for="(paragraph, index) in block.paragraphs"
                :key="`${index}:${paragraph.kind}`"
                :class="`diff-${paragraph.kind}`"
              >
                <span>{{ paragraph.kind === 'added' ? '+' : '−' }}</span
                >{{ paragraph.text }}
              </p>
            </div>
            <template v-else>
              <p v-for="(paragraph, index) in block.paragraphs" :key="`${index}:${paragraph.kind}`" class="diff-same">
                <span> </span>{{ paragraph.text }}
              </p>
            </template>
          </template>
          <p v-if="!review.blocks.length" class="diff-empty">正文与提案内容相同。</p>
        </div>
      </details>
      <label v-if="review.titleChanged" class="proposal-title-choice">
        <input
          type="checkbox"
          :checked="!!selectedTitles[review.proposal.id]"
          :disabled="!canReview || review.stale || review.localDraftPending || savingChapter"
          @change="changeTitleSelected(review.proposal.id, $event)"
        />
        同时采纳提案标题：{{ review.proposal.title }}
      </label>
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
          class="button secondary"
          :disabled="
            !canReview ||
            review.stale ||
            review.localDraftPending ||
            savingChapter ||
            !hasSelectedChanges(review.proposal)
          "
          @click="applySelected(review)"
        >
          应用所选修改
        </button>
        <button
          class="button primary"
          :disabled="!canReview || review.stale || review.localDraftPending || savingChapter"
          @click="emit('apply', review.proposal)"
        >
          {{ review.stale ? '版本已过期' : review.localDraftPending ? '请先处理草稿' : '全部应用到正文' }}
        </button>
      </div>
    </article>
  </div>
</template>
