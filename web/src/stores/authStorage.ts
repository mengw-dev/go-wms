import type { EntityID } from '@/api/types'

const TOKEN_KEY = 'WMS_TOKEN'
const USER_KEY = 'WMS_USER'
const CURRENT_KEY = 'WMS_AUTH_CURRENT_V2'
const RECORD_PREFIX = 'WMS_AUTH_RECORD_V2:'
const PROFILE_PREFIX = 'WMS_AUTH_PROFILE_V2:'

export interface AuthUser {
  user_id: EntityID
  username: string
  nickname: string
  roles: string[]
  perms: string[]
}
export type AuthStorageSource = 'session' | 'persistent' | 'legacy' | 'none'
export interface StoredAuth {
  token: string
  user: AuthUser | null
  source: AuthStorageSource
  id: string | null
  marker: string
}

function parseUser(raw: string | null): AuthUser | null {
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as Partial<AuthUser> | null
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null
    if (typeof value.user_id !== 'string' || !value.user_id || typeof value.username !== 'string' || !value.username) return null
    const strings = (items: unknown) => Array.isArray(items) ? items.filter((v): v is string => typeof v === 'string') : []
    return { user_id: value.user_id, username: value.username, nickname: typeof value.nickname === 'string' ? value.nickname : '', roles: strings(value.roles), perms: strings(value.perms) }
  } catch { return null }
}
function clearPair(storage: Storage) {
  storage.removeItem(TOKEN_KEY); storage.removeItem(USER_KEY)
}
function readPair(storage: Storage) {
  const token = storage.getItem(TOKEN_KEY) || '', raw = storage.getItem(USER_KEY), user = parseUser(raw)
  if (token && user) return { token, user }
  if (token || raw) clearPair(storage)
  return null
}
function marker(id: string | null, record: string | null, legacyUser: string | null = null) {
  return JSON.stringify([id, record, legacyUser])
}
export function persistentAuthMarker(): string {
  const id = localStorage.getItem(CURRENT_KEY)
  return id === null
    ? marker(null, localStorage.getItem(TOKEN_KEY), localStorage.getItem(USER_KEY))
    : marker(id, localStorage.getItem(RECORD_PREFIX + id))
}
function parseRecord(raw: string | null): { token: string; user: AuthUser } | null {
  try {
    const record = JSON.parse(raw || 'null')
    const user = parseUser(JSON.stringify(record?.user))
    return typeof record?.token === 'string' && record.token && user ? { token: record.token, user } : null
  } catch { return null }
}

export function readStoredAuth(): StoredAuth {
  const session = readPair(sessionStorage)
  if (session) return { ...session, source: 'session', id: null, marker: '' }
  const id = localStorage.getItem(CURRENT_KEY)
  if (id === null) {
    // 旧格式只读兼容，不自动迁移，以免旧标签页的迁移覆盖正在发布的新登录。
    const token = localStorage.getItem(TOKEN_KEY), raw = localStorage.getItem(USER_KEY), user = parseUser(raw)
    if (token && user) return { token, user, source: 'legacy', id, marker: marker(null, token, raw) }
    if (token || raw) clearPair(localStorage)
    return { token: '', user: null, source: 'none', id, marker: marker(null, null) }
  }
  const raw = localStorage.getItem(RECORD_PREFIX + id), record = parseRecord(raw)
  const profile = parseUser(localStorage.getItem(PROFILE_PREFIX + id))
  return {
    token: record?.token || '', user: record && profile?.user_id === record.user.user_id ? profile : record?.user || null,
    source: 'persistent', id, marker: marker(id, raw),
  }
}

/** 仅回收已过期JWT的不可变记录；不能按“当前未选中”删除另一页尚未发布的登录。 */
function pruneExpiredRecords() {
  const keys: string[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i)
    if (key?.startsWith(RECORD_PREFIX)) keys.push(key)
  }
  for (const key of keys) {
    try {
      const record = parseRecord(localStorage.getItem(key))
      const payload = record?.token.split('.')[1]
      if (!payload) continue
      const exp: unknown = JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/'))).exp
      if (typeof exp !== 'number' || !Number.isFinite(exp) || exp * 1000 > Date.now()) continue
      localStorage.removeItem(key)
      localStorage.removeItem(PROFILE_PREFIX + key.slice(RECORD_PREFIX.length))
    } catch { /* 非JWT/损坏记录不据此删除；真正鉴权仍由服务端完成。 */ }
  }
  const profiles: string[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i)
    if (key?.startsWith(PROFILE_PREFIX)) profiles.push(key)
  }
  for (const key of profiles) {
    // 迟到资料最多留下无凭据的孤立昵称/权限缓存；ID不复用，可以安全回收。
    if (localStorage.getItem(RECORD_PREFIX + key.slice(PROFILE_PREFIX.length)) === null) localStorage.removeItem(key)
  }
}

export function writeStoredAuth(token: string, user: AuthUser, isDemo: boolean): StoredAuth {
  if (isDemo) {
    // 体验会话只覆盖本标签页，不能退出其他标签页的普通账号。
    sessionStorage.setItem(TOKEN_KEY, token)
    sessionStorage.setItem(USER_KEY, JSON.stringify(user))
    return { token, user, source: 'session', id: null, marker: '' }
  }
  pruneExpiredRecords()
  // 不依赖跨标签页的读改写计数器；getRandomValues也支持局域网HTTP环境。
  const id = Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('')
  const previousID = localStorage.getItem(CURRENT_KEY), raw = JSON.stringify({ token, user })
  localStorage.setItem(RECORD_PREFIX + id, raw)
  try { localStorage.setItem(CURRENT_KEY, id) }
  catch (error) { localStorage.removeItem(RECORD_PREFIX + id); throw error }
  clearPair(sessionStorage)
  clearPair(localStorage)
  // ID只创建一次、永不复用；删除此前ID不会删除另一页的新记录。
  if (previousID !== null) {
    localStorage.removeItem(RECORD_PREFIX + previousID)
    localStorage.removeItem(PROFILE_PREFIX + previousID)
  }
  return { token, user, source: 'persistent', id, marker: marker(id, raw) }
}

export function writeStoredProfile(user: AuthUser, source: AuthStorageSource, id: string | null) {
  if (source === 'session') sessionStorage.setItem(USER_KEY, JSON.stringify(user))
  else if (source === 'persistent' && id !== null) localStorage.setItem(PROFILE_PREFIX + id, JSON.stringify(user))
  // 旧格式仅在内存刷新资料；新登录才建立V2记录，避免写回共享旧凭据。
}

export function removeStoredAuth(source: AuthStorageSource, id: string | null): string {
  if (source === 'session') { clearPair(sessionStorage); return '' }
  if (source === 'persistent' && id !== null) {
    localStorage.removeItem(RECORD_PREFIX + id)
    localStorage.removeItem(PROFILE_PREFIX + id)
    // 保留指针（允许指向不存在的记录表示退出），绝不比较后删除共享指针。
    return marker(id, null)
  }
  clearPair(localStorage)
  return marker(null, null)
}
