import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { approveStocktakeOrder } from '@/api/stocktake'
import { useAuthStore } from '@/stores/auth'
import { loadPending } from '@/utils/pendingOperations'
import { useStocktakeApprove } from './useStocktakeApprove'

vi.mock('@/api/stocktake', () => ({
  approveStocktakeOrder: vi.fn(),
}))

const mockedApprove = vi.mocked(approveStocktakeOrder)

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

function loginAs(userId: string) {
  const auth = useAuthStore()
  auth.setAuth({
    token: 't',
    user_id: userId,
    username: `u${userId}`,
    nickname: 'Operator',
    roles: ['operator'],
    perms: [],
  })
}

function seedApproveRecord(userId: string, storage: Storage, overrides: Record<string, unknown> = {}) {
  storage.setItem(
    'WMS_PENDING_OPS',
    JSON.stringify({
      version: 1,
      items: [
        {
          schemaVersion: 1,
          userId,
          sessionNamespace: '',
          scope: 'stocktake.approve',
          objectId: '30',
          operationKey: 'seed-appr-key',
          businessPayload: {},
          createdAt: new Date().toISOString(),
          state: 'sending',
          ...overrides,
        },
      ],
    }),
  )
}

beforeEach(() => {
  vi.resetAllMocks()
  vi.stubGlobal('localStorage', createStorage())
  vi.stubGlobal('sessionStorage', createStorage())
  setActivePinia(createPinia())
  loginAs('1')
})

describe('useStocktakeApprove 操作 key 生命周期', () => {
  it('成功审核后的再次审核生成两个不同的新 key', async () => {
    mockedApprove.mockResolvedValue(undefined)
    const { approve } = useStocktakeApprove()

    await approve('30')
    await approve('30')

    expect(mockedApprove).toHaveBeenCalledTimes(2)
    expect(mockedApprove.mock.calls[1][1]).not.toBe(mockedApprove.mock.calls[0][1])
  })

  it('审核超时后再次点击自动按原 key 重试', async () => {
    mockedApprove.mockRejectedValueOnce({ kind: 'network', message: '网络异常' })
    mockedApprove.mockResolvedValueOnce(undefined)
    const { approve, stateOf } = useStocktakeApprove()

    await expect(approve('30')).rejects.toThrow('网络异常')
    expect(stateOf('30').pending).not.toBeNull()

    await approve('30')

    expect(mockedApprove).toHaveBeenCalledTimes(2)
    expect(mockedApprove.mock.calls[1][1]).toBe(mockedApprove.mock.calls[0][1])
    expect(stateOf('30').pending).toBeNull()
  })

  it('服务端确认未执行（未录全实盘）后可用新 key 重新审核', async () => {
    mockedApprove.mockRejectedValueOnce({ kind: 'business', code: 60006, message: '盘点单仍有未录入实盘数量的明细' })
    mockedApprove.mockResolvedValueOnce(undefined)
    const { approve, stateOf } = useStocktakeApprove()

    await expect(approve('30')).rejects.toThrow('仍有未录入实盘数量')
    expect(stateOf('30').pending).toBeNull()

    await approve('30')
    expect(mockedApprove).toHaveBeenCalledTimes(2)
    expect(mockedApprove.mock.calls[1][1]).not.toBe(mockedApprove.mock.calls[0][1])
  })

  it('版本冲突类可重试错误保留原 key 记录', async () => {
    mockedApprove.mockRejectedValueOnce({ kind: 'business', code: 60005, message: '盘点单已被其他人操作，请刷新重试' })
    const { approve, stateOf } = useStocktakeApprove()

    await expect(approve('30')).rejects.toThrow('已被其他人操作')
    expect(stateOf('30').pending).not.toBeNull()
  })

  it('双击提交不会创建两次业务操作', async () => {
    let resolveFirst!: () => void
    mockedApprove.mockReturnValueOnce(new Promise<void>((resolve) => (resolveFirst = resolve)))
    const { approve } = useStocktakeApprove()

    const first = approve('30')
    await expect(approve('30')).rejects.toThrow('请勿重复提交')

    expect(mockedApprove).toHaveBeenCalledTimes(1)
    resolveFirst()
    await first
  })
})

describe('useStocktakeApprove 刷新恢复与隔离', () => {
  it('刷新后恢复待确认审核：再次点击自动按原 key 重试并清除记录', async () => {
    seedApproveRecord('1', localStorage)
    mockedApprove.mockResolvedValue(undefined)
    const { stateOf, restoreForOrders, approve } = useStocktakeApprove()

    expect(restoreForOrders(['30'])).toBe('')
    expect(stateOf('30').pending?.operationKey).toBe('seed-appr-key')

    await approve('30')

    expect(mockedApprove.mock.calls[0][1]).toBe('seed-appr-key')
    expect(stateOf('30').pending).toBeNull()
    expect(loadPending({ userId: '1', sessionNamespace: '' }, localStorage).items).toHaveLength(0)
  })

  it('超过保留期的待确认审核不允许自动重试', async () => {
    seedApproveRecord('1', localStorage, {
      createdAt: new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString(),
    })
    const { stateOf, restoreForOrders, approve } = useStocktakeApprove()

    restoreForOrders(['30'])
    expect(stateOf('30').pending?.stale).toBe(true)

    await expect(approve('30')).rejects.toThrow('超过保留期')
    expect(mockedApprove).not.toHaveBeenCalled()
  })

  it('账号切换后看不到其他用户的待确认操作', async () => {
    seedApproveRecord('1', localStorage)
    const { stateOf, restoreForOrders } = useStocktakeApprove()
    restoreForOrders(['30'])
    expect(stateOf('30').pending).not.toBeNull()

    loginAs('2')
    restoreForOrders(['30'])
    expect(stateOf('30').pending).toBeNull()
  })

  it('存储写入失败时不发送请求并明确报错', async () => {
    vi.stubGlobal('localStorage', createFailingStorage())
    const { approve } = useStocktakeApprove()

    await expect(approve('30')).rejects.toThrow('无法保存待确认操作记录')
    expect(mockedApprove).not.toHaveBeenCalled()
  })

  it('核对后放弃待确认记录后可以开始新操作', async () => {
    seedApproveRecord('1', localStorage)
    mockedApprove.mockResolvedValue(undefined)
    const { stateOf, restoreForOrders, discard, approve } = useStocktakeApprove()

    restoreForOrders(['30'])
    discard('30')
    expect(stateOf('30').pending).toBeNull()

    await approve('30')
    expect(mockedApprove.mock.calls[0][1]).not.toBe('seed-appr-key')
  })
})