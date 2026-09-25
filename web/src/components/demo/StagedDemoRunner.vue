<script setup lang="ts">
import { computed, ref } from 'vue'
import { CircleCheckFilled, Clock, Document, VideoPlay } from '@element-plus/icons-vue'
import { listLocations, listSkus, listWarehouses } from '@/api/basic'
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
import { formatTime } from '@/utils'

type StepKey =
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

interface ExecutionStep {
  key: StepKey
  group: string
  title: string
  description: string
}

interface StepFact {
  label: string
  value: string
}

interface StepResult {
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

type DemoRunMode = 'auto' | 'step'
type DemoRunScope = 'full' | 'inbound' | 'outbound'

const emit = defineEmits<{
  navigate: [path: string]
  'state-change': [state: { started: boolean; active: boolean; completed: boolean; progress: number; running: boolean }]
}>()

const GROUPS = [
  { key: 'inbound-order', title: '入库单', subtitle: '创建与审核' },
  { key: 'receiving', title: '收货上架', subtitle: '收货与上架' },
  { key: 'inventory', title: '库存形成', subtitle: '库存与流水' },
  { key: 'outbound-order', title: '出库单', subtitle: '审核与 FIFO' },
  { key: 'pick-ship', title: '拣货发货', subtitle: '任务与发货' },
]

const EXECUTION_STEPS: ExecutionStep[] = [
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
  if (scope.value === 'outbound') return EXECUTION_STEPS.filter((step) => ['outbound-create', 'outbound-submit', 'outbound-allocate', 'outbound-pick'].includes(step.key))
  if (scope.value === 'inbound') return EXECUTION_STEPS.filter((step) => !['outbound-create', 'outbound-submit', 'outbound-allocate', 'outbound-pick'].includes(step.key))
  return EXECUTION_STEPS
})
const currentStep = computed(() => activeSteps.value[currentIndex.value] ?? null)
const completed = computed(() => started.value && currentIndex.value >= activeSteps.value.length)
const progress = computed(() => Math.round((currentIndex.value / Math.max(activeSteps.value.length, 1)) * 100))
const lastResult = computed(() => results.value.at(-1) ?? null)
const currentGroupIndex = computed(() => {
  if (!currentStep.value) return GROUPS.length - 1
  return GROUPS.findIndex((group) => group.key === currentStep.value?.group)
})

const groupStates = computed(() => GROUPS.map((group, index) => {
  const steps = activeSteps.value.filter((step) => step.group === group.key)
  if (!steps.length) return index < currentGroupIndex.value ? 'completed' : 'pending'
  const done = steps.every((step) => results.value.some((result) => result.key === step.key))
  if (done) return 'completed'
  if (index === currentGroupIndex.value) return currentStep.value ? 'active' : 'completed'
  return 'pending'
}))

function emitState(): void {
  emit('state-change', {
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
  emitState()
  if (mode.value === 'auto') void runAll()
}

function record(result: StepResult): void {
  results.value = [...results.value, result]
  currentIndex.value += 1
  error.value = ''
  if (currentIndex.value >= activeSteps.value.length) runCompletedAt.value = new Date().toISOString()
  emitState()
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
  if (!location) throw new Error('没有可用库位')
  return location
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
  emitState()
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
    emitState()
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

async function openCurrentPage(): Promise<void> {
  if (await validateCurrentPage()) {
    emit('navigate', lastResult.value?.route || '/demo')
  }
}

function openLogs(): void {
  const params = new globalThis.URLSearchParams({ tab: 'operations', source: 'staged', scenario: 'full' })
  if (runStartedAt.value) params.set('started_at', runStartedAt.value)
  if (runCompletedAt.value) params.set('completed_at', runCompletedAt.value)
  emit('navigate', '/demo/activity?' + params.toString())
}

async function runAll(): Promise<void> {
  while (started.value && !completed.value && mode.value === 'auto') {
    await executeNext()
    if (completed.value) break
    await new Promise((resolve) => window.setTimeout(resolve, 900))
  }
}

defineExpose({ start })
</script>

<template>
  <section class="staged-runner" aria-label="分步执行自动演示">
    <header class="staged-head">
      <div>
        <span>自动演示 · 分步执行</span>
        <h3>{{ completed ? '全部流程已完成' : currentStep?.title }}</h3>
        <p>{{ completed ? '每一步都通过真实业务接口执行，可以继续打开页面核对。' : currentStep?.description }}</p>
      </div>
      <el-tag :type="completed ? 'success' : 'primary'" effect="plain">{{ completed ? '已完成' : `${currentIndex} / ${EXECUTION_STEPS.length}` }}</el-tag>
    </header>

    <el-progress :percentage="progress" :status="completed ? 'success' : undefined" :show-text="false" :stroke-width="5" />

    <div class="staged-layout">
      <aside class="stage-list">
        <span class="panel-title">业务步骤</span>
        <div v-for="(group, index) in GROUPS" :key="group.key" class="stage-item" :class="groupStates[index]">
          <span>{{ String(index + 1).padStart(2, '0') }}</span>
          <div><b>{{ group.title }}</b><small>{{ group.subtitle }}</small></div>
          <el-icon v-if="groupStates[index] === 'completed'"><CircleCheckFilled /></el-icon>
          <el-icon v-else><Clock /></el-icon>
        </div>
      </aside>

      <main class="stage-focus">
        <span class="panel-title">当前执行</span>
        <div v-if="!completed" class="next-step-card">
          <b>{{ currentStep?.title }}</b>
          <p>{{ currentStep?.description }}</p>
          <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
          <el-button v-if="mode === 'step'" type="primary" :icon="VideoPlay" :loading="running" @click="executeNext">执行下一步</el-button>
          <el-tag v-else type="primary" effect="plain">自动执行中，每步间隔约 0.9 秒</el-tag>
        </div>
        <el-result v-else icon="success" title="分步流程已完成" sub-title="所有步骤均调用真实业务接口完成。" />

        <div v-if="results.length" class="executed-list">
          <div v-for="item in results" :key="item.key" class="executed-item">
            <el-icon><CircleCheckFilled /></el-icon>
            <div><b>{{ item.title }}</b><span>{{ item.objectNo }} · {{ formatTime(item.createdAt) }}</span></div>
          </div>
        </div>
      </main>

      <aside class="stage-facts">
        <span class="panel-title">最近一步结果</span>
        <template v-if="lastResult">
          <dl>
            <div><dt>业务对象</dt><dd>{{ lastResult.objectNo }}</dd></div>
            <div><dt>当前状态</dt><dd>{{ lastResult.status }}</dd></div>
            <div><dt>状态变化</dt><dd>{{ lastResult.before }} → {{ lastResult.after }}</dd></div>
            <div><dt>数量变化</dt><dd>{{ lastResult.quantity }}</dd></div>
            <div v-for="fact in lastResult.facts" :key="fact.label"><dt>{{ fact.label }}</dt><dd>{{ fact.value }}</dd></div>
          </dl>
          <div class="stage-actions">
            <el-button size="small" :disabled="running" @click="openCurrentPage">打开真实页面</el-button>
            <el-button size="small" type="primary" plain :icon="Document" @click="openLogs">操作日志</el-button>
          </div>
        </template>
        <el-empty v-else description="点击“执行下一步”后显示真实结果" :image-size="62" />
      </aside>
    </div>
  </section>
</template>

<style scoped>
.staged-runner { display: grid; gap: 14px; color: var(--el-text-color-primary); }
.staged-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
.staged-head span, .panel-title { color: var(--el-color-primary); font-size: 11px; font-weight: 700; letter-spacing: 0.05em; }
.staged-head h3 { margin: 4px 0 3px; font-size: 21px; }
.staged-head p { margin: 0; color: var(--el-text-color-secondary); font-size: 13px; line-height: 1.6; }
.staged-layout { min-height: 360px; display: grid; grid-template-columns: 205px minmax(0, 1fr) 270px; gap: 12px; }
.stage-list, .stage-focus, .stage-facts { min-width: 0; padding: 14px; border: 1px solid var(--el-border-color-light); border-radius: 12px; background: var(--el-fill-color-extra-light); }
.stage-list { display: grid; align-content: start; gap: 7px; }
.stage-item { padding: 10px; border-radius: 9px; display: grid; grid-template-columns: 26px minmax(0, 1fr) 16px; align-items: center; gap: 8px; background: var(--el-bg-color); color: var(--el-text-color-regular); }
.stage-item.active { border: 1px solid var(--el-color-primary-light-5); background: var(--el-color-primary-light-9); }
.stage-item.completed .el-icon { color: var(--el-color-success); }
.stage-item b, .stage-item small { display: block; }
.stage-item small { margin-top: 3px; color: var(--el-text-color-secondary); font-size: 11px; }
.stage-focus { display: flex; flex-direction: column; background: var(--el-bg-color); }
.next-step-card { margin-top: 10px; padding: 14px; border-radius: 10px; background: var(--el-fill-color-lighter); }
.next-step-card b { font-size: 18px; }
.next-step-card p { margin: 6px 0 14px; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.6; }
.executed-list { display: grid; gap: 7px; margin-top: 14px; }
.executed-item { padding: 8px 9px; border-radius: 8px; display: flex; gap: 8px; color: var(--el-color-success); background: var(--el-color-success-light-9); }
.executed-item div { min-width: 0; }
.executed-item b, .executed-item span { display: block; }
.executed-item b { color: var(--el-text-color-primary); font-size: 12px; }
.executed-item span { margin-top: 3px; color: var(--el-text-color-secondary); font-size: 11px; }
.stage-facts { display: flex; flex-direction: column; }
.stage-facts dl { margin: 8px 0 0; display: grid; gap: 7px; }
.stage-facts dl > div { padding: 8px 9px; border-radius: 8px; background: var(--el-bg-color); }
.stage-facts dt { color: var(--el-text-color-secondary); font-size: 11px; }
.stage-facts dd { margin: 3px 0 0; overflow-wrap: anywhere; font-size: 12px; font-weight: 600; }
.stage-actions { display: flex; flex-wrap: wrap; gap: 7px; margin-top: auto; padding-top: 12px; }
.stage-actions .el-button { margin-left: 0; }
@media (max-width: 980px) { .staged-layout { grid-template-columns: 1fr; } .stage-list { grid-template-columns: repeat(5, minmax(0, 1fr)); } }
</style>
