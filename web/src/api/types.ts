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

export interface PageQuery {
  page?: number
  page_size?: number
}

// ---------- 认证 ----------

export interface LoginParams {
  username: string
  password: string
}

export interface LoginResult {
  token: string
  user_id: EntityID
  username: string
  nickname: string
  roles: string[]
  perms: string[]
}

export interface ProfileResult {
  user_id: EntityID
  username: string
  nickname: string
  roles: string[]
  perms: string[]
}

export interface ChangePasswordParams {
  old_password: string
  new_password: string
}

// ---------- 系统：用户 ----------

export interface UserItem {
  id: EntityID
  username: string
  nickname: string
  status: number
  role_ids: EntityID[]
  created_at: string
  updated_at: string
}

export interface UserListQuery extends PageQuery {
  keyword?: string
  status?: number | ''
}

export interface UserCreateParams {
  username: string
  password: string
  nickname: string
  role_ids: EntityID[]
}

export interface UserUpdateParams {
  nickname: string
  status?: number
  role_ids?: EntityID[]
}

// ---------- 系统：角色 ----------

export interface RoleItem {
  id: EntityID
  name: string
  perms: string
  remark: string
  created_at: string
  updated_at: string
}

export interface RoleParams {
  name: string
  perms: string
  remark: string
}

export interface RoleListQuery extends PageQuery {
  keyword?: string
}

// ---------- 系统：操作日志 ----------

export interface OperLogItem {
  id: EntityID
  user_id: EntityID
  username: string
  path: string
  method: string
  params: string
  ip: string
  cost_ms: number
  status: number
  result: string
  created_at: string
}

export interface OperLogListQuery extends PageQuery {
  username?: string
}

// ---------- 基础数据：仓库 ----------

export interface WarehouseItem {
  id: EntityID
  code: string
  name: string
  remark: string
  status: number
  created_at: string
  updated_at: string
}

export interface WarehouseParams {
  code: string
  name: string
  remark: string
}

export interface WarehouseListQuery extends PageQuery {
  keyword?: string
  status?: number | ''
}

// ---------- 基础数据：库位 ----------

export interface LocationItem {
  id: EntityID
  warehouse_id: EntityID
  code: string
  zone: string
  status: number
  created_at: string
}

export interface LocationListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  zone?: string
  code?: string
  status?: number | ''
}

export interface LocationBatchParams {
  warehouse_id: EntityID
  zone: string
  row_from: number
  row_to: number
  col_from: number
  col_to: number
}

// ---------- 基础数据：货品 ----------

export interface SkuItem {
  id: EntityID
  code: string
  barcode: string
  name: string
  spec: string
  unit: string
  status: number
  created_at: string
}

export interface SkuParams {
  code: string
  barcode: string
  name: string
  spec: string
  unit: string
}

export interface SkuListQuery extends PageQuery {
  keyword?: string
}

// ---------- 库存 ----------

export interface InventoryItem {
  id: EntityID
  warehouse_id: EntityID
  location_id: EntityID
  sku_id: EntityID
  batch_no: string
  stock_quantity: number
  available_quantity: number
  allocated_quantity: number
  stock_in_time: string
  location_code: string
}

export interface InventoryListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  location_id?: EntityID | ''
  sku_id?: EntityID | ''
  sku_keyword?: string
}

export interface InventorySummaryItem {
  sku_id: EntityID
  sku_code: string
  sku_name: string
  unit: string
  stock_quantity: number
  available_quantity: number
  allocated_quantity: number
}

export interface InventorySummaryQuery extends PageQuery {
  warehouse_id?: EntityID | ''
}

export interface InventoryTransItem {
  id: EntityID
  inventory_id: EntityID
  trans_type: string
  quantity_change: number
  before_quantity: number
  after_quantity: number
  available_before: number
  available_after: number
  order_no: string
  task_no: string
  operator: string
  created_at: string
}

export interface InventoryTransQuery extends PageQuery {
  inventory_id?: EntityID | ''
  order_no?: string
  trans_type?: string
}

// ---------- 任务 ----------

export interface TaskItem {
  id: EntityID
  task_no: string
  task_type: string
  status: string
  order_id: EntityID
  order_no: string
  detail_id: EntityID
  allocation_id: EntityID
  sku_id: EntityID
  warehouse_id: EntityID
  target_qty: number
  done_qty: number
  operator: string
  created_at: string
}

export interface TaskListQuery extends PageQuery {
  order_id?: EntityID | ''
  task_type?: string
  status?: string
}

// ---------- 入库 ----------

export interface OrderDetailLine {
  sku_id: EntityID
  expected_qty: number
}

export interface InboundOrderItem {
  id: EntityID
  order_no: string
  warehouse_id: EntityID
  status: string
  source: string
  import_task_id?: string
  remark: string
  expected_qty: number
  received_qty: number
  defective_qty: number
  created_by: string
  created_at: string
  updated_at: string
}

export interface InboundOrderDetailRow {
  id: EntityID
  order_id: EntityID
  sku_id: EntityID
  sku_code: string
  sku_name: string
  expected_qty: number
  received_qty: number
  defective_qty: number
  batch_no: string
}

export interface InboundCreateParams {
  warehouse_id: EntityID
  remark: string
  details: OrderDetailLine[]
}

export interface InboundOrderDetail {
  order: InboundOrderItem
  details: InboundOrderDetailRow[]
  tasks: TaskItem[]
}

export interface InboundOrderListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  status?: string
  keyword?: string
  import_task_id?: string
  created_at_from?: string
  created_at_to?: string
}

export interface ReceiveParams {
  detail_id: EntityID
  qty: number
  defective_qty: number
  batch_no: string
}

// 批量操作结果。
export interface BatchOperResult {
  success: number
  fail: number
  errors?: { id: EntityID; msg: string }[]
}

export interface PutawayParams {
  task_id: EntityID
  location_id: EntityID
  qty: number
}

export interface ImportTaskItem {
  task_id: string
  status: string
  file_name: string
  total_rows: number
  success_rows: number
  fail_rows: number
  error_msg: string
  created_at?: string
}

// ---------- 出库 ----------

export interface OutboundOrderItem {
  id: EntityID
  order_no: string
  biz_order_no: string
  warehouse_id: EntityID
  status: string
  remark: string
  expected_qty: number
  allocated_qty: number
  picked_qty: number
  created_by: string
  created_at: string
  updated_at: string
}

export interface OutboundOrderDetailRow {
  id: EntityID
  order_id: EntityID
  sku_id: EntityID
  sku_code: string
  sku_name: string
  expected_qty: number
  allocated_qty: number
  picked_qty: number
}

export interface AllocationItem {
  id: EntityID
  order_id: EntityID
  detail_id: EntityID
  inventory_id: EntityID
  sku_id: EntityID
  location_id: EntityID
  location_code: string
  batch_no: string
  allocated_qty: number
  picked_qty: number
  status: string
}

export interface OutboundCreateParams {
  warehouse_id: EntityID
  biz_order_no: string
  remark: string
  details: OrderDetailLine[]
}

export interface OutboundOrderDetail {
  order: OutboundOrderItem
  details: OutboundOrderDetailRow[]
  allocations: AllocationItem[]
  tasks: TaskItem[]
}

export interface OutboundOrderListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  status?: string
  keyword?: string
  created_at_from?: string
  created_at_to?: string
}

export interface PickParams {
  task_id: EntityID
  qty: number
}

// ---------- 盘点 ----------

export interface StocktakeOrderItem {
  id: EntityID
  order_no: string
  warehouse_id: EntityID
  location_id: EntityID
  location_code: string
  status: string
  remark: string
  created_by: string
  created_at: string
  updated_at: string
}

export interface StocktakeDetailItem {
  id: EntityID
  order_id: EntityID
  inventory_id: EntityID
  sku_id: EntityID
  sku_code: string
  sku_name: string
  location_id: EntityID
  location_code: string
  batch_no: string
  book_qty: number
  actual_qty: number | null
  diff_qty: number
  adjusted: boolean
}

export interface StocktakeCreateParams {
  warehouse_id: EntityID
  location_id?: EntityID
  location_code?: string
  remark: string
}

export interface StocktakeOrderDetail {
  order: StocktakeOrderItem
  details: StocktakeDetailItem[]
}

export interface StocktakeOrderListQuery extends PageQuery {
  warehouse_id?: EntityID | ''
  status?: string
  keyword?: string
}

export interface StocktakeActualParams {
  detail_id: EntityID
  actual_qty: number
}
