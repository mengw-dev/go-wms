import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, nextTick, ssrContextKey } from 'vue'
import { createPinia } from 'pinia'
import type { DemoSessionInfo, LoginResult } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import Login from './index.vue'

const api = vi.hoisted(() => ({
  login: vi.fn(), acquireDemoSession: vi.fn(), claimDemoAccount: vi.fn(),
  claimPersonalAccount: vi.fn(), listPersonalAccounts: vi.fn(), getVersion: vi.fn(), push: vi.fn(),
}))
vi.mock('@/api/auth', () => ({ login: api.login }))
vi.mock('@/api/demo', () => ({ acquireDemoSession: api.acquireDemoSession, claimDemoAccount: api.claimDemoAccount }))
vi.mock('@/api/personal', () => ({ claimPersonalAccount: api.claimPersonalAccount, listPersonalAccounts: api.listPersonalAccounts }))
vi.mock('@/api/version', () => ({ getVersion: api.getVersion }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }), useRouter: () => ({ push: api.push }) }))
vi.mock('element-plus', () => ({ ElMessage: Object.assign(vi.fn(), { success: vi.fn() }) }))

const renderer = createRenderer<object, object>({
  patchProp() {}, insert() {}, remove() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
})
type State = {
  submit(): Promise<void>; startDemo(): Promise<void>; startPersonal(): Promise<void>
  formRef: { validate(): Promise<boolean> }; selectedAccount: string; busy: boolean
}
const unmounts: (() => void)[] = []
const flush = async () => { for (let i = 0; i < 10; i++) await Promise.resolve(); await nextTick() }
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function storage(): Storage {
  const map = new Map<string, string>()
  return { getItem: key => map.get(key) ?? null, setItem: (key, value) => { map.set(key, value) }, removeItem: key => { map.delete(key) } } as Storage
}
const result = (demo = false): LoginResult => ({ token: 'same-token', user_id: '2', username: 'operator', nickname: 'current', roles: [], perms: demo ? ['wms:demo'] : [] })
async function mount() {
  const pinia = createPinia()
  const app = renderer.createApp({ ...Login, render: () => null })
  app.use(pinia); app.provide(ssrContextKey, { modules: new Set<string>() }); app.mount({})
  const state = (app._instance as unknown as { setupState: State }).setupState
  const unmount = () => app.unmount()
  unmounts.push(unmount)
  state.formRef = { validate: async () => true }; state.selectedAccount = 'user1'
  await flush()
  return { state, auth: useAuthStore(pinia), unmount }
}
beforeEach(() => {
  vi.resetAllMocks()
  vi.stubGlobal('localStorage', storage()); vi.stubGlobal('sessionStorage', storage())
  api.getVersion.mockResolvedValue({ demo_enabled: true, personal_enabled: true })
  api.listPersonalAccounts.mockResolvedValue([])
  api.claimDemoAccount.mockResolvedValue({ username: 'demo', password: 'secret', tenant_id: '2', total: 10 })
  api.claimPersonalAccount.mockResolvedValue({ username: 'user1', password: 'secret', tenant_id: '3' })
  api.login.mockResolvedValue(result())
  api.acquireDemoSession.mockResolvedValue({ session_id: 'lease', expires_in: 300 })
})
afterEach(() => { unmounts.splice(0).forEach(fn => fn()); vi.unstubAllGlobals() })

describe('登录入口竞争（真实 SFC）', () => {
  it('验证表单开始就互斥，三种入口不能重叠', async () => {
    const { state } = await mount()
    const validation = deferred<boolean>()
    state.formRef = { validate: () => validation.promise }
    const pending = state.submit()
    await state.startDemo(); await state.startPersonal(); void state.submit()
    expect(api.claimDemoAccount).not.toHaveBeenCalled()
    expect(api.claimPersonalAccount).not.toHaveBeenCalled()
    expect(api.login).not.toHaveBeenCalled()
    validation.resolve(true); await pending; await flush()
    expect(api.login).toHaveBeenCalledTimes(1)
    expect(api.push).toHaveBeenCalledTimes(1)
  })

  it.each(['submit', 'startDemo', 'startPersonal'] as const)('%s 晚到失败不清除新登录', async entry => {
    const { state, auth } = await mount()
    const old = deferred<LoginResult>()
    api.login.mockReturnValueOnce(old.promise)
    const pending = state[entry](); await flush()
    auth.setAuth(result())
    old.reject(new Error('old failure')); await pending
    expect(auth.token).toBe('same-token')
    expect(localStorage.getItem('WMS_TOKEN')).toBe('same-token')
    expect(api.push).not.toHaveBeenCalled()
  })

  it.each(['submit', 'startDemo', 'startPersonal'] as const)('%s 离页后晚到成功不能重新登录或跳转', async entry => {
    const { state, auth, unmount } = await mount()
    const old = deferred<LoginResult>()
    api.login.mockReturnValueOnce(old.promise)
    const pending = state[entry](); await flush(); unmount()
    old.resolve(result()); await pending
    expect(auth.isLoggedIn).toBe(false)
    expect(api.push).not.toHaveBeenCalled()
  })

  it('旧体验租约晚到不能绑定到新会话，即使账号和 token 相同', async () => {
    const { state, auth } = await mount()
    const lease = deferred<DemoSessionInfo>()
    api.login.mockResolvedValue(result(true)); api.acquireDemoSession.mockReturnValueOnce(lease.promise)
    const pending = state.startDemo(); await flush()
    expect(api.acquireDemoSession).toHaveBeenCalledOnce()
    auth.setAuth(result(true)); auth.setDemoSession({ session_id: 'new', expires_in: 300 })
    lease.resolve({ session_id: 'old', expires_in: 100 }); await pending
    expect(auth.demoSessionId).toBe('new'); expect(api.push).not.toHaveBeenCalled()
  })

  it('本次领取失败清理本次临时登录；完成登录后卸载则保留', async () => {
    const { state, auth, unmount } = await mount()
    api.login.mockResolvedValue(result(true)); api.acquireDemoSession.mockRejectedValueOnce(new Error('full'))
    await state.startDemo(); expect(auth.isLoggedIn).toBe(false)
    api.login.mockResolvedValue(result())
    await state.submit(); unmount()
    expect(auth.isLoggedIn).toBe(true); expect(api.push).toHaveBeenCalledOnce()
  })
})
