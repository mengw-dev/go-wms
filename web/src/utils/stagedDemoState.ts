import type { StagedRequest } from '@/api/demoExecution'

export type StepKey =
  | 'inbound-create'
  | 'inbound-submit'
  | 'inbound-approve'
  | 'inbound-receive'
  | 'inbound-putaway'
  | 'inventory-check'
  | 'outbound-create'
  | 'outbound-submit'
  | 'outbound-allocate'
  | 'outbound-pick'

export interface ExecutionStep {
  key: StepKey
  group: string
  title: string
  description: string
}

export interface StepFact {
  label: string
  value: string
}

export interface StepResult {
  key: StepKey
  title: string
  objectNo: string
  status: string
  before: string
  after: string
  quantity: string
  createdAt: string
  route: string
  facts: StepFact[]
}

export type DemoRunMode = 'auto' | 'step'
export type DemoRunScope = 'full' | 'inbound' | 'outbound'

export interface StagedDemoState {
  started: boolean
  active: boolean
  completed: boolean
  progress: number
  running: boolean
}

export interface UseStagedDemoRunnerOptions {
  onStateChange?: (state: StagedDemoState) => void
}

export interface StagedIdentity { userId: string; sessionId: string }
export interface DemoWrite extends StagedRequest {
  name: string
  step: StepKey
  operationKey: string
  status: 'sending' | 'done'
  response: unknown
}
export interface StagedSnapshot {
  version: 1
  runId: string
  mode: DemoRunMode
  scope: DemoRunScope
  startedAt: string
  completedAt: string
  index: number
  results: StepResult[]
  inboundID: string
  inboundNo: string
  outboundID: string
  outboundNo: string
  warehouse: { id: string; code: string } | null
  sku: { id: string; code: string } | null
  writes: DemoWrite[]
}

const STORAGE_PREFIX = 'WMS_STAGED_DEMO_V1:'
export const STAGED_RETENTION_MS = 7 * 24 * 60 * 60 * 1000

export function executionSteps(scope: DemoRunScope): ExecutionStep[] {
  if (scope === 'inbound') return STAGED_EXECUTION_STEPS.filter((step) => !step.key.startsWith('outbound'))
  if (scope === 'outbound') return STAGED_EXECUTION_STEPS.filter((step) => step.key.startsWith('outbound'))
  return STAGED_EXECUTION_STEPS
}

function object(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}
function text(value: unknown): value is string { return typeof value === 'string' && value.length <= 1000 }
function id(value: unknown): value is string { return typeof value === 'string' && /^[1-9]\d{0,18}$/.test(value) }
function uuid(value: unknown): value is string { return typeof value === 'string' && /^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/i.test(value) }
function qty(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) > 0 && Number(value) <= 2147483647 }
function date(value: unknown): value is string { return text(value) && Number.isFinite(Date.parse(value)) }

/** 存储来自浏览器，类型声明不能替代运行时校验；损坏记录禁止静默丢弃后重新开始。 */
function validWrite(value: unknown): value is DemoWrite {
  if (!object(value) || !text(value.name) || !value.name || !uuid(value.operationKey) ||
    !['sending', 'done'].includes(String(value.status)) || !object(value.payload)) return false
  if (!STAGED_EXECUTION_STEPS.some((step) => step.key === value.step)) return false
  const p = value.payload
  const steps: Record<string, StepKey> = {
    'inbound-create': 'inbound-create', 'inbound-submit': 'inbound-submit', 'inbound-approve': 'inbound-approve',
    receive: 'inbound-receive', putaway: 'inbound-putaway', 'location-create': 'inbound-putaway',
    'outbound-create': 'outbound-create', 'outbound-submit': 'outbound-submit', 'outbound-approve': 'outbound-allocate', pick: 'outbound-pick',
  }
  if (steps[String(value.kind)] !== value.step) return false
  const name = value.kind === 'location-create' ? 'location' : value.kind === 'pick' ? 'pick:' + String(value.objectId)
    : value.kind === 'outbound-approve' ? 'outbound-allocate' : String(value.kind)
  if (value.name !== name) return false
  const noTarget = ['inbound-create', 'outbound-create', 'location-create'].includes(String(value.kind))
  if (noTarget ? value.objectId !== '' : !id(value.objectId)) return false
  switch (value.kind) {
    case 'inbound-create': case 'outbound-create':
      return id(p.warehouse_id) && text(p.remark) && p.remark.length > 0 &&
        (value.kind !== 'outbound-create' || (text(p.biz_order_no) && p.biz_order_no.length > 0)) &&
        Array.isArray(p.details) && p.details.length === 1 && object(p.details[0]) && id(p.details[0].sku_id) && qty(p.details[0].expected_qty) &&
        (value.status !== 'done' || (object(value.response) && id(value.response.id) && text(value.response.order_no)))
    case 'inbound-submit': case 'inbound-approve': case 'outbound-submit': case 'outbound-approve':
      return Object.keys(p).length === 0
    case 'receive':
      return id(p.detail_id) && qty(p.qty) && p.defective_qty === 0 && text(p.batch_no) && p.batch_no.length > 0
    case 'putaway': return id(p.location_id) && qty(p.qty)
    case 'pick': return qty(p.qty) && text(p.location_code) && p.location_code.length > 0 && text(p.batch_no) && p.batch_no.length > 0
    case 'location-create':
      return id(p.warehouse_id) && text(p.zone) && p.zone.startsWith('D') && p.row_from === 1 && p.row_to === 1 && p.col_from === 1 && p.col_to === 1
    default: return false
  }
}

export function validStagedSnapshot(value: unknown): value is StagedSnapshot {
  if (!object(value) || value.version !== 1 || !uuid(value.runId) ||
    !['step', 'auto'].includes(String(value.mode)) || !['full', 'inbound', 'outbound'].includes(String(value.scope)) ||
    !date(value.startedAt) || (value.completedAt !== '' && !date(value.completedAt))) return false
  const steps = executionSteps(value.scope as DemoRunScope)
  if (!Array.isArray(value.results) || value.index !== value.results.length || value.results.length > steps.length) return false
  if ((value.completedAt !== '') !== (value.results.length === steps.length)) return false
  if (![value.inboundID, value.outboundID].every((v) => v === '' || id(v)) || !text(value.inboundNo) || !text(value.outboundNo)) return false
  if (![value.warehouse, value.sku].every((v) => v === null || (object(v) && id(v.id) && text(v.code)))) return false
  if (!value.results.every((r, i) => object(r) && r.key === steps[i]?.key &&
    [r.title, r.objectNo, r.status, r.before, r.after, r.quantity, r.route].every(text) && date(r.createdAt) &&
    Array.isArray(r.facts) && r.facts.every((f) => object(f) && text(f.label) && text(f.value)))) return false
  if (!Array.isArray(value.writes) || value.writes.length > 200 || !value.writes.every(validWrite)) return false
  const writes = value.writes
  if (new Set(writes.map((w) => w.name)).size !== writes.length) return false
  if (new Set(writes.map((w) => w.operationKey)).size !== writes.length) return false
  if (value.results.some((r) => r.key !== 'inventory-check' && !writes.some((w) => w.step === r.key && w.status === 'done'))) return false
  if (value.results.some((r) => r.key === 'inbound-create') && (!id(value.inboundID) || !value.inboundNo)) return false
  if (value.results.some((r) => r.key === 'outbound-create') && (!id(value.outboundID) || !value.outboundNo)) return false
  return value.writes.every((w) => {
    const index = steps.findIndex((step) => step.key === w.step)
    return index >= 0 && index <= Number(value.index) && (index === value.index || w.status === 'done')
  })
}

function storageKey(identity: StagedIdentity): string {
  return STORAGE_PREFIX + encodeURIComponent(identity.userId) + ':' + encodeURIComponent(identity.sessionId)
}

export function loadStagedDemo(identity: StagedIdentity): { snapshot: StagedSnapshot | null; error: string } {
  try {
    const raw = sessionStorage.getItem(storageKey(identity))
    if (!raw) return { snapshot: null, error: '' }
    const snapshot: unknown = JSON.parse(raw)
    if (!validStagedSnapshot(snapshot)) throw new Error('invalid snapshot')
    return { snapshot, error: '' }
  } catch {
    return { snapshot: null, error: '演示进度无法读取或已损坏，请核对业务记录后使用“重新开始”重置演示。' }
  }
}

export function saveStagedDemo(identity: StagedIdentity, snapshot: StagedSnapshot): void {
  if (!identity.userId || !identity.sessionId || !validStagedSnapshot(snapshot)) throw new Error('演示进度无效，已停止提交')
  try { sessionStorage.setItem(storageKey(identity), JSON.stringify(snapshot)) } catch {
    throw new Error('浏览器无法保存演示进度，已停止提交，请检查存储空间后恢复。')
  }
}

/** 只能在服务端已成功重置演示数据后调用。 */
export function clearStagedDemo(identity: StagedIdentity): void {
  sessionStorage.removeItem(storageKey(identity))
}

export const STAGED_GROUPS = [
  { key: 'inbound-order', title: '入库单', subtitle: '创建与审核' },
  { key: 'receiving', title: '收货上架', subtitle: '收货与上架' },
  { key: 'inventory', title: '库存形成', subtitle: '库存与流水' },
  { key: 'outbound-order', title: '出库单', subtitle: '审核与 FIFO' },
  { key: 'pick-ship', title: '拣货发货', subtitle: '任务与发货' },
]
export const STAGED_EXECUTION_STEPS: ExecutionStep[] = [
  { key: 'inbound-create', group: 'inbound-order', title: '创建入库单', description: '创建真实入库草稿，完成后即可打开入库单页面查看。' },
  { key: 'inbound-submit', group: 'inbound-order', title: '提交入库单', description: '将入库单从草稿推进到已提交状态。' },
  { key: 'inbound-approve', group: 'inbound-order', title: '审核入库单', description: '审核通过并生成真实收货任务。' },
  { key: 'inbound-receive', group: 'receiving', title: '完成收货', description: '登记真实批次和收货数量。' },
  { key: 'inbound-putaway', group: 'receiving', title: '完成上架', description: '选择空闲库位并完成真实上架。' },
  { key: 'inventory-check', group: 'inventory', title: '核对库存与流水', description: '查询本次入库产生的库存流水。' },
  { key: 'outbound-create', group: 'outbound-order', title: '创建出库单', description: '创建真实出库草稿。' },
  { key: 'outbound-submit', group: 'outbound-order', title: '提交出库单', description: '将出库单推进到已提交状态。' },
  { key: 'outbound-allocate', group: 'outbound-order', title: '审核并 FIFO 分配', description: '执行真实 FIFO 分配并生成拣货任务。' },
  { key: 'outbound-pick', group: 'pick-ship', title: '完成拣货与发货', description: '按真实任务完成拣货和库存扣减。' },
]
