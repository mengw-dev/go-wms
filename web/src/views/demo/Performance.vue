<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, Tickets, TrendCharts } from '@element-plus/icons-vue'
import {
  getDemoPerformance,
  restockDemo,
  runConcurrentDemo,
  runConcurrentPicking,
  runConcurrentShortageDemo,
} from '@/api/demo'
import type {
  DemoConcurrentResult,
  DemoConcurrentShortageResult,
  DemoPerformanceSnapshot,
  DemoPickingResult,
} from '@/api/types'
import PageHeader from '@/components/common/PageHeader.vue'
import { useAutoRefresh } from '@/composables/autoRefresh'

type AllocationMode = 'sufficient' | 'shortage'

interface Metric {
  label: string
  value: string | number
  tone?: 'success' | 'danger' | 'warning'
}

const router = useRouter()
const loading = ref(false)
const loadError = ref('')
const data = ref<DemoPerformanceSnapshot | null>(null)

const allocationDialogVisible = ref(false)
const allocationMode = ref<AllocationMode>('sufficient')
const allocationConcurrency = ref(20)
const allocationInitialStock = ref(100)
const allocationQty = ref(5)
const allocationRunning = ref(false)
const allocationError = ref('')
const concurrentResult = ref<DemoConcurrentResult | null>(null)
const shortageResult = ref<DemoConcurrentShortageResult | null>(null)

const pickingDialogVisible = ref(false)
const pickingWorkers = ref(10)
const pickingContenders = ref(5)
const pickingRunning = ref(false)
const pickingError = ref('')
const pickingResult = ref<DemoPickingResult | null>(null)

const mechanismDialogVisible = ref(false)
const runtimeVisible = ref(false)

const currentAvailable = computed(() => data.value?.business.available_total ?? 0)
const allocationDemand = computed(() => allocationConcurrency.value * allocationQty.value)
const allocationResult = computed(() => allocationMode.value === 'shortage' ? shortageResult.value : concurrentResult.value)
const allocationMetrics = computed<Metric[]>(() => {
  const result = allocationResult.value
  if (!result) return []
  if (allocationMode.value === 'shortage') {
    const shortage = result as DemoConcurrentShortageResult
    return [
      { label: '成功分配', value: shortage.success, tone: 'success' },
      { label: '拒绝请求', value: shortage.insufficient_rejected + shortage.other_failed, tone: shortage.insufficient_rejected ? 'warning' : undefined },
      { label: '剩余库存', value: shortage.remaining_available },
      { label: '是否超卖', value: shortage.limit_respected ? '未超卖' : '异常', tone: shortage.limit_respected ? 'success' : 'danger' },
      { label: '负库存行', value: shortage.negative_rows, tone: shortage.negative_rows ? 'danger' : 'success' },
      { label: '库存不变量', value: shortage.invariant_ok ? '通过' : '异常', tone: shortage.invariant_ok ? 'success' : 'danger' },
      { label: 'FIFO 执行', value: shortage.allocated_quantity > 0 ? '已执行' : '未分配' },
    ]
  }
  const sufficient = result as DemoConcurrentResult
  return [
    { label: '成功分配', value: sufficient.success, tone: 'success' },
    { label: '拒绝请求', value: sufficient.failed, tone: sufficient.failed ? 'warning' : undefined },
    { label: '剩余库存', value: sufficient.available_total },
    { label: '是否超卖', value: sufficient.negative_rows === 0 && sufficient.invariant_ok ? '未超卖' : '异常', tone: sufficient.negative_rows === 0 && sufficient.invariant_ok ? 'success' : 'danger' },
    { label: '负库存行', value: sufficient.negative_rows, tone: sufficient.negative_rows ? 'danger' : 'success' },
    { label: '库存不变量', value: sufficient.invariant_ok ? '通过' : '异常', tone: sufficient.invariant_ok ? 'success' : 'danger' },
    { label: 'FIFO 执行', value: sufficient.allocated_quantity === sufficient.total_demand ? '按需求完成' : '部分完成' },
  ]
})
const pickingMetrics = computed<Metric[]>(() => {
  const result = pickingResult.value
  if (!result) return []
  return [
    { label: '完成任务', value: result.completed_tasks, tone: 'success' },
    { label: '重复扫码拦截', value: result.duplicate_scan_rejected, tone: result.duplicate_scan_rejected ? 'warning' : undefined },
    { label: '重复发货', value: '接口未返回' },
    { label: '任务状态', value: result.completed_tasks === result.task_count ? '全部完成' : '部分完成', tone: result.completed_tasks === result.task_count ? 'success' : 'warning' },
    { label: '库存证据', value: result.inventory_trans.length },
    { label: '已发货订单', value: result.shipped_orders, tone: 'success' },
  ]
})
const allocationLast = computed(() => allocationResult.value?.summary || '尚未运行')
const pickingLast = computed(() => pickingResult.value?.summary || '尚未运行')

async function load(silent = false): Promise<void> {
  if (!silent) loading.value = true
  try {
    data.value = await getDemoPerformance()
    loadError.value = ''
  } catch {
    loadError.value = '运行状态暂时不可用'
  } finally {
    if (!silent) loading.value = false
  }
}

async function ensureAllocationStock(): Promise<void> {
  const target = Math.max(1, allocationInitialStock.value)
  if (target <= currentAvailable.value) return
  await restockDemo(target - currentAvailable.value)
  await load(true)
}

async function runAllocation(): Promise<void> {
  if (allocationRunning.value) return
  allocationRunning.value = true
  allocationError.value = ''
  if (allocationMode.value === 'sufficient') concurrentResult.value = null
  else shortageResult.value = null
  try {
    await ensureAllocationStock()
    if (allocationMode.value === 'shortage' && allocationDemand.value <= currentAvailable.value) {
      allocationError.value = '供给不足模式要求总需求高于当前可用库存。请提高请求数或每单数量。'
      return
    }
    if (allocationMode.value === 'sufficient') {
      concurrentResult.value = await runConcurrentDemo(allocationConcurrency.value, allocationQty.value)
    } else {
      shortageResult.value = await runConcurrentShortageDemo(allocationConcurrency.value, allocationQty.value)
    }
    await load(true)
  } catch (error) {
    allocationError.value = error instanceof Error ? error.message : '并发实验未完成'
  } finally {
    allocationRunning.value = false
  }
}

async function runPicking(): Promise<void> {
  if (pickingRunning.value) return
  pickingRunning.value = true
  pickingError.value = ''
  pickingResult.value = null
  try {
    pickingResult.value = await runConcurrentPicking(pickingWorkers.value, pickingContenders.value)
    await load(true)
  } catch (error) {
    pickingError.value = error instanceof Error ? error.message : '拣货实验未完成'
  } finally {
    pickingRunning.value = false
  }
}

function openAllocation(mode: AllocationMode): void {
  allocationMode.value = mode
  allocationDialogVisible.value = true
}

onMounted(() => load())
useAutoRefresh(
  () => load(true),
  5000,
  () => !allocationDialogVisible.value && !pickingDialogVisible.value && !mechanismDialogVisible.value && !runtimeVisible.value,
)
</script>

<template>
  <div v-loading="loading" class="app-page engineering-page">
    <PageHeader title="工程验证" description="用三个核心实验验证真实业务接口的并发边界、任务状态和异步任务可靠性。">
      <template #actions>
        <el-button @click="router.push('/demo')">返回演示中心</el-button>
        <el-button :icon="Tickets" @click="router.push('/demo/activity')">业务证据</el-button>
        <el-button :icon="TrendCharts" @click="runtimeVisible = true">运行状态</el-button>
        <el-button :icon="Refresh" type="primary" @click="load()">刷新</el-button>
      </template>
    </PageHeader>

    <div class="platform-strip">
      <span><i :class="data?.database.status === 'ok' ? 'ok' : 'warn'"></i>MySQL {{ data?.database.status || '检查中' }}</span>
      <span><i :class="data?.redis.status === 'ok' ? 'ok' : 'warn'"></i>Redis {{ data?.redis.status || '检查中' }}</span>
      <span>当前可用库存 <b>{{ currentAvailable }}</b></span>
      <span v-if="loadError" class="warn-text">{{ loadError }}</span>
    </div>

    <section class="engineering-grid">
      <article class="app-card engineering-card" data-section="allocation">
        <div class="card-head">
          <span class="card-index">01</span>
          <div><h3>并发库存分配</h3><p>验证 FIFO、库存边界和防超卖，可切换库存充足或供给不足模式。</p></div>
        </div>
        <div class="tag-line"><span>并发分配</span><span>FIFO</span><span>防超卖</span></div>
        <div class="last-result"><span>上次结果</span><b>{{ allocationLast }}</b></div>
        <div class="card-actions">
          <el-button type="primary" @click="openAllocation('sufficient')">库存充足模式</el-button>
          <el-button @click="openAllocation('shortage')">供给不足模式</el-button>
        </div>
      </article>

      <article class="app-card engineering-card" data-section="picking">
        <div class="card-head">
          <span class="card-index">02</span>
          <div><h3>拣货作业验证</h3><p>模拟多个拣货请求并发操作，验证重复扫码拦截和任务闭环。</p></div>
        </div>
        <div class="tag-line"><span>并发拣货</span><span>重复扫码</span><span>任务状态</span></div>
        <div class="last-result"><span>上次结果</span><b>{{ pickingLast }}</b></div>
        <div class="card-actions"><el-button type="primary" @click="pickingDialogVisible = true">配置并运行</el-button></div>
      </article>

      <article class="app-card engineering-card" data-section="import">
        <div class="card-head">
          <span class="card-index">03</span>
          <div><h3>异步导入可靠性</h3><p>查看异步任务领取、行级幂等、失败保留和补偿恢复机制。</p></div>
        </div>
        <div class="tag-line"><span>异步任务</span><span>可重试</span><span>可恢复</span></div>
        <div class="mechanism-note"><b>当前只展示机制</b><span>没有可用安全实验接口，不生成静态成功数据。</span></div>
        <div class="card-actions"><el-button type="primary" @click="mechanismDialogVisible = true">查看机制</el-button></div>
      </article>
    </section>
  </div>

  <el-dialog v-model="allocationDialogVisible" title="配置并发库存分配" width="min(760px, 94vw)" :close-on-click-modal="false">
    <el-radio-group v-model="allocationMode" class="mode-switch">
      <el-radio-button value="sufficient">库存充足</el-radio-button>
      <el-radio-button value="shortage">供给不足</el-radio-button>
    </el-radio-group>
    <div class="config-grid">
      <label><span>并发请求数</span><el-input-number v-model="allocationConcurrency" :min="1" :max="100" /></label>
      <label><span>初始库存目标</span><el-input-number v-model="allocationInitialStock" :min="1" :max="5000" /></label>
      <label><span>每单申请数量</span><el-input-number v-model="allocationQty" :min="1" :max="10" /></label>
    </div>
    <div class="config-note">本次总需求 {{ allocationDemand }} 件；初始库存不足目标值时，会先调用真实补货接口。</div>
    <el-alert v-if="allocationError" :title="allocationError" type="warning" :closable="false" show-icon />
    <div v-if="allocationMetrics.length" class="result-grid">
      <div v-for="item in allocationMetrics" :key="item.label"><span>{{ item.label }}</span><b :class="item.tone">{{ item.value }}</b></div>
    </div>
    <template #footer>
      <el-button @click="allocationDialogVisible = false">关闭</el-button>
      <el-button type="primary" :loading="allocationRunning" @click="runAllocation">运行实验</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="pickingDialogVisible" title="配置拣货作业验证" width="min(720px, 94vw)" :close-on-click-modal="false">
    <div class="config-grid">
      <label><span>并发拣货员</span><el-input-number v-model="pickingWorkers" :min="1" :max="50" /></label>
      <label><span>抢单 / 重复请求数</span><el-input-number v-model="pickingContenders" :min="0" :max="50" /></label>
      <label class="switch-field"><span>模拟并发操作</span><el-switch :model-value="true" disabled /></label>
      <label class="switch-field"><span>模拟重复扫码</span><el-switch :model-value="true" disabled /></label>
    </div>
    <div class="config-note">拣货任务由实验接口按真实库存自动准备；“重复发货”没有独立接口字段，结果不会伪造。</div>
    <el-alert v-if="pickingError" :title="pickingError" type="warning" :closable="false" show-icon />
    <div v-if="pickingMetrics.length" class="result-grid">
      <div v-for="item in pickingMetrics" :key="item.label"><span>{{ item.label }}</span><b :class="item.tone">{{ item.value }}</b></div>
    </div>
    <template #footer>
      <el-button @click="pickingDialogVisible = false">关闭</el-button>
      <el-button @click="router.push('/demo/activity')">查看记录</el-button>
      <el-button type="primary" :loading="pickingRunning" @click="runPicking">运行模拟 PDA 实验</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="mechanismDialogVisible" title="异步导入可靠性机制" width="min(680px, 94vw)" :close-on-click-modal="false">
    <div class="mechanism-grid">
      <div><b>任务领取</b><span>PENDING 任务由单消费者领取，使用执行标识防止旧 worker 覆盖新状态。</span></div>
      <div><b>行级幂等</b><span>以任务 ID 和 Excel 行号保证补偿重跑不会重复建单。</span></div>
      <div><b>失败保留</b><span>失败任务保留源文件用于排查；成功完成后才清理源文件。</span></div>
      <div><b>恢复与重试</b><span>处理中超时的任务会被后续 worker 重新领取，并受租户和事务边界保护。</span></div>
    </div>
    <el-alert title="当前页面只展示实现机制，不使用静态成功数据冒充真实实验。" type="info" :closable="false" show-icon />
    <template #footer><el-button type="primary" @click="mechanismDialogVisible = false">关闭</el-button></template>
  </el-dialog>

  <el-drawer v-model="runtimeVisible" title="运行状态与指标快照" size="min(460px, 94vw)" append-to-body>
    <div v-if="data" class="runtime-list">
      <div><span>数据库</span><b>{{ data.database.status }} · {{ data.database.latency_ms }} ms</b></div>
      <div><span>Redis</span><b>{{ data.redis.status }} · {{ data.redis.latency_ms }} ms</b></div>
      <div><span>数据库连接</span><b>{{ data.pool.open_connections }} / {{ data.pool.max_open_connections }}</b></div>
      <div><span>Go 协程</span><b>{{ data.runtime.goroutines }}</b></div>
      <div><span>内存占用</span><b>{{ data.runtime.memory_alloc_mb }} MB</b></div>
      <div><span>库存三数量</span><b>现存 {{ data.business.stock_total }} / 可用 {{ data.business.available_total }} / 分配 {{ data.business.allocated_total }}</b></div>
    </div>
  </el-drawer>
</template>

<style scoped>
.engineering-page { width: min(1180px, 100%); margin: 0 auto; }
.platform-strip { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 18px; padding: 10px 14px; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; color: var(--el-text-color-secondary); background: var(--el-bg-color); font-size: 12px; }
.platform-strip span { display: inline-flex; align-items: center; gap: 6px; }
.platform-strip i { width: 7px; height: 7px; border-radius: 50%; background: var(--el-color-warning); }
.platform-strip i.ok { background: var(--el-color-success); }
.platform-strip i.warn, .warn-text { color: var(--el-color-warning); }
.platform-strip b { color: var(--el-text-color-primary); }
.engineering-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
.engineering-card { min-height: 260px; padding: 18px; display: flex; flex-direction: column; }
.card-head { display: grid; grid-template-columns: 32px minmax(0, 1fr); gap: 10px; }
.card-index { width: 32px; height: 32px; border-radius: 9px; display: grid; place-items: center; color: var(--el-color-primary); background: var(--el-color-primary-light-9); font-family: var(--gowms-num-font); font-size: 12px; font-weight: 700; }
.card-head h3 { margin: 2px 0 6px; font-size: 17px; }
.card-head p { margin: 0; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.65; }
.tag-line { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 14px; }
.tag-line span { padding: 3px 8px; border-radius: 999px; color: var(--el-color-primary); background: var(--el-color-primary-light-9); font-size: 11px; }
.last-result, .mechanism-note { min-height: 62px; margin-top: 14px; padding: 10px 11px; border-radius: 9px; background: var(--el-fill-color-lighter); }
.last-result span, .last-result b, .mechanism-note b, .mechanism-note span { display: block; }
.last-result span, .mechanism-note span { color: var(--el-text-color-secondary); font-size: 11px; }
.last-result b, .mechanism-note b { margin-top: 4px; font-size: 12px; line-height: 1.5; }
.card-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: auto; padding-top: 14px; }
.card-actions .el-button { margin-left: 0; }
.mode-switch { margin-bottom: 16px; }
.config-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.config-grid label { min-width: 0; padding: 11px 12px; border: 1px solid var(--el-border-color-lighter); border-radius: 9px; display: grid; gap: 7px; }
.config-grid label > span { color: var(--el-text-color-secondary); font-size: 12px; }
.config-grid .el-input-number { width: 100%; }
.switch-field { display: flex !important; align-items: center; justify-content: space-between; }
.config-note { margin: 12px 0; padding: 9px 11px; border-radius: 8px; color: var(--el-text-color-secondary); background: var(--el-fill-color-light); font-size: 12px; line-height: 1.6; }
.result-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; margin-top: 14px; }
.result-grid > div { min-width: 0; padding: 10px; border-radius: 9px; background: var(--el-fill-color-extra-light); }
.result-grid span, .result-grid b { display: block; }
.result-grid span { color: var(--el-text-color-secondary); font-size: 11px; }
.result-grid b { margin-top: 4px; font-size: 16px; }
.result-grid b.success { color: var(--el-color-success); }
.result-grid b.warning { color: var(--el-color-warning); }
.result-grid b.danger { color: var(--el-color-danger); }
.mechanism-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; margin-bottom: 14px; }
.mechanism-grid > div { padding: 12px; border-radius: 9px; background: var(--el-fill-color-extra-light); }
.mechanism-grid b, .mechanism-grid span { display: block; }
.mechanism-grid span { margin-top: 5px; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.6; }
.runtime-list { display: grid; gap: 8px; }
.runtime-list > div { padding: 11px 12px; border-radius: 9px; background: var(--el-fill-color-extra-light); }
.runtime-list span, .runtime-list b { display: block; }
.runtime-list span { color: var(--el-text-color-secondary); font-size: 11px; }
.runtime-list b { margin-top: 4px; font-size: 13px; }
@media (max-width: 900px) { .engineering-grid { grid-template-columns: 1fr; } .result-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
