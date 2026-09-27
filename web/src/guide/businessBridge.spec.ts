import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { BUSINESS_EVENTS, emitBusinessEvent } from '@/events/businessEvents'
import { useGuideStore } from '@/stores/guide'
import { installGuideBusinessBridge } from './businessBridge'

describe('guide business bridge', () => {
  let uninstall: () => void

  beforeEach(() => {
    setActivePinia(createPinia())
    uninstall = installGuideBusinessBridge()
  })

  afterEach(() => {
    uninstall()
  })

  it('advances the guide when a business event matches the current step', () => {
    const guide = useGuideStore()
    guide.start('inbound')

    emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_CREATED, { orderId: '100', orderNo: 'IN-100' })

    expect(guide.currentStepDefinition?.id).toBe('inbound-submit')
    expect(guide.orderId).toBe('100')
    expect(guide.orderNo).toBe('IN-100')
  })

  it('syncs an order that was already advanced before the page was opened', () => {
    const guide = useGuideStore()
    guide.start('inbound')
    emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_CREATED, { orderId: '100', orderNo: 'IN-100' })

    emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_LOADED, {
      orderId: '100',
      orderNo: 'IN-100',
      status: 'SUBMITTED',
    })

    expect(guide.currentStepDefinition?.id).toBe('inbound-approve')
  })

  it('ignores business events that belong to another order', () => {
    const guide = useGuideStore()
    guide.start('inbound')
    emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_CREATED, { orderId: '100', orderNo: 'IN-100' })

    emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_SUBMITTED, { orderId: '999', orderNo: 'IN-999' })

    expect(guide.currentStepDefinition?.id).toBe('inbound-submit')
  })

  it('keeps the business flow untouched when no guide is running', () => {
    const guide = useGuideStore()

    expect(() =>
      emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_CREATED, { orderId: '100', orderNo: 'IN-100' }),
    ).not.toThrow()
    expect(guide.active).toBe(false)
    expect(guide.orderId).toBe('')
  })
})

describe('business events without the guide layer', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('emits without subscribers and produces no side effect', () => {
    const guide = useGuideStore()

    expect(() =>
      emitBusinessEvent(BUSINESS_EVENTS.STOCKTAKE_ORDER_CREATED, { orderId: '1', orderNo: 'ST-1' }),
    ).not.toThrow()
    expect(guide.active).toBe(false)
  })
})
