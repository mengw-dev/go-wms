import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, nextTick, ssrContextKey } from 'vue'
import { createPinia } from 'pinia'
import type { LoginResult, ProfileResult } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import Layout from './Layout.vue'

const mocks = vi.hoisted(() => ({ getProfile: vi.fn(), changePassword: vi.fn(), release: vi.fn(), confirm: vi.fn(), push: vi.fn() }))
vi.mock('@/api/auth', () => ({ getProfile: mocks.getProfile, changePassword: mocks.changePassword }))
vi.mock('@/api/demo', () => ({ releaseDemoSession: mocks.release }))
vi.mock('@/components/DemoConsole.vue', () => ({ default: {} }))
vi.mock('@/components/demo/ManualGuide.vue', () => ({ default: {} }))
vi.mock('./WmsNavigation.vue', () => ({ default: {} }))
vi.mock('vue-router', () => ({ useRoute: () => ({ path: '/', meta: {} }), useRouter: () => ({ push: mocks.push }) }))
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn() }, ElMessageBox: { confirm: mocks.confirm } }))
const renderer = createRenderer<object, object>({
  patchProp() {}, insert() {}, remove() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
})
type State = { onCommand(command: string): Promise<void>; submitPassword(): Promise<void>; pwdFormRef: { validate(): Promise<boolean> } }
const unmounts: (() => void)[] = []
const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); await nextTick() }
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(yes => { resolve = yes })
  return { promise, resolve }
}
function storage(): Storage {
  const map = new Map<string, string>()
  return { getItem: key => map.get(key) ?? null, setItem: (key, value) => { map.set(key, value) }, removeItem: key => { map.delete(key) } } as Storage
}
const login = (id = '2', demo = false): LoginResult => ({ token: `token-${id}`, user_id: id, username: `user-${id}`, nickname: 'current', roles: [], perms: demo ? ['wms:demo'] : [] })
async function mount(demo = false) {
  const pinia = createPinia(), auth = useAuthStore(pinia)
  auth.setAuth(login('2', demo))
  const app = renderer.createApp({ ...Layout, render: () => null })
  app.use(pinia); app.provide(ssrContextKey, { modules: new Set<string>() }); app.mount({})
  const state = (app._instance as unknown as { setupState: State }).setupState
  const unmount = () => app.unmount(); unmounts.push(unmount)
  state.pwdFormRef = { validate: async () => true }
  await flush()
  return { state, auth, unmount }
}
beforeEach(() => {
  vi.resetAllMocks()
  vi.stubGlobal('localStorage', storage()); vi.stubGlobal('sessionStorage', storage())
  vi.stubGlobal('window', { matchMedia: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }) })
  mocks.getProfile.mockResolvedValue(null); mocks.confirm.mockResolvedValue(true)
})
afterEach(() => { unmounts.splice(0).forEach(fn => fn()); vi.unstubAllGlobals() })

describe('Layout 认证操作归属（真实 SFC）', () => {
  it.each(['switch', 'same-token', 'ABA', 'unmount'] as const)('旧资料晚到被忽略：%s', async mode => {
    const old = deferred<ProfileResult>(); mocks.getProfile.mockReturnValueOnce(old.promise)
    const { auth, unmount } = await mount()
    if (mode === 'unmount') unmount()
    else if (mode === 'switch') auth.setAuth(login('3'))
    else { if (mode === 'ABA') auth.setAuth(login('3')); auth.setAuth(login()) }
    const token = auth.token, before = useAuthStore(createPinia()).user
    old.resolve({ ...login(), nickname: 'stale', perms: ['*'] }); await flush()
    expect(auth.token).toBe(token); expect(auth.user?.nickname).toBe('current')
    expect(auth.perms).toEqual([]); expect(useAuthStore(createPinia()).user).toEqual(before)
  })
  it('有效资料更新昵称与权限，返回其他用户则拒绝', async () => {
    const profile = deferred<ProfileResult>(); mocks.getProfile.mockReturnValueOnce(profile.promise)
    const { auth } = await mount()
    profile.resolve({ ...login(), nickname: 'fresh', perms: ['wms:inbound:view'] }); await flush()
    expect(auth.user?.nickname).toBe('fresh'); expect(auth.hasPerm('wms:inbound:view')).toBe(true)
    expect(auth.setProfile(login('3'), auth.snapshot())).toBe(false)
    expect(auth.user?.user_id).toBe('2')
  })
  it('退出确认期间切换身份不能退出新账号，也不能释放新租约', async () => {
    const { state, auth } = await mount(true)
    const confirmation = deferred<boolean>(); mocks.confirm.mockReturnValueOnce(confirmation.promise)
    const pending = state.onCommand('logout')
    auth.setAuth(login('3', true)); confirmation.resolve(true); await pending
    expect(mocks.release).not.toHaveBeenCalled(); expect(auth.user?.user_id).toBe('3')
    expect(mocks.push).not.toHaveBeenCalled()
  })
  it('体验释放晚到后不能清除新会话', async () => {
    const { state, auth } = await mount(true)
    const release = deferred<void>(); mocks.release.mockReturnValueOnce(release.promise)
    const pending = state.onCommand('logout'); await flush()
    auth.setAuth(login('3')); release.resolve(); await pending
    expect(auth.user?.user_id).toBe('3'); expect(mocks.push).not.toHaveBeenCalled()
  })
  it('改密成功晚到不能清除之后的新登录', async () => {
    const { state, auth } = await mount()
    const change = deferred<void>(); mocks.changePassword.mockReturnValueOnce(change.promise)
    await state.onCommand('password'); const pending = state.submitPassword(); await flush()
    auth.setAuth(login()); change.resolve(); await pending
    expect(auth.isLoggedIn).toBe(true); expect(mocks.push).not.toHaveBeenCalled()
  })
  it('旧密码弹窗不能把旧表单作为新身份提交；正常改密仍退出', async () => {
    const { state, auth } = await mount()
    await state.onCommand('password'); auth.setAuth(login('3')); await state.submitPassword()
    expect(mocks.changePassword).not.toHaveBeenCalled()
    await state.onCommand('password'); await state.submitPassword()
    expect(mocks.changePassword).toHaveBeenCalledOnce(); expect(auth.isLoggedIn).toBe(false)
    expect(mocks.push).toHaveBeenCalledWith('/login')
  })
})
