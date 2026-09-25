import { describe, expect, it } from 'vitest'
import type { DemoActivitySnapshot, DemoOperationLog, InboundOrderItem } from '@/api/types'
import { buildDemoOperationRows } from './demoOperations'

function operation(overrides: Partial<DemoOperationLog>): DemoOperationLog {
  return {
    id: '1',
    user_id: '1',
    username: 'demo',
    path: '/api/v1/inbound/orders/101/submit',
    method: 'POST',
    params: '',
    cost_ms: 2,
    status: 200,
    result: '{"code":0,"msg":"success","data":null}',
    created_at: '2026-09-25T10:00:00+08:00',
    ...overrides,
  }
}

const order = {
  id: '101',
  order_no: 'RK20260925000001',
} as InboundOrderItem

const snapshot = {
  inbound_orders: [order],
  outbound_orders: [],
  stocktake_orders: [],
  tasks: [],
  inventory_trans: [],
  operations: [],
} as DemoActivitySnapshot

describe('buildDemoOperationRows', () => {
  it('uses snapshot order number and status transition', () => {
    const [row] = buildDemoOperationRows([operation({})], snapshot)
    expect(row.objectNo).toBe('RK20260925000001')
    expect(row.beforeStatus).toBe('草稿')
    expect(row.afterStatus).toBe('已提交')
  })

  it('reads quantity and marks failed operations', () => {
    const [row] = buildDemoOperationRows([
      operation({
        path: '/api/v1/inbound/orders/101/receive',
        params: '{"qty":10}',
        status: 500,
      }),
    ], snapshot)

    expect(row.operation).toBe('完成收货')
    expect(row.quantityChange).toBe('+10')
    expect(row.afterStatus).toBe('处理失败')
  })
})
