import { effectScope, ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, afterEach, expect, it, vi } from 'vitest'
import { scanOrderSKU, type ScannedSKU } from '@/api/scanning'
import { useAuthStore } from '@/stores/auth'
import { useSkuScan } from './useSkuScan'
import { scanCode } from '@/utils/scanCode'
vi.mock('@/api/scanning', () => ({ scanOrderSKU: vi.fn() }))
const row={id:'10',code:'SKU-A',barcode:'00123',name:'货品'}
const scopes:ReturnType<typeof effectScope>[]=[]
function login(id='42') { useAuthStore().setAuth({token:'test',user_id:id,username:id,nickname:id,roles:[],perms:[]}) }
function storage(): Storage {
  const data = new Map<string,string>()
  return { getItem:k=>data.get(k)??null,setItem:(k,v)=>{data.set(k,v)},removeItem:k=>{data.delete(k)},clear:()=>data.clear(),key:i=>[...data.keys()][i]??null,get length(){return data.size} }
}
beforeEach(()=>{vi.resetAllMocks();vi.stubGlobal('localStorage',storage());vi.stubGlobal('sessionStorage',storage());setActivePinia(createPinia());login()})
afterEach(()=>{scopes.splice(0).forEach(s=>s.stop());vi.unstubAllGlobals()})
function setup(expected='') {const scope=effectScope();scopes.push(scope);const task=ref('1'),order=ref('100');const op=scope.run(()=>useSkuScan('inbound',()=>order.value,()=>task.value,()=>expected))!;return{op,task,order,scope}}
it('preserves leading zeros and case; a repeated scan remains a read-only lookup',async()=>{
  const {op}=setup();op.code.value='00123';vi.mocked(scanOrderSKU).mockResolvedValue([row]);await op.scan();await op.scan()
  expect(scanOrderSKU).toHaveBeenNthCalledWith(2,'inbound','100','00123');expect(op.match.value).toEqual(row);expect(op.blocked.value).toBe(false)
  expect(scanCode('Ab-001')).toBe('Ab-001')
})
it.each(['','   ','a'.repeat(65),'00\u0001123'])('rejects invalid input before request: %j',async value=>{
  const {op}=setup();op.code.value=value;await op.scan();expect(scanOrderSKU).not.toHaveBeenCalled();expect(op.error.value).toBeTruthy()
})
it.each([{rows:[]},{rows:[row,{...row,id:'11'}]}])('does not guess on missing or ambiguous matches',async ({rows})=>{
  const {op}=setup();op.code.value='00123';vi.mocked(scanOrderSKU).mockResolvedValue(rows);await op.scan();expect(op.match.value).toBeUndefined();expect(op.blocked.value).toBe(true)
})
it('rejects mismatched selected task or a malformed success',async()=>{
  const {op}=setup('99');op.code.value='00123';vi.mocked(scanOrderSKU).mockResolvedValue([row]);await op.scan();expect(op.error.value).toContain('货品不符')
  vi.mocked(scanOrderSKU).mockResolvedValue([{...row,barcode:'wrong'}]);await op.scan();expect(op.match.value).toBeUndefined()
})
it.each(['task','order','account','close'] as const)('ignores late result after %s changes',async kind=>{
  const {op,task,order,scope}=setup();let resolve!:(rows:ScannedSKU[])=>void
  vi.mocked(scanOrderSKU).mockImplementation(()=>new Promise(done=>{resolve=done}));op.code.value='00123';const pending=op.scan()
  if(kind==='task')task.value='2';else if(kind==='order')order.value='200';else if(kind==='account')login('99');else scope.stop()
  resolve([row]);await pending;expect(op.match.value).toBeUndefined()
})
it('typing a new code clears the previous verification and ignores the older response',async()=>{
  const {op}=setup();let resolve!:(rows:ScannedSKU[])=>void
  vi.mocked(scanOrderSKU).mockImplementationOnce(()=>new Promise(done=>{resolve=done}));op.code.value='00123';const old=op.scan()
  op.code.value='SKU-B';vi.mocked(scanOrderSKU).mockResolvedValueOnce([{...row,id:'20',code:'SKU-B'}]);await op.scan();resolve([row]);await old
  expect(op.match.value?.id).toBe('20');expect(op.code.value).toBe('SKU-B')
  op.code.value='';expect(op.match.value).toBeUndefined();expect(op.blocked.value).toBe(false)
})
it('surfaces request failure without preserving an old verified object',async()=>{
  const {op}=setup();op.code.value='00123';vi.mocked(scanOrderSKU).mockResolvedValueOnce([row]);await op.scan();vi.mocked(scanOrderSKU).mockRejectedValueOnce(Error('network'));await op.scan()
  expect(op.match.value).toBeUndefined();expect(op.error.value).toBe('network')
})
