import type { EntityID, OrderDetailLine, PageQuery } from './common'
import type { TaskItem } from './task'

export interface InboundOrderItem {
  id: EntityID
  order_no: string
  warehouse_id: EntityID
  status: string
  source: string
  /** 手工入库单为 null；由 Excel 导入创建的入库单为批次任务 ID。 */
  import_task_id: string | null
  remark: string
  expected_qty: number
  received_qty: number
  defective_qty: number
  created_by: string
  created_at: string
  updated_at: string
}

export interface InboundOrderDetailRow {
  id: EntityID
  order_id: EntityID
  sku_id: EntityID
  sku_code: string
  sku_name: string
  expected_qty: number
  received_qty: number
  defective_qty: number
  batch_no: string
}

export interface InboundCreateParams {
  warehouse_id: EntityID
  remark: string
  details: OrderDetailLine[]
}

export interface InboundOrderDetail {
  order: InboundOrderItem
  details: InboundOrderDetailRow[]
  tasks: TaskItem[]
}

export interface InboundOrderListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  status?: string
  keyword?: string
  import_task_id?: string
  created_at_from?: string
  created_at_to?: string
}

export interface ReceiveParams {
  detail_id: EntityID
  qty: number
  defective_qty: number
  batch_no: string
}

export interface PutawayParams {
  location_id: EntityID
  qty: number
}

export interface ImportTaskItem {
  task_id: string
  status: string
  file_name: string
  total_rows: number
  success_rows: number
  fail_rows: number
  error_msg: string
  created_at?: string
}
