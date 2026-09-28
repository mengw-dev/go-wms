import type { EntityID } from './common'
import type { InboundOrderItem } from './inbound'
import type { InventoryTransItem } from './inventory'
import type { OutboundOrderItem } from './outbound'
import type { StocktakeOrderItem } from './stocktake'
import type { TaskItem } from './task'

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

export interface DemoConcurrentShortageResult {
  concurrency: number
  qty_per_order: number
  total_demand: number
  stock_before: number
  available_before: number
  allocated_before: number
  success: number
  insufficient_rejected: number
  other_failed: number
  allocated_quantity: number
  remaining_available: number
  stock_total: number
  available_total: number
  allocated_total: number
  negative_rows: number
  limit_respected: boolean
  invariant_ok: boolean
  invariant_message: string
  validation_scope: string
  not_validated: string
  summary: string
  steps: DemoScenarioStep[]
}

export interface DemoPickingResult {
  workers: number
  contenders: number
  experiment_order_count: number
  experiment_order_nos: string[]
  prepared_stock_quantity: number
  task_count: number
  total_target: number
  concurrent_scan_attempts: number
  concurrent_success: number
  competition_rejected: number
  still_incomplete_after_concurrent: number
  worker_success: number
  worker_rejected: number
  contender_success: number
  contender_rejected: number
  cleanup_remaining_tasks: number
  cleanup_picked_quantity: number
  cleanup_rejected: number
  duplicate_scan_attempts: number
  duplicate_scan_rejected: number
  duplicate_scan_success: number
  final_picked: number
  completed_tasks: number
  shipped_orders: number
  duration_ms: number
  stock_total: number
  available_total: number
  allocated_total: number
  negative_rows: number
  invariant_ok: boolean
  invariant_message: string
  inventory_trans: DemoPickingInventoryEvidence[]
  summary: string
  steps: DemoScenarioStep[]
}

export interface DemoPickingInventoryEvidence {
  trans_type: string
  quantity_change: number
  before_quantity: number
  after_quantity: number
  available_before: number
  available_after: number
  order_no: string
  task_no: string
  created_at: string
}
