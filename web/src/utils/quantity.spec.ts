import { describe, expect, it } from 'vitest'
import { MAX_QUANTITY, validOrderQuantities } from './quantity'

describe('order quantity bounds', () => {
  it('accepts the exact cumulative INT limit', () => {
    expect(validOrderQuantities([{ expected_qty: MAX_QUANTITY - 1 }, { expected_qty: 1 }])).toBe(true)
  })
  it.each([[MAX_QUANTITY, 1], [MAX_QUANTITY + 1], [1.5], [0], [-1], [Number.NaN], [Number.POSITIVE_INFINITY]])('rejects invalid quantity rows %s', (...quantities) => {
    expect(validOrderQuantities(quantities.map((expected_qty) => ({ expected_qty })))).toBe(false)
  })
})
