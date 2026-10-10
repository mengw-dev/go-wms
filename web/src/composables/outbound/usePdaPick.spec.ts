import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { claimPdaTask, pickPdaTask } from '@/api/outbound'
import type { ClaimResult, PickResult } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { loadPending } from '@/utils/pendingOperations'
import { usePdaPick } from './usePdaPick'

// PDA 领取/拣货接口换成受控实现，便于断言调用参数（claim_token 与 Idempotency-Key 复用）。
vi.mock('@/api/outbound', () => ({
  claimPdaTask: vi.fn(),
  pickPdaTask: vi.fn(),
}))

const mockedClaim = vi.mocked(claimPdaTask)
const mockedPick = vi.mocked(pickPdaTask)

function createStorage(): Storage {
  const store = new Map<string, string>()
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => {
      store.set(key, String(value))
    },
    removeItem: (key: string) => {
      store.delete(key)
    },
    clear: () => {
      store.clear()
    },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
    get length() {
      return store.size
    },
  } as Storage
}

function createFailingStorage(): Storage {
  const store = new Map<string, string>()
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: () => {
      throw new Error('QuotaExceededError')
    },
    removeItem: () => {
      throw new Error('QuotaExceededError')
    },
    clear: () => {},
    key: (index: number) => Array.from(store.keys())[index] ?? null,
    get length() {
      return store.size
    },
  } as Storage
}

function loginAs(userId: string, perms: string[] = [], sessionId = '') {
  const auth = useAuthStore()
  auth.setAuth({
    token: 't',
    user_id: userId,
    username: `u${userId}`,
    nickname: 'Operator',
    roles: ['operator'],
    perms,
  })
  if (perms.includes('wms:demo') && sessionId) {
    auth.setDemoSession({ session_id: sessionId, expires_in: 600 })
  }
}

/** 预置一条待确认操作记录（模拟刷新/关闭前的落盘状态）。 */
function seedPickRecord(userId: string, sessionNamespace: string, storage: Storage, overrides: Record<string, unknown> = {}) {
  seedRecords(storage, [
    {
      schemaVersion: 1,
      userId,
      sessionNamespace,
      scope: 'outbound.pick',
      objectId: '1',
      operationKey: 'seed-key-1',
      businessPayload: { qty: 1, location_code: 'A-01' },
      createdAt: new Date().toISOString(),
      state: 'uncertain',
      ...overrides,
    },
  ])
}

function seedClaimRecord(userId: string, sessionNamespace: string, storage: Storage) {
  seedRecords(storage, [
    {
      schemaVersion: 1,
      userId,
      sessionNamespace,
      scope: 'outbound.claim',
      objectId: '1',
      operationKey: 'seed-claim-key-1',
      businessPayload: {},
      createdAt: new Date().toISOString(),
      state: 'sending',
    },
  ])
}

function seedRecords(storage: Storage, items: unknown[]) {
  storage.setItem('WMS_PENDING_OPS', JSON.stringify({ version: 1, items }))
}

function claimResult(overrides: Partial<ClaimResult> = {}): ClaimResult {
  return {
    claim_token: 'token-1',
    lease_expire_at: '2026-10-06T12:00:00Z',
    task_status: 'IN_PROGRESS',
    done_qty: 0,
    remaining_qty: 2,
    order_status: 'PICKING',
    ...overrides,
  }
}

function pickResult(overrides: Partial<PickResult> = {}): PickResult {
  return {
    task_status: 'IN_PROGRESS',
    done_qty: 1,
    remaining_qty: 1,
    order_status: 'PICKING',
    ...overrides,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

beforeEach(() => {
  vi.resetAllMocks()
  vi.stubGlobal('localStorage', createStorage())
  vi.stubGlobal('sessionStorage', createStorage())
  setActivePinia(createPinia())
  loginAs('1')
})

describe('usePdaPick 领取与拣货基础闭环', () => {
  it('领取成功后，拣货请求携带领取凭证 claim_token', async () => {
    mockedClaim.mockResolvedValue(claimResult())
    mockedPick.mockResolvedValue(pickResult())
    const { claim, pick } = usePdaPick()

    expect(await claim('1')).toBe(true)
    await pick('1', { qty: 1, location_code: 'A-01', batch_no: 'B001' })

    const data = mockedPick.mock.calls[0][1]
    expect(data.claim_token).toBe('token-1')
    expect(data).toMatchObject({ qty: 1, location_code: 'A-01', batch_no: 'B001' })
  })

  it('未领取时拣货请求不携带凭证，且领取失败不会产生拣货凭证', async () => {
    mockedClaim.mockRejectedValue({ message: '领取失败' })
    mockedPick.mockRejectedValue({ message: '缺少领取凭证' })
    const { claim, pick, stateOf } = usePdaPick()

    expect(await claim('1')).toBe(false)
    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('缺少领取凭证')

    const data = mockedPick.mock.calls[0][1]
    expect(data.claim_token).toBe('')
    expect(stateOf('1').claimToken).toBe('')
  })

  it('业务拒绝且后端返回快照时，同样刷新任务快照并保留错误信息', async () => {
    const rejected = pickResult({ task_status: 'CREATED', done_qty: 0, remaining_qty: 2 })
    mockedPick.mockRejectedValue({ kind: 'business', code: 50008, message: '扫描的库位与任务不一致', data: rejected })
    const { pick, stateOf } = usePdaPick()

    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('扫描的库位与任务不一致')

    expect(stateOf('1').snapshot).toEqual(rejected)
    expect(stateOf('1').lastError).toBe('扫描的库位与任务不一致')
  })
})

describe('usePdaPick 操作 key 生命周期', () => {
  it('同一次拣货操作超时重试复用同一个 key 和同一组参数', async () => {
    mockedPick.mockRejectedValueOnce({ kind: 'network', message: '网络异常' })
    mockedPick.mockResolvedValueOnce(pickResult())
    const { pick } = usePdaPick()

    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('网络异常')
    await pick('1', { qty: 1, location_code: 'A-01' })

    expect(mockedPick).toHaveBeenCalledTimes(2)
    expect(mockedPick.mock.calls[1][2]).toBe(mockedPick.mock.calls[0][2])
    expect(mockedPick.mock.calls[1][1]).toEqual({ qty: 1, location_code: 'A-01', claim_token: '' })
  })

  it('连续两次同参数的合法拣货生成两个不同的新 key', async () => {
    mockedPick.mockResolvedValue(pickResult())
    const { pick } = usePdaPick()

    await pick('1', { qty: 1, location_code: 'A-01' })
    await pick('1', { qty: 1, location_code: 'A-01' })

    expect(mockedPick).toHaveBeenCalledTimes(2)
    expect(mockedPick.mock.calls[1][2]).not.toBe(mockedPick.mock.calls[0][2])
  })

  it('结果未知时修改参数被拒绝，不会产生新请求', async () => {
    mockedPick.mockRejectedValueOnce({ kind: 'network', message: '网络异常' })
    const { pick, stateOf } = usePdaPick()

    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('网络异常')
    mockedPick.mockResolvedValueOnce(pickResult())
    await expect(pick('1', { qty: 2, location_code: 'A-01' })).rejects.toThrow('存在未确认的拣货操作')

    expect(mockedPick).toHaveBeenCalledTimes(1)
    expect(stateOf('1').pendingPick?.businessPayload.qty).toBe(1)
  })

  it('服务端确认未执行（静态校验类拒绝）后可修正内容开始新操作', async () => {
    mockedPick.mockRejectedValueOnce({ kind: 'business', code: 50008, message: '扫描的库位与任务不一致' })
    mockedPick.mockResolvedValueOnce(pickResult())
    const { pick, stateOf } = usePdaPick()

    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('扫描的库位与任务不一致')
    expect(stateOf('1').pendingPick).toBeNull()

    await pick('1', { qty: 1, location_code: 'B-02' })
    expect(mockedPick).toHaveBeenCalledTimes(2)
    expect(mockedPick.mock.calls[1][2]).not.toBe(mockedPick.mock.calls[0][2])
  })

  it('状态/数量类拒绝不能确认未执行，保留原 key 记录', async () => {
    mockedPick.mockRejectedValueOnce({ kind: 'business', code: 50004, message: '拣货数量超过任务剩余数量' })
    const { pick, stateOf } = usePdaPick()

    await expect(pick('1', { qty: 9, location_code: 'A-01' })).rejects.toThrow('拣货数量超过任务剩余数量')

    expect(stateOf('1').pendingPick).not.toBeNull()
    expect(stateOf('1').pendingPick?.businessPayload.qty).toBe(9)
  })

  it('双击提交不会创建两次业务操作', async () => {
    const first = deferred<PickResult>()
    mockedPick.mockReturnValueOnce(first.promise)
    const { pick } = usePdaPick()

    const p1 = pick('1', { qty: 1, location_code: 'A-01' })
    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('操作正在进行中')

    expect(mockedPick).toHaveBeenCalledTimes(1)
    first.resolve(pickResult())
    await p1
  })

  it('领取超时重试复用同一个领取 key', async () => {
    mockedClaim.mockRejectedValueOnce({ kind: 'network', message: '网络异常' })
    mockedClaim.mockResolvedValueOnce(claimResult())
    const { claim } = usePdaPick()

    expect(await claim('1')).toBe(false)
    expect(await claim('1')).toBe(true)

    expect(mockedClaim).toHaveBeenCalledTimes(2)
    expect(mockedClaim.mock.calls[1][1]).toBe(mockedClaim.mock.calls[0][1])
  })

  it('已完成领取后的再次领取使用新的领取 key', async () => {
    mockedClaim.mockResolvedValue(claimResult())
    const { claim } = usePdaPick()

    expect(await claim('1')).toBe(true)
    expect(await claim('1')).toBe(true)

    expect(mockedClaim.mock.calls[1][1]).not.toBe(mockedClaim.mock.calls[0][1])
  })

  it('租约过期被拒绝后凭证失效，重新领取使用新的领取 key', async () => {
    mockedClaim.mockResolvedValue(claimResult())
    const { claim, pick, stateOf } = usePdaPick()

    expect(await claim('1')).toBe(true)
    expect(stateOf('1').claimToken).toBe('token-1')

    mockedPick.mockRejectedValueOnce({ kind: 'business', code: 40019, message: '任务租约已过期，请重新领取任务' })
    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('任务租约已过期')
    expect(stateOf('1').claimToken).toBe('')

    mockedClaim.mockResolvedValue(claimResult({ claim_token: 'token-2' }))
    expect(await claim('1')).toBe(true)
    expect(stateOf('1').claimToken).toBe('token-2')
    expect(mockedClaim.mock.calls[1][1]).not.toBe(mockedClaim.mock.calls[0][1])
  })
})

describe('usePdaPick 刷新恢复与隔离', () => {
  it('刷新后恢复待确认拣货：原 key 原参数重试，成功后清除记录', async () => {
    seedPickRecord('1', '', localStorage)
    mockedPick.mockResolvedValue(pickResult())
    const { stateOf, restoreForTasks, recoverPick } = usePdaPick()

    expect(restoreForTasks(['1'])).toBe('')
    expect(stateOf('1').pendingPick?.operationKey).toBe('seed-key-1')
    expect(stateOf('1').pendingPick?.stale).toBe(false)

    await recoverPick('1')

    expect(mockedPick.mock.calls[0][2]).toBe('seed-key-1')
    expect(mockedPick.mock.calls[0][1]).toEqual({ qty: 1, location_code: 'A-01', claim_token: '' })
    expect(stateOf('1').pendingPick).toBeNull()
    expect(loadPending({ userId: '1', sessionNamespace: '' }, localStorage).items).toHaveLength(0)
  })

  it('刷新后恢复未确认的领取（sending 按 uncertain 恢复）', async () => {
    seedClaimRecord('1', '', localStorage)
    mockedClaim.mockResolvedValue(claimResult())
    const { stateOf, restoreForTasks, recoverClaim } = usePdaPick()

    expect(restoreForTasks(['1'])).toBe('')
    expect(stateOf('1').pendingClaim?.operationKey).toBe('seed-claim-key-1')

    expect(await recoverClaim('1')).toBe(true)
    expect(mockedClaim.mock.calls[0][1]).toBe('seed-claim-key-1')
    expect(stateOf('1').pendingClaim).toBeNull()
  })

  it('超过保留期的待确认操作不允许自动重试', async () => {
    seedPickRecord('1', '', localStorage, {
      createdAt: new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString(),
    })
    mockedPick.mockResolvedValue(pickResult())
    const { stateOf, restoreForTasks, recoverPick } = usePdaPick()

    restoreForTasks(['1'])
    expect(stateOf('1').pendingPick?.stale).toBe(true)

    await expect(recoverPick('1')).rejects.toThrow('超过保留期')
    expect(mockedPick).not.toHaveBeenCalled()
  })

  it('任务 A 的领取凭证不会带到任务 B', async () => {
    mockedClaim.mockResolvedValue(claimResult())
    const { claim, stateOf } = usePdaPick()

    await claim('1')

    expect(stateOf('1').claimToken).toBe('token-1')
    expect(stateOf('2').claimToken).toBe('')
  })

  it('任务 A 的晚返回只更新 A 的状态，不污染任务 B', async () => {
    const lateA = deferred<PickResult>()
    const fastB = deferred<PickResult>()
    mockedPick.mockReturnValueOnce(lateA.promise).mockReturnValueOnce(fastB.promise)
    const { pick, stateOf } = usePdaPick()

    const pickA = pick('1', { qty: 1, location_code: 'A-01' })
    const pickB = pick('2', { qty: 1, location_code: 'B-01' })
    fastB.resolve(pickResult({ done_qty: 1 }))
    await pickB

    lateA.resolve(pickResult({ done_qty: 2, remaining_qty: 0, task_status: 'COMPLETED' }))
    await pickA

    expect(stateOf('2').snapshot).toEqual(pickResult({ done_qty: 1 }))
    expect(stateOf('2').lastError).toBe('')
    expect(stateOf('1').snapshot?.done_qty).toBe(2)
  })

  it('账号切换后看不到其他用户的待确认操作', async () => {
    seedPickRecord('1', '', localStorage)
    const { stateOf, restoreForTasks } = usePdaPick()
    restoreForTasks(['1'])
    expect(stateOf('1').pendingPick).not.toBeNull()

    loginAs('2')
    restoreForTasks(['1'])
    expect(stateOf('1').pendingPick).toBeNull()

    loginAs('1')
    restoreForTasks(['1'])
    expect(stateOf('1').pendingPick?.operationKey).toBe('seed-key-1')
  })

  it('同一演示账号的不同访客会话互相隔离', async () => {
    loginAs('9', ['wms:demo'], 's-9')
    // 会话 s-9 的待确认记录落在 sessionStorage（演示账号随会话失效）。
    seedPickRecord('9', 's-9', sessionStorage)
    const { stateOf, restoreForTasks } = usePdaPick()
    restoreForTasks(['1'])
    expect(stateOf('1').pendingPick?.operationKey).toBe('seed-key-1')

    // 同一账号换了一个访客会话：不应看到或重发上一个会话的操作。
    useAuthStore().setDemoSession({ session_id: 's-10', expires_in: 600 })
    restoreForTasks(['1'])
    expect(stateOf('1').pendingPick).toBeNull()
  })

  it('存储写入失败时不发送请求并明确报错', async () => {
    loginAs('1')
    vi.stubGlobal('localStorage', createFailingStorage())
    mockedPick.mockResolvedValue(pickResult())
    const { pick } = usePdaPick()

    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('无法保存待确认操作记录')
    expect(mockedPick).not.toHaveBeenCalled()
  })

  it('存储记录损坏时阻止新操作并报告错误', async () => {
    localStorage.setItem('WMS_PENDING_OPS', '{corrupt')
    mockedPick.mockResolvedValue(pickResult())
    const { storageError, restoreForTasks, pick } = usePdaPick()

    const restoreError = restoreForTasks(['1'])
    expect(restoreError).toContain('损坏')
    expect(storageError.value).toBe(restoreError)

    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('损坏')
    expect(mockedPick).not.toHaveBeenCalled()
  })

  it('核对后放弃待确认记录后可以开始新操作', async () => {
    seedPickRecord('1', '', localStorage)
    mockedPick.mockResolvedValue(pickResult())
    const { stateOf, restoreForTasks, discardPick, pick } = usePdaPick()

    restoreForTasks(['1'])
    discardPick('1')
    expect(stateOf('1').pendingPick).toBeNull()

    await pick('1', { qty: 1, location_code: 'A-01' })
    expect(mockedPick.mock.calls[0][2]).not.toBe('seed-key-1')
  })
})