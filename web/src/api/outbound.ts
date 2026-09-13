import { del, get, post } from './request'
import type {
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
export function getOutboundOrder(id: number | string) {
  return get<OutboundOrderDetail>(`/outbound/orders/${id}`)
}

export function createOutboundOrder(data: OutboundCreateParams) {
  return post<OutboundOrderItem>('/outbound/orders', data)
}

export function deleteOutboundOrder(id: number) {
  return del<void>(`/outbound/orders/${id}`)
}

/** 提交 */
export function submitOutboundOrder(id: number) {
  return post<void>(`/outbound/orders/${id}/submit`)
}

/** 审核（即分配库存） */
export function approveOutboundOrder(id: number) {
  return post<void>(`/outbound/orders/${id}/approve`)
}

export function cancelOutboundOrder(id: number) {
  return post<void>(`/outbound/orders/${id}/cancel`)
}

/** 拣货（路径 :id 即 task_id） */
export function pickOutboundTask(taskId: number, data: PickParams) {
  return post<void>(`/outbound/tasks/${taskId}/pick`, data)
}
