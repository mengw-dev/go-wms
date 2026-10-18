import { listLocations, listSkus, listWarehouses } from '@/api/basic'
import type { EntityID, SkuItem } from '@/api/types'
import { COMMON_STATUS, LOCATION_STATUS } from '@/constants'
import { loadAllPages } from './pagination'

export interface IdOption {
  id: EntityID
  label: string
}

/** 全量启用仓库下拉（page_size 上限 100） */
export async function loadWarehouseOptions(): Promise<IdOption[]> {
  const rows = await loadAllPages((page, page_size) => listWarehouses({ page, page_size, status: COMMON_STATUS.ENABLED }))
  return rows.map((w) => ({ id: w.id, label: `${w.code}（${w.name}）` }))
}

/** 仓库 id → 名称映射 */
export function toOptionMap(options: IdOption[]): Record<EntityID, string> {
  const map: Record<EntityID, string> = {}
  for (const o of options) map[o.id] = o.label
  return map
}

/** 货品 id → 货品映射（表格中 sku_id 反显用） */
export async function loadSkuMap(): Promise<Record<EntityID, SkuItem>> {
  const rows = await loadAllPages((page, page_size) => listSkus({ page, page_size }))
  const map: Record<EntityID, SkuItem> = {}
  for (const s of rows) map[s.id] = s
  return map
}

/** 指定仓库的空闲库位下拉（上架选择库位用） */
export async function loadIdleLocationOptions(warehouseId: EntityID): Promise<IdOption[]> {
  const rows = await loadAllPages((page, page_size) => listLocations({
    page,
    page_size,
    warehouse_id: warehouseId,
    status: LOCATION_STATUS.IDLE,
  }))
  return rows.map((l) => ({ id: l.id, label: l.code }))
}

/** 指定仓库的全部库位下拉（盘点选择库位用） */
export async function loadLocationOptions(warehouseId: EntityID): Promise<IdOption[]> {
  const rows = await loadAllPages((page, page_size) => listLocations({ page, page_size, warehouse_id: warehouseId }))
  return rows.map((l) => ({ id: l.id, label: l.code }))
}
