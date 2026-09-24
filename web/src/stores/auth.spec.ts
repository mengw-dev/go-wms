import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from './auth'

describe('auth store permissions', () => {
  beforeEach(() => {
    const storage = new Map<string, string>()
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
      clear: () => storage.clear(),
    })
    vi.stubGlobal('sessionStorage', {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
      clear: () => storage.clear(),
    })
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
