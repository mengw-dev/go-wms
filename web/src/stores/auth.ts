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

function safeParseUser(raw: string | null): AuthUser | null {
  if (!raw) return null
  try {
    return JSON.parse(raw) as AuthUser
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem(TOKEN_KEY) || '',
    user: safeParseUser(localStorage.getItem(USER_KEY)),
    demoSessionId: sessionStorage.getItem(DEMO_SESSION_KEY) || '',
    demoSessionExpiresIn: 0,
  }),
  getters: {
    isLoggedIn: (state) => !!state.token,
    isDemo: (state) => (state.user?.perms ?? []).includes('wms:demo'),
    displayName: (state) => state.user?.nickname || state.user?.username || '未知用户',
    perms: (state) => state.user?.perms ?? [],
    hasPerm: (state) => (perm: string) =>
      state.user?.user_id === '1' || (state.user?.perms ?? []).some((p) => p === '*' || p === perm),
  },
  actions: {
    setAuth(result: LoginResult) {
      this.clearDemoSession()
      this.token = result.token
      this.user = {
        user_id: result.user_id,
        username: result.username,
        nickname: result.nickname,
        roles: result.roles ?? [],
        perms: result.perms ?? [],
      }
      localStorage.setItem(TOKEN_KEY, result.token)
      localStorage.setItem(USER_KEY, JSON.stringify(this.user))
    },
    setProfile(profile: ProfileResult) {
      this.user = {
        user_id: profile.user_id,
        username: profile.username,
        nickname: profile.nickname,
        roles: profile.roles ?? [],
        perms: profile.perms ?? [],
      }
      localStorage.setItem(USER_KEY, JSON.stringify(this.user))
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
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(USER_KEY)
    },
  },
})
