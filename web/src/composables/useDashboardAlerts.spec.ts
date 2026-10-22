import { effectScope } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { listInventoryAlerts, type AlertList } from '@/api/inventoryAlerts'
import { useDashboardAlerts } from './useDashboardAlerts'

vi.mock('@/api/inventoryAlerts', () => ({ listInventoryAlerts: vi.fn() }))
const result = (n: number): AlertList => ({ list: [], total: n, warehouses: [], summary: { configured: n, out_of_stock: n, low: 0 }, checked_at: '2026-10-22T01:00:00Z' })
function deferred() {
  let resolve!: (value: AlertList) => void, reject!: (error: Error) => void
  const promise = new Promise<AlertList>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
beforeEach(() => vi.resetAllMocks())
describe('仪表盘预警摘要', () => {
  it('无库存权限不发请求，不把未核对的数据当作零', async () => {
    const scope = effectScope(), state = scope.run(() => useDashboardAlerts(() => false))!
    await state.load('1')
    expect(listInventoryAlerts).not.toHaveBeenCalled()
    expect(state.result.value).toBeUndefined()
    scope.stop()
  })
  it('仓库切换立即清除旧摘要，旧成功不覆盖新失败或清理loading', async () => {
    const a = deferred(), b = deferred()
    vi.mocked(listInventoryAlerts).mockResolvedValueOnce(result(8)).mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise)
    const scope = effectScope(), state = scope.run(() => useDashboardAlerts(() => true))!
    await state.load('1')
    const old = state.load('1'), current = state.load('2')
    expect(state.result.value).toBeUndefined()
    a.resolve(result(99)); await old
    expect(state.loading.value).toBe(true)
    b.reject(Error('failed')); await current
    expect(state.result.value).toBeUndefined(); expect(state.error.value).toBe(true)
    expect(listInventoryAlerts).toHaveBeenLastCalledWith({ warehouse_id: '2', state: 'ALERT', sku_keyword: '', page: 1, page_size: 1 })
    scope.stop()
  })
  it('同仓刷新失败保留带时间的成功结果，后续成功清除错误', async () => {
    vi.mocked(listInventoryAlerts).mockResolvedValueOnce(result(3)).mockRejectedValueOnce(Error('failed')).mockResolvedValueOnce(result(0))
    const scope = effectScope(), state = scope.run(() => useDashboardAlerts(() => true))!
    await state.load(''); await state.load('')
    expect(state.result.value?.summary.out_of_stock).toBe(3); expect(state.error.value).toBe(true)
    await state.load('')
    expect(state.result.value?.summary.configured).toBe(0); expect(state.error.value).toBe(false)
    scope.stop()
  })
  it('销毁后的成功、失败和再次调用均不修改状态或发请求', async () => {
    for (const reject of [false, true]) {
      const a = deferred()
      vi.mocked(listInventoryAlerts).mockReturnValueOnce(a.promise)
      const scope = effectScope(), state = scope.run(() => useDashboardAlerts(() => true))!, pending = state.load('1')
      scope.stop()
      if (reject) a.reject(Error('failed')); else a.resolve(result(9))
      await pending
      expect(state.result.value).toBeUndefined(); expect(state.error.value).toBe(false)
      const count = vi.mocked(listInventoryAlerts).mock.calls.length
      await state.load('2'); expect(vi.mocked(listInventoryAlerts)).toHaveBeenCalledTimes(count)
    }
  })
})
