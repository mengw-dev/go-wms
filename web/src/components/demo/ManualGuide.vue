<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type CSSProperties } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getDemoActivity } from '@/api/demo'
import type { DemoActivitySnapshot } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { GUIDE_SCENARIO_LABELS, getGuideStep, resolveGuideRoute, useGuideStore } from '@/stores/guide'
import { formatTime } from '@/utils'
import { buildDemoOperationRows, type DemoOperationRow } from '@/utils/demoOperations'

interface TargetRect {
  top: number
  left: number
  width: number
  height: number
}

type ActionState = 'idle' | 'pending' | 'failed'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const guide = useGuideStore()

const targetRect = ref<TargetRect | null>(null)
const viewport = ref({ width: window.innerWidth, height: window.innerHeight })
const businessLayerOpen = ref(false)
const actionState = ref<ActionState>('idle')
const recordsVisible = ref(false)
const recordsLoading = ref(false)
const recordsError = ref('')
const activity = ref<DemoActivitySnapshot | null>(null)

let targetObserver: InstanceType<typeof window.ResizeObserver> | undefined
let bodyObserver: InstanceType<typeof window.MutationObserver> | undefined
let locateFrame: number | undefined
let locateAttempts = 0
let lastLocatedStepId = ''
let observedTarget: HTMLElement | null = null
let actionRevision = ''
let cancellationClicked = false

const visible = computed(() => auth.isDemo && (guide.active || guide.completed))
const contentVisible = computed(
  () => visible.value && !guide.completed && actionState.value === 'idle' && !businessLayerOpen.value,
)
const step = computed(() => guide.currentStepDefinition)
const scenarioLabel = computed(() =>
  guide.scenario ? GUIDE_SCENARIO_LABELS[guide.scenario] : '业务',
)
const completionSteps = computed(() => {
  if (guide.scenario === 'inbound') {
    return ['创建', '提交', '审核', '收货', '上架', '库存增加']
  }
  if (guide.scenario === 'outbound') {
    return ['创建', '提交', '审核分配', '查看任务', '拣货发货', '库存流水']
  }
  if (guide.scenario === 'stocktake') {
    return ['创建', '账面快照', '录入实盘', '查看差异', '审核调整', '库存流水']
  }
  return guide.scenario ? [GUIDE_SCENARIO_LABELS[guide.scenario] + '单', '业务已完成'] : []
})
const businessOperations = computed<DemoOperationRow[]>(() => {
  return buildDemoOperationRows(activity.value?.operations ?? [])
    .filter((item) =>
      ['/api/v1/inbound', '/api/v1/outbound', '/api/v1/inventory', '/api/v1/tasks'].some((prefix) =>
        item.raw.path.startsWith(prefix),
      ),
    )
    .slice(0, 20)
})

function httpStatusType(status: number): 'success' | 'warning' | 'danger' {
  if (status >= 200 && status < 300) return 'success'
  if (status >= 400 && status < 500) return 'warning'
  return 'danger'
}

const highlightStyle = computed<CSSProperties>(() => {
  const rect = targetRect.value
  if (!rect) return {}
  const padding = 5
  return {
    top: `${Math.max(0, rect.top - padding)}px`,
    left: `${Math.max(0, rect.left - padding)}px`,
    width: `${rect.width + padding * 2}px`,
    height: `${rect.height + padding * 2}px`,
  }
})

const panelStyle = computed<CSSProperties>(() => {
  const panelWidth = Math.min(320, viewport.value.width - 24)
  if (viewport.value.width <= 640) {
    return {
      left: '12px',
      right: '12px',
      bottom: '12px',
      width: 'auto',
    }
  }

  const rect = targetRect.value
  if (!rect) {
    return {
      left: '50%',
      top: '112px',
      width: `${panelWidth}px`,
      transform: 'translateX(-50%)',
    }
  }

  const estimatedHeight = 188
  const gap = 12
  const edge = 12
  const minTop = 112
  let left = Math.min(Math.max(edge, rect.left), viewport.value.width - panelWidth - edge)
  let top = rect.top + rect.height + gap

  if (top + estimatedHeight > viewport.value.height - edge) {
    const above = rect.top - estimatedHeight - gap
    if (above >= minTop) {
      top = above
    } else if (rect.left + rect.width + panelWidth + gap <= viewport.value.width - edge) {
      left = rect.left + rect.width + gap
      top = Math.min(Math.max(minTop, rect.top), viewport.value.height - estimatedHeight - edge)
    } else if (rect.left - panelWidth - gap >= edge) {
      left = rect.left - panelWidth - gap
      top = Math.min(Math.max(minTop, rect.top), viewport.value.height - estimatedHeight - edge)
    } else {
      left = Math.min(Math.max(edge, rect.left), viewport.value.width - panelWidth - edge)
      top = Math.max(minTop, viewport.value.height - estimatedHeight - edge)
    }
  }

  return {
    left: `${left}px`,
    top: `${top}px`,
    width: `${panelWidth}px`,
  }
})

const completedPanelStyle = computed<CSSProperties>(() =>
  viewport.value.width <= 640
    ? { left: '12px', right: '12px', bottom: '12px', width: 'auto' }
    : { right: '24px', bottom: '24px', width: 'min(420px, calc(100vw - 48px))' },
)

function clearTarget(): void {
  targetRect.value = null
  targetObserver?.disconnect()
  targetObserver = undefined
  observedTarget = null
}

function refreshTarget(): void {
  if (!contentVisible.value || !step.value) {
    clearTarget()
    return
  }

  const element = document.querySelector<HTMLElement>(step.value.target)
  if (!element) {
    clearTarget()
    return
  }

  if (observedTarget !== element) {
    clearTarget()
    observedTarget = element
    if (window.ResizeObserver) {
      targetObserver = new window.ResizeObserver(() => refreshTarget())
      targetObserver.observe(element)
    }
  }

  const rect = element.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) {
    targetRect.value = null
    return
  }
  targetRect.value = {
    top: rect.top,
    left: rect.left,
    width: rect.width,
    height: rect.height,
  }

  if (lastLocatedStepId !== step.value.id) {
    lastLocatedStepId = step.value.id
    element.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' })
  }
}

function scheduleTargetLocate(reset = false): void {
  if (reset) locateAttempts = 0
  if (locateFrame !== undefined) window.cancelAnimationFrame(locateFrame)

  const locate = () => {
    locateFrame = undefined
    refreshTarget()
    if (!targetRect.value && contentVisible.value && locateAttempts < 30) {
      locateAttempts += 1
      locateFrame = window.requestAnimationFrame(locate)
      return
    }
    if (!targetRect.value && contentVisible.value) {
      guide.setMismatch('未找到当前步骤对应的业务按钮。请重新定位当前步骤，或重新开始/退出引导。')
    }
  }
  locateFrame = window.requestAnimationFrame(locate)
}

function updateViewport(): void {
  viewport.value = { width: window.innerWidth, height: window.innerHeight }
  refreshTarget()
}

function guideRevision(): string {
  return [guide.currentStep, guide.lastOutcome, guide.mismatch, ...guide.verifiedStepIds].join('|')
}

function isElementVisible(element: HTMLElement): boolean {
  if (element.hidden) return false
  if (element.getAttribute('aria-hidden') === 'true') return false
  const style = window.getComputedStyle(element)
  if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0) {
    return false
  }
  const rect = element.getBoundingClientRect()
  return rect.width > 0 && rect.height > 0
}

function hasBusinessLayer(): boolean {
  return Array.from(document.querySelectorAll<HTMLElement>('.el-overlay')).some(isElementVisible)
}

function hasBusinessError(): boolean {
  return Array.from(
    document.querySelectorAll<HTMLElement>('.el-message--error, .el-notification--error'),
  ).some(isElementVisible)
}

function updateBusinessLayerState(): void {
  const layerOpen = hasBusinessLayer()
  const errorOpen = hasBusinessError()
  businessLayerOpen.value = layerOpen

  if (actionState.value === 'pending' && errorOpen) {
    actionState.value = 'failed'
    cancellationClicked = false
    return
  }

  if (actionState.value === 'failed' && !errorOpen && !layerOpen) {
    actionState.value = 'idle'
    cancellationClicked = false
    if (guide.active && !guide.completed) {
      guide.setMismatch('当前操作未成功，请重新定位当前步骤或重新开始引导。')
    }
    return
  }

  if (actionState.value === 'pending' && cancellationClicked && !layerOpen && !errorOpen) {
    actionState.value = 'idle'
    cancellationClicked = false
  }
}

function beginTargetAction(): void {
  actionRevision = guideRevision()
  cancellationClicked = false
  actionState.value = 'pending'
  clearTarget()
}

function syncActionFromGuide(): void {
  if (actionState.value === 'pending' && guideRevision() !== actionRevision) {
    actionState.value = 'idle'
    cancellationClicked = false
  }
}

function onDocumentClickCapture(event: globalThis.Event): void {
  const target = event.target as HTMLElement | null
  if (!target?.closest) return

  const clickedButton = target.closest('button')
  const clickedBusinessContainer = clickedButton?.closest('.el-dialog, .el-message-box')
  if (clickedButton && clickedBusinessContainer) {
    const buttonText = clickedButton.textContent?.replace(/\s+/g, '') || ''
    if (['取消', '返回', '关闭', '继续演示'].some((label) => buttonText.includes(label))) {
      cancellationClicked = true
    }
  }

  if (!contentVisible.value || !step.value) return
  const actionable = target.closest('button, a, [role="button"], .el-button')
  if (!actionable || !actionable.closest(step.value.target)) return
  beginTargetAction()
}

function onKeydownCapture(event: KeyboardEvent): void {
  if (event.key === 'Escape' && hasBusinessLayer()) cancellationClicked = true
}

function resolvedCurrentStepRoute(): string {
  if (!guide.scenario) return ''
  const current = getGuideStep(guide.scenario, guide.currentStep)
  if (!current) return ''
  return resolveGuideRoute(current.route, guide.orderId, guide.orderNo)
}

async function restartGuide(): Promise<void> {
  actionState.value = 'idle'
  cancellationClicked = false
  const firstStep = guide.restart()
  if (!firstStep) return
  await router.push(firstStep.route)
}

function completedOrderPath(): string {
  if (guide.scenario === 'outbound') return `/outbound/orders/${guide.orderId}`
  if (guide.scenario === 'stocktake') return `/stocktake/orders/${guide.orderId}`
  return `/inbound/orders/${guide.orderId}`
}

async function completeNavigate(path: string): Promise<void> {
  actionState.value = 'idle'
  cancellationClicked = false
  guide.cancel()
  await router.push(path)
}

function manualEvidencePath(): string {
  const params = new window.URLSearchParams()
  if (guide.scenario) params.set('scenario', guide.scenario)
  params.set('source', 'manual')
  if (guide.orderId) params.set('order_id', guide.orderId)
  if (guide.orderNo) params.set('order_no', guide.orderNo)
  if (guide.taskId) params.set('task_id', guide.taskId)
  if (guide.taskNo) params.set('task_no', guide.taskNo)
  if (guide.startedAt) params.set('started_at', new Date(guide.startedAt).toISOString())
  params.set('completed_at', new Date().toISOString())
  return `/demo/activity?${params.toString()}`
}

async function openRecords(): Promise<void> {
  recordsVisible.value = true
  recordsLoading.value = true
  recordsError.value = ''
  try {
    activity.value = await getDemoActivity(30)
  } catch {
    recordsError.value = '操作记录暂时无法加载，请稍后重试。'
  } finally {
    recordsLoading.value = false
  }
}

function repositionGuide(): void {
  actionState.value = 'idle'
  cancellationClicked = false
  if (guide.reposition(route.path)) scheduleTargetLocate(true)
}

function skipGuide(): void {
  actionState.value = 'idle'
  cancellationClicked = false
  guide.cancel()
  ElMessage.info('已跳过本次手动引导')
}

function exitGuide(): void {
  actionState.value = 'idle'
  cancellationClicked = false
  guide.cancel()
}

watch(
  () => [visible.value, contentVisible.value, guide.currentStep, route.fullPath] as const,
  async ([isVisible, isContentVisible]) => {
    if (!isVisible) {
      clearTarget()
      stopBodyObserver()
      lastLocatedStepId = ''
      return
    }
    startBodyObserver()
    updateBusinessLayerState()
    if (!isContentVisible) {
      clearTarget()
      return
    }
    await nextTick()
    scheduleTargetLocate(true)
  },
  { immediate: true },
)

watch(
  [visible, () => guide.completed],
  ([isVisible, completed]) => {
    document.body.classList.toggle('manual-guide-active', isVisible && !completed)
  },
  { immediate: true },
)

watch(
  () => guide.currentStep,
  async (current, previous) => {
    if (!guide.active || guide.completed || current <= previous) return
    await nextTick()
    const targetRoute = resolvedCurrentStepRoute()
    if (targetRoute && targetRoute.split('?')[0] !== route.path) {
      await router.push(targetRoute)
    }
  },
)

watch(guideRevision, syncActionFromGuide)
watch(
  () => guide.completed,
  (completed) => {
    if (completed) {
      actionState.value = 'idle'
      businessLayerOpen.value = false
      clearTarget()
    }
  },
)

watch(
  () => auth.isDemo,
  (isDemo) => {
    if (!isDemo) {
      actionState.value = 'idle'
      guide.cancel()
    }
  },
)

function startBodyObserver(): void {
  if (bodyObserver) return
  bodyObserver = new window.MutationObserver(() => {
    updateBusinessLayerState()
    refreshTarget()
  })
  bodyObserver.observe(document.body, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['class', 'style', 'hidden'],
  })
}

function stopBodyObserver(): void {
  bodyObserver?.disconnect()
  bodyObserver = undefined
}

onMounted(() => {
  window.addEventListener('resize', updateViewport, { passive: true })
  window.addEventListener('scroll', refreshTarget, { passive: true })
  document.addEventListener('click', onDocumentClickCapture, true)
  document.addEventListener('keydown', onKeydownCapture, true)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateViewport)
  window.removeEventListener('scroll', refreshTarget)
  document.removeEventListener('click', onDocumentClickCapture, true)
  document.removeEventListener('keydown', onKeydownCapture, true)
  if (locateFrame !== undefined) window.cancelAnimationFrame(locateFrame)
  targetObserver?.disconnect()
  stopBodyObserver()
  document.body.classList.remove('manual-guide-active')
})
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="manual-guide" aria-live="polite">
      <section v-if="guide.active && step" class="guide-strip" aria-label="手动演示状态">
        <div class="guide-strip__mode">
          <i aria-hidden="true"></i>
          <span>演示模式</span>
          <b>{{ scenarioLabel }}流程</b>
        </div>
        <div class="guide-strip__progress">
          <span>第 {{ guide.currentStepNumber }} / {{ guide.totalSteps }} 步</span>
          <i aria-hidden="true"></i>
          <b>{{ step.title }}</b>
          <small v-if="guide.lastOutcome">当前操作已完成</small>
        </div>
        <div class="guide-strip__actions">
          <el-button link type="primary" @click="openRecords">操作日志</el-button>
          <el-button link type="danger" @click="exitGuide">退出演示</el-button>
        </div>
      </section>

      <section
        v-if="guide.completed"
        class="guide-panel guide-panel--complete"
        style="position: fixed"
        :style="completedPanelStyle"
        aria-label="手动业务引导完成"
      >
        <span class="guide-kicker">{{ scenarioLabel }}手动引导</span>
        <h2>手动流程已完成</h2>
        <div class="guide-complete-flow">
          <template v-for="(item, index) in completionSteps" :key="item">
            <span>{{ item }}</span>
            <b v-if="index < completionSteps.length - 1">→</b>
          </template>
        </div>
        <p>{{ guide.lastOutcome || '真实业务操作已完成，可以继续核对结果。' }}</p>
        <div class="guide-complete-facts">
          <span v-if="guide.orderNo || guide.orderId">{{ guide.scenario === 'outbound' ? '出库单' : '入库单' }}：{{ guide.orderNo || guide.orderId }}</span>
          <span v-if="guide.taskNo || guide.taskId">{{ guide.scenario === 'outbound' ? '拣货任务' : '上架任务' }}：{{ guide.taskNo || guide.taskId }}</span>
          <span v-for="fact in guide.facts" :key="fact.label">{{ fact.label }}：{{ fact.value }}</span>
        </div>
        <div class="guide-complete-actions">
          <el-button size="small" @click="completeNavigate(manualEvidencePath())">查看业务证据</el-button>
          <el-button size="small" :disabled="!guide.orderId" @click="completeNavigate(completedOrderPath())">查看业务对象</el-button>
          <el-button size="small" type="primary" @click="completeNavigate('/demo')">返回演示中心</el-button>
        </div>
        <div class="guide-actions">
          <el-button @click="restartGuide">重新开始</el-button>
          <el-button type="primary" @click="exitGuide">退出引导</el-button>
        </div>
      </section>

      <template v-else-if="contentVisible">
        <div v-if="targetRect" class="guide-highlight" :style="highlightStyle"></div>

        <section
          v-if="step"
          class="guide-panel guide-bubble"
          style="position: fixed"
          :style="panelStyle"
          aria-label="手动业务引导"
        >
          <div class="guide-bubble__head">
            <div>
              <span class="guide-kicker">{{ scenarioLabel }}流程</span>
              <b>{{ step.title }}</b>
            </div>
            <span>第 {{ guide.currentStepNumber }} / {{ guide.totalSteps }} 步</span>
          </div>
          <p>{{ step.description }}</p>

          <div v-if="guide.mismatch" class="guide-mismatch" role="alert">
            <b>当前状态与引导不一致</b>
            <span>{{ guide.mismatch }}</span>
            <div class="guide-mismatch-actions">
              <el-button size="small" @click="repositionGuide">重新定位</el-button>
              <el-button size="small" @click="restartGuide">重新开始</el-button>
            </div>
          </div>

          <template v-else>
            <div v-if="guide.lastOutcome" class="guide-outcome">{{ guide.lastOutcome }}</div>
            <div v-else class="guide-waiting">
              <i aria-hidden="true"></i>
              <span>请完成当前高亮的真实业务操作，成功后会解锁下一步。</span>
            </div>

            <div class="guide-actions guide-actions--auto">
              <span>真实业务操作成功后，引导会自动进入下一步。</span>
              <el-button link @click="skipGuide">跳过</el-button>
            </div>
          </template>
        </section>
      </template>
    </div>

    <el-drawer
      v-model="recordsVisible"
      class="guide-records-drawer"
      title="本次操作日志"
      size="min(440px, 92vw)"
      append-to-body
      :close-on-click-modal="true"
    >
      <div v-loading="recordsLoading" class="guide-records">
        <el-alert v-if="recordsError" :title="recordsError" type="warning" :closable="false" show-icon />
        <template v-else>
          <article v-for="operation in businessOperations" :key="operation.key" class="guide-record">
            <div>
              <b>{{ operation.operation }} · {{ operation.objectNo }}</b>
              <span>{{ operation.beforeStatus }} → {{ operation.afterStatus }} · 数量 {{ operation.quantityChange }} · {{ formatTime(operation.createdAt) }}</span>
            </div>
            <el-tag :type="httpStatusType(operation.raw.status)" size="small">{{ operation.raw.status }}</el-tag>
          </article>
          <el-empty v-if="!recordsLoading && businessOperations.length === 0" description="本次还没有业务操作记录" />
        </template>
      </div>
    </el-drawer>
  </Teleport>
</template>

<style scoped>
.manual-guide {
  position: relative;
}

.guide-strip {
  position: fixed;
  top: 60px;
  right: 0;
  left: 220px;
  z-index: 1900;
  height: 48px;
  padding: 0 18px;
  border-bottom: 1px solid var(--el-border-color-light);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  color: var(--el-text-color-regular);
  background: color-mix(in srgb, var(--el-bg-color-overlay) 96%, transparent);
  box-shadow: 0 2px 8px rgba(31, 41, 55, 0.05);
  backdrop-filter: blur(8px);
}

.guide-strip__mode,
.guide-strip__progress,
.guide-strip__actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.guide-strip__mode i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--el-color-primary);
  box-shadow: 0 0 0 4px var(--el-color-primary-light-9);
}

.guide-strip__mode span,
.guide-strip__progress span,
.guide-strip__progress small {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.guide-strip__progress i {
  width: 1px;
  height: 14px;
  background: var(--el-border-color);
}

.guide-strip__actions .el-button {
  margin-left: 0;
}

.guide-highlight {
  position: fixed;
  z-index: 1800;
  pointer-events: none;
  border: 2px solid var(--el-color-primary);
  border-radius: 9px;
  background: transparent;
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--el-color-primary) 18%, transparent);
  transition: top 0.16s ease, left 0.16s ease, width 0.16s ease, height 0.16s ease;
}

.guide-panel {
  z-index: 1810;
  pointer-events: none;
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  color: var(--el-text-color-primary);
  background: var(--el-bg-color-overlay);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.14);
}

.guide-bubble {
  padding: 14px;
}

.guide-bubble__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.guide-bubble__head > div {
  min-width: 0;
}

.guide-bubble__head b {
  display: block;
  margin-top: 3px;
  font-size: 16px;
}

.guide-bubble__head > span {
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.guide-kicker {
  color: var(--el-color-primary);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.guide-panel > p {
  margin: 8px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.guide-waiting,
.guide-outcome,
.guide-mismatch {
  margin-top: 10px;
  padding: 9px 10px;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.55;
}

.guide-waiting {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
}

.guide-waiting i {
  width: 6px;
  height: 6px;
  margin-top: 5px;
  flex: none;
  border-radius: 50%;
  background: var(--el-color-warning);
}

.guide-outcome {
  color: var(--el-color-success);
  background: var(--el-color-success-light-9);
}

.guide-mismatch {
  display: flex;
  flex-direction: column;
  gap: 5px;
  color: var(--el-text-color-regular);
  background: var(--el-color-danger-light-9);
}

.guide-mismatch b {
  color: var(--el-color-danger);
}

.guide-mismatch-actions,
.guide-complete-actions,
.guide-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.guide-mismatch-actions {
  margin-top: 5px;
}

.guide-actions {
  justify-content: space-between;
  gap: 8px;
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.guide-actions--auto > span {
  color: var(--el-text-color-secondary);
  font-size: 11px;
  line-height: 1.5;
}

.guide-actions > div {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.guide-panel button {
  pointer-events: auto;
}

.guide-actions .el-button,
.guide-complete-actions .el-button,
.guide-mismatch-actions .el-button {
  margin-left: 0;
}

.guide-panel--complete {
  padding: 16px;
}

.guide-panel--complete h2 {
  margin: 6px 0 0;
  font-size: 18px;
}

.guide-complete-flow {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  margin-top: 10px;
  color: var(--el-text-color-regular);
  font-size: 12px;
  font-weight: 600;
}

.guide-complete-flow b {
  color: var(--el-text-color-placeholder);
  font-weight: 400;
}

.guide-complete-actions {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.guide-records {
  min-height: 120px;
}

.guide-record {
  padding: 11px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.guide-record:last-child {
  border-bottom: 0;
}

.guide-record > div {
  min-width: 0;
}

.guide-record b,
.guide-record span {
  display: block;
}

.guide-record b {
  font-size: 13px;
}

.guide-record span {
  margin-top: 3px;
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

@media (max-width: 768px) {
  .guide-strip {
    left: 0;
    padding: 0 12px;
  }

  .guide-strip__progress small,
  .guide-strip__mode span {
    display: none;
  }

  .guide-panel {
    max-height: 54vh;
    overflow-y: auto;
  }
}
.guide-complete-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 5px 12px;
  margin-top: 10px;
  color: var(--el-text-color-regular);
  font-size: 12px;
}

</style>

<style>
body.manual-guide-active .main {
  padding-top: 64px;
}
</style>
