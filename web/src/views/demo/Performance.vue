<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, Tickets, TrendCharts } from '@element-plus/icons-vue'
import { getDemoPerformance, restockDemo, runConcurrentDemo } from '@/api/demo'
import type { DemoConcurrentResult, DemoPerformanceSnapshot } from '@/api/types'
import { useAutoRefresh } from '@/composables/autoRefresh'

const router = useRouter()
const loading = ref(false)
const data = ref<DemoPerformanceSnapshot | null>(null)
const loadError = ref('')
const concurrentConcurrency = ref(20)
const concurrentQty = ref(1)
const concurrentRunning = ref(false)
const restockRunning = ref(false)
const concurrentResult = ref<DemoConcurrentResult | null>(null)
const concurrentError = ref('')

const concurrentDemand = computed(() => concurrentConcurrency.value * concurrentQty.value)

const dbHealthy = computed(() => data.value?.database.status === 'ok')
const redisHealthy = computed(() => data.value?.redis.status === 'ok')
const poolUsage = computed(() => {
  const pool = data.value?.pool
  if (!pool || pool.max_open_connections <= 0) return 0
  return Math.min(100, Math.round((pool.open_connections / pool.max_open_connections) * 100))
})
async function runAllocationExperiment() {
  if (concurrentRunning.value) return
  concurrentRunning.value = true
  concurrentResult.value = null
  concurrentError.value = ''
  try {
    concurrentResult.value = await runConcurrentDemo(concurrentConcurrency.value, concurrentQty.value)
    await load(true)
  } catch (error) {
    concurrentError.value = error instanceof Error ? error.message : '并发实验未完成'
  } finally {
    concurrentRunning.value = false
  }
}

async function restockForExperiment() {
  if (restockRunning.value) return
  restockRunning.value = true
  try {
    await restockDemo(500)
    await load(true)
  } finally {
    restockRunning.value = false
  }
}

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    data.value = await getDemoPerformance()
    loadError.value = ''
  } catch {
    loadError.value = '运行状态数据暂时不可用，请稍后重试'
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
        <h2>运行状态与指标快照</h2>
        <p>数据来自当前在线演示实例的真实状态查询；每 3 秒刷新一次。本页不是压力测试、吞吐量测试或容量结论。</p>
      </div>
      <div class="head-actions">
        <el-button :icon="Tickets" @click="router.push('/demo/activity')">业务证据</el-button>
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

    <section class="experiment-panel">
      <div class="experiment-head">
        <div>
          <span class="experiment-kicker">工程验证</span>
          <h3>并发库存分配一致性</h3>
          <p>只验证库存充足时，多张出库单并发创建、提交、审核后的 FIFO 分配和库存三数量一致性。</p>
        </div>
        <el-tag type="warning" effect="plain">不是供不应求证明</el-tag>
      </div>

      <div class="experiment-controls">
        <label>
          <span>并发订单数</span>
          <el-input-number v-model="concurrentConcurrency" :min="1" :max="100" />
        </label>
        <label>
          <span>每单需求</span>
          <el-input-number v-model="concurrentQty" :min="1" :max="10" />
        </label>
        <div class="demand-preview">
          <span>本次总需求</span>
          <b>{{ concurrentDemand }} 件</b>
        </div>
        <el-button :loading="restockRunning" @click="restockForExperiment">补货 500 件</el-button>
        <el-button type="primary" :loading="concurrentRunning" @click="runAllocationExperiment">
          运行真实并发实验
        </el-button>
      </div>

      <el-alert
        v-if="concurrentError"
        :title="concurrentError"
        type="warning"
        :closable="false"
        show-icon
      />

      <template v-if="concurrentResult">
        <div class="scope-grid">
          <div>
            <span>验证范围</span>
            <p>{{ concurrentResult.validation_scope }}</p>
          </div>
          <div>
            <span>不验证什么</span>
            <p>{{ concurrentResult.not_validated }}</p>
          </div>
          <div>
            <span>统计口径</span>
            <p>{{ concurrentResult.task_stats_scope }}</p>
          </div>
        </div>

        <div class="experiment-grid">
          <div class="snapshot-box">
            <b>实验开始前库存</b>
            <span>stock <strong>{{ concurrentResult.stock_before }}</strong></span>
            <span>available <strong>{{ concurrentResult.available_before }}</strong></span>
            <span>allocated <strong>{{ concurrentResult.allocated_before }}</strong></span>
          </div>
          <div class="snapshot-box">
            <b>本次实验输入</b>
            <span>订单数 <strong>{{ concurrentResult.concurrency }}</strong></span>
            <span>总需求 <strong>{{ concurrentResult.total_demand }}</strong></span>
            <span>每单需求 <strong>{{ concurrentResult.qty_per_order }}</strong></span>
          </div>
          <div class="snapshot-box">
            <b>单据执行结果</b>
            <span>创建成功 <strong>{{ concurrentResult.created_orders }}</strong></span>
            <span>提交成功 <strong>{{ concurrentResult.submitted_orders }}</strong></span>
            <span>审核成功 <strong>{{ concurrentResult.approved_orders }}</strong></span>
          </div>
          <div class="snapshot-box">
            <b>审核与任务证据</b>
            <span>审核失败 <strong>{{ concurrentResult.approval_failed }}</strong></span>
            <span>实际分配 <strong>{{ concurrentResult.allocated_quantity }} 件</strong></span>
            <span>本次 PICK <strong>{{ concurrentResult.pick_task_count }}</strong></span>
          </div>
          <div class="snapshot-box">
            <b>实验结束库存</b>
            <span>stock <strong>{{ concurrentResult.stock_total }}</strong></span>
            <span>available <strong>{{ concurrentResult.available_total }}</strong></span>
            <span>allocated <strong>{{ concurrentResult.allocated_total }}</strong></span>
          </div>
          <div class="snapshot-box" :class="{ failed: !concurrentResult.invariant_ok }">
            <b>一致性检查</b>
            <span>负数库存行 <strong>{{ concurrentResult.negative_rows }}</strong></span>
            <span>stock = available + allocated <strong>{{ concurrentResult.invariant_ok ? '成立' : '异常' }}</strong></span>
            <span>其他失败 <strong>{{ concurrentResult.other_failed }}</strong></span>
          </div>
        </div>

        <div class="experiment-summary" :class="{ failed: !concurrentResult.invariant_ok }">
          {{ concurrentResult.summary }}
        </div>
      </template>
    </section>

    <template v-if="data">
      <div class="health-grid">
        <div class="health-card" :class="{ down: !dbHealthy }">
          <div class="health-icon">DB</div>
          <div>
            <span>MySQL 状态</span>
            <strong>{{ dbHealthy ? '正常' : '异常' }}</strong>
            <small>当前探测延迟 {{ data.database.latency_ms }} ms</small>
          </div>
        </div>
        <div class="health-card" :class="{ down: !redisHealthy }">
          <div class="health-icon">R</div>
          <div>
            <span>Redis 状态</span>
            <strong>{{ redisHealthy ? '正常' : '异常' }}</strong>
            <small>当前探测延迟 {{ data.redis.latency_ms }} ms</small>
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
            <b>数据库连接池快照</b>
          </div>
          <p class="panel-note">当前连接数占最大连接数的 {{ poolUsage }}%，仅反映查询时刻的瞬时状态。</p>
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
            <b>当前业务统计</b>
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
            <b>进程内存快照</b>
          </div>
          <p class="panel-note">只展示 Go 运行时当前值。实例未配置容器内存上限，因此不计算占用百分比。</p>
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
          <b>指标说明</b>
        </div>
        <p class="load-desc">
          这是演示环境在查询时刻的运行状态与业务指标快照，不是压力测试、吞吐量测试或容量证明。
          执行「并发分配实验」后，可以结合刷新结果观察数据库连接、Go 协程和内存等指标。
          若出现「业务拒绝」并非故障：可用库存不足时，出库分配会按业务规则拒绝。
        </p>
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

.experiment-panel {
  margin-bottom: 16px;
  padding: 20px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  background: var(--el-bg-color);
  box-shadow: var(--el-box-shadow-light);
}

.experiment-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.experiment-head h3 {
  margin: 4px 0 6px;
  font-size: 18px;
}

.experiment-head p {
  max-width: 720px;
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.experiment-kicker {
  color: var(--el-color-primary);
  font-size: 12px;
  font-weight: 700;
}

.experiment-controls {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px;
  margin: 16px 0;
}

.experiment-controls label,
.demand-preview {
  display: flex;
  flex-direction: column;
  gap: 5px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.demand-preview {
  min-width: 100px;
  padding: 7px 10px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}

.demand-preview b {
  color: var(--el-text-color-primary);
  font-size: 15px;
}

.scope-grid,
.experiment-grid {
  display: grid;
  gap: 10px;
}

.scope-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: 14px;
}

.scope-grid div {
  padding: 12px;
  border-left: 3px solid var(--el-color-primary);
  border-radius: 6px;
  background: var(--el-fill-color-extra-light);
}

.scope-grid span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.scope-grid p {
  margin: 5px 0 0;
  font-size: 13px;
  line-height: 1.65;
}

.experiment-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: 12px;
}

.snapshot-box {
  padding: 14px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 9px;
  background: var(--el-fill-color-extra-light);
}

.snapshot-box b {
  display: block;
  margin-bottom: 8px;
}

.snapshot-box span {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 2px 0;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.snapshot-box strong {
  color: var(--el-text-color-primary);
}

.snapshot-box.failed {
  border-color: var(--el-color-danger-light-5);
}

.experiment-summary {
  margin-top: 12px;
  padding: 12px 14px;
  border-radius: 8px;
  color: var(--el-color-success);
  background: var(--el-color-success-light-9);
  font-weight: 600;
}

.experiment-summary.failed {
  color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
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

.panel-note {
  margin: -4px 0 14px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
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
  margin: 0;
  color: var(--el-text-color-secondary);
  line-height: 1.8;
}

.last-updated {
  margin-top: 12px;
  text-align: right;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

@media (max-width: 1100px) {
  .health-grid,
  .experiment-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .experiment-head {
    flex-direction: column;
  }

  .scope-grid,
  .experiment-grid {
    grid-template-columns: 1fr;
  }

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