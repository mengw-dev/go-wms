import { effectScope } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { moveStock, listStockMoves, type StockMove } from '@/api/stockMoves'
import { useAuthStore } from '@/stores/auth'
import { useStockMove } from './useStockMove'
import { loadPending } from '@/utils/pendingOperations'

vi.mock('@/api/stockMoves', () => ({ moveStock: vi.fn(), listStockMoves: vi.fn() }))
function storage(): Storage {
  const data = new Map<string,string>()
  return { getItem: k => data.get(k) ?? null, setItem: (k,v) => {data.set(k,v)}, removeItem: k => {data.delete(k)}, clear: () => data.clear(), key: i => [...data.keys()][i] ?? null, get length() {return data.size} }
}
function login(id = '1') { useAuthStore().setAuth({token:'test', user_id:id, username:id, nickname:id, roles:[], perms:['wms:inventory', 'wms:inventory:move']}) }
const payload = {source_inventory_id:'10', target_location_id:'20',quantity:3,reason:'整理'}
const receipt = {...payload,id:'100',move_no:'YK100'} as StockMove
const scopes: ReturnType<typeof effectScope>[] = []
function setup() {const s=effectScope();scopes.push(s);return s.run(useStockMove)!}
beforeEach(() => { vi.resetAllMocks(); vi.stubGlobal('localStorage',storage()); vi.stubGlobal('sessionStorage',storage()); setActivePinia(createPinia());login() })
afterEach(() => {scopes.splice(0).forEach(s=>s.stop());vi.unstubAllGlobals()})
it('saves before send, blocks simultaneous requests and reuses original payload/key after reload', async () => {
  const a=setup(); vi.mocked(moveStock).mockRejectedValueOnce(Error('network'))
  await a.send(payload)
  const original=vi.mocked(moveStock).mock.calls[0]![1]
  expect(loadPending({userId:'1',sessionNamespace:''},localStorage).items).toHaveLength(1)
  const b=setup();b.restore();const item=b.pending.value[0]!
  await b.send({...payload,quantity:4});expect(moveStock).toHaveBeenCalledTimes(1)
  let resolve!: (r:StockMove)=>void
  vi.mocked(moveStock).mockImplementationOnce(()=>new Promise(done=>{resolve=done}))
  const request=b.retry(item)
  await b.retry(item);b.forget(item)
  expect(moveStock).toHaveBeenCalledTimes(2)
  expect(vi.mocked(moveStock).mock.calls[1]).toEqual([payload,original])
  resolve(receipt);await request
  expect(b.pending.value).toHaveLength(0)
})
it('reconciles lost response via durable receipt without sending another stock mutation',async()=>{
  const op=setup();vi.mocked(moveStock).mockRejectedValueOnce(Error('network'));await op.send(payload)
  const item=op.pending.value[0]!;vi.mocked(listStockMoves).mockResolvedValueOnce({list:[receipt],total:1})
  expect(await op.check(item)).toEqual(receipt);expect(op.pending.value).toHaveLength(0);expect(moveStock).toHaveBeenCalledTimes(1)
})
it.each([400,30001,30010,30011])('clears definitively rejected code %s only',async code=>{
  const op=setup();vi.mocked(moveStock).mockRejectedValueOnce({code,message:'rejected'});await op.send(payload)
  expect(op.pending.value).toHaveLength(0)
})
it.each([403,500,10008])('retains uncertain or authorization error %s',async code=>{
  const op=setup();vi.mocked(moveStock).mockRejectedValueOnce({code,message:'uncertain'});await op.send(payload)
  expect(op.pending.value).toHaveLength(1)
})
it('blocks a request when storage is corrupt or unavailable', async()=>{
  const op=setup();localStorage.setItem('WMS_PENDING_OPS','bad-json');await op.send(payload);expect(moveStock).not.toHaveBeenCalled()
  localStorage.clear();vi.spyOn(localStorage,'setItem').mockImplementation(()=>{throw Error('quota')});await op.send(payload);expect(moveStock).not.toHaveBeenCalled()
})
it('keeps old account pending when a late response arrives after identity changes',async()=>{
  const op=setup();let resolve!:(r:StockMove)=>void
  vi.mocked(moveStock).mockImplementationOnce(()=>new Promise(done=>{resolve=done}));const request=op.send(payload)
  login('2');resolve(receipt);expect(await request).toBeUndefined()
  expect(loadPending({userId:'1',sessionNamespace:''},localStorage).items).toHaveLength(1)
  op.restore();expect(op.pending.value).toHaveLength(0)
})
it('keeps original request for malformed success and mismatched receipt',async()=>{
  const op=setup();vi.mocked(moveStock).mockResolvedValueOnce({...receipt,quantity:9});await op.send(payload)
  expect(op.pending.value).toHaveLength(1)
  vi.mocked(listStockMoves).mockResolvedValueOnce({list:[{...receipt,reason:'different'}],total:1})
  await op.check(op.pending.value[0]!);expect(op.pending.value).toHaveLength(1)
})
it('will not resend requests older than seven days, but still allows record reconciliation',async()=>{
  const op=setup();vi.mocked(moveStock).mockRejectedValueOnce(Error('network'));await op.send(payload)
  const saved=JSON.parse(localStorage.getItem('WMS_PENDING_OPS')!); const raw=JSON.stringify(saved).replace(/"createdAt":"[^"]+"/g,'"createdAt":"2020-01-01T00:00:00Z"');localStorage.setItem('WMS_PENDING_OPS',raw)
  op.restore();await op.retry(op.pending.value[0]!);expect(moveStock).toHaveBeenCalledTimes(1)
  vi.mocked(listStockMoves).mockResolvedValueOnce({list:[receipt],total:1});await op.check(op.pending.value[0]!);expect(op.pending.value).toHaveLength(0)
})
