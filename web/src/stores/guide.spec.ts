import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { GUIDE_EVENTS, useGuideStore } from './guide'

describe('manual guide store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('starts at the real business entry and blocks completion without a result', () => {
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

  it('unlocks next only after the matching business result', () => {
    const guide = useGuideStore()
    guide.start('outbound')

    expect(
      guide.recordBusinessResult(GUIDE_EVENTS.outboundOrderCreated, {
        orderId: '42',
        taskId: 'task-7',
        message: '出库单已创建。',
      }),
    ).toBe(true)
    expect(guide.canAdvance).toBe(true)
    expect(guide.orderId).toBe('42')
    expect(guide.taskId).toBe('task-7')
    expect(guide.lastOutcome).toBe('出库单已创建。')
    expect(guide.next()).toBe(true)
    expect(guide.completed).toBe(true)
  })

  it('reports a mismatch instead of advancing for an unrelated result', () => {
    const guide = useGuideStore()
    guide.start('stocktake')

    expect(guide.recordBusinessResult(GUIDE_EVENTS.inboundOrderCreated, { orderId: '9' })).toBe(false)
    expect(guide.canAdvance).toBe(false)
    expect(guide.mismatch).toContain('当前业务状态与引导不一致')
    expect(guide.orderId).toBe('')
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
  })
})