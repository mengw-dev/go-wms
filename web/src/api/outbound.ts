import { del, get, post, type RequestOptions } from './request'
import type {
  BatchOperResult,
  ClaimResult,
  EntityID,
  OutboundCreateParams,
  OutboundOrderDetail,
  OutboundOrderItem,
  OutboundOrderListQuery,
  PageData,
  PickParams,
  PickResult,
} from './types'

export function listOutboundOrders(params: OutboundOrderListQuery, options?: RequestOptions) {
  return get<PageData<OutboundOrderItem>>('/outbound/orders', params as Record<string, unknown>, options)
}

/** 详情：{ order, details, allocations, tasks } */
export function getOutboundOrder(id: EntityID) {
  return get<OutboundOrderDetail>(`/outbound/orders/${id}`)
}

export function createOutboundOrder(data: OutboundCreateParams) {
  return post<OutboundOrderItem>('/outbound/orders', data)
}

export function deleteOutboundOrder(id: EntityID) {
  return del<void>(`/outbound/orders/${id}`)
}

/** 提交 */
export function submitOutboundOrder(id: EntityID) {
  return post<void>(`/outbound/orders/${id}/submit`)
}

/** 审核（即分配库存） */
export function approveOutboundOrder(id: EntityID) {
  return post<void>(`/outbound/orders/${id}/approve`)
}

export function cancelOutboundOrder(id: EntityID) {
  return post<void>(`/outbound/orders/${id}/cancel`)
}

/** 拣货（后台宽松入口，路径 :id 即 task_id） */
export function pickOutboundTask(taskId: EntityID, data: PickParams) {
  return post<void>(`/outbound/tasks/${taskId}/pick`, data)
}

/**
 * PDA 领取（续领）拣货任务：强制携带 Idempotency-Key（重试回放同一凭证，不轮换 token）。
 * 返回领取凭证、租约到期时间与任务快照。
 */
export function claimPdaTask(taskId: EntityID, idempotencyKey: string) {
  return post<ClaimResult>(`/pda/tasks/${taskId}/claim`, undefined, {
    headers: { 'Idempotency-Key': idempotencyKey },
  })
}

/**
 * PDA 拣货（强制扫码入口）：必须携带 Idempotency-Key 与扫码核对信息，返回任务快照；
 * 业务拒绝时后端仍随错误返回快照（request 层将其放入 ApiError.data），可就地刷新进度。
 */
export function pickPdaTask(taskId: EntityID, data: PickParams, idempotencyKey: string) {
  return post<PickResult>(`/pda/tasks/${taskId}/pick`, data, {
    headers: { 'Idempotency-Key': idempotencyKey },
  })
}

/** 批量删除出库单（仅 DRAFT） */
export function batchDeleteOutboundOrders(ids: EntityID[]) {
  return post<BatchOperResult>('/outbound/orders/batch-delete', { ids })
}

/** 批量提交出库单（DRAFT → SUBMITTED） */
export function batchSubmitOutboundOrders(ids: EntityID[]) {
  return post<BatchOperResult>('/outbound/orders/batch-submit', { ids })
}

/** 批量审核出库单（SUBMITTED → PICKING，含库存分配） */
export function batchApproveOutboundOrders(ids: EntityID[]) {
  return post<BatchOperResult>('/outbound/orders/batch-approve', { ids })
}

/** 批量作废出库单 */
export function batchCancelOutboundOrders(ids: EntityID[]) {
  return post<BatchOperResult>('/outbound/orders/batch-cancel', { ids })
}
