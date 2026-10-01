<script setup lang="ts">
const authMode = defineModel<'login' | 'register'>('authMode', { required: true })
const email = defineModel<string>('email', { required: true })
const password = defineModel<string>('password', { required: true })

defineProps<{
  busy: boolean
  errorMessage: string
}>()

const emit = defineEmits<{
  submit: []
}>()
</script>

<template>
  <div class="auth-shell">
    <div class="auth-card">
      <div class="brand-mark large">卷</div>
      <div class="eyebrow">STORY STUDIO / AI WRITING</div>
      <h1>让长篇创作<br /><span>有迹可循</span></h1>
      <p class="auth-intro">从人物关系、世界设定到章节正文，把灵感组织成一部完整的小说。</p>
      <form class="auth-form" @submit.prevent="emit('submit')">
        <label
          >邮箱地址<input v-model="email" type="email" autocomplete="email" placeholder="writer@example.com" required
        /></label>
        <label
          >密码<input
            v-model="password"
            type="password"
            :autocomplete="authMode === 'register' ? 'new-password' : 'current-password'"
            placeholder="至少 10 位"
            minlength="10"
            required
        /></label>
        <p v-if="errorMessage" class="form-error">{{ errorMessage }}</p>
        <button class="button primary wide" :disabled="busy">
          {{ busy ? '请稍候…' : authMode === 'register' ? '创建创作空间' : '登录工作台' }}
        </button>
      </form>
      <button class="text-button auth-switch" @click="authMode = authMode === 'register' ? 'login' : 'register'">
        {{ authMode === 'register' ? '已有账户？登录' : '还没有账户？创建账户' }}
      </button>
      <div class="auth-foot"><span class="status-dot"></span> 私有工作区 · 内容由你掌控</div>
    </div>
    <div class="auth-art">
      <div class="art-orbit orbit-one"></div>
      <div class="art-orbit orbit-two"></div>
      <div class="art-cover">
        <span>STORY<br />ARCHIVE</span><strong>未完成的<br />世界</strong><small>VOL. 01 — YOUR STORY</small>
      </div>
      <div class="art-note note-top">人物弧光 <span>已梳理</span></div>
      <div class="art-note note-bottom">第 04 章 <span>继续书写</span></div>
      <div class="art-caption">一个好故事，值得被认真搭建。</div>
    </div>
  </div>
</template>
