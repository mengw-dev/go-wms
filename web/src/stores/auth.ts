import { defineStore } from 'pinia'
import type { LoginResult, ProfileResult } from '@/api/types'

const TOKEN_KEY = 'WMS_TOKEN'
const USER_KEY = 'WMS_USER'

export interface AuthUser {
  user_id: number
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
  }),
  getters: {
    isLoggedIn: (state) => !!state.token,
    displayName: (state) => state.user?.nickname || state.user?.username || '未知用户',
    perms: (state) => state.user?.perms ?? [],
  },
  actions: {
    setAuth(result: LoginResult) {
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
    clear() {
      this.token = ''
      this.user = null
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(USER_KEY)
    },
  },
})
