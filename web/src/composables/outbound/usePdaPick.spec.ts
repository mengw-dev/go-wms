import { beforeEach, describe, expect, it, vi } from 'vitest'
import { claimPdaTask, pickPdaTask } from '@/api/outbound'
import type { ClaimResult, PickResult } from '@/api/types'
import { usePdaPick } from './usePdaPick'

// PDA 领取/拣货接口换成受控实现，便于断言调用参数（claim_token 与 Idempotency-Key 复用）。
vi.mock('@/api/outbound', () => ({
  claimPdaTask: vi.fn(),
  pickPdaTask: vi.fn(),
}))

const mockedClaim = vi.mocked(claimPdaTask)
const mockedPick = vi.mocked(pickPdaTask)

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

beforeEach(() => {
  vi.clearAllMocks()
})

describe('usePdaPick', () => {
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
    const { claim, pick } = usePdaPick()

    expect(await claim('1')).toBe(false)
    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow()

    const data = mockedPick.mock.calls[0][1]
    expect(data.claim_token).toBe('')
  })

  it('重试同一次拣货操作时复用相同的 Idempotency-Key', async () => {
    mockedPick.mockRejectedValueOnce({ message: '网络异常' })
    mockedPick.mockResolvedValueOnce(pickResult())
    const { pick } = usePdaPick()

    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('网络异常')
    await pick('1', { qty: 1, location_code: 'A-01' })

    const firstKey = mockedPick.mock.calls[0][2]
    const retryKey = mockedPick.mock.calls[1][2]
    expect(retryKey).toBe(firstKey)
  })

  it('成功的拣货响应会写入任务快照', async () => {
    mockedPick.mockResolvedValue(pickResult())
    const { pick, snapshot } = usePdaPick()

    await pick('1', { qty: 1, location_code: 'A-01' })

    expect(snapshot.value).toEqual(pickResult())
  })

  it('业务拒绝且后端返回快照时，同样刷新任务快照并保留错误信息', async () => {
    const rejected = pickResult({ task_status: 'CREATED', done_qty: 0, remaining_qty: 2 })
    mockedPick.mockRejectedValue({ message: '任务已被他人领取', data: rejected })
    const { pick, snapshot, lastError } = usePdaPick()

    await expect(pick('1', { qty: 1, location_code: 'A-01' })).rejects.toThrow('任务已被他人领取')

    expect(snapshot.value).toEqual(rejected)
    expect(lastError.value).toBe('任务已被他人领取')
  })

  it('新一轮操作 reset 后不再携带上一轮的领取凭证与快照', async () => {
    mockedClaim.mockResolvedValue(claimResult())
    mockedPick.mockResolvedValue(pickResult())
    const { claim, pick, reset, claimToken, snapshot } = usePdaPick()

    await claim('1')
    await pick('1', { qty: 1, location_code: 'A-01' })
    expect(claimToken.value).toBe('token-1')
    expect(snapshot.value).not.toBeNull()

    reset()
    expect(claimToken.value).toBe('')
    expect(snapshot.value).toBeNull()
  })
})
