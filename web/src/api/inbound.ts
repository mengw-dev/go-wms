import { del, get, post, put, upload } from './request'
import type {
  ImportTaskItem,
  InboundCreateParams,
  InboundOrderDetail,
  InboundOrderItem,
  InboundOrderListQuery,
  PageData,
  PutawayParams,
  ReceiveParams,
} from './types'

export function listInboundOrders(params: InboundOrderListQuery) {
  return get<PageData<InboundOrderItem>>('/inbound/orders', params as Record<string, unknown>)
}

/** 详情：{ order, details, tasks } */
export function getInboundOrder(id: number | string) {
  return get<InboundOrderDetail>(`/inbound/orders/${id}`)
}

/** 仅草稿可保存 */
export function createInboundOrder(data: InboundCreateParams) {
  return post<InboundOrderItem>('/inbound/orders', data)
}

export function updateInboundOrder(id: number, data: InboundCreateParams) {
  return put<void>(`/inbound/orders/${id}`, data)
}

export function deleteInboundOrder(id: number) {
  return del<void>(`/inbound/orders/${id}`)
}

export function submitInboundOrder(id: number) {
  return post<void>(`/inbound/orders/${id}/submit`)
}

export function approveInboundOrder(id: number) {
  return post<void>(`/inbound/orders/${id}/approve`)
}

export function cancelInboundOrder(id: number) {
  return post<void>(`/inbound/orders/${id}/cancel`)
}

/** 收货 */
export function receiveInbound(id: number, data: ReceiveParams) {
  return post<void>(`/inbound/orders/${id}/receive`, data)
}

/** 上架（路径 :id 即 task_id） */
export function putawayInboundTask(taskId: number, data: PutawayParams) {
  return post<void>(`/inbound/tasks/${taskId}/putaway`, data)
}

/** Excel 导入（multipart，字段名 file），返回异步任务 id */
export function importInboundExcel(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return upload<{ task_id: string }>('/inbound/import', formData)
}

/** 查询导入任务状态 */
export function getImportStatus(taskId: string) {
  return get<ImportTaskItem>(`/inbound/import/${taskId}`)
}
