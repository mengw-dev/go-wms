import { watch, type WatchStopHandle } from 'vue'
import {
  BUSINESS_EVENTS,
  onBusinessEvent,
  type BusinessEventPayload,
  type BusinessStocktakeDetail,
  type BusinessTaskRef,
} from '@/events/businessEvents'
import {
  GUIDE_EVENTS,
  useGuideStore,
  type GuideBusinessResult,
  type GuideFact,
} from '@/stores/guide'

/**
 * Guide 业务桥。
 *
 * 这是把“业务事件”翻译成“引导步骤完成”的唯一位置：
 * 业务页面只发送中性业务事件，本文件负责匹配当前引导场景、判断当前步骤是否完成、
 * 保存业务证据，并维护引导与真实业务状态不一致时的提示。
 */

const INBOUND_SUBMITTED_STATUS = ['SUBMITTED', 'APPROVED', 'RECEIVING', 'PUTAWAY', 'COMPLETED']
const INBOUND_APPROVED_STATUS = ['APPROVED', 'RECEIVING', 'PUTAWAY', 'COMPLETED']
const INBOUND_RECEIVED_STATUS = ['PUTAWAY', 'COMPLETED']

const OUTBOUND_SUBMITTED_STATUS = ['SUBMITTED', 'APPROVED', 'PICKING', 'SHIPPED']
const OUTBOUND_ALLOCATED_STATUS = ['PICKING', 'SHIPPED']

interface InboundOrderSnapshot {
  orderId: string
  orderNo: string
  status: string
  putawayTask: BusinessTaskRef | null
}

interface OutboundOrderSnapshot {
  orderId: string
  orderNo: string
  status: string
  allocationCount: number
  allocatedQty: number
  pickTaskCount: number
  pickTask: BusinessTaskRef | null
}

interface StocktakeOrderSnapshot {
  orderId: string
  orderNo: string
  status: string
  details: BusinessStocktakeDetail[]
}

interface InventorySnapshot {
  orderNo: string
  transTypes: string[]
  transTotal: number
}

let inboundSnapshot: InboundOrderSnapshot | null = null
let outboundSnapshot: OutboundOrderSnapshot | null = null
let stocktakeSnapshot: StocktakeOrderSnapshot | null = null
let inventorySnapshot: InventorySnapshot | null = null

/** 只有当前引导步骤正好等待该业务事件时才记录，避免跳过或回退已完成的步骤。 */
function recordIfCurrentStep(event: string, result: GuideBusinessResult, orderId?: string): void {
  const guide = useGuideStore()
  if (guide.currentStepDefinition?.event !== event) return
  if (orderId && guide.orderId && guide.orderId !== orderId) return
  guide.recordBusinessResult(event, result)
}

function signed(value: number): string {
  return `${value > 0 ? '+' : ''}${value}`
}

function summarizeStocktakeDetails(
  details: BusinessStocktakeDetail[],
  persistedDiff: boolean,
): { book: number; actual: number; diff: number } {
  return details.reduce(
    (sum, detail) => {
      sum.book += detail.bookQty
      sum.actual += detail.actualQty ?? 0
      sum.diff += persistedDiff ? detail.diffQty : (detail.actualQty ?? 0) - detail.bookQty
      return sum
    },
    { book: 0, actual: 0, diff: 0 },
  )
}

function stocktakeFacts(
  details: BusinessStocktakeDetail[],
  status: string,
  persistedDiff: boolean,
): GuideFact[] {
  // 草稿阶段只汇总已录入实盘的行，避免把“未盘”当成 0 计入差异。
  const scope = status === 'DRAFT' ? details.filter((detail) => detail.actualQty !== null) : details
  if (!scope.length) return []
  const summary = summarizeStocktakeDetails(scope, persistedDiff)
  return [
    { label: '账面库存', value: String(summary.book) },
    { label: '实盘库存', value: String(summary.actual) },
    { label: '差异', value: signed(summary.diff) },
    { label: '调整数量', value: signed(summary.diff) },
    { label: '盘点明细', value: `${scope.length} 行` },
  ]
}

/** 入库单：按页面加载到的业务状态核对引导步骤。 */
function reconcileInbound(): void {
  const guide = useGuideStore()
  const snapshot = inboundSnapshot
  const step = guide.currentStepDefinition
  if (!guide.active || guide.scenario !== 'inbound' || !step || !snapshot) return
  if (!snapshot.status || guide.orderId !== snapshot.orderId) return

  const base = { orderId: snapshot.orderId, orderNo: snapshot.orderNo }
  const task = snapshot.putawayTask
  const status = snapshot.status

  if (step.id === 'inbound-submit') {
    if (status === 'DRAFT') {
      guide.setMismatch('')
      return
    }
    if (INBOUND_SUBMITTED_STATUS.includes(status)) {
      recordIfCurrentStep(GUIDE_EVENTS.inboundOrderSubmitted, {
        ...base,
        message: `提交完成：${snapshot.orderNo} 已从草稿变为已提交。`,
      })
      return
    }
  }

  if (step.id === 'inbound-approve') {
    if (status === 'SUBMITTED') {
      guide.setMismatch('')
      return
    }
    if (INBOUND_APPROVED_STATUS.includes(status)) {
      recordIfCurrentStep(GUIDE_EVENTS.inboundOrderApproved, {
        ...base,
        message: `审核完成：${snapshot.orderNo} 已从已提交变为已审核。下一步：收货。`,
      })
      return
    }
  }

  if (step.id === 'inbound-receive') {
    if (status === 'APPROVED' || status === 'RECEIVING') {
      guide.setMismatch('')
      return
    }
    if (INBOUND_RECEIVED_STATUS.includes(status)) {
      recordIfCurrentStep(GUIDE_EVENTS.inboundReceived, {
        ...base,
        taskId: task ? task.taskId : guide.taskId,
        taskNo: task?.taskNo || guide.taskNo,
        message: task
          ? `收货完成：${snapshot.orderNo} 已生成上架任务 ${task.taskNo}。下一步：查看并完成上架。`
          : `收货完成：${snapshot.orderNo} 已收齐。下一步：查看上架任务。`,
      })
      return
    }
  }

  if (step.id === 'inbound-tasks') {
    if (task) {
      recordIfCurrentStep(GUIDE_EVENTS.inboundPutawayReady, {
        taskId: task.taskId,
        taskNo: task.taskNo,
        message: `上架任务已生成：${task.taskNo}，待上架 ${Math.max(task.targetQty - task.doneQty, 0)}。`,
      })
      return
    }
    if (status === 'COMPLETED') {
      guide.setMismatch('当前入库单已完成上架，没有可查看的待上架任务。请重新开始本次引导。')
      return
    }
  }

  if (step.id === 'inbound-putaway') {
    if (task) {
      guide.setMismatch('')
      return
    }
    if (status === 'COMPLETED') {
      recordIfCurrentStep(GUIDE_EVENTS.inboundPutawayCompleted, {
        ...base,
        message: `上架完成：${snapshot.orderNo} 的库存已在真实业务事务中增加。`,
      })
      return
    }
  }

  if (status === 'CANCELED') {
    guide.setMismatch(`入库单 ${snapshot.orderNo} 已取消，当前步骤无法继续。请重新开始本次引导。`)
    return
  }

  if (step.id !== 'inbound-create') {
    guide.setMismatch(`当前业务状态 ${status} 与引导步骤“${step.title}”不一致，请重新定位当前步骤。`)
  }
}

/** 出库单：按页面加载到的业务状态核对引导步骤。 */
function reconcileOutbound(): void {
  const guide = useGuideStore()
  const snapshot = outboundSnapshot
  const step = guide.currentStepDefinition
  if (!guide.active || guide.scenario !== 'outbound' || !step || !snapshot) return
  if (!snapshot.status || guide.orderId !== snapshot.orderId) return

  const base = { orderId: snapshot.orderId, orderNo: snapshot.orderNo }
  const status = snapshot.status
  const pendingTask = snapshot.pickTask
  const allocationCompleted = OUTBOUND_ALLOCATED_STATUS.includes(status) && snapshot.allocationCount > 0

  if (step.id === 'outbound-submit') {
    if (status === 'DRAFT') {
      guide.setMismatch('')
      return
    }
    if (OUTBOUND_SUBMITTED_STATUS.includes(status)) {
      recordIfCurrentStep(GUIDE_EVENTS.outboundOrderSubmitted, {
        ...base,
        message: `提交完成：${snapshot.orderNo} 已从草稿变为已提交。`,
      })
      return
    }
  }

  if (step.id === 'outbound-allocate') {
    if (status === 'SUBMITTED') {
      guide.setMismatch('')
      return
    }
    if (allocationCompleted) {
      recordIfCurrentStep(GUIDE_EVENTS.outboundOrderAllocated, {
        ...base,
        taskId: pendingTask ? pendingTask.taskId : guide.taskId,
        taskNo: pendingTask?.taskNo || guide.taskNo,
        message: `系统刚刚完成库存分配：共 ${snapshot.allocationCount} 条分配记录、${snapshot.allocatedQty} 件，已生成 ${snapshot.pickTaskCount} 个拣货任务。下一步：查看拣货任务。`,
      })
      return
    }
  }

  if (step.id === 'outbound-tasks') {
    if (pendingTask) {
      recordIfCurrentStep(GUIDE_EVENTS.outboundPickTasksReady, {
        taskId: pendingTask.taskId,
        taskNo: pendingTask.taskNo,
        message: `已查看真实分配结果和拣货任务 ${pendingTask.taskNo}，待拣 ${Math.max(pendingTask.targetQty - pendingTask.doneQty, 0)} 件。`,
      })
      return
    }
    if (status === 'SHIPPED') {
      guide.setMismatch('当前出库单已完成拣货发货，没有可查看的待执行拣货任务。请重新开始本次引导。')
      return
    }
  }

  if (step.id === 'outbound-pick') {
    if (status === 'PICKING') {
      guide.setMismatch(pendingTask ? '' : '当前没有待执行的拣货任务，请重新定位当前步骤。')
      return
    }
    if (status === 'SHIPPED') {
      recordIfCurrentStep(GUIDE_EVENTS.outboundPicked, {
        ...base,
        message: `拣货完成：${snapshot.orderNo} 的所有分配行均已拣满，系统已在真实业务事务中完成发货扣减。`,
      })
      return
    }
  }

  if (step.id === 'outbound-shipped') {
    if (status === 'PICKING') {
      guide.setMismatch('')
      return
    }
    if (status === 'SHIPPED') {
      recordIfCurrentStep(GUIDE_EVENTS.outboundShipped, {
        ...base,
        message: `发货完成：${snapshot.orderNo} 已变为已发货。下一步：查看库存流水。`,
      })
      return
    }
  }

  if (status === 'CANCELED') {
    guide.setMismatch(`出库单 ${snapshot.orderNo} 已取消，当前步骤无法继续。请重新开始本次引导。`)
    return
  }

  if (step.id !== 'outbound-create') {
    guide.setMismatch(`当前业务状态 ${status} 与引导步骤“${step.title}”不一致，请重新定位当前步骤。`)
  }
}

/** 盘点单：按页面加载到的业务状态核对引导步骤。 */
function reconcileStocktake(): void {
  const guide = useGuideStore()
  const snapshot = stocktakeSnapshot
  const step = guide.currentStepDefinition
  if (!guide.active || guide.scenario !== 'stocktake' || !step || !snapshot) return
  if (!snapshot.status || guide.orderId !== snapshot.orderId) return

  const base = { orderId: snapshot.orderId, orderNo: snapshot.orderNo }
  const details = snapshot.details
  const counted = details.filter((detail) => detail.actualQty !== null)
  const allActualEntered = details.length > 0 && counted.length === details.length
  const adjustmentCompleted =
    snapshot.status === 'COMPLETED' && details.length > 0 && details.every((detail) => detail.adjusted)

  if (snapshot.status === 'CANCELLED') {
    guide.setMismatch(`盘点单 ${snapshot.orderNo} 已取消，当前步骤无法继续。请重新开始本次引导。`)
    return
  }

  if (step.id === 'stocktake-snapshot') {
    if (!details.length) {
      guide.setMismatch('当前盘点单没有账面快照，无法继续。')
      return
    }
    recordIfCurrentStep(GUIDE_EVENTS.stocktakeSnapshotReady, {
      ...base,
      facts: stocktakeFacts(details, snapshot.status, false),
      message: `${snapshot.orderNo} 已生成账面快照，共 ${details.length} 行。`,
    })
    return
  }

  if (step.id === 'stocktake-actual') {
    if (adjustmentCompleted) {
      recordIfCurrentStep(GUIDE_EVENTS.stocktakeActualCompleted, {
        ...base,
        facts: stocktakeFacts(details, snapshot.status, true),
        message: '全部明细已完成实盘录入。',
      })
      return
    }
    if (allActualEntered) {
      recordIfCurrentStep(GUIDE_EVENTS.stocktakeActualCompleted, {
        ...base,
        facts: stocktakeFacts(details, snapshot.status, false),
        message: `全部 ${details.length} 行实盘数量已保存，可以查看差异。`,
      })
      return
    }
    guide.setMismatch('')
    return
  }

  if (step.id === 'stocktake-difference') {
    if (!allActualEntered) {
      guide.setMismatch('请先保存全部明细的实盘数量，再查看账实差异。')
      return
    }
    recordIfCurrentStep(GUIDE_EVENTS.stocktakeDifferenceReviewed, {
      ...base,
      facts: stocktakeFacts(details, snapshot.status, adjustmentCompleted),
      message: '已核对账面、实盘与差异汇总。',
    })
    return
  }

  if (step.id === 'stocktake-approve') {
    if (adjustmentCompleted) {
      recordIfCurrentStep(GUIDE_EVENTS.stocktakeApproved, {
        ...base,
        facts: stocktakeFacts(details, snapshot.status, true),
        message: `审核完成：${snapshot.orderNo} 已按实际盘点结果调整库存。`,
      })
      return
    }
    if (!allActualEntered) {
      guide.setMismatch('存在尚未录入实盘数量的明细，不能审核。')
      return
    }
    guide.setMismatch('')
    return
  }

  if (step.id === 'stocktake-inventory' && adjustmentCompleted) {
    guide.setMismatch('盘点已审核完成，请前往库存流水核对 ADJUST 调整记录。')
  }
}

/** 库存流水：核对引导中的单据号与真实流水类型。 */
function reconcileInventory(): void {
  const guide = useGuideStore()
  const snapshot = inventorySnapshot
  const step = guide.currentStepDefinition
  if (!guide.active || !step || !snapshot) return

  const scenarioLabel =
    guide.scenario === 'inbound' && step.event === GUIDE_EVENTS.inboundInventoryReviewed
      ? '入库单'
      : guide.scenario === 'outbound' && step.event === GUIDE_EVENTS.outboundInventoryReviewed
        ? '出库单'
        : guide.scenario === 'stocktake' && step.event === GUIDE_EVENTS.stocktakeInventoryReviewed
          ? '盘点单'
          : ''
  if (!scenarioLabel) return

  if (!guide.orderNo || guide.orderNo !== snapshot.orderNo) {
    guide.setMismatch(`当前库存流水与引导中的${scenarioLabel}不一致，请重新定位当前步骤。`)
    return
  }

  if (scenarioLabel === '入库单') {
    if (snapshot.transTypes.length === 0) {
      guide.setMismatch(`暂未找到入库单 ${guide.orderNo} 的库存流水，请确认上架是否完成。`)
      return
    }
    recordIfCurrentStep(GUIDE_EVENTS.inboundInventoryReviewed, {
      message: `已查看入库单 ${guide.orderNo} 的 ${snapshot.transTotal} 条库存流水。`,
    })
    return
  }

  if (scenarioLabel === '出库单') {
    if (!snapshot.transTypes.includes('SHIP')) {
      guide.setMismatch(`暂未找到出库单 ${guide.orderNo} 的发货扣减流水，请确认拣货发货是否完成。`)
      return
    }
    recordIfCurrentStep(GUIDE_EVENTS.outboundInventoryReviewed, {
      message: `已查看出库单 ${guide.orderNo} 的 ${snapshot.transTotal} 条库存流水，其中包含 SHIP 发货扣减。`,
    })
    return
  }

  if (!snapshot.transTypes.includes('ADJUST')) {
    guide.setMismatch(`暂未找到盘点单 ${guide.orderNo} 的 ADJUST 调整流水，请确认审核是否完成。`)
    return
  }
  recordIfCurrentStep(GUIDE_EVENTS.stocktakeInventoryReviewed, {
    message: `已查看盘点单 ${guide.orderNo} 的库存调整流水。`,
  })
}

function storeInboundSnapshot(payload: BusinessEventPayload): void {
  if (!payload.orderId) return
  inboundSnapshot = {
    orderId: payload.orderId,
    orderNo: payload.orderNo || '',
    status: payload.status || '',
    putawayTask: payload.putawayTask ?? null,
  }
  reconcileInbound()
}

function storeOutboundSnapshot(payload: BusinessEventPayload): void {
  if (!payload.orderId) return
  outboundSnapshot = {
    orderId: payload.orderId,
    orderNo: payload.orderNo || '',
    status: payload.status || '',
    allocationCount: payload.allocationCount ?? 0,
    allocatedQty: payload.allocatedQty ?? 0,
    pickTaskCount: payload.pickTaskCount ?? 0,
    pickTask: payload.pickTask ?? null,
  }
  reconcileOutbound()
}

function storeStocktakeSnapshot(payload: BusinessEventPayload): void {
  if (!payload.orderId) return
  stocktakeSnapshot = {
    orderId: payload.orderId,
    orderNo: payload.orderNo || '',
    status: payload.status || '',
    details: payload.details ?? [],
  }
  reconcileStocktake()
}

function storeInventorySnapshot(payload: BusinessEventPayload): void {
  if (!payload.orderNo) return
  inventorySnapshot = {
    orderNo: payload.orderNo,
    transTypes: payload.transTypes ?? [],
    transTotal: payload.transTotal ?? 0,
  }
  reconcileInventory()
}

let stopWatch: WatchStopHandle | null = null

/**
 * 安装 Guide 业务桥，应在应用装配阶段调用一次。
 * 返回卸载函数，主要供单元测试使用。
 */
export function installGuideBusinessBridge(): () => void {
  stopWatch?.()
  const guide = useGuideStore()

  const unsubscribes = [
    onBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_CREATED, (payload) => {
      recordIfCurrentStep(GUIDE_EVENTS.inboundOrderCreated, {
        orderId: payload.orderId,
        orderNo: payload.orderNo,
        message: `已创建入库单 ${payload.orderNo}，当前状态为草稿。`,
      }, payload.orderId)
    }),
    onBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_SUBMITTED, (payload) => {
      recordIfCurrentStep(GUIDE_EVENTS.inboundOrderSubmitted, {
        orderId: payload.orderId,
        orderNo: payload.orderNo,
        message: `提交完成：${payload.orderNo} 已从草稿变为已提交。`,
      }, payload.orderId)
    }),
    onBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_APPROVED, (payload) => {
      recordIfCurrentStep(GUIDE_EVENTS.inboundOrderApproved, {
        orderId: payload.orderId,
        orderNo: payload.orderNo,
        message: `审核完成：${payload.orderNo} 已从已提交变为已审核，下一步可以收货。`,
      }, payload.orderId)
    }),
    onBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_LOADED, storeInboundSnapshot),

    onBusinessEvent(BUSINESS_EVENTS.OUTBOUND_ORDER_CREATED, (payload) => {
      recordIfCurrentStep(GUIDE_EVENTS.outboundOrderCreated, {
        orderId: payload.orderId,
        orderNo: payload.orderNo,
        message: `已创建出库单 ${payload.orderNo}，当前状态为草稿。`,
      }, payload.orderId)
    }),
    onBusinessEvent(BUSINESS_EVENTS.OUTBOUND_ORDER_SUBMITTED, (payload) => {
      recordIfCurrentStep(GUIDE_EVENTS.outboundOrderSubmitted, {
        orderId: payload.orderId,
        orderNo: payload.orderNo,
        message: `提交完成：${payload.orderNo} 已从草稿变为已提交。`,
      }, payload.orderId)
    }),
    onBusinessEvent(BUSINESS_EVENTS.OUTBOUND_ORDER_ALLOCATED, (payload) => {
      recordIfCurrentStep(GUIDE_EVENTS.outboundOrderAllocated, {
        orderId: payload.orderId,
        orderNo: payload.orderNo,
        message: `系统刚刚完成库存分配：${payload.orderNo} 已按真实库存生成分配结果。下一步：查看拣货任务。`,
      }, payload.orderId)
    }),
    onBusinessEvent(BUSINESS_EVENTS.OUTBOUND_ORDER_LOADED, storeOutboundSnapshot),

    onBusinessEvent(BUSINESS_EVENTS.STOCKTAKE_ORDER_CREATED, (payload) => {
      recordIfCurrentStep(GUIDE_EVENTS.stocktakeOrderCreated, {
        orderId: payload.orderId,
        orderNo: payload.orderNo,
        message: `已创建盘点单 ${payload.orderNo}，系统已生成真实账面快照。`,
      }, payload.orderId)
    }),
    onBusinessEvent(BUSINESS_EVENTS.STOCKTAKE_ORDER_LOADED, storeStocktakeSnapshot),

    onBusinessEvent(BUSINESS_EVENTS.INVENTORY_TRANS_LOADED, storeInventorySnapshot),
  ]

  // 引导推进或切换步骤后，用最近一次业务快照重新核对，避免重复请求业务接口。
  stopWatch = watch(
    () => [guide.active, guide.scenario, guide.currentStep, guide.completed, guide.orderId, guide.orderNo] as const,
    () => {
      if (!guide.active) return
      reconcileInbound()
      reconcileOutbound()
      reconcileStocktake()
      reconcileInventory()
    },
  )

  return () => {
    for (const unsubscribe of unsubscribes) unsubscribe()
    stopWatch?.()
    stopWatch = null
  }
}
