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
  runDemoScenario,
} from '@/api/demo'
import type { DemoConcurrentResult, DemoScenarioResult } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { emitDataChanged } from '@/utils/events'

type ScenarioKey = 'inbound_drafts' | 'outbound_drafts' | 'stocktake_drafts' | 'full'

const router = useRouter()
const auth = useAuthStore()
const visible = ref(false)
const running = ref<ScenarioKey | ''>('')
const concurrentRunning = ref(false)
const acquiring = ref(false)
const remaining = ref(0)
const result = ref<DemoScenarioResult | null>(null)
const concurrentResult = ref<DemoConcurrentResult | null>(null)
const draftParams = reactive({
  inboundCount: 3,
  inboundQty: 20,
  outboundCount: 3,
  outboundQty: 5,
  stocktakeCount: 2,
  concurrentCount: 20,
  concurrentQty: 1,
})
let countdownTimer: number | undefined
let heartbeatTimer: number | undefined
let redirectingToLogin = false

const remainingText = computed(() => {
  const seconds = Math.max(0, remaining.value)
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
})
const busy = computed(() => acquiring.value || running.value !== '' || concurrentRunning.value)

function clearTimers() {
  if (countdownTimer !== undefined) window.clearInterval(countdownTimer)
  if (heartbeatTimer !== undefined) window.clearInterval(heartbeatTimer)
  countdownTimer = undefined
  heartbeatTimer = undefined
}

function startTimers() {
  clearTimers()
  if (remaining.value <= 0) remaining.value = auth.demoSessionExpiresIn || 300
  countdownTimer = window.setInterval(() => {
    remaining.value = Math.max(0, remaining.value - 1)
    if (remaining.value <= 0) void refreshSession()
  }, 1000)
  heartbeatTimer = window.setInterval(() => {
    if (remaining.value <= 30) void refreshSession()
  }, 10_000)
}

async function refreshSession() {
  if (busy.value || !auth.demoSessionId) return
  try {
    const info = await heartbeatDemoSession()
    auth.setDemoSession(info)
    remaining.value = info.expires_in
  } catch {
    if (redirectingToLogin) return
    redirectingToLogin = true
    clearTimers()
    auth.clear()
    ElMessage.warning('业务会话已失效，请重新登录')
    router.push('/login')
  }
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
      auth.clearDemoSession()
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
    emitDataChanged()
    ElMessage.success(result.value.summary)
  } finally {
    running.value = ''
  }
}

async function runConcurrent() {
  if (busy.value) return
  concurrentRunning.value = true
  concurrentResult.value = null
  result.value = null
  try {
    concurrentResult.value = await runConcurrentDemo(draftParams.concurrentCount, draftParams.concurrentQty)
    emitDataChanged()
    ElMessage.success(concurrentResult.value.summary)
  } finally {
    concurrentRunning.value = false
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

function releaseKeepalive() {
  if (!auth.isDemo || !auth.demoSessionId || !auth.token) return
  const baseURL = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')
  void window.fetch(`${baseURL}/demo/session/release`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${auth.token}`,
      'X-Demo-Session': auth.demoSessionId,
    },
    keepalive: true,
  }).catch(() => undefined)
}

onMounted(() => {
  window.addEventListener('beforeunload', releaseKeepalive)
  void initialize()
})

onBeforeUnmount(() => {
  clearTimers()
  window.removeEventListener('beforeunload', releaseKeepalive)
})
</script>

<template>
  <button v-if="auth.isDemo" class="demo-fab" type="button" @click="visible = true">
    <el-icon><VideoPlay /></el-icon>
    <span>业务流程中心</span>
    <small>{{ remainingText }}</small>
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
      <div>
        <span class="demo-label">会话剩余时间</span>
        <strong>{{ remainingText }}</strong>
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
      <el-button type="warning" :loading="concurrentRunning" :disabled="busy && !concurrentRunning" @click="runConcurrent">
        并发出库测试
      </el-button>
      <el-button :disabled="busy" @click="goPerformance">性能指标</el-button>
      <el-button :disabled="busy" @click="goActivity">操作记录</el-button>
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
        <span>并发出库测试</span>
        <el-input-number v-model="draftParams.concurrentCount" :min="1" :max="30" size="small" />
        <em>张并发</em>
        <el-input-number v-model="draftParams.concurrentQty" :min="1" :max="10" size="small" />
        <em>件/张</em>
        <small>测试库存行锁、FIFO、事务重试、防超卖</small>
      </div>
    </div>

    <el-divider content-position="left">执行结果</el-divider>
    <div v-if="concurrentResult" class="demo-result">
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

.demo-status > div {
  display: flex;
  align-items: center;
  gap: 10px;
}

.demo-label {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.demo-status strong {
  font-variant-numeric: tabular-nums;
  font-size: 20px;
}

.demo-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 16px;
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

  .demo-actions .el-button {
    width: 100%;
    margin-left: 0;
  }
}
</style>