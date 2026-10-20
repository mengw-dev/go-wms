import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, type EffectScope } from 'vue'
import { listLocations, listSkus, listWarehouses } from '@/api/basic'
import { getInboundOrder, listInboundOrders } from '@/api/inbound'
import { getOutboundOrder, listOutboundOrders } from '@/api/outbound'
import { listInventoryTrans } from '@/api/inventory'
import { executeStagedRequest, type StagedRequest, type StagedRequestKind } from '@/api/demoExecution'
import { useAuthStore } from '@/stores/auth'
import { loadStagedDemo, STAGED_RETENTION_MS } from '@/utils/stagedDemoState'
import { useStagedDemoRunner } from './useStagedDemoRunner'

vi.mock('@/api/basic', () => ({ listLocations: vi.fn(), listSkus: vi.fn(), listWarehouses: vi.fn() }))
vi.mock('@/api/inbound', () => ({ getInboundOrder: vi.fn(), listInboundOrders: vi.fn() }))
vi.mock('@/api/outbound', () => ({ getOutboundOrder: vi.fn(), listOutboundOrders: vi.fn() }))
vi.mock('@/api/inventory', () => ({ listInventoryTrans: vi.fn() }))
vi.mock('@/api/demoExecution', () => ({ executeStagedRequest: vi.fn() }))
vi.mock('@/utils/demoEvidence', () => ({ rememberDemoExecutionWindow: vi.fn() }))

function storage(): Storage {
  const entries = new Map<string, string>()
  return { getItem: (key) => entries.get(key) ?? null, setItem: (key, value) => { entries.set(key, value) },
    removeItem: (key) => { entries.delete(key) }, clear: () => entries.clear(), key: (index) => [...entries.keys()][index] ?? null,
    get length() { return entries.size } }
}
const identity = { userId: '1', sessionId: 'demo-session' }
const scopes: EffectScope[] = []
function createRunner() {
  const scope = effectScope(); scopes.push(scope)
  return { runner: scope.run(() => useStagedDemoRunner())!, stop: () => scope.stop() }
}
function login(id = '1', session = 'demo-session') {
  const auth = useAuthStore()
  auth.setAuth({ token: 'token', user_id: id, username: id, nickname: id, roles: [], perms: ['wms:demo'] })
  auth.setDemoSession({ session_id: session, expires_in: 300 })
}
function checkpoint() { return loadStagedDemo(identity).snapshot! }

let incoming: { id: string; order_no: string; warehouse_id: string; remark: unknown; status: string } | null
let outgoing: typeof incoming
let received: number, putaway: number
let picked: Map<string, number>, replayed: Set<string>
let lose: StagedRequestKind | '', noCommit: boolean, emptyLocations: boolean, createdZone: string
let calls: { request: StagedRequest; key: string }[]

beforeEach(() => {
  vi.resetAllMocks()
  vi.stubGlobal('localStorage', storage()); vi.stubGlobal('sessionStorage', storage())
  setActivePinia(createPinia()); login()
  incoming = null; outgoing = null; received = 0; putaway = 0; picked = new Map(); replayed = new Set()
  lose = ''; noCommit = false; emptyLocations = false; createdZone = ''; calls = []
  vi.mocked(listWarehouses).mockResolvedValue({ list: [{ id: '10', code: 'WH01' }], total: 1 } as never)
  vi.mocked(listSkus).mockResolvedValue({ list: [{ id: '20', code: 'SKU000001' }], total: 1 } as never)
  vi.mocked(listLocations).mockImplementation(async () => ({ list: emptyLocations && !createdZone ? [] : [{ id: '30', code: createdZone ? `${createdZone}-01-01` : 'A01-01-01' }], total: 1 }) as never)
  vi.mocked(listInboundOrders).mockImplementation(async () => ({ list: incoming ? [incoming] : [], total: incoming ? 1 : 0 }) as never)
  vi.mocked(listOutboundOrders).mockImplementation(async () => ({ list: outgoing ? [outgoing] : [], total: outgoing ? 1 : 0 }) as never)
  vi.mocked(getInboundOrder).mockImplementation(async () => ({ order: { ...incoming }, details: [{ id: '101', sku_id: '20', expected_qty: 10, received_qty: received }],
    tasks: received ? [{ id: '102', task_type: 'PUTAWAY', done_qty: putaway, target_qty: 10, status: putaway === 10 ? 'COMPLETED' : 'PENDING' }] : [] }) as never)
  vi.mocked(getOutboundOrder).mockImplementation(async () => ({ order: { ...outgoing }, details: [{ sku_id: '20', expected_qty: 20 }],
    tasks: [['201', 7], ['202', 13]].map(([id, qty]) => ({ id, task_type: 'PICK', done_qty: picked.get(String(id)) || 0,
      target_qty: qty, status: picked.get(String(id)) === qty ? 'COMPLETED' : 'PENDING', location_code: 'A01-01-01', batch_no: 'B1' })) }) as never)
  vi.mocked(listInventoryTrans).mockResolvedValue({ list: [{ quantity_change: 10 }], total: 1 } as never)
  vi.mocked(executeStagedRequest).mockImplementation(async (request, key) => {
    calls.push({ request: structuredClone(request), key })
    // A real backend deduplicates a committed quantity request by its original key.
    if (replayed.has(key)) return null
    if (noCommit && lose === request.kind) { lose = ''; throw new Error('network before dispatch') }
    let result: unknown = null
    switch (request.kind) {
      case 'inbound-create': incoming = { id: '100', order_no: 'RK100', warehouse_id: '10', remark: request.payload.remark, status: 'DRAFT' }; result = incoming; break
      case 'inbound-submit': incoming!.status = 'SUBMITTED'; break
      case 'inbound-approve': incoming!.status = 'APPROVED'; break
      case 'receive': received += Number(request.payload.qty); incoming!.status = 'PUTAWAY'; break
      case 'putaway': putaway += Number(request.payload.qty); incoming!.status = 'COMPLETED'; break
      case 'outbound-create': outgoing = { id: '200', order_no: 'CK200', warehouse_id: '10', remark: request.payload.remark, status: 'DRAFT', ...{ biz_order_no: request.payload.biz_order_no } }; result = outgoing; break
      case 'outbound-submit': outgoing!.status = 'SUBMITTED'; break
      case 'outbound-approve': outgoing!.status = 'PICKING'; break
      case 'pick': picked.set(request.objectId, (picked.get(request.objectId) || 0) + Number(request.payload.qty)); if (picked.size === 2) outgoing!.status = 'SHIPPED'; break
      case 'location-create': createdZone = String(request.payload.zone); break
    }
    replayed.add(key)
    if (lose === request.kind) { lose = ''; throw new Error('response lost after commit') }
    return result
  })
})

afterEach(() => { scopes.splice(0).forEach((scope) => scope.stop()); vi.useRealTimers(); vi.unstubAllGlobals() })

async function advance(runner: ReturnType<typeof useStagedDemoRunner>, index: number) {
  while (runner.currentIndex.value < index) {
    const before = runner.currentIndex.value
    await runner.executeNext()
    expect(runner.error.value).toBe('')
    expect(runner.currentIndex.value).toBe(before + 1)
  }
}

describe('分步演示的持久化与请求恢复', () => {
  it.each([['receive', 3], ['putaway', 4], ['pick', 9]] as const)('%s 提交成功丢响应，刷新后终态也以原 key 和参数回放', async (kind, index) => {
    const first = createRunner(); first.runner.start()
    await advance(first.runner, index)
    lose = kind
    await first.runner.executeNext()
    expect(first.runner.error.value).toContain('response lost')
    const original = calls.at(-1)!
    expect(checkpoint().writes.at(-1)?.status).toBe('sending')
    first.stop()
    const restored = createRunner().runner
    expect(restored.paused.value).toBe(true)
    expect(restored.currentIndex.value).toBe(index)
    const before = calls.length
    await restored.executeNext()
    expect(restored.error.value).toBe('')
    expect(calls[before]).toEqual(original)
    await advance(restored, 10)
    expect(received).toBe(10); expect(putaway).toBe(10)
    expect([...picked.values()].reduce((sum, qty) => sum + qty, 0)).toBe(20)
    expect(restored.completed.value).toBe(true)
    expect(restored.lastResult.value?.quantity).toBe('-20 件库存')
    expect(checkpoint().writes.every((write) => write.status === 'done')).toBe(true)
  })

  it.each(['inbound-create', 'outbound-create', 'inbound-submit', 'inbound-approve', 'outbound-submit', 'outbound-approve'] as const)('%s 丢响应后只读核对，不重发无 key 的写请求', async (kind) => {
    const index = { 'inbound-create': 0, 'inbound-submit': 1, 'inbound-approve': 2, 'outbound-create': 6, 'outbound-submit': 7, 'outbound-approve': 8 }[kind]
    const first = createRunner(); first.runner.start(); await advance(first.runner, index)
    lose = kind; await first.runner.executeNext(); first.stop()
    const restored = createRunner().runner
    const before = calls.length
    await restored.executeNext()
    expect(restored.error.value).toBe('')
    expect(restored.currentIndex.value).toBe(index + 1)
    expect(calls).toHaveLength(before)
  })

  it('无法确认建单是否执行时保留原意图，恢复和重新开始均不能另建单', async () => {
    const { runner } = createRunner(); runner.start(); lose = 'inbound-create'; noCommit = true
    await runner.executeNext()
    const intent = checkpoint().writes[0]
    await runner.executeNext()
    expect(runner.error.value).toContain('未重新发送')
    runner.start('auto')
    expect(runner.error.value).toContain('尚未完成')
    expect(calls).toHaveLength(1)
    expect(checkpoint().writes[0]).toEqual(intent)
  })

  it('多任务拣货最后一项丢响应，恢复已发货订单时不会再拣前一项', async () => {
    const { runner } = createRunner(); runner.start(); await advance(runner, 9)
    const backend = vi.mocked(executeStagedRequest).getMockImplementation()!
    let lost = false
    vi.mocked(executeStagedRequest).mockImplementation((request, key, signal) => {
      if (request.kind === 'pick' && request.objectId === '202' && !lost) { lose = 'pick'; lost = true }
      return backend(request, key, signal)
    })
    await runner.executeNext()
    expect(outgoing!.status).toBe('SHIPPED')
    await runner.executeNext()
    expect(runner.completed.value).toBe(true)
    expect(calls.filter(({ request }) => request.kind === 'pick' && request.objectId === '201')).toHaveLength(1)
    const final = calls.filter(({ request }) => request.kind === 'pick' && request.objectId === '202')
    expect(final).toHaveLength(2); expect(final[0]).toEqual(final[1])
    expect(runner.lastResult.value?.quantity).toBe('-20 件库存')
  })

  it('多个同标记单据不能凭任意匹配确认建单成功', async () => {
    const { runner } = createRunner(); runner.start(); lose = 'inbound-create'; await runner.executeNext()
    vi.mocked(listInboundOrders).mockResolvedValue({ list: [incoming, { ...incoming, id: '999' }], total: 2 } as never)
    await runner.executeNext()
    expect(runner.error.value).toContain('未重新发送'); expect(calls).toHaveLength(1)
  })

  it('没有空闲库位时，丢响应后的批量建库位也按原区码只读确认', async () => {
    emptyLocations = true
    const { runner } = createRunner(); runner.start(); await advance(runner, 4)
    lose = 'location-create'; await runner.executeNext()
    expect(runner.error.value).toContain('response lost')
    await runner.executeNext()
    expect(runner.error.value).toBe('')
    expect(calls.filter(({ request }) => request.kind === 'location-create')).toHaveLength(1)
    expect(putaway).toBe(10)
  })

  it('存储写失败时先停止，不发送业务请求', async () => {
    const { runner } = createRunner(); runner.start()
    vi.spyOn(sessionStorage, 'setItem').mockImplementation(() => { throw new Error('quota') })
    await runner.executeNext()
    expect(runner.error.value).toContain('无法保存'); expect(calls).toHaveLength(0)
  })

  it('暂停时的迟到查询不能发起后续写入', async () => {
    let resolve!: (value: never) => void
    vi.mocked(listWarehouses).mockImplementationOnce(() => new Promise((done) => { resolve = done }))
    const { runner } = createRunner(); runner.start()
    const pending = runner.executeNext()
    runner.pause()
    resolve({ list: [{ id: '10', code: 'WH01' }], total: 1 } as never)
    await pending
    expect(calls).toHaveLength(0)
    expect(runner.currentIndex.value).toBe(0)
    expect(runner.paused.value).toBe(true)
  })

  it('恢复记录中的数量或步骤被损坏时禁止执行', async () => {
    const first = createRunner(); first.runner.start(); await advance(first.runner, 3)
    lose = 'receive'; await first.runner.executeNext(); first.stop()
    const storedKey = Array.from({ length: sessionStorage.length }, (_, i) => sessionStorage.key(i)!).find((key) => key.startsWith('WMS_STAGED_DEMO'))!
    const saved = checkpoint()
    saved.writes.at(-1)!.payload.qty = -10
    sessionStorage.setItem(storedKey, JSON.stringify(saved))
    const before = calls.length
    const { runner } = createRunner(); runner.start(); await runner.executeNext()
    expect(runner.error.value).toContain('损坏'); expect(calls).toHaveLength(before)
  })

  it('损坏记录不静默覆盖，超过七天不自动重发', async () => {
    const first = createRunner(); first.runner.start(); lose = 'receive'; await advance(first.runner, 3); await first.runner.executeNext(); first.stop()
    const storedKey = Array.from({ length: sessionStorage.length }, (_, i) => sessionStorage.key(i)!).find((key) => key.startsWith('WMS_STAGED_DEMO'))!
    const saved = sessionStorage.getItem(storedKey)!
    sessionStorage.setItem(storedKey, '{invalid')
    const broken = createRunner(); broken.runner.start('auto'); await broken.runner.executeNext()
    expect(broken.runner.error.value).toContain('损坏'); expect(sessionStorage.getItem(storedKey)).toBe('{invalid'); broken.stop()
    sessionStorage.setItem(storedKey, saved)
    const before = calls.length
    vi.useFakeTimers(); vi.setSystemTime(Date.now() + STAGED_RETENTION_MS + 1)
    const expired = createRunner().runner; await expired.executeNext()
    expect(expired.error.value).toContain('恢复窗口'); expect(calls).toHaveLength(before)
  })

  it('自动间隔暂停或销毁后不再发出下一步，刷新也不会自动运行', async () => {
    vi.useFakeTimers()
    const first = createRunner(); first.runner.start('auto')
    await vi.advanceTimersByTimeAsync(0)
    expect(first.runner.currentIndex.value).toBe(1)
    first.runner.pause(); await vi.advanceTimersByTimeAsync(5000)
    expect(calls).toHaveLength(1)
    first.runner.resume(); await vi.advanceTimersByTimeAsync(0)
    expect(first.runner.currentIndex.value).toBe(2)
    first.stop(); await vi.advanceTimersByTimeAsync(5000)
    const restored = createRunner().runner
    await vi.advanceTimersByTimeAsync(5000)
    expect(restored.paused.value).toBe(true); expect(calls).toHaveLength(2)
  })

  it('双击只发一次，切换账号取消旧等待且晚响应不能写进新账号进度', async () => {
    let resolve!: (value: unknown) => void
    vi.mocked(executeStagedRequest).mockImplementationOnce(() => new Promise((done) => { resolve = done }))
    const { runner } = createRunner(); runner.start()
    const pending = runner.executeNext()
    await vi.waitFor(() => expect(executeStagedRequest).toHaveBeenCalledTimes(1))
    await runner.executeNext(); expect(executeStagedRequest).toHaveBeenCalledTimes(1)
    const signal = vi.mocked(executeStagedRequest).mock.calls[0]![2]
    login('2', 'other-session')
    expect(signal.aborted).toBe(true)
    resolve({ id: '100', order_no: 'RK100' }); await pending
    expect(runner.started.value).toBe(false); expect(runner.results.value).toHaveLength(0)
    expect(checkpoint().writes[0]?.status).toBe('sending')
    expect(loadStagedDemo({ userId: '2', sessionId: 'other-session' }).snapshot).toBeNull()
  })

  it('续期不打断演示，完整运行后新演示使用新标记与新操作键', async () => {
    const { runner } = createRunner(); runner.start(); await advance(runner, 1)
    useAuthStore().setDemoSession({ session_id: identity.sessionId, expires_in: 300 })
    expect(runner.paused.value).toBe(false)
    const oldKey = calls[0]!.key, oldRun = checkpoint().runId
    await advance(runner, 10)
    runner.start(); await runner.executeNext()
    expect(checkpoint().runId).not.toBe(oldRun); expect(calls.at(-1)!.key).not.toBe(oldKey)
  })
})
