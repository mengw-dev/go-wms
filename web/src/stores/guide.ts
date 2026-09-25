import { defineStore } from 'pinia'

export type GuideScenario = 'inbound' | 'outbound' | 'stocktake'

export interface GuideStep {
  id: string
  route: string
  target: string
  title: string
  description: string
  event: string
}

export interface GuideFact {
  label: string
  value: string
}

export interface GuideBusinessResult {
  orderId?: string
  orderNo?: string
  taskId?: string
  taskNo?: string
  message?: string
  facts?: GuideFact[]
}

export const GUIDE_EVENTS = {
  inboundOrderCreated: 'inbound.order.created',
  inboundOrderSubmitted: 'inbound.order.submitted',
  inboundOrderApproved: 'inbound.order.approved',
  inboundReceived: 'inbound.received',
  inboundPutawayReady: 'inbound.putaway.ready',
  inboundPutawayCompleted: 'inbound.putaway.completed',
  inboundInventoryReviewed: 'inbound.inventory.reviewed',
  outboundOrderCreated: 'outbound.order.created',
  outboundOrderSubmitted: 'outbound.order.submitted',
  outboundOrderAllocated: 'outbound.order.allocated',
  outboundPickTasksReady: 'outbound.pick.tasks.ready',
  outboundPicked: 'outbound.picked',
  outboundShipped: 'outbound.shipped',
  outboundInventoryReviewed: 'outbound.inventory.reviewed',
  stocktakeOrderCreated: 'stocktake.order.created',
  stocktakeSnapshotReady: 'stocktake.snapshot.ready',
  stocktakeActualCompleted: 'stocktake.actual.completed',
  stocktakeDifferenceReviewed: 'stocktake.difference.reviewed',
  stocktakeApproved: 'stocktake.approved',
  stocktakeInventoryReviewed: 'stocktake.inventory.reviewed',
} as const

const GUIDE_STEPS: Record<GuideScenario, readonly GuideStep[]> = {
  inbound: [
    {
      id: 'inbound-create',
      route: '/inbound/orders',
      target: '[data-tour="inbound-create"]',
      title: '创建入库单',
      description: '点击“新建入库单”，填写仓库、货品和数量。保存成功后，引导才会解锁下一步。',
      event: GUIDE_EVENTS.inboundOrderCreated,
    },
    {
      id: 'inbound-submit',
      route: '/inbound/orders/:orderId',
      target: '[data-tour="inbound-submit"]',
      title: '提交入库单',
      description: '确认刚才创建的入库单，点击“提交”。状态会从草稿变为已提交。',
      event: GUIDE_EVENTS.inboundOrderSubmitted,
    },
    {
      id: 'inbound-approve',
      route: '/inbound/orders/:orderId',
      target: '[data-tour="inbound-approve"]',
      title: '审核入库单',
      description: '审核通过后，入库单才能进入收货环节。',
      event: GUIDE_EVENTS.inboundOrderApproved,
    },
    {
      id: 'inbound-receive',
      route: '/inbound/orders/:orderId',
      target: '[data-tour="inbound-receive"]',
      title: '完成收货',
      description: '按实收数量登记批次。整单收齐后，系统会生成上架任务。',
      event: GUIDE_EVENTS.inboundReceived,
    },
    {
      id: 'inbound-tasks',
      route: '/inbound/orders/:orderId',
      target: '[data-tour="inbound-tasks"]',
      title: '查看上架任务',
      description: '在关联任务中查看系统生成的上架任务和待上架数量。',
      event: GUIDE_EVENTS.inboundPutawayReady,
    },
    {
      id: 'inbound-putaway',
      route: '/inbound/orders/:orderId',
      target: '[data-tour="inbound-putaway"]',
      title: '完成上架',
      description: '选择库位并完成上架，库存会在真实业务事务中增加。',
      event: GUIDE_EVENTS.inboundPutawayCompleted,
    },
    {
      id: 'inbound-inventory',
      route: '/inventory?order_no=:orderNo',
      target: '[data-tour="inventory-evidence"]',
      title: '查看库存与流水',
      description: '库存页会按本次入库单筛选真实流水，确认数量变化和来源单据。',
      event: GUIDE_EVENTS.inboundInventoryReviewed,
    },
  ],
  outbound: [
    {
      id: 'outbound-create',
      route: '/outbound/orders',
      target: '[data-tour="outbound-create"]',
      title: '创建出库单',
      description: '点击“新建出库单”，填写业务单号和出库明细。创建成功后，引导会记录真实订单 ID。',
      event: GUIDE_EVENTS.outboundOrderCreated,
    },
    {
      id: 'outbound-submit',
      route: '/outbound/orders/:orderId',
      target: '[data-tour="outbound-submit"]',
      title: '提交出库单',
      description: '确认刚创建的出库单，点击“提交”。状态会从草稿变为已提交。',
      event: GUIDE_EVENTS.outboundOrderSubmitted,
    },
    {
      id: 'outbound-allocate',
      route: '/outbound/orders/:orderId',
      target: '[data-tour="outbound-allocate"]',
      title: '审核并执行 FIFO 分配',
      description: '点击“审核（分配）”。系统会按真实库存完成 FIFO 分配，并在当前页面展示分配结果。',
      event: GUIDE_EVENTS.outboundOrderAllocated,
    },
    {
      id: 'outbound-tasks',
      route: '/outbound/orders/:orderId',
      target: '[data-tour="outbound-tasks"]',
      title: '查看拣货任务',
      description: '在“分配明细”和“关联任务”中核对真实分配结果与系统生成的 PICK 任务。',
      event: GUIDE_EVENTS.outboundPickTasksReady,
    },
    {
      id: 'outbound-pick',
      route: '/outbound/orders/:orderId',
      target: '[data-tour="outbound-pick"]',
      title: '完成拣货',
      description: '按任务要求完成拣货。分配数量全部拣满后，系统会在真实业务事务中完成发货扣减。',
      event: GUIDE_EVENTS.outboundPicked,
    },
    {
      id: 'outbound-shipped',
      route: '/outbound/orders/:orderId',
      target: '[data-tour="outbound-shipped"]',
      title: '确认发货完成',
      description: '订单状态变为“已发货”，表示所有分配行均已拣满并完成库存扣减。',
      event: GUIDE_EVENTS.outboundShipped,
    },
    {
      id: 'outbound-inventory',
      route: '/inventory?order_no=:orderNo',
      target: '[data-tour="inventory-evidence"]',
      title: '查看库存流水',
      description: '按本次出库单筛选库存流水，核对 ALLOCATE 分配和 SHIP 发货的真实数量变化。',
      event: GUIDE_EVENTS.outboundInventoryReviewed,
    },
  ],
  stocktake: [
    {
      id: 'stocktake-create',
      route: '/stocktake/orders',
      target: '[data-tour="stocktake-create"]',
      title: '创建盘点单',
      description: '点击“新建盘点单”，选择仓库和盘点范围。创建成功后，系统会生成真实账面快照。',
      event: GUIDE_EVENTS.stocktakeOrderCreated,
    },
    {
      id: 'stocktake-snapshot',
      route: '/stocktake/orders/:orderId',
      target: '[data-tour="stocktake-snapshot"]',
      title: '核对账面快照',
      description: '查看系统按盘点范围生成的账面库存。快照数据来自创建盘点单时的真实库存。',
      event: GUIDE_EVENTS.stocktakeSnapshotReady,
    },
    {
      id: 'stocktake-actual',
      route: '/stocktake/orders/:orderId',
      target: '[data-tour="stocktake-actual"]',
      title: '录入实盘数量',
      description: '逐行填写实盘数量并点击“保存”。全部明细保存后，才会进入差异确认步骤。',
      event: GUIDE_EVENTS.stocktakeActualCompleted,
    },
    {
      id: 'stocktake-difference',
      route: '/stocktake/orders/:orderId',
      target: '[data-tour="stocktake-difference"]',
      title: '查看账实差异',
      description: '核对账面、实盘和差异。这里展示的是当前真实盘点明细计算出的汇总。',
      event: GUIDE_EVENTS.stocktakeDifferenceReviewed,
    },
    {
      id: 'stocktake-approve',
      route: '/stocktake/orders/:orderId',
      target: '[data-tour="stocktake-approve"]',
      title: '审核并调整库存',
      description: '点击“审核”。系统会在真实业务事务中锁定库存、写入盘点调整流水并完成盘点单。',
      event: GUIDE_EVENTS.stocktakeApproved,
    },
    {
      id: 'stocktake-inventory',
      route: '/inventory?order_no=:orderNo',
      target: '[data-tour="inventory-evidence"]',
      title: '查看库存调整流水',
      description: '按本次盘点单筛选库存流水，确认 ADJUST 调整记录和实际数量变化。',
      event: GUIDE_EVENTS.stocktakeInventoryReviewed,
    },
  ],
}

export function getGuideStep(scenario: GuideScenario, index: number): GuideStep | null {
  return GUIDE_STEPS[scenario][index] ?? null
}

export const GUIDE_SCENARIO_LABELS: Record<GuideScenario, string> = {
  inbound: '入库',
  outbound: '出库',
  stocktake: '盘点',
}

export function resolveGuideRoute(route: string, orderId: string, orderNo: string): string {
  return route
    .replace(':orderId', encodeURIComponent(orderId))
    .replace(':orderNo', encodeURIComponent(orderNo))
}

const GUIDE_STORAGE_KEY = 'wms-manual-guide-v1'

interface PersistedGuideState {
  active: boolean
  scenario: GuideScenario | null
  currentStep: number
  orderId: string
  orderNo: string
  taskId: string
  taskNo: string
  startedAt: number
  completed: boolean
  verifiedStepIds: string[]
  facts: GuideFact[]
  lastOutcome: string
  mismatch: string
}

function loadPersistedGuide(): PersistedGuideState | null {
  if (typeof window === 'undefined') return null
  const raw = window.sessionStorage.getItem(GUIDE_STORAGE_KEY)
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as Partial<PersistedGuideState>
    if (value.scenario !== 'inbound' && value.scenario !== 'outbound' && value.scenario !== 'stocktake') return null
    if (typeof value.active !== 'boolean' || typeof value.currentStep !== 'number') return null
    return {
      active: value.active,
      scenario: value.scenario,
      currentStep: Math.max(0, value.currentStep),
      orderId: value.orderId || '',
      orderNo: value.orderNo || '',
      taskId: value.taskId || '',
      taskNo: value.taskNo || '',
      startedAt: typeof value.startedAt === 'number' ? value.startedAt : Date.now(),
      completed: Boolean(value.completed),
      verifiedStepIds: Array.isArray(value.verifiedStepIds) ? value.verifiedStepIds.filter((item): item is string => typeof item === 'string') : [],
      facts: Array.isArray(value.facts) ? value.facts.filter((item): item is GuideFact => Boolean(item && typeof item.label === 'string' && typeof item.value === 'string')) : [],
      lastOutcome: value.lastOutcome || '',
      mismatch: value.mismatch || '',
    }
  } catch {
    return null
  }
}

export const useGuideStore = defineStore('guide', {
  state: () => {
    const persisted = loadPersistedGuide()
    return {
      active: persisted?.active ?? false,
      scenario: persisted?.scenario ?? null,
      currentStep: persisted?.currentStep ?? 0,
      orderId: persisted?.orderId ?? '',
      orderNo: persisted?.orderNo ?? '',
      taskId: persisted?.taskId ?? '',
      taskNo: persisted?.taskNo ?? '',
      startedAt: persisted?.startedAt ?? 0,
      completed: persisted?.completed ?? false,
      verifiedStepIds: persisted?.verifiedStepIds ?? [],
      facts: persisted?.facts ?? [],
      lastOutcome: persisted?.lastOutcome ?? '',
      mismatch: persisted?.mismatch ?? '',
    }
  },
  getters: {
    steps(state): readonly GuideStep[] {
      return state.scenario ? GUIDE_STEPS[state.scenario] : []
    },
    currentStepDefinition(state): GuideStep | null {
      return state.scenario ? GUIDE_STEPS[state.scenario][state.currentStep] ?? null : null
    },
    currentStepRoute(state): string {
      const step = state.scenario ? GUIDE_STEPS[state.scenario][state.currentStep] : null
      if (!step) return ''
      return resolveGuideRoute(step.route, state.orderId, state.orderNo)
    },
    currentStepNumber(state): number {
      return state.scenario ? state.currentStep + 1 : 0
    },
    totalSteps(state): number {
      return state.scenario ? GUIDE_STEPS[state.scenario].length : 0
    },
    canGoPrevious(state): boolean {
      return state.active && !state.completed && state.currentStep > 0
    },
    canAdvance(state): boolean {
      const step = state.scenario ? GUIDE_STEPS[state.scenario][state.currentStep] : null
      return Boolean(state.active && !state.completed && step && state.verifiedStepIds.includes(step.id) && !state.mismatch)
    },
  },
  actions: {
    persist(): void {
      if (typeof window === 'undefined') return
      const snapshot: PersistedGuideState = {
        active: this.active,
        scenario: this.scenario,
        currentStep: this.currentStep,
        orderId: this.orderId,
        orderNo: this.orderNo,
        taskId: this.taskId,
        taskNo: this.taskNo,
        startedAt: this.startedAt,
        completed: this.completed,
        verifiedStepIds: [...this.verifiedStepIds],
        facts: this.facts.map((fact) => ({ ...fact })),
        lastOutcome: this.lastOutcome,
        mismatch: this.mismatch,
      }
      window.sessionStorage.setItem(GUIDE_STORAGE_KEY, JSON.stringify(snapshot))
    },
    start(scenario: GuideScenario): GuideStep {
      this.active = true
      this.scenario = scenario
      this.currentStep = 0
      this.orderId = ''
      this.orderNo = ''
      this.taskId = ''
      this.taskNo = ''
      this.startedAt = Date.now()
      this.completed = false
      this.verifiedStepIds = []
      this.facts = []
      this.lastOutcome = ''
      this.mismatch = ''
      this.persist()
      return GUIDE_STEPS[scenario][0]
    },
    recordBusinessResult(event: string, result: GuideBusinessResult = {}): boolean {
      const currentStep = this.currentStepDefinition
      if (!this.active || this.completed || !currentStep || !this.scenario) return false
      const targetIndex = GUIDE_STEPS[this.scenario].findIndex((item) => item.event === event)
      if (targetIndex < 0 || targetIndex < this.currentStep) {
        this.mismatch = `当前业务状态与引导不一致：当前步骤需要完成“${currentStep.title}”。`
        this.persist()
        return false
      }
      const relocated = targetIndex > this.currentStep
      if (relocated) {
        for (let index = 0; index < targetIndex; index += 1) {
          const previousId = GUIDE_STEPS[this.scenario][index].id
          if (!this.verifiedStepIds.includes(previousId)) this.verifiedStepIds.push(previousId)
        }
        this.currentStep = targetIndex
      }
      const step = GUIDE_STEPS[this.scenario][targetIndex]
      if (!this.verifiedStepIds.includes(step.id)) this.verifiedStepIds.push(step.id)
      if (result.orderId) this.orderId = result.orderId
      if (result.orderNo) this.orderNo = result.orderNo
      if (result.taskId) this.taskId = result.taskId
      if (result.taskNo) this.taskNo = result.taskNo
      if (result.facts?.length) {
        const merged = new Map(this.facts.map((fact) => [fact.label, fact]))
        for (const fact of result.facts) {
          if (fact.label) merged.set(fact.label, fact)
        }
        this.facts = Array.from(merged.values())
      }
      const message = result.message || '当前步骤已在真实业务中完成。'
      this.lastOutcome = relocated
        ? `检测到当前业务已经进入下一阶段，已为你定位到对应步骤。${message}`
        : message
      this.mismatch = ''
      this.persist()
      return true
    },
    next(): boolean {
      if (!this.canAdvance) return false
      if (this.currentStep >= this.totalSteps - 1) {
        this.completed = true
        this.active = false
        this.persist()
        return true
      }
      this.currentStep += 1
      this.mismatch = ''
      this.lastOutcome = ''
      this.persist()
      return true
    },
    previous(): boolean {
      if (!this.canGoPrevious) return false
      this.currentStep -= 1
      this.mismatch = ''
      this.lastOutcome = ''
      this.persist()
      return true
    },
    reposition(routePath: string): boolean {
      if (!this.active || !this.scenario) return false
      const matches = GUIDE_STEPS[this.scenario]
        .map((step, index) => ({ step, index }))
        .filter(({ step }) => resolveGuideRoute(step.route, this.orderId, this.orderNo).split('?')[0] === routePath)
      const target = matches.find(({ step }) => !this.verifiedStepIds.includes(step.id)) ?? matches.at(-1)
      if (!target) {
        this.mismatch = '当前页面不在本次引导流程中，无法重新定位。'
        return false
      }
      this.currentStep = target.index
      this.mismatch = ''
      this.lastOutcome = ''
      this.persist()
      return true
    },
    setMismatch(message: string): void {
      if (!this.active || this.completed) return
      this.mismatch = message
      this.persist()
    },
    restart(): GuideStep | null {
      if (!this.scenario) return null
      return this.start(this.scenario)
    },
    cancel(): void {
      this.active = false
      this.completed = false
      this.scenario = null
      this.currentStep = 0
      this.orderId = ''
      this.orderNo = ''
      this.taskId = ''
      this.taskNo = ''
      this.startedAt = 0
      this.verifiedStepIds = []
      this.facts = []
      this.lastOutcome = ''
      this.mismatch = ''
      if (typeof window !== 'undefined') window.sessionStorage.removeItem(GUIDE_STORAGE_KEY)
    },
  },
})
