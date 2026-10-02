<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, type ModelProvider, type ModelProviderInput } from '../api'

const emit = defineEmits<{ providersUpdated: [] }>()
const providers = ref<ModelProvider[]>([])
const defaultProviderId = ref('')
const selectedProviderId = ref('')
const saved = ref<ModelProvider | null>(null)
const canManage = ref(false)
const form = ref<ModelProviderInput>({
  name: '',
  baseUrl: '',
  model: '',
  temperature: 0.7,
  maxTokens: 0,
  timeoutSec: 480,
  inputCostPerMillion: 0,
  outputCostPerMillion: 0,
  apiKey: '',
  clearApiKey: false,
})
const loading = ref(true)
const saving = ref(false)
const testing = ref(false)
const error = ref('')
const notice = ref('')

const canTest = computed(
  () =>
    canManage.value &&
    !loading.value &&
    !saving.value &&
    !testing.value &&
    (!!form.value.apiKey.trim() || (!!saved.value?.apiKeyConfigured && !form.value.clearApiKey)),
)

function syncForm(provider: ModelProvider) {
  selectedProviderId.value = provider.id
  saved.value = provider
  form.value = {
    name: provider.name,
    baseUrl: provider.baseUrl,
    model: provider.model,
    temperature: provider.temperature,
    maxTokens: provider.maxTokens,
    timeoutSec: provider.timeoutSec,
    inputCostPerMillion: provider.inputCostPerMillion,
    outputCostPerMillion: provider.outputCostPerMillion,
    apiKey: '',
    clearApiKey: false,
  }
}

async function refreshProviders() {
  const result = await api.modelProviders()
  providers.value = result.providers
  defaultProviderId.value = result.defaultProviderId
  canManage.value = result.canManage
  emit('providersUpdated')
}

async function loadSettings() {
  loading.value = true
  error.value = ''
  try {
    await refreshProviders()
    const selected = providers.value.find((provider) => provider.id === selectedProviderId.value)
    const fallback = providers.value.find((provider) => provider.id === defaultProviderId.value) ?? providers.value[0]
    if (selected) syncForm(selected)
    else if (fallback) syncForm(fallback)
    else startNewProvider()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取模型设置失败'
  } finally {
    loading.value = false
  }
}

function startNewProvider() {
  selectedProviderId.value = ''
  saved.value = null
  const base = providers.value.find((provider) => provider.id === defaultProviderId.value)
  form.value = {
    name: '',
    baseUrl: base?.baseUrl ?? 'https://api.openai.com/v1',
    model: base?.model ?? 'gpt-4o-mini',
    temperature: base?.temperature ?? 0.7,
    maxTokens: base?.maxTokens ?? 0,
    timeoutSec: base?.timeoutSec ?? 480,
    inputCostPerMillion: base?.inputCostPerMillion ?? 0,
    outputCostPerMillion: base?.outputCostPerMillion ?? 0,
    apiKey: '',
    clearApiKey: false,
  }
  error.value = ''
  notice.value = ''
}

function selectProvider(event: Event) {
  const id = (event.target as HTMLSelectElement).value
  const provider = providers.value.find((item) => item.id === id)
  if (provider) syncForm(provider)
}

function onApiKeyInput() {
  form.value.clearApiKey = false
  notice.value = ''
}

function requestSettings(): ModelProviderInput {
  return {
    name: form.value.name.trim(),
    baseUrl: form.value.baseUrl.trim(),
    model: form.value.model.trim(),
    temperature: Number(form.value.temperature),
    maxTokens: Number(form.value.maxTokens),
    timeoutSec: Number(form.value.timeoutSec),
    inputCostPerMillion: Number(form.value.inputCostPerMillion),
    outputCostPerMillion: Number(form.value.outputCostPerMillion),
    apiKey: form.value.apiKey,
    clearApiKey: form.value.clearApiKey,
  }
}

async function saveSettings() {
  if (!canManage.value) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const input = requestSettings()
    const result = selectedProviderId.value
      ? await api.updateModelProvider(selectedProviderId.value, input)
      : await api.createModelProvider(input)
    await refreshProviders()
    const savedProvider = providers.value.find((provider) => provider.id === result.provider.id)
    if (savedProvider) syncForm(savedProvider)
    notice.value = '服务商设置已保存，新任务可以选择此配置。'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '保存模型设置失败'
  } finally {
    saving.value = false
  }
}

async function testConnection() {
  if (!canManage.value) return
  testing.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await api.testModelProvider(selectedProviderId.value, requestSettings())
    notice.value = result.message || '模型服务连接成功。'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '连接模型服务失败'
  } finally {
    testing.value = false
  }
}

async function setDefault() {
  if (!canManage.value || !selectedProviderId.value) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    await api.setDefaultModelProvider(selectedProviderId.value)
    await refreshProviders()
    const provider = providers.value.find((item) => item.id === selectedProviderId.value)
    if (provider) syncForm(provider)
    notice.value = '默认服务商已更新。'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '更新默认服务商失败'
  } finally {
    saving.value = false
  }
}

async function deleteProvider() {
  if (!canManage.value || !selectedProviderId.value) return
  if (!window.confirm(`确定删除模型服务商“${saved.value?.name ?? ''}”吗？`)) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    await api.deleteModelProvider(selectedProviderId.value)
    selectedProviderId.value = ''
    await refreshProviders()
    const provider = providers.value.find((item) => item.id === defaultProviderId.value) ?? providers.value[0]
    if (provider) syncForm(provider)
    else startNewProvider()
    notice.value = '服务商已删除。'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '删除模型服务商失败'
  } finally {
    saving.value = false
  }
}

onMounted(loadSettings)
</script>

<template>
  <section class="page model-settings-page">
    <div class="page-heading model-settings-heading">
      <div>
        <div class="eyebrow">LOCAL MODEL CONNECTION</div>
        <h1>让灵感，接上你的模型。</h1>
        <p>保存多个 OpenAI 兼容服务商，创建任务时可以选择使用哪一组配置。</p>
      </div>
      <span class="model-state" :class="saved?.apiKeyConfigured ? 'is-ready' : 'is-demo'">
        <i></i>{{ saved?.apiKeyConfigured ? '真实模型已配置' : '当前为演示模式' }}
      </span>
    </div>

    <div class="model-settings-layout">
      <form class="model-form-card" @submit.prevent="saveSettings">
        <div class="model-form-heading">
          <div>
            <span class="eyebrow">MODEL PROVIDERS / {{ providers.length.toString().padStart(2, '0') }}</span>
            <h2>{{ saved?.name || '新增模型服务商' }}</h2>
          </div>
          <button v-if="canManage" class="text-button" type="button" @click="startNewProvider">＋ 新增服务商</button>
        </div>

        <div v-if="loading" class="settings-loading">正在读取本机设置…</div>
        <template v-else>
          <label class="model-field field-wide">
            <span>已保存的服务商</span>
            <select :value="selectedProviderId" @change="selectProvider">
              <option v-for="provider in providers" :key="provider.id" :value="provider.id">
                {{ provider.name }}{{ provider.id === defaultProviderId ? '（默认）' : '' }}
              </option>
              <option v-if="!selectedProviderId" value="" selected>新增服务商</option>
            </select>
          </label>
          <div class="model-fields">
            <label class="model-field field-wide">
              <span>服务商名称</span>
              <input v-model="form.name" required maxlength="80" :disabled="!canManage" placeholder="例如：写作主模型" />
            </label>
            <label class="model-field field-wide">
              <span>API 基础地址</span>
              <input
                v-model="form.baseUrl"
                type="url"
                required
                :disabled="!canManage"
                placeholder="https://api.openai.com/v1"
                autocomplete="url"
              />
              <small>填写服务商提供的 API 根地址，不需要附加 `/chat/completions`。</small>
            </label>
            <label class="model-field field-wide">
              <span>模型名称</span>
              <input
                v-model="form.model"
                required
                maxlength="200"
                :disabled="!canManage"
                placeholder="例如：gpt-4o-mini"
                autocomplete="off"
              />
            </label>
            <label class="model-field field-wide">
              <span>API Key</span>
              <input
                v-model="form.apiKey"
                type="password"
                maxlength="4096"
                :disabled="!canManage"
                :placeholder="saved?.apiKeyConfigured ? '已保存密钥；留空可保持不变' : '粘贴服务商 API Key'"
                autocomplete="new-password"
                spellcheck="false"
                @input="onApiKeyInput"
              />
              <small v-if="saved?.apiKeyConfigured">密钥会加密后保存在本机数据库中，接口不会回传密钥。</small>
              <small v-else>尚未配置密钥时，使用此服务商的任务会进入演示模式。</small>
            </label>
            <label v-if="saved?.apiKeyConfigured" class="model-clear-key">
              <input v-model="form.clearApiKey" type="checkbox" :disabled="!canManage" />
              <span>保存时清除当前已保存的 API Key</span>
            </label>
            <label class="model-field">
              <span>温度</span>
              <input
                v-model.number="form.temperature"
                type="number"
                min="0"
                max="2"
                step="0.1"
                required
                :disabled="!canManage"
              />
              <small>范围 0–2；数值越高，表达越发散。</small>
            </label>
            <label class="model-field">
              <span>最大输出 Token</span>
              <input
                v-model.number="form.maxTokens"
                type="number"
                min="0"
                max="1000000"
                step="1"
                required
                :disabled="!canManage"
              />
              <small>设为 0 时使用服务商默认值。</small>
            </label>
            <label class="model-field">
              <span>请求超时（秒）</span>
              <input
                v-model.number="form.timeoutSec"
                type="number"
                min="1"
                max="1800"
                step="1"
                required
                :disabled="!canManage"
              />
              <small>长章节可适当调高，范围 1–1800 秒。</small>
            </label>
            <label class="model-field">
              <span>输入价格（USD / 百万 Token）</span>
              <input
                v-model.number="form.inputCostPerMillion"
                type="number"
                min="0"
                max="1000000"
                step="0.0001"
                required
                :disabled="!canManage"
              />
            </label>
            <label class="model-field">
              <span>输出价格（USD / 百万 Token）</span>
              <input
                v-model.number="form.outputCostPerMillion"
                type="number"
                min="0"
                max="1000000"
                step="0.0001"
                required
                :disabled="!canManage"
              />
            </label>
            <p class="model-price-note field-wide">
              按服务商公布的美元 / 百万 Token 单价填写。两项都为 0 时费用显示为未配置；没有返回 usage 的兼容服务商会按文本长度估算 Token。
            </p>
          </div>

          <div class="model-form-footer">
            <p v-if="!canManage" class="model-feedback">只有本机首个注册账户可以修改此共享设置。</p>
            <p v-else-if="error" class="model-feedback is-error" role="alert">{{ error }}</p>
            <p v-else-if="notice" class="model-feedback is-success" role="status">{{ notice }}</p>
            <p v-else class="model-feedback">配置保存在本机数据库中，新任务启动时会固定使用所选服务商。</p>
            <div class="model-actions">
              <button
                v-if="canManage && saved && !saved.isDefault"
                class="button secondary"
                type="button"
                :disabled="saving || testing"
                @click="setDefault"
              >设为默认</button>
              <button
                v-if="canManage && saved && providers.length > 1"
                class="button secondary"
                type="button"
                :disabled="saving || testing"
                @click="deleteProvider"
              >删除</button>
              <button
                v-if="canManage"
                class="button secondary"
                type="button"
                :disabled="!canTest"
                @click="testConnection"
              >
                {{ testing ? '正在测试…' : '测试连接' }}
              </button>
              <button v-if="canManage" class="button primary" type="submit" :disabled="saving || testing">
                {{ saving ? '正在保存…' : saved ? '保存设置' : '新增服务商' }}
              </button>
            </div>
          </div>
        </template>
      </form>

      <aside class="model-settings-aside">
        <div class="local-config-card">
          <span class="eyebrow">LOCAL ONLY</span>
          <div class="local-config-glyph">⌘</div>
          <h2>留在你的设备上</h2>
          <p>服务商配置保存在本机数据库中，API Key 使用本机加密密钥保护，接口不会回显密钥。</p>
          <div class="local-config-divider"></div>
          <div class="local-config-detail"><span>配置范围</span><strong>本机所有账户与工作区</strong></div>
          <div class="local-config-detail"><span>管理权限</span><strong>首个注册账户</strong></div>
          <div class="local-config-detail"><span>网络请求</span><strong>仅在测试或生成时发送</strong></div>
        </div>
        <div class="model-help-card">
          <strong>还没有 API Key？</strong>
          <p>可以先保存 API 地址和模型名称，之后补充密钥。未配置密钥时，生成任务仍可用演示模式体验流程。</p>
        </div>
      </aside>
    </div>
  </section>
</template>
