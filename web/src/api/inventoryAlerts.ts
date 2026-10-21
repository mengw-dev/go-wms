import { del, get, put } from './request'
import type { EntityID, PageData } from './types'

export type AlertState = 'ALERT' | 'ALL' | 'LOW' | 'OUT_OF_STOCK' | 'NORMAL' | 'PAUSED' | 'UNAVAILABLE'
export interface InventoryAlert {
  id: EntityID; warehouse_id: EntityID; warehouse_code: string; warehouse_name: string
  sku_id: EntityID; sku_code: string; sku_name: string; unit: string
  min_available_qty: number; enabled: boolean; stock_quantity: number; available_qty: number
  allocated_qty: number; gap_qty: number; state: AlertState; updated_by: string; updated_at: string
}
export interface AlertQuery { warehouse_id: EntityID | ''; sku_keyword: string; state: AlertState; page: number; page_size: number }
export interface AlertRuleParams { warehouse_id: EntityID; sku_id: EntityID; min_available_qty: number; enabled: boolean }
export interface AlertOption { id: EntityID; code: string; name: string; unit: string }
export interface AlertList { list: InventoryAlert[]; total: number; warehouses: AlertOption[]; summary: { configured: number; out_of_stock: number; low: number }; checked_at: string }
export function listAlertOptions(kind: 'warehouse' | 'sku', page: number, page_size: number) { return get<PageData<AlertOption>>('/inventory/alert-options', {kind,page,page_size}) }
export const ALERT_STATE_TEXT: Record<AlertState, string> = { ALERT: '需关注', ALL: '全部规则', LOW: '低库存', OUT_OF_STOCK: '可用库存为零', NORMAL: '正常', PAUSED: '已暂停', UNAVAILABLE: '基础资料已停用或删除' }
export function listInventoryAlerts(query: AlertQuery) { return get<AlertList>('/inventory/alerts', { ...query, warehouse_id: query.warehouse_id || undefined }, { silentError: true }) }
export function saveInventoryAlertRule(data: AlertRuleParams) { return put<{ id: EntityID }>('/inventory/alert-rules', data) }
export function deleteInventoryAlertRule(id: EntityID) { return del<void>(`/inventory/alert-rules/${id}`) }
