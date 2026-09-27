/**
 * 业务事件层。
 *
 * 这里只描述“业务上发生了什么”，不包含引导步骤、演示场景、证据步骤等概念。
 * 业务页面负责 emit，Demo / Guide 外挂层负责 on；删除 Demo 时业务页面无需改动。
 *
 * 事件负载只放业务数据：单据 ID、单据号、业务状态、业务任务和明细。
 */

export const BUSINESS_EVENTS = {
  INBOUND_ORDER_CREATED: 'inbound.order.created',
  INBOUND_ORDER_SUBMITTED: 'inbound.order.submitted',
  INBOUND_ORDER_APPROVED: 'inbound.order.approved',
  INBOUND_ORDER_LOADED: 'inbound.order.loaded',

  OUTBOUND_ORDER_CREATED: 'outbound.order.created',
  OUTBOUND_ORDER_SUBMITTED: 'outbound.order.submitted',
  OUTBOUND_ORDER_ALLOCATED: 'outbound.order.allocated',
  OUTBOUND_ORDER_LOADED: 'outbound.order.loaded',

  STOCKTAKE_ORDER_CREATED: 'stocktake.order.created',
  STOCKTAKE_ORDER_LOADED: 'stocktake.order.loaded',

  INVENTORY_TRANS_LOADED: 'inventory.trans.loaded',
} as const

export type BusinessEventName = (typeof BUSINESS_EVENTS)[keyof typeof BUSINESS_EVENTS]

/** 单据关联的业务任务（上架 / 拣货）。 */
export interface BusinessTaskRef {
  taskId: string
  taskNo: string
  targetQty: number
  doneQty: number
}

/** 盘点单明细的业务字段，用于汇总账面、实盘与差异。 */
export interface BusinessStocktakeDetail {
  bookQty: number
  actualQty: number | null
  diffQty: number
  adjusted: boolean
}

export interface BusinessEventPayload {
  orderId?: string
  orderNo?: string
  warehouseId?: string
  /** 单据当前业务状态，用于页面加载后同步已经发生的业务事实。 */
  status?: string
  /** 入库单关联的上架任务。 */
  putawayTask?: BusinessTaskRef | null
  /** 出库单分配汇总。 */
  allocationCount?: number
  allocatedQty?: number
  pickTaskCount?: number
  /** 出库单待执行的拣货任务。 */
  pickTask?: BusinessTaskRef | null
  /** 盘点单明细。 */
  details?: BusinessStocktakeDetail[]
  /** 库存流水的业务类型集合与总条数。 */
  transTypes?: string[]
  transTotal?: number
}

export type BusinessEventListener = (payload: BusinessEventPayload) => void

const listeners = new Map<BusinessEventName, Set<BusinessEventListener>>()

/** 订阅业务事件，返回取消订阅函数。 */
export function onBusinessEvent(event: BusinessEventName, listener: BusinessEventListener): () => void {
  let group = listeners.get(event)
  if (!group) {
    group = new Set()
    listeners.set(event, group)
  }
  group.add(listener)
  return () => {
    group.delete(listener)
  }
}

/**
 * 发送业务事件。
 *
 * 没有订阅者（例如未安装 Demo / Guide 外挂层）时不会产生任何副作用，
 * 因此业务页面可以无条件发送事件。
 */
export function emitBusinessEvent(event: BusinessEventName, payload: BusinessEventPayload = {}): void {
  const group = listeners.get(event)
  if (!group) return
  for (const listener of group) listener(payload)
}
