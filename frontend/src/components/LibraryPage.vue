<script setup lang="ts">
import type { Novel } from '../api'

const showCreate = defineModel<boolean>('showCreate', { required: true })
const novelTitle = defineModel<string>('novelTitle', { required: true })
const novelGenre = defineModel<string>('novelGenre', { required: true })
const novelSummary = defineModel<string>('novelSummary', { required: true })
const plannedChapters = defineModel<number>('plannedChapters', { required: true })

defineProps<{
  novels: Novel[]
  stats: { chapterCount: number; words: number; entities: number; pending: number }
  canCreate: boolean
}>()

const emit = defineEmits<{
  create: []
  open: [novel: Novel]
}>()

const coverSymbols = ['雾', '月', '岛', '光']
</script>

<template>
  <section class="page library-page">
    <div class="page-heading library-heading">
      <div>
        <div class="eyebrow">YOUR WRITING SPACE</div>
        <h1>把故事，写成作品。</h1>
        <p>管理正在构建的世界、章节与创作流程。</p>
      </div>
      <button v-if="canCreate" class="button primary" @click="showCreate = !showCreate">
        <span>＋</span> 新建小说
      </button>
    </div>
    <div class="stat-row">
      <div class="stat-card">
        <span>我的作品</span><strong>{{ novels.length.toString().padStart(2, '0') }}</strong
        ><small>持续生长的故事</small>
      </div>
      <div class="stat-card">
        <span>已写章节</span><strong>{{ stats.chapterCount.toString().padStart(2, '0') }}</strong
        ><small>{{ stats.words.toLocaleString() }} 字正文</small>
      </div>
      <div class="stat-card">
        <span>设定实体</span><strong>{{ stats.entities.toString().padStart(2, '0') }}</strong
        ><small>人物、地点与关系</small>
      </div>
      <div class="stat-card accent-stat">
        <span>待审提案</span><strong>{{ stats.pending.toString().padStart(2, '0') }}</strong
        ><small>等待你确认的修改</small>
      </div>
    </div>
    <div v-if="showCreate" class="create-panel">
      <div class="panel-title">
        <div>
          <span class="eyebrow">NEW PROJECT</span>
          <h2>创建一部新小说</h2>
        </div>
        <button class="icon-button" @click="showCreate = false">×</button>
      </div>
      <div class="create-grid">
        <label>作品名称<input v-model="novelTitle" placeholder="例如：雾港来信" maxlength="120" /></label>
        <label>题材<input v-model="novelGenre" placeholder="悬疑 / 奇幻 / 科幻…" /></label>
        <label>计划章节<input v-model.number="plannedChapters" type="number" min="0" max="10000" /></label>
        <label class="span-two"
          >一句话梗概<textarea v-model="novelSummary" rows="2" placeholder="这个故事讲述了……"></textarea>
        </label>
      </div>
      <div class="form-actions">
        <button class="button secondary" @click="showCreate = false">取消</button>
        <button class="button primary" :disabled="!novelTitle.trim()" @click="emit('create')">创建项目</button>
      </div>
    </div>
    <div class="section-heading">
      <div>
        <span class="eyebrow">RECENT PROJECTS</span>
        <h2>继续你的故事</h2>
      </div>
      <span class="muted">{{ novels.length }} 个项目</span>
    </div>
    <div v-if="novels.length" class="novel-grid">
      <button v-for="(novel, index) in novels" :key="novel.id" class="novel-card" @click="emit('open', novel)">
        <div class="novel-card-cover" :class="`cover-${index % 4}`">
          <span class="cover-label">{{ novel.genre || '未分类' }}</span>
          <div class="cover-symbol">{{ coverSymbols[index % 4] }}</div>
          <div class="cover-bottom">
            {{ novel.title.slice(0, 2) }} · STORY NO. {{ (index + 1).toString().padStart(2, '0') }}
          </div>
        </div>
        <div class="novel-card-info">
          <div>
            <h3>{{ novel.title }}</h3>
            <p>{{ novel.summary || '故事正在等待第一个章节。' }}</p>
          </div>
          <span class="arrow-circle">↗</span>
        </div>
        <div class="novel-card-meta">
          <span>{{ novel.chapterCount }} 章</span>
          <span>{{ novel.approvalMode === 'auto' ? '自动写入' : '人工审核' }}</span>
          <span>{{ novel.genre || '待添加题材' }}</span>
        </div>
      </button>
      <button class="new-card" @click="showCreate = true">
        <span class="new-card-plus">＋</span>
        <strong>开启一个新世界</strong>
        <small>从灵感开始，搭建你的下一部作品</small>
      </button>
    </div>
    <div v-else class="empty-state">
      <div class="empty-glyph">卷</div>
      <h3>你的第一部小说，从这里开始</h3>
      <p>创建项目后，可以整理设定、绘制人物关系，再和 Agent 一起完成章节。</p>
      <button class="button primary" @click="showCreate = true">创建第一部小说</button>
    </div>
  </section>
</template>
