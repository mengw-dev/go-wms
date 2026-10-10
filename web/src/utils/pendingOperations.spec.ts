import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { EntityID } from '@/api/types'
import {
  PENDING_CONTAINER_VERSION,
  PENDING_RETENTION_MS,
  findPending,
  isStale,
  loadPending,
  removePending,
  upsertPending,
  type OperationIdentity,
  type PendingOperation,
} from './pendingOperations'

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

const identityA: OperationIdentity = { userId: '1', sessionNamespace: '' }
const identityB: OperationIdentity = { userId: '2', sessionNamespace: '' }

function makeOperation(overrides: Partial<PendingOperation> = {}): PendingOperation {
  return {
    schemaVersion: 1,
    userId: '1',
    sessionNamespace: '',
    scope: 'outbound.pick',
    objectId: '1',
    operationKey: 'key-1',
    businessPayload: { qty: 1, location_code: 'A-01' },
    createdAt: new Date().toISOString(),
    state: 'sending',
    ...overrides,
  }
}

let storage: Storage

beforeEach(() => {
  storage = createStorage()
  vi.stubGlobal('localStorage', storage)
})

describe('pendingOperations', () => {
  it('写入后可查找、加载并删除同一条操作', () => {
    const op = makeOperation()
    expect(upsertPending(identityA, storage, op)).toEqual({ ok: true, error: '' })

    expect(findPending(identityA, storage, 'outbound.pick', '1' as EntityID)?.operationKey).toBe('key-1')
    // loadPending 会把 sending 归一化为 uncertain（刷新恢复语义）。
    expect(loadPending(identityA, storage)).toEqual({ items: [{ ...op, state: 'uncertain' }], error: '' })

    expect(removePending(identityA, storage, 'outbound.pick', '1' as EntityID, 'key-1').ok).toBe(true)
    expect(findPending(identityA, storage, 'outbound.pick', '1' as EntityID)).toBeNull()
  })

  it('upsert 只更新同 key 的记录，不覆盖同对象下其他 key 的记录', () => {
    upsertPending(identityA, storage, makeOperation({ operationKey: 'key-1', state: 'sending' }))
    upsertPending(identityA, storage, makeOperation({ operationKey: 'key-2', state: 'uncertain' }))
    upsertPending(identityA, storage, makeOperation({ operationKey: 'key-1', state: 'uncertain' }))

    const loaded = loadPending(identityA, storage)
    expect(loaded.items).toHaveLength(2)
    expect(loaded.items.find((item) => item.operationKey === 'key-1')?.state).toBe('uncertain')
    expect(loaded.items.find((item) => item.operationKey === 'key-2')?.state).toBe('uncertain')
  })

  it('刷新恢复时 prepared/sending 统一按 uncertain 返回', () => {
    upsertPending(identityA, storage, makeOperation({ state: 'prepared' }))
    const loaded = loadPending(identityA, storage)
    expect(loaded.items[0].state).toBe('uncertain')
  })

  it('存储内容不是合法 JSON 时报错且不返回任何条目', () => {
    storage.setItem('WMS_PENDING_OPS', '{not json')
    const loaded = loadPending(identityA, storage)
    expect(loaded.items).toHaveLength(0)
    expect(loaded.error).toContain('损坏')
  })

  it('容器版本不支持时报错', () => {
    storage.setItem('WMS_PENDING_OPS', JSON.stringify({ version: PENDING_CONTAINER_VERSION + 1, items: [] }))
    const loaded = loadPending(identityA, storage)
    expect(loaded.error).toContain('版本不支持')
  })

  it('条目字段非法（状态不是枚举、ID 不是字符串）视为损坏', () => {
    for (const bad of [makeOperation({ state: 'banana' as never }), { ...makeOperation(), objectId: 123 }]) {
      storage.setItem('WMS_PENDING_OPS', JSON.stringify({ version: PENDING_CONTAINER_VERSION, items: [bad] }))
      const loaded = loadPending(identityA, storage)
      expect(loaded.error).toContain('损坏')
    }
  })

  it('不同用户之间的记录互相隔离', () => {
    upsertPending(identityA, storage, makeOperation())
    expect(loadPending(identityB, storage).items).toHaveLength(0)
    expect(findPending(identityB, storage, 'outbound.pick', '1' as EntityID)).toBeNull()
  })

  it('同一用户不同演示会话（sessionNamespace）之间互相隔离', () => {
    const sessionX: OperationIdentity = { userId: '9', sessionNamespace: 's-x' }
    const sessionY: OperationIdentity = { userId: '9', sessionNamespace: 's-y' }
    upsertPending(sessionX, storage, makeOperation({ userId: '9', sessionNamespace: 's-x' }))
    expect(findPending(sessionY, storage, 'outbound.pick', '1' as EntityID)).toBeNull()
    expect(findPending(sessionX, storage, 'outbound.pick', '1' as EntityID)?.operationKey).toBe('key-1')
  })

  it('存储写入失败时明确返回错误', () => {
    const failing = createFailingStorage()
    const result = upsertPending(identityA, failing, makeOperation())
    expect(result.ok).toBe(false)
    expect(result.error).toContain('无法保存')
  })

  it('读取不可用的存储时返回明确错误', () => {
    const failing = createFailingStorage()
    failing.getItem = () => {
      throw new Error('SecurityError')
    }
    const loaded = loadPending(identityA, failing)
    expect(loaded.error).toContain('存储不可用')
  })

  it('按保留窗口判断是否过期：超期不能再自动重试', () => {
    const fresh = makeOperation({ createdAt: new Date(Date.now() - 1000).toISOString() })
    const expired = makeOperation({ createdAt: new Date(Date.now() - PENDING_RETENTION_MS - 1000).toISOString() })
    expect(isStale(fresh)).toBe(false)
    expect(isStale(expired)).toBe(true)
  })
})