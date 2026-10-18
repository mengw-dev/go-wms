import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { receiveInbound, putawayInboundTask } from '@/api/inbound'
import { claimPdaTask, pickPdaTask } from '@/api/outbound'
import { approveStocktakeOrder } from '@/api/stocktake'
import { useAuthStore } from '@/stores/auth'
import { loadPending } from '@/utils/pendingOperations'
import { useReceive } from './inbound/useReceive'
import { usePutaway } from './inbound/usePutaway'
import { usePdaPick } from './outbound/usePdaPick'
import { useStocktakeApprove } from './stocktake/useStocktakeApprove'

vi.mock('@/api/inbound', () => ({ receiveInbound: vi.fn(), putawayInboundTask: vi.fn() }))
vi.mock('@/api/outbound', () => ({ claimPdaTask: vi.fn(), pickPdaTask: vi.fn() }))
vi.mock('@/api/stocktake', () => ({ approveStocktakeOrder: vi.fn() }))

function storage(): Storage {
  const entries = new Map<string, string>()
  return {
    getItem: (key) => entries.get(key) ?? null,
    setItem: (key, value) => { entries.set(key, value) },
    removeItem: (key) => { entries.delete(key) },
    clear: () => entries.clear(),
    key: (index) => [...entries.keys()][index] ?? null,
    get length() { return entries.size },
  }
}

function login(id: string) {
  useAuthStore().setAuth({ token: 'token', user_id: id, username: id, nickname: id, roles: [], perms: [] })
}

beforeEach(() => {
  vi.resetAllMocks()
  vi.stubGlobal('localStorage', storage())
  vi.stubGlobal('sessionStorage', storage())
  setActivePinia(createPinia())
  login('1')
})

const snapshot = { task_status: 'COMPLETED', done_qty: 2, remaining_qty: 0, order_status: 'SHIPPED' }
const claimSnapshot = { ...snapshot, claim_token: 'claim-token', lease_expire_at: '2099-01-01T00:00:00Z' }

const cases = [
  { name: '收货', setup: () => {
    const op = useReceive()
    return { submit: () => op.receive('100', '1', { qty: 2, defective_qty: 0, batch_no: 'B1' }), recover: () => op.recoverReceive('100', '1'), discard: () => op.discardReceive('1'), state: () => op.stateOf('1'), api: vi.mocked(receiveInbound), result: undefined }
  } },
  { name: '上架', setup: () => {
    const op = usePutaway()
    return { submit: () => op.putaway('1', { qty: 2, location_id: '7' }), recover: () => op.recoverPutaway('1'), discard: () => op.discardPutaway('1'), state: () => op.stateOf('1'), api: vi.mocked(putawayInboundTask), result: undefined }
  } },
  { name: '盘点审核', setup: () => {
    const op = useStocktakeApprove()
    return { submit: () => op.approve('1'), recover: () => op.approve('1'), discard: () => op.discard('1'), state: () => op.stateOf('1'), api: vi.mocked(approveStocktakeOrder), result: undefined }
  } },
  { name: '拣货', setup: () => {
    const op = usePdaPick()
    return { submit: () => op.pick('1', { qty: 2, location_code: 'A', batch_no: 'B1' }), recover: () => op.recoverPick('1'), discard: () => op.discardPick('1'), state: () => op.stateOf('1'), api: vi.mocked(pickPdaTask), result: snapshot }
  } },
]

describe.each(cases)('$name 恢复边界', ({ setup }) => {
  it('恢复重试期间禁止第二次恢复和放弃，成功后才移除记录', async () => {
    const op = setup()
    op.api.mockRejectedValueOnce(new Error('network'))
    await expect(op.submit()).rejects.toThrow('network')
    let resolve!: (value: never) => void
    op.api.mockImplementationOnce(() => new Promise<never>((done) => { resolve = done }))
    const request = op.recover()
    await expect(op.recover()).rejects.toThrow(/正在进行|请勿重复/)
    op.discard()
    expect(loadPending({ userId: '1', sessionNamespace: '' }, localStorage).items).toHaveLength(1)
    resolve(op.result as never)
    await request
    expect(op.api).toHaveBeenCalledTimes(2)
    expect(loadPending({ userId: '1', sessionNamespace: '' }, localStorage).items).toHaveLength(0)
  })

  it('账号切换时隔离旧请求晚返回，原账号仍能恢复同一个 key', async () => {
    const op = setup()
    let resolve!: (value: never) => void
    op.api.mockImplementationOnce(() => new Promise<never>((done) => { resolve = done }))
    const request = op.submit()
    const rejected = expect(request).rejects.toThrow('登录身份已变更')
    login('2')
    const otherState = op.state()
    expect(otherState.submitting).toBe(false)
    resolve(op.result as never)
    await rejected
    expect(otherState.lastError).toBe('')
    expect(loadPending({ userId: '1', sessionNamespace: '' }, localStorage).items).toHaveLength(1)
    login('1')
    op.api.mockResolvedValueOnce(op.result as never)
    await op.recover()
    expect(op.api.mock.calls[1]?.at(-1)).toBe(op.api.mock.calls[0]?.at(-1))
  })

  it('提交前存储被损坏时阻止发送，不能把读失败当作没有记录', async () => {
    const op = setup()
    localStorage.setItem('WMS_PENDING_OPS', '{invalid')
    await expect(op.submit()).rejects.toThrow(/损坏/)
    expect(op.api).not.toHaveBeenCalled()
  })

  it('内容冲突后的恢复不会绕过阻止标记继续发送', async () => {
    const op = setup()
    op.api.mockRejectedValueOnce({ code: 40901, message: 'conflict' })
    await expect(op.submit()).rejects.toEqual({ code: 40901, message: 'conflict' })
    await expect(op.recover()).rejects.toThrow(/核对/)
    expect(op.api).toHaveBeenCalledTimes(1)
  })

  it('后端无法核对结果时保留原 key 和参数，恢复仍发送同一操作', async () => {
    const op = setup()
    const unknown = { code: 50301, status: 503, message: '结果暂时无法确认' }
    op.api.mockRejectedValueOnce(unknown)
    await expect(op.submit()).rejects.toEqual(unknown)
    const pending = loadPending({ userId: '1', sessionNamespace: '' }, localStorage).items[0]
    expect(pending?.state).toBe('uncertain')
    op.api.mockResolvedValueOnce(op.result as never)
    await op.recover()
    expect(op.api.mock.calls[1]?.at(-1)).toBe(pending?.operationKey)
  })
})

it('领取恢复同样禁止双击和发送中放弃，切账号不泄露凭证', async () => {
  const op = usePdaPick()
  vi.mocked(claimPdaTask).mockRejectedValueOnce(new Error('network'))
  expect(await op.claim('1')).toBe(false)
  let resolve!: (value: typeof claimSnapshot) => void
  vi.mocked(claimPdaTask).mockImplementationOnce(() => new Promise<typeof claimSnapshot>((done) => { resolve = done }))
  const request = op.recoverClaim('1')
  expect(await op.recoverClaim('1')).toBe(false)
  op.discardClaim('1')
  expect(loadPending({ userId: '1', sessionNamespace: '' }, localStorage).items).toHaveLength(1)
  login('2')
  resolve(claimSnapshot)
  expect(await request).toBe(false)
  expect(op.stateOf('1').claimToken).toBe('')
  expect(claimPdaTask).toHaveBeenCalledTimes(2)
})

it.each([useReceive, usePutaway, usePdaPick, useStocktakeApprove])('销毁作用域时注销存储监听', (create) => {
  const addEventListener = vi.fn()
  const removeEventListener = vi.fn()
  vi.stubGlobal('window', { addEventListener, removeEventListener })
  const scope = effectScope()
  scope.run(() => { create() })
  expect(addEventListener).toHaveBeenCalledTimes(1)
  scope.stop()
  expect(removeEventListener).toHaveBeenCalledWith('storage', addEventListener.mock.calls[0]?.[1])
  vi.unstubAllGlobals()
})
