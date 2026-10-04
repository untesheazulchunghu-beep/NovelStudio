<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type Novel, type ReferenceDocument, type ReferenceHit } from '../api'

const props = defineProps<{ novel: Novel; canEdit: boolean }>()
const documents = ref<ReferenceDocument[]>([])
const results = ref<ReferenceHit[]>([])
const title = ref('')
const content = ref('')
const query = ref('')
const busy = ref(false)
const deletingId = ref('')
const notice = ref('')
const error = ref('')
let searchRequest = 0

async function load() {
  try {
    documents.value = (await api.references(props.novel.id)).documents
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取参考资料失败'
  }
}
async function create() {
  if (!title.value.trim() || !content.value.trim()) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await api.createReference(props.novel.id, { title: title.value, content: content.value })
    title.value = ''
    content.value = ''
    notice.value = '参考资料已加入作品库'
    await load()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '导入参考资料失败'
  } finally {
    busy.value = false
  }
}

async function remove(document: ReferenceDocument) {
  if (!props.canEdit || deletingId.value) return
  if (!window.confirm(`确定删除参考资料“${document.title}”吗？`)) return
  deletingId.value = document.id
  error.value = ''
  notice.value = ''
  try {
    await api.deleteReference(props.novel.id, document.id)
    documents.value = documents.value.filter((item) => item.id !== document.id)
    results.value = results.value.filter((item) => item.documentId !== document.id)
    notice.value = '参考资料已删除'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '删除参考资料失败'
  } finally {
    deletingId.value = ''
  }
}
async function search() {
  const requestId = ++searchRequest
  const value = query.value.trim()
  if (value.length < 2) {
    results.value = []
    return
  }
  try {
    const next = await api.searchReferences(props.novel.id, value)
    if (requestId === searchRequest) results.value = next.results
  } catch (cause) {
    if (requestId === searchRequest) error.value = cause instanceof Error ? cause.message : '搜索失败'
  }
}
onMounted(() => void load())
</script>

<template>
  <section class="page reference-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">REFERENCE LIBRARY</div>
        <h1>为故事保留真实的资料。</h1>
        <p>导入设定、采访、考据和灵感片段，生成章节时会自动检索相关内容。</p>
      </div>
    </div>
    <div class="reference-layout">
      <form v-if="canEdit" class="reference-import panel-card" @submit.prevent="create">
        <div class="eyebrow">NEW SOURCE</div>
        <h2>导入一份参考资料</h2>
        <input v-model="title" required maxlength="160" placeholder="资料标题，例如：雾港地理考据" />
        <textarea v-model="content" required rows="12" placeholder="粘贴文本内容，系统会自动分段并建立索引"></textarea>
        <button class="button primary" :disabled="busy">{{ busy ? '正在导入…' : '加入参考库' }}</button>
      </form>
      <div class="reference-browser panel-card">
        <div class="section-heading">
          <div>
            <div class="eyebrow">SOURCES / {{ documents.length.toString().padStart(2, '0') }}</div>
            <h2>作品参考资料</h2>
          </div>
        </div>
        <div class="library-search"><input v-model="query" placeholder="搜索参考资料内容" @input="search" /></div>
        <div v-if="query.length >= 2" class="reference-results">
          <article v-for="hit in results" :key="hit.id">
            <strong>{{ hit.title }} · 分段 {{ hit.chunkNumber }}</strong>
            <p>{{ hit.content }}</p>
          </article>
          <span v-if="!results.length" class="muted">没有找到相关片段</span>
        </div>
        <div v-else class="reference-list">
          <article v-for="document in documents" :key="document.id">
            <div>
              <strong>{{ document.title }}</strong
              ><span>{{ document.chunkCount }} 个文本分段</span>
            </div>
            <button
              v-if="canEdit"
              class="text-button"
              type="button"
              :disabled="deletingId === document.id"
              @click="remove(document)"
            >
              {{ deletingId === document.id ? '删除中…' : '删除' }}
            </button>
          </article>
          <p v-if="!documents.length" class="muted">还没有参考资料。</p>
        </div>
        <p v-if="notice" class="notice">{{ notice }}</p>
        <p v-if="error" class="error-message">{{ error }}</p>
      </div>
    </div>
  </section>
</template>
