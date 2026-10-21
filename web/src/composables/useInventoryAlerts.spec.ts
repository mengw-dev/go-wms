import { effectScope } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { listInventoryAlerts, type AlertList } from '@/api/inventoryAlerts'
import { useInventoryAlerts } from './useInventoryAlerts'
vi.mock('@/api/inventoryAlerts', () => ({ listInventoryAlerts: vi.fn() }))
function deferred() { let resolve!: (data: AlertList) => void, reject!: (e: Error) => void; const promise = new Promise<AlertList>((yes,no) => {resolve=yes;reject=no});return {promise,resolve,reject} }
const result = (total: number): AlertList => ({list:[],total,warehouses:[],summary:{configured:total,low:0,out_of_stock:0},checked_at:'now'})
describe('预警查询生命周期',()=>{
  it('旧筛选响应不覆盖新结果或提前清loading',async()=>{
    const first=deferred(),latest=deferred();vi.mocked(listInventoryAlerts).mockReturnValueOnce(first.promise).mockReturnValueOnce(latest.promise)
    const scope=effectScope(),state=scope.run(()=>useInventoryAlerts('1'))!
    const a=state.load();state.query.state='ALL';const b=state.search()
    first.resolve(result(1));await a;expect(state.loading.value).toBe(true)
    latest.resolve(result(2));await b;expect(state.result.value.total).toBe(2);expect(state.loading.value).toBe(false);scope.stop()
  })
  it('最新失败不被旧成功覆盖，并保留上次结果说明',async()=>{
    const first=deferred(),latest=deferred();vi.mocked(listInventoryAlerts).mockReturnValueOnce(first.promise).mockReturnValueOnce(latest.promise)
    const scope=effectScope(),state=scope.run(useInventoryAlerts)!,a=state.load(),b=state.load()
    latest.reject(Error('failed'));await b;first.resolve(result(99));await a
    expect(state.result.value.total).toBe(0);expect(state.error.value).toBeTruthy();expect(state.loading.value).toBe(false);scope.stop()
  })
  it('销毁后晚响应不写入，后续不再发请求',async()=>{
    const first=deferred();vi.mocked(listInventoryAlerts).mockReturnValueOnce(first.promise)
    const scope=effectScope(),state=scope.run(useInventoryAlerts)!,a=state.load();scope.stop();first.resolve(result(99));await a
    expect(state.result.value.total).toBe(0);const before=vi.mocked(listInventoryAlerts).mock.calls.length;await state.load();expect(vi.mocked(listInventoryAlerts).mock.calls.length).toBe(before)
  })
})
