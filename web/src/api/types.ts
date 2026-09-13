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
  user_id: number
  username: string
  nickname: string
  roles: string[]
  perms: string[]
}

export interface ProfileResult {
  user_id: number
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
  id: number
  username: string
  nickname: string
  status: number
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
  role_ids: number[]
}

export interface UserUpdateParams {
  nickname: string
  status?: number
  role_ids?: number[]
}

// ---------- 系统：角色 ----------

export interface RoleItem {
  id: number
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
  id: number
  user_id: number
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
  id: number
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
  id: number
  warehouse_id: number
  code: string
  zone: string
  status: number
  created_at: string
}

export interface LocationListQuery extends PageQuery {
  warehouse_id?: number | ''
  zone?: string
  code?: string
  status?: number | ''
}

export interface LocationBatchParams {
  warehouse_id: number
  zone: string
  row_from: number
  row_to: number
  col_from: number
  col_to: number
}

// ---------- 基础数据：货品 ----------

export interface SkuItem {
  id: number
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
  id: number
  warehouse_id: number
  location_id: number
  sku_id: number
  batch_no: string
  stock_quantity: number
  available_quantity: number
  allocated_quantity: number
  stock_in_time: string
  location_code: string
}

export interface InventoryListQuery extends PageQuery {
  warehouse_id?: number | ''
  location_id?: number | ''
  sku_id?: number | ''
  sku_keyword?: string
}

export interface InventorySummaryItem {
  sku_id: number
  sku_code: string
  sku_name: string
  unit: string
  stock_quantity: number
  available_quantity: number
  allocated_quantity: number
}

export interface InventorySummaryQuery extends PageQuery {
  warehouse_id?: number | ''
}

export interface InventoryTransItem {
  id: number
  inventory_id: number
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
  inventory_id?: number | ''
  order_no?: string
  trans_type?: string
}

// ---------- 任务 ----------

export interface TaskItem {
  id: number
  task_no: string
  task_type: string
  status: string
  order_id: number
  order_no: string
  detail_id: number
  allocation_id: number
  sku_id: number
  warehouse_id: number
  target_qty: number
  done_qty: number
  operator: string
  created_at: string
}

export interface TaskListQuery extends PageQuery {
  order_id?: number | ''
  task_type?: string
  status?: string
}

// ---------- 入库 ----------

export interface OrderDetailLine {
  sku_id: number
  expected_qty: number
}

export interface InboundOrderItem {
  id: number
  order_no: string
  warehouse_id: number
  status: string
  source: string
  remark: string
  expected_qty: number
  received_qty: number
  defective_qty: number
  created_by: string
  created_at: string
  updated_at: string
}

export interface InboundOrderDetailRow {
  id: number
  order_id: number
  sku_id: number
  sku_code: string
  sku_name: string
  expected_qty: number
  received_qty: number
  defective_qty: number
  batch_no: string
}

export interface InboundCreateParams {
  warehouse_id: number
  remark: string
  details: OrderDetailLine[]
}

export interface InboundOrderDetail {
  order: InboundOrderItem
  details: InboundOrderDetailRow[]
  tasks: TaskItem[]
}

export interface InboundOrderListQuery extends PageQuery {
  warehouse_id?: number | ''
  status?: string
  keyword?: string
}

export interface ReceiveParams {
  detail_id: number
  qty: number
  defective_qty: number
  batch_no: string
}

export interface PutawayParams {
  task_id: number
  location_id: number
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
}

// ---------- 出库 ----------

export interface OutboundOrderItem {
  id: number
  order_no: string
  biz_order_no: string
  warehouse_id: number
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
  id: number
  order_id: number
  sku_id: number
  sku_code: string
  sku_name: string
  expected_qty: number
  allocated_qty: number
  picked_qty: number
}

export interface AllocationItem {
  id: number
  order_id: number
  detail_id: number
  inventory_id: number
  sku_id: number
  location_id: number
  location_code: string
  batch_no: string
  allocated_qty: number
  picked_qty: number
  status: string
}

export interface OutboundCreateParams {
  warehouse_id: number
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
  warehouse_id?: number | ''
  status?: string
  keyword?: string
}

export interface PickParams {
  task_id: number
  qty: number
}

// ---------- 盘点 ----------

export interface StocktakeOrderItem {
  id: number
  order_no: string
  warehouse_id: number
  location_id: number
  location_code: string
  status: string
  remark: string
  created_by: string
  created_at: string
  updated_at: string
}

export interface StocktakeDetailItem {
  id: number
  order_id: number
  inventory_id: number
  sku_id: number
  sku_code: string
  sku_name: string
  location_id: number
  location_code: string
  batch_no: string
  book_qty: number
  actual_qty: number | null
  diff_qty: number
  adjusted: boolean
}

export interface StocktakeCreateParams {
  warehouse_id: number
  location_id: number
  location_code?: string
  remark: string
}

export interface StocktakeOrderDetail {
  order: StocktakeOrderItem
  details: StocktakeDetailItem[]
}

export interface StocktakeOrderListQuery extends PageQuery {
  warehouse_id?: number | ''
  status?: string
  keyword?: string
}

export interface StocktakeActualParams {
  detail_id: number
  actual_qty: number
}
