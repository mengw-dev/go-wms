import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import type { LoginResult } from '@/api/types'
import { useAuthStore } from './auth'
import { installAuthStorageSync } from './authSync'

function storage(): Storage {
  const map = new Map<string, string>()
  return {
    getItem: key => map.get(key) ?? null, setItem: (key, value) => { map.set(key, value) },
    removeItem: key => { map.delete(key) }, clear: () => map.clear(),
    key: index => [...map.keys()][index] ?? null, get length() { return map.size },
  }
}
const result = (id = '1'): LoginResult => ({ token: `token-${id}`, user_id: id, username: `user-${id}`, nickname: id, roles: [], perms: [] })
const listeners = new Map<string, Set<EventListener>>()
const cleanup: (() => void)[] = []
const emit = (name: string, event = {} as Event) => listeners.get(name)?.forEach(fn => fn(event))
const tab = () => useAuthStore(createPinia())
const persistentKeys = () => Array.from({ length: localStorage.length }, (_, i) => localStorage.key(i)!)
beforeEach(() => {
  vi.restoreAllMocks(); listeners.clear()
  vi.stubGlobal('localStorage', storage()); vi.stubGlobal('sessionStorage', storage())
  vi.stubGlobal('window', {
    addEventListener: (name: string, fn: EventListener) => { if (!listeners.has(name)) listeners.set(name, new Set()); listeners.get(name)!.add(fn) },
    removeEventListener: (name: string, fn: EventListener) => listeners.get(name)?.delete(fn),
  })
  setActivePinia(createPinia())
})
afterEach(() => { cleanup.splice(0).forEach(fn => fn()); vi.unstubAllGlobals(); vi.restoreAllMocks() })

describe('跨标签页共享登录', () => {
  it('旧页清理只能删除自己的凭据，新页与刷新后的身份都保留', () => {
    const a = tab(); a.setAuth(result('A'))
    const snapshot = a.snapshot(), b = tab(); b.setAuth(result('B'))
    expect(a.isCurrent(snapshot)).toBe(false)
    a.clear()
    expect(b.token).toBe('token-B'); expect(tab().user?.user_id).toBe('B')
    expect(b.storageIsCurrent()).toBe(true)
  })
  it('精确插入“开始删除后新登录”交错，不能通过比较后删除共享key误伤新登录', () => {
    const a = tab(); a.setAuth(result('A'))
    const b = tab(), remove = localStorage.removeItem.bind(localStorage)
    let interleaved = false
    vi.spyOn(localStorage, 'removeItem').mockImplementation(key => {
      if (!interleaved && key.startsWith('WMS_AUTH_RECORD_V2:')) { interleaved = true; b.setAuth(result('B')) }
      remove(key)
    })
    a.clear()
    expect(interleaved).toBe(true); expect(tab().token).toBe('token-B')
  })
  it('相同token重新登录也有不同会话ID，旧页不能清除或更新新会话', () => {
    const a = tab(); a.setAuth(result())
    const snapshot = a.snapshot(), id = a.persistentID, b = tab(); b.setAuth(result())
    expect(b.persistentID).not.toBe(id)
    expect(a.setProfile({ ...result(), nickname: 'old profile' }, snapshot)).toBe(false)
    a.clear(); expect(tab().token).toBe('token-1'); expect(tab().user?.nickname).toBe('1')
  })
  it('资料写入过程中另一页退出，不得用迟到资料复活登录', () => {
    const a = tab(); a.setAuth(result())
    const b = tab(), set = localStorage.setItem.bind(localStorage)
    vi.spyOn(localStorage, 'setItem').mockImplementation((key, value) => {
      if (key.startsWith('WMS_AUTH_PROFILE_V2:')) b.clear()
      set(key, value)
    })
    a.setProfile({ ...result(), nickname: 'late' }, a.snapshot())
    expect(tab().isLoggedIn).toBe(false)
    expect(a.storageIsCurrent()).toBe(false)
  })
  it('资料写入过程中另一页登录，不能污染新身份；正常资料刷新可恢复', () => {
    const a = tab(); a.setAuth(result('A'))
    const b = tab(), set = localStorage.setItem.bind(localStorage)
    let interleaved = false
    vi.spyOn(localStorage, 'setItem').mockImplementation((key, value) => {
      if (!interleaved && key.startsWith('WMS_AUTH_PROFILE_V2:')) { interleaved = true; b.setAuth(result('B')) }
      set(key, value)
    })
    a.setProfile({ ...result('A'), nickname: 'old' }, a.snapshot())
    expect(tab().user?.user_id).toBe('B'); expect(tab().user?.nickname).toBe('B')
    b.setProfile({ ...result('B'), nickname: 'fresh' }, b.snapshot())
    expect(tab().user?.nickname).toBe('fresh')
  })
  it('旧格式只读兼容，旧页退出不能删除新格式登录；退出后不回退旧凭据', () => {
    localStorage.setItem('WMS_TOKEN', 'legacy'); localStorage.setItem('WMS_USER', JSON.stringify(result()))
    const a = tab(); expect(a.token).toBe('legacy')
    const b = tab(); b.setAuth(result('B')); a.clear()
    expect(tab().token).toBe('token-B')
    localStorage.setItem('WMS_TOKEN', 'legacy'); localStorage.setItem('WMS_USER', JSON.stringify(result()))
    b.clear(); expect(tab().isLoggedIn).toBe(false)
  })
  it('发布指针失败不覆盖旧登录，不留下新令牌记录', () => {
    const a = tab(); a.setAuth(result('A'))
    const keys = persistentKeys(), set = localStorage.setItem.bind(localStorage)
    vi.spyOn(localStorage, 'setItem').mockImplementation((key, value) => {
      if (key === 'WMS_AUTH_CURRENT_V2') throw new Error('storage full')
      set(key, value)
    })
    expect(() => a.setAuth(result('B'))).toThrow('storage full')
    expect(a.token).toBe('token-A'); expect(tab().token).toBe('token-A')
    expect(persistentKeys()).toEqual(keys)
  })
  it('普通登录事件失效旧页面并只重载一次，清理监听后不再响应', async () => {
    const pinia = createPinia(), a = useAuthStore(pinia); a.setAuth(result('A'))
    const reload = vi.fn(), stop = installAuthStorageSync(reload); cleanup.push(stop)
    tab().setAuth(result('B'))
    emit('storage', { storageArea: localStorage } as StorageEvent); emit('focus'); await nextTick()
    expect(a.token).toBe(''); expect(a.externalSessionChanged).toBe(true)
    expect(reload).toHaveBeenCalledOnce(); expect(tab().token).toBe('token-B')
    stop(); expect([...listeners.values()].every(set => set.size === 0)).toBe(true)
  })
  it.each(['focus', 'pageshow'])('%s不依赖storage事件送达即可阻断旧页面', event => {
    const a = tab(); a.setAuth(result('A'))
    const reload = vi.fn(); cleanup.push(installAuthStorageSync(reload))
    tab().setAuth(result('B')); emit(event)
    expect(a.isLoggedIn).toBe(false); expect(reload).toHaveBeenCalledOnce()
  })
  it('普通页观察其他页退出时同步退出，资料刷新不引起重载', () => {
    const a = tab(); a.setAuth(result())
    const reload = vi.fn(); cleanup.push(installAuthStorageSync(reload))
    const b = tab(); b.setProfile({ ...result(), nickname: 'fresh' }, b.snapshot())
    emit('storage', { storageArea: localStorage } as StorageEvent)
    expect(reload).not.toHaveBeenCalled()
    b.clear(); emit('storage', { storageArea: localStorage } as StorageEvent)
    expect(a.isLoggedIn).toBe(false); expect(reload).toHaveBeenCalledOnce()
  })
  it('体验登录、普通页切换和体验退出互不删除凭据', () => {
    const ordinary = tab(); ordinary.setAuth(result('A'))
    const demo = tab(); demo.setAuth({ ...result('D'), perms: ['wms:demo'] })
    const demoStorage = sessionStorage
    const reload = vi.fn(); cleanup.push(installAuthStorageSync(reload))
    // 第二个标签页具有独立sessionStorage，但共享localStorage。
    vi.stubGlobal('sessionStorage', storage()); const b = tab(); b.setAuth(result('B'))
    vi.stubGlobal('sessionStorage', demoStorage)
    emit('storage', { storageArea: localStorage } as StorageEvent)
    expect(reload).not.toHaveBeenCalled(); expect(demo.token).toBe('token-D')
    demo.clear(); expect(b.storageIsCurrent()).toBe(true); expect(tab().token).toBe('token-B')
  })
  it('读取半成品/损坏V2记录不回退拼接旧凭据，也不删除其他记录', () => {
    const a = tab(); a.setAuth(result())
    localStorage.setItem('WMS_AUTH_RECORD_V2:broken', '{oops')
    localStorage.setItem('WMS_AUTH_CURRENT_V2', 'broken')
    expect(tab().isLoggedIn).toBe(false)
    expect(localStorage.getItem(`WMS_AUTH_RECORD_V2:${a.persistentID}`)).not.toBeNull()
  })
  it('并发发布按最后发布的登录生效，不回收另一页尚未发布的有效记录', () => {
    const a = tab(), b = tab(), set = localStorage.setItem.bind(localStorage)
    let interleaved = false
    vi.spyOn(localStorage, 'setItem').mockImplementation((key, value) => {
      set(key, value)
      if (!interleaved && key.startsWith('WMS_AUTH_RECORD_V2:')) {
        interleaved = true; b.setAuth(result('B'))
      }
    })
    a.setAuth(result('A'))
    expect(tab().token).toBe('token-A'); expect(a.storageIsCurrent()).toBe(true)
    expect(b.storageIsCurrent()).toBe(false)
  })
  it('下一次登录回收过期JWT与孤立资料，不碰待确认业务记录', () => {
    const expired = `header.${btoa(JSON.stringify({ exp: 1 }))}.signature`
    localStorage.setItem('WMS_AUTH_RECORD_V2:expired', JSON.stringify({ token: expired, user: result() }))
    localStorage.setItem('WMS_AUTH_PROFILE_V2:orphan', JSON.stringify(result()))
    localStorage.setItem('WMS_PENDING_OPS', 'business-history')
    tab().setAuth(result())
    expect(localStorage.getItem('WMS_AUTH_RECORD_V2:expired')).toBeNull()
    expect(localStorage.getItem('WMS_AUTH_PROFILE_V2:orphan')).toBeNull()
    expect(localStorage.getItem('WMS_PENDING_OPS')).toBe('business-history')
  })
})
