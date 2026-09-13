import { listLocations, listSkus, listWarehouses } from '@/api/basic'
import type { SkuItem } from '@/api/types'
import { COMMON_STATUS, LOCATION_STATUS } from '@/constants'

export interface IdOption {
  id: number
  label: string
}

/** 全量启用仓库下拉（page_size 上限 100） */
export async function loadWarehouseOptions(): Promise<IdOption[]> {
  const data = await listWarehouses({ page: 1, page_size: 100, status: COMMON_STATUS.ENABLED })
  return (data.list ?? []).map((w) => ({ id: w.id, label: `${w.code}（${w.name}）` }))
}

/** 仓库 id → 名称映射 */
export function toOptionMap(options: IdOption[]): Record<number, string> {
  const map: Record<number, string> = {}
  for (const o of options) map[o.id] = o.label
  return map
}

/** 货品 id → 货品映射（表格中 sku_id 反显用） */
export async function loadSkuMap(): Promise<Record<number, SkuItem>> {
  const data = await listSkus({ page: 1, page_size: 100 })
  const map: Record<number, SkuItem> = {}
  for (const s of data.list ?? []) map[s.id] = s
  return map
}

/** 指定仓库的空闲库位下拉（上架选择库位用） */
export async function loadIdleLocationOptions(warehouseId: number): Promise<IdOption[]> {
  const data = await listLocations({
    page: 1,
    page_size: 100,
    warehouse_id: warehouseId,
    status: LOCATION_STATUS.IDLE,
  })
  return (data.list ?? []).map((l) => ({ id: l.id, label: l.code }))
}

/** 指定仓库的全部库位下拉（盘点选择库位用） */
export async function loadLocationOptions(warehouseId: number): Promise<IdOption[]> {
  const data = await listLocations({ page: 1, page_size: 100, warehouse_id: warehouseId })
  return (data.list ?? []).map((l) => ({ id: l.id, label: l.code }))
}
