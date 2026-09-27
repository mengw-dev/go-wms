/**
 * Guide 内部事件定义。
 *
 * 业务页面发送的是 events/businessEvents.ts 中的中性业务事件；
 * 这里的事件只在 Guide 层内部使用，由 guide/businessBridge.ts 完成两者之间的转换。
 */
export const GUIDE_EVENTS = {
  inboundOrderCreated: 'inbound.order.created',
  inboundOrderSubmitted: 'inbound.order.submitted',
  inboundOrderApproved: 'inbound.order.approved',
  inboundReceived: 'inbound.received',
  inboundPutawayReady: 'inbound.putaway.ready',
  inboundPutawayCompleted: 'inbound.putaway.completed',
  inboundInventoryReviewed: 'inbound.inventory.reviewed',
  outboundOrderCreated: 'outbound.order.created',
  outboundOrderSubmitted: 'outbound.order.submitted',
  outboundOrderAllocated: 'outbound.order.allocated',
  outboundPickTasksReady: 'outbound.pick.tasks.ready',
  outboundPicked: 'outbound.picked',
  outboundShipped: 'outbound.shipped',
  outboundInventoryReviewed: 'outbound.inventory.reviewed',
  stocktakeOrderCreated: 'stocktake.order.created',
  stocktakeSnapshotReady: 'stocktake.snapshot.ready',
  stocktakeActualCompleted: 'stocktake.actual.completed',
  stocktakeDifferenceReviewed: 'stocktake.difference.reviewed',
  stocktakeApproved: 'stocktake.approved',
  stocktakeInventoryReviewed: 'stocktake.inventory.reviewed',
} as const
