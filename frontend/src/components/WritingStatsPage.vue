<template>
  <div class="stats-page">
    <div class="stats-header">
      <h2>写作统计</h2>
      <div class="controls">
        <select v-model="selectedDays" @change="loadStats">
          <option :value="7">最近 7 天</option>
          <option :value="30">最近 30 天</option>
          <option :value="90">最近 90 天</option>
        </select>
        <button class="btn-secondary" @click="showGoalDialog = true">设置目标</button>
      </div>
    </div>

    <div v-if="loading" class="loading">加载中...</div>
    <div v-else-if="error" class="error">{{ error }}</div>
    <div v-else class="stats-content">
      <!-- Goal Card -->
      <div v-if="goal.dailyWordGoal > 0" class="goal-card">
        <h3>每日目标</h3>
        <div class="goal-display">
          <span class="goal-number">{{ goal.dailyWordGoal }}</span>
          <span class="goal-label">字/天</span>
        </div>
        <div v-if="todayStats" class="today-progress">
          <div class="progress-bar">
            <div class="progress-fill" :style="{ width: progressPercent + '%' }"></div>
          </div>
          <span class="progress-text">今日已写 {{ todayStats.netWords }} 字 ({{ progressPercent }}%)</span>
        </div>
      </div>

      <!-- Summary Cards -->
      <div class="summary-cards">
        <div class="summary-card">
          <div class="summary-label">总净增</div>
          <div class="summary-value">{{ totalNetWords }}</div>
        </div>
        <div class="summary-card">
          <div class="summary-label">总新增</div>
          <div class="summary-value">{{ totalWordsAdded }}</div>
        </div>
        <div class="summary-card">
          <div class="summary-label">总删除</div>
          <div class="summary-value">{{ totalWordsDeleted }}</div>
        </div>
        <div class="summary-card">
          <div class="summary-label">平均每天</div>
          <div class="summary-value">{{ avgPerDay }}</div>
        </div>
      </div>

      <!-- Chart -->
      <div class="chart-container">
        <h3>字数趋势</h3>
        <div class="chart">
          <div v-for="stat in reversedStats" :key="stat.date" class="chart-bar-group">
            <div class="chart-bar-container">
              <div
                class="chart-bar positive"
                :style="{ height: getBarHeight(stat.wordsAdded) + 'px' }"
                :title="`新增: ${stat.wordsAdded}`"
              ></div>
              <div
                class="chart-bar negative"
                :style="{ height: getBarHeight(stat.wordsDeleted) + 'px' }"
                :title="`删除: ${stat.wordsDeleted}`"
              ></div>
            </div>
            <div class="chart-label">{{ formatDate(stat.date) }}</div>
            <div class="chart-value">{{ stat.netWords > 0 ? '+' : '' }}{{ stat.netWords }}</div>
          </div>
        </div>
      </div>

      <!-- Table -->
      <div class="stats-table">
        <h3>详细数据</h3>
        <table>
          <thead>
            <tr>
              <th>日期</th>
              <th>新增</th>
              <th>删除</th>
              <th>净增</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="stat in stats" :key="stat.date">
              <td>{{ stat.date }}</td>
              <td class="positive">+{{ stat.wordsAdded }}</td>
              <td class="negative">-{{ stat.wordsDeleted }}</td>
              <td :class="stat.netWords >= 0 ? 'positive' : 'negative'">
                {{ stat.netWords > 0 ? '+' : '' }}{{ stat.netWords }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Goal Dialog -->
    <div v-if="showGoalDialog" class="modal-overlay" @click.self="showGoalDialog = false">
      <div class="modal-content">
        <h3>设置写作目标</h3>
        <div class="form-group">
          <label>每日字数目标</label>
          <input v-model.number="newGoal" type="number" min="0" step="100" placeholder="例如: 3000" />
          <p class="hint">设置为 0 表示不设目标</p>
        </div>
        <div class="modal-actions">
          <button class="btn-secondary" @click="showGoalDialog = false">取消</button>
          <button class="btn-primary" @click="saveGoal">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { fetchWritingStats, fetchWritingGoal, updateWritingGoal } from '../api'

const props = defineProps<{ novelId: string }>()

const stats = ref<Array<{ date: string; wordsAdded: number; wordsDeleted: number; netWords: number }>>([])
const goal = ref({ dailyWordGoal: 0 })
const loading = ref(true)
const error = ref('')
const selectedDays = ref(30)
const showGoalDialog = ref(false)
const newGoal = ref(0)

const reversedStats = computed(() => [...stats.value].reverse())

const todayStats = computed(() => {
  const today = new Date().toISOString().split('T')[0]
  return stats.value.find((s) => s.date === today)
})

const progressPercent = computed(() => {
  if (!todayStats.value || goal.value.dailyWordGoal === 0) return 0
  return Math.min(100, Math.round((todayStats.value.netWords / goal.value.dailyWordGoal) * 100))
})

const totalNetWords = computed(() => stats.value.reduce((sum, s) => sum + s.netWords, 0))
const totalWordsAdded = computed(() => stats.value.reduce((sum, s) => sum + s.wordsAdded, 0))
const totalWordsDeleted = computed(() => stats.value.reduce((sum, s) => sum + s.wordsDeleted, 0))
const avgPerDay = computed(() => {
  if (stats.value.length === 0) return 0
  return Math.round(totalNetWords.value / stats.value.length)
})

const maxWords = computed(() => {
  return Math.max(...stats.value.map((s) => Math.max(s.wordsAdded, s.wordsDeleted)), 1)
})

function getBarHeight(words: number): number {
  return Math.round((words / maxWords.value) * 100)
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return `${date.getMonth() + 1}/${date.getDate()}`
}

async function loadStats() {
  loading.value = true
  error.value = ''
  try {
    stats.value = await fetchWritingStats(props.novelId, selectedDays.value)
  } catch (err) {
    error.value = '加载统计数据失败'
    console.error(err)
  } finally {
    loading.value = false
  }
}

async function loadGoal() {
  try {
    goal.value = await fetchWritingGoal(props.novelId)
  } catch (err) {
    console.error('加载写作目标失败', err)
  }
}

async function saveGoal() {
  try {
    await updateWritingGoal(props.novelId, newGoal.value)
    goal.value.dailyWordGoal = newGoal.value
    showGoalDialog.value = false
  } catch (err) {
    alert('保存写作目标失败')
    console.error(err)
  }
}

onMounted(async () => {
  await Promise.all([loadStats(), loadGoal()])
  newGoal.value = goal.value.dailyWordGoal
})
</script>

<style scoped>
.stats-page {
  padding: 1.5rem;
  max-width: 1200px;
  margin: 0 auto;
}

.stats-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2rem;
}

.stats-header h2 {
  margin: 0;
  font-size: 1.5rem;
}

.controls {
  display: flex;
  gap: 1rem;
  align-items: center;
}

.controls select {
  padding: 0.5rem;
  border: 1px solid #334155;
  background: #1e293b;
  color: #f8fafc;
  border-radius: 4px;
}

.loading,
.error {
  text-align: center;
  padding: 2rem;
  color: #94a3b8;
}

.error {
  color: #f87171;
}

.goal-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 1.5rem;
  margin-bottom: 2rem;
}

.goal-card h3 {
  margin: 0 0 1rem 0;
  font-size: 1rem;
  color: #94a3b8;
}

.goal-display {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.goal-number {
  font-size: 2.5rem;
  font-weight: bold;
  color: #38bdf8;
}

.goal-label {
  color: #94a3b8;
}

.today-progress {
  margin-top: 1rem;
}

.progress-bar {
  height: 8px;
  background: #334155;
  border-radius: 4px;
  overflow: hidden;
  margin-bottom: 0.5rem;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #38bdf8, #22d3ee);
  transition: width 0.3s;
}

.progress-text {
  font-size: 0.875rem;
  color: #94a3b8;
}

.summary-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1rem;
  margin-bottom: 2rem;
}

.summary-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 1.5rem;
}

.summary-label {
  font-size: 0.875rem;
  color: #94a3b8;
  margin-bottom: 0.5rem;
}

.summary-value {
  font-size: 2rem;
  font-weight: bold;
  color: #f8fafc;
}

.chart-container {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 1.5rem;
  margin-bottom: 2rem;
}

.chart-container h3 {
  margin: 0 0 1.5rem 0;
  font-size: 1rem;
  color: #94a3b8;
}

.chart {
  display: flex;
  gap: 0.5rem;
  align-items: flex-end;
  min-height: 150px;
  padding: 1rem 0;
}

.chart-bar-group {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.25rem;
}

.chart-bar-container {
  display: flex;
  gap: 2px;
  align-items: flex-end;
  height: 120px;
}

.chart-bar {
  width: 8px;
  border-radius: 2px 2px 0 0;
  transition: height 0.3s;
}

.chart-bar.positive {
  background: #22d3ee;
}

.chart-bar.negative {
  background: #f97316;
}

.chart-label {
  font-size: 0.75rem;
  color: #64748b;
}

.chart-value {
  font-size: 0.75rem;
  color: #94a3b8;
  font-weight: 500;
}

.stats-table {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 1.5rem;
}

.stats-table h3 {
  margin: 0 0 1rem 0;
  font-size: 1rem;
  color: #94a3b8;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th {
  text-align: left;
  padding: 0.75rem;
  border-bottom: 1px solid #334155;
  color: #94a3b8;
  font-weight: 500;
  font-size: 0.875rem;
}

td {
  padding: 0.75rem;
  border-bottom: 1px solid #334155;
  color: #f8fafc;
}

tr:last-child td {
  border-bottom: none;
}

.positive {
  color: #22d3ee;
}

.negative {
  color: #f97316;
}

.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-content {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 2rem;
  width: 90%;
  max-width: 400px;
}

.modal-content h3 {
  margin: 0 0 1.5rem 0;
  font-size: 1.25rem;
}

.form-group {
  margin-bottom: 1.5rem;
}

.form-group label {
  display: block;
  margin-bottom: 0.5rem;
  color: #94a3b8;
  font-size: 0.875rem;
}

.form-group input {
  width: 100%;
  padding: 0.5rem;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 4px;
  color: #f8fafc;
  font-size: 1rem;
}

.hint {
  margin-top: 0.5rem;
  font-size: 0.75rem;
  color: #64748b;
}

.modal-actions {
  display: flex;
  gap: 1rem;
  justify-content: flex-end;
}

.btn-primary,
.btn-secondary {
  padding: 0.5rem 1rem;
  border-radius: 4px;
  border: none;
  font-size: 0.875rem;
  cursor: pointer;
  transition: background 0.2s;
}

.btn-primary {
  background: #38bdf8;
  color: #0f172a;
}

.btn-primary:hover {
  background: #22d3ee;
}

.btn-secondary {
  background: #334155;
  color: #f8fafc;
}

.btn-secondary:hover {
  background: #475569;
}
</style>
