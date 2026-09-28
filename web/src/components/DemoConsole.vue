<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Document, HomeFilled, Refresh } from '@element-plus/icons-vue'
import { resetDemoData } from '@/api/demo'
import DemoRunViewer from '@/components/demo/DemoRunViewer.vue'
import StagedDemoRunner from '@/components/demo/StagedDemoRunner.vue'
import { useDemoRunner } from '@/composables/demo/useDemoRunner'
import { useDemoSession } from '@/composables/demo/useDemoSession'
import { useAuthStore } from '@/stores/auth'
import { GUIDE_SCENARIO_LABELS, useGuideStore } from '@/stores/guide'
import {
  emitDataChanged,
  OPEN_DEMO_CONSOLE_EVENT,
  RUN_DEMO_SCENARIO_EVENT,
  RUN_DEMO_STAGED_EVENT,
  type DemoConsoleScenario,
} from '@/utils/events'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const guide = useGuideStore()
const ARCHITECTURE_URL = '/overview.html'
const visible = ref(false)
const resetting = ref(false)
const stagedDialogVisible = ref(false)
const stagedRunnerRef = ref<InstanceType<typeof StagedDemoRunner>>()
const stagedStarted = ref(false)
const stagedActive = ref(false)
const stagedCompleted = ref(false)
const stagedProgress = ref(0)
const stagedRunning = ref(false)
const stagedRunnerKey = ref(0)

// 演示会话（领取 / 心跳续期 / 空闲释放）与场景执行结果分别由 composable 维护，
// 这里只消费它们的状态并负责展示与交互。
const { acquiring, exiting, remaining, idleTimeoutMinutes, initialize, release } = useDemoSession({
  onSessionClosed: () => {
    visible.value = false
    resultDialogVisible.value = false
    stagedDialogVisible.value = false
  },
})
const { scenarioRunning, runningScenario, result, resultDialogVisible, runScenario, clearResult } = useDemoRunner({
  onBeforeRun: () => {
    if (guide.active || guide.completed) guide.cancel()
  },
  onSettled: () => emitDataChanged(),
})

const busy = computed(
  () => acquiring.value || scenarioRunning.value || resetting.value || exiting.value,
)
const resetUnavailable = computed(() => busy.value || stagedRunning.value)
const guideActive = computed(() => guide.active && Boolean(guide.scenario))
const guideCompleted = computed(() => !guide.active && guide.completed && Boolean(guide.scenario))
const guideScenarioLabel = computed(() => guide.scenario ? GUIDE_SCENARIO_LABELS[guide.scenario] : '业务')
const guideActionText = computed(() => guide.completed ? '查看业务证据' : '回到当前引导')
const sessionStatusText = computed(() => {
  if (resetting.value) return '正在重新开始'
  if (scenarioRunning.value) return '正在执行真实业务'
  if (guideActive.value) return `引导演示进行中 · ${guideScenarioLabel.value}`
  if (guideCompleted.value) return '引导演示已完成'
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
  if (guideActive.value || guideCompleted.value) return `引导演示 · ${guideScenarioLabel.value}`
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
  if (guideActive.value) {
    return `第 ${guide.currentStepNumber} / ${guide.totalSteps} 步 · ${guide.currentStepDefinition?.title || '引导演示'}`
  }
  if (guideCompleted.value) return '引导流程已完成'
  if (stagedRunning.value) return '正在调用真实业务接口'
  if (stagedCompleted.value) return '全部业务步骤已完成'
  if (stagedActive.value) return `分步执行中 · ${stagedProgress.value}%`
  if (result.value?.steps.length) return `已完成 ${result.value.steps.length} / ${result.value.steps.length} 步`
  return '尚未开始'
})

function onOpenDemoConsole() {
  visible.value = true
}

function onOpenStagedDemo(event: globalThis.Event) {
  const detail = (event as globalThis.CustomEvent<{ mode?: 'auto' | 'step'; scope?: 'full' | 'inbound' | 'outbound' }>).detail
  if (guide.active || guide.completed) guide.cancel()
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

watch(
  () => guide.active,
  (active) => {
    if (active) clearStagedRun()
  },
)

function openDemoControl() {
  visible.value = true
}

function openArchitecture(): void {
  window.open(ARCHITECTURE_URL, '_blank', 'noopener,noreferrer')
}

function openCurrentGuideStep(): void {
  if (guide.currentStepRoute) void navigateTo(guide.currentStepRoute)
}

function clearStagedRun(): void {
  stagedDialogVisible.value = false
  stagedStarted.value = false
  stagedActive.value = false
  stagedCompleted.value = false
  stagedProgress.value = 0
  stagedRunning.value = false
  stagedRunnerKey.value += 1
}

function startScenario(scenario: DemoConsoleScenario) {
  if (busy.value) return
  void runScenario(scenario)
}

function onRunDemoScenario(event: unknown) {
  const scenario = (event as { detail?: { scenario?: DemoConsoleScenario } }).detail?.scenario || 'full'
  if (route.path !== '/demo') visible.value = true
  startScenario(scenario)
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
    clearResult()
    guide.cancel()
    clearStagedRun()
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
  await release()
}

onMounted(() => {
  window.addEventListener(OPEN_DEMO_CONSOLE_EVENT, onOpenDemoConsole)
  window.addEventListener(RUN_DEMO_SCENARIO_EVENT, onRunDemoScenario)
  window.addEventListener(RUN_DEMO_STAGED_EVENT, onOpenStagedDemo)
  void initialize()
})

onBeforeUnmount(() => {
  window.removeEventListener(OPEN_DEMO_CONSOLE_EVENT, onOpenDemoConsole)
  window.removeEventListener(RUN_DEMO_SCENARIO_EVENT, onRunDemoScenario)
  window.removeEventListener(RUN_DEMO_STAGED_EVENT, onOpenStagedDemo)
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
            v-if="guideActive"
            type="primary"
            @click="openCurrentGuideStep"
          >
            {{ guideActionText }}
          </el-button>
          <el-button
            v-else-if="guideCompleted"
            type="primary"
            @click="navigateTo('/demo/activity')"
          >
            {{ guideActionText }}
          </el-button>
          <el-button
            v-else-if="stagedStarted"
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
          <button type="button" @click="openArchitecture">
            <b>系统架构图</b>
            <span>查看项目整体架构与部署链路</span>
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
