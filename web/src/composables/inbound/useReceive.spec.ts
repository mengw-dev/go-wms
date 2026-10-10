import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { receiveInbound } from '@/api/inbound'
import { useAuthStore } from '@/stores/auth'
import { loadPending } from '@/utils/pendingOperations'
import { useReceive } from './useReceive'

vi.mock('@/api/inbound', () => ({
  receiveInbound: vi.fn(),
}))

const mockedReceive = vi.mocked(receiveInbound)

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

function seedReceiveRecord(userId: string, storage: Storage, overrides: Record<string, unknown> = {}) {
  storage.setItem(
    'WMS_PENDING_OPS',
    JSON.stringify({
      version: 1,
      items: [
        {
          schemaVersion: 1,
          userId,
          sessionNamespace: '',
          scope: 'inbound.receive',
          objectId: '10',
          operationKey: 'seed-recv-key',
          businessPayload: { qty: 30, defective_qty: 5, batch_no: 'BATCH-A' },
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

const payload = { qty: 30, defective_qty: 5, batch_no: 'BATCH-A' }

describe('useReceive 操作 key 生命周期', () => {
  it('成功收货后同参数的再次收货生成两个不同的新 key', async () => {
    mockedReceive.mockResolvedValue(undefined)
    const { receive } = useReceive()

    await receive('1', '10', payload)
    await receive('1', '10', payload)

    expect(mockedReceive).toHaveBeenCalledTimes(2)
    expect(mockedReceive.mock.calls[1][2]).not.toBe(mockedReceive.mock.calls[0][2])
  })

  it('收货超时重试复用同一个 key 和同一组参数', async () => {
    mockedReceive.mockRejectedValueOnce({ kind: 'network', message: '网络异常' })
    mockedReceive.mockResolvedValueOnce(undefined)
    const { receive } = useReceive()

    await expect(receive('1', '10', payload)).rejects.toThrow('网络异常')
    await receive('1', '10', payload)

    expect(mockedReceive).toHaveBeenCalledTimes(2)
    expect(mockedReceive.mock.calls[1][2]).toBe(mockedReceive.mock.calls[0][2])
    expect(mockedReceive.mock.calls[1][1]).toEqual({ detail_id: '10', ...payload })
  })

  it('结果未知时修改参数被拒绝，不产生新请求', async () => {
    mockedReceive.mockRejectedValueOnce({ kind: 'network', message: '网络异常' })
    const { receive, stateOf } = useReceive()

    await expect(receive('1', '10', payload)).rejects.toThrow('网络异常')
    await expect(receive('1', '10', { ...payload, qty: 40 })).rejects.toThrow('存在未确认的收货操作')

    expect(mockedReceive).toHaveBeenCalledTimes(1)
    expect(stateOf('10').pending?.businessPayload.qty).toBe(30)
  })

  it('服务端确认未执行（数量超限）后可修正内容用新 key 提交', async () => {
    mockedReceive.mockRejectedValueOnce({ kind: 'business', code: 40004, message: '收货数量超过剩余应收数量' })
    mockedReceive.mockResolvedValueOnce(undefined)
    const { receive, stateOf } = useReceive()

    await expect(receive('1', '10', { ...payload, qty: 999 })).rejects.toThrow('收货数量超过剩余应收数量')
    expect(stateOf('10').pending).toBeNull()

    await receive('1', '10', payload)
    expect(mockedReceive).toHaveBeenCalledTimes(2)
    expect(mockedReceive.mock.calls[1][2]).not.toBe(mockedReceive.mock.calls[0][2])
  })

  it('版本冲突类可重试错误保留原 key 记录', async () => {
    mockedReceive.mockRejectedValueOnce({ kind: 'business', code: 40003, message: '入库单已被其他人操作，请刷新重试' })
    const { receive, stateOf } = useReceive()

    await expect(receive('1', '10', payload)).rejects.toThrow('入库单已被其他人操作')
    expect(stateOf('10').pending).not.toBeNull()
    expect(stateOf('10').pending?.businessPayload.qty).toBe(30)
  })

  it('双击提交不会创建两次业务操作', async () => {
    let resolveFirst!: () => void
    mockedReceive.mockReturnValueOnce(new Promise<void>((resolve) => (resolveFirst = resolve)))
    const { receive } = useReceive()

    const first = receive('1', '10', payload)
    await expect(receive('1', '10', payload)).rejects.toThrow('请勿重复提交')

    expect(mockedReceive).toHaveBeenCalledTimes(1)
    resolveFirst()
    await first
  })
})

describe('useReceive 刷新恢复与隔离', () => {
  it('刷新后恢复待确认收货：原 key 原参数重试，成功后清除记录', async () => {
    seedReceiveRecord('1', localStorage)
    mockedReceive.mockResolvedValue(undefined)
    const { stateOf, restoreForDetails, recoverReceive } = useReceive()

    expect(restoreForDetails(['10'])).toBe('')
    expect(stateOf('10').pending?.operationKey).toBe('seed-recv-key')
    expect(stateOf('10').pending?.stale).toBe(false)

    await recoverReceive('1', '10')

    expect(mockedReceive.mock.calls[0][2]).toBe('seed-recv-key')
    expect(mockedReceive.mock.calls[0][1]).toEqual({ detail_id: '10', ...payload })
    expect(stateOf('10').pending).toBeNull()
    expect(loadPending({ userId: '1', sessionNamespace: '' }, localStorage).items).toHaveLength(0)
  })

  it('超过保留期的待确认收货不允许自动重试', async () => {
    seedReceiveRecord('1', localStorage, {
      createdAt: new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString(),
    })
    const { stateOf, restoreForDetails, recoverReceive } = useReceive()

    restoreForDetails(['10'])
    expect(stateOf('10').pending?.stale).toBe(true)

    await expect(recoverReceive('1', '10')).rejects.toThrow('超过保留期')
    expect(mockedReceive).not.toHaveBeenCalled()
  })

  it('账号切换后看不到其他用户的待确认操作', async () => {
    seedReceiveRecord('1', localStorage)
    const { stateOf, restoreForDetails } = useReceive()
    restoreForDetails(['10'])
    expect(stateOf('10').pending).not.toBeNull()

    loginAs('2')
    restoreForDetails(['10'])
    expect(stateOf('10').pending).toBeNull()
  })

  it('存储写入失败时不发送请求并明确报错', async () => {
    vi.stubGlobal('localStorage', createFailingStorage())
    const { receive } = useReceive()

    await expect(receive('1', '10', payload)).rejects.toThrow('无法保存待确认操作记录')
    expect(mockedReceive).not.toHaveBeenCalled()
  })

  it('核对后放弃待确认记录后可以开始新操作', async () => {
    seedReceiveRecord('1', localStorage)
    mockedReceive.mockResolvedValue(undefined)
    const { stateOf, restoreForDetails, discardReceive, receive } = useReceive()

    restoreForDetails(['10'])
    discardReceive('10')
    expect(stateOf('10').pending).toBeNull()

    await receive('1', '10', payload)
    expect(mockedReceive.mock.calls[0][2]).not.toBe('seed-recv-key')
  })
})