import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { get } from './request'

const mocks = vi.hoisted(() => ({ adapter: vi.fn(), push: vi.fn(), error: vi.fn(), warning: vi.fn() }))
vi.mock('axios', async (original) => {
  const actual = await original<typeof import('axios')>()
  return { ...actual, default: { ...actual.default, create: (config: object) => actual.default.create({ ...config, adapter: mocks.adapter }) } }
})
vi.mock('@/router', () => ({ default: { currentRoute: { value: { path: '/inbound/orders' } }, push: mocks.push } }))
vi.mock('element-plus', () => ({ ElMessage: { error: mocks.error, warning: mocks.warning } }))

function storage(): Storage {
  const values = new Map<string, string>()
  return { getItem: (k) => values.get(k) ?? null, setItem: (k, v) => { values.set(k, v) }, removeItem: (k) => { values.delete(k) }, clear: () => values.clear(), key: () => null, get length() { return values.size } }
}
function login(token: string) {
  useAuthStore().setAuth({ token, user_id: '42', username: 'operator', nickname: '', roles: [], perms: ['wms:demo'] })
}

describe('request authentication ownership', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('localStorage', storage())
    vi.stubGlobal('sessionStorage', storage())
    vi.stubGlobal('window', { setTimeout: vi.fn() })
    setActivePinia(createPinia())
    login('old')
  })

  it.each([401, 423])('late %s cannot clear a new login even with the same token', async (status) => {
    let reject!: (reason: unknown) => void
    let sent!: InternalAxiosRequestConfig
    mocks.adapter.mockImplementation((config) => { sent = config; return new Promise((_, fail) => { reject = fail }) })
    const result = get('/protected').catch((e: unknown) => e)
    await vi.waitFor(() => expect(sent).toBeDefined())
    login('old')
    reject(new AxiosError('expired', undefined, sent, undefined, { status, data: { code: status === 401 ? 40100 : 70003 }, config: sent, statusText: '', headers: {} }))
    expect(await result).toBeInstanceOf(Error)
    expect(useAuthStore().token).toBe('old')
    expect(mocks.push).not.toHaveBeenCalled()
    expect(mocks.error).not.toHaveBeenCalled()
    expect(mocks.warning).not.toHaveBeenCalled()
  })

  it('a current 401 clears auth and redirects once', async () => {
    mocks.adapter.mockImplementation((config) => Promise.reject(new AxiosError('expired', undefined, config, undefined, { status: 401, data: { code: 40100 }, config, statusText: '', headers: {} })))
    await expect(get('/protected')).rejects.toMatchObject({ kind: 'auth' })
    expect(useAuthStore().token).toBe('')
    expect(mocks.push).toHaveBeenCalledWith('/login')
  })

  it('a current expired demo session clears auth', async () => {
    mocks.adapter.mockImplementation((config) => Promise.reject(new AxiosError('expired', undefined, config, undefined, { status: 423, data: { code: 70003 }, config, statusText: '', headers: {} })))
    await expect(get('/protected')).rejects.toMatchObject({ kind: 'demo' })
    expect(useAuthStore().token).toBe('')
    expect(mocks.warning).toHaveBeenCalledOnce()
  })
})
