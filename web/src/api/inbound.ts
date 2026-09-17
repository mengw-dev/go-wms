import { del, get, post, put, upload } from './request'
import type {
  BatchOperResult,
  EntityID,
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
export function getInboundOrder(id: EntityID) {
  return get<InboundOrderDetail>(`/inbound/orders/${id}`)
}

/** 仅草稿可保存 */
export function createInboundOrder(data: InboundCreateParams) {
  return post<InboundOrderItem>('/inbound/orders', data)
}

export function updateInboundOrder(id: EntityID, data: InboundCreateParams) {
  return put<void>(`/inbound/orders/${id}`, data)
}

export function deleteInboundOrder(id: EntityID) {
  return del<void>(`/inbound/orders/${id}`)
}

export function submitInboundOrder(id: EntityID) {
  return post<void>(`/inbound/orders/${id}/submit`)
}

export function approveInboundOrder(id: EntityID) {
  return post<void>(`/inbound/orders/${id}/approve`)
}

export function cancelInboundOrder(id: EntityID) {
  return post<void>(`/inbound/orders/${id}/cancel`)
}

/** 收货 */
export function receiveInbound(id: EntityID, data: ReceiveParams) {
  return post<void>(`/inbound/orders/${id}/receive`, data)
}

/** 上架（路径 :id 即 task_id） */
export function putawayInboundTask(taskId: EntityID, data: PutawayParams) {
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

/** 批量删除入库单（仅 DRAFT） */
export function batchDeleteInboundOrders(ids: EntityID[]) {
  return post<BatchOperResult>('/inbound/orders/batch-delete', { ids })
}

/** 批量提交入库单（DRAFT → SUBMITTED） */
export function batchSubmitInboundOrders(ids: EntityID[]) {
  return post<BatchOperResult>('/inbound/orders/batch-submit', { ids })
}

/** 批量审核入库单（SUBMITTED → APPROVED） */
export function batchApproveInboundOrders(ids: EntityID[]) {
  return post<BatchOperResult>('/inbound/orders/batch-approve', { ids })
}

/** 批量作废入库单 */
export function batchCancelInboundOrders(ids: EntityID[]) {
  return post<BatchOperResult>('/inbound/orders/batch-cancel', { ids })
}

/** 按导入批次删除 DRAFT 入库单 */
export function deleteInboundByImportTask(taskId: string) {
  return del<BatchOperResult>(`/inbound/import/${taskId}/orders`)
}
