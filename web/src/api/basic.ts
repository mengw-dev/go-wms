import { del, get, post, put } from './request'
import type {
  EntityID,
  LocationBatchParams,
  LocationItem,
  LocationListQuery,
  PageData,
  SkuItem,
  SkuListQuery,
  SkuParams,
  WarehouseItem,
  WarehouseListQuery,
  WarehouseParams,
} from './types'

// ---------- 仓库 ----------

export function listWarehouses(params: WarehouseListQuery) {
  return get<PageData<WarehouseItem>>('/basic/warehouses', params as Record<string, unknown>)
}

export function createWarehouse(data: WarehouseParams) {
  return post<WarehouseItem>('/basic/warehouses', data)
}

export function updateWarehouse(id: EntityID, data: WarehouseParams) {
  return put<void>(`/basic/warehouses/${id}`, data)
}

export function deleteWarehouse(id: EntityID) {
  return del<void>(`/basic/warehouses/${id}`)
}

export function updateWarehouseStatus(id: EntityID, status: number) {
  return put<void>(`/basic/warehouses/${id}/status`, { status })
}

// ---------- 库位 ----------

export function listLocations(params: LocationListQuery) {
  return get<PageData<LocationItem>>('/basic/locations', params as Record<string, unknown>)
}

export function batchCreateLocations(data: LocationBatchParams) {
  return post<void>('/basic/locations/batch', data)
}

export function updateLocationStatus(id: EntityID, status: number) {
  return put<void>(`/basic/locations/${id}/status`, { status })
}

export function deleteLocation(id: EntityID) {
  return del<void>(`/basic/locations/${id}`)
}

// ---------- 货品 ----------

export function listSkus(params: SkuListQuery) {
  return get<PageData<SkuItem>>('/basic/skus', params as Record<string, unknown>)
}

export function createSku(data: SkuParams) {
  return post<SkuItem>('/basic/skus', data)
}

export function updateSku(id: EntityID, data: SkuParams) {
  return put<void>(`/basic/skus/${id}`, data)
}

export function deleteSku(id: EntityID) {
  return del<void>(`/basic/skus/${id}`)
}

/** 按条码查询货品 */
export function getSkuByBarcode(barcode: string) {
  return get<SkuItem>(`/basic/skus/barcode/${encodeURIComponent(barcode)}`)
}
