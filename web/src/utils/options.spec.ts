import { beforeEach, describe, expect, it, vi } from 'vitest'
import { listLocations, listSkus, listWarehouses } from '@/api/basic'
import { loadIdleLocationOptions, loadLocationOptions, loadSkuMap, loadWarehouseOptions } from './options'

vi.mock('@/api/basic', () => ({ listLocations: vi.fn(), listSkus: vi.fn(), listWarehouses: vi.fn() }))

describe('complete business options', () => {
  beforeEach(() => vi.resetAllMocks())

  it.each(['warehouses', 'skus', 'idle locations', 'all locations'])('loads %s beyond the first 100', async (kind) => {
    const rows = Array.from({ length: 205 }, (_, i) => ({ id: String(i + 1), code: `C${i + 1}`, name: `N${i + 1}`, warehouse_id: '9', zone: 'Z', status: 1, created_at: '', updated_at: '', remark: '', barcode: `B${i}`, spec: '', unit: '件' }))
    const fetch = kind === 'warehouses' ? vi.mocked(listWarehouses) : kind === 'skus' ? vi.mocked(listSkus) : vi.mocked(listLocations)
    // 仅查询逻辑依赖 id/code/name；其余业务字段不参与本测试。
    const pageRows = (page = 1) => rows.slice((page - 1) * 100, page * 100)
    vi.mocked(listWarehouses).mockImplementation(async (q) => ({ total: rows.length, list: pageRows(q.page) }))
    vi.mocked(listSkus).mockImplementation(async (q) => ({ total: rows.length, list: pageRows(q.page) }))
    vi.mocked(listLocations).mockImplementation(async (q) => ({ total: rows.length, list: pageRows(q.page) }))
    const result = kind === 'warehouses' ? await loadWarehouseOptions() : kind === 'skus' ? Object.values(await loadSkuMap()) : kind === 'idle locations' ? await loadIdleLocationOptions('9') : await loadLocationOptions('9')
    expect(result).toHaveLength(205)
    expect(result.some((row) => row.id === '205')).toBe(true)
    expect(fetch).toHaveBeenCalledTimes(3)
    if (kind.includes('locations')) expect(fetch).toHaveBeenLastCalledWith(expect.objectContaining({ page: 3, warehouse_id: '9' }))
  })

  it('does not return a partial list when a later page fails', async () => {
    vi.mocked(listWarehouses).mockResolvedValueOnce({ total: 101, list: [{ id: '1', code: 'W', name: 'warehouse' }] } as Awaited<ReturnType<typeof listWarehouses>>).mockRejectedValueOnce(new Error('page failed'))
    await expect(loadWarehouseOptions()).rejects.toThrow('page failed')
  })
})
