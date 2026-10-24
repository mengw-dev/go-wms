import { get, post } from './request'
import type { EntityID, OutboundOrderItem, PageData } from './types'
import { isReceiptID } from '@/utils/commandReceipt'

export type WaveStatus = 'CREATED' | 'PICKING' | 'SORTING' | 'READY' | 'COMPLETED' | 'CANCELLED' | 'ABORTED'
export interface Wave {
  id: EntityID; wave_no: string; warehouse_id: EntityID; status: WaveStatus
  abort_reason?: string; remark: string; created_by: string; claimed_by: string; lease_expire_at: string | null; created_at: string
}
export interface WaveOrder {
  id: EntityID; wave_id: EntityID; order_id: EntityID; order_no: string; slot_code: string; shipped_at: string | null
}
export interface WaveLine {
  id: EntityID; inventory_id: EntityID; sku_id: EntityID; sku_code: string; barcode: string; sku_name: string
  location_code: string; batch_no: string; target_qty: number; picked_qty: number
}
export interface WaveItem {
  id: EntityID; wave_order_id: EntityID; line_id: EntityID; task_id: EntityID
  allocation_id: EntityID; target_qty: number; sorted_qty: number
}
export interface WaveDetail { wave: Wave; orders: WaveOrder[]; lines: WaveLine[]; items: WaveItem[] }
export interface CreateWaveParams { order_ids: EntityID[]; remark: string }
export interface WaveQuantityParams { qty: number; sku_code: string; location_code?: string; batch_no: string; slot_code?: string }
export interface WaveAck { wave_id: EntityID }
export function isWaveAck(value: unknown, command: WaveCommand): value is WaveAck {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const id = (value as WaveAck).wave_id
  return isReceiptID(id) && (command.action === 'create' || command.wave_id === id)
}
export type WaveCommand =
  | { action: 'create'; data: CreateWaveParams }
  | { action: 'claim' | 'cancel'; wave_id: EntityID }
  | { action: 'collect' | 'sort'; wave_id: EntityID; part_id: EntityID; data: WaveQuantityParams }
  | { action: 'ship'; wave_id: EntityID; part_id: EntityID }

export function listWaves(query: Record<string, unknown>) { return get<PageData<Wave>>('/outbound/waves', query) }
export function getWave(id: EntityID) { return get<WaveDetail>(`/outbound/waves/${id}`) }
export function waveCandidates(query: { warehouse_id: EntityID; keyword: string; limit: number }) {
  return get<OutboundOrderItem[]>('/outbound/waves/candidates', query)
}
export function sendWaveCommand(command: WaveCommand, key: string): Promise<WaveAck> {
  const config = { headers: { 'Idempotency-Key': key } }
  if (command.action === 'create') return post('/outbound/waves', command.data, config)
  const base = `/outbound/waves/${command.wave_id}`
  if (command.action === 'collect') return post(`${base}/lines/${command.part_id}/collect`, command.data, config)
  if (command.action === 'sort') return post(`${base}/items/${command.part_id}/sort`, command.data, config)
  if (command.action === 'ship') return post(`${base}/orders/${command.part_id}/ship`, undefined, config)
  return post(`${base}/${command.action}`, undefined, config)
}

export const WAVE_STATUS_TEXT: Record<WaveStatus, string> = {
  CREATED: '待拣货', PICKING: '拣货中', SORTING: '分货中', READY: '待发货', COMPLETED: '已完成', CANCELLED: '已撤销', ABORTED: '异常结束',
}

export function isWaveCommand(value: unknown): value is WaveCommand {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const c = value as Record<string, unknown>
  const id = (v: unknown) => typeof v === 'string' && /^[1-9]\d*$/.test(v)
  if (c.action === 'create') {
    const d = c.data as Partial<CreateWaveParams> | undefined
    return !!d && Array.isArray(d.order_ids) && d.order_ids.length > 0 && d.order_ids.length <= 50 && d.order_ids.every(id) && new Set(d.order_ids).size === d.order_ids.length && typeof d.remark === 'string'
  }
  if (!id(c.wave_id)) return false
  if (c.action === 'claim' || c.action === 'cancel') return true
  if (!id(c.part_id)) return false
  if (c.action === 'ship') return true
  if (c.action !== 'collect' && c.action !== 'sort') return false
  const d = c.data as Partial<WaveQuantityParams> | undefined
  return !!d && Number.isInteger(d.qty) && (d.qty ?? 0) > 0 && (d.qty ?? 0) <= 2147483647 &&
    typeof d.sku_code === 'string' && !!d.sku_code.trim() && typeof d.batch_no === 'string' &&
    (c.action === 'collect' ? typeof d.location_code === 'string' && !!d.location_code.trim() : typeof d.slot_code === 'string' && !!d.slot_code.trim())
}
