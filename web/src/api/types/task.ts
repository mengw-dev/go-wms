import type { EntityID, PageQuery } from './common'

export interface TaskItem {
	/** 非零时由波次工作台处理；旧响应可缺省。 */
  wave_id?: EntityID
  id: EntityID
  task_no: string
  task_type: string
  status: string
  order_id: EntityID
  order_no: string
  detail_id: EntityID
  allocation_id: EntityID
  sku_id: EntityID
  warehouse_id: EntityID
  /** 拣货任务的作业库位与批次（收货/上架任务为空） */
  location_id: EntityID
  location_code: string
  batch_no: string
  target_qty: number
  done_qty: number
  operator: string
  created_at: string
}

export interface TaskListQuery extends PageQuery {
  /** 任务号或来源单号包含匹配 */
  keyword?: string
  warehouse_id?: EntityID | ''
  order_id?: EntityID | ''
  task_type?: string
  status?: string
}
