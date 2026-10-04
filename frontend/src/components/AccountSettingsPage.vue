<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api, type User, type Invitation } from '../api'

const props = defineProps<{ user: User; currentWorkspaceId?: string }>()
const emit = defineEmits<{ logout: [] }>()

const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const busy = ref(false)
const notice = ref('')
const error = ref('')

const invitations = ref<Invitation[]>([])
const showInviteDialog = ref(false)
const inviteRole = ref('editor')
const inviteExpiresIn = ref(7)
const inviteBusy = ref(false)
const inviteNotice = ref('')
const inviteError = ref('')
const generatedInviteUrl = ref('')

async function changePassword() {
  error.value = ''
  notice.value = ''
  if (newPassword.value.length < 10 || newPassword.value.length > 72) {
    error.value = '新密码长度必须为 10–72 位'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    error.value = '两次输入的新密码不一致'
    return
  }
  busy.value = true
  try {
    await api.changePassword(oldPassword.value, newPassword.value)
    oldPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    notice.value = '密码已修改，所有设备的旧会话已失效'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '修改密码失败'
  } finally {
    busy.value = false
  }
}

async function loadInvitations() {
  if (!props.currentWorkspaceId) return
  try {
    invitations.value = await api.listInvitations(props.currentWorkspaceId)
  } catch (cause) {
    console.error('加载邀请列表失败:', cause)
  }
}

async function createInvite() {
  if (!props.currentWorkspaceId) return
  inviteError.value = ''
  inviteNotice.value = ''
  generatedInviteUrl.value = ''
  inviteBusy.value = true
  try {
    const inv = await api.createInvitation(props.currentWorkspaceId, inviteRole.value, inviteExpiresIn.value)
    const url = `${window.location.origin}/invite/${inv.id}`
    generatedInviteUrl.value = url
    inviteNotice.value = '邀请链接已生成'
    await loadInvitations()
  } catch (cause) {
    inviteError.value = cause instanceof Error ? cause.message : '创建邀请失败'
  } finally {
    inviteBusy.value = false
  }
}

async function deleteInvite(invitationId: string) {
  if (!props.currentWorkspaceId) return
  if (!confirm('确认删除此邀请？')) return
  try {
    await api.deleteInvitation(props.currentWorkspaceId, invitationId)
    await loadInvitations()
  } catch (cause) {
    alert('删除失败: ' + (cause instanceof Error ? cause.message : '未知错误'))
  }
}

function copyInviteUrl() {
  if (!generatedInviteUrl.value) return
  navigator.clipboard.writeText(generatedInviteUrl.value)
  inviteNotice.value = '已复制到剪贴板'
}

onMounted(() => {
  loadInvitations()
})
</script>

<template>
  <section class="page account-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">ACCOUNT SETTINGS</div>
        <h1>账户管理</h1>
        <p>修改密码、查看邮箱信息</p>
      </div>
    </div>
    <div class="account-layout">
      <div class="panel-card">
        <div class="eyebrow">PROFILE</div>
        <h2>账户信息</h2>
        <div class="account-info">
          <div class="info-row">
            <label>邮箱</label>
            <span>{{ user.email }}</span>
          </div>
          <div class="info-row">
            <label>账户 ID</label>
            <span class="mono">{{ user.id }}</span>
          </div>
        </div>
      </div>
      <form class="panel-card" @submit.prevent="changePassword">
        <div class="eyebrow">SECURITY</div>
        <h2>修改密码</h2>
        <div class="form-field">
          <label for="old-password">当前密码</label>
          <input id="old-password" v-model="oldPassword" type="password" required autocomplete="current-password" />
        </div>
        <div class="form-field">
          <label for="new-password">新密码 (10–72 位)</label>
          <input
            id="new-password"
            v-model="newPassword"
            type="password"
            required
            minlength="10"
            maxlength="72"
            autocomplete="new-password"
          />
        </div>
        <div class="form-field">
          <label for="confirm-password">确认新密码</label>
          <input
            id="confirm-password"
            v-model="confirmPassword"
            type="password"
            required
            minlength="10"
            maxlength="72"
            autocomplete="new-password"
          />
        </div>
        <button class="button primary" :disabled="busy">{{ busy ? '修改中…' : '修改密码' }}</button>
        <p v-if="notice" class="notice">{{ notice }}</p>
        <p v-if="error" class="error-message">{{ error }}</p>
        <p class="hint">修改密码后，所有设备的旧会话将立即失效，需要重新登录。</p>
      </form>
      <div class="panel-card">
        <div class="eyebrow">SESSIONS</div>
        <h2>会话管理</h2>
        <p class="hint">当前登录会话在浏览器关闭后 7 天内有效。</p>
        <button class="button secondary" @click="emit('logout')">在所有设备上登出</button>
      </div>
      <div v-if="currentWorkspaceId" class="panel-card">
        <div class="eyebrow">INVITATIONS</div>
        <h2>成员邀请</h2>
        <p class="hint">生成邀请链接，允许他人加入当前工作区。</p>
        <button class="button primary" @click="showInviteDialog = true">创建邀请链接</button>
        <div v-if="invitations.length > 0" class="invitations-list">
          <div v-for="inv in invitations" :key="inv.id" class="invitation-item">
            <div class="invitation-info">
              <span class="role-badge">{{ inv.role }}</span>
              <span class="expires">过期: {{ new Date(inv.expires_at).toLocaleString() }}</span>
              <span v-if="inv.used_at" class="used">已使用</span>
            </div>
            <button v-if="!inv.used_at" class="button-icon" title="删除" @click="deleteInvite(inv.id)">✕</button>
          </div>
        </div>
      </div>
    </div>
    <div v-if="showInviteDialog" class="dialog-overlay" @click.self="showInviteDialog = false">
      <div class="dialog">
        <h2>创建邀请链接</h2>
        <div class="form-field">
          <label for="invite-role">角色</label>
          <select id="invite-role" v-model="inviteRole">
            <option value="owner">所有者</option>
            <option value="editor">编辑</option>
            <option value="reviewer">审阅者</option>
            <option value="reader">只读</option>
          </select>
        </div>
        <div class="form-field">
          <label for="invite-expires">有效期（天）</label>
          <input id="invite-expires" v-model.number="inviteExpiresIn" type="number" min="1" max="30" />
        </div>
        <div v-if="generatedInviteUrl" class="invite-url-box">
          <input :value="generatedInviteUrl" readonly @click="copyInviteUrl" />
          <button class="button secondary" @click="copyInviteUrl">复制</button>
        </div>
        <p v-if="inviteNotice" class="notice">{{ inviteNotice }}</p>
        <p v-if="inviteError" class="error-message">{{ inviteError }}</p>
        <div class="dialog-actions">
          <button class="button secondary" @click="showInviteDialog = false">关闭</button>
          <button class="button primary" :disabled="inviteBusy" @click="createInvite">
            {{ inviteBusy ? '生成中…' : '生成链接' }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
