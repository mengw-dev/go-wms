/**
 * Guide 静态步骤定义。
 *
 * 每个场景是一份只读步骤表：路由、高亮目标、文案、完成事件。
 * Store 只保存运行态，不保存这些静态配置。
 */
import { GUIDE_EVENTS } from './events'
import type { GuideScenario, GuideStep } from './types'

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

export function getGuideSteps(scenario: GuideScenario): readonly GuideStep[] {
  return GUIDE_STEPS[scenario]
}

export function getGuideStep(scenario: GuideScenario, index: number): GuideStep | null {
  return GUIDE_STEPS[scenario][index] ?? null
}

export const GUIDE_SCENARIO_LABELS: Record<GuideScenario, string> = {
  inbound: '入库',
  outbound: '出库',
  stocktake: '盘点',
}
