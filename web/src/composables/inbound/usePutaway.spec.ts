import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { putawayInboundTask } from '@/api/inbound'
import { useAuthStore } from '@/stores/auth'
import { loadPending } from '@/utils/pendingOperations'
import { usePutaway } from './usePutaway'

vi.mock('@/api/inbound', () => ({
  putawayInboundTask: vi.fn(),
}))

const mockedPutaway = vi.mocked(putawayInboundTask)

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

function seedPutawayRecord(userId: string, storage: Storage, overrides: Record<string, unknown> = {}) {
  storage.setItem(
    'WMS_PENDING_OPS',
    JSON.stringify({
      version: 1,
      items: [
        {
          schemaVersion: 1,
          userId,
          sessionNamespace: '',
          scope: 'inbound.putaway',
          objectId: '20',
          operationKey: 'seed-pa-key',
          businessPayload: { location_id: '7', qty: 4 },
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

const payload = { location_id: '7', qty: 4 }

describe('usePutaway 操作 key 生命周期', () => {
  it('成功上架后同参数的再次上架生成两个不同的新 key', async () => {
    mockedPutaway.mockResolvedValue(undefined)
    const { putaway } = usePutaway()

    await putaway('20', payload)
    await putaway('20', payload)

    expect(mockedPutaway).toHaveBeenCalledTimes(2)
    expect(mockedPutaway.mock.calls[1][2]).not.toBe(mockedPutaway.mock.calls[0][2])
  })

  it('上架超时重试复用同一个 key 和同一组参数', async () => {
    mockedPutaway.mockRejectedValueOnce({ kind: 'network', message: '网络异常' })
    mockedPutaway.mockResolvedValueOnce(undefined)
    const { putaway } = usePutaway()

    await expect(putaway('20', payload)).rejects.toThrow('网络异常')
    await putaway('20', payload)

    expect(mockedPutaway).toHaveBeenCalledTimes(2)
    expect(mockedPutaway.mock.calls[1][2]).toBe(mockedPutaway.mock.calls[0][2])
    expect(mockedPutaway.mock.calls[1][1]).toEqual(payload)
  })

  it('结果未知时修改参数被拒绝，不产生新请求', async () => {
    mockedPutaway.mockRejectedValueOnce({ kind: 'network', message: '网络异常' })
    const { putaway, stateOf } = usePutaway()

    await expect(putaway('20', payload)).rejects.toThrow('网络异常')
    await expect(putaway('20', { ...payload, qty: 8 })).rejects.toThrow('存在未确认的上架操作')

    expect(mockedPutaway).toHaveBeenCalledTimes(1)
    expect(stateOf('20').pending?.businessPayload.qty).toBe(4)
  })

  it('服务端确认未执行（数量超限）后可修正内容用新 key 提交', async () => {
    mockedPutaway.mockRejectedValueOnce({ kind: 'business', code: 40005, message: '上架数量超过任务剩余数量' })
    mockedPutaway.mockResolvedValueOnce(undefined)
    const { putaway, stateOf } = usePutaway()

    await expect(putaway('20', { ...payload, qty: 999 })).rejects.toThrow('上架数量超过任务剩余数量')
    expect(stateOf('20').pending).toBeNull()

    await putaway('20', payload)
    expect(mockedPutaway).toHaveBeenCalledTimes(2)
    expect(mockedPutaway.mock.calls[1][2]).not.toBe(mockedPutaway.mock.calls[0][2])
  })

  it('状态/库存类错误保留原 key 记录，不擅自删除', async () => {
    mockedPutaway.mockRejectedValueOnce({ kind: 'business', code: 30201, message: '可用库存不足' })
    const { putaway, stateOf } = usePutaway()

    await expect(putaway('20', payload)).rejects.toThrow('可用库存不足')
    expect(stateOf('20').pending).not.toBeNull()
  })

  it('双击提交不会创建两次业务操作', async () => {
    let resolveFirst!: () => void
    mockedPutaway.mockReturnValueOnce(new Promise<void>((resolve) => (resolveFirst = resolve)))
    const { putaway } = usePutaway()

    const first = putaway('20', payload)
    await expect(putaway('20', payload)).rejects.toThrow('请勿重复提交')

    expect(mockedPutaway).toHaveBeenCalledTimes(1)
    resolveFirst()
    await first
  })
})

describe('usePutaway 刷新恢复与隔离', () => {
  it('刷新后恢复待确认上架：原 key 原参数重试，成功后清除记录', async () => {
    seedPutawayRecord('1', localStorage)
    mockedPutaway.mockResolvedValue(undefined)
    const { stateOf, restoreForTasks, recoverPutaway } = usePutaway()

    expect(restoreForTasks(['20'])).toBe('')
    expect(stateOf('20').pending?.operationKey).toBe('seed-pa-key')

    await recoverPutaway('20')

    expect(mockedPutaway.mock.calls[0][2]).toBe('seed-pa-key')
    expect(mockedPutaway.mock.calls[0][1]).toEqual(payload)
    expect(stateOf('20').pending).toBeNull()
    expect(loadPending({ userId: '1', sessionNamespace: '' }, localStorage).items).toHaveLength(0)
  })

  it('超过保留期的待确认上架不允许自动重试', async () => {
    seedPutawayRecord('1', localStorage, {
      createdAt: new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString(),
    })
    const { stateOf, restoreForTasks, recoverPutaway } = usePutaway()

    restoreForTasks(['20'])
    expect(stateOf('20').pending?.stale).toBe(true)

    await expect(recoverPutaway('20')).rejects.toThrow('超过保留期')
    expect(mockedPutaway).not.toHaveBeenCalled()
  })

  it('账号切换后看不到其他用户的待确认操作', async () => {
    seedPutawayRecord('1', localStorage)
    const { stateOf, restoreForTasks } = usePutaway()
    restoreForTasks(['20'])
    expect(stateOf('20').pending).not.toBeNull()

    loginAs('2')
    restoreForTasks(['20'])
    expect(stateOf('20').pending).toBeNull()
  })

  it('存储写入失败时不发送请求并明确报错', async () => {
    vi.stubGlobal('localStorage', createFailingStorage())
    const { putaway } = usePutaway()

    await expect(putaway('20', payload)).rejects.toThrow('无法保存待确认操作记录')
    expect(mockedPutaway).not.toHaveBeenCalled()
  })

  it('核对后放弃待确认记录后可以开始新操作', async () => {
    seedPutawayRecord('1', localStorage)
    mockedPutaway.mockResolvedValue(undefined)
    const { stateOf, restoreForTasks, discardPutaway, putaway } = usePutaway()

    restoreForTasks(['20'])
    discardPutaway('20')
    expect(stateOf('20').pending).toBeNull()

    await putaway('20', payload)
    expect(mockedPutaway.mock.calls[0][2]).not.toBe('seed-pa-key')
  })
})