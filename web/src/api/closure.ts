import { get, post } from './request'
import type { EntityID, PageData } from './types'

export interface WorkException {
  id: EntityID; order_id: EntityID; order_no: string; task_id: EntityID; inventory_id: EntityID
  kind: 'SHORTAGE' | 'DAMAGE'; quantity: number; reason: string; status: 'OPEN' | 'RESOLVED' | 'CLOSED'
  resolution: string; operator: string; resolved_by: string; created_at: string; resolved_at: string | null
}
export interface CustomerReturn {
  sku_code: string; sku_name: string; unit: string; location_code: string
  id: EntityID; return_no: string; order_id: EntityID; order_no: string; allocation_id: EntityID
  warehouse_id: EntityID; location_id: EntityID; sku_id: EntityID; batch_no: string
  good_qty: number; bad_qty: number; reason: string; operator: string; created_at: string
}
export type ClosureCommand = (
  | { action: 'freeze'; data: { inventory_id: EntityID; quantity: number; unfreeze: boolean; reason: string } }
  | { action: 'exception'; order_id: EntityID; data: { task_id: EntityID; kind: 'SHORTAGE' | 'DAMAGE'; quantity: number; reason: string; confirm_returned: boolean } }
  | { action: 'resolve'; order_id: EntityID; exception_id: EntityID; data: { close: boolean; reason: string } }
  | { action: 'return'; order_id: EntityID; data: { allocation_id: EntityID; location_id: EntityID; good_qty: number; bad_qty: number; reason: string } }
  | { action: 'abort'; wave_id: EntityID; data: { confirm_returned: boolean; reason: string } }
) & { label?: string }

export interface ClosureAck { object_id?: EntityID; inventory_id?: EntityID; wave_id?: EntityID }
export interface ReturnableRow { allocation_id: EntityID; sku_id: EntityID; sku_code: string; sku_name: string; batch_no: string; shipped_qty: number; returned_qty: number; returnable_qty: number }
export function getReturnable(id: EntityID) { return get<ReturnableRow[]>(`/outbound/orders/${id}/returnable`) }
export function getReturnLocations(id: EntityID, page: number, page_size: number) { return get<PageData<{ id: EntityID; code: string }>>(`/outbound/orders/${id}/return-locations`, { page, page_size }) }
export function listExceptions(params: Record<string, unknown>) { return get<PageData<WorkException>>('/outbound/exceptions', params) }
export function listReturns(params: Record<string, unknown>) { return get<PageData<CustomerReturn>>('/outbound/returns', params) }
export function sendClosureCommand(command: ClosureCommand, key: string): Promise<ClosureAck> {
  const config = { headers: { 'Idempotency-Key': key } }
  switch (command.action) {
    case 'freeze': return post('/inventory/freeze', command.data, config)
    case 'exception': return post('/outbound/exceptions', command.data, config)
    case 'resolve': return post(`/outbound/exceptions/${command.exception_id}/resolve`, command.data, config)
    case 'return': return post('/outbound/returns', command.data, config)
    case 'abort': return post(`/outbound/waves/${command.wave_id}/abort`, command.data, config)
  }
}
export const closureActionText = { freeze: '冻结/解冻', exception: '登记异常', resolve: '处理异常', return: '退货验收', abort: '异常结束波次' }
export function isClosureCommand(value: unknown): value is ClosureCommand {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const c = value as Record<string, unknown>, d = c.data as Record<string, unknown> | undefined
  const id = (v: unknown) => typeof v === 'string' && /^[1-9]\d{0,18}$/.test(v)
  const qty = (v: unknown, zero = false) => Number.isInteger(v) && Number(v) >= (zero ? 0 : 1) && Number(v) <= 2147483647
  if (c.label !== undefined && (typeof c.label !== 'string' || c.label.length > 512)) return false
  if (!d || typeof d.reason !== 'string' || !d.reason.trim() || Array.from(d.reason).length > 255) return false
  if (c.action === 'freeze') return id(d.inventory_id) && qty(d.quantity) && typeof d.unfreeze === 'boolean'
  if (c.action === 'abort') return id(c.wave_id) && d.confirm_returned === true
  if (!id(c.order_id)) return false
  if (c.action === 'exception') return id(d.task_id) && ['SHORTAGE', 'DAMAGE'].includes(String(d.kind)) && qty(d.quantity) && d.confirm_returned === true
  if (c.action === 'resolve') return id(c.exception_id) && typeof d.close === 'boolean'
  if (c.action === 'return') return id(d.allocation_id) && id(d.location_id) && qty(d.good_qty, true) && qty(d.bad_qty, true) && Number(d.good_qty) + Number(d.bad_qty) > 0 && Number(d.good_qty) + Number(d.bad_qty) <= 2147483647
  return false
}
