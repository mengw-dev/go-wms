import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, nextTick } from 'vue'
import { createPinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import type { DemoSessionInfo } from '@/api/types'
import { useDemoSession } from './useDemoSession'

const api = vi.hoisted(() => ({ acquire: vi.fn(), heartbeat: vi.fn(), release: vi.fn(), push: vi.fn(), closed: vi.fn() }))
vi.mock('@/api/demo', () => ({ acquireDemoSession: api.acquire, heartbeatDemoSession: api.heartbeat, releaseDemoSession: api.release }))
vi.mock('@/api/request', () => ({ ApiError: class extends Error { code = 70003 } }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: api.push }) }))
vi.mock('element-plus', () => ({ ElMessage: { warning: vi.fn() } }))
const renderer = createRenderer<object, object>({
  patchProp() {}, insert() {}, remove() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
})
const unmounts: (() => void)[] = []
const timers = new Map<number, () => void>()
const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); await nextTick() }
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function storage(): Storage {
  const map = new Map<string, string>()
  return { getItem: key => map.get(key) ?? null, setItem: (key, value) => { map.set(key, value) }, removeItem: key => { map.delete(key) } } as Storage
}
const login = { token: 'same', user_id: '2', username: 'demo', nickname: '', roles: [], perms: ['wms:demo'] }
function mount(lease = true) {
  const pinia = createPinia(), auth = useAuthStore(pinia)
  auth.setAuth(login)
  if (lease) auth.setDemoSession({ session_id: 'old', expires_in: 300 })
  let session!: ReturnType<typeof useDemoSession>
  const app = renderer.createApp({ setup() { session = useDemoSession({ onSessionClosed: api.closed }); return () => null } })
  app.use(pinia); app.mount({})
  const unmount = () => app.unmount(); unmounts.push(unmount)
  return { auth, session, unmount }
}
beforeEach(() => {
  vi.resetAllMocks(); timers.clear()
  vi.stubGlobal('localStorage', storage()); vi.stubGlobal('sessionStorage', storage())
  let id = 0
  vi.stubGlobal('window', { addEventListener() {}, removeEventListener() {}, setInterval: (fn: () => void) => { timers.set(++id, fn); return id }, clearInterval: (id: number) => timers.delete(id) })
  api.acquire.mockResolvedValue({ session_id: 'old', expires_in: 300 })
  api.heartbeat.mockResolvedValue({ session_id: 'old', expires_in: 300 })
})
afterEach(() => { unmounts.splice(0).forEach(fn => fn()); vi.unstubAllGlobals() })
describe('体验会话异步归属', () => {
  it.each([true, false])('旧初始化成功不能覆盖新租约（已有租约=%s）', async lease => {
    const { auth, session } = mount(lease)
    const pending = deferred<DemoSessionInfo>()
    ;(lease ? api.heartbeat : api.acquire).mockReturnValueOnce(pending.promise)
    const initialize = session.initialize()
    auth.setAuth(login); auth.setDemoSession({ session_id: 'new', expires_in: 500 })
    pending.resolve({ session_id: 'old', expires_in: 1 }); await initialize
    expect(auth.demoSessionId).toBe('new'); expect(auth.demoSessionExpiresIn).toBe(500)
    expect(timers.size).toBe(0)
  })
  it.each([true, false])('旧初始化失败不能清理新登录（已有租约=%s）', async lease => {
    const { auth, session } = mount(lease)
    const pending = deferred<DemoSessionInfo>()
    ;(lease ? api.heartbeat : api.acquire).mockReturnValueOnce(pending.promise)
    const initialize = session.initialize()
    auth.setAuth(login); auth.setDemoSession({ session_id: 'new', expires_in: 300 })
    pending.reject(new Error('old failure')); await initialize
    expect(auth.demoSessionId).toBe('new'); expect(auth.isLoggedIn).toBe(true)
    expect(api.push).not.toHaveBeenCalled()
  })
  it('离页后的初始化不能重新启动计时器', async () => {
    const { session, unmount } = mount()
    const pending = deferred<DemoSessionInfo>(); api.heartbeat.mockReturnValueOnce(pending.promise)
    const initialize = session.initialize(); unmount()
    pending.resolve({ session_id: 'old', expires_in: 300 }); await initialize
    expect(timers.size).toBe(0)
  })
  it('主动释放晚到不能退出新会话', async () => {
    const { auth, session } = mount()
    const pending = deferred<void>(); api.release.mockReturnValueOnce(pending.promise)
    const releasing = session.release()
    auth.setAuth(login); auth.setDemoSession({ session_id: 'new', expires_in: 300 })
    pending.resolve(); await releasing
    expect(auth.demoSessionId).toBe('new'); expect(api.closed).not.toHaveBeenCalled()
    expect(api.push).not.toHaveBeenCalled()
  })
  it('正常初始化启动计时器；身份变化停止旧定时任务，旧组件不得退出新身份', async () => {
    const { auth, session } = mount(false)
    await session.initialize(); expect(timers.size).toBe(2)
    auth.setAuth(login); await flush(); expect(timers.size).toBe(0)
    await session.release(); expect(api.release).not.toHaveBeenCalled()
    expect(auth.isLoggedIn).toBe(true)
  })
  it('正常主动释放清理自己的登录并跳转', async () => {
    const { auth, session } = mount()
    await session.initialize(); await session.release()
    expect(auth.isLoggedIn).toBe(false); expect(timers.size).toBe(0)
    expect(api.closed).toHaveBeenCalledOnce(); expect(api.push).toHaveBeenCalledWith('/login')
  })
  it('空闲释放在途换账号，不退出新会话', async () => {
    const { auth, session } = mount()
    api.heartbeat.mockResolvedValueOnce({ session_id: 'old', expires_in: 1 })
    await session.initialize()
    const pending = deferred<void>(); api.release.mockReturnValueOnce(pending.promise)
    const countdown = [...timers.values()][0]!
    countdown(); expect(api.release).toHaveBeenCalledOnce()
    auth.setAuth(login); auth.setDemoSession({ session_id: 'new', expires_in: 300 })
    pending.resolve(); await flush()
    expect(auth.demoSessionId).toBe('new'); expect(api.push).not.toHaveBeenCalled()
    expect(api.closed).not.toHaveBeenCalled()
  })
  it('两个初始化回复乱序时，旧租约不能覆盖已经接受的新租约', async () => {
    const { auth, session } = mount()
    const first = deferred<DemoSessionInfo>(), second = deferred<DemoSessionInfo>()
    api.heartbeat.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const a = session.initialize(), b = session.initialize()
    second.resolve({ session_id: 'renewed', expires_in: 300 }); await b
    first.resolve({ session_id: 'old', expires_in: 1 }); await a
    expect(auth.demoSessionId).toBe('renewed'); expect(auth.demoSessionExpiresIn).toBe(300)
  })
})
