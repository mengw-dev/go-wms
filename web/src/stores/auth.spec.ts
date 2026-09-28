import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { LoginResult } from '@/api/types'
import { useAuthStore } from './auth'

const TOKEN_KEY = 'WMS_TOKEN'
const USER_KEY = 'WMS_USER'

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

function userJSON(overrides: Record<string, unknown> = {}): string {
  return JSON.stringify({
    user_id: '2',
    username: 'operator',
    nickname: 'Operator',
    roles: ['operator'],
    perms: ['wms:inbound:view'],
    ...overrides,
  })
}

describe('auth store permissions', () => {
  beforeEach(() => {
    // session 与 local 必须是两份独立存储，否则无法验证跨存储拼接。
    vi.stubGlobal('localStorage', createStorage())
    vi.stubGlobal('sessionStorage', createStorage())
    setActivePinia(createPinia())
  })

  it('grants only explicitly listed permissions without a wildcard', () => {
    const auth = useAuthStore()
    auth.user = {
      user_id: '2',
      username: 'operator',
      nickname: 'Operator',
      roles: ['operator'],
      perms: ['wms:inbound:view'],
    }
    expect(auth.hasPerm('wms:inbound:view')).toBe(true)
    expect(auth.hasPerm('wms:system:user')).toBe(false)
  })

  it('does not grant permissions based on user ID 1', () => {
    const auth = useAuthStore()
    auth.user = { user_id: '1', username: 'admin', nickname: 'Admin', roles: [], perms: [] }
    expect(auth.hasPerm('wms:any:permission')).toBe(false)
  })

  it('grants arbitrary permissions with a wildcard regardless of user ID', () => {
    const auth = useAuthStore()
    auth.user = { user_id: '2', username: 'admin', nickname: 'Admin', roles: [], perms: ['*'] }
    expect(auth.hasPerm('wms:inbound:view')).toBe(true)
    expect(auth.hasPerm('wms:any:permission')).toBe(true)
  })
})

describe('auth store 登录态读取', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', createStorage())
    vi.stubGlobal('sessionStorage', createStorage())
    setActivePinia(createPinia())
  })

  it('读取完整的 session 登录态', () => {
    sessionStorage.setItem(TOKEN_KEY, 'session-token')
    sessionStorage.setItem(USER_KEY, userJSON())

    const auth = useAuthStore()

    expect(auth.isLoggedIn).toBe(true)
    expect(auth.token).toBe('session-token')
    expect(auth.user?.username).toBe('operator')
  })

  it('读取完整的 local 登录态', () => {
    localStorage.setItem(TOKEN_KEY, 'local-token')
    localStorage.setItem(USER_KEY, userJSON({ username: 'remembered' }))

    const auth = useAuthStore()

    expect(auth.token).toBe('local-token')
    expect(auth.user?.username).toBe('remembered')
  })

  it('session 与 local 同时完整时以 session 为准', () => {
    sessionStorage.setItem(TOKEN_KEY, 'session-token')
    sessionStorage.setItem(USER_KEY, userJSON({ username: 'session-user' }))
    localStorage.setItem(TOKEN_KEY, 'local-token')
    localStorage.setItem(USER_KEY, userJSON({ username: 'local-user' }))

    const auth = useAuthStore()

    expect(auth.token).toBe('session-token')
    expect(auth.user?.username).toBe('session-user')
  })

  it('不跨存储拼接 token：session 只有 token、local 只有 user 时视为未登录', () => {
    sessionStorage.setItem(TOKEN_KEY, 'session-token')
    localStorage.setItem(USER_KEY, userJSON())

    const auth = useAuthStore()

    expect(auth.isLoggedIn).toBe(false)
    expect(auth.user).toBeNull()
  })

  it('只有 token 没有 user 时清理残留并视为未登录', () => {
    sessionStorage.setItem(TOKEN_KEY, 'session-token')

    const auth = useAuthStore()

    expect(auth.isLoggedIn).toBe(false)
    expect(sessionStorage.getItem(TOKEN_KEY)).toBeNull()
  })

  it('只有 user 没有 token 时清理残留并视为未登录', () => {
    localStorage.setItem(USER_KEY, userJSON())

    const auth = useAuthStore()

    expect(auth.isLoggedIn).toBe(false)
    expect(localStorage.getItem(USER_KEY)).toBeNull()
  })

  it('user JSON 损坏时清理残留并视为未登录', () => {
    localStorage.setItem(TOKEN_KEY, 'local-token')
    localStorage.setItem(USER_KEY, '{not-json')

    const auth = useAuthStore()

    expect(auth.isLoggedIn).toBe(false)
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull()
    expect(localStorage.getItem(USER_KEY)).toBeNull()
  })

  it('user 缺少关键字段时视为未登录', () => {
    localStorage.setItem(TOKEN_KEY, 'local-token')
    localStorage.setItem(USER_KEY, JSON.stringify({ nickname: '没有账号' }))

    const auth = useAuthStore()

    expect(auth.isLoggedIn).toBe(false)
  })

  it('session 损坏时回退到完整的 local 登录态', () => {
    sessionStorage.setItem(TOKEN_KEY, 'broken-session-token')
    localStorage.setItem(TOKEN_KEY, 'local-token')
    localStorage.setItem(USER_KEY, userJSON({ username: 'local-user' }))

    const auth = useAuthStore()

    expect(auth.token).toBe('local-token')
    expect(auth.user?.username).toBe('local-user')
    expect(sessionStorage.getItem(TOKEN_KEY)).toBeNull()
  })
})

describe('auth store 登录态写入与清理', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', createStorage())
    vi.stubGlobal('sessionStorage', createStorage())
    setActivePinia(createPinia())
  })

  function loginResult(overrides: Partial<LoginResult> = {}): LoginResult {
    return {
      token: 'token-1',
      user_id: '2',
      username: 'operator',
      nickname: 'Operator',
      roles: ['operator'],
      perms: ['wms:inbound:view'],
      ...overrides,
    }
  }

  it('普通账号写入 localStorage，并清掉 session 残留', () => {
    sessionStorage.setItem(TOKEN_KEY, 'stale-token')
    sessionStorage.setItem(USER_KEY, userJSON())

    const auth = useAuthStore()
    auth.setAuth(loginResult())

    expect(localStorage.getItem(TOKEN_KEY)).toBe('token-1')
    expect(localStorage.getItem(USER_KEY)).toContain('operator')
    expect(sessionStorage.getItem(TOKEN_KEY)).toBeNull()
    expect(sessionStorage.getItem(USER_KEY)).toBeNull()
  })

  it('演示账号写入 sessionStorage，并清掉 local 残留', () => {
    localStorage.setItem(TOKEN_KEY, 'stale-token')
    localStorage.setItem(USER_KEY, userJSON())

    const auth = useAuthStore()
    auth.setAuth(loginResult({ perms: ['wms:demo'] }))

    expect(sessionStorage.getItem(TOKEN_KEY)).toBe('token-1')
    expect(sessionStorage.getItem(USER_KEY)).toContain('operator')
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull()
    expect(auth.isDemo).toBe(true)
  })

  it('setProfile 只更新 user，不改动 token', () => {
    const auth = useAuthStore()
    auth.setAuth(loginResult())

    auth.setProfile({
      user_id: '2',
      username: 'operator',
      nickname: '新昵称',
      roles: ['operator'],
      perms: ['wms:outbound:view'],
    })

    expect(auth.user?.nickname).toBe('新昵称')
    expect(localStorage.getItem(TOKEN_KEY)).toBe('token-1')
    expect(JSON.parse(localStorage.getItem(USER_KEY) ?? '{}').nickname).toBe('新昵称')
  })

  it('logout 清理两份存储', () => {
    const auth = useAuthStore()
    auth.setAuth(loginResult())
    sessionStorage.setItem(TOKEN_KEY, 'stale-token')

    auth.clear()

    expect(auth.isLoggedIn).toBe(false)
    expect(auth.user).toBeNull()
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull()
    expect(localStorage.getItem(USER_KEY)).toBeNull()
    expect(sessionStorage.getItem(TOKEN_KEY)).toBeNull()
  })
})
