import { describe, expect, it } from 'vitest'
import type { DemoActivitySnapshot, DemoOperationLog, InboundOrderItem } from '@/api/types'
import {
  buildBusinessOperationRows,
  buildDemoOperationRows,
  buildExperimentOperationRows,
} from './demoOperations'

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

  it('maps demo experiments and restock to readable operations', () => {
    const rows = buildExperimentOperationRows([
      operation({ id: '20', path: '/api/v1/demo/run/concurrent', params: '{"concurrency":20,"qty_per_order":5}' }),
      operation({ id: '21', path: '/api/v1/demo/run/picking', params: '{"workers":10,"contenders":5}' }),
      operation({ id: '22', path: '/api/v1/demo/run/restock', params: '{"qty":500}' }),
      operation({ id: '23', path: '/api/v1/demo/session/heartbeat' }),
    ])

    expect(rows.map((row) => row.operation)).toEqual([
      '并发库存分配实验',
      'PDA 拣货作业验证',
      '演示库存补货',
    ])
    expect(rows[0].quantityChange).toBe('20 请求 × 5 件')
    expect(rows[1].quantityChange).toBe('10 拣货员 / 5 竞争请求')
    expect(rows[2].quantityChange).toBe('+500 件')
  })

  it('shows only real business actions in business evidence', () => {
    const rows = buildBusinessOperationRows([
      operation({ id: '30', path: '/api/v1/inbound/orders/101/submit' }),
      operation({ id: '31', path: '/api/v1/demo/session/heartbeat' }),
      operation({ id: '32', path: '/api/v1/demo/run/concurrent' }),
      operation({ id: '33', path: '/api/v1/demo/reset' }),
      operation({ id: '34', path: '/api/v1/inbound/orders/batch-submit' }),
      operation({ id: '35', path: '/api/v1/inbound/orders/101/submit', method: 'GET' }),
    ], snapshot)

    expect(rows.map((row) => row.operation)).toEqual(['提交入库单'])
  })

  it('drops unknown business requests instead of showing generic POST operations', () => {
    expect(buildDemoOperationRows([
      operation({ path: '/api/v1/inbound/orders/batch-submit' }),
    ])).toEqual([])
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
