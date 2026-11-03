import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, nextTick, reactive, ssrContextKey } from 'vue'
import { createPinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import DemoConsole from './DemoConsole.vue'

const mocks = vi.hoisted(() => ({ confirm: vi.fn(), close: vi.fn(), reset: vi.fn(), release: vi.fn(), clearStaged: vi.fn(), clearResult: vi.fn(), changed: vi.fn() }))
const navigation = reactive({ path: '/demo', fullPath: '/demo' })
vi.mock('vue-router', () => ({ useRoute: () => navigation, useRouter: () => ({ push: vi.fn() }) }))
vi.mock('element-plus', () => ({ ElMessageBox: { confirm: mocks.confirm, close: mocks.close }, ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }))
vi.mock('@/api/demo', () => ({ resetDemoData: mocks.reset }))
vi.mock('@/components/demo/DemoRunViewer.vue', () => ({ default: {} }))
vi.mock('@/components/demo/StagedDemoRunner.vue', () => ({ default: {} }))
vi.mock('@/composables/demo/useDemoDock', () => ({ useDemoDock: () => ({}) }))
vi.mock('@/composables/demo/useDemoSession', async () => {
  const { ref } = await import('vue')
  return { useDemoSession: () => ({ acquiring: ref(false), exiting: ref(false), remaining: ref(300), idleTimeoutMinutes: ref(5), initialize: vi.fn(), release: mocks.release }) }
})
vi.mock('@/composables/demo/useDemoRunner', async () => {
  const { ref } = await import('vue')
  return { useDemoRunner: () => ({ scenarioRunning: ref(false), runningScenario: ref(null), result: ref(null), resultDialogVisible: ref(false), runScenario: vi.fn(), clearResult: mocks.clearResult }) }
})
vi.mock('@/stores/guide', () => ({ GUIDE_SCENARIO_LABELS: {}, useGuideStore: () => ({ active: false, completed: false, cancel: vi.fn() }) }))
vi.mock('@/utils/stagedDemoState', () => ({ clearStagedDemo: mocks.clearStaged, executionSteps: () => [], loadStagedDemo: () => ({ snapshot: null, error: '' }) }))
vi.mock('@/utils/events', () => ({ emitDataChanged: mocks.changed, OPEN_DEMO_CONSOLE_EVENT: 'open-demo', RUN_DEMO_SCENARIO_EVENT: 'run-demo', RUN_DEMO_STAGED_EVENT: 'staged-demo' }))
const renderer = createRenderer<object, object>({
  patchProp() {}, insert() {}, remove() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
})
type State = { resetData(): Promise<void>; releaseAndExit(): Promise<void> }
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
function login(auth: ReturnType<typeof useAuthStore>) {
  auth.setAuth({ token: 'same', user_id: '2', username: 'demo', nickname: '', roles: [], perms: ['wms:demo'] })
  auth.setDemoSession({ session_id: 'same-lease', expires_in: 300 })
}
function mount() {
  const pinia = createPinia(), auth = useAuthStore(pinia); login(auth)
  const app = renderer.createApp({ ...DemoConsole, render: () => null })
  app.use(pinia); app.provide(ssrContextKey, { modules: new Set<string>() }); app.mount({})
  const state = (app._instance as unknown as { setupState: State }).setupState
  const unmount = () => app.unmount(); unmounts.push(unmount)
  return { state, auth, unmount }
}
beforeEach(() => {
  vi.resetAllMocks(); navigation.path = '/demo'; navigation.fullPath = '/demo'
  vi.stubGlobal('localStorage', storage()); vi.stubGlobal('sessionStorage', storage())
  vi.stubGlobal('window', { addEventListener() {}, removeEventListener() {} })
  mocks.confirm.mockResolvedValue('confirm'); mocks.reset.mockResolvedValue(undefined)
})
afterEach(() => { unmounts.splice(0).forEach(fn => fn()); vi.unstubAllGlobals() })
describe('Demo重置与退出确认归属（真实SFC）', () => {
  it.each(['resetData', 'releaseAndExit'] as const)('%s 确认等待期间相同身份重新登录也取消旧操作', async action => {
    const { state, auth } = mount(), gate = deferred<string>()
    mocks.confirm.mockReturnValueOnce(gate.promise)
    const pending = state[action](); login(auth); gate.resolve('confirm'); await pending
    expect(mocks.reset).not.toHaveBeenCalled(); expect(mocks.release).not.toHaveBeenCalled()
    expect(mocks.close).toHaveBeenCalledOnce()
  })
  it.each(['resetData', 'releaseAndExit'] as const)('%s 离页晚确认不能发请求', async action => {
    const { state, unmount } = mount(), gate = deferred<string>()
    mocks.confirm.mockReturnValueOnce(gate.promise)
    const pending = state[action](); unmount(); gate.resolve('confirm'); await pending
    expect(mocks.reset).not.toHaveBeenCalled(); expect(mocks.release).not.toHaveBeenCalled()
  })
  it('重置回复晚到不能清理新会话的演示进度', async () => {
    const { state, auth } = mount(), gate = deferred<void>()
    mocks.reset.mockReturnValueOnce(gate.promise)
    const pending = state.resetData(); await flush()
    expect(mocks.reset).toHaveBeenCalledOnce()
    login(auth); gate.resolve(); await pending
    expect(mocks.clearStaged).not.toHaveBeenCalled(); expect(mocks.clearResult).not.toHaveBeenCalled()
    expect(mocks.changed).not.toHaveBeenCalled()
  })
  it('正常确认仍执行且重置在途不能重复发起', async () => {
    const { state } = mount(), gate = deferred<void>()
    mocks.reset.mockReturnValueOnce(gate.promise)
    const pending = state.resetData(); await flush(); await state.resetData()
    expect(mocks.reset).toHaveBeenCalledOnce()
    gate.resolve(); await pending
    expect(mocks.clearStaged).toHaveBeenCalledWith({ userId: '2', sessionId: 'same-lease' })
    expect(mocks.changed).toHaveBeenCalledOnce()
    await state.releaseAndExit(); expect(mocks.release).toHaveBeenCalledOnce()
  })
})
