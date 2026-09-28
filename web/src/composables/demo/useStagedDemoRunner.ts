import { computed, ref } from 'vue'
import { batchCreateLocations, listLocations, listSkus, listWarehouses } from '@/api/basic'
import {
  approveInboundOrder,
  createInboundOrder,
  getInboundOrder,
  putawayInboundTask,
  receiveInbound,
  submitInboundOrder,
} from '@/api/inbound'
import { listInventoryTrans } from '@/api/inventory'
import {
  approveOutboundOrder,
  createOutboundOrder,
  getOutboundOrder,
  pickOutboundTask,
  submitOutboundOrder,
} from '@/api/outbound'
import type { EntityID, LocationItem, SkuItem, WarehouseItem } from '@/api/types'
import { rememberDemoExecutionWindow } from '@/utils/demoEvidence'

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

/**
 * 自动模式下的回放节奏：每步之间等待一段时间，便于观察真实执行过程。
 * 仅为展示节奏，不影响任何业务请求的先后顺序。
 */
const STEP_INTERVAL_MS = 900

/**
 * 分步自动演示的执行状态机。
 *
 * 职责：维护步骤列表、当前进度、逐步调用真实业务接口并记录结果。
 * 组件只负责展示与触发，不承担业务编排。
 */
export function useStagedDemoRunner(options: UseStagedDemoRunnerOptions = {}) {
  const mode = ref<DemoRunMode>('step')
  const scope = ref<DemoRunScope>('full')
  const started = ref(false)
  const currentIndex = ref(0)
  const results = ref<StepResult[]>([])
  const running = ref(false)
  const error = ref('')
  const runStartedAt = ref('')
  const runCompletedAt = ref('')
  const inboundID = ref<EntityID>('')
  const inboundNo = ref('')
  const outboundID = ref<EntityID>('')
  const outboundNo = ref('')
  let warehouse: WarehouseItem | null = null
  let sku: SkuItem | null = null

  const activeSteps = computed(() => {
    if (scope.value === 'outbound') return STAGED_EXECUTION_STEPS.filter((step) => ['outbound-create', 'outbound-submit', 'outbound-allocate', 'outbound-pick'].includes(step.key))
    if (scope.value === 'inbound') return STAGED_EXECUTION_STEPS.filter((step) => !['outbound-create', 'outbound-submit', 'outbound-allocate', 'outbound-pick'].includes(step.key))
    return STAGED_EXECUTION_STEPS
  })
  const currentStep = computed(() => activeSteps.value[currentIndex.value] ?? null)
  const completed = computed(() => started.value && currentIndex.value >= activeSteps.value.length)
  const progress = computed(() => Math.round((currentIndex.value / Math.max(activeSteps.value.length, 1)) * 100))
  const lastResult = computed(() => results.value.at(-1) ?? null)
  const currentGroupIndex = computed(() => {
    if (!currentStep.value) return STAGED_GROUPS.length - 1
    return STAGED_GROUPS.findIndex((group) => group.key === currentStep.value?.group)
  })

  const groupStates = computed(() => STAGED_GROUPS.map((group, index) => {
    const steps = activeSteps.value.filter((step) => step.group === group.key)
    if (!steps.length) return index < currentGroupIndex.value ? 'completed' : 'pending'
    const done = steps.every((step) => results.value.some((result) => result.key === step.key))
    if (done) return 'completed'
    if (index === currentGroupIndex.value) return currentStep.value ? 'active' : 'completed'
    return 'pending'
  }))

  function notifyState(): void {
    options.onStateChange?.({
      started: started.value,
      active: started.value && !completed.value,
      completed: completed.value,
      progress: progress.value,
      running: running.value,
    })
  }

  function start(runMode: DemoRunMode = 'step', runScope: DemoRunScope = 'full'): void {
    mode.value = runMode
    scope.value = runScope
    started.value = true
    currentIndex.value = 0
    results.value = []
    running.value = false
    error.value = ''
    runStartedAt.value = new Date().toISOString()
    runCompletedAt.value = ''
    inboundID.value = ''
    inboundNo.value = ''
    outboundID.value = ''
    outboundNo.value = ''
    warehouse = null
    sku = null
    notifyState()
    if (mode.value === 'auto') void runAll()
  }

  function record(result: StepResult): void {
    results.value = [...results.value, result]
    currentIndex.value += 1
    error.value = ''
    if (currentIndex.value >= activeSteps.value.length) {
      runCompletedAt.value = new Date().toISOString()
      rememberDemoExecutionWindow(
        scope.value,
        runStartedAt.value,
        runCompletedAt.value,
        results.value.map((item) => ({ label: item.title, path: item.route })),
      )
    }
    notifyState()
  }

  async function ensureRefs(): Promise<void> {
    if (warehouse && sku) return
    const [warehouses, skus] = await Promise.all([
      listWarehouses({ page: 1, page_size: 100, keyword: 'WH01' }),
      listSkus({ page: 1, page_size: 100, keyword: 'SKU000001' }),
    ])
    warehouse = warehouses.list.find((item) => item.code === 'WH01') ?? warehouses.list[0] ?? null
    sku = skus.list.find((item) => item.code === 'SKU000001') ?? skus.list[0] ?? null
    if (!warehouse || !sku) throw new Error('演示基础数据不存在')
  }

  async function idleLocation(): Promise<LocationItem> {
    if (!warehouse) throw new Error('演示仓库不存在')
    const locations = await listLocations({ page: 1, page_size: 100, warehouse_id: warehouse.id, status: 1 })
    const location = locations.list[0]
    if (location) return location

    const zone = `D${Date.now().toString(36).toUpperCase()}`
    await batchCreateLocations({
      warehouse_id: warehouse.id,
      zone,
      row_from: 1,
      row_to: 1,
      col_from: 1,
      col_to: 1,
    })
    const created = await listLocations({
      page: 1,
      page_size: 10,
      warehouse_id: warehouse.id,
      status: 1,
      keyword: zone,
    })
    const fallback = created.list[0]
    if (!fallback) throw new Error('没有可用库位，自动创建库位失败，请重置演示数据')
    return fallback
  }

  async function executeInboundCreate(): Promise<void> {
    await ensureRefs()
    const order = await createInboundOrder({
      warehouse_id: warehouse!.id,
      remark: '分步演示：创建入库单',
      details: [{ sku_id: sku!.id, expected_qty: 10 }],
    })
    inboundID.value = order.id
    inboundNo.value = order.order_no
    record({
      key: 'inbound-create', title: '创建入库单', objectNo: order.order_no,
      status: '草稿', before: '-', after: '草稿', quantity: '计划 +10 件',
      createdAt: new Date().toISOString(), route: `/inbound/orders/${order.id}`,
      facts: [{ label: '业务对象', value: order.order_no }, { label: '当前状态', value: '草稿' }, { label: '计划数量', value: '10 件' }],
    })
  }

  async function executeInboundSubmit(): Promise<void> {
    await submitInboundOrder(inboundID.value)
    record({
      key: 'inbound-submit', title: '提交入库单', objectNo: inboundNo.value,
      status: '已提交', before: '草稿', after: '已提交', quantity: '-',
      createdAt: new Date().toISOString(), route: `/inbound/orders/${inboundID.value}`,
      facts: [{ label: '业务对象', value: inboundNo.value }, { label: '状态变化', value: '草稿 → 已提交' }],
    })
  }

  async function executeInboundApprove(): Promise<void> {
    await approveInboundOrder(inboundID.value)
    record({
      key: 'inbound-approve', title: '审核入库单', objectNo: inboundNo.value,
      status: '已审核', before: '已提交', after: '已审核·待收货', quantity: '-',
      createdAt: new Date().toISOString(), route: `/inbound/orders/${inboundID.value}`,
      facts: [{ label: '业务对象', value: inboundNo.value }, { label: '状态变化', value: '已提交 → 已审核' }, { label: '后续任务', value: '生成收货任务' }],
    })
  }

  async function executeInboundReceive(): Promise<void> {
    const detail = await getInboundOrder(inboundID.value)
    const line = detail.details[0]
    if (!line) throw new Error('入库单没有明细')
    const qty = Math.max(line.expected_qty - line.received_qty, 1)
    const batchNo = 'B' + Date.now()
    await receiveInbound(inboundID.value, { detail_id: line.id, qty, defective_qty: 0, batch_no: batchNo })
    record({
      key: 'inbound-receive', title: '完成收货', objectNo: inboundNo.value,
      status: '已收货', before: '已审核', after: '收货完成', quantity: `+${qty} 件`,
      createdAt: new Date().toISOString(), route: `/inbound/orders/${inboundID.value}`,
      facts: [{ label: '业务对象', value: inboundNo.value }, { label: '批次', value: batchNo }, { label: '收货数量', value: qty + ' 件' }],
    })
  }

  async function executeInboundPutaway(): Promise<void> {
    const detail = await getInboundOrder(inboundID.value)
    const task = detail.tasks.find((item) => item.task_type === 'PUTAWAY' && item.status !== 'COMPLETED' && item.done_qty < item.target_qty)
    if (!task) throw new Error('没有待上架任务')
    const location = await idleLocation()
    const qty = task.target_qty - task.done_qty
    await putawayInboundTask(task.id, { location_id: location.id, qty })
    record({
      key: 'inbound-putaway', title: '完成上架', objectNo: task.task_no,
      status: '已完成', before: '待上架', after: '库存已增加', quantity: `+${qty} 件`,
      createdAt: new Date().toISOString(), route: `/inbound/orders/${inboundID.value}`,
      facts: [{ label: '上架任务', value: task.task_no }, { label: '库位', value: location.code }, { label: '上架数量', value: qty + ' 件' }],
    })
  }

  async function executeInventoryCheck(): Promise<void> {
    const data = await listInventoryTrans({ page: 1, page_size: 20, order_no: inboundNo.value })
    if (!data.list.length) throw new Error('没有查询到本次入库流水')
    const change = data.list.reduce((total, item) => total + item.quantity_change, 0)
    record({
      key: 'inventory-check', title: '核对库存与流水', objectNo: inboundNo.value,
      status: '可追溯', before: '-', after: '库存已入账', quantity: `${change > 0 ? '+' : ''}${change} 件`,
      createdAt: new Date().toISOString(), route: `/inventory?order_no=${encodeURIComponent(inboundNo.value)}`,
      facts: [{ label: '关联单据', value: inboundNo.value }, { label: '流水数量', value: data.total + ' 条' }, { label: '库存变化', value: change + ' 件' }],
    })
  }

  async function executeOutboundCreate(): Promise<void> {
    await ensureRefs()
    const order = await createOutboundOrder({
      warehouse_id: warehouse!.id,
      biz_order_no: 'STEP' + Date.now(),
      remark: '分步演示：创建出库单',
      details: [{ sku_id: sku!.id, expected_qty: 20 }],
    })
    outboundID.value = order.id
    outboundNo.value = order.order_no
    record({
      key: 'outbound-create', title: '创建出库单', objectNo: order.order_no,
      status: '草稿', before: '-', after: '草稿', quantity: '计划 -20 件',
      createdAt: new Date().toISOString(), route: `/outbound/orders/${order.id}`,
      facts: [{ label: '业务对象', value: order.order_no }, { label: '当前状态', value: '草稿' }, { label: '需求数量', value: '20 件' }],
    })
  }

  async function executeOutboundSubmit(): Promise<void> {
    await submitOutboundOrder(outboundID.value)
    record({
      key: 'outbound-submit', title: '提交出库单', objectNo: outboundNo.value,
      status: '已提交', before: '草稿', after: '已提交', quantity: '-',
      createdAt: new Date().toISOString(), route: `/outbound/orders/${outboundID.value}`,
      facts: [{ label: '业务对象', value: outboundNo.value }, { label: '状态变化', value: '草稿 → 已提交' }],
    })
  }

  async function executeOutboundAllocate(): Promise<void> {
    await approveOutboundOrder(outboundID.value)
    const detail = await getOutboundOrder(outboundID.value)
    const allocated = detail.allocations.reduce((total, item) => total + item.allocated_qty, 0)
    record({
      key: 'outbound-allocate', title: '审核并 FIFO 分配', objectNo: outboundNo.value,
      status: '已分配', before: '已提交', after: 'FIFO 分配完成', quantity: `-${allocated} 件可用库存`,
      createdAt: new Date().toISOString(), route: `/outbound/orders/${outboundID.value}`,
      facts: [{ label: '业务对象', value: outboundNo.value }, { label: 'FIFO 批次', value: detail.allocations.length + ' 个' }, { label: '分配数量', value: allocated + ' 件' }],
    })
  }

  async function executeOutboundPick(): Promise<void> {
    const detail = await getOutboundOrder(outboundID.value)
    const tasks = detail.tasks.filter((task) => task.task_type === 'PICK' && task.status !== 'COMPLETED' && task.done_qty < task.target_qty)
    let picked = 0
    for (const task of tasks) {
      const qty = task.target_qty - task.done_qty
      await pickOutboundTask(task.id, { qty, location_code: task.location_code, batch_no: task.batch_no })
      picked += qty
    }
    if (!tasks.length) throw new Error('没有待拣货任务')
    record({
      key: 'outbound-pick', title: '完成拣货与发货', objectNo: outboundNo.value,
      status: '已发货', before: '拣货中', after: '已发货', quantity: `-${picked} 件库存`,
      createdAt: new Date().toISOString(), route: `/tasks?order_id=${outboundID.value}`,
      facts: [{ label: '出库单', value: outboundNo.value }, { label: '完成任务', value: tasks.length + ' 个' }, { label: '拣货数量', value: picked + ' 件' }],
    })
  }

  async function executeNext(): Promise<void> {
    if (!started.value || completed.value || running.value || !currentStep.value) return
    running.value = true
    error.value = ''
    notifyState()
    try {
      const actions: Record<StepKey, () => Promise<void>> = {
        'inbound-create': executeInboundCreate,
        'inbound-submit': executeInboundSubmit,
        'inbound-approve': executeInboundApprove,
        'inbound-receive': executeInboundReceive,
        'inbound-putaway': executeInboundPutaway,
        'inventory-check': executeInventoryCheck,
        'outbound-create': executeOutboundCreate,
        'outbound-submit': executeOutboundSubmit,
        'outbound-allocate': executeOutboundAllocate,
        'outbound-pick': executeOutboundPick,
      }
      await actions[currentStep.value.key]()
    } catch (err) {
      error.value = err instanceof Error ? err.message : '当前步骤执行失败'
    } finally {
      running.value = false
      notifyState()
    }
  }

  async function validateCurrentPage(): Promise<boolean> {
    try {
      const key = lastResult.value?.key
      if (key?.startsWith('inbound') && inboundID.value) await getInboundOrder(inboundID.value)
      if (key?.startsWith('outbound') && outboundID.value) await getOutboundOrder(outboundID.value)
      if (key === 'inventory-check' && inboundNo.value) {
        const data = await listInventoryTrans({ page: 1, page_size: 1, order_no: inboundNo.value })
        if (!data.list.length) throw new Error('库存流水不存在')
      }
      return true
    } catch {
      error.value = '当前演示数据已重置或对象不存在，请重新开始分步执行。'
      return false
    }
  }

  async function runAll(): Promise<void> {
    while (started.value && !completed.value && mode.value === 'auto') {
      await executeNext()
      if (error.value) return
      if (completed.value) break
      await new Promise((resolve) => window.setTimeout(resolve, STEP_INTERVAL_MS))
    }
  }

  return {
    mode,
    scope,
    started,
    currentIndex,
    results,
    running,
    error,
    runStartedAt,
    runCompletedAt,
    currentStep,
    completed,
    progress,
    lastResult,
    groupStates,
    start,
    executeNext,
    validateCurrentPage,
  }
}
