<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, Tickets, TrendCharts } from '@element-plus/icons-vue'
import { getDemoPerformance } from '@/api/demo'
import type { DemoPerformanceSnapshot } from '@/api/types'
import { useAutoRefresh } from '@/composables/autoRefresh'

const router = useRouter()
const loading = ref(false)
const data = ref<DemoPerformanceSnapshot | null>(null)
const loadError = ref('')

const dbHealthy = computed(() => data.value?.database.status === 'ok')
const redisHealthy = computed(() => data.value?.redis.status === 'ok')
const poolUsage = computed(() => {
  const pool = data.value?.pool
  if (!pool || pool.max_open_connections <= 0) return 0
  return Math.min(100, Math.round((pool.open_connections / pool.max_open_connections) * 100))
})
const memoryUsage = computed(() => {
  if (!data.value) return 0
  return Math.min(100, Math.round((data.value.runtime.memory_sys_mb / 512) * 100))
})

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    data.value = await getDemoPerformance()
    loadError.value = ''
  } catch {
    loadError.value = '性能数据暂时不可用，请稍后重试'
  } finally {
    if (!silent) loading.value = false
  }
}

function formatTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => load())
useAutoRefresh(() => load(true), 3000)
</script>

<template>
  <div v-loading="loading" class="performance-page">
    <div class="page-head">
      <div>
        <h2>系统性能指标</h2>
        <p>页面每 3 秒自动刷新，可直接向 HR 展示数据库、Redis、连接池和业务实时状态。</p>
      </div>
      <div class="head-actions">
        <el-button :icon="Tickets" @click="router.push('/demo/activity')">操作记录</el-button>
        <el-button :icon="Refresh" type="primary" @click="load()">立即刷新</el-button>
      </div>
    </div>

    <el-alert
      v-if="loadError"
      :title="loadError"
      type="warning"
      show-icon
      :closable="false"
      class="page-alert"
    />

    <template v-if="data">
      <div class="health-grid">
        <div class="health-card" :class="{ down: !dbHealthy }">
          <div class="health-icon">DB</div>
          <div>
            <span>MySQL 状态</span>
            <strong>{{ dbHealthy ? '正常' : '异常' }}</strong>
            <small>{{ data.database.latency_ms }} ms</small>
          </div>
        </div>
        <div class="health-card" :class="{ down: !redisHealthy }">
          <div class="health-icon">R</div>
          <div>
            <span>Redis 状态</span>
            <strong>{{ redisHealthy ? '正常' : '异常' }}</strong>
            <small>{{ data.redis.latency_ms }} ms</small>
          </div>
        </div>
        <div class="health-card">
          <div class="health-icon">G</div>
          <div>
            <span>Go 协程</span>
            <strong>{{ data.runtime.goroutines }}</strong>
            <small>当前运行中</small>
          </div>
        </div>
        <div class="health-card">
          <div class="health-icon">M</div>
          <div>
            <span>进程内存</span>
            <strong>{{ data.runtime.memory_alloc_mb.toFixed(1) }} MB</strong>
            <small>已分配</small>
          </div>
        </div>
      </div>

      <div class="metrics-grid">
        <section class="metric-panel">
          <div class="panel-title">
            <el-icon><TrendCharts /></el-icon>
            <b>数据库连接池</b>
          </div>
          <div class="progress-row">
            <span>连接使用率</span>
            <strong>{{ poolUsage }}%</strong>
          </div>
          <el-progress :percentage="poolUsage" :stroke-width="12" />
          <div class="metric-list">
            <span>最大连接 <b>{{ data.pool.max_open_connections }}</b></span>
            <span>已打开 <b>{{ data.pool.open_connections }}</b></span>
            <span>使用中 <b>{{ data.pool.in_use }}</b></span>
            <span>空闲 <b>{{ data.pool.idle }}</b></span>
          </div>
          <div class="metric-foot">
            等待次数 {{ data.pool.wait_count }}，累计等待 {{ data.pool.wait_duration_ms }} ms
          </div>
        </section>

        <section class="metric-panel">
          <div class="panel-title">
            <el-icon><Tickets /></el-icon>
            <b>今日业务与任务</b>
          </div>
          <div class="number-grid">
            <div><span>今日入库</span><strong>{{ data.business.inbound_today }}</strong></div>
            <div><span>今日出库</span><strong>{{ data.business.outbound_today }}</strong></div>
            <div><span>待办任务</span><strong>{{ data.business.pending_tasks }}</strong></div>
            <div><span>库存行数</span><strong>{{ data.business.inventory_rows }}</strong></div>
          </div>
        </section>

        <section class="metric-panel">
          <div class="panel-title">
            <b>库存三数量校验</b>
          </div>
          <div class="stock-total">{{ data.business.stock_total }}</div>
          <p class="stock-formula">
            stock = available + allocated
          </p>
          <div class="metric-list two-col">
            <span>可用量 <b>{{ data.business.available_total }}</b></span>
            <span>分配量 <b>{{ data.business.allocated_total }}</b></span>
          </div>
        </section>

        <section class="metric-panel">
          <div class="panel-title">
            <b>进程内存</b>
          </div>
          <div class="progress-row">
            <span>系统内存占用</span>
            <strong>{{ memoryUsage }}%</strong>
          </div>
          <el-progress :percentage="memoryUsage" :stroke-width="12" color="#10b981" />
          <div class="metric-list">
            <span>Alloc <b>{{ data.runtime.memory_alloc_mb.toFixed(1) }} MB</b></span>
            <span>Sys <b>{{ data.runtime.memory_sys_mb.toFixed(1) }} MB</b></span>
            <span>GC 次数 <b>{{ data.runtime.num_gc }}</b></span>
          </div>
        </section>
      </div>

      <section class="load-panel">
        <div class="panel-title">
          <el-icon><TrendCharts /></el-icon>
          <b>怎样向 HR 展示真实压测</b>
        </div>
        <p class="load-desc">
          页面中的“并发演示”会从后端并发创建并处理出库单，展示库存锁和防超卖；
          真正的 HTTP 压测使用 k6，在压测进行时 HR 可以停留在这个页面观察连接池、协程和内存变化。
        </p>
        <div class="command-list">
          <code>.\scripts\windows\run-k6.ps1 -Mode flow -BaseUrl http://127.0.0.1:18080</code>
          <code>.\scripts\windows\run-k6.ps1 -Mode stress -BaseUrl http://127.0.0.1:18080 -WarehouseId 3 -SkuId 11 -RemoteWrite</code>
          <code>.\scripts\windows\run-k6.ps1 -Mode wave -BaseUrl http://127.0.0.1:18080 -WarehouseId 3 -SkuId 11</code>
        </div>
      </section>

      <div class="last-updated">最后更新时间：{{ formatTime(data.checked_at) }}</div>
    </template>
  </div>
</template>

<style scoped>
.performance-page {
  min-height: 100%;
}

.page-head {
  display: flex;
  justify-content: space-between;
  gap: 20px;
  align-items: flex-start;
  margin-bottom: 16px;
}

.page-head h2 {
  margin: 0 0 6px;
  font-size: 22px;
}

.page-head p {
  margin: 0;
  color: var(--el-text-color-secondary);
}

.head-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.page-alert {
  margin-bottom: 16px;
}

.health-grid,
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.health-card,
.metric-panel,
.load-panel {
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  background: var(--el-bg-color);
  box-shadow: var(--el-box-shadow-light);
}

.health-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px;
  border-top: 3px solid #10b981;
}

.health-card.down {
  border-top-color: #ef4444;
}

.health-icon {
  width: 42px;
  height: 42px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  background: var(--el-fill-color-light);
  color: var(--el-color-primary);
  font-weight: 800;
}

.health-card span,
.health-card small {
  display: block;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.health-card strong {
  display: block;
  margin: 4px 0;
  font-size: 20px;
}

.metrics-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin-top: 14px;
}

.metric-panel {
  padding: 18px;
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  font-size: 15px;
}

.progress-row {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.metric-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 16px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.metric-list.two-col {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.metric-list b {
  color: var(--el-text-color-primary);
}

.metric-foot {
  margin-top: 14px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.number-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.number-grid div {
  padding: 14px;
  border-radius: 10px;
  background: var(--el-fill-color-lighter);
}

.number-grid span {
  display: block;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.number-grid strong {
  display: block;
  margin-top: 6px;
  font-size: 26px;
}

.stock-total {
  font-size: 36px;
  font-weight: 800;
  color: var(--el-color-primary);
}

.stock-formula {
  margin: 4px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.load-panel {
  margin-top: 14px;
  padding: 18px;
}

.load-desc {
  margin: 0 0 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.8;
}

.command-list {
  display: grid;
  gap: 8px;
}

.command-list code {
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
  overflow-x: auto;
}

.last-updated {
  margin-top: 12px;
  text-align: right;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

@media (max-width: 1100px) {
  .health-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .page-head,
  .health-grid,
  .metrics-grid {
    grid-template-columns: 1fr;
  }

  .page-head {
    display: block;
  }

  .head-actions {
    margin-top: 12px;
  }
}
</style>