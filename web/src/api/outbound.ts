import { del, get, post } from './request'
import type {
  EntityID,
  OutboundCreateParams,
  OutboundOrderDetail,
  OutboundOrderItem,
  OutboundOrderListQuery,
  PageData,
  PickParams,
} from './types'

export function listOutboundOrders(params: OutboundOrderListQuery) {
  return get<PageData<OutboundOrderItem>>('/outbound/orders', params as Record<string, unknown>)
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

/** 拣货（路径 :id 即 task_id） */
export function pickOutboundTask(taskId: EntityID, data: PickParams) {
  return post<void>(`/outbound/tasks/${taskId}/pick`, data)
}
