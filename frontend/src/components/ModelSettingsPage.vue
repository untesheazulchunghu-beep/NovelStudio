<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, type ModelSettings, type ModelSettingsInput } from '../api'

const saved = ref<ModelSettings | null>(null)
const canManage = ref(false)
const form = ref<ModelSettingsInput>({
  baseUrl: '',
  model: '',
  temperature: 0.7,
  maxTokens: 0,
  timeoutSec: 480,
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

function syncForm(settings: ModelSettings) {
  saved.value = settings
  form.value = {
    baseUrl: settings.baseUrl,
    model: settings.model,
    temperature: settings.temperature,
    maxTokens: settings.maxTokens,
    timeoutSec: settings.timeoutSec,
    apiKey: '',
    clearApiKey: false,
  }
}

async function loadSettings() {
  loading.value = true
  error.value = ''
  try {
    const result = await api.modelSettings()
    canManage.value = result.canManage
    syncForm(result.settings)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取模型设置失败'
  } finally {
    loading.value = false
  }
}

function onApiKeyInput() {
  form.value.clearApiKey = false
  notice.value = ''
}

function requestSettings(): ModelSettingsInput {
  return {
    baseUrl: form.value.baseUrl.trim(),
    model: form.value.model.trim(),
    temperature: Number(form.value.temperature),
    maxTokens: Number(form.value.maxTokens),
    timeoutSec: Number(form.value.timeoutSec),
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
    syncForm(await api.updateModelSettings(requestSettings()))
    notice.value = '设置已保存，新任务将立即使用当前配置。'
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
    const result = await api.testModelSettings(requestSettings())
    notice.value = result.message || '模型服务连接成功。'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '连接模型服务失败'
  } finally {
    testing.value = false
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
        <p>配置一个 OpenAI 兼容接口，之后的生成任务会立即使用新设置。</p>
      </div>
      <span class="model-state" :class="saved?.apiKeyConfigured ? 'is-ready' : 'is-demo'">
        <i></i>{{ saved?.apiKeyConfigured ? '真实模型已配置' : '当前为演示模式' }}
      </span>
    </div>

    <div class="model-settings-layout">
      <form class="model-form-card" @submit.prevent="saveSettings">
        <div class="model-form-heading">
          <div>
            <span class="eyebrow">PROVIDER / 01</span>
            <h2>OpenAI 兼容服务</h2>
          </div>
          <span class="provider-mark">API</span>
        </div>

        <div v-if="loading" class="settings-loading">正在读取本机设置…</div>
        <template v-else>
          <div class="model-fields">
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
              <small v-if="saved?.apiKeyConfigured">密钥只写入本机配置文件，不会从服务器回传。</small>
              <small v-else>尚未配置密钥时，作品生成使用安全的演示文本。</small>
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
          </div>

          <div class="model-form-footer">
            <p v-if="!canManage" class="model-feedback">只有本机首个注册账户可以修改此共享设置。</p>
            <p v-else-if="error" class="model-feedback is-error" role="alert">{{ error }}</p>
            <p v-else-if="notice" class="model-feedback is-success" role="status">{{ notice }}</p>
            <p v-else class="model-feedback">设置保存后立即生效，无需重启服务。</p>
            <div class="model-actions">
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
                {{ saving ? '正在保存…' : '保存设置' }}
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
          <p>模型设置保存在当前运行目录的 `.env` 文件中，不进入作品数据库。API Key 不会被接口回显。</p>
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
