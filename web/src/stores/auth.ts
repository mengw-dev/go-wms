import { defineStore } from 'pinia'
import type { DemoSessionInfo, EntityID, LoginResult, ProfileResult } from '@/api/types'
import { persistentAuthMarker, readStoredAuth, removeStoredAuth, writeStoredAuth, writeStoredProfile, type AuthUser } from './authStorage'
export type { AuthUser } from './authStorage'

const DEMO_SESSION_KEY = 'WMS_DEMO_SESSION'
/** 捕获请求发起时的身份；版本区分退出后以相同 token 再次登录的情况。 */
export interface AuthSnapshot {
  version: number
  token: string
  userID: EntityID | undefined
  demoSession: string
}

export const useAuthStore = defineStore('auth', {
  state: () => {
    const auth = readStoredAuth()
    return {
      token: auth?.token ?? '',
      user: auth?.user ?? null,
      storageSource: auth.source,
      persistentID: auth.id,
      persistentMarker: auth.marker,
      externalSessionChanged: false,
      demoSessionId: sessionStorage.getItem(DEMO_SESSION_KEY) || '',
      demoSessionExpiresIn: 0,
      sessionVersion: 0,
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
    snapshot(): AuthSnapshot {
      return { version: this.sessionVersion, token: this.token, userID: this.user?.user_id, demoSession: this.demoSessionId }
    },
    isCurrent(snapshot: AuthSnapshot): boolean {
      return this.storageIsCurrent() && snapshot.version === this.sessionVersion && snapshot.token === this.token &&
        snapshot.userID === this.user?.user_id && snapshot.demoSession === this.demoSessionId
    },
    storageIsCurrent(): boolean {
      return !this.externalSessionChanged &&
        (this.storageSource === 'session' || this.persistentMarker === persistentAuthMarker())
    },
    /** 不把旧页面自动切成新身份继续提交；先失效旧工作，再由应用重载。 */
    invalidateIfStorageChanged(): boolean {
      if (this.storageIsCurrent()) return false
      if (!this.externalSessionChanged) {
        this.externalSessionChanged = true
        this.sessionVersion++
        this.token = ''
        this.user = null
      }
      return true
    },
    setAuth(result: LoginResult) {
      const isDemo = (result.perms ?? []).includes('wms:demo')
      const user: AuthUser = {
        user_id: result.user_id,
        username: result.username,
        nickname: result.nickname,
        roles: result.roles ?? [],
        perms: result.perms ?? [],
      }
      const stored = writeStoredAuth(result.token, user, isDemo)
      this.sessionVersion++
      this.clearDemoSession()
      this.storageSource = stored.source
      this.persistentID = stored.id
      this.persistentMarker = stored.marker
      this.externalSessionChanged = false
      this.token = result.token
      this.user = user
    },
    setProfile(profile: ProfileResult, snapshot: AuthSnapshot): boolean {
      if (!this.isCurrent(snapshot) || !this.token || profile.user_id !== this.user?.user_id) return false
      // 存储介质属于登录会话，不能由一次资料刷新迁移到另一份存储。
      const user: AuthUser = {
        user_id: profile.user_id,
        username: profile.username,
        nickname: profile.nickname,
        roles: profile.roles ?? [],
        perms: profile.perms ?? [],
      }
      this.user = user
      writeStoredProfile(user, this.storageSource, this.persistentID)
      return true
    },
    setDemoSession(info: DemoSessionInfo) {
      // A heartbeat renews the same identity; only replacing the session invalidates work.
      if (this.demoSessionId !== info.session_id) this.sessionVersion++
      this.demoSessionId = info.session_id
      this.demoSessionExpiresIn = info.expires_in
      sessionStorage.setItem(DEMO_SESSION_KEY, info.session_id)
    },
    clearDemoSession() {
      this.sessionVersion++
      this.demoSessionId = ''
      this.demoSessionExpiresIn = 0
      sessionStorage.removeItem(DEMO_SESSION_KEY)
    },
    clear() {
      this.persistentMarker = removeStoredAuth(this.storageSource, this.persistentID)
      if (this.storageSource === 'session') {
        this.storageSource = 'none'
        this.persistentMarker = persistentAuthMarker()
      }
      this.sessionVersion++
      this.clearDemoSession()
      this.token = ''
      this.user = null
    },
  },
})
