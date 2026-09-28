import type { EntityID, PageQuery } from './common'

export interface StocktakeOrderItem {
  id: EntityID
  order_no: string
  warehouse_id: EntityID
  location_id: EntityID
  location_code: string
  status: string
  remark: string
  created_by: string
  created_at: string
  updated_at: string
}

export interface StocktakeDetailItem {
  id: EntityID
  order_id: EntityID
  inventory_id: EntityID
  sku_id: EntityID
  sku_code: string
  sku_name: string
  location_id: EntityID
  location_code: string
  batch_no: string
  book_qty: number
  actual_qty: number | null
  diff_qty: number
  adjusted: boolean
}

export interface StocktakeCreateParams {
  warehouse_id: EntityID
  location_id?: EntityID
  location_code?: string
  remark: string
}

export interface StocktakeOrderDetail {
  order: StocktakeOrderItem
  details: StocktakeDetailItem[]
}

export interface StocktakeOrderListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  status?: string
  keyword?: string
}

export interface StocktakeActualParams {
  detail_id: EntityID
  actual_qty: number
}
