import { defineStore } from 'pinia'
import type { DemoSessionInfo, EntityID, LoginResult, ProfileResult } from '@/api/types'

const TOKEN_KEY = 'WMS_TOKEN'
const USER_KEY = 'WMS_USER'
const DEMO_SESSION_KEY = 'WMS_DEMO_SESSION'

export interface AuthUser {
  user_id: EntityID
  username: string
  nickname: string
  roles: string[]
  perms: string[]
}

interface AuthPair {
  token: string
  user: AuthUser
}

function parseStringList(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string')
}

/**
 * 解析 user 原始数据。JSON 损坏或缺少关键字段时返回 null，
 * 调用方据此把整份登录态视为无效。
 */
function parseUser(raw: string | null): AuthUser | null {
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as Partial<AuthUser> | null
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null
    if (typeof value.user_id !== 'string' || !value.user_id) return null
    if (typeof value.username !== 'string' || !value.username) return null
    return {
      user_id: value.user_id,
      username: value.username,
      nickname: typeof value.nickname === 'string' ? value.nickname : '',
      roles: parseStringList(value.roles),
      perms: parseStringList(value.perms),
    }
  } catch {
    return null
  }
}

/**
 * 读取单个存储介质中的登录态。
 * token 与 user 必须同时存在且 user 可解析，否则清掉这份残留并返回 null，
 * 避免出现“有 token 无 user”或“有 user 无 token”的拼接结果。
 */
function readAuthPair(storage: Storage): AuthPair | null {
  const rawToken = storage.getItem(TOKEN_KEY) || ''
  const rawUser = storage.getItem(USER_KEY)
  const user = parseUser(rawUser)
  if (rawToken && user) return { token: rawToken, user }
  if (rawToken || rawUser) clearAuthPair(storage)
  return null
}

function readSessionAuth(): AuthPair | null {
  return readAuthPair(sessionStorage)
}

function readPersistentAuth(): AuthPair | null {
  return readAuthPair(localStorage)
}

/**
 * 写入登录态。token 与 user 始终写进同一个存储介质，
 * 并先清掉另一份，避免两份存储混用。
 */
function writeAuth(token: string, user: AuthUser, isDemo: boolean): void {
  const storage = pickStorage(isDemo)
  clearAuthPair(localStorage)
  clearAuthPair(sessionStorage)
  storage.setItem(TOKEN_KEY, token)
  storage.setItem(USER_KEY, JSON.stringify(user))
}

function writeUser(user: AuthUser, isDemo: boolean): void {
  pickStorage(isDemo).setItem(USER_KEY, JSON.stringify(user))
}

function clearAuthPair(storage: Storage): void {
  storage.removeItem(TOKEN_KEY)
  storage.removeItem(USER_KEY)
}

function clearAuth(): void {
  clearAuthPair(localStorage)
  clearAuthPair(sessionStorage)
}

/**
 * 初始化登录态：session 与 local 不能混拼。
 * 完整 session 登录态优先；否则取完整 local 登录态；都不完整即未登录。
 */
function readInitialAuth(): AuthPair | null {
  return readSessionAuth() ?? readPersistentAuth()
}

/**
 * 选择存储介质：演示账号使用 sessionStorage，关闭标签页即登出；
 * 普通账号使用 localStorage，保持登录状态。
 */
function pickStorage(isDemo: boolean): Storage {
  return isDemo ? sessionStorage : localStorage
}

export const useAuthStore = defineStore('auth', {
  state: () => {
    const auth = readInitialAuth()
    return {
      token: auth?.token ?? '',
      user: auth?.user ?? null,
      demoSessionId: sessionStorage.getItem(DEMO_SESSION_KEY) || '',
      demoSessionExpiresIn: 0,
    }
  },
  getters: {
    isLoggedIn: (state) => !!state.token,
    isDemo: (state) => (state.user?.perms ?? []).includes('wms:demo'),
    displayName: (state) => state.user?.nickname || state.user?.username || '未知用户',
    perms: (state) => state.user?.perms ?? [],
    hasPerm: (state) => (perm: string) =>
      (state.user?.perms ?? []).some((p) => p === '*' || p === perm),
  },
  actions: {
    setAuth(result: LoginResult) {
      this.clearDemoSession()
      const isDemo = (result.perms ?? []).includes('wms:demo')
      const user: AuthUser = {
        user_id: result.user_id,
        username: result.username,
        nickname: result.nickname,
        roles: result.roles ?? [],
        perms: result.perms ?? [],
      }
      writeAuth(result.token, user, isDemo)
      this.token = result.token
      this.user = user
    },
    setProfile(profile: ProfileResult) {
      const isDemo = (profile.perms ?? []).includes('wms:demo')
      const user: AuthUser = {
        user_id: profile.user_id,
        username: profile.username,
        nickname: profile.nickname,
        roles: profile.roles ?? [],
        perms: profile.perms ?? [],
      }
      this.user = user
      writeUser(user, isDemo)
    },
    setDemoSession(info: DemoSessionInfo) {
      this.demoSessionId = info.session_id
      this.demoSessionExpiresIn = info.expires_in
      sessionStorage.setItem(DEMO_SESSION_KEY, info.session_id)
    },
    clearDemoSession() {
      this.demoSessionId = ''
      this.demoSessionExpiresIn = 0
      sessionStorage.removeItem(DEMO_SESSION_KEY)
    },
    clear() {
      this.clearDemoSession()
      this.token = ''
      this.user = null
      clearAuth()
    },
  },
})
