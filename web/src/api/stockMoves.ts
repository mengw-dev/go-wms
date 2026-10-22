import { get, post } from './request'
import type { PageData } from './types'

export interface MovePayload { source_inventory_id: string; target_location_id: string; quantity: number; reason: string }
export interface StockMove {
  id: string; move_no: string; warehouse_id: string; warehouse_code: string
  source_inventory_id: string; target_inventory_id: string; source_location_id: string; target_location_id: string
  source_location_code: string; target_location_code: string; sku_id: string; sku_code: string; sku_name: string
  unit: string; batch_no: string; quantity: number; stock_in_time: string; operator: string; reason: string; created_at: string
}
export interface MoveSource { id: string; warehouse_id: string; warehouse_code: string; location_code: string; sku_code: string; sku_name: string; unit: string; batch_no: string; available_qty: number; stock_in_time: string; enabled: boolean }
export function listStockMoves(query: { warehouse_id?: string; keyword?: string; operation_key?: string; page: number; page_size: number }) { return get<PageData<StockMove>>('/inventory/moves', query, { silentError: true }) }
export function moveStock(payload: MovePayload, operationKey: string) { return post<StockMove>('/inventory/moves', payload, { headers: { 'Idempotency-Key': operationKey }, silentError: true }) }
export function getMoveSource(id: string) { return get<MoveSource>(`/inventory/move-source/${id}`, undefined, { silentError: true }) }
export function listMoveLocations(id: string, keyword = '') { return get<PageData<{ id: string; code: string }>>('/inventory/move-locations', { source_inventory_id: id, keyword, page: 1, page_size: 100 }, { silentError: true }) }
