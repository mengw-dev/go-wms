<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Document, HomeFilled, Refresh } from '@element-plus/icons-vue'
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
import StagedDemoRunner from '@/components/demo/StagedDemoRunner.vue'
import { useAuthStore } from '@/stores/auth'
import {
  emitDataChanged,
  OPEN_DEMO_CONSOLE_EVENT,
  RUN_DEMO_SCENARIO_EVENT,
  RUN_DEMO_STAGED_EVENT,
  type DemoConsoleScenario,
} from '@/utils/events'
import { rememberDemoEvidence } from '@/utils/demoEvidence'
import { mergeDemoScenarioResults } from '@/utils/demoScenario'

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
const stagedDialogVisible = ref(false)
const stagedRunnerRef = ref<InstanceType<typeof StagedDemoRunner>>()
const stagedStarted = ref(false)
const stagedActive = ref(false)
const stagedCompleted = ref(false)
const stagedProgress = ref(0)
const stagedRunning = ref(false)
const stagedRunnerKey = ref(0)
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
let countdownWarningShown = false

// 交互事件节流：鼠标移动等高频事件最多每秒记录一次。
const ACTIVITY_THROTTLE_MS = 1_000
// 续期检查周期与两次续期之间的最小间隔。
const RENEW_CHECK_INTERVAL_MS = 10_000
const RENEW_MIN_INTERVAL_MS = 20_000
// 剩余时间低于该阈值（秒）时，用户交互立即触发续期，不等定时器。
const RENEW_URGENT_SECONDS = 30
const busy = computed(
  () => acquiring.value || scenarioRunning.value || resetting.value || exiting.value,
)
const resetUnavailable = computed(() => busy.value || stagedRunning.value)
const idleTimeoutMinutes = computed(() => Math.max(1, Math.round(idleTTL() / 60)))
const sessionStatusText = computed(() => {
  if (resetting.value) return '正在重新开始'
  if (scenarioRunning.value) return '正在执行真实业务'
  if (stagedCompleted.value) return '分步流程已完成'
  if (stagedRunning.value) return '正在执行真实业务'
  if (stagedActive.value) return `分步执行中 · ${stagedProgress.value}%`
  return '演示环境正常'
})
const stagedActionText = computed(() => stagedCompleted.value ? '查看分步结果' : '继续分步演示')
const scenarioLabels: Record<DemoConsoleScenario, string> = {
  inbound: '入库自动演示',
  outbound: '出库自动演示',
  stocktake: '盘点自动演示',
  full: '完整业务闭环',
}

const currentScenarioLabel = computed(() => {
  if (runningScenario.value) return scenarioLabels[runningScenario.value]
  if (stagedStarted.value) return '分步执行演示'
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
  if (stagedRunning.value) return '正在调用真实业务接口'
  if (stagedCompleted.value) return '全部业务步骤已完成'
  if (stagedActive.value) return `分步执行中 · ${stagedProgress.value}%`
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
    if (remaining.value === 60 && !countdownWarningShown) {
      countdownWarningShown = true
      ElMessage.warning('演示会话将在 1 分钟内空闲释放，可继续操作或点击“重新开始”重置数据')
    }
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
    countdownWarningShown = false
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
    countdownWarningShown = false
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

function onOpenStagedDemo(event: globalThis.Event) {
  const detail = (event as globalThis.CustomEvent<{ mode?: 'auto' | 'step'; scope?: 'full' | 'inbound' | 'outbound' }>).detail
  visible.value = false
  stagedDialogVisible.value = true
  void nextTick(() => stagedRunnerRef.value?.start(detail?.mode || 'step', detail?.scope || 'full'))
}

function onStagedState(state: { started: boolean; active: boolean; completed: boolean; progress: number; running: boolean }) {
  stagedStarted.value = state.started
  stagedActive.value = state.active
  stagedCompleted.value = state.completed
  stagedProgress.value = state.progress
  stagedRunning.value = state.running
}

function openDemoControl() {
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

async function runFullScenario(): Promise<DemoScenarioResult> {
  const inbound = await requestDemoScenario('inbound')
  try {
    const outbound = await requestDemoScenario('outbound')
    return mergeDemoScenarioResults([inbound, outbound])
  } catch (error) {
    if (error instanceof ApiError && isDemoScenarioResult(error.data)) {
      return mergeDemoScenarioResults([inbound, error.data])
    }
    throw error
  }
}

async function runScenario(scenario: DemoConsoleScenario) {
  if (busy.value) return
  scenarioRunning.value = true
  runningScenario.value = scenario
  result.value = null
  resultDialogVisible.value = false
  const startedAt = new Date().toISOString()
  try {
    const demoResult = scenario === 'full' ? await runFullScenario() : await requestDemoScenario(scenario)
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
  if (route.path !== '/demo') visible.value = true
  void runScenario(scenario)
}

function isDemoScenarioResult(value: unknown): value is DemoScenarioResult {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<DemoScenarioResult>
  return typeof candidate.summary === 'string' && Array.isArray(candidate.steps)
}

async function navigateTo(path: string) {
  resultDialogVisible.value = false
  stagedDialogVisible.value = false
  visible.value = false
  await router.push(path)
}

async function resetData() {
  try {
    await ElMessageBox.confirm(
      '将重新开始当前演示并恢复仓库、货品、库存和单据，确定继续吗？',
      '重置演示数据',
      {
        type: 'warning',
        confirmButtonText: '确定重置',
        cancelButtonText: '取消',
        closeOnClickModal: true,
      },
    )
  } catch {
    return
  }
  resetting.value = true
  try {
    await resetDemoData()
    result.value = null
    resultDialogVisible.value = false
    stagedDialogVisible.value = false
    stagedStarted.value = false
    stagedActive.value = false
    stagedCompleted.value = false
    stagedProgress.value = 0
    stagedRunning.value = false
    stagedRunnerKey.value += 1
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
      {
        type: 'warning',
        confirmButtonText: '退出并重置',
        cancelButtonText: '继续演示',
        closeOnClickModal: true,
      },
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
    stagedDialogVisible.value = false
    visible.value = false
    exiting.value = false
    await router.push('/login')
  }
}

onMounted(() => {
  window.addEventListener(OPEN_DEMO_CONSOLE_EVENT, onOpenDemoConsole)
  window.addEventListener(RUN_DEMO_SCENARIO_EVENT, onRunDemoScenario)
  window.addEventListener(RUN_DEMO_STAGED_EVENT, onOpenStagedDemo)
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
  window.removeEventListener(RUN_DEMO_STAGED_EVENT, onOpenStagedDemo)
  window.removeEventListener('mousemove', markActivity)
  window.removeEventListener('mousedown', markActivity)
  window.removeEventListener('wheel', markActivity)
  window.removeEventListener('scroll', markActivity)
  window.removeEventListener('touchstart', markActivity)
  window.removeEventListener('keydown', markActivity)
})
</script>

<template>
  <div
    v-if="auth.isDemo && auth.demoSessionId && !visible"
    class="demo-statusbar"
    :class="{ urgent: remaining <= 60 }"
    aria-label="演示环境状态"
  >
    <i aria-hidden="true"></i>
    <span>{{ sessionStatusText }} · 数据隔离已启用</span>
    <button
      type="button"
      class="reset-action"
      :disabled="resetUnavailable"
      @click="resetData"
    >
      一键重置数据
    </button>
    <button type="button" @click="openDemoControl">演示控制</button>
  </div>

  <el-drawer
    v-model="visible"
    class="demo-console-drawer"
    title="演示控制"
    size="min(400px, 92vw)"
    direction="rtl"
    append-to-body
    :close-on-click-modal="true"
  >
    <div class="controller-body">
      <section class="controller-state" aria-label="当前 Demo 会话">
        <div class="state-title">
          <i aria-hidden="true"></i>
          <div>
            <b>{{ sessionStatusText }}</b>
            <span>独立演示数据 · 页面切换不会重置</span>
          </div>
        </div>
        <div class="state-current">
          <span>当前体验</span>
          <b>{{ currentScenarioLabel }}</b>
          <small>{{ currentStepText }}</small>
        </div>
        <p class="idle-hint">挂机 {{ idleTimeoutMinutes }} 分钟后自动退出演示。</p>
      </section>

      <section class="controller-section">
        <div class="controller-actions">
          <el-button
            v-if="stagedStarted"
            type="primary"
            @click="stagedDialogVisible = true"
          >
            {{ stagedActionText }}
          </el-button>
          <el-button
            v-else-if="result"
            type="primary"
            :icon="Document"
            @click="resultDialogVisible = true"
          >
            查看最近结果
          </el-button>
          <el-button v-else type="primary" :icon="HomeFilled" @click="navigateTo('/demo')">
            返回演示中心
          </el-button>
        </div>
      </section>

      <section class="controller-section">
        <h4>快速查看</h4>
        <div class="quick-links">
          <button type="button" @click="navigateTo('/demo/activity')">
            <b>业务证据</b>
            <span>本次真实单据、流水与结果</span>
          </button>
          <button type="button" @click="navigateTo('/demo/activity?tab=operations')">
            <b>业务操作记录</b>
            <span>演示期间调用的真实业务接口</span>
          </button>
          <button type="button" @click="navigateTo('/demo/performance')">
            <b>工程验证</b>
            <span>并发、拣货与异步可靠性实验</span>
          </button>
        </div>
      </section>

      <section class="controller-footer">
        <el-button
          :icon="Refresh"
          :loading="resetting"
          :disabled="resetUnavailable && !resetting"
          @click="resetData"
        >
          一键重置数据
        </el-button>
        <el-button
          type="danger"
          link
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
    v-model="stagedDialogVisible"
    class="demo-staged-dialog"
    title="分步执行演示"
    width="min(1120px, 96vw)"
    top="4vh"
    append-to-body
    :close-on-click-modal="true"
    aria-label="分步执行演示"
  >
    <StagedDemoRunner
      :key="stagedRunnerKey"
      ref="stagedRunnerRef"
      @navigate="navigateTo"
      @state-change="onStagedState"
    />
  </el-dialog>

  <el-dialog
    v-model="resultDialogVisible"
    class="demo-result-dialog"
    width="min(1120px, 96vw)"
    top="4vh"
    append-to-body
    destroy-on-close
    :close-on-click-modal="true"
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
  z-index: 1700;
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
  flex-wrap: wrap;
  justify-content: flex-end;
  max-width: min(560px, calc(100vw - 24px));
}

.demo-statusbar > i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--el-color-success);
}

.demo-statusbar.urgent > i {
  background: var(--el-color-warning);
}

.demo-statusbar.urgent b {
  color: var(--el-color-warning);
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

.demo-statusbar .reset-action {
  color: var(--el-color-warning-dark-2, var(--el-color-warning));
  background: var(--el-color-warning-light-9);
}

.demo-statusbar button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.controller-body {
  display: grid;
  gap: 16px;
}

.controller-state {
  padding: 14px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
  background: var(--el-fill-color-extra-light);
}

.state-title {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.state-title > i {
  width: 9px;
  height: 9px;
  margin-top: 5px;
  border-radius: 50%;
  flex: 0 0 auto;
  background: var(--el-color-success);
  box-shadow: 0 0 0 4px var(--el-color-success-light-9);
}

.state-title b,
.state-title span,
.state-current b,
.state-current small {
  display: block;
}

.state-title b {
  font-size: 15px;
}

.state-title span {
  margin-top: 4px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.state-current {
  margin-top: 13px;
  padding: 11px 12px;
  border-radius: 9px;
  background: var(--el-bg-color);
}

.state-current span {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.state-current b {
  margin-top: 4px;
  font-size: 15px;
}

.state-current small {
  margin-top: 3px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.idle-hint {
  margin: 10px 0 0;
  color: var(--el-text-color-placeholder);
  font-size: 11px;
}

.controller-section h4 {
  margin: 0 0 9px;
  color: var(--el-text-color-regular);
  font-size: 13px;
}

.controller-actions {
  display: flex;
}

.controller-actions .el-button {
  width: 100%;
  margin-left: 0;
}

.quick-links {
  display: grid;
  gap: 8px;
}

.quick-links button {
  width: 100%;
  padding: 10px 11px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 9px;
  display: grid;
  gap: 3px;
  color: var(--el-text-color-primary);
  background: var(--el-bg-color);
  text-align: left;
  cursor: pointer;
  transition: border-color 0.16s ease, background-color 0.16s ease;
}

.quick-links button:hover {
  border-color: var(--el-color-primary-light-5);
  background: var(--el-color-primary-light-9);
}

.quick-links b {
  font-size: 13px;
}

.quick-links span {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.controller-footer {
  padding-top: 14px;
  border-top: 1px solid var(--el-border-color-lighter);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.controller-footer .el-button {
  margin-left: 0;
}

@media (max-width: 520px) {
  .demo-statusbar {
    right: 12px;
    bottom: 12px;
  }

  .controller-footer {
    flex-direction: column;
    align-items: stretch;
  }

  .controller-footer .el-button {
    width: 100%;
  }
}
</style>
