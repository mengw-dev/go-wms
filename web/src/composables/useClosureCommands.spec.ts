import { effectScope, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { sendClosureCommand, type ClosureAck, type ClosureCommand } from '@/api/closure'
import { useAuthStore } from '@/stores/auth'
import { loadPending, PENDING_RETENTION_MS } from '@/utils/pendingOperations'
import { useClosureCommands } from './useClosureCommands'

vi.mock('@/api/closure', async original => ({ ...await original<typeof import('@/api/closure')>(), sendClosureCommand: vi.fn() }))
vi.mock('@/api/request', () => ({ get: vi.fn(), post: vi.fn() }))
function storage(): Storage {
  const data = new Map<string, string>()
  return { getItem: k => data.get(k) ?? null, setItem: (k,v) => { data.set(k,v) }, removeItem: k => { data.delete(k) }, clear: () => data.clear(), key: i => [...data.keys()][i] ?? null, get length() { return data.size } }
}
const scopes: ReturnType<typeof effectScope>[] = []
function setup() { const scope = effectScope(); scopes.push(scope); return { scope, flow: scope.run(useClosureCommands)! } }
function login(id = '1', demo = '') {
  const auth = useAuthStore()
  auth.setAuth({ token: `token-${id}`, user_id: id, username: 'picker', nickname: '', roles: [], perms: demo ? ['wms:demo'] : [] })
  if (demo) auth.setDemoSession({ session_id: demo, expires_in: 600 })
}
const command: ClosureCommand = { action: 'freeze', data: { inventory_id: '9223372036854775800', quantity: 2, unfreeze: false, reason: '质量待检' } }
function deferred() { let resolve!: (ack: ClosureAck) => void; const promise = new Promise<ClosureAck>(r => { resolve = r }); return { promise, resolve } }
beforeEach(() => { vi.resetAllMocks(); vi.stubGlobal('localStorage', storage()); vi.stubGlobal('sessionStorage', storage()); setActivePinia(createPinia()); login() })
afterEach(() => { scopes.splice(0).forEach(s => s.stop()); vi.unstubAllGlobals() })

describe('库存与异常命令持久化与恢复', () => {
  it('同会话续期保留进行中的命令，并接受原成功回执', async () => {
    login('1', 'session-a')
    const { flow } = setup(), request = deferred()
    vi.mocked(sendClosureCommand).mockReturnValueOnce(request.promise)
    const result = flow.run(command)
    useAuthStore().setDemoSession({ session_id: 'session-a', expires_in: 300 })
    await nextTick()
    expect(flow.busy.value).toBe(true)
    request.resolve({ inventory_id: '9223372036854775800' })
    expect(await result).toEqual({ inventory_id: '9223372036854775800' })
    expect(flow.pending.value).toBeNull()
  })
  it('成功才清除记录，ID不经过数字转换', async () => {
    const { flow } = setup(), request = deferred()
    vi.mocked(sendClosureCommand).mockReturnValueOnce(request.promise)
    const result = flow.run(command)
    expect(flow.pending.value?.businessPayload).toEqual(command)
    expect(flow.busy.value).toBe(true)
    expect(sendClosureCommand).toHaveBeenCalledWith(command, expect.any(String))
    request.resolve({ inventory_id: '9223372036854775800' }); await result
    expect(flow.pending.value).toBeNull(); expect(flow.busy.value).toBe(false)
  })
  it('结果未知后刷新，用持久化参数和原key恢复，阻止创建另一笔', async () => {
    const { flow, scope } = setup()
    vi.mocked(sendClosureCommand).mockRejectedValueOnce({ kind:'network' })
    await flow.run(command)
    const key = vi.mocked(sendClosureCommand).mock.calls[0]![1]
    await flow.run({ action:'freeze', data: { inventory_id:'2', quantity:1, unfreeze:true, reason:'复检合格' } })
    expect(sendClosureCommand).toHaveBeenCalledTimes(1)
    scope.stop()
    const restored = setup().flow
    vi.mocked(sendClosureCommand).mockResolvedValueOnce({ inventory_id: '9223372036854775800' })
    await restored.recover()
    expect(sendClosureCommand).toHaveBeenLastCalledWith(command, key)
    expect(restored.pending.value).toBeNull()
  })
  it.each([400,43101,43102,43103,30201])('确认回滚的业务拒绝 %d 可以解除阻塞', async code => {
    const { flow } = setup()
    vi.mocked(sendClosureCommand).mockRejectedValueOnce({ code, status:409 })
    await flow.run(command)
    expect(flow.pending.value).toBeNull()
  })
  it.each([{code:40901,status:409},{code:50406,status:503},{code:50406,status:401},{code:500,status:500}])('冲突、鉴权和未知失败保留key：%o', async failure => {
    const { flow } = setup()
    vi.mocked(sendClosureCommand).mockRejectedValueOnce(failure)
    await flow.run(command)
    expect(flow.pending.value?.operationKey).toBeTruthy()
  })
  it('写入存储失败时不发请求', async () => {
    const original = localStorage.setItem
    localStorage.setItem = (key,value) => { if (key === 'WMS_PENDING_OPS') throw Error('quota'); original(key,value) }
    const { flow } = setup(); await flow.run(command)
    expect(sendClosureCommand).not.toHaveBeenCalled(); expect(flow.error.value).toBeTruthy()
  })
  it('离开页面后晚成功不丢弃未确认记录', async () => {
    const { flow, scope } = setup(), request = deferred()
    vi.mocked(sendClosureCommand).mockReturnValueOnce(request.promise)
    const result = flow.run(command); scope.stop()
    request.resolve({inventory_id:'9223372036854775800'}); await result
    expect(loadPending({userId:'1',sessionNamespace:''},localStorage).items).toHaveLength(1)
  })
  it('切换账号后晚响应不污染新账号正在发送的状态', async () => {
    const { flow } = setup(), first = deferred(), second = deferred()
    vi.mocked(sendClosureCommand).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const old = flow.run(command); login('2'); await nextTick()
    const current = flow.run({ action:'freeze', data: { inventory_id:'2', quantity:1, unfreeze:true, reason:'复检合格' } })
    first.resolve({inventory_id:'9223372036854775800'}); await old
    expect(flow.busy.value).toBe(true); expect(flow.pending.value?.userId).toBe('2')
    second.resolve({inventory_id:'2'}); await current
    expect(flow.busy.value).toBe(false)
    expect(loadPending({userId:'1',sessionNamespace:''},localStorage).items).toHaveLength(1)
  })
  it('过期或损坏命令禁止自动重发', async () => {
    const { flow } = setup()
    vi.mocked(sendClosureCommand).mockRejectedValueOnce({kind:'network'}); await flow.run(command)
    const container = JSON.parse(localStorage.getItem('WMS_PENDING_OPS')!)
    container.items[0].createdAt = new Date(Date.now()-PENDING_RETENTION_MS-1).toISOString()
    localStorage.setItem('WMS_PENDING_OPS',JSON.stringify(container)); await flow.recover()
    expect(sendClosureCommand).toHaveBeenCalledTimes(1); expect(flow.error.value).toContain('7天')
    container.items[0].createdAt = new Date().toISOString(); container.items[0].businessPayload = {action:'delete',wave_id:'1'}
    localStorage.setItem('WMS_PENDING_OPS',JSON.stringify(container)); await flow.recover()
    expect(sendClosureCommand).toHaveBeenCalledTimes(1); expect(flow.error.value).toContain('损坏')
  })
  it('演示会话使用独立存储，切换会话不读取旧命令', async () => {
    login('1','session-a'); const { flow } = setup()
    vi.mocked(sendClosureCommand).mockRejectedValueOnce({kind:'network'}); await flow.run(command)
    expect(localStorage.getItem('WMS_PENDING_OPS')).toBeNull()
    expect(sessionStorage.getItem('WMS_PENDING_OPS')).toContain('session-a')
    login('1','session-b'); await nextTick(); expect(flow.pending.value).toBeNull()
  })
})
