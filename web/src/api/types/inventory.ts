import type { EntityID, PageQuery } from './common'

export interface InventoryItem {
  id: EntityID
  warehouse_id: EntityID
  location_id: EntityID
  sku_id: EntityID
  batch_no: string
  stock_quantity: number
  available_quantity: number
  allocated_quantity: number
  stock_in_time: string
  location_code: string
}

export interface InventoryListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  location_id?: EntityID | ''
  sku_id?: EntityID | ''
  sku_keyword?: string
  /** 只返回现存量大于 0 的库存行 */
  in_stock_only?: boolean
}

export interface InventorySummaryItem {
  sku_id: EntityID
  sku_code: string
  sku_name: string
  unit: string
  stock_quantity: number
  available_quantity: number
  allocated_quantity: number
}

export interface InventorySummaryQuery extends PageQuery {
  warehouse_id?: EntityID | ''
}

export interface InventoryTransItem {
  id: EntityID
  inventory_id: EntityID
  trans_type: string
  quantity_change: number
  before_quantity: number
  after_quantity: number
  available_before: number
  available_after: number
  order_no: string
  task_no: string
  operator: string
  created_at: string
}

export interface InventoryTransQuery extends PageQuery {
  inventory_id?: EntityID | ''
  order_no?: string
  trans_type?: string
}
