import { computed, getCurrentScope, onScopeDispose, ref, watch } from 'vue'
import { listLocations, listSkus, listWarehouses } from '@/api/basic'
import { getInboundOrder, listInboundOrders } from '@/api/inbound'
import { listInventoryTrans } from '@/api/inventory'
import { getOutboundOrder, listOutboundOrders } from '@/api/outbound'
import { executeStagedRequest, type StagedRequest } from '@/api/demoExecution'
import { useAuthStore } from '@/stores/auth'
import { rememberDemoExecutionWindow } from '@/utils/demoEvidence'
import {
  executionSteps, loadStagedDemo, saveStagedDemo, STAGED_GROUPS, STAGED_RETENTION_MS,
  type DemoRunMode, type DemoRunScope, type DemoWrite, type StagedIdentity, type StagedSnapshot,
  type StepResult, type UseStagedDemoRunnerOptions,
} from '@/utils/stagedDemoState'
export { STAGED_GROUPS, STAGED_EXECUTION_STEPS } from '@/utils/stagedDemoState'

type Guard = () => void
const STEP_INTERVAL_MS = 900

/** 一次演示的进度与写请求一起落盘；刷新后始终暂停，由用户明确恢复。 */
export function useStagedDemoRunner(options: UseStagedDemoRunnerOptions = {}) {
  const auth = useAuthStore()
  const mode = ref<DemoRunMode>('step')
  const scope = ref<DemoRunScope>('full')
  const started = ref(false)
  const paused = ref(false)
  const currentIndex = ref(0)
  const results = ref<StepResult[]>([])
  const running = ref(false)
  const error = ref('')
  const runStartedAt = ref('')
  const runCompletedAt = ref('')
  const inboundID = ref('')
  const inboundNo = ref('')
  const outboundID = ref('')
  const outboundNo = ref('')
  let runId = ''
  let warehouse: StagedSnapshot['warehouse'] = null
  let sku: StagedSnapshot['sku'] = null
  let writes: DemoWrite[] = []
  let owner: StagedIdentity = { userId: '', sessionId: '' }
  let generation = 0
  let disposed = false
  let storageError = ''
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let wake: (() => void) | undefined

  const activeSteps = computed(() => executionSteps(scope.value))
  const currentStep = computed(() => activeSteps.value[currentIndex.value] ?? null)
  const completed = computed(() => started.value && currentIndex.value >= activeSteps.value.length)
  const progress = computed(() => Math.round(currentIndex.value / activeSteps.value.length * 100))
  const lastResult = computed(() => results.value.at(-1) ?? null)
  const groupStates = computed(() => STAGED_GROUPS.map((group) => {
    const steps = activeSteps.value.filter((step) => step.group === group.key)
    if (steps.length && steps.every((step) => results.value.some((r) => r.key === step.key))) return 'completed'
    return currentStep.value?.group === group.key ? 'active' : 'pending'
  }))
  function identity(): StagedIdentity {
    return { userId: auth.isDemo ? String(auth.user?.user_id || '') : '', sessionId: auth.isDemo ? auth.demoSessionId : '' }
  }
  function sameIdentity(): boolean {
    const current = identity()
    return !!owner.userId && !!owner.sessionId && owner.userId === current.userId && owner.sessionId === current.sessionId
  }
  function notifyState(): void {
    options.onStateChange?.({ started: started.value, active: started.value && !completed.value,
      completed: completed.value, progress: progress.value, running: running.value })
  }
  function snapshot(): StagedSnapshot {
    return { version: 1, runId, mode: mode.value, scope: scope.value, startedAt: runStartedAt.value,
      completedAt: runCompletedAt.value, index: currentIndex.value, results: results.value,
      inboundID: inboundID.value, inboundNo: inboundNo.value, outboundID: outboundID.value, outboundNo: outboundNo.value,
      warehouse, sku, writes }
  }
  function install(value: StagedSnapshot): void {
    runId = value.runId; mode.value = value.mode; scope.value = value.scope; runStartedAt.value = value.startedAt
    runCompletedAt.value = value.completedAt; currentIndex.value = value.index; results.value = value.results
    inboundID.value = value.inboundID; inboundNo.value = value.inboundNo
    outboundID.value = value.outboundID; outboundNo.value = value.outboundNo
    warehouse = value.warehouse; sku = value.sku; writes = value.writes; started.value = true
  }
  function persist(change: Partial<StagedSnapshot> = {}): void {
    if (!sameIdentity() || disposed) throw new Error('演示会话已经切换，已停止执行')
    const next = { ...snapshot(), ...change }
    saveStagedDemo(owner, next)
    install(next)
  }
  function pause(): void {
    generation++
    paused.value = true
    controller?.abort()
    if (timer !== undefined) clearTimeout(timer)
    timer = undefined
    wake?.(); wake = undefined
    notifyState()
  }
  function restore(): void {
    pause()
    owner = identity()
    started.value = false; currentIndex.value = 0; results.value = []; writes = []
    runId = ''; runStartedAt.value = ''; runCompletedAt.value = ''
    inboundID.value = ''; inboundNo.value = ''; outboundID.value = ''; outboundNo.value = ''
    warehouse = null; sku = null; error.value = ''; storageError = ''
    if (owner.userId && owner.sessionId) {
      const loaded = loadStagedDemo(owner)
      storageError = loaded.error
      error.value = loaded.error
      if (loaded.snapshot) install(loaded.snapshot)
    }
    notifyState()
  }
  function start(runMode: DemoRunMode = 'step', runScope: DemoRunScope = 'full'): void {
    if (running.value || disposed) return
    if (storageError) { error.value = storageError; return }
    if (started.value && !completed.value) {
      error.value = '已恢复尚未完成的演示，请继续当前步骤；重新开始需先重置演示数据。'
      paused.value = true
      return
    }
    pause()
    owner = identity()
    const initial: StagedSnapshot = { version: 1, runId: crypto.randomUUID(), mode: runMode, scope: runScope,
      startedAt: new Date().toISOString(), completedAt: '', index: 0, results: [], inboundID: '', inboundNo: '',
      outboundID: '', outboundNo: '', warehouse: null, sku: null, writes: [] }
    try {
      saveStagedDemo(owner, initial)
      install(initial); paused.value = false; error.value = ''; notifyState()
      if (mode.value === 'auto') void runAll()
    } catch (e) { error.value = e instanceof Error ? e.message : '无法保存演示进度' }
  }
  async function read<T>(fn: () => Promise<T>, guard: Guard): Promise<T> {
    guard(); const result = await fn(); guard(); return result
  }
  async function ensureRefs(guard: Guard): Promise<void> {
    if (warehouse && sku) return
    const [warehouses, skus] = await read(() => Promise.all([
      listWarehouses({ page: 1, page_size: 100, keyword: 'WH01' }),
      listSkus({ page: 1, page_size: 100, keyword: 'SKU000001' }),
    ]), guard)
    const w = warehouses.list.find((item) => item.code === 'WH01')
    const s = skus.list.find((item) => item.code === 'SKU000001')
    if (!w || !s) throw new Error('演示基础数据不存在')
    persist({ warehouse: { id: String(w.id), code: w.code }, sku: { id: String(s.id), code: s.code } })
  }
  /** 没有请求级幂等的写操作丢响应后只读核对，查不到并不等于从未执行。 */
  async function reconcile(write: DemoWrite, guard: Guard): Promise<unknown> {
    const p = write.payload
    if (write.kind === 'inbound-create' || write.kind === 'outbound-create') {
      const inbound = write.kind === 'inbound-create'
      const matches: { id: string; order_no: string }[] = []
      for (let page = 1; page <= 20; page++) {
        const data = inbound
          ? await read(() => listInboundOrders({ page, page_size: 100 }), guard)
          : await read(() => listOutboundOrders({ page, page_size: 100 }), guard)
        for (const row of data.list) {
          if (row.remark === p.remark && String(row.warehouse_id) === p.warehouse_id &&
            (inbound || ('biz_order_no' in row && row.biz_order_no === p.biz_order_no))) matches.push({ id: String(row.id), order_no: row.order_no })
        }
        if (page * 100 >= data.total) break
        if (page === 20) throw new Error('记录较多，无法安全核对，请打开真实页面检查')
      }
      if (matches.length === 1) {
        const found = matches[0]!
        const detail = inbound ? await read(() => getInboundOrder(found.id), guard) : await read(() => getOutboundOrder(found.id), guard)
        const expected = (p.details as { sku_id: string; expected_qty: number }[])[0]!
        if (detail.details.length === 1 && String(detail.details[0]!.sku_id) === expected.sku_id && detail.details[0]!.expected_qty === expected.expected_qty) return found
      }
    } else if (write.kind === 'location-create') {
      const data = await read(() => listLocations({ page: 1, page_size: 100, warehouse_id: String(p.warehouse_id), keyword: String(p.zone) }), guard)
      if (data.list.filter((l) => l.code === `${p.zone}-01-01`).length === 1) return null
    } else {
      const inbound = write.kind.startsWith('inbound')
      const detail = inbound ? await read(() => getInboundOrder(write.objectId), guard) : await read(() => getOutboundOrder(write.objectId), guard)
      const states = inbound
        ? write.kind === 'inbound-submit' ? ['SUBMITTED', 'APPROVED', 'RECEIVING', 'PUTAWAY', 'COMPLETED'] : ['APPROVED', 'RECEIVING', 'PUTAWAY', 'COMPLETED']
        : write.kind === 'outbound-submit' ? ['SUBMITTED', 'APPROVED', 'PICKING', 'SHIPPED'] : ['APPROVED', 'PICKING', 'SHIPPED']
      if (states.includes(detail.order.status)) return null
    }
    throw new Error('上次请求结果仍待核对，未重新发送。请稍后恢复，或到真实页面确认并完成该步骤；也可重置演示后重新开始。')
  }
  async function write(name: string, prepare: () => StagedRequest | Promise<StagedRequest>, guard: Guard): Promise<DemoWrite> {
    guard()
    let operation = writes.find((item) => item.name === name)
    const fresh = !operation
    if (!operation) {
      const request = await prepare(); guard()
      operation = { ...request, name, step: currentStep.value!.key, operationKey: crypto.randomUUID(), status: 'sending', response: null }
      persist({ writes: [...writes, operation] })
    }
    if (operation.status === 'done') return operation
    const keyed = ['receive', 'putaway', 'pick'].includes(operation.kind)
    const reply = fresh || keyed
      ? await executeStagedRequest(operation, operation.operationKey, controller!.signal)
      : await reconcile(operation, guard)
    guard()
    let response: unknown = null
    if (operation.kind.endsWith('-create') && operation.kind !== 'location-create') {
      const order = reply as { id?: unknown; order_no?: unknown } | null
      if (!order?.id || typeof order.order_no !== 'string') throw new Error('建单响应不完整，请恢复核对')
      response = { id: String(order.id), order_no: order.order_no }
    }
    const done: DemoWrite = { ...operation, status: 'done', response }
    persist({ writes: writes.map((item) => item.name === name ? done : item) })
    return done
  }
  async function idleLocation(guard: Guard) {
    const existing = writes.find((w) => w.name === 'location')
    if (existing) await write('location', () => existing, guard)
    let data = await read(() => listLocations({ page: 1, page_size: 100, warehouse_id: warehouse!.id, status: 1 }), guard)
    if (data.list[0]) return data.list[0]
    const zone = 'D' + runId.replaceAll('-', '').slice(0, 16).toUpperCase()
    await write('location', () => ({ kind: 'location-create', objectId: '', payload: {
      warehouse_id: warehouse!.id, zone, row_from: 1, row_to: 1, col_from: 1, col_to: 1,
    } }), guard)
    data = await read(() => listLocations({ page: 1, page_size: 100, warehouse_id: warehouse!.id, status: 1, keyword: zone }), guard)
    if (!data.list[0]) throw new Error('没有可用库位，请核对或重置演示数据')
    return data.list[0]
  }
  function finish(guard: Guard, result: Omit<StepResult, 'key' | 'title' | 'createdAt'>): void {
    guard()
    const step = currentStep.value!
    const next = [...results.value, { ...result, key: step.key, title: step.title, createdAt: new Date().toISOString() }]
    const finishedAt = next.length === activeSteps.value.length ? new Date().toISOString() : ''
    persist({ results: next, index: next.length, completedAt: finishedAt })
    if (finishedAt) rememberDemoExecutionWindow(scope.value, runStartedAt.value, finishedAt, next.map((r) => ({ label: r.title, path: r.route })))
    notifyState()
  }
  async function executeStep(guard: Guard): Promise<void> {
    const key = currentStep.value!.key
    if (key === 'inbound-create' || key === 'outbound-create') {
      await ensureRefs(guard)
      const inbound = key === 'inbound-create'
      const op = await write(key, () => ({ kind: key, objectId: '', payload: {
        warehouse_id: warehouse!.id, remark: `分步演示：${runId}：${inbound ? '创建入库单' : '创建出库单'}`,
        ...(inbound ? {} : { biz_order_no: 'STEP' + runId }),
        details: [{ sku_id: sku!.id, expected_qty: inbound ? 10 : 20 }],
      } }), guard)
      const order = op.response as { id: string; order_no: string }
      persist(inbound ? { inboundID: order.id, inboundNo: order.order_no } : { outboundID: order.id, outboundNo: order.order_no })
      finish(guard, { objectNo: order.order_no, status: '草稿', before: '-', after: '草稿', quantity: `计划 ${inbound ? '+10' : '20'} 件`,
        route: `/${inbound ? 'inbound' : 'outbound'}/orders/${order.id}`, facts: [{ label: '业务对象', value: order.order_no }] })
    } else if (['inbound-submit', 'inbound-approve', 'outbound-submit', 'outbound-allocate'].includes(key)) {
      const inbound = key.startsWith('inbound')
      const submit = key.endsWith('submit')
      const id = inbound ? inboundID.value : outboundID.value
      await write(key, () => ({ kind: key === 'outbound-allocate' ? 'outbound-approve' : key as StagedRequest['kind'], objectId: id, payload: {} }), guard)
      finish(guard, { objectNo: inbound ? inboundNo.value : outboundNo.value, status: submit ? '已提交' : '已审核',
        before: submit ? '草稿' : '已提交', after: submit ? '已提交' : inbound ? '已审核·待收货' : 'FIFO 分配完成', quantity: '-',
        route: `/${inbound ? 'inbound' : 'outbound'}/orders/${id}`, facts: [{ label: '状态变化', value: submit ? '草稿 → 已提交' : '审核完成' }] })
    } else if (key === 'inbound-receive') {
      const op = await write('receive', async () => {
        const detail = await read(() => getInboundOrder(inboundID.value), guard)
        const line = detail.details[0]
        if (!line || line.expected_qty <= line.received_qty) throw new Error('没有待收货明细，请核对真实单据')
        return { kind: 'receive', objectId: inboundID.value, payload: { detail_id: String(line.id), qty: line.expected_qty - line.received_qty,
          defective_qty: 0, batch_no: 'B' + runId.replaceAll('-', '').slice(0, 20) } }
      }, guard)
      finish(guard, { objectNo: inboundNo.value, status: '已收货', before: '已审核', after: '收货完成', quantity: `+${op.payload.qty} 件`,
        route: `/inbound/orders/${inboundID.value}`, facts: [{ label: '批次', value: String(op.payload.batch_no) }] })
    } else if (key === 'inbound-putaway') {
      const op = await write('putaway', async () => {
        const detail = await read(() => getInboundOrder(inboundID.value), guard)
        const task = detail.tasks.find((t) => t.task_type === 'PUTAWAY' && t.done_qty < t.target_qty && t.status !== 'COMPLETED')
        if (!task) throw new Error('没有待上架任务，请核对真实单据')
        const location = await idleLocation(guard)
        return { kind: 'putaway', objectId: String(task.id), payload: { location_id: String(location.id), qty: task.target_qty - task.done_qty } }
      }, guard)
      finish(guard, { objectNo: inboundNo.value, status: '已完成', before: '待上架', after: '库存已增加', quantity: `+${op.payload.qty} 件`,
        route: `/inbound/orders/${inboundID.value}`, facts: [{ label: '上架任务 ID', value: op.objectId }, { label: '库位 ID', value: String(op.payload.location_id) }] })
    } else if (key === 'inventory-check') {
      const data = await read(() => listInventoryTrans({ page: 1, page_size: 20, order_no: inboundNo.value }), guard)
      if (!data.list.length) throw new Error('没有查询到本次入库流水')
      const change = data.list.reduce((sum, item) => sum + item.quantity_change, 0)
      finish(guard, { objectNo: inboundNo.value, status: '可追溯', before: '-', after: '库存已入账', quantity: `${change > 0 ? '+' : ''}${change} 件`,
        route: `/inventory?order_no=${encodeURIComponent(inboundNo.value)}`, facts: [{ label: '库存流水', value: data.total + ' 条' }] })
    } else if (key === 'outbound-pick') {
      for (const op of writes.filter((w) => w.step === key && w.status === 'sending')) await write(op.name, () => op, guard)
      const detail = await read(() => getOutboundOrder(outboundID.value), guard)
      for (const task of detail.tasks.filter((t) => t.task_type === 'PICK' && t.status !== 'COMPLETED' && t.done_qty < t.target_qty)) {
        await write('pick:' + task.id, () => ({ kind: 'pick', objectId: String(task.id), payload: {
          qty: task.target_qty - task.done_qty, location_code: task.location_code, batch_no: task.batch_no,
        } }), guard)
      }
      const done = writes.filter((w) => w.step === key && w.status === 'done')
      const latest = await read(() => getOutboundOrder(outboundID.value), guard)
      if (!done.length || latest.order.status !== 'SHIPPED') throw new Error('拣货结果未全部确认，请恢复或核对出库单')
      const picked = done.reduce((sum, w) => sum + Number(w.payload.qty), 0)
      finish(guard, { objectNo: outboundNo.value, status: '已发货', before: '拣货中', after: '已发货', quantity: `-${picked} 件库存`,
        route: `/tasks?order_id=${outboundID.value}`, facts: [{ label: '出库单', value: outboundNo.value }, { label: '完成任务', value: done.length + ' 个' }] })
    }
  }
  async function executeNext(): Promise<void> {
    if (!started.value || completed.value || running.value || !currentStep.value || disposed) return
    const epoch = generation
    const guard = () => {
      if (disposed || epoch !== generation || !sameIdentity()) throw new Error('演示已暂停或会话已切换')
      if (Date.now() - Date.parse(runStartedAt.value) >= STAGED_RETENTION_MS) throw new Error('演示进度已超过安全恢复窗口，请核对业务记录或重置演示。')
    }
    running.value = true; paused.value = false; error.value = ''; controller = new AbortController(); notifyState()
    try { guard(); await executeStep(guard) } catch (e) {
      if (!disposed && epoch === generation && sameIdentity()) error.value = e instanceof Error ? e.message : '当前步骤结果待确认，请恢复核对'
    } finally {
      running.value = false
      if (!disposed) notifyState()
    }
  }
  async function runAll(): Promise<void> {
    const epoch = generation
    while (!disposed && epoch === generation && !paused.value && started.value && !completed.value && mode.value === 'auto') {
      await executeNext()
      if (error.value || completed.value || paused.value || epoch !== generation) return
      await new Promise<void>((resolve) => { wake = resolve; timer = setTimeout(() => { wake = undefined; timer = undefined; resolve() }, STEP_INTERVAL_MS) })
    }
  }
  function resume(): void {
    if (running.value || disposed || !started.value || completed.value) return
    paused.value = false; error.value = ''
    if (mode.value === 'auto') void runAll()
    else void executeNext()
  }
  async function validateCurrentPage(): Promise<boolean> {
    const epoch = generation
    const guard = () => { if (disposed || epoch !== generation || !sameIdentity()) throw new Error('演示会话已切换') }
    try {
      const key = lastResult.value?.key
      if (key?.startsWith('inbound') && inboundID.value) await read(() => getInboundOrder(inboundID.value), guard)
      if (key?.startsWith('outbound') && outboundID.value) await read(() => getOutboundOrder(outboundID.value), guard)
      return true
    } catch { if (!disposed && epoch === generation) error.value = '演示数据已重置或无法核对，请检查真实业务记录。'; return false }
  }
  const stopWatch = watch([() => auth.user?.user_id, () => auth.demoSessionId, () => auth.isDemo], restore, { flush: 'sync' })
  restore()
  if (getCurrentScope()) onScopeDispose(() => { pause(); disposed = true; stopWatch() })
  return { mode, scope, started, paused, currentIndex, results, running, error, runStartedAt, runCompletedAt,
    currentStep, completed, progress, lastResult, groupStates, start, executeNext, resume, pause, validateCurrentPage }
}
