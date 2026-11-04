import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, nextTick, reactive, ssrContextKey, type Component } from 'vue'
import { createPinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { loadPending, removePending, upsertPending, type PendingOperation } from '@/utils/pendingOperations'
import type { InboundOrderDetailRow } from '@/api/types'
import ReceiveDialog from './ReceiveDialog.vue'
import PutawayDialog from './PutawayDialog.vue'
import PickDialog from './PickDialog.vue'

const mocks = vi.hoisted(() => ({ confirm: vi.fn(), close: vi.fn(), success: vi.fn(), warning: vi.fn(), read: vi.fn(), write: vi.fn() }))
const route = reactive({ fullPath: '/orders', params: {} })
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('element-plus', () => ({ ElMessageBox: { confirm: mocks.confirm, close: mocks.close }, ElMessage: { success: mocks.success, warning: mocks.warning, error: vi.fn() } }))
vi.mock('@/api/inbound', () => ({ getInboundOrder: mocks.read, receiveInbound: mocks.write, putawayInboundTask: mocks.write }))
vi.mock('@/api/outbound', () => ({ getOutboundOrder: mocks.read, claimPdaTask: mocks.write, pickPdaTask: mocks.write }))
vi.mock('@/api/scanning', () => ({ scanOrderSKU: vi.fn(), putawayLocations: vi.fn().mockResolvedValue({ list: [], total: 0 }) }))
vi.mock('@/components/common/ScanField.vue', () => ({ default: {} }))

const renderer = createRenderer<object, object>({
  patchProp() {}, insert() {}, remove() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
})
type State = {
  open(id: string): Promise<void>; visible: boolean; form: { task_id: string };
  discard(row?: InboundOrderDetailRow): Promise<void>;
  handleDiscardClaim(): Promise<void>; handleDiscardPick(): Promise<void>;
}
const identity = { userId: '1', sessionNamespace: '' }
const row = { id: '1', sku_id: '2', expected_qty: 10, received_qty: 0, batch_no: 'B' } as InboundOrderDetailRow
const cases = [
  { name: '收货', component: ReceiveDialog, scope: 'inbound.receive', payload: { qty: 1, defective_qty: 0, batch_no: 'B' }, discard: (s: State) => s.discard(row) },
  { name: '上架', component: PutawayDialog, scope: 'inbound.putaway', payload: { qty: 1, location_id: '3' }, discard: (s: State) => s.discard() },
  { name: '领取', component: PickDialog, scope: 'outbound.claim', payload: null, discard: (s: State) => s.handleDiscardClaim() },
  { name: '拣货', component: PickDialog, scope: 'outbound.pick', payload: { qty: 1, location_code: 'L', batch_no: 'B' }, discard: (s: State) => s.handleDiscardPick() },
]
const unmounts: (() => void)[] = []
function storage(): Storage {
  const map = new Map<string, string>()
  return { getItem: k => map.get(k) ?? null, setItem: (k, v) => { map.set(k, v) }, removeItem: k => { map.delete(k) }, clear: () => map.clear(), key: i => [...map.keys()][i] ?? null, get length() { return map.size } }
}
function login(auth: ReturnType<typeof useAuthStore>) { auth.setAuth({ token: 'same-token', user_id: '1', username: 'operator', nickname: '', roles: [], perms: [] }) }
function mount(component: Component) {
  const pinia = createPinia(), auth = useAuthStore(pinia); login(auth)
  const app = renderer.createApp({ ...component, render: () => null })
  app.use(pinia); app.provide(ssrContextKey, { modules: new Set<string>() }); app.mount({})
  const state = (app._instance as unknown as { setupState: State }).setupState
  let mounted = true
  const unmount = () => { if (mounted) { mounted = false; app.unmount() } }; unmounts.push(unmount)
  return { state, auth, unmount }
}
function deferred() { let resolve!: (value: string) => void; const promise = new Promise<string>(yes => { resolve = yes }); return { promise, resolve } }
beforeEach(() => {
  vi.resetAllMocks(); route.fullPath = '/orders'
  vi.stubGlobal('localStorage', storage()); vi.stubGlobal('sessionStorage', storage())
  vi.stubGlobal('window', { addEventListener() {}, removeEventListener() {} })
  vi.stubGlobal('document', { addEventListener() {}, removeEventListener() {} })
  mocks.confirm.mockResolvedValue('confirm')
})
afterEach(() => { unmounts.splice(0).forEach(fn => fn()); vi.unstubAllGlobals() })

describe.each(cases)('$name 放弃确认（真实组件与持久化）', ({ component, scope, payload, discard }) => {
  async function setup() {
    const mounted = mount(component)
    const original: PendingOperation = { schemaVersion: 1, ...identity, scope, objectId: '1', operationKey: 'original-key', businessPayload: payload, createdAt: new Date().toISOString(), state: 'uncertain' }
    expect(upsertPending(identity, localStorage, original).ok).toBe(true)
    const type = scope === 'inbound.putaway' ? 'PUTAWAY' : 'PICK'
    mocks.read.mockResolvedValue({ order: { order_no: 'ORDER-1' }, details: [row], tasks: [{ id: '1', sku_id: '2', task_type: type, status: 'IN_PROGRESS', target_qty: 10, done_qty: 0, location_code: 'L', batch_no: 'B' }] })
    await mounted.state.open('100'); await nextTick()
    return { ...mounted, original }
  }

  it('确认等待时记录被替换，旧确认不能删除新key或提示成功', async () => {
    const { state, original } = await setup(), gate = deferred()
    mocks.confirm.mockReturnValueOnce(gate.promise)
    const action = discard(state)
    expect(mocks.confirm).toHaveBeenCalledOnce()
    removePending(identity, localStorage, scope, '1', original.operationKey)
    const replacement = { ...original, operationKey: 'new-key' }
    upsertPending(identity, localStorage, replacement)
    // 不派发storage事件，模拟旧页尚未收到跨标签通知。
    gate.resolve('confirm'); await action
    expect(loadPending(identity, localStorage).items).toEqual([replacement])
    expect(mocks.success).not.toHaveBeenCalled(); expect(mocks.write).not.toHaveBeenCalled()
    expect(mocks.warning).toHaveBeenCalledWith('记录已变化或操作仍在进行，请重新核对')
  })

  it.each(['close', 'unmount', 'relogin', 'route'] as const)('%s 后旧确认不能清理记录', async change => {
    const { state, auth, unmount, original } = await setup(), gate = deferred()
    mocks.confirm.mockReturnValueOnce(gate.promise)
    const action = discard(state)
    if (change === 'close') { state.visible = false; state.visible = true }
    if (change === 'unmount') unmount()
    if (change === 'relogin') login(auth)
    if (change === 'route') route.fullPath = '/other'
    gate.resolve('confirm'); await action
    expect(loadPending(identity, localStorage).items).toEqual([original])
    expect(mocks.success).not.toHaveBeenCalled(); expect(mocks.write).not.toHaveBeenCalled()
    expect(mocks.close).toHaveBeenCalledOnce()
  })

  it('正常确认只清除原记录，不发送业务请求', async () => {
    const { state } = await setup()
    await discard(state)
    expect(loadPending(identity, localStorage).items).toHaveLength(0)
    expect(mocks.success).toHaveBeenCalledOnce(); expect(mocks.write).not.toHaveBeenCalled()
  })

  it('本地写入失败时保留记录且不谎报放弃成功', async () => {
    const { state, original } = await setup()
    vi.spyOn(localStorage, 'setItem').mockImplementation(() => { throw new Error('QuotaExceededError') })
    await discard(state)
    expect(loadPending(identity, localStorage).items).toEqual([original])
    expect(mocks.success).not.toHaveBeenCalled(); expect(mocks.write).not.toHaveBeenCalled()
  })
})
