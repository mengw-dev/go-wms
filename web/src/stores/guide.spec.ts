import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { GUIDE_EVENTS, useGuideStore } from './guide'

describe('manual guide store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('starts at the real business entry and blocks manual advancement without a result', () => {
    const guide = useGuideStore()
    const firstStep = guide.start('inbound')

    expect(firstStep.target).toBe('[data-tour="inbound-create"]')
    expect(guide.active).toBe(true)
    expect(guide.currentStep).toBe(0)
    expect(guide.currentStepNumber).toBe(1)
    expect(guide.canAdvance).toBe(false)
    expect(guide.next()).toBe(false)
    expect(guide.completed).toBe(false)
  })

  it('auto-advances through every real inbound milestone', () => {
    const guide = useGuideStore()
    guide.start('inbound')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, {
      orderId: '100',
      orderNo: 'IN-100',
      message: '已创建入库单 IN-100。',
    })).toBe(true)
    expect(guide.currentStepDefinition?.id).toBe('inbound-submit')
    expect(guide.currentStepRoute).toBe('/inbound/orders/100')
    expect(guide.lastOutcome).toContain('已创建入库单')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderSubmitted)).toBe(true)
    expect(guide.currentStepDefinition?.id).toBe('inbound-approve')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderApproved)).toBe(true)
    expect(guide.currentStepDefinition?.id).toBe('inbound-receive')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundReceived, {
      taskId: '7',
      taskNo: 'PUT-7',
    })).toBe(true)
    expect(guide.currentStepDefinition?.id).toBe('inbound-tasks')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundPutawayReady)).toBe(true)
    expect(guide.currentStepDefinition?.id).toBe('inbound-putaway')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundPutawayCompleted)).toBe(true)
    expect(guide.currentStepDefinition?.id).toBe('inbound-inventory')
    expect(guide.currentStepRoute).toBe('/inventory?order_no=IN-100')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundInventoryReviewed)).toBe(true)
    expect(guide.orderNo).toBe('IN-100')
    expect(guide.taskNo).toBe('PUT-7')
    expect(guide.completed).toBe(true)
    expect(guide.active).toBe(false)
  })

  it('clears a mismatch when the correct inbound result is recorded', () => {
    const guide = useGuideStore()
    guide.start('inbound')
    guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, {
      orderId: '100',
      orderNo: 'IN-100',
    })
    guide.setMismatch('状态不一致')

    expect(guide.canAdvance).toBe(false)
    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderSubmitted)).toBe(true)
    expect(guide.mismatch).toBe('')
    expect(guide.currentStepDefinition?.id).toBe('inbound-approve')
  })

  it('auto-advances through every real outbound milestone', () => {
    const guide = useGuideStore()
    guide.start('outbound')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.outboundOrderCreated, {
      orderId: '42',
      orderNo: 'OUT-42',
      message: '出库单已创建。',
    })).toBe(true)
    expect(guide.currentStepDefinition?.id).toBe('outbound-submit')
    expect(guide.currentStepRoute).toBe('/outbound/orders/42')

    guide.recordBusinessResult(GUIDE_EVENTS.outboundOrderSubmitted)
    expect(guide.currentStepDefinition?.id).toBe('outbound-allocate')

    guide.recordBusinessResult(GUIDE_EVENTS.outboundOrderAllocated, {
      taskId: '7',
      taskNo: 'PICK-7',
      message: '系统刚刚完成库存分配。',
    })
    expect(guide.currentStepDefinition?.id).toBe('outbound-tasks')
    expect(guide.lastOutcome).toContain('库存分配')

    guide.recordBusinessResult(GUIDE_EVENTS.outboundPickTasksReady)
    expect(guide.currentStepDefinition?.id).toBe('outbound-pick')

    guide.recordBusinessResult(GUIDE_EVENTS.outboundPicked)
    expect(guide.currentStepDefinition?.id).toBe('outbound-shipped')

    guide.recordBusinessResult(GUIDE_EVENTS.outboundShipped)
    expect(guide.currentStepDefinition?.id).toBe('outbound-inventory')
    expect(guide.currentStepRoute).toBe('/inventory?order_no=OUT-42')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.outboundInventoryReviewed)).toBe(true)
    expect(guide.taskNo).toBe('PICK-7')
    expect(guide.completed).toBe(true)
    expect(guide.active).toBe(false)
  })

  it('reports a mismatch instead of advancing for an unrelated result', () => {
    const guide = useGuideStore()
    guide.start('stocktake')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, { orderId: '9' })).toBe(false)
    expect(guide.canAdvance).toBe(false)
    expect(guide.mismatch).toContain('当前业务状态与引导不一致')
    expect(guide.orderId).toBe('')
  })

  it('auto-advances through the stocktake milestones and keeps authoritative facts', () => {
    const guide = useGuideStore()
    guide.start('stocktake')

    guide.recordBusinessResult(GUIDE_EVENTS.stocktakeOrderCreated, {
      orderId: '88',
      orderNo: 'PD-88',
      message: '已创建盘点单。',
    })
    expect(guide.currentStepDefinition?.id).toBe('stocktake-snapshot')
    expect(guide.currentStepRoute).toBe('/stocktake/orders/88')

    guide.recordBusinessResult(GUIDE_EVENTS.stocktakeSnapshotReady, {
      facts: [{ label: '账面库存', value: '100' }],
    })
    expect(guide.currentStepDefinition?.id).toBe('stocktake-actual')

    guide.recordBusinessResult(GUIDE_EVENTS.stocktakeActualCompleted, {
      facts: [{ label: '实盘库存', value: '97' }, { label: '差异', value: '-3' }],
    })
    expect(guide.currentStepDefinition?.id).toBe('stocktake-difference')

    guide.recordBusinessResult(GUIDE_EVENTS.stocktakeDifferenceReviewed)
    expect(guide.currentStepDefinition?.id).toBe('stocktake-approve')

    guide.recordBusinessResult(GUIDE_EVENTS.stocktakeApproved, {
      message: '审核完成，库存已调整。',
    })
    expect(guide.currentStepDefinition?.id).toBe('stocktake-inventory')
    expect(guide.currentStepRoute).toBe('/inventory?order_no=PD-88')

    guide.recordBusinessResult(GUIDE_EVENTS.stocktakeInventoryReviewed)
    expect(guide.completed).toBe(true)
    expect(guide.active).toBe(false)
    expect(guide.facts).toContainEqual({ label: '差异', value: '-3' })
  })

  it('keeps the guide moving when the real business state is ahead', () => {
    const guide = useGuideStore()
    guide.start('inbound')
    guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, {
      orderId: '101',
      orderNo: 'IN-101',
    })
    expect(guide.currentStepDefinition?.id).toBe('inbound-submit')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderApproved, {
      message: '审核完成：IN-101 已进入已审核状态。',
    })).toBe(true)
    expect(guide.currentStepDefinition?.id).toBe('inbound-receive')
    // 被跳过的中间步骤是“推断完成”，不是真实验证
    expect(guide.inferredStepIds).toContain('inbound-submit')
    expect(guide.verifiedStepIds).not.toContain('inbound-submit')
    expect(guide.verifiedStepIds).toContain('inbound-approve')
    expect(guide.lastOutcome).toContain('已自动同步引导进度')
  })

  it('restores the active guide from session storage after a refresh', () => {
    const values = new Map<string, string>()
    vi.stubGlobal('window', {
      sessionStorage: {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => values.set(key, value),
        removeItem: (key: string) => values.delete(key),
      },
    })
    try {
      const guide = useGuideStore()
      guide.start('outbound')
      guide.recordBusinessResult(GUIDE_EVENTS.outboundOrderCreated, {
        orderId: '55',
        orderNo: 'OUT-55',
        message: '出库单已创建。',
      })

      setActivePinia(createPinia())
      const restored = useGuideStore()
      expect(restored.active).toBe(true)
      expect(restored.scenario).toBe('outbound')
      expect(restored.orderId).toBe('55')
      expect(restored.orderNo).toBe('OUT-55')
      expect(restored.verifiedStepIds).toContain('outbound-create')
      expect(restored.currentStepDefinition?.id).toBe('outbound-submit')
      restored.cancel()
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('restores inferred steps separately from verified ones after a refresh', () => {
    const values = new Map<string, string>()
    vi.stubGlobal('window', {
      sessionStorage: {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => values.set(key, value),
        removeItem: (key: string) => values.delete(key),
      },
    })
    try {
      const guide = useGuideStore()
      guide.start('inbound')
      guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, { orderId: '7', orderNo: 'IN-7' })
      guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderApproved)

      setActivePinia(createPinia())
      const restored = useGuideStore()
      expect(restored.inferredStepIds).toContain('inbound-submit')
      expect(restored.verifiedStepIds).toContain('inbound-create')
      expect(restored.verifiedStepIds).toContain('inbound-approve')
      expect(restored.verifiedStepIds).not.toContain('inbound-submit')
      restored.cancel()
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('can cancel a guide without leaving stale business identifiers', () => {
    const guide = useGuideStore()
    guide.start('inbound')
    guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, { orderId: '100' })
    guide.cancel()

    expect(guide.active).toBe(false)
    expect(guide.scenario).toBeNull()
    expect(guide.orderId).toBe('')
    expect(guide.verifiedStepIds).toEqual([])
    expect(guide.inferredStepIds).toEqual([])
  })
})

describe('manual guide continuation and repositioning', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('advances with next() after returning to a completed step', () => {
    const guide = useGuideStore()
    guide.start('inbound')
    guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, { orderId: '100', orderNo: 'IN-100' })
    expect(guide.currentStepDefinition?.id).toBe('inbound-submit')

    expect(guide.reposition('/inbound/orders')).toBe(true)
    expect(guide.currentStep).toBe(0)
    expect(guide.canAdvance).toBe(true)

    expect(guide.next()).toBe(true)
    expect(guide.currentStep).toBe(1)
    expect(guide.currentStepDefinition?.id).toBe('inbound-submit')
  })

  it('finishes the guide when next() is used on the last step', () => {
    const guide = useGuideStore()
    guide.start('stocktake')
    guide.currentStep = guide.totalSteps - 1
    guide.verifiedStepIds = ['stocktake-inventory']

    expect(guide.canAdvance).toBe(true)
    expect(guide.next()).toBe(true)
    expect(guide.completed).toBe(true)
    expect(guide.active).toBe(false)
  })

  it('moves back with previous() and stops at the first step', () => {
    const guide = useGuideStore()
    guide.start('inbound')
    guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, { orderId: '100', orderNo: 'IN-100' })
    expect(guide.currentStep).toBe(1)

    expect(guide.canGoPrevious).toBe(true)
    expect(guide.previous()).toBe(true)
    expect(guide.currentStep).toBe(0)

    expect(guide.canGoPrevious).toBe(false)
    expect(guide.previous()).toBe(false)
  })

  it('reports a mismatch when repositioning from a route outside the scenario', () => {
    const guide = useGuideStore()
    guide.start('inbound')

    expect(guide.reposition('/system/users')).toBe(false)
    expect(guide.mismatch).toContain('不在本次引导流程中')
    expect(guide.currentStep).toBe(0)
  })

  it('restarts the current scenario from the first step', () => {
    const guide = useGuideStore()
    guide.start('inbound')
    guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, { orderId: '100', orderNo: 'IN-100' })

    expect(guide.restart()).not.toBeNull()
    expect(guide.active).toBe(true)
    expect(guide.currentStep).toBe(0)
    expect(guide.orderId).toBe('')
    expect(guide.verifiedStepIds).toEqual([])
  })

  it('treats skipping as cancel and leaves nothing behind', () => {
    const guide = useGuideStore()
    guide.start('outbound')
    guide.recordBusinessResult(GUIDE_EVENTS.outboundOrderCreated, { orderId: '42', orderNo: 'OUT-42' })

    // “跳过”按钮走的是 guide.cancel()
    guide.cancel()

    expect(guide.active).toBe(false)
    expect(guide.completed).toBe(false)
    expect(guide.scenario).toBeNull()
    expect(guide.currentStepDefinition).toBeNull()
    expect(guide.mismatch).toBe('')
  })

  it('ignores mismatch reports while no guide is running', () => {
    const guide = useGuideStore()

    guide.setMismatch('不应写入')

    expect(guide.mismatch).toBe('')
    expect(guide.active).toBe(false)
  })
})
