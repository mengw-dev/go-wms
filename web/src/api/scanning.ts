import { get } from './request'
import type { PageData } from './types'

export interface ScannedSKU { id: string; code: string; name: string; barcode: string }
export interface PutawayLocation { id: string; code: string; status: number }
export function scanOrderSKU(kind: 'inbound' | 'outbound', id: string, code: string) {
  return get<ScannedSKU[]>(`/${kind}/orders/${id}/scan-sku`, { code }, { silentError: true })
}
export function putawayLocations(id: string, query: { code?: string; keyword?: string; page?: number; page_size?: number } = {}) {
  return get<PageData<PutawayLocation>>(`/inbound/orders/${id}/putaway-locations`, { page: 1, page_size: 100, ...query }, { silentError: true })
}
