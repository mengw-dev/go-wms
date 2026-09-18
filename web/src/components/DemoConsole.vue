<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, VideoPlay } from '@element-plus/icons-vue'
import {
  acquireDemoSession,
  heartbeatDemoSession,
  releaseDemoSession,
  resetDemoData,
  runConcurrentDemo,
  runConcurrentPicking,
  runDemoScenario,
  restockDemo,
} from '@/api/demo'
import { ApiError } from '@/api/request'
import type { DemoConcurrentResult, DemoPickingResult, DemoScenarioResult } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { emitDataChanged } from '@/utils/events'

type ScenarioKey = 'inbound_drafts' | 'outbound_drafts' | 'stocktake_drafts' | 'full'

const router = useRouter()
const auth = useAuthStore()
const visible = ref(false)
const running = ref<ScenarioKey | ''>('')
const concurrentRunning = ref(false)
const pickingRunning = ref(false)
const restockRunning = ref(false)
const acquiring = ref(false)
const remaining = ref(0)
const result = ref<DemoScenarioResult | null>(null)
const concurrentResult = ref<DemoConcurrentResult | null>(null)
const pickingResult = ref<DemoPickingResult | null>(null)
const draftParams = reactive({
  inboundCount: 3,
  inboundQty: 20,
  outboundCount: 3,
  outboundQty: 5,
  stocktakeCount: 2,
  concurrentCount: 20,
  concurrentQty: 1,
  pickingWorkers: 10,
  pickingContenders: 5,
  restockQty: 500,
})
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
  () => acquiring.value || running.value !== '' || concurrentRunning.value || pickingRunning.value || restockRunning.value,
)
const idleTimeoutMinutes = computed(() => Math.max(1, Math.round(idleTTL() / 60)))

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
 * 倒计时不再本地乐观重置为完整 TTL（那会造成本地显示与 Redis 实际剩余时间的竞态），
 * 而是等续期成功后按后端返回的真实 TTL 同步；剩余时间临近耗尽时立即触发续期。
 */
function markActivity() {
  if (!auth.isDemo || !auth.demoSessionId) return
  const now = Date.now()
  if (now - lastActivityAt < ACTIVITY_THROTTLE_MS) return
  lastActivityAt = now
  renewPending = true
  // 后端 TTL 临近耗尽时立即续期，不等 10 秒定时器，
  // 避免活跃用户在续期窗口内被 70003 踢出。
  if (remaining.value > 0 && remaining.value <= RENEW_URGENT_SECONDS) {
    void refreshSession()
  }
}

/**
 * 启动空闲倒计时与续期定时器：
 * - 倒计时每秒递减，表示"无操作"的剩余时间，归零即自动释放会话；
 * - 只有发生过用户交互（renewPending）时才向后端续期，且两次续期至少间隔
 *   20 秒，挂机期间不会续期，会话锁会随 TTL 到期自动释放。
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
    // busy（场景执行中）不跳过续期：心跳只延长 Redis TTL，与业务数据无交互，
    // 长场景跑到一半同样需要保活，否则会话可能在执行中过期导致下一个请求被踢。
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
    // 用后端返回的真实 TTL 重置本地倒计时，
    // 保证倒计时始终反映 Redis 剩余时间而非乐观估计。
    remaining.value = info.expires_in
    lastRenewAt = Date.now()
    renewPending = false
  } catch (error) {
    // 仅会话真正失效（70003）时才清理登录态并停止续期；
    // 网络抖动/超时等瞬时错误保留状态，由下一个定时器周期自然重试。
    if (error instanceof ApiError && error.code === 70003) {
      if (redirectingToLogin) return
      redirectingToLogin = true
      clearTimers()
      // 会话失效由 request.ts 拦截器统一处理跳转，这里仅清理本地状态。
      auth.clear()
    }
  } finally {
    refreshing = false
  }
}

/**
 * 空闲超时：主动释放会话（释放锁并恢复演示数据），然后回到登录页。
 * 若后端 TTL 已先到期，释放接口会返回失败，忽略即可——拦截器会统一处理登出。
 */
async function handleIdleTimeout() {
  if (redirectingToLogin) return
  redirectingToLogin = true
  clearTimers()
  try {
    await releaseDemoSession()
  } catch {
    // 后端会话可能已随 TTL 过期，忽略错误继续本地登出。
  }
  auth.clear()
  ElMessage.warning(`长时间未操作，演示会话已自动释放（数据已恢复初始状态）`)
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
    router.push('/login')
  } finally {
    acquiring.value = false
  }
}

async function initialize() {
  if (!auth.isDemo) return
  const autoOpen = sessionStorage.getItem('WMS_DEMO_AUTO_OPEN') === '1'
  sessionStorage.removeItem('WMS_DEMO_AUTO_OPEN')
  if (auth.demoSessionId) {
    try {
      const info = await heartbeatDemoSession()
      auth.setDemoSession(info)
      remaining.value = info.expires_in
      startTimers()
      visible.value = autoOpen
      return
    } catch {
      // 会话失效已由 request.ts 拦截器统一跳转登录，这里无需重复处理。
      auth.clearDemoSession()
      return
    }
  }
  await acquire()
  visible.value = autoOpen
}

async function runScenario(scenario: ScenarioKey) {
  if (busy.value) return
  running.value = scenario
  result.value = null
  const options = (() => {
    if (scenario === 'inbound_drafts') return { count: draftParams.inboundCount, qty: draftParams.inboundQty }
    if (scenario === 'outbound_drafts') return { count: draftParams.outboundCount, qty: draftParams.outboundQty }
    if (scenario === 'stocktake_drafts') return { count: draftParams.stocktakeCount }
    return {}
  })()
  try {
    result.value = await runDemoScenario(scenario, options)
    concurrentResult.value = null
    pickingResult.value = null
    emitDataChanged()
    ElMessage.success(result.value.summary)
  } finally {
    running.value = ''
  }
}

function isDemoError(error: unknown, code: number) {
  return error instanceof ApiError && error.code === code
}

async function executeConcurrentAllocation() {
  concurrentResult.value = await runConcurrentDemo(draftParams.concurrentCount, draftParams.concurrentQty)
  pickingResult.value = null
  result.value = null
  emitDataChanged()
  ElMessage.success(concurrentResult.value.summary)
}

async function executeConcurrentAllocationWithRestock() {
  try {
    await executeConcurrentAllocation()
  } catch (error) {
    if (!isDemoError(error, 70006)) throw error
    const demand = draftParams.concurrentCount * draftParams.concurrentQty
    const qty = Math.max(draftParams.restockQty, demand)
    try {
      await ElMessageBox.confirm(
        `当前可用库存不足以满足 ${demand} 件并发出库需求。是否先通过完整入库流程补充 ${qty} 件，然后自动继续审核分配？`,
        '库存不足',
        {
          type: 'warning',
          confirmButtonText: '一键补货并继续',
          cancelButtonText: '先不执行',
        },
      )
    } catch {
      return
    }
    restockRunning.value = true
    try {
      result.value = await restockDemo(qty)
      concurrentResult.value = null
      pickingResult.value = null
      emitDataChanged()
      ElMessage.success(result.value.summary)
    } finally {
      restockRunning.value = false
    }
    await executeConcurrentAllocation()
  }
}

async function runConcurrentAllocation() {
  if (busy.value) return
  concurrentRunning.value = true
  try {
    await executeConcurrentAllocationWithRestock()
  } finally {
    concurrentRunning.value = false
  }
}

async function runPicking() {
  pickingResult.value = await runConcurrentPicking(draftParams.pickingWorkers, draftParams.pickingContenders)
  concurrentResult.value = null
  result.value = null
  emitDataChanged()
  ElMessage.success(pickingResult.value.summary)
}

async function runConcurrentPickingDemo() {
  if (busy.value) return
  pickingRunning.value = true
  try {
    try {
      await runPicking()
    } catch (error) {
      if (!isDemoError(error, 70005)) throw error
      try {
        await ElMessageBox.confirm(
          '当前没有可用的拣货任务，请先执行“并发出库审核分配”生成任务，再开始 PDA 并发拣货。',
          '缺少拣货任务',
          {
            type: 'warning',
            confirmButtonText: '去执行并继续',
            cancelButtonText: '先不执行',
          },
        )
      } catch {
        return
      }
      await executeConcurrentAllocationWithRestock()
      await runPicking()
    }
  } finally {
    pickingRunning.value = false
  }
}

async function runRestock() {
  if (busy.value) return
  restockRunning.value = true
  result.value = null
  try {
    result.value = await restockDemo(draftParams.restockQty)
    concurrentResult.value = null
    pickingResult.value = null
    emitDataChanged()
    ElMessage.success(result.value.summary)
  } finally {
    restockRunning.value = false
  }
}

async function goPerformance() {
  await router.push('/demo/performance')
  visible.value = false
}

async function goActivity() {
  await router.push('/demo/activity')
  visible.value = false
}

/** 打开项目架构全景图（技术架构分层 + 业务流程 + 数据模型） */
function goArchitecture() {
  window.open('/overview.html', '_blank', 'noopener,noreferrer')
  visible.value = false
}

async function goTarget(path?: string) {
  if (!path) return
  await router.push(path)
  visible.value = false
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
  await resetDemoData()
  result.value = null
  concurrentResult.value = null
  pickingResult.value = null
  emitDataChanged()
  ElMessage.success('演示数据已恢复为初始状态')
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
  try {
    await releaseDemoSession()
  } finally {
    auth.clear()
    visible.value = false
    router.push('/login')
  }
}

/**
 * 页面关闭/刷新时尽力释放演示会话，重置演示数据并让出会话锁。
 * 登录态由 auth store 处理：关闭标签页即登出，刷新页面也会在启动时强制登出。
 */
function releaseKeepalive() {
  if (!auth.isDemo || !auth.token) return
  const token = auth.token
  const sessionId = auth.demoSessionId
  if (!sessionId) return
  const baseURL = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')
  void window
    .fetch(`${baseURL}/demo/session/release`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
        'X-Demo-Session': sessionId,
      },
      keepalive: true,
    })
    .catch(() => undefined)
}

onMounted(() => {
  // beforeunload 覆盖刷新/关闭，pagehide 覆盖 bfcache 等场景，双重保险确保登出。
  window.addEventListener('beforeunload', releaseKeepalive)
  window.addEventListener('pagehide', releaseKeepalive)
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
  window.removeEventListener('beforeunload', releaseKeepalive)
  window.removeEventListener('pagehide', releaseKeepalive)
  window.removeEventListener('mousemove', markActivity)
  window.removeEventListener('mousedown', markActivity)
  window.removeEventListener('wheel', markActivity)
  window.removeEventListener('scroll', markActivity)
  window.removeEventListener('touchstart', markActivity)
  window.removeEventListener('keydown', markActivity)
})
</script>

<template>
  <button v-if="auth.isDemo" class="demo-fab" type="button" @click="visible = true">
    <el-icon><VideoPlay /></el-icon>
    <span>业务流程中心</span>
    <small v-if="showCountdown" class="fab-countdown">{{ remainingText }}</small>
  </button>

  <el-dialog
    v-model="visible"
    class="demo-console-dialog"
    title="业务流程中心"
    width="640px"
    align-center
    :close-on-click-modal="false"
  >
    <el-alert
      type="info"
      :closable="false"
      show-icon
      title="当前是独立业务环境，同一实例同一时间只允许一个操作会话。退出后会恢复初始数据。"
      class="demo-tip"
    />

    <div class="demo-status">
      <div class="demo-status-main">
        <div>
          <template v-if="showCountdown">
            <span class="demo-label">空闲剩余时间</span>
            <strong class="is-warning">{{ remainingText }}</strong>
          </template>
          <span v-else class="demo-label">演示会话进行中</span>
        </div>
        <small class="demo-idle-hint">有操作时自动续期，挂机 {{ idleTimeoutMinutes }} 分钟后自动释放会话并恢复初始数据</small>
      </div>
      <el-tag type="success" effect="plain">数据隔离已启用</el-tag>
    </div>

    <div class="demo-actions">
      <el-button type="primary" :loading="running === 'full'" :disabled="busy && running !== 'full'" @click="runScenario('full')">
        一键完整流程
      </el-button>
      <el-button :loading="running === 'inbound_drafts'" :disabled="busy && running !== 'inbound_drafts'" @click="runScenario('inbound_drafts')">
        模拟 Excel 批量入库
      </el-button>
      <el-button :loading="running === 'outbound_drafts'" :disabled="busy && running !== 'outbound_drafts'" @click="runScenario('outbound_drafts')">
        模拟上游批量出库
      </el-button>
      <el-button :loading="running === 'stocktake_drafts'" :disabled="busy && running !== 'stocktake_drafts'" @click="runScenario('stocktake_drafts')">
        批量创建盘点单
      </el-button>
      <el-button type="success" :loading="restockRunning" :disabled="busy && !restockRunning" @click="runRestock">
        一键补货入库
      </el-button>
      <el-button type="warning" :loading="concurrentRunning" :disabled="busy && !concurrentRunning" @click="runConcurrentAllocation">
        并发出库审核分配
      </el-button>
      <el-button type="warning" plain :loading="pickingRunning" :disabled="busy && !pickingRunning" @click="runConcurrentPickingDemo">
        PDA 并发拣货
      </el-button>
      <el-button :disabled="busy" @click="goPerformance">性能指标</el-button>
      <el-button :disabled="busy" @click="goActivity">操作记录</el-button>
      <el-tooltip placement="top" :show-after="150">
        <template #content>
          项目架构全景图 — 技术分层架构图 + 业务流程图（入库/出库）+ 库存三数量模型 + 核心数据表分组图 + 安全可观测矩阵 + 技术栈清单 + 核心技术设计卡片，依据仓库实际代码生成
        </template>
        <el-button :disabled="busy" type="primary" plain @click="goArchitecture">架构全景图</el-button>
      </el-tooltip>
    </div>

    <div class="demo-params">
      <div class="param-line">
        <span>批量入库</span>
        <el-input-number v-model="draftParams.inboundCount" :min="1" :max="20" size="small" />
        <em>张</em>
        <el-input-number v-model="draftParams.inboundQty" :min="1" :max="1000" size="small" />
        <em>件/张</em>
      </div>
      <div class="param-line">
        <span>上游出库</span>
        <el-input-number v-model="draftParams.outboundCount" :min="1" :max="20" size="small" />
        <em>张</em>
        <el-input-number v-model="draftParams.outboundQty" :min="1" :max="1000" size="small" />
        <em>件/张</em>
      </div>
      <div class="param-line">
        <span>盘点草稿</span>
        <el-input-number v-model="draftParams.stocktakeCount" :min="1" :max="20" size="small" />
        <em>张</em>
      </div>
      <div class="param-line concurrent-param">
        <span>并发审核分配</span>
        <el-input-number v-model="draftParams.concurrentCount" :min="1" :max="100" size="small" />
        <em>张并发</em>
        <el-input-number v-model="draftParams.concurrentQty" :min="1" :max="10" size="small" />
        <em>件/张</em>
        <small>并发创建、提交、审核出库单，生成拣货任务，不执行拣货</small>
      </div>
      <div class="param-line">
        <span>PDA 并发拣货</span>
        <el-input-number v-model="draftParams.pickingWorkers" :min="1" :max="100" size="small" />
        <em>名拣货员</em>
        <el-input-number v-model="draftParams.pickingContenders" :min="0" :max="50" size="small" />
        <em>名抢单者</em>
        <small>使用上一步生成的拣货任务，模拟逐件扫码、多人抢单和防超拣</small>
      </div>
      <div class="param-line">
        <span>一键补货</span>
        <el-input-number v-model="draftParams.restockQty" :min="1" :max="2000" size="small" />
        <em>件</em>
        <small>完整执行入库单→提交→审核→收货→上架，不清空当前单据</small>
      </div>
    </div>

    <el-divider content-position="left">执行结果</el-divider>
    <div v-if="pickingResult" class="demo-result">
      <el-alert type="success" :closable="false" show-icon :title="pickingResult.summary" />
      <div class="concurrent-focus">
        任务 {{ pickingResult.task_count }} 个，拣货员 {{ pickingResult.workers }} 名，抢单者
        {{ pickingResult.contenders }} 名，最终拣货 {{ pickingResult.final_picked }}/{{ pickingResult.total_target }} 件
      </div>
      <el-timeline class="demo-timeline">
        <el-timeline-item
          v-for="(step, index) in pickingResult.steps"
          :key="`${step.title}-${index}`"
          :timestamp="step.title"
          color="var(--el-color-warning)"
        >
          {{ step.detail }}
        </el-timeline-item>
      </el-timeline>
    </div>
    <div v-else-if="concurrentResult" class="demo-result">
      <el-alert type="success" :closable="false" show-icon :title="concurrentResult.summary" />
      <div class="concurrent-focus">
        测试重点：{{ concurrentResult.test_focus }}，总需求 {{ concurrentResult.total_demand }} 件
      </div>
      <el-timeline class="demo-timeline">
        <el-timeline-item
          v-for="(step, index) in concurrentResult.steps"
          :key="`${step.title}-${index}`"
          :timestamp="step.title"
          color="var(--el-color-warning)"
        >
          {{ step.detail }}
        </el-timeline-item>
      </el-timeline>
    </div>
    <div v-else-if="result" class="demo-result">
      <el-alert type="success" :closable="false" show-icon :title="result.summary" />
      <div v-if="result.target_path" class="result-action">
        <el-button type="primary" plain @click="goTarget(result.target_path)">
          {{ result.target_label || '前往处理' }}
        </el-button>
      </div>
      <el-timeline class="demo-timeline">
        <el-timeline-item
          v-for="(step, index) in result.steps"
          :key="`${step.title}-${index}`"
          :timestamp="step.title"
          color="var(--el-color-primary)"
        >
          {{ step.detail }}
        </el-timeline-item>
      </el-timeline>
    </div>
    <el-empty v-else description="点击上方按钮开始模拟业务流程" :image-size="72" />

    <template #footer>
      <el-button :icon="Refresh" :disabled="busy" @click="resetData">重置数据</el-button>
      <el-button type="danger" plain :disabled="busy" @click="releaseAndExit">退出并重置</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.demo-fab {
  position: fixed;
  right: 24px;
  bottom: 24px;
  z-index: 2000;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border: none;
  border-radius: 999px;
  color: #fff;
  background: linear-gradient(135deg, #4f46e5, #0ea5e9);
  box-shadow: 0 8px 24px rgba(79, 70, 229, 0.35);
  cursor: pointer;
  font-weight: 600;
}

.demo-fab small {
  padding-left: 8px;
  border-left: 1px solid rgba(255, 255, 255, 0.35);
  font-variant-numeric: tabular-nums;
  font-weight: 500;
}

/* 临近释放才出现的倒计时，用暖色提醒用户回来操作 */
.demo-fab .fab-countdown {
  color: #ffe08a;
  animation: fab-countdown-pulse 1.6s ease-in-out infinite;
}

@keyframes fab-countdown-pulse {
  0%,
  100% {
    opacity: 1;
  }

  50% {
    opacity: 0.55;
  }
}

.demo-tip {
  margin-bottom: 16px;
}

.demo-status {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 10px;
  background: var(--el-fill-color-lighter);
}

.demo-status-main {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}

.demo-status-main > div {
  display: flex;
  align-items: center;
  gap: 10px;
}

.demo-idle-hint {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.demo-label {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.demo-status strong {
  font-variant-numeric: tabular-nums;
  font-size: 20px;
}

/* 临近释放才出现的倒计时，用暖色提醒用户回来操作 */
.demo-status strong.is-warning {
  color: var(--el-color-warning);
}

.demo-actions {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
  margin-top: 16px;
}

/* 等宽网格下 Element Plus 的相邻按钮左间距会把按钮挤出格子，统一清零。 */
.demo-actions .el-button {
  width: 100%;
  margin-left: 0;
}

.demo-params {
  display: grid;
  gap: 8px;
  margin-top: 14px;
  padding: 12px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 10px;
  background: var(--el-fill-color-lighter);
}

.param-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.param-line > span:first-child {
  width: 88px;
  color: var(--el-text-color-primary);
}

.param-line em {
  font-style: normal;
  color: var(--el-text-color-secondary);
}

.param-line small {
  margin-left: 4px;
  color: var(--el-text-color-secondary);
}

.concurrent-param {
  padding-top: 8px;
  border-top: 1px dashed var(--el-border-color-light);
}

.concurrent-focus {
  margin-top: 10px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.demo-result {
  margin-top: 8px;
}

.result-action {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

.demo-timeline {
  margin-top: 16px;
  padding-left: 4px;
}

@media (max-width: 768px) {
  .demo-fab {
    right: 12px;
    bottom: 12px;
  }

  .demo-actions {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
