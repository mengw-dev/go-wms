<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ArrowRight,
  Box,
  Connection,
  Document,
  Download,
  Monitor,
  QuestionFilled,
  Refresh,
  Tickets,
  TrendCharts,
  Upload,
  VideoPlay,
} from '@element-plus/icons-vue'
import { getDemoActivity, releaseDemoSession, resetDemoData } from '@/api/demo'
import type { DemoActivitySnapshot } from '@/api/types'
import DemoTour from '@/components/demo/DemoTour.vue'
import { useAuthStore } from '@/stores/auth'
import { useGuideStore, type GuideScenario } from '@/stores/guide'
import { statusText, taskTypeText } from '@/constants'
import { formatTime } from '@/utils'
import { onDataChanged, runDemoScenarioInConsole } from '@/utils/events'

interface EvidenceItem {
  kind: string
  title: string
  detail: string
  createdAt: string
  path: string
}

const router = useRouter()
const auth = useAuthStore()
const guide = useGuideStore()

const activity = ref<DemoActivitySnapshot | null>(null)
const activityLoading = ref(false)
const resetLoading = ref(false)
const exitLoading = ref(false)
const remaining = ref(auth.demoSessionExpiresIn || 300)
const demoTour = ref<{ open: () => void } | null>(null)

let countdownTimer: number | undefined
let disposeDataChanged: (() => void) | undefined

const remainingText = computed(() => {
  const seconds = Math.max(0, remaining.value)
  const minutes = Math.floor(seconds / 60)
  return `${String(minutes).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
})

const evidenceCounts = computed(() => [
  { label: '入库单', value: activity.value?.inbound_orders.length ?? 0 },
  { label: '出库单', value: activity.value?.outbound_orders.length ?? 0 },
  { label: '盘点单', value: activity.value?.stocktake_orders.length ?? 0 },
  { label: '任务', value: activity.value?.tasks.length ?? 0 },
  { label: '库存流水', value: activity.value?.inventory_trans.length ?? 0 },
])

const latestEvidence = computed<EvidenceItem | null>(() => {
  const current = activity.value
  if (!current) return null

  const items: EvidenceItem[] = [
    ...current.inbound_orders.map((item) => ({
      kind: '入库单',
      title: item.order_no,
      detail: statusText(item.status),
      createdAt: item.created_at,
      path: `/inbound/orders/${item.id}`,
    })),
    ...current.outbound_orders.map((item) => ({
      kind: '出库单',
      title: item.order_no,
      detail: statusText(item.status),
      createdAt: item.created_at,
      path: `/outbound/orders/${item.id}`,
    })),
    ...current.stocktake_orders.map((item) => ({
      kind: '盘点单',
      title: item.order_no,
      detail: statusText(item.status),
      createdAt: item.created_at,
      path: `/stocktake/orders/${item.id}`,
    })),
    ...current.tasks.map((item) => ({
      kind: '作业任务',
      title: item.task_no,
      detail: `${taskTypeText(item.task_type)} · ${statusText(item.status)}`,
      createdAt: item.created_at,
      path: '/tasks',
    })),
    ...current.inventory_trans.map((item) => ({
      kind: '库存流水',
      title: item.order_no || item.task_no || '库存变更',
      detail: `${item.trans_type} · ${item.quantity_change > 0 ? '+' : ''}${item.quantity_change}`,
      createdAt: item.created_at,
      path: '/inventory',
    })),
  ]

  return (
    items
      .filter((item) => item.createdAt)
      .sort((left, right) => new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime())[0] ??
    null
  )
})

watch(
  () => auth.demoSessionExpiresIn,
  (value) => {
    if (value > 0) remaining.value = value
  },
)

async function loadActivity() {
  activityLoading.value = true
  try {
    activity.value = await getDemoActivity(8)
  } finally {
    activityLoading.value = false
  }
}

function startCountdown() {
  countdownTimer = window.setInterval(() => {
    if (remaining.value > 0) remaining.value -= 1
  }, 1000)
}

function startManualGuide(scenario: GuideScenario) {
  const firstStep = guide.start(scenario)
  void router.push(firstStep.route)
}

function runScenario(scenario: 'inbound' | 'outbound' | 'stocktake' | 'full') {
  runDemoScenarioInConsole(scenario)
}

function goPerformance() {
  router.push('/demo/performance')
}

function goActivity() {
  router.push('/demo/activity')
}

function openTour() {
  demoTour.value?.open()
}

function openOverview() {
  window.open('/overview.html', '_blank', 'noopener,noreferrer')
}

function openSource() {
  window.open('https://github.com/mengw-dev/go-wms', '_blank', 'noopener,noreferrer')
}

async function resetData() {
  try {
    await ElMessageBox.confirm(
      '将恢复仓库、货品、库存和单据到初始演示数据，确定继续吗？',
      '重置演示数据',
      { type: 'warning', confirmButtonText: '确定重置', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  resetLoading.value = true
  try {
    await resetDemoData()
    await loadActivity()
    ElMessage.success('演示数据已恢复为初始状态')
  } finally {
    resetLoading.value = false
  }
}

async function releaseAndExit() {
  try {
    await ElMessageBox.confirm(
      '退出后当前演示数据会立即恢复初始状态，确定退出吗？',
      '退出演示',
      { type: 'warning', confirmButtonText: '退出并重置', cancelButtonText: '继续演示' },
    )
  } catch {
    return
  }
  exitLoading.value = true
  try {
    await releaseDemoSession().catch(() => undefined)
    auth.clear()
    await router.push('/login')
  } finally {
    exitLoading.value = false
  }
}

onMounted(() => {
  startCountdown()
  void loadActivity()
  disposeDataChanged = onDataChanged(() => void loadActivity())
})

onBeforeUnmount(() => {
  if (countdownTimer !== undefined) window.clearInterval(countdownTimer)
  disposeDataChanged?.()
})
</script>

<template>
  <div class="demo-home">
    <section class="session-bar" data-tour="demo-session" aria-label="演示环境状态">
      <div class="session-status">
        <el-tag type="success" effect="plain">独立演示租户</el-tag>
        <span class="status-item">
          会话剩余 <b>{{ remainingText }}</b>
        </span>
        <span class="status-divider" aria-hidden="true"></span>
        <span class="status-item status-isolation"><i></i>数据隔离已启用</span>
      </div>
      <div class="session-actions">
        <el-button text :icon="QuestionFilled" @click="openTour">快速导览</el-button>
        <el-button text :loading="resetLoading" @click="resetData">重置数据</el-button>
        <el-button text type="danger" :loading="exitLoading" @click="releaseAndExit">退出并重置</el-button>
      </div>
    </section>

    <section class="hero-panel">
      <div class="hero-copy">
        <span class="eyebrow">WMS 项目体验中心</span>
        <h1>从真实业务流程理解这套 WMS</h1>
        <p>
          面向中小型仓储场景的模块化单体 WMS，覆盖入库、出库、库存与盘点闭环，重点展示库存一致性、
          事务边界和多租户隔离。
        </p>
        <div class="hero-actions">
          <el-button type="primary" size="large" :icon="VideoPlay" @click="runScenario('full')">
            自动演示完整业务闭环
          </el-button>
          <el-button size="large" :icon="ArrowRight" @click="startManualGuide('inbound')">
            亲自体验入库
          </el-button>
        </div>
      </div>
      <div class="flow-panel" data-tour="complete-flow">
        <div class="flow-head">
          <b>完整业务闭环</b>
          <small>每一步都调用真实业务 Service</small>
        </div>
        <ol class="flow-list">
          <li><span>01</span><b>入库</b><small>创建入库单</small></li>
          <li><span>02</span><b>收货</b><small>登记批次与数量</small></li>
          <li><span>03</span><b>上架</b><small>生成库存</small></li>
          <li><span>04</span><b>出库审核</b><small>FIFO 锁库</small></li>
          <li><span>05</span><b>拣货</b><small>扣减库存</small></li>
          <li><span>06</span><b>盘点</b><small>核对差异</small></li>
        </ol>
        <div class="flow-note">
          <Connection />
          <span>入库单、库存流水、任务状态和出库单可在业务页面继续核对。</span>
        </div>
      </div>
    </section>

    <section class="home-section">
      <div class="section-heading">
        <div>
          <span class="section-kicker">业务场景</span>
          <h2>选择一项业务开始体验</h2>
        </div>
        <p>场景入口保留参数控制和后续操作，不在 Demo 中复制业务逻辑。</p>
      </div>
      <div class="scenario-grid">
        <article class="scenario-card scenario-card--primary">
          <div class="card-icon"><Download /></div>
          <h3>批量入库</h3>
          <p>批量创建入库草稿，再通过真实业务页面完成收货、残品登记与上架。</p>
          <div class="scenario-actions">
            <el-button text type="primary" @click="runScenario('inbound')">自动演示</el-button>
            <el-button text @click="startManualGuide('inbound')">亲自体验入库</el-button>
          </div>
        </article>
        <article class="scenario-card">
          <div class="card-icon"><Upload /></div>
          <h3>上游出库</h3>
          <p>模拟上游订单批量创建出库单，连续体验审核、FIFO 分配与拣货。</p>
          <div class="scenario-actions">
            <el-button text type="primary" @click="runScenario('outbound')">自动演示</el-button>
            <el-button text @click="startManualGuide('outbound')">亲自体验出库</el-button>
          </div>
        </article>
        <article class="scenario-card">
          <div class="card-icon"><Tickets /></div>
          <h3>库存盘点</h3>
          <p>批量生成盘点草稿，录入实盘数量并完成盘盈盘亏审核。</p>
          <div class="scenario-actions">
            <el-button text type="primary" @click="runScenario('stocktake')">自动演示</el-button>
            <el-button text @click="startManualGuide('stocktake')">亲自体验盘点</el-button>
          </div>
        </article>
      </div>
    </section>

    <section class="home-section" data-tour="demo-verification">
      <div class="section-heading">
        <div>
          <span class="section-kicker">工程验证</span>
          <h2>观察并发与运行状态</h2>
        </div>
        <p>实验会调用真实业务接口并保留失败结果，不使用本地定时器伪造执行过程。</p>
      </div>
      <div class="verify-grid">
        <article class="verify-card">
          <div class="card-icon"><Box /></div>
          <div>
            <h3>并发库存分配一致性</h3>
            <p>库存充足时并发审核出库单，核对本次订单的 FIFO 分配、PICK 任务和库存不变量。</p>
          </div>
          <el-button @click="goPerformance">打开实验</el-button>
        </article>
        <article class="verify-card">
          <div class="card-icon"><Connection /></div>
          <div>
            <h3>供给不足并发验证</h3>
            <p>总需求大于可用库存时并发执行真实出库审核，区分库存不足拒绝和其他失败。</p>
          </div>
          <el-button @click="goPerformance">打开实验</el-button>
        </article>
        <article class="verify-card">
          <div class="card-icon"><Document /></div>
          <div>
            <h3>模拟 PDA 并发拣货</h3>
            <p>并发扫码后明确展示仍未完成任务，再分开展示顺序收尾和最终业务状态。</p>
          </div>
          <el-button @click="goPerformance">打开实验</el-button>
        </article>
        <article class="verify-card">
          <div class="card-icon"><Monitor /></div>
          <div>
            <h3>运行状态</h3>
            <p>查看数据库、Redis、连接池、Go 运行时和当前业务指标快照。</p>
          </div>
          <el-button @click="goPerformance">查看状态</el-button>
        </article>
      </div>
    </section>

    <section class="home-section" data-tour="demo-evidence">
      <div class="section-heading">
        <div>
          <span class="section-kicker">结果与证据</span>
          <h2>最近一次业务执行结果</h2>
        </div>
        <div class="section-actions">
          <el-button :icon="Refresh" :loading="activityLoading" @click="loadActivity">刷新记录</el-button>
          <el-button :icon="TrendCharts" @click="goActivity">查看完整证据</el-button>
        </div>
      </div>
      <div class="evidence-panel">
        <template v-if="latestEvidence">
          <div class="latest-evidence">
            <el-tag effect="plain">{{ latestEvidence.kind }}</el-tag>
            <div>
              <b>{{ latestEvidence.title }}</b>
              <small>{{ formatTime(latestEvidence.createdAt) }}</small>
            </div>
            <strong>{{ latestEvidence.detail }}</strong>
            <el-button text type="primary" @click="router.push(latestEvidence.path)">查看记录</el-button>
          </div>
          <div class="evidence-counts">
            <div v-for="item in evidenceCounts" :key="item.label">
              <span>{{ item.label }}</span>
              <b>{{ item.value }}</b>
              <small>最近记录</small>
            </div>
          </div>
        </template>
        <div v-else class="empty-evidence">
          <b>还没有业务执行记录</b>
          <span>从“开始完整演示”进入，完成后的单据、任务和库存流水会显示在这里。</span>
        </div>
      </div>
    </section>

    <section class="home-section" data-tour="demo-project">
      <div class="section-heading">
        <div>
          <span class="section-kicker">项目说明</span>
          <h2>需要继续深入了解时</h2>
        </div>
        <p>业务视角保持简洁，技术细节通过独立入口展开，不拆成两套系统。</p>
      </div>
      <div class="project-grid">
        <button type="button" @click="openOverview">
          <b>项目概览</b>
          <span>业务流程、数据模型与工程能力总览</span>
          <ArrowRight />
        </button>
        <button type="button" @click="openOverview">
          <b>架构与核心设计</b>
          <span>分层结构、事务边界与库存一致性说明</span>
          <ArrowRight />
        </button>
        <button type="button" @click="openOverview">
          <b>技术实现</b>
          <span>多租户、异步任务、可观测性与可靠性设计</span>
          <ArrowRight />
        </button>
        <button type="button" @click="openSource">
          <b>源码入口</b>
          <span>前往 GitHub 查看完整实现与测试</span>
          <ArrowRight />
        </button>
      </div>
    </section>

    <DemoTour ref="demoTour" @complete="runScenario('full')" />
  </div>
</template>

<style scoped>
.demo-home {
  width: min(1180px, 100%);
  margin: 0 auto;
  padding-bottom: 24px;
}

.session-bar,
.hero-panel,
.scenario-card,
.verify-card,
.evidence-panel,
.project-grid button {
  border: 1px solid var(--el-border-color-light);
  background: var(--el-bg-color);
  box-shadow: var(--el-box-shadow-light);
}

.session-bar {
  min-height: 52px;
  padding: 9px 14px;
  border-radius: var(--gowms-radius-card);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.session-status,
.session-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.status-item {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.status-item b {
  color: var(--el-text-color-primary);
  font-family: var(--gowms-num-font);
  font-variant-numeric: tabular-nums;
}

.status-divider {
  width: 1px;
  height: 18px;
  background: var(--el-border-color);
}

.status-isolation i {
  display: inline-block;
  width: 7px;
  height: 7px;
  margin-right: 6px;
  border-radius: 50%;
  background: var(--el-color-success);
}

.hero-panel {
  margin-top: 14px;
  padding: 30px;
  border-radius: var(--gowms-radius-card);
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(360px, 0.95fr);
  gap: 34px;
  overflow: hidden;
  position: relative;
}

.hero-panel::after {
  content: '';
  position: absolute;
  width: 220px;
  height: 220px;
  right: -120px;
  top: -120px;
  border-radius: 50%;
  background: var(--el-color-primary-light-9);
  pointer-events: none;
}

.hero-copy,
.flow-panel {
  position: relative;
  z-index: 1;
}

.eyebrow,
.section-kicker {
  color: var(--el-color-primary);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.hero-copy h1 {
  max-width: 680px;
  margin: 10px 0 14px;
  color: var(--el-text-color-primary);
  font-size: clamp(28px, 3.4vw, 42px);
  line-height: 1.25;
  letter-spacing: -0.02em;
}

.hero-copy > p {
  max-width: 720px;
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 15px;
  line-height: 1.9;
}

.hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 24px;
}

.flow-panel {
  padding: 18px;
  border: 1px solid var(--el-border-color);
  border-radius: 10px;
  background: var(--el-fill-color-extra-light);
}

.flow-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.flow-head b {
  font-size: 15px;
}

.flow-head small,
.flow-note {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.flow-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin: 14px 0;
  padding: 0;
  list-style: none;
}

.flow-list li {
  display: grid;
  grid-template-columns: 28px 1fr;
  grid-template-rows: auto auto;
  column-gap: 8px;
  padding: 8px;
  border-radius: 8px;
  background: var(--el-bg-color);
}

.flow-list span {
  grid-row: 1 / 3;
  align-self: center;
  color: var(--el-color-primary);
  font-family: var(--gowms-num-font);
  font-size: 12px;
  font-weight: 700;
}

.flow-list b {
  font-size: 13px;
}

.flow-list small {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.flow-note {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  line-height: 1.6;
}

.flow-note svg {
  width: 15px;
  flex: 0 0 auto;
  margin-top: 2px;
  color: var(--el-color-primary);
}

.home-section {
  margin-top: 28px;
}

.section-heading {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 13px;
}

.section-heading h2 {
  margin: 4px 0 0;
  color: var(--el-text-color-primary);
  font-size: 20px;
}

.section-heading > p {
  max-width: 560px;
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
  text-align: right;
}

.section-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.scenario-grid,
.verify-grid,
.project-grid {
  display: grid;
  gap: 12px;
}

.scenario-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.scenario-card,
.verify-card {
  border-radius: var(--gowms-radius-card);
  transition: border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease;
}

.scenario-card {
  padding: 20px;
  border-top: 3px solid var(--el-border-color);
}

.scenario-card:hover,
.verify-card:hover {
  border-color: var(--el-color-primary-light-5);
  box-shadow: var(--el-box-shadow);
  transform: translateY(-2px);
}

.scenario-card--primary {
  border-top-color: var(--el-color-primary);
}

.card-icon {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.card-icon svg {
  width: 19px;
}

.scenario-card h3,
.verify-card h3 {
  margin: 14px 0 7px;
  color: var(--el-text-color-primary);
  font-size: 16px;
}

.scenario-card p,
.verify-card p {
  min-height: 42px;
  margin: 0 0 12px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.scenario-card .el-button {
  padding-left: 0;
}

.scenario-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 14px;
}

.verify-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.verify-card {
  padding: 18px;
  display: grid;
  grid-template-columns: 36px 1fr;
  gap: 12px;
}

.verify-card h3 {
  margin-top: 0;
}

.verify-card .el-button {
  grid-column: 2;
  justify-self: start;
}

.evidence-panel {
  padding: 18px;
  border-radius: var(--gowms-radius-card);
}

.latest-evidence {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 12px;
  padding: 14px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 9px;
  background: var(--el-fill-color-extra-light);
}

.latest-evidence div {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.latest-evidence b,
.latest-evidence strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.latest-evidence small {
  color: var(--el-text-color-secondary);
}

.latest-evidence strong {
  color: var(--el-text-color-regular);
  font-size: 13px;
  font-weight: 500;
}

.evidence-counts {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
  margin-top: 12px;
}

.evidence-counts div {
  padding: 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  text-align: center;
}

.evidence-counts span,
.evidence-counts small {
  display: block;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.evidence-counts b {
  display: block;
  margin: 4px 0 2px;
  color: var(--el-text-color-primary);
  font-family: var(--gowms-num-font);
  font-size: 22px;
}

.empty-evidence {
  min-height: 124px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 7px;
  border: 1px dashed var(--el-border-color);
  border-radius: 9px;
  color: var(--el-text-color-secondary);
}

.empty-evidence b {
  color: var(--el-text-color-primary);
}

.empty-evidence span {
  font-size: 13px;
}

.project-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.project-grid button {
  min-height: 116px;
  padding: 16px;
  border-radius: var(--gowms-radius-card);
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 8px 10px;
  text-align: left;
  cursor: pointer;
  color: var(--el-text-color-primary);
}

.project-grid button:hover {
  border-color: var(--el-color-primary-light-5);
}

.project-grid button b {
  font-size: 15px;
}

.project-grid button span {
  grid-column: 1 / 3;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.65;
}

.project-grid button svg {
  width: 16px;
  color: var(--el-color-primary);
}

@media (max-width: 980px) {
  .hero-panel {
    grid-template-columns: 1fr;
  }

  .scenario-grid,
  .verify-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .project-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 700px) {
  .session-bar,
  .section-heading {
    align-items: flex-start;
    flex-direction: column;
  }

  .section-heading > p {
    text-align: left;
  }

  .hero-panel {
    padding: 22px;
  }

  .scenario-grid,
  .verify-grid,
  .project-grid,
  .evidence-counts {
    grid-template-columns: 1fr;
  }

  .latest-evidence {
    grid-template-columns: 1fr;
  }

  .latest-evidence strong {
    white-space: normal;
  }
}
</style>
