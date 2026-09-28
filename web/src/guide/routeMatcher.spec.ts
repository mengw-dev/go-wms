import { describe, expect, it } from 'vitest'
import { isGuideFlowRoute, matchGuideStepByRoute, resolveGuideRoute } from './routeMatcher'

describe('resolveGuideRoute', () => {
  it('substitutes order placeholders', () => {
    expect(resolveGuideRoute('/inbound/orders/:orderId', '100', 'IN-100')).toBe('/inbound/orders/100')
  })

  it('encodes substituted values', () => {
    expect(resolveGuideRoute('/inventory?order_no=:orderNo', '100', 'IN 100/1')).toBe(
      '/inventory?order_no=IN%20100%2F1',
    )
  })

  it('leaves routes without placeholders untouched', () => {
    expect(resolveGuideRoute('/inbound/orders', '', '')).toBe('/inbound/orders')
  })
})

describe('matchGuideStepByRoute', () => {
  it('returns the first unfinished step when a route maps to several steps', () => {
    const match = matchGuideStepByRoute('inbound', '/inbound/orders/100', '100', 'IN-100', [])

    expect(match?.step.id).toBe('inbound-submit')
    expect(match?.index).toBe(1)
  })

  it('skips steps that are already done', () => {
    const match = matchGuideStepByRoute(
      'inbound',
      '/inbound/orders/100',
      '100',
      'IN-100',
      ['inbound-submit', 'inbound-approve'],
    )

    expect(match?.step.id).toBe('inbound-receive')
    expect(match?.index).toBe(3)
  })

  it('stays on the last step of the route when everything is done', () => {
    const done = ['inbound-submit', 'inbound-approve', 'inbound-receive', 'inbound-tasks', 'inbound-putaway']
    const match = matchGuideStepByRoute('inbound', '/inbound/orders/100', '100', 'IN-100', done)

    expect(match?.step.id).toBe('inbound-putaway')
  })

  it('ignores the query string when the step filters by order number', () => {
    const match = matchGuideStepByRoute('inbound', '/inventory', '100', 'IN-100', [])

    expect(match?.step.id).toBe('inbound-inventory')
  })

  it('returns null for a route outside the scenario', () => {
    expect(matchGuideStepByRoute('inbound', '/outbound/orders', '', '', [])).toBeNull()
    expect(matchGuideStepByRoute('stocktake', '/dashboard', '', '', [])).toBeNull()
  })
})

describe('isGuideFlowRoute', () => {
  it('recognizes routes that belong to the scenario', () => {
    expect(isGuideFlowRoute('outbound', '/outbound/orders', '', '')).toBe(true)
    expect(isGuideFlowRoute('outbound', '/inventory', '', 'OUT-1')).toBe(true)
  })

  it('rejects unrelated routes', () => {
    expect(isGuideFlowRoute('outbound', '/system/users', '', '')).toBe(false)
    expect(isGuideFlowRoute('outbound', '/inbound/orders', '', '')).toBe(false)
  })
})
