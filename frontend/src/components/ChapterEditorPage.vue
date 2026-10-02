<script setup lang="ts">
import { computed } from 'vue'
import type { Chapter, ChapterRevision, GraphEdge, ModelProvider, Novel, NovelTask } from '../api'
import { countManuscriptCharacters } from '../word-count'

const draftTitle = defineModel<string>('draftTitle', { required: true })
const draftBody = defineModel<string>('draftBody', { required: true })
const draftGoal = defineModel<string>('draftGoal', { required: true })
const draftKeyEvents = defineModel<string>('draftKeyEvents', { required: true })
const draftHook = defineModel<string>('draftHook', { required: true })
const guideDraft = defineModel<string>('guideDraft', { required: true })
const runInstruction = defineModel<string>('runInstruction', { required: true })
const providerId = defineModel<string>('providerId', { required: true })
const selectedChapterIds = defineModel<string[]>('selectedChapterIds', { required: true })
const showBlueprint = defineModel<boolean>('showBlueprint', { required: true })
const showHistory = defineModel<boolean>('showHistory', { required: true })

defineProps<{
  novel: Novel
  chapters: Chapter[]
  deletedChapters: Chapter[]
  activeChapterId: string
  activeChapter: Chapter | null
  wordTotal: number
  roleLabel: string
  canEdit: boolean
  savingChapter: boolean
  refreshingMemory: boolean
  unsaved: boolean
  autoSaveState: 'saved' | 'pending' | 'saving' | 'error'
  saveConflict: { remote: Chapter; baseRevision: number } | null
  chapterRevisions: ChapterRevision[]
  loadingHistory: boolean
  edges: GraphEdge[]
  entityCount: number
  tasks: NovelTask[]
  providers: ModelProvider[]
  savingGuide: boolean
  markdownHref: string
  textHref: string
  activeTaskRunning: boolean
}>()

const emit = defineEmits<{
  updateApproval: [event: Event]
  addChapter: []
  saveChapter: []
  refreshMemory: []
  deleteNovel: []
  selectChapter: [chapterId: string]
  moveChapter: [chapterId: string, offset: -1 | 1]
  deleteChapter: []
  restoreChapter: [chapter: Chapter]
  toggleHistory: []
  restoreRevision: [revision: ChapterRevision]
  discardConflict: []
  saveConflict: []
  saveGuide: []
  startBatch: []
  startReview: []
  startContinuityCheck: []
  resumeTask: [task: NovelTask]
  openKnowledge: []
}>()

const revisionSourceLabels: Record<string, string> = {
  manual: '手动编辑前',
  proposal: '提案应用前',
  'agent-auto': '自动写入前',
  restore: '版本恢复前',
}

const wordCount = computed(() => countManuscriptCharacters(draftBody.value))
</script>

<template>
  <section class="page writing-page">
    <div class="writing-head">
      <div>
        <div class="eyebrow">MANUSCRIPT / {{ novel.genre || 'NOVEL PROJECT' }}</div>
        <h1>{{ novel.title }}</h1>
        <div class="writing-meta">
          <span>{{ chapters.length }} 个章节</span><span class="meta-dot"></span>
          <span>{{ wordTotal.toLocaleString() }} 字</span><span class="meta-dot"></span>
          <span class="role-chip">{{ roleLabel }}</span>
        </div>
      </div>
      <div class="writing-actions">
        <label class="approval-setting">
          <span>Agent 写入</span>
          <select :value="novel.approvalMode" :disabled="!canEdit" @change="emit('updateApproval', $event)">
            <option value="manual">人工审核</option>
            <option value="auto">自动写入</option>
          </select>
        </label>
        <a class="button secondary" :href="markdownHref" download>导出 Markdown</a>
        <a class="button secondary" :href="textHref" download>导出 TXT</a>
        <button v-if="canEdit" class="button secondary" @click="emit('addChapter')">＋ 新章节</button>
        <button v-if="canEdit" class="button primary" :disabled="savingChapter" @click="emit('saveChapter')">
          {{ savingChapter ? '保存中…' : '保存章节' }}
        </button>
      </div>
    </div>
    <div v-if="canEdit" class="writing-danger-actions">
      <span>整部作品可从这里移入回收站，所有内容都可以恢复。</span>
      <button class="text-button" :disabled="savingChapter" @click="emit('deleteNovel')">移入回收站</button>
    </div>
    <div class="editor-layout">
      <aside class="chapter-rail">
        <div class="rail-heading">
          <span>目录</span><span class="muted">{{ chapters.length }}</span>
        </div>
        <div v-for="(chapter, index) in chapters" :key="chapter.id" class="chapter-row">
          <button
            class="chapter-item"
            :class="{ selected: chapter.id === activeChapterId }"
            @click="emit('selectChapter', chapter.id)"
          >
            <span class="chapter-number">{{ chapter.number.toString().padStart(2, '0') }}</span>
            <span class="chapter-copy">
              <strong>{{ chapter.title }}</strong>
              <small
                >{{ chapter.body ? `${countManuscriptCharacters(chapter.body)} 字 · v${chapter.revision}` : '尚未开始'
                }}<span v-if="chapter.memoryStatus === 'stale'"> · 记忆待刷新</span>
                <span v-else-if="chapter.memoryStatus === 'missing' && chapter.body"> · 尚无记忆</span></small
              >
            </span>
            <span v-if="chapter.id === activeChapterId" class="chapter-active"></span>
          </button>
          <div v-if="canEdit" class="chapter-order-controls">
            <button
              :disabled="index === 0"
              :aria-label="`上移第 ${chapter.number} 章`"
              @click.stop="emit('moveChapter', chapter.id, -1)"
            >
              ↑
            </button>
            <button
              :disabled="index === chapters.length - 1"
              :aria-label="`下移第 ${chapter.number} 章`"
              @click.stop="emit('moveChapter', chapter.id, 1)"
            >
              ↓
            </button>
          </div>
        </div>
        <button v-if="canEdit" class="add-chapter-link" @click="emit('addChapter')">＋ 添加章节</button>
        <button
          v-if="canEdit && activeChapter"
          class="chapter-trash-link"
          :disabled="savingChapter"
          @click="emit('deleteChapter')"
        >
          将当前章节移入回收站
        </button>
        <details v-if="canEdit && deletedChapters.length" class="chapter-trash-list">
          <summary>已删除章节 · {{ deletedChapters.length }}</summary>
          <button
            v-for="chapter in deletedChapters"
            :key="chapter.id"
            class="deleted-chapter-row"
            :disabled="savingChapter"
            @click="emit('restoreChapter', chapter)"
          >
            <span>第 {{ chapter.number }} 章 · {{ chapter.title }}</span
            ><strong>恢复</strong>
          </button>
        </details>
      </aside>
      <div class="editor-center">
        <div v-if="activeChapter" class="manuscript">
          <div class="manuscript-toolbar">
            <span>
              <i class="save-dot" :class="{ conflict: saveConflict, saving: savingChapter }"></i>
              {{
                saveConflict
                  ? '版本冲突待处理'
                  : autoSaveState === 'saving'
                    ? '正在自动保存…'
                    : autoSaveState === 'error'
                      ? '自动保存失败'
                      : unsaved
                        ? '待自动保存'
                        : '已保存'
              }}
            </span>
            <div class="toolbar-actions">
              <button
                class="history-trigger"
                :class="{ active: showBlueprint }"
                @click="showBlueprint = !showBlueprint"
              >
                本章蓝图
              </button>
              <button class="history-trigger" @click="emit('toggleHistory')">
                版本记录 <span>{{ chapterRevisions.length }}</span>
              </button>
              <button
                v-if="canEdit && activeChapter && activeChapter.memoryStatus !== 'current'"
                class="history-trigger"
                :disabled="
                  refreshingMemory || savingChapter || unsaved || activeTaskRunning || !activeChapter.body.trim()
                "
                @click="emit('refreshMemory')"
              >
                {{ refreshingMemory ? '刷新中…' : '刷新记忆' }}
              </button>
              <span v-else-if="activeChapter?.memoryStatus === 'current'" class="memory-state">记忆已同步</span>
            </div>
          </div>
          <input v-model="draftTitle" class="chapter-title-input" :readonly="!canEdit" placeholder="章节标题" />
          <div class="chapter-subtitle">
            CHAPTER {{ activeChapter.number.toString().padStart(2, '0') }} <span>·</span> {{ novel.title }}
          </div>
          <textarea
            v-model="draftBody"
            class="manuscript-textarea"
            :readonly="!canEdit"
            placeholder="在这里写下故事的开端……&#10;&#10;你也可以告诉右侧的创作 Agent，想要续写、润色或调整的方向。"
          ></textarea>
          <div class="manuscript-footer">
            <span>{{ wordCount.toLocaleString() }} 字</span
            ><span>停止输入约 2.5 秒后自动保存；本地草稿也会同步缓存</span>
          </div>
        </div>
        <div v-else class="empty-state compact">
          <h3>选择一个章节开始写作</h3>
          <p>创建章节后，正文编辑器和 AI 创作工具会在这里出现。</p>
        </div>
        <div v-if="saveConflict && activeChapter" class="save-conflict-panel" role="alert">
          <div class="revision-panel-head">
            <div>
              <span class="eyebrow">REVISION CONFLICT</span
              ><strong>服务器版本已更新 · v{{ saveConflict.remote.revision }}</strong>
            </div>
          </div>
          <p>本地草稿基于 v{{ saveConflict.baseRevision }}。请比较两侧内容并手动合并后再决定是否覆盖。</p>
          <div class="conflict-columns">
            <article>
              <span>本地草稿</span><strong>{{ draftTitle }}</strong>
              <pre>{{ draftBody || '（空白正文）' }}</pre>
              <small>目标：{{ draftGoal || '未设置' }}</small>
              <small>关键事件：{{ draftKeyEvents || '未设置' }}</small>
              <small>章末钩子：{{ draftHook || '未设置' }}</small>
            </article>
            <article>
              <span>服务器最新版</span><strong>{{ saveConflict.remote.title }}</strong>
              <pre>{{ saveConflict.remote.body || '（空白正文）' }}</pre>
              <small>目标：{{ saveConflict.remote.goal || '未设置' }}</small>
              <small>关键事件：{{ saveConflict.remote.keyEvents || '未设置' }}</small>
              <small>章末钩子：{{ saveConflict.remote.suspenseHook || '未设置' }}</small>
            </article>
          </div>
          <div class="conflict-actions">
            <button class="button secondary" @click="emit('discardConflict')">使用服务器版本</button>
            <button class="button primary" :disabled="!canEdit || savingChapter" @click="emit('saveConflict')">
              保存本地/合并稿
            </button>
          </div>
        </div>
        <div v-if="showBlueprint && activeChapter" class="blueprint-panel">
          <div class="revision-panel-head">
            <div><span class="eyebrow">CHAPTER BLUEPRINT</span><strong>本章蓝图</strong></div>
            <button class="text-button" @click="showBlueprint = false">收起</button>
          </div>
          <p class="blueprint-hint">这些规划会注入生成提示词；只修改蓝图不会改变正文版本号。</p>
          <label class="blueprint-field"
            ><span>本章目标</span
            ><textarea
              v-model="draftGoal"
              :readonly="!canEdit"
              rows="2"
              placeholder="这一章要完成什么，例如：主角抵达雾港并察觉异常"
            ></textarea>
          </label>
          <label class="blueprint-field"
            ><span>关键事件</span
            ><textarea
              v-model="draftKeyEvents"
              :readonly="!canEdit"
              rows="2"
              placeholder="必须发生的情节节点，可每行一条"
            ></textarea>
          </label>
          <label class="blueprint-field"
            ><span>章末钩子</span
            ><textarea v-model="draftHook" :readonly="!canEdit" rows="2" placeholder="结尾留给读者的悬念"></textarea>
          </label>
        </div>
        <div v-if="showHistory && activeChapter" class="revision-panel">
          <div class="revision-panel-head">
            <div><span class="eyebrow">CHAPTER HISTORY</span><strong>章节历史版本</strong></div>
            <button class="text-button" @click="showHistory = false">收起</button>
          </div>
          <p v-if="loadingHistory" class="revision-empty">正在读取版本记录…</p>
          <p v-else-if="!chapterRevisions.length" class="revision-empty">
            首次保存或应用提案后，会在这里保留可恢复的旧版本。
          </p>
          <article v-for="revision in chapterRevisions" :key="revision.revision" class="revision-row">
            <div class="revision-row-top">
              <strong>版本 {{ revision.revision }}</strong
              ><span
                >{{ revisionSourceLabels[revision.source] || revision.source }} ·
                {{ new Date(revision.createdAt).toLocaleString('zh-CN') }}</span
              >
            </div>
            <p>{{ revision.body.slice(0, 220) || '（空白章节）' }}</p>
            <button class="text-button" :disabled="!canEdit" @click="emit('restoreRevision', revision)">
              恢复到此版本 ↗
            </button>
          </article>
        </div>
        <div class="assistant-panel">
          <div class="assistant-top">
            <div class="assistant-icon">AI</div>
            <div><strong>创作 Agent</strong><span>读取项目设定，生成可审核的章节提案</span></div>
            <span class="assistant-status"><i></i> 就绪</span>
          </div>
          <div class="assistant-body">
            <label class="field-label">本次创作目标</label>
            <textarea
              v-model="runInstruction"
              rows="3"
              placeholder="例如：续写主角抵达雾港后的场景，保持克制、冷峻的叙述风格……"
            ></textarea>
            <div class="assistant-options">
              <label>
                <span>模型服务商</span>
                <select v-model="providerId">
                  <option v-for="provider in providers" :key="provider.id" :value="provider.id">
                    {{ provider.name }}{{ provider.isDefault ? '（默认）' : '' }}
                  </option>
                </select>
              </label>
              <label
                ><span>处理章节</span>
                <select v-model="selectedChapterIds" multiple size="2">
                  <option v-for="chapter in chapters" :key="chapter.id" :value="chapter.id">
                    {{ chapter.number.toString().padStart(2, '0') }} · {{ chapter.title }}
                  </option>
                </select>
              </label>
              <button
                class="button primary run-button"
                :disabled="!canEdit || !chapters.length || activeTaskRunning"
                @click="emit('startBatch')"
              >
                开始生成 <span>↗</span>
              </button>
              <button
                class="button secondary"
                :disabled="!canEdit || !chapters.length || activeTaskRunning"
                @click="emit('startReview')"
              >
                审稿清单
              </button>
              <button
                class="button secondary"
                :disabled="!canEdit || !chapters.length || activeTaskRunning"
                @click="emit('startContinuityCheck')"
              >
                连续性检查
              </button>
            </div>
            <p class="assistant-hint">
              将使用小说设定与知识图谱作为上下文。{{
                novel.approvalMode === 'auto'
                  ? '生成内容会自动写入并保留旧版本。'
                  : '生成结果先进入提案审核，不会直接覆盖正文。'
              }}
            </p>
          </div>
        </div>
        <slot name="generation" />
      </div>
      <aside class="writing-aside">
        <div class="aside-card">
          <div class="aside-card-heading">
            <span class="eyebrow">WRITING GUIDE</span><span class="muted">注入生成</span>
          </div>
          <h3>写作指南</h3>
          <p class="aside-hint">统一文风与硬性约束，每次生成都会带上；建议使用「禁止…」「必须…」这类负向约束。</p>
          <textarea
            v-model="guideDraft"
            class="guide-textarea"
            :readonly="!canEdit"
            rows="5"
            placeholder="例如：禁止使用现代词汇；保持冷峻克制的叙述；每章结尾留一个悬念。"
          ></textarea>
          <button
            class="button secondary wide"
            :disabled="!canEdit || savingGuide || guideDraft === novel.writingGuide"
            @click="emit('saveGuide')"
          >
            {{ savingGuide ? '保存中…' : '保存写作指南' }}
          </button>
        </div>
        <div class="aside-card">
          <div class="aside-card-heading">
            <span class="eyebrow">PROJECT CONTEXT</span
            ><button class="text-button" @click="emit('openKnowledge')">查看图谱 ↗</button>
          </div>
          <h3>故事资料</h3>
          <p>{{ novel.summary || '为你的故事补充一句梗概，让 Agent 更了解创作方向。' }}</p>
          <div class="context-metrics">
            <div>
              <strong>{{ entityCount }}</strong
              ><span>设定实体</span>
            </div>
            <div>
              <strong>{{ edges.length }}</strong
              ><span>人物关系</span>
            </div>
          </div>
          <div class="context-tag-row">
            <span>{{ novel.genre || '待定题材' }}</span
            ><span>{{ novel.plannedChapters || '—' }} 章计划</span>
          </div>
        </div>
        <div class="aside-card">
          <div class="aside-card-heading">
            <span class="eyebrow">RECENT TASKS</span><span class="muted">{{ tasks.length }}</span>
          </div>
          <div v-if="tasks.length" class="task-list">
            <button
              v-for="item in tasks.slice(0, 6)"
              :key="item.id"
              class="task-list-item"
              @click="emit('resumeTask', item)"
            >
              <span class="task-dot" :class="item.status"></span>
              <div>
                <strong>{{
                  item.kind === 'workflow'
                    ? '工作流执行'
                    : item.kind === 'review'
                      ? '关键事件审稿'
                      : item.kind === 'continuity'
                        ? '连续性检查'
                        : '批量章节生成'
                }}</strong>
                <small>{{
                  item.status === 'completed'
                    ? '已完成'
                    : item.status === 'failed'
                      ? '失败'
                      : item.status === 'canceled'
                        ? '已取消'
                        : item.status === 'interrupted'
                          ? '已中断'
                          : item.status === 'running'
                            ? '执行中 · 查看事件'
                            : '排队中 · 查看事件'
                }}</small>
              </div>
              <span class="muted">{{ item.progress }}%</span>
            </button>
          </div>
          <p v-else class="aside-empty">还没有创作任务</p>
        </div>
        <div class="aside-note">
          <span>创作提示</span>
          <p>把明确的情绪、人物目标和场景限制写进任务说明，Agent 会更容易给出可用的第一稿。</p>
        </div>
      </aside>
    </div>
  </section>
</template>
