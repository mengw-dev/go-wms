import type { EntityID, PageQuery } from './common'

// ---------- 基础数据：仓库 ----------

export interface WarehouseItem {
  id: EntityID
  code: string
  name: string
  remark: string
  status: number
  created_at: string
  updated_at: string
}

export interface WarehouseParams {
  code: string
  name: string
  remark: string
}

export interface WarehouseListQuery extends PageQuery {
  keyword?: string
  status?: number | ''
}

// ---------- 基础数据：库位 ----------

export interface LocationItem {
  id: EntityID
  warehouse_id: EntityID
  code: string
  zone: string
  status: number
  created_at: string
}

export interface LocationListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  zone?: string
  keyword?: string
  status?: number | ''
}

export interface LocationBatchParams {
  warehouse_id: EntityID
  zone: string
  row_from: number
  row_to: number
  col_from: number
  col_to: number
}

// ---------- 基础数据：货品 ----------

export interface SkuItem {
  id: EntityID
  code: string
  barcode: string
  name: string
  spec: string
  unit: string
  status: number
  created_at: string
}

export interface SkuParams {
  code: string
  barcode: string
  name: string
  spec: string
  unit: string
}

export interface SkuListQuery extends PageQuery {
  keyword?: string
}
