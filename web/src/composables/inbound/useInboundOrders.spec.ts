import { beforeEach, describe, expect, it, vi } from 'vitest'
import { listInboundOrders } from '@/api/inbound'
import type { InboundOrderItem, PageData } from '@/api/types'
import { useInboundOrders } from './useInboundOrders'

vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))
vi.mock('@/api/inbound', () => ({
  listInboundOrders: vi.fn(), listImports: vi.fn(),
  approveInboundOrder: vi.fn(), batchApproveInboundOrders: vi.fn(),
  batchCancelInboundOrders: vi.fn(), batchDeleteInboundOrders: vi.fn(), batchSubmitInboundOrders: vi.fn(),
  cancelInboundOrder: vi.fn(), deleteInboundByImportTask: vi.fn(), deleteInboundOrder: vi.fn(), submitInboundOrder: vi.fn(),
}))
vi.mock('@/utils/options', () => ({ loadSkuMap: vi.fn(), loadWarehouseOptions: vi.fn(), toOptionMap: vi.fn() }))

function deferred() {
  let resolve!: (value: PageData<InboundOrderItem>) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<PageData<InboundOrderItem>>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

describe('入库查询与后台刷新交错', () => {
  beforeEach(() => vi.mocked(listInboundOrders).mockReset())

  it('后台刷新接管手动查询后结束加载，旧响应不覆盖新结果', async () => {
    const manual = deferred(), background = deferred()
    vi.mocked(listInboundOrders).mockReturnValueOnce(manual.promise).mockReturnValueOnce(background.promise)
    const state = useInboundOrders()
    const first = state.load()
    const latest = state.load(true)
    expect(state.loading.value).toBe(true)
    background.resolve({ list: [], total: 2 })
    await latest
    expect(state.loading.value).toBe(false)
    manual.resolve({ list: [], total: 1 })
    await first
    expect(state.total.value).toBe(2)
  })

  it('旧手动查询先结束时等待最新后台查询', async () => {
    const manual = deferred(), background = deferred()
    vi.mocked(listInboundOrders).mockReturnValueOnce(manual.promise).mockReturnValueOnce(background.promise)
    const state = useInboundOrders()
    const first = state.load(), latest = state.load(true)
    manual.resolve({ list: [], total: 1 })
    await first
    expect(state.loading.value).toBe(true)
    background.resolve({ list: [], total: 2 })
    await latest
    expect(state.loading.value).toBe(false)
    expect(state.total.value).toBe(2)
  })

  it('最新后台查询失败也会结束加载，旧查询不回填结果', async () => {
    const manual = deferred(), background = deferred()
    vi.mocked(listInboundOrders).mockReturnValueOnce(manual.promise).mockReturnValueOnce(background.promise)
    const state = useInboundOrders()
    const first = state.load(), latest = state.load(true)
    const rejected = expect(latest).rejects.toThrow('refresh failed')
    background.reject(new Error('refresh failed'))
    await rejected
    expect(state.loading.value).toBe(false)
    manual.resolve({ list: [], total: 1 })
    await first
    expect(state.total.value).toBe(0)
  })

  it('旧后台查询结束不会提前关闭新的手动查询加载', async () => {
    const background = deferred(), manual = deferred()
    vi.mocked(listInboundOrders).mockReturnValueOnce(background.promise).mockReturnValueOnce(manual.promise)
    const state = useInboundOrders()
    const first = state.load(true), latest = state.load()
    background.resolve({ list: [], total: 1 })
    await first
    expect(state.loading.value).toBe(true)
    manual.resolve({ list: [], total: 2 })
    await latest
    expect(state.loading.value).toBe(false)
    expect(state.total.value).toBe(2)
  })
})
