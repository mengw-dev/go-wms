import { get } from './request'
import type {
  InventoryItem,
  InventoryListQuery,
  InventorySummaryItem,
  InventorySummaryQuery,
  InventoryTransItem,
  InventoryTransQuery,
  PageData,
} from './types'

/** 库存明细 */
export function listInventory(params: InventoryListQuery) {
  return get<PageData<InventoryItem>>('/inventory', params as Record<string, unknown>)
}

/** 按 SKU 汇总 */
export function listInventorySummary(params: InventorySummaryQuery) {
  return get<PageData<InventorySummaryItem>>('/inventory/summary', params as Record<string, unknown>)
}

/** 库存流水 */
export function listInventoryTrans(params: InventoryTransQuery) {
  return get<PageData<InventoryTransItem>>('/inventory/trans', params as Record<string, unknown>)
}
