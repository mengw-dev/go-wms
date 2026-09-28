/**
 * Guide 高亮目标定位。
 *
 * 只负责 DOM：按选择器查找目标元素、读取位置、监听 resize/scroll，
 * 目标暂未出现时按帧重试。不依赖 Guide Store，也不判断业务状态；
 * 当前是否需要高亮由 anchor 回调决定，找不到目标如何处理由调用方决定。
 */
import { onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'

export interface GuideTargetRect {
  top: number
  left: number
  width: number
  height: number
}

export interface GuideTargetAnchor {
  /** 当前步骤的高亮目标选择器。 */
  selector: string
  /** 当前步骤 ID，同一目标只自动滚动一次。 */
  stepId: string
}

export interface UseGuideTargetOptions {
  /** 返回当前需要定位的目标；返回 null 表示现在不需要高亮。 */
  anchor: () => GuideTargetAnchor | null
  /** 连续重试后仍找不到目标时的回调。 */
  onLocateFailed?: () => void
}

const MAX_LOCATE_ATTEMPTS = 30

export function useGuideTarget(options: UseGuideTargetOptions) {
  const targetRect = ref<GuideTargetRect | null>(null)
  const targetElement = shallowRef<HTMLElement | null>(null)
  const viewport = ref({ width: window.innerWidth, height: window.innerHeight })

  let targetObserver: InstanceType<typeof window.ResizeObserver> | undefined
  let locateFrame: number | undefined
  let locateAttempts = 0
  let lastLocatedStepId = ''
  let observedTarget: HTMLElement | null = null

  function clearTarget(): void {
    targetRect.value = null
    targetElement.value = null
    targetObserver?.disconnect()
    targetObserver = undefined
    observedTarget = null
  }

  function refreshTarget(): void {
    const anchor = options.anchor()
    if (!anchor) {
      clearTarget()
      return
    }

    const element = document.querySelector<HTMLElement>(anchor.selector)
    if (!element) {
      clearTarget()
      return
    }

    if (observedTarget !== element) {
      clearTarget()
      observedTarget = element
      targetElement.value = element
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

    if (lastLocatedStepId !== anchor.stepId) {
      lastLocatedStepId = anchor.stepId
      element.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' })
    }
  }

  /** 按帧调度一次定位；目标未出现时重试，超过上限后回调 onLocateFailed。 */
  function locateTarget(reset = false): void {
    if (reset) locateAttempts = 0
    if (locateFrame !== undefined) window.cancelAnimationFrame(locateFrame)

    const locate = () => {
      locateFrame = undefined
      refreshTarget()
      if (targetRect.value || !options.anchor()) return
      if (locateAttempts < MAX_LOCATE_ATTEMPTS) {
        locateAttempts += 1
        locateFrame = window.requestAnimationFrame(locate)
        return
      }
      options.onLocateFailed?.()
    }
    locateFrame = window.requestAnimationFrame(locate)
  }

  /** 忘记“已滚动定位”的步骤，下次出现同一目标时重新滚动。 */
  function resetLocatedStep(): void {
    lastLocatedStepId = ''
  }

  function updateViewport(): void {
    viewport.value = { width: window.innerWidth, height: window.innerHeight }
    refreshTarget()
  }

  onMounted(() => {
    window.addEventListener('resize', updateViewport, { passive: true })
    window.addEventListener('scroll', refreshTarget, { passive: true })
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', updateViewport)
    window.removeEventListener('scroll', refreshTarget)
    if (locateFrame !== undefined) window.cancelAnimationFrame(locateFrame)
    targetObserver?.disconnect()
  })

  return {
    targetRect,
    targetElement,
    viewport,
    locateTarget,
    refreshTarget,
    clearTarget,
    resetLocatedStep,
  }
}

export type GuideTarget = ReturnType<typeof useGuideTarget>
