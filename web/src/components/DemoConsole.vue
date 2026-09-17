<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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

type ScenarioKey = 'inbound' | 'outbound' | 'stocktake' | 'full'

const router = useRouter()
const auth = useAuthStore()
const visible = ref(false)
const running = ref<ScenarioKey | ''>('')
const concurrentRunning = ref(false)
const acquiring = ref(false)
const remaining = ref(0)
const result = ref<DemoScenarioResult | null>(null)
const concurrentResult = ref<DemoConcurrentResult | null>(null)
let countdownTimer: number | undefined
let heartbeatTimer: number | undefined

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
  heartbeatTimer = window.setInterval(() => void refreshSession(), 60_000)
}

async function refreshSession() {
  if (busy.value || !auth.demoSessionId) return
  try {
    const info = await heartbeatDemoSession()
    auth.setDemoSession(info)
    remaining.value = info.expires_in
  } catch {
    clearTimers()
    auth.clear()
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
    visible.value = true
  } catch {
    auth.clear()
    router.push('/login')
  } finally {
    acquiring.value = false
  }
}

async function initialize() {
  if (!auth.isDemo) return
  if (auth.demoSessionId) {
    try {
      const info = await heartbeatDemoSession()
      auth.setDemoSession(info)
      remaining.value = info.expires_in
      startTimers()
      visible.value = true
      return
    } catch {
      auth.clearDemoSession()
    }
  }
  await acquire()
}

async function runScenario(scenario: ScenarioKey) {
  if (busy.value) return
  running.value = scenario
  result.value = null
  try {
    result.value = await runDemoScenario(scenario)
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
    concurrentResult.value = await runConcurrentDemo(20)
    emitDataChanged()
    ElMessage.success(concurrentResult.value.summary)
  } finally {
    concurrentRunning.value = false
  }
}

function goPerformance() {
  visible.value = false
  router.push('/demo/performance')
}

function goActivity() {
  visible.value = false
  router.push('/demo/activity')
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
    <span>演示控制台</span>
    <small>{{ remainingText }}</small>
  </button>

  <el-dialog
    v-model="visible"
    class="demo-console-dialog"
    title="WMS 业务流程演示"
    width="640px"
    align-center
    :close-on-click-modal="false"
  >
    <el-alert
      type="info"
      :closable="false"
      show-icon
      title="当前是独立演示环境，同一实例同一时间只允许一个演示会话。退出后会恢复初始数据。"
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
      <el-button type="primary" size="large" :loading="running === 'full'" :disabled="busy && running !== 'full'" @click="runScenario('full')">
        一键完整流程演示
      </el-button>
      <el-button :loading="running === 'inbound'" :disabled="busy && running !== 'inbound'" @click="runScenario('inbound')">
        入库演示
      </el-button>
      <el-button :loading="running === 'outbound'" :disabled="busy && running !== 'outbound'" @click="runScenario('outbound')">
        出库演示
      </el-button>
      <el-button :loading="running === 'stocktake'" :disabled="busy && running !== 'stocktake'" @click="runScenario('stocktake')">
        盘点演示
      </el-button>
      <el-button type="warning" :loading="concurrentRunning" :disabled="busy && !concurrentRunning" @click="runConcurrent">
        并发业务演示
      </el-button>
      <el-button :disabled="busy" @click="goPerformance">性能指标</el-button>
      <el-button :disabled="busy" @click="goActivity">操作记录</el-button>
    </div>

    <el-divider content-position="left">执行结果</el-divider>
    <div v-if="concurrentResult" class="demo-result">
      <el-alert type="success" :closable="false" show-icon :title="concurrentResult.summary" />
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

.demo-result {
  margin-top: 8px;
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