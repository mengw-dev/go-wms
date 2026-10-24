import { effectScope, nextTick, reactive } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ElMessageBox } from 'element-plus'
import { useScopedConfirm } from './useScopedConfirm'

const state = vi.hoisted(() => ({ route: { fullPath: '/inbound/orders/1' }, auth: { sessionVersion: 1, token: 'first', demoSessionId: '' } }))
vi.mock('vue-router', () => ({ useRoute: () => state.route }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => state.auth }))
const dialogs = vi.hoisted(() => ({ confirm: vi.fn(), close: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessageBox: dialogs }))
const scopes: ReturnType<typeof effectScope>[] = []
function setup() { const scope = effectScope(); scopes.push(scope); return { scope, confirm: scope.run(useScopedConfirm)! } }
function deferred() { let resolve!: (value: 'confirm') => void; const promise = new Promise<'confirm'>(yes => { resolve = yes }); return { promise, resolve } }
beforeEach(() => {
  vi.resetAllMocks()
  state.route = reactive({ fullPath: '/inbound/orders/1' })
  state.auth = reactive({ sessionVersion: 1, token: 'first', demoSessionId: '' })
})
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()) })

it('accepts a current confirmation and preserves normal cancellation', async () => {
  const flow = setup()
  dialogs.confirm.mockResolvedValueOnce('confirm').mockRejectedValueOnce('cancel')
  await expect(flow.confirm('提交？', '提示')).resolves.toBe('confirm')
  await expect(flow.confirm('取消？', '提示')).rejects.toBe('cancel')
  expect(ElMessageBox.close).not.toHaveBeenCalled()
})
for (const change of ['route', 'login', 'unmount']) {
  it(`rejects a late confirmation after ${change}`, async () => {
    const gate = deferred(), flow = setup()
    dialogs.confirm.mockReturnValueOnce(gate.promise)
    const result = flow.confirm('审核？', '提示')
    const rejected = expect(result).rejects.toThrow('已取消')
    if (change === 'route') state.route.fullPath = '/outbound/orders/2'
    else if (change === 'login') { state.auth.sessionVersion++; state.auth.token = 'second' }
    else flow.scope.stop()
    await nextTick()
    expect(ElMessageBox.close).toHaveBeenCalledOnce()
    gate.resolve('confirm')
    await rejected
  })
}
it('does not open duplicate confirmations and remains reusable after cancellation', async () => {
  const gate = deferred(), flow = setup()
  dialogs.confirm.mockReturnValueOnce(gate.promise).mockResolvedValueOnce('confirm')
  const first = flow.confirm('第一次', '提示')
  await expect(flow.confirm('重复', '提示')).rejects.toThrow('等待确认')
  gate.resolve('confirm'); await first
  await expect(flow.confirm('下一次', '提示')).resolves.toBe('confirm')
  expect(ElMessageBox.confirm).toHaveBeenCalledTimes(2)
})
it('does not close other dialogs when its page has no outstanding confirmation', () => {
  const flow = setup(); flow.scope.stop()
  expect(ElMessageBox.close).not.toHaveBeenCalled()
})
