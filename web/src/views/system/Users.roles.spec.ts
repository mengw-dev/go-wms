import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, nextTick, ssrContextKey } from 'vue'
import { createPinia } from 'pinia'
import type { RoleItem, UserItem } from '@/api/types'
import Users from './Users.vue'

const api = vi.hoisted(() => ({
  listUsers: vi.fn(), listAssignableRoles: vi.fn(), createUser: vi.fn(), updateUser: vi.fn(),
}))
vi.mock('@/api/system', () => ({
  ...api, deleteUser: vi.fn(), updateUserStatus: vi.fn(), resetUserPassword: vi.fn(),
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))
vi.mock('element-plus', async (importOriginal) => ({
  ...await importOriginal<typeof import('element-plus')>(),
  ElMessage: { success: vi.fn() },
}))

const renderer = createRenderer<object, object>({
  patchProp() {}, insert() {}, remove() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
})
type State = {
  openCreate(): void
  openEdit(row: UserItem): void
  loadRoleOptions(id?: string): Promise<void>
  submit(): Promise<void>
  roleOptions: RoleItem[]
  rolesReady: boolean
  rolesLoading: boolean
  dialog: { visible: boolean; editingId: string }
  form: { role_ids: string[] }
  formRef: { validate(): Promise<boolean> }
}
const unmounts: (() => void)[] = []
const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); await nextTick() }

async function mount() {
  const app = renderer.createApp({ ...Users, render: () => null })
  app.use(createPinia())
  app.provide(ssrContextKey, { modules: new Set<string>() })
  app.mount({})
  const state = (app._instance as unknown as { setupState: State }).setupState
  unmounts.push(() => app.unmount())
  await flush()
  state.formRef = { validate: async () => true }
  return state
}

function deferred() {
  let resolve!: (roles: RoleItem[]) => void
  let reject!: (err: Error) => void
  const promise = new Promise<RoleItem[]>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const role = (id: string): RoleItem => ({ id, name: id, perms: 'wms:basic', remark: '', created_at: '', updated_at: '' })
const user = (id: string): UserItem => ({
  id, username: id, nickname: 'original', status: 1, role_ids: ['existing'], created_at: '', updated_at: '',
})

beforeEach(() => {
  vi.stubGlobal('localStorage', { getItem: () => null })
  vi.stubGlobal('sessionStorage', { getItem: () => null })
  vi.clearAllMocks()
  api.listUsers.mockResolvedValue({ list: [], total: 0 })
  api.listAssignableRoles.mockReset().mockResolvedValue([role('own')])
  api.createUser.mockResolvedValue(undefined)
  api.updateUser.mockResolvedValue(undefined)
})
afterEach(() => {
  unmounts.splice(0).forEach(fn => fn())
  vi.unstubAllGlobals()
})

describe('用户管理可授予角色', () => {
  it('编辑使用目标用户，创建重新使用当前租户；旧回复不能混入新目标', async () => {
    const state = await mount()
    expect(api.listAssignableRoles).toHaveBeenCalledWith(undefined)
    const old = deferred(), current = deferred()
    api.listAssignableRoles.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    state.openEdit(user('foreign-user'))
    expect(api.listAssignableRoles).toHaveBeenLastCalledWith('foreign-user')
    state.openCreate()
    expect(api.listAssignableRoles).toHaveBeenLastCalledWith(undefined)
    current.resolve([role('current')]); await flush()
    old.resolve([role('foreign')]); await flush()
    expect(state.roleOptions.map(r => r.id)).toEqual(['current'])
    expect(state.form.role_ids).toEqual([])
  })

  it('角色加载失败保留原关联并禁止保存，恢复后才提交原角色', async () => {
    const state = await mount()
    api.listAssignableRoles.mockRejectedValueOnce(new Error('forbidden'))
    state.openEdit(user('target')); await flush()
    expect(state.rolesReady).toBe(false)
    expect(state.form.role_ids).toEqual(['existing'])
    await state.submit()
    expect(api.updateUser).not.toHaveBeenCalled()
    api.listAssignableRoles.mockResolvedValueOnce([role('existing')])
    await state.loadRoleOptions('target')
    await state.submit()
    expect(api.updateUser).toHaveBeenCalledWith('target', expect.objectContaining({ role_ids: ['existing'] }))
  })

  it('角色仍在加载时不可提交，关闭后晚到回复不能恢复可提交状态', async () => {
    const state = await mount()
    const pending = deferred()
    api.listAssignableRoles.mockReturnValueOnce(pending.promise)
    state.openEdit(user('target')); await nextTick()
    await state.submit()
    expect(api.updateUser).not.toHaveBeenCalled()
    state.dialog.visible = false; await nextTick()
    pending.resolve([role('late')]); await flush()
    expect(state.rolesReady).toBe(false)
    expect(state.roleOptions).toEqual([])
  })
})
