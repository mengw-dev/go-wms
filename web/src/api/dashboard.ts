import { get } from './request'

export interface DashboardOrder {
  id: string; order_no: string; status: string; warehouse_id: string; created_at: string
}
export interface DashboardOrders {
  statuses: { status: string; total: number; today: number }[]
  days: { date: string; total: number }[]
  recent: DashboardOrder[]
}
export interface DashboardTask {
  id: string; order_id: string; order_no: string; task_no: string; task_type: string
  status: string; warehouse_id: string; created_at: string
}
export interface DashboardOverview {
  generated_at: string; today_from: string; today_to: string; trend_from: string; warehouse_id: string
  access: { inbound: boolean; outbound: boolean; tasks: boolean; inventory: boolean; basic: boolean; stocktake: boolean }
  warehouses: { id: string; code: string; name: string }[]
  inbound?: DashboardOrders; outbound?: DashboardOrders
  task_counts?: { task_type: string; status: string; total: number; older_24h: number }[]
  oldest_tasks?: DashboardTask[]
  inventory?: { skus: number; rows: number; allocated_rows: number }
  locations?: { enabled: number; occupied: number }
  draft_stocktakes?: number
}

export function getDashboardOverview(warehouseId: string) {
  return get<DashboardOverview>('/dashboard/overview', warehouseId ? { warehouse_id: warehouseId } : {}, { silentError: true })
}
