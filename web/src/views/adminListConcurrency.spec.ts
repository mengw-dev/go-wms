import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, ssrContextKey, type Component } from 'vue'
import Warehouses from './basic/Warehouses.vue'
import Locations from './basic/Locations.vue'
import Skus from './basic/Skus.vue'
import Users from './system/Users.vue'
import Roles from './system/Roles.vue'
import OperLogs from './system/OperLogs.vue'

const api = vi.hoisted(() => ({
  listWarehouses: vi.fn(), listLocations: vi.fn(), listSkus: vi.fn(),
  listUsers: vi.fn(), listRoles: vi.fn(), listOperLogs: vi.fn(), listAllRoles: vi.fn(),
}))
vi.mock('@/api/basic', () => ({
  listWarehouses: api.listWarehouses, listLocations: api.listLocations, listSkus: api.listSkus,
  createWarehouse: vi.fn(), updateWarehouse: vi.fn(), deleteWarehouse: vi.fn(), updateWarehouseStatus: vi.fn(),
  batchCreateLocations: vi.fn(), deleteLocation: vi.fn(), updateLocationStatus: vi.fn(),
  createSku: vi.fn(), updateSku: vi.fn(), deleteSku: vi.fn(),
}))
vi.mock('@/api/system', () => ({
  listUsers: api.listUsers, listRoles: api.listRoles, listOperLogs: api.listOperLogs, listAllRoles: api.listAllRoles,
  createUser: vi.fn(), updateUser: vi.fn(), deleteUser: vi.fn(), updateUserStatus: vi.fn(), resetUserPassword: vi.fn(),
  createRole: vi.fn(), updateRole: vi.fn(), deleteRole: vi.fn(),
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))

type TestPage = { list: { id: string }[]; total: number }
type ListState = {
  load: () => Promise<void>
  search: () => void
  query: Record<string, unknown>
  loading: boolean
  list: { id: string }[]
  total: number
}

function deferred() {
  let resolve!: (value: TestPage) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<TestPage>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

function result(id: string, total: number): TestPage {
  return { list: [{ id }], total }
}

// Mount the real SFC setup and lifecycle in Node; the visual tree is outside these state tests.
const renderer = createRenderer<object, object>({
  patchProp() {}, insert() {}, remove() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
})
const unmounts: (() => void)[] = []

async function mountPage(component: Component) {
  const app = renderer.createApp({ ...component, render: () => null })
  app.provide(ssrContextKey, { modules: new Set<string>() })
  app.mount({})
  // Script-setup bindings stay private in production; inspect them only in this test harness.
  const state = (app._instance as unknown as { setupState: ListState }).setupState
  let mounted = true
  const unmount = () => {
    if (!mounted) return
    mounted = false
    app.unmount()
  }
  unmounts.push(unmount)
  // Locations first loads warehouse options; Users separately loads role options.
  for (let i = 0; i < 8; i++) await Promise.resolve()
  return { state, unmount }
}

const pages = [
  { name: '仓库', component: Warehouses, request: api.listWarehouses, filter: 'keyword' },
  { name: '库位', component: Locations, request: api.listLocations, filter: 'keyword' },
  { name: '货品', component: Skus, request: api.listSkus, filter: 'keyword' },
  { name: '用户', component: Users, request: api.listUsers, filter: 'keyword' },
  { name: '角色', component: Roles, request: api.listRoles, filter: 'keyword' },
  { name: '日志', component: OperLogs, request: api.listOperLogs, filter: 'username' },
]

beforeEach(() => {
  for (const request of Object.values(api)) request.mockReset().mockResolvedValue({ list: [], total: 0 })
  api.listAllRoles.mockResolvedValue([])
})
afterEach(() => {
  unmounts.splice(0).forEach(unmount => unmount())
})

describe.each(pages)('$name 列表请求交错', ({ component, request, filter }) => {
  it('新筛选先返回后，旧结果不能覆盖列表及总数', async () => {
    const { state } = await mountPage(component)
    const older = deferred(), newer = deferred()
    request.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    state.query[filter] = 'old'
    const first = state.load()
    state.query[filter] = 'new'
    const latest = state.load()
    expect(request.mock.calls.at(-2)?.[0]).toMatchObject({ [filter]: 'old' })
    expect(request.mock.calls.at(-1)?.[0]).toMatchObject({ [filter]: 'new' })
    newer.resolve(result('new', 22))
    await latest
    older.resolve(result('old', 11))
    await first
    expect(state.list).toEqual([{ id: 'new' }])
    expect(state.total).toBe(22)
    expect(state.loading).toBe(false)
  })

  it('旧请求先结束时，仍等待最新分页结果并保持加载', async () => {
    const { state } = await mountPage(component)
    const older = deferred(), newer = deferred()
    request.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const first = state.load()
    state.query.page = 2
    const latest = state.load()
    expect(request.mock.calls.at(-1)?.[0]).toMatchObject({ page: 2 })
    older.resolve(result('old', 11))
    await first
    expect(state.loading).toBe(true)
    expect(state.list).toEqual([])
    expect(state.total).toBe(0)
    newer.resolve(result('page-2', 22))
    await latest
    expect(state.loading).toBe(false)
    expect(state.list).toEqual([{ id: 'page-2' }])
  })

  it('旧请求失败不能提前结束最新请求的加载', async () => {
    const { state } = await mountPage(component)
    const older = deferred(), newer = deferred()
    request.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const first = state.load(), latest = state.load()
    const rejected = expect(first).rejects.toThrow('old failed')
    older.reject(new Error('old failed'))
    await rejected
    expect(state.loading).toBe(true)
    newer.resolve(result('new', 22))
    await latest
    expect(state.loading).toBe(false)
    expect(state.total).toBe(22)
  })

  it('最新请求失败后，保留此前结果并拒绝旧请求晚回填', async () => {
    const { state } = await mountPage(component)
    request.mockResolvedValueOnce(result('seed', 3))
    await state.load()
    const older = deferred(), newer = deferred()
    request.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const first = state.load(), latest = state.load()
    const rejected = expect(latest).rejects.toThrow('new failed')
    newer.reject(new Error('new failed'))
    await rejected
    expect(state.loading).toBe(false)
    older.resolve(result('old', 11))
    await first
    expect(state.list).toEqual([{ id: 'seed' }])
    expect(state.total).toBe(3)
  })

  it('最新空结果正常清空旧列表和总数', async () => {
    const { state } = await mountPage(component)
    request.mockResolvedValueOnce(result('seed', 3))
    await state.load()
    request.mockResolvedValueOnce({ list: [], total: 0 })
    await state.load()
    expect(state.list).toEqual([])
    expect(state.total).toBe(0)
    expect(state.loading).toBe(false)
  })

  it('卸载页面后，晚返回结果不再写入页面状态', async () => {
    const { state, unmount } = await mountPage(component)
    const pending = deferred()
    request.mockReturnValueOnce(pending.promise)
    const loading = state.load()
    unmount()
    pending.resolve(result('late', 11))
    await loading
    expect(state.list).toEqual([])
    expect(state.total).toBe(0)
  })

  it('卸载后触发的刷新不再发送列表请求', async () => {
    const { state, unmount } = await mountPage(component)
    unmount()
    const calls = request.mock.calls.length
    await state.load()
    expect(request).toHaveBeenCalledTimes(calls)
  })
})

it('库位页面等待仓库选项时离开，选项返回后不再启动列表查询', async () => {
  const options = deferred()
  api.listWarehouses.mockReturnValueOnce(options.promise)
  const { unmount } = await mountPage(Locations)
  expect(api.listLocations).not.toHaveBeenCalled()
  unmount()
  options.resolve({ list: [], total: 0 })
  for (let i = 0; i < 8; i++) await Promise.resolve()
  expect(api.listLocations).not.toHaveBeenCalled()
})
