/** 对外 ID 统一使用字符串，避免 JavaScript Number 精度丢失。 */
export type EntityID = string

/** 后端统一响应结构 */
export interface ApiResponse<T = unknown> {
  code: number
  msg: string
  data: T
}

/** 分页响应结构 */
export interface PageData<T> {
  list: T[]
  total: number
}

/** 分页查询公共参数 */
export interface PageQuery {
  page?: number
  page_size?: number
}

/** 批量操作结果。 */
export interface BatchOperResult {
  success: number
  fail: number
  errors?: { id: EntityID; msg: string }[]
}

/** 入库/出库单提交时的明细行。 */
export interface OrderDetailLine {
  sku_id: EntityID
  expected_qty: number
}

/** GET /version 公开返回的构建信息与特性开关 */
export interface VersionResult {
  version: string
  commit?: string
  build_time?: string
  /** 演示模块总开关（demo.enabled），false 时前端隐藏演示相关入口 */
  demo_enabled?: boolean
  /** 持久体验账号开关（personal.enabled），false 时前端隐藏"个人空间"入口 */
  personal_enabled?: boolean
}
