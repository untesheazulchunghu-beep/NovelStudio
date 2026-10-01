<script setup lang="ts">
import type { Novel } from '../api'

defineProps<{
  novels: Novel[]
  canManageNovel: (novel: Novel) => boolean
}>()

const emit = defineEmits<{
  restore: [novel: Novel]
}>()
</script>

<template>
  <section class="page trash-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">RECOVERY / ARCHIVE</div>
        <h1>回收站</h1>
        <p>移入回收站的作品会保留原有章节、设定和历史记录，可随时恢复。</p>
      </div>
    </div>
    <div v-if="novels.length" class="trash-list">
      <article v-for="novel in novels" :key="novel.id" class="trash-item">
        <div>
          <span class="eyebrow"
            >{{ novel.genre || 'NOVEL PROJECT' }} · 已删除
            {{ new Date(novel.deletedAt ?? '').toLocaleDateString('zh-CN') }}</span
          >
          <h2>{{ novel.title }}</h2>
          <p>{{ novel.summary || '没有填写作品简介' }}</p>
        </div>
        <button class="button secondary" :disabled="!canManageNovel(novel)" @click="emit('restore', novel)">
          恢复作品
        </button>
      </article>
    </div>
    <div v-else class="empty-state">
      <div class="empty-glyph">⌫</div>
      <h3>回收站还是空的</h3>
      <p>移入回收站的作品会显示在这里。</p>
    </div>
  </section>
</template>
