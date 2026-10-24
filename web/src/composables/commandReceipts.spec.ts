import { effectScope } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { sendClosureCommand, type ClosureAck, type ClosureCommand } from '@/api/closure'
import { sendWaveCommand, type WaveAck, type WaveCommand } from '@/api/wave'
import { claimPdaTask, pickPdaTask } from '@/api/outbound'
import type { ClaimResult, PickResult } from '@/api/types'
import { useClosureCommands } from './useClosureCommands'
import { useWaveCommands } from './outbound/useWaveCommands'
import { usePdaPick } from './outbound/usePdaPick'

vi.mock('@/api/closure', async original => ({ ...await original<typeof import('@/api/closure')>(), sendClosureCommand: vi.fn() }))
vi.mock('@/api/wave', async original => ({ ...await original<typeof import('@/api/wave')>(), sendWaveCommand: vi.fn() }))
vi.mock('@/api/outbound', () => ({ claimPdaTask: vi.fn(), pickPdaTask: vi.fn() }))
vi.mock('@/api/request', () => ({ get: vi.fn(), post: vi.fn() }))
function storage(): Storage { const map = new Map<string,string>(); return { getItem:k=>map.get(k)??null, setItem:(k,v)=>{map.set(k,v)}, removeItem:k=>{map.delete(k)}, clear:()=>map.clear(), key:i=>[...map.keys()][i]??null, get length(){return map.size} } }
const scopes: ReturnType<typeof effectScope>[] = []
function setup<T>(factory: () => T) { const scope = effectScope(); scopes.push(scope); return scope.run(factory)! }
const freeze: ClosureCommand = { action:'freeze', data:{ inventory_id:'1', unfreeze:false, quantity:1, reason:'待检' } }
const collect: WaveCommand = { action:'collect', wave_id:'1', part_id:'2', data:{qty:1,sku_code:'SKU',batch_no:'B',location_code:'A'} }
const snapshot: PickResult = {task_status:'IN_PROGRESS',done_qty:1,remaining_qty:1,order_status:'PICKING'}
const claim: ClaimResult = {...snapshot,claim_token:'credential',lease_expire_at:'2099-01-01T00:00:00Z'}
beforeEach(() => { vi.resetAllMocks(); vi.stubGlobal('localStorage',storage()); vi.stubGlobal('sessionStorage',storage()); setActivePinia(createPinia()); useAuthStore().setAuth({token:'test',user_id:'42',username:'worker',nickname:'',roles:[],perms:['*']}) })
afterEach(() => { scopes.splice(0).forEach(s=>s.stop()); vi.unstubAllGlobals() })

it.each([null,{}, {inventory_id:1},{inventory_id:'2'},{inventory_id:'0'},{inventory_id:'9223372036854775808'}])('freeze retains its original request for invalid success %j', async bad => {
  const flow=setup(useClosureCommands)
  vi.mocked(sendClosureCommand).mockResolvedValueOnce(bad as ClosureAck).mockResolvedValueOnce({inventory_id:'1'})
  await flow.run(freeze)
  const key=vi.mocked(sendClosureCommand).mock.calls[0]![1]
  expect(flow.pending.value?.operationKey).toBe(key)
  expect(flow.error.value).toContain('回执')
  await flow.recover()
  expect(sendClosureCommand).toHaveBeenLastCalledWith(freeze,key)
  expect(flow.pending.value).toBeNull()
})
it.each([null,{}, {wave_id:1},{wave_id:'2'},{wave_id:'0'},{wave_id:'9223372036854775808'}])('wave retains its original request for invalid success %j', async bad => {
  const flow=setup(useWaveCommands)
  vi.mocked(sendWaveCommand).mockResolvedValueOnce(bad as WaveAck).mockResolvedValueOnce({wave_id:'1'})
  await flow.run(collect)
  const key=vi.mocked(sendWaveCommand).mock.calls[0]![1]
  expect(flow.pending.value?.operationKey).toBe(key)
  await flow.recover()
  expect(sendWaveCommand).toHaveBeenLastCalledWith(collect,key)
  expect(flow.pending.value).toBeNull()
})
it('resolve validates its exception ID; new receipts and wave creation require valid string IDs',async()=>{
  const flow=setup(useClosureCommands)
  vi.mocked(sendClosureCommand).mockResolvedValueOnce({object_id:'99'})
  await flow.run({action:'resolve',order_id:'1',exception_id:'3',data:{close:true,reason:'关闭'}})
  expect(flow.pending.value).not.toBeNull()
  flow.discard()
  vi.mocked(sendClosureCommand).mockResolvedValueOnce({object_id:'9223372036854775800'})
  await flow.run({action:'return',order_id:'1',data:{allocation_id:'2',location_id:'3',good_qty:1,bad_qty:0,reason:'退货'}})
  expect(flow.pending.value).toBeNull()
  const wave=setup(useWaveCommands)
  vi.mocked(sendWaveCommand).mockResolvedValueOnce({wave_id:'0'})
  await wave.run({action:'create',data:{order_ids:['1'],remark:''}})
  expect(wave.pending.value).not.toBeNull()
})
it.each([{}, {done_qty:-1}, {remaining_qty:'1'}])('PDA malformed pick %j keeps the original key and snapshot',async bad=>{
  const flow=setup(usePdaPick)
  vi.mocked(claimPdaTask).mockResolvedValueOnce(claim); await flow.claim('1')
  vi.mocked(pickPdaTask).mockResolvedValueOnce({...snapshot,...bad} as PickResult)
  if (!Object.keys(bad).length) vi.mocked(pickPdaTask).mockReset().mockResolvedValueOnce({} as PickResult)
  await expect(flow.pick('1',{qty:1})).rejects.toThrow('回执')
  const key=vi.mocked(pickPdaTask).mock.calls[0]![2]
  expect(flow.stateOf('1').pendingPick?.operationKey).toBe(key)
  expect(flow.stateOf('1').snapshot).toEqual(snapshot)
  vi.mocked(pickPdaTask).mockResolvedValueOnce(snapshot); await flow.recoverPick('1')
  expect(vi.mocked(pickPdaTask).mock.calls.at(-1)![2]).toBe(key)
})
it.each([{}, {...claim,claim_token:''}, {...claim,lease_expire_at:'invalid'}])('PDA malformed claim %j remains recoverable',async bad=>{
  const flow=setup(usePdaPick)
  vi.mocked(claimPdaTask).mockResolvedValueOnce(bad as ClaimResult)
  expect(await flow.claim('1')).toBe(false)
  expect(flow.stateOf('1').pendingClaim).not.toBeNull()
  expect(flow.stateOf('1').claimToken).toBe('')
  const key=vi.mocked(claimPdaTask).mock.calls[0]![1]
  vi.mocked(claimPdaTask).mockResolvedValueOnce(claim)
  expect(await flow.recoverClaim('1')).toBe(true)
  expect(claimPdaTask).toHaveBeenLastCalledWith('1',key)
  expect(flow.stateOf('1').pendingClaim).toBeNull()
})

it('accepts an expired original claim replay and a completed pick with zero remaining',async()=>{
  const flow=setup(usePdaPick)
  vi.mocked(claimPdaTask).mockResolvedValueOnce({...claim,lease_expire_at:'2020-01-01T00:00:00Z'})
  expect(await flow.claim('1')).toBe(true)
  expect(flow.stateOf('1').pendingClaim).toBeNull()
  const completed: PickResult={task_status:'COMPLETED',order_status:'SHIPPED',done_qty:2,remaining_qty:0}
  vi.mocked(pickPdaTask).mockResolvedValueOnce(completed)
  expect(await flow.pick('1',{qty:1})).toEqual(completed)
  expect(flow.stateOf('1').pendingPick).toBeNull()
})
