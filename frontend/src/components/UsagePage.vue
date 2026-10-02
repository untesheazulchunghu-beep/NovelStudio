<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type ModelUsageReport, type UsageTotals } from '../api'

const report = ref<ModelUsageReport | null>(null)
const loading = ref(true)
const error = ref('')

async function loadUsage() {
  loading.value = true
  error.value = ''
  try {
    report.value = await api.modelUsage()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取模型用量失败'
  } finally {
    loading.value = false
  }
}

function formatTokens(value: number) {
  return value.toLocaleString()
}

function formatCost(item: UsageTotals) {
  return item.costKnown ? `$${item.estimatedCostUsd.toFixed(8)}` : '未配置费率'
}

function formatCalls(item: UsageTotals) {
  const parts: string[] = []
  if (item.reportedTokenCalls) parts.push(`${formatTokens(item.reportedTokenCalls)} 次实际用量`)
  if (item.estimatedTokenCalls) parts.push(`${formatTokens(item.estimatedTokenCalls)} 次估算用量`)
  return parts.length ? parts.join(' · ') : '0 次调用'
}

function formatDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

onMounted(loadUsage)
</script>

<template>
  <section class="page model-usage-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">LOCAL MODEL USAGE</div>
        <h1>模型用量</h1>
        <p>汇总本机所有作品和账户的模型调用。实际用量来自服务商返回值；兼容服务商缺少用量时按文本长度估算。两项费率都为 0 时费用未知，旧任务不会回填。</p>
      </div>
      <button class="button secondary" type="button" :disabled="loading" @click="loadUsage">
        {{ loading ? '正在读取…' : '刷新' }}
      </button>
    </div>

    <p v-if="error" class="usage-error" role="alert">{{ error }}</p>
    <div v-if="loading && !report" class="settings-loading">正在读取模型用量…</div>
    <template v-else-if="report">
      <div class="model-usage-summary">
        <article class="model-usage-card">
          <span>累计 Token</span>
          <strong>{{ formatTokens(report.summary.totalTokens) }}</strong>
          <small>输入 {{ formatTokens(report.summary.promptTokens) }} · 输出 {{ formatTokens(report.summary.completionTokens) }}</small>
        </article>
        <article class="model-usage-card">
          <span>估算费用</span>
          <strong>{{ formatCost(report.summary) }}</strong>
          <small>{{ formatCalls(report.summary) }}</small>
        </article>
        <article class="model-usage-card">
          <span>用量来源</span>
          <strong>{{ formatTokens(report.summary.reportedTokenCalls + report.summary.estimatedTokenCalls) }} 次调用</strong>
          <small>费用按 Provider 中手动维护的 USD / 百万 Token 单价计算。</small>
        </article>
      </div>

      <div class="model-usage-columns">
        <section class="model-usage-section">
          <div class="model-usage-section-head"><h2>按作品</h2><span>{{ report.novels.length }} 部作品</span></div>
          <div v-if="!report.novels.length" class="model-usage-empty">暂无已记录的用量</div>
          <div v-for="item in report.novels" :key="item.novelId" class="model-usage-row">
            <div><strong>{{ item.novelTitle }}</strong><small>{{ formatCalls(item) }}</small></div>
            <div class="model-usage-row-total"><b>{{ formatTokens(item.totalTokens) }}</b><small>{{ formatCost(item) }}</small></div>
          </div>
        </section>
        <section class="model-usage-section">
          <div class="model-usage-section-head"><h2>按用户</h2><span>{{ report.users.length }} 个账户</span></div>
          <div v-if="!report.users.length" class="model-usage-empty">暂无已记录的用量</div>
          <div v-for="item in report.users" :key="item.userId" class="model-usage-row">
            <div><strong>{{ item.userEmail }}</strong><small>{{ formatCalls(item) }}</small></div>
            <div class="model-usage-row-total"><b>{{ formatTokens(item.totalTokens) }}</b><small>{{ formatCost(item) }}</small></div>
          </div>
        </section>
      </div>

      <section class="model-usage-section model-usage-task-section">
        <div class="model-usage-section-head"><h2>最近任务</h2><span>最多显示 200 条</span></div>
        <div v-if="!report.tasks.length" class="model-usage-empty">暂无已记录的用量</div>
        <div v-else class="model-usage-table-scroll">
          <table class="model-usage-table">
            <thead>
              <tr><th>时间</th><th>作品 / 任务</th><th>用户</th><th>输入</th><th>输出</th><th>合计</th><th>费用</th><th>用量来源</th></tr>
            </thead>
            <tbody>
              <tr v-for="item in report.tasks" :key="item.taskId">
                <td>{{ formatDate(item.createdAt) }}</td>
                <td><strong>{{ item.novelTitle }}</strong><small>{{ item.kind }} · {{ item.status }}</small></td>
                <td>{{ item.userEmail }}</td>
                <td>{{ formatTokens(item.promptTokens) }}</td>
                <td>{{ formatTokens(item.completionTokens) }}</td>
                <td><b>{{ formatTokens(item.totalTokens) }}</b></td>
                <td>{{ formatCost(item) }}</td>
                <td>{{ formatCalls(item) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </section>
</template>
