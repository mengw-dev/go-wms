import { get, post } from './request'
import type {
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
export function getStocktakeOrder(id: number | string) {
  return get<StocktakeOrderDetail>(`/stocktake/orders/${id}`)
}

/** 新建盘点（location_id 传 0 表示整仓） */
export function createStocktakeOrder(data: StocktakeCreateParams) {
  return post<StocktakeOrderItem>('/stocktake/orders', data)
}

/** 录入实盘数 */
export function submitStocktakeActual(id: number, data: StocktakeActualParams) {
  return post<void>(`/stocktake/orders/${id}/actual`, data)
}

/** 审核（按差异调整库存） */
export function approveStocktakeOrder(id: number) {
  return post<void>(`/stocktake/orders/${id}/approve`)
}

export function cancelStocktakeOrder(id: number) {
  return post<void>(`/stocktake/orders/${id}/cancel`)
}
