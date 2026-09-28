import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue'
import { onDataChanged } from '@/utils/events'

export interface AutoRefreshController {
  /** 当前是否有一次刷新正在执行 */
  running: Ref<boolean>
  /** 最近一次刷新成功的时间戳；从未成功过为 null */
  lastUpdatedAt: Ref<number | null>
  /** 最近一次刷新失败的异常；刷新成功后清空 */
  error: Ref<unknown>
  /** 启动定时轮询与数据变更订阅，重复调用无副作用 */
  start: () => void
  /** 停止定时轮询与数据变更订阅 */
  stop: () => void
  /** 立即刷新一次，仍受防重叠与 shouldRefresh 约束 */
  refreshNow: () => Promise<void>
}

// intervalMs 为 0 时不启用定时轮询，只在数据变更事件触发时刷新。
const TIMER_DISABLED = 0

/**
 * 页面自动刷新：定时轮询 + 数据变更（如演示流程完成）后立即刷新。
 *
 * 同一时刻最多只有一个 refresh 在执行：上一次未结束时，定时器与数据变更
 * 事件触发的刷新都会被直接跳过，避免请求重叠与响应乱序。
 */
export function useAutoRefresh(
  refresh: () => void | Promise<void>,
  intervalMs = 5000,
  shouldRefresh: () => boolean = () => true,
): AutoRefreshController {
  const running = ref(false)
  const lastUpdatedAt = ref<number | null>(null)
  const error = ref<unknown>(null)

  let timer: number | undefined
  let unsubscribe: (() => void) | undefined
  let inFlight = false

  async function trigger() {
    if (inFlight) return
    if (!shouldRefresh()) return
    inFlight = true
    running.value = true
    error.value = null
    try {
      await refresh()
      lastUpdatedAt.value = Date.now()
    } catch (err) {
      // 自动刷新失败由调用方决定是否展示提示，这里只记录，不向上抛出。
      error.value = err
    } finally {
      inFlight = false
      running.value = false
    }
  }

  function start() {
    if (intervalMs > TIMER_DISABLED && timer === undefined) {
      timer = window.setInterval(() => { void trigger() }, intervalMs)
    }
    if (!unsubscribe) {
      unsubscribe = onDataChanged(() => { void trigger() })
    }
  }

  function stop() {
    if (timer !== undefined) {
      window.clearInterval(timer)
      timer = undefined
    }
    unsubscribe?.()
    unsubscribe = undefined
  }

  onMounted(start)
  onBeforeUnmount(stop)

  return { running, lastUpdatedAt, error, start, stop, refreshNow: () => trigger() }
}
