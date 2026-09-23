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

export interface GuideBusinessResult {
  orderId?: string
  orderNo?: string
  taskId?: string
  taskNo?: string
  message?: string
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
  stocktakeOrderCreated: 'stocktake.order.created',
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
  ],
  stocktake: [
    {
      id: 'stocktake-create',
      route: '/stocktake/orders',
      target: '[data-tour="stocktake-create"]',
      title: '创建盘点单',
      description: '点击“新建盘点单”，选择仓库和盘点范围。创建成功后，系统会生成账面快照。',
      event: GUIDE_EVENTS.stocktakeOrderCreated,
    },
  ],
}

export const GUIDE_SCENARIO_LABELS: Record<GuideScenario, string> = {
  inbound: '入库',
  outbound: '出库',
  stocktake: '盘点',
}

function resolveGuideRoute(route: string, orderId: string, orderNo: string): string {
  return route
    .replace(':orderId', encodeURIComponent(orderId))
    .replace(':orderNo', encodeURIComponent(orderNo))
}

export const useGuideStore = defineStore('guide', {
  state: () => ({
    active: false,
    scenario: null as GuideScenario | null,
    currentStep: 0,
    orderId: '',
    orderNo: '',
    taskId: '',
    taskNo: '',
    startedAt: 0,
    completed: false,
    verifiedStepIds: [] as string[],
    lastOutcome: '',
    mismatch: '',
  }),
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
      this.lastOutcome = ''
      this.mismatch = ''
      return GUIDE_STEPS[scenario][0]
    },
    recordBusinessResult(event: string, result: GuideBusinessResult = {}): boolean {
      const step = this.currentStepDefinition
      if (!this.active || this.completed || !step) return false
      if (event !== step.event) {
        this.mismatch = `当前业务状态与引导不一致：当前步骤需要完成“${step.title}”。`
        return false
      }

      if (!this.verifiedStepIds.includes(step.id)) this.verifiedStepIds.push(step.id)
      if (result.orderId) this.orderId = result.orderId
      if (result.orderNo) this.orderNo = result.orderNo
      if (result.taskId) this.taskId = result.taskId
      if (result.taskNo) this.taskNo = result.taskNo
      this.lastOutcome = result.message || '当前步骤已在真实业务中完成。'
      this.mismatch = ''
      return true
    },
    next(): boolean {
      if (!this.canAdvance) return false
      if (this.currentStep >= this.totalSteps - 1) {
        this.completed = true
        this.active = false
        return true
      }
      this.currentStep += 1
      this.mismatch = ''
      this.lastOutcome = ''
      return true
    },
    previous(): boolean {
      if (!this.canGoPrevious) return false
      this.currentStep -= 1
      this.mismatch = ''
      this.lastOutcome = ''
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
      return true
    },
    setMismatch(message: string): void {
      if (!this.active || this.completed) return
      this.mismatch = message
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
      this.lastOutcome = ''
      this.mismatch = ''
    },
  },
})
