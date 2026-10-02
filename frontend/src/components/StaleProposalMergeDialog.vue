<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ProposalMergeInput, ProposalMergePreview } from '../api'

const props = defineProps<{ preview: ProposalMergePreview; busy: boolean }>()
const emit = defineEmits<{
  close: []
  apply: [input: ProposalMergeInput]
}>()

const mergedTitle = ref('')
const mergedText = ref('')
const hasChanges = computed(
  () => mergedTitle.value.trim() !== props.preview.currentTitle || mergedText.value !== props.preview.currentText,
)

watch(
  () => props.preview,
  (preview) => {
    mergedTitle.value = preview.currentTitle
    mergedText.value = preview.currentText
  },
  { immediate: true },
)

function loadCurrent() {
  mergedTitle.value = props.preview.currentTitle
  mergedText.value = props.preview.currentText
}

function loadProposal() {
  mergedTitle.value = props.preview.proposalTitle
  mergedText.value = props.preview.proposalText
}

function requestClose() {
  if (props.busy) return
  if (hasChanges.value && !window.confirm('关闭会丢弃尚未保存的合并稿，仍要关闭吗？')) return
  emit('close')
}

function submit() {
  emit('apply', {
    expectedRevision: props.preview.currentRevision,
    title: mergedTitle.value,
    text: mergedText.value,
  })
}
</script>

<template>
  <div class="merge-dialog-backdrop" @click.self="requestClose" @keydown.esc="requestClose">
    <section class="merge-dialog" role="dialog" aria-modal="true" aria-labelledby="merge-dialog-title">
      <header class="merge-dialog-header">
        <div>
          <span class="eyebrow">THREE-WAY REVIEW</span>
          <h2 id="merge-dialog-title">合并过期提案</h2>
          <p>
            以 v{{ preview.baseRevision }} 为共同基准，比较当前 v{{ preview.currentRevision }} 与生成提案，整理合并稿。
          </p>
        </div>
        <button class="button text-button" type="button" :disabled="busy" @click="requestClose">关闭</button>
      </header>

      <div class="merge-versions">
        <article class="merge-version-card">
          <span class="eyebrow">共同基准 · v{{ preview.baseRevision }}</span>
          <h3>{{ preview.baseTitle }}</h3>
          <pre>{{ preview.baseText || '（空正文）' }}</pre>
        </article>
        <article class="merge-version-card current">
          <span class="eyebrow">当前正文 · v{{ preview.currentRevision }}</span>
          <h3>{{ preview.currentTitle }}</h3>
          <pre>{{ preview.currentText || '（空正文）' }}</pre>
        </article>
        <article class="merge-version-card proposed">
          <span class="eyebrow">生成提案 · 基于 v{{ preview.baseRevision }}</span>
          <h3>{{ preview.proposalTitle }}</h3>
          <pre>{{ preview.proposalText || '（空正文）' }}</pre>
        </article>
      </div>

      <section class="merge-draft">
        <div class="merge-draft-heading">
          <div>
            <span class="eyebrow">YOUR MERGED VERSION</span>
            <h3>合并稿</h3>
          </div>
          <div class="merge-draft-actions">
            <button class="button secondary" type="button" :disabled="busy" @click="loadCurrent">重置为当前正文</button>
            <button class="button secondary" type="button" :disabled="busy" @click="loadProposal">载入提案全文</button>
          </div>
        </div>
        <label class="merge-title-field">
          合并后标题
          <input v-model="mergedTitle" maxlength="120" :disabled="busy" />
        </label>
        <label class="merge-body-field">
          合并后正文
          <textarea v-model="mergedText" rows="14" :disabled="busy"></textarea>
        </label>
        <p class="merge-safety-note">保存时会再次检查当前章节版本；若预览后正文又有更新，需要重新打开合并窗口。</p>
      </section>

      <footer class="merge-dialog-footer">
        <button class="button secondary" type="button" :disabled="busy" @click="requestClose">取消</button>
        <button
          class="button primary"
          type="button"
          :disabled="busy || !hasChanges || !mergedTitle.trim() || mergedTitle.trim().length > 120"
          @click="submit"
        >
          {{ busy ? '正在保存…' : '保存合并稿并处理提案' }}
        </button>
      </footer>
    </section>
  </div>
</template>
