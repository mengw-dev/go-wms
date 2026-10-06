import type { EntityID, OrderDetailLine, PageQuery } from './common'
import type { TaskItem } from './task'

export interface OutboundOrderItem {
  id: EntityID
  order_no: string
  biz_order_no: string
  warehouse_id: EntityID
  status: string
  remark: string
  expected_qty: number
  allocated_qty: number
  picked_qty: number
  created_by: string
  created_at: string
  updated_at: string
}

export interface OutboundOrderDetailRow {
  id: EntityID
  order_id: EntityID
  sku_id: EntityID
  sku_code: string
  sku_name: string
  expected_qty: number
  allocated_qty: number
  picked_qty: number
}

export interface AllocationItem {
  id: EntityID
  order_id: EntityID
  detail_id: EntityID
  inventory_id: EntityID
  sku_id: EntityID
  location_id: EntityID
  location_code: string
  batch_no: string
  allocated_qty: number
  picked_qty: number
  status: string
}

export interface OutboundCreateParams {
  warehouse_id: EntityID
  biz_order_no: string
  remark: string
  details: OrderDetailLine[]
}

export interface OutboundOrderDetail {
  order: OutboundOrderItem
  details: OutboundOrderDetailRow[]
  allocations: AllocationItem[]
  tasks: TaskItem[]
}

export interface OutboundOrderListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  status?: string
  keyword?: string
  created_at_from?: string
  created_at_to?: string
}

export interface PickParams {
  qty: number
  /** 扫码核对字段（可选）：填写时后端校验必须与任务要求的库位/批次一致 */
  location_code?: string
  batch_no?: string
  /** PDA 领取凭证：领取后提交拣货必须携带，租约过期或凭证不符会被拒绝 */
  claim_token?: string
}

/** 拣货结果快照（PDA 现场就地刷新进度，成功与业务拒绝都会返回）。 */
export interface PickResult {
  task_status: string
  done_qty: number
  remaining_qty: number
  order_status: string
}

/** 领取（续领）拣货任务的结果：领取凭证 + 租约到期时间 + 任务快照。 */
export interface ClaimResult extends PickResult {
  claim_token: string
  lease_expire_at: string
}
