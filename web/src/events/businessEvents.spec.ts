import { describe, expect, it, vi } from 'vitest'
import businessEventsSource from './businessEvents.ts?raw'
import { BUSINESS_EVENTS, emitBusinessEvent, onBusinessEvent } from './businessEvents'

/**
 * 业务事件层是业务页面与 Demo / Guide 外挂层之间唯一的契约。
 * 这里验证两件事：事件分发本身可靠，以及它不依赖任何 Guide / Demo 概念。
 */
describe('business events layer', () => {
  it('emits without subscribers and without any store', () => {
    // 不创建 Pinia、不安装 Bridge，业务页面依然可以无条件发送事件。
    expect(() => emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_CREATED, { orderId: '100' })).not.toThrow()
    expect(() => emitBusinessEvent(BUSINESS_EVENTS.INVENTORY_TRANS_LOADED)).not.toThrow()
  })

  it('delivers the business payload to subscribers', () => {
    const listener = vi.fn()
    const unsubscribe = onBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_CREATED, listener)

    emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_CREATED, {
      orderId: '100',
      orderNo: 'IN-100',
      warehouseId: '1',
    })

    expect(listener).toHaveBeenCalledTimes(1)
    expect(listener).toHaveBeenCalledWith({ orderId: '100', orderNo: 'IN-100', warehouseId: '1' })
    unsubscribe()
  })

  it('notifies every subscriber of the same event', () => {
    const first = vi.fn()
    const second = vi.fn()
    const unsubscribeFirst = onBusinessEvent(BUSINESS_EVENTS.OUTBOUND_ORDER_SUBMITTED, first)
    const unsubscribeSecond = onBusinessEvent(BUSINESS_EVENTS.OUTBOUND_ORDER_SUBMITTED, second)

    emitBusinessEvent(BUSINESS_EVENTS.OUTBOUND_ORDER_SUBMITTED, { orderId: '42' })

    expect(first).toHaveBeenCalledTimes(1)
    expect(second).toHaveBeenCalledTimes(1)
    unsubscribeFirst()
    unsubscribeSecond()
  })

  it('stops delivering after unsubscribe', () => {
    const listener = vi.fn()
    const unsubscribe = onBusinessEvent(BUSINESS_EVENTS.STOCKTAKE_ORDER_CREATED, listener)
    unsubscribe()

    emitBusinessEvent(BUSINESS_EVENTS.STOCKTAKE_ORDER_CREATED, { orderId: '9' })

    expect(listener).not.toHaveBeenCalled()
  })

  it('only forwards the business fields it was given', () => {
    const listener = vi.fn()
    const unsubscribe = onBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_SUBMITTED, listener)

    emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_SUBMITTED, { orderId: '100', orderNo: 'IN-100' })

    const payload = listener.mock.calls[0][0] as Record<string, unknown>
    expect(Object.keys(payload).sort()).toEqual(['orderId', 'orderNo'])
    unsubscribe()
  })

  it('does not reference guide or demo concepts', () => {
    // 依赖方向硬约束：业务事件层不能反向依赖 Guide / Demo，
    // 否则删除外挂层会破坏正常业务模块。
    for (const forbidden of ['useGuideStore', 'GUIDE_EVENTS', 'guideStep', 'demoStep', 'evidenceStep', 'tour']) {
      expect(businessEventsSource).not.toContain(forbidden)
    }
    expect(businessEventsSource).not.toContain("from '@/guide")
    expect(businessEventsSource).not.toContain("from '@/stores")
    expect(businessEventsSource).not.toContain("from '@/demo")
  })
})
