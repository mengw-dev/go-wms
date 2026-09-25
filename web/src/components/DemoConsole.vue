<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Document, HomeFilled, Refresh, VideoPlay } from '@element-plus/icons-vue'
import {
  acquireDemoSession,
  heartbeatDemoSession,
  releaseDemoSession,
  resetDemoData,
  runDemoScenario as requestDemoScenario,
} from '@/api/demo'
import { ApiError } from '@/api/request'
import type { DemoScenarioResult } from '@/api/types'
import DemoRunViewer from '@/components/demo/DemoRunViewer.vue'
import { useAuthStore } from '@/stores/auth'
import {
  emitDataChanged,
  OPEN_DEMO_CONSOLE_EVENT,
  RUN_DEMO_SCENARIO_EVENT,
  type DemoConsoleScenario,
} from '@/utils/events'
import { rememberDemoEvidence } from '@/utils/demoEvidence'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const visible = ref(false)
const acquiring = ref(false)
const scenarioRunning = ref(false)
const resetting = ref(false)
const exiting = ref(false)
const remaining = ref(0)
const result = ref<DemoScenarioResult | null>(null)
const resultDialogVisible = ref(false)
const runningScenario = ref<DemoConsoleScenario | null>(null)
let countdownTimer: number | undefined
let renewTimer: number | undefined
let redirectingToLogin = false
let refreshing = false
// 最近一次用户交互与最近一次向后端续期的时间戳（毫秒）。
let lastActivityAt = 0
let lastRenewAt = 0
// 有交互后置为 true，等待续期定时器把后端 TTL 同步刷新。
let renewPending = false

// 交互事件节流：鼠标移动等高频事件最多每秒记录一次。
const ACTIVITY_THROTTLE_MS = 1_000
// 续期检查周期与两次续期之间的最小间隔。
const RENEW_CHECK_INTERVAL_MS = 10_000
const RENEW_MIN_INTERVAL_MS = 20_000
// 剩余时间低于该阈值（秒）时，用户交互立即触发续期，不等定时器。
const RENEW_URGENT_SECONDS = 30
// 悬浮球与状态区的倒计时只在临近释放时出现，避免一直跳数字影响观感。
const COUNTDOWN_VISIBLE_SECONDS = 60

const remainingText = computed(() => {
  const seconds = Math.max(0, remaining.value)
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
})
const showCountdown = computed(() => remaining.value > 0 && remaining.value <= COUNTDOWN_VISIBLE_SECONDS)
const busy = computed(
  () => acquiring.value || scenarioRunning.value || resetting.value || exiting.value,
)
const idleTimeoutMinutes = computed(() => Math.max(1, Math.round(idleTTL() / 60)))
const sessionState = computed(() => {
  if (auth.demoSessionId) return '进行中'
  if (acquiring.value) return '接入中'
  return '未建立'
})
const scenarioLabels: Record<DemoConsoleScenario, string> = {
  inbound: '入库自动演示',
  outbound: '出库自动演示',
  stocktake: '盘点自动演示',
  full: '完整业务闭环',
}

const currentScenarioLabel = computed(() => {
  if (runningScenario.value) return scenarioLabels[runningScenario.value]
  const name = result.value?.name
  if (name && Object.prototype.hasOwnProperty.call(scenarioLabels, name)) {
    return scenarioLabels[name as DemoConsoleScenario]
  }
  return '等待开始'
})

const currentStepText = computed(() => {
  if (resetting.value) return '正在重置演示数据'
  if (exiting.value) return '正在退出演示'
  if (acquiring.value) return '正在接入演示会话'
  if (scenarioRunning.value) return '正在执行真实业务'
  if (result.value?.steps.length) return `已完成 ${result.value.steps.length} / ${result.value.steps.length} 步`
  return '尚未开始'
})

/** 会话空闲时长（秒），来源于后端下发的 TTL。 */
function idleTTL() {
  return auth.demoSessionExpiresIn || 300
}

function clearTimers() {
  if (countdownTimer !== undefined) window.clearInterval(countdownTimer)
  if (renewTimer !== undefined) window.clearInterval(renewTimer)
  countdownTimer = undefined
  renewTimer = undefined
}

/**
 * 记录一次用户交互并标记需要向后端续期。高频事件通过时间戳节流，避免频繁触发。
 * 倒计时不本地乐观重置为完整 TTL，续期成功后按后端返回的真实 TTL 同步。
 */
function markActivity() {
  if (!auth.isDemo || !auth.demoSessionId) return
  const now = Date.now()
  if (now - lastActivityAt < ACTIVITY_THROTTLE_MS) return
  lastActivityAt = now
  renewPending = true
  if (remaining.value > 0 && remaining.value <= RENEW_URGENT_SECONDS) {
    void refreshSession()
  }
}

/**
 * 启动空闲倒计时与续期定时器：
 * - 倒计时每秒递减，表示无操作的剩余时间，归零即自动释放会话；
 * - 只有发生过用户交互时才向后端续期，挂机期间会随 TTL 到期自动释放。
 */
function startTimers() {
  clearTimers()
  if (remaining.value <= 0) remaining.value = idleTTL()
  lastRenewAt = Date.now()
  renewPending = false
  countdownTimer = window.setInterval(() => {
    if (remaining.value <= 0) {
      void handleIdleTimeout()
      return
    }
    remaining.value -= 1
    if (remaining.value === 0) void handleIdleTimeout()
  }, 1000)
  renewTimer = window.setInterval(() => {
    // 场景执行期间仍需续期，避免长流程进行中会话先过期。
    if (!renewPending || refreshing || !auth.demoSessionId) return
    if (Date.now() - lastRenewAt < RENEW_MIN_INTERVAL_MS) return
    void refreshSession()
  }, RENEW_CHECK_INTERVAL_MS)
}

/** 向后端续期演示会话（仅在最近有用户交互时调用）。 */
async function refreshSession() {
  if (refreshing || !auth.demoSessionId) return
  refreshing = true
  try {
    const info = await heartbeatDemoSession()
    auth.setDemoSession(info)
    remaining.value = info.expires_in
    lastRenewAt = Date.now()
    renewPending = false
  } catch (error) {
    if (error instanceof ApiError && error.code === 70003) {
      if (redirectingToLogin) return
      redirectingToLogin = true
      clearTimers()
      auth.clear()
    }
  } finally {
    refreshing = false
  }
}

/**
 * 空闲超时：主动释放会话（释放锁并恢复演示数据），然后回到登录页。
 * 若后端 TTL 已先到期，释放接口会返回失败，忽略即可。
 */
async function handleIdleTimeout() {
  if (redirectingToLogin) return
  redirectingToLogin = true
  clearTimers()
  try {
    await releaseDemoSession()
  } catch {
    // 后端会话可能已随 TTL 过期，继续清理本地状态。
  }
  auth.clear()
  visible.value = false
  ElMessage.warning('长时间未操作，演示会话已自动释放（数据已恢复初始状态）')
  await router.push('/login')
}

async function acquire() {
  if (acquiring.value) return
  acquiring.value = true
  try {
    const info = await acquireDemoSession()
    auth.setDemoSession(info)
    remaining.value = info.expires_in
    startTimers()
  } catch {
    if (redirectingToLogin) return
    redirectingToLogin = true
    auth.clear()
    await router.push('/login')
  } finally {
    acquiring.value = false
  }
}

function onOpenDemoConsole() {
  visible.value = true
}

async function initialize() {
  if (!auth.isDemo) return
  if (auth.demoSessionId) {
    try {
      const info = await heartbeatDemoSession()
      auth.setDemoSession(info)
      remaining.value = info.expires_in
      startTimers()
      return
    } catch {
      auth.clearDemoSession()
      return
    }
  }
  await acquire()
}

async function runScenario(scenario: DemoConsoleScenario) {
  if (busy.value) return
  scenarioRunning.value = true
  runningScenario.value = scenario
  result.value = null
  resultDialogVisible.value = false
  const startedAt = new Date().toISOString()
  try {
    const demoResult = await requestDemoScenario(scenario)
    result.value = demoResult
    resultDialogVisible.value = true
    rememberDemoEvidence(demoResult, startedAt)
    emitDataChanged()
    if (demoResult.status !== 'failed') {
      ElMessage.success(demoResult.summary)
    }
  } catch (error) {
    if (error instanceof ApiError && isDemoScenarioResult(error.data)) {
      result.value = error.data
      resultDialogVisible.value = true
      rememberDemoEvidence(error.data, startedAt)
      emitDataChanged()
    }
  } finally {
    scenarioRunning.value = false
    runningScenario.value = null
  }
}

function onRunDemoScenario(event: unknown) {
  const scenario = (event as { detail?: { scenario?: DemoConsoleScenario } }).detail?.scenario || 'full'
  visible.value = true
  void runScenario(scenario)
}

function isDemoScenarioResult(value: unknown): value is DemoScenarioResult {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<DemoScenarioResult>
  return typeof candidate.summary === 'string' && Array.isArray(candidate.steps)
}

async function navigateTo(path: string) {
  resultDialogVisible.value = false
  visible.value = false
  await router.push(path)
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
  resetting.value = true
  try {
    await resetDemoData()
    result.value = null
    resultDialogVisible.value = false
    emitDataChanged()
    ElMessage.success('演示数据已恢复为初始状态')
  } finally {
    resetting.value = false
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
  exiting.value = true
  try {
    await releaseDemoSession()
  } catch {
    // 会话可能已过期，仍需清理本地登录态并返回登录页。
  } finally {
    auth.clear()
    resultDialogVisible.value = false
    visible.value = false
    exiting.value = false
    await router.push('/login')
  }
}

onMounted(() => {
  window.addEventListener(OPEN_DEMO_CONSOLE_EVENT, onOpenDemoConsole)
  window.addEventListener(RUN_DEMO_SCENARIO_EVENT, onRunDemoScenario)
  // 用户交互才会刷新空闲倒计时并向后端续期。
  window.addEventListener('mousemove', markActivity, { passive: true })
  window.addEventListener('mousedown', markActivity, { passive: true })
  window.addEventListener('wheel', markActivity, { passive: true })
  window.addEventListener('scroll', markActivity, { passive: true })
  window.addEventListener('touchstart', markActivity, { passive: true })
  window.addEventListener('keydown', markActivity)
  void initialize()
})

onBeforeUnmount(() => {
  clearTimers()
  window.removeEventListener(OPEN_DEMO_CONSOLE_EVENT, onOpenDemoConsole)
  window.removeEventListener(RUN_DEMO_SCENARIO_EVENT, onRunDemoScenario)
  window.removeEventListener('mousemove', markActivity)
  window.removeEventListener('mousedown', markActivity)
  window.removeEventListener('wheel', markActivity)
  window.removeEventListener('scroll', markActivity)
  window.removeEventListener('touchstart', markActivity)
  window.removeEventListener('keydown', markActivity)
})
</script>

<template>
  <button
    v-if="auth.isDemo && !(route.path === '/demo' && !scenarioRunning && !result)"
    class="demo-fab"
    type="button"
    aria-label="打开演示控制"
    @click="visible = true"
  >
    <el-icon><VideoPlay /></el-icon>
    <span>演示控制</span>
    <small v-if="showCountdown" class="fab-countdown">{{ remainingText }}</small>
  </button>

  <div
    v-if="auth.isDemo && route.path === '/demo' && !scenarioRunning && !result"
    class="demo-statusbar"
    aria-label="演示环境状态"
  >
    <i aria-hidden="true"></i>
    <span>演示环境 · 闲置剩余</span>
    <b>{{ remainingText }}</b>
    <button type="button" @click="visible = true">演示控制</button>
  </div>

  <el-drawer
    v-model="visible"
    class="demo-console-drawer"
    :title="auth.demoSessionId ? '演示进行中' : '演示控制'"
    size="min(420px, 92vw)"
    direction="rtl"
    append-to-body
    :close-on-click-modal="false"
  >
    <div class="controller-body">
      <section class="controller-state" aria-label="当前 Demo 会话">
        <div class="state-head">
          <div>
            <el-tag type="success" effect="plain">独立演示租户</el-tag>
            <span class="isolation">数据隔离已启用</span>
          </div>
          <div class="remaining">
            <span>剩余时间</span>
            <strong>{{ remainingText }}</strong>
          </div>
        </div>
        <dl class="status-list">
          <div>
            <dt>演示场景</dt>
            <dd>{{ currentScenarioLabel }}</dd>
          </div>
          <div>
            <dt>当前步骤</dt>
            <dd>{{ currentStepText }}</dd>
          </div>
          <div>
            <dt>会话状态</dt>
            <dd>{{ sessionState }}</dd>
          </div>
        </dl>
        <p class="idle-hint">
          有操作时自动续期；挂机 {{ idleTimeoutMinutes }} 分钟后自动释放并恢复演示数据。
        </p>
      </section>

      <section class="run-summary">
        <div class="section-title">
          <b>最近一次执行</b>
          <span>{{ currentScenarioLabel }}</span>
        </div>
        <p v-if="result" class="run-summary-copy">{{ result.summary }}</p>
        <p v-else-if="scenarioRunning" class="run-summary-copy">
          正在调用真实业务 Service 执行，完成后可查看完整业务结果。
        </p>
        <p v-else class="run-summary-copy">当前还没有执行结果，可从演示中心开始体验。</p>
        <div class="controller-actions">
          <el-button
            type="primary"
            :icon="Document"
            :disabled="!result"
            @click="resultDialogVisible = true"
          >
            查看结果
          </el-button>
          <el-button :icon="HomeFilled" @click="navigateTo('/demo')">返回演示中心</el-button>
        </div>
      </section>

      <section class="danger-section">
        <el-button
          :icon="Refresh"
          :loading="resetting"
          :disabled="busy && !resetting"
          @click="resetData"
        >
          重置数据
        </el-button>
        <el-button
          type="danger"
          plain
          :loading="exiting"
          :disabled="busy && !exiting"
          @click="releaseAndExit"
        >
          退出演示
        </el-button>
      </section>
    </div>
  </el-drawer>

  <el-dialog
    v-model="resultDialogVisible"
    class="demo-result-dialog"
    width="min(960px, 96vw)"
    top="3vh"
    append-to-body
    destroy-on-close
    :close-on-click-modal="false"
    aria-label="自动演示结果"
  >
    <DemoRunViewer v-if="result" :result="result" @navigate="navigateTo" />
  </el-dialog>
</template>

<style scoped>
.demo-statusbar {
  position: fixed;
  right: 24px;
  bottom: 20px;
  z-index: 2000;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 9px 11px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 9px;
  color: var(--el-text-color-secondary);
  background: var(--el-bg-color);
  box-shadow: var(--el-box-shadow-light);
  font-size: 12px;
}

.demo-statusbar > i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--el-color-success);
}

.demo-statusbar b {
  color: var(--el-text-color-primary);
  font-family: var(--gowms-num-font);
}

.demo-statusbar button {
  padding: 3px 7px;
  border: 0;
  border-radius: 6px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  cursor: pointer;
}

.demo-fab {
  position: fixed;
  right: 24px;
  bottom: 24px;
  z-index: 2000;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 11px 15px;
  border: none;
  border-radius: 999px;
  color: #fff;
  background: var(--el-color-primary);
  box-shadow: 0 8px 22px rgba(64, 158, 255, 0.28);
  cursor: pointer;
  font-weight: 600;
}

.demo-fab:hover {
  background: var(--el-color-primary-dark-2);
}

.demo-fab small {
  padding-left: 8px;
  border-left: 1px solid rgba(255, 255, 255, 0.45);
  font-variant-numeric: tabular-nums;
  font-weight: 500;
}

.demo-fab .fab-countdown {
  color: #fff3bf;
}

.controller-body {
  display: grid;
  gap: 18px;
}

.controller-state,
.run-summary {
  padding: 16px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  background: var(--el-bg-color);
}

.state-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.state-head > div:first-child {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}

.isolation {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.remaining {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 3px;
}

.remaining span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.remaining strong {
  font-family: var(--gowms-num-font);
  font-size: 22px;
  font-variant-numeric: tabular-nums;
}

.status-list {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin: 16px 0 0;
}

.status-list div {
  padding: 10px 12px;
  border-radius: 9px;
  background: var(--el-fill-color-lighter);
}

.status-list dt {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.status-list dd {
  margin: 5px 0 0;
  color: var(--el-text-color-primary);
  font-weight: 600;
}

.idle-hint {
  margin: 12px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.section-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.section-title b {
  color: var(--el-text-color-primary);
}

.section-title span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  text-align: right;
}

.run-summary-copy {
  margin: 0;
  padding: 14px;
  border-radius: 9px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-lighter);
  font-size: 13px;
  line-height: 1.7;
}

.controller-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 12px;
}

.controller-actions .el-button {
  width: 100%;
  margin-left: 0;
}

.demo-result-dialog :deep(.el-dialog__body) {
  max-height: 90vh;
  padding-top: 8px;
  overflow-y: auto;
}

.danger-section {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding-top: 2px;
}

.danger-section .el-button {
  margin-left: 0;
}

@media (max-height: 760px) and (min-width: 761px) {
  .demo-statusbar {
    bottom: 8px;
    padding: 6px 9px;
  }
}

@media (max-width: 520px) {
  .demo-statusbar {
    right: 12px;
    bottom: 12px;
  }

  .demo-fab {
    right: 12px;
    bottom: 12px;
  }

  .demo-fab span {
    display: none;
  }

  .state-head {
    flex-direction: column;
  }

  .remaining {
    align-items: flex-start;
  }

  .status-list,
  .controller-actions {
    grid-template-columns: 1fr;
  }

  .danger-section {
    align-items: stretch;
    flex-direction: column;
  }

  .danger-section .el-button {
    width: 100%;
  }
}
</style>