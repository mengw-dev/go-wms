import { get, post } from './request'
import type {
  EntityID,
  PageData,
  StocktakeActualParams,
  StocktakeCreateParams,
  StocktakeOrderDetail,
  StocktakeOrderItem,
  StocktakeOrderListQuery,
} from './types'

export function listStocktakeOrders(params: StocktakeOrderListQuery) {
  return get<PageData<StocktakeOrderItem>>('/stocktake/orders', params as Record<string, unknown>)
}

/** 详情：{ order, details } */
export function getStocktakeOrder(id: EntityID) {
  return get<StocktakeOrderDetail>(`/stocktake/orders/${id}`)
}

/** 新建盘点（location_id 传 0 表示整仓） */
export function createStocktakeOrder(data: StocktakeCreateParams) {
  return post<StocktakeOrderItem>('/stocktake/orders', data)
}

/** 录入实盘数 */
export function submitStocktakeActual(id: EntityID, data: StocktakeActualParams) {
  return post<void>(`/stocktake/orders/${id}/actual`, data)
}

/**
 * 审核（按差异调整库存）。Idempotency-Key 可选：项目自带客户端全部携带，
 * 同 key 重试回放空成功（data 仍为 null），审核成功但响应丢失后的重试
 * 不会因「已终态」被误报为业务失败；同 key 用于其他单据返回 409；
 * 未携带 key 的调用方保持旧契约但不具备请求级去重保证。
 */
export function approveStocktakeOrder(id: EntityID, idempotencyKey?: string) {
  return post<void>(`/stocktake/orders/${id}/approve`, undefined, {
    headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined,
  })
}

export function cancelStocktakeOrder(id: EntityID) {
  return post<void>(`/stocktake/orders/${id}/cancel`)
}
