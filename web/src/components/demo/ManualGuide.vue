<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type CSSProperties } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { GUIDE_SCENARIO_LABELS, useGuideStore } from '@/stores/guide'

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
const orderFactLabel = computed(() => {
  if (guide.scenario === 'outbound') return '出库单'
  if (guide.scenario === 'stocktake') return '盘点单'
  return '入库单'
})
const taskFactLabel = computed(() => {
  if (guide.scenario === 'outbound') return '拣货任务'
  if (guide.scenario === 'stocktake') return '盘点任务'
  return '上架任务'
})

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
  const panelWidth = Math.min(360, viewport.value.width - 24)
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
      top: '24px',
      width: `${panelWidth}px`,
      transform: 'translateX(-50%)',
    }
  }

  const estimatedHeight = 310
  const gap = 14
  const left = Math.min(Math.max(12, rect.left), viewport.value.width - panelWidth - 12)
  let top = rect.top + rect.height + gap
  if (top + estimatedHeight > viewport.value.height - 12) {
    const above = rect.top - estimatedHeight - gap
    top = above >= 12 ? above : Math.max(12, viewport.value.height - estimatedHeight - 12)
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

async function nextStep(): Promise<void> {
  if (!guide.next()) return
  if (!guide.completed && guide.currentStepRoute) {
    await router.push(guide.currentStepRoute)
  }
}

async function previousStep(): Promise<void> {
  if (!guide.previous() || !guide.currentStepRoute) return
  await router.push(guide.currentStepRoute)
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
})
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="manual-guide" aria-live="polite">
      <template v-if="guide.completed">
        <section
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
          <p>{{ guide.lastOutcome || '真实业务操作已完成，可以继续在业务页面核对结果。' }}</p>
          <div class="guide-complete-facts">
            <span v-if="guide.orderNo || guide.orderId">{{ orderFactLabel }}：{{ guide.orderNo || guide.orderId }}</span>
            <span v-if="guide.taskNo || guide.taskId">{{ taskFactLabel }}：{{ guide.taskNo || guide.taskId }}</span>
            <span v-for="fact in guide.facts" :key="fact.label">{{ fact.label }}：{{ fact.value }}</span>
          </div>
          <div class="guide-complete-actions">
            <el-button size="small" @click="completeNavigate(manualEvidencePath())">查看业务证据</el-button>
            <el-button size="small" :disabled="!guide.orderId" @click="completeNavigate(completedOrderPath())">
              查看业务对象
            </el-button>
            <el-button size="small" type="primary" @click="completeNavigate('/demo')">返回演示中心</el-button>
          </div>
          <div class="guide-actions">
            <el-button @click="restartGuide">重新开始</el-button>
            <el-button type="primary" @click="exitGuide">退出引导</el-button>
          </div>
        </section>
      </template>

      <template v-else-if="contentVisible">
        <div v-if="!targetRect" class="guide-shade"></div>
        <div v-else class="guide-highlight" :style="highlightStyle"></div>

        <section
          v-if="step"
          class="guide-panel"
          style="position: fixed"
          :style="panelStyle"
          aria-label="手动业务引导"
        >
          <div class="guide-head">
            <span class="guide-kicker">{{ scenarioLabel }}手动引导</span>
            <strong>第 {{ guide.currentStepNumber }} / {{ guide.totalSteps }} 步</strong>
          </div>
          <h2>{{ step.title }}</h2>
          <p>{{ step.description }}</p>

          <div v-if="guide.mismatch" class="guide-mismatch" role="alert">
            <b>当前业务状态与引导不一致</b>
            <span>{{ guide.mismatch }}</span>
            <div class="guide-mismatch-actions">
              <el-button size="small" @click="repositionGuide">重新定位当前步骤</el-button>
              <el-button size="small" @click="restartGuide">重新开始</el-button>
              <el-button size="small" type="danger" plain @click="exitGuide">退出引导</el-button>
            </div>
          </div>

          <template v-else>
            <div v-if="guide.lastOutcome" class="guide-outcome">
              <b>刚才发生了什么</b>
              <span>{{ guide.lastOutcome }}</span>
            </div>
            <div v-else class="guide-waiting">
              <span class="guide-waiting-dot"></span>
              请先在页面中完成当前真实业务操作，下一步会在结果确认后解锁。
            </div>

            <div class="guide-actions">
              <div>
                <el-button v-if="guide.canGoPrevious" size="small" @click="previousStep">上一步</el-button>
                <el-button v-if="guide.canAdvance" size="small" type="primary" @click="nextStep">
                  下一步
                </el-button>
              </div>
              <div>
                <el-button link @click="skipGuide">跳过引导</el-button>
                <el-button link type="danger" @click="exitGuide">退出引导</el-button>
              </div>
            </div>
          </template>
        </section>
      </template>
    </div>
  </Teleport>
</template>

<style scoped>
.manual-guide {
  position: relative;
  z-index: 2600;
}

.guide-shade {
  position: fixed;
  inset: 0;
  z-index: 2600;
  pointer-events: none;
  background: rgba(8, 15, 28, 0.58);
}

.guide-highlight {
  position: fixed;
  z-index: 2601;
  pointer-events: none;
  border: 2px solid var(--el-color-primary);
  border-radius: 10px;
  box-shadow:
    0 0 0 9999px rgba(8, 15, 28, 0.58),
    0 0 0 5px color-mix(in srgb, var(--el-color-primary) 24%, transparent);
  transition: top 0.16s ease, left 0.16s ease, width 0.16s ease, height 0.16s ease;
}

.guide-panel {
  z-index: 2602;
  pointer-events: none;
  padding: 18px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 14px;
  color: var(--el-text-color-primary);
  background: var(--el-bg-color-overlay);
  box-shadow: 0 18px 48px rgba(8, 15, 28, 0.28);
}

.guide-panel--complete {
  border-color: var(--el-color-success-light-5);
  background: var(--el-bg-color-overlay);
}

.guide-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.guide-kicker {
  color: var(--el-color-primary);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.guide-head strong {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  font-weight: 500;
}

.guide-panel h2 {
  margin: 10px 0 8px;
  font-size: 19px;
}

.guide-panel > p {
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.75;
}

.guide-waiting,
.guide-outcome,
.guide-mismatch {
  margin-top: 14px;
  padding: 11px 12px;
  border-radius: 9px;
  font-size: 12px;
  line-height: 1.65;
}

.guide-waiting {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
}

.guide-waiting-dot {
  width: 7px;
  height: 7px;
  margin-top: 5px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--el-color-warning);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--el-color-warning) 14%, transparent);
}

.guide-outcome {
  display: flex;
  flex-direction: column;
  gap: 3px;
  color: var(--el-text-color-regular);
  background: var(--el-color-success-light-9);
}

.guide-outcome b {
  color: var(--el-color-success);
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

.guide-mismatch-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 5px;
}

.guide-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 16px;
  padding-top: 13px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.guide-panel button {
  pointer-events: auto;
}

.guide-actions .el-button,
.guide-complete-actions .el-button,
.guide-mismatch-actions .el-button {
  margin-left: 0;
}

.guide-complete-flow {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  margin-top: 12px;
  color: var(--el-text-color-regular);
  font-size: 12px;
  font-weight: 600;
}

.guide-complete-flow b {
  color: var(--el-text-color-placeholder);
  font-weight: 400;
}

.guide-complete-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  margin-top: 12px;
  color: var(--el-text-color-regular);
  font-size: 12px;
}

.guide-complete-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 14px;
  padding-top: 13px;
  border-top: 1px solid var(--el-border-color-lighter);
}

@media (max-width: 640px) {
  .guide-panel {
    max-height: 58vh;
    overflow-y: auto;
  }

  .guide-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .guide-actions > div {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}
</style>