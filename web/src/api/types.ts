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
  tenant_id?: EntityID
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
  nickname?: string
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
  keyword?: string
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
  /** 只返回现存量大于 0 的库存行 */
  in_stock_only?: boolean
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
  /** 拣货任务的作业库位与批次（收货/上架任务为空） */
  location_id: EntityID
  location_code: string
  batch_no: string
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
  /** 手工入库单为 null；由 Excel 导入创建的入库单为批次任务 ID。 */
  import_task_id: string | null
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
  qty: number
  /** 扫码核对字段（可选）：填写时后端校验必须与任务要求的库位/批次一致 */
  location_code?: string
  batch_no?: string
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

// ---------- 演示模式 ----------

export interface DemoSessionInfo {
  session_id: string
  expires_in: number
}

export interface DemoScenarioStep {
  title: string
  detail: string
  status?: 'pending' | 'completed' | 'failed'
  object?: string
  duration_ms?: number
  status_change?: string
  technical?: string
  error?: string
  facts?: DemoScenarioFact[]
}

export interface DemoScenarioFact {
  label: string
  value: string
}

export interface DemoScenarioEvidence {
  label: string
  value: string
  detail?: string
}

export interface DemoScenarioLink {
  label: string
  path: string
}

export interface DemoScenarioImplementation {
  orchestration: string
  business_files: string[]
  call_chain: string[]
}

export interface DemoScenarioResult {
  name: string
  summary: string
  status?: 'completed' | 'failed'
  target_path?: string
  target_label?: string
  evidence_title?: string
  evidence?: DemoScenarioEvidence[]
  links?: DemoScenarioLink[]
  implementation?: DemoScenarioImplementation
  steps: DemoScenarioStep[]
}

// ---------- 演示中心 ----------

export interface DemoComponentHealth {
  status: string
  latency_ms: number
  message?: string
}

export interface DemoDBPoolStats {
  max_open_connections: number
  open_connections: number
  in_use: number
  idle: number
  wait_count: number
  wait_duration_ms: number
}

export interface DemoRuntimeStats {
  goroutines: number
  memory_alloc_mb: number
  memory_sys_mb: number
  num_gc: number
}

export interface DemoBusinessStats {
  inbound_today: number
  outbound_today: number
  pending_tasks: number
  inventory_rows: number
  stock_total: number
  available_total: number
  allocated_total: number
}

export interface DemoPerformanceSnapshot {
  checked_at: string
  database: DemoComponentHealth
  redis: DemoComponentHealth
  pool: DemoDBPoolStats
  runtime: DemoRuntimeStats
  business: DemoBusinessStats
}

export interface DemoOperationLog {
  id: EntityID
  user_id: EntityID
  username: string
  path: string
  method: string
  params?: string
  ip?: string
  cost_ms: number
  status: number
  result?: string
  created_at: string
}

export interface DemoActivitySnapshot {
  operations: DemoOperationLog[]
  inbound_orders: InboundOrderItem[]
  outbound_orders: OutboundOrderItem[]
  stocktake_orders: StocktakeOrderItem[]
  tasks: TaskItem[]
  inventory_trans: InventoryTransItem[]
}

export interface DemoConcurrentResult {
  concurrency: number
  qty_per_order: number
  total_demand: number
  stock_before: number
  available_before: number
  allocated_before: number
  created_orders: number
  submitted_orders: number
  approved_orders: number
  approval_failed: number
  other_failed: number
  allocated_quantity: number
  pick_task_count: number
  success: number
  failed: number
  duration_ms: number
  stock_total: number
  available_total: number
  allocated_total: number
  negative_rows: number
  invariant_ok: boolean
  invariant_message: string
  validation_scope: string
  not_validated: string
  task_stats_scope: string
  test_focus: string
  summary: string
  steps: DemoScenarioStep[]
}

export interface DemoPickingResult {
  workers: number
  contenders: number
  task_count: number
  total_target: number
  worker_success: number
  worker_rejected: number
  contender_success: number
  contender_rejected: number
  final_picked: number
  completed_tasks: number
  shipped_orders: number
  duration_ms: number
  stock_total: number
  available_total: number
  allocated_total: number
  negative_rows: number
  summary: string
  steps: DemoScenarioStep[]
}
