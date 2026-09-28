/**
 * 手动引导（Guide）运行器。
 *
 * 负责引导 Overlay 的完整生命周期：可见性判断、步骤切换时自动导航、
 * 偏离流程时自动结束、业务弹层/操作结果跟踪、完成后记录证据时间窗。
 * ManualGuide.vue 只消费这里暴露的状态和动作做展示，不再自己维护状态机。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { isGuideFlowRoute } from '@/guide/routeMatcher'
import { useAuthStore } from '@/stores/auth'
import { GUIDE_SCENARIO_LABELS, useGuideStore } from '@/stores/guide'
import { rememberDemoExecutionWindow } from '@/utils/demoEvidence'
import { useGuideTarget } from './useGuideTarget'

type ActionState = 'idle' | 'pending' | 'failed'

export function useGuideRunner() {
  const route = useRoute()
  const router = useRouter()
  const auth = useAuthStore()
  const guide = useGuideStore()

  const businessLayerOpen = ref(false)
  const actionState = ref<ActionState>('idle')

  let bodyObserver: InstanceType<typeof window.MutationObserver> | undefined
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

  const target = useGuideTarget({
    anchor: () => {
      if (!contentVisible.value || !step.value) return null
      return { selector: step.value.target, stepId: step.value.id }
    },
    onLocateFailed: () => {
      guide.setMismatch('未找到当前步骤对应的业务按钮。请重新定位当前步骤，或重新开始/退出引导。')
    },
  })

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
    target.clearTarget()
  }

  function syncActionFromGuide(): void {
    if (actionState.value === 'pending' && guideRevision() !== actionRevision) {
      actionState.value = 'idle'
      cancellationClicked = false
    }
  }

  function onDocumentClickCapture(event: globalThis.Event): void {
    const clickTarget = event.target as HTMLElement | null
    if (!clickTarget?.closest) return

    const clickedButton = clickTarget.closest('button')
    const clickedBusinessContainer = clickedButton?.closest('.el-dialog, .el-message-box')
    if (clickedButton && clickedBusinessContainer) {
      const buttonText = clickedButton.textContent?.replace(/\s+/g, '') || ''
      if (['取消', '返回', '关闭', '继续演示'].some((label) => buttonText.includes(label))) {
        cancellationClicked = true
      }
    }

    if (!contentVisible.value || !step.value) return
    const actionable = clickTarget.closest('button, a, [role="button"], .el-button')
    if (!actionable || !actionable.closest(step.value.target)) return
    beginTargetAction()
  }

  function onKeydownCapture(event: KeyboardEvent): void {
    if (event.key === 'Escape' && hasBusinessLayer()) cancellationClicked = true
  }

  function stopGuideOutsideFlow(): void {
    actionState.value = 'idle'
    cancellationClicked = false
    businessLayerOpen.value = false
    target.clearTarget()
    guide.cancel()
    ElMessage.info('已离开引导流程，本次引导已自动结束')
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
    if (guide.reposition(route.path)) target.locateTarget(true)
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
        target.clearTarget()
        target.resetLocatedStep()
        stopBodyObserver()
        return
      }
      startBodyObserver()
      updateBusinessLayerState()
      if (!isContentVisible) {
        target.clearTarget()
        return
      }
      await nextTick()
      target.locateTarget(true)
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
      const targetRoute = guide.currentStepRoute
      if (targetRoute && targetRoute.split('?')[0] !== route.path) {
        await router.push(targetRoute)
      }
    },
  )

  watch(guideRevision, syncActionFromGuide)

  watch(
    () => [route.path, guide.active, guide.scenario, guide.orderId, guide.orderNo] as const,
    ([path, active]) => {
      if (!active || !guide.scenario) return
      if (!isGuideFlowRoute(guide.scenario, path, guide.orderId, guide.orderNo)) stopGuideOutsideFlow()
    },
    { immediate: true },
  )

  watch(
    () => guide.completed,
    (completed) => {
      if (completed) {
        actionState.value = 'idle'
        businessLayerOpen.value = false
        target.clearTarget()
        if (guide.scenario && guide.startedAt) {
          rememberDemoExecutionWindow(
            guide.scenario,
            new Date(guide.startedAt).toISOString(),
            new Date().toISOString(),
            guide.orderId ? [{ label: '业务单据', path: completedOrderPath() }] : [],
          )
        }
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
      target.refreshTarget()
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
    document.addEventListener('click', onDocumentClickCapture, true)
    document.addEventListener('keydown', onKeydownCapture, true)
  })

  onBeforeUnmount(() => {
    document.removeEventListener('click', onDocumentClickCapture, true)
    document.removeEventListener('keydown', onKeydownCapture, true)
    stopBodyObserver()
    document.body.classList.remove('manual-guide-active')
  })

  return {
    visible,
    contentVisible,
    step,
    scenarioLabel,
    targetRect: target.targetRect,
    targetElement: target.targetElement,
    viewport: target.viewport,
    restartGuide,
    skipGuide,
    exitGuide,
    repositionGuide,
    completeNavigate,
    completedOrderPath,
    manualEvidencePath,
  }
}
