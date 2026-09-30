import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DATA_CHANGED_EVENT } from '@/utils/events'
import { useAutoRefresh } from './autoRefresh'

type WindowEventHandler = (event: Event) => void

/**
 * 测试环境是 node，没有 window：
 * 这个桩同时提供前端的 DOM 事件能力（events.ts 依赖）与定时器能力。
 * 定时器动态委托给 globalThis，这样 vi.useFakeTimers() 替换全局定时器后依然生效。
 */
function createWindowStub() {
  const listeners = new Map<string, Set<WindowEventHandler>>()
  return {
    setInterval: (handler: () => void, timeout?: number) =>
      globalThis.setInterval(handler, timeout) as unknown as number,
    clearInterval: (id: number) => {
      globalThis.clearInterval(id)
    },
    setTimeout: (handler: () => void, timeout?: number) =>
      globalThis.setTimeout(handler, timeout) as unknown as number,
    clearTimeout: (id: number) => {
      globalThis.clearTimeout(id)
    },
    addEventListener: (type: string, handler: WindowEventHandler) => {
      const handlers = listeners.get(type) ?? new Set<WindowEventHandler>()
      handlers.add(handler)
      listeners.set(type, handlers)
    },
    removeEventListener: (type: string, handler: WindowEventHandler) => {
      listeners.get(type)?.delete(handler)
    },
    dispatchEvent: (event: Event) => {
      listeners.get(event.type)?.forEach((handler) => {
        handler(event)
      })
      return true
    },
  }
}

function createDeferred<T>() {
  let resolve: (value: T) => void = () => {}
  let reject: (reason?: unknown) => void = () => {}
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

/** 让 await 之后的逻辑跑完 */
async function flushMicrotasks() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

describe('useAutoRefresh 防重叠与生命周期', () => {
  let testWindow: ReturnType<typeof createWindowStub>

  beforeEach(() => {
    vi.useFakeTimers()
    testWindow = createWindowStub()
    vi.stubGlobal('window', testWindow)
    // 组件外调用 onMounted/onBeforeUnmount 只会产生 Vue 警告，静默掉以保持输出干净
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  function emitDataChanged() {
    testWindow.dispatchEvent(new Event(DATA_CHANGED_EVENT))
  }

  it('inFlight 时不立即刷新，当前请求完成后只补刷一次', async () => {
    const deferred = createDeferred<void>()
    const refresh = vi.fn(() => deferred.promise)
    const auto = useAutoRefresh(refresh, 1000)
    auto.start()

    await vi.advanceTimersByTimeAsync(1000)
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(auto.running.value).toBe(true)

    // 请求仍悬挂：推进 5 个间隔并额外触发一次数据变更
    await vi.advanceTimersByTimeAsync(5000)
    emitDataChanged()
    await flushMicrotasks()
    // 期间不应并发刷新
    expect(refresh).toHaveBeenCalledTimes(1)

    deferred.resolve()
    await flushMicrotasks()
    // 结束后应补刷一次，总共 2 次
    expect(refresh).toHaveBeenCalledTimes(2)
    expect(auto.running.value).toBe(false)
    expect(auto.error.value).toBeNull()
    expect(auto.lastUpdatedAt.value).not.toBeNull()

    // 停止后不再自动刷新
    auto.stop()
    await vi.advanceTimersByTimeAsync(1000)
    expect(refresh).toHaveBeenCalledTimes(2)
  })

  it('intervalMs 为 0 时只有数据变更事件触发刷新，stop() 取消订阅', async () => {
    const refresh = vi.fn(() => Promise.resolve())
    const auto = useAutoRefresh(refresh, 0)
    auto.start()

    await vi.advanceTimersByTimeAsync(10000)
    expect(refresh).not.toHaveBeenCalled()

    emitDataChanged()
    await flushMicrotasks()
    expect(refresh).toHaveBeenCalledTimes(1)

    auto.stop()
    emitDataChanged()
    await flushMicrotasks()
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('stop() 之后定时器不再触发刷新', async () => {
    const refresh = vi.fn(() => Promise.resolve())
    const auto = useAutoRefresh(refresh, 1000)
    auto.start()

    await vi.advanceTimersByTimeAsync(1000)
    expect(refresh).toHaveBeenCalledTimes(1)

    auto.stop()
    await vi.advanceTimersByTimeAsync(10000)
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('重复 start() 不会叠加定时器', async () => {
    const refresh = vi.fn(() => Promise.resolve())
    const auto = useAutoRefresh(refresh, 1000)
    auto.start()
    auto.start()

    await vi.advanceTimersByTimeAsync(1000)
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('refreshNow() 立即刷新一次，请求悬挂期间不会重复发起', async () => {
    const deferred = createDeferred<void>()
    const refresh = vi.fn(() => deferred.promise)
    const auto = useAutoRefresh(refresh, 1000)

    const first = auto.refreshNow()
    await flushMicrotasks()
    expect(refresh).toHaveBeenCalledTimes(1)

    await auto.refreshNow()
    expect(refresh).toHaveBeenCalledTimes(1)

    deferred.resolve()
    await first
    expect(auto.running.value).toBe(false)
    expect(auto.lastUpdatedAt.value).not.toBeNull()
  })

  it('shouldRefresh() 为 false 时不发起请求', async () => {
    const shouldRefresh = vi.fn(() => false)
    const refresh = vi.fn(() => Promise.resolve())
    const auto = useAutoRefresh(refresh, 1000, shouldRefresh)
    auto.start()

    await vi.advanceTimersByTimeAsync(3000)
    expect(shouldRefresh).toHaveBeenCalled()
    expect(refresh).not.toHaveBeenCalled()

    await auto.refreshNow()
    expect(refresh).not.toHaveBeenCalled()
    expect(auto.running.value).toBe(false)
  })

  it('refresh 抛错时记录到 error 且不产生未处理拒绝，之后仍可继续刷新', async () => {
    const boom = new Error('加载失败')
    const refresh = vi.fn<() => Promise<void>>(() => Promise.reject(boom))
    const auto = useAutoRefresh(refresh, 1000)

    await expect(auto.refreshNow()).resolves.toBeUndefined()
    expect(auto.error.value).toBe(boom)
    expect(auto.running.value).toBe(false)
    expect(auto.lastUpdatedAt.value).toBeNull()

    refresh.mockImplementation(() => Promise.resolve())
    await auto.refreshNow()
    expect(auto.error.value).toBeNull()
    expect(auto.lastUpdatedAt.value).not.toBeNull()
  })

  // onBeforeUnmount 注册的清理函数在 node 环境（无组件实例）无法触发，
  // 这里直接调用它注册的同一个 stop() 验证等价的清理行为。
  it('stop() 等价于组件卸载清理：清空定时器与订阅，重复调用安全', async () => {
    const refresh = vi.fn(() => Promise.resolve())
    const auto = useAutoRefresh(refresh, 1000)
    auto.start()
    auto.stop()
    auto.stop()

    await vi.advanceTimersByTimeAsync(5000)
    emitDataChanged()
    await flushMicrotasks()
    expect(refresh).not.toHaveBeenCalled()
  })
})
