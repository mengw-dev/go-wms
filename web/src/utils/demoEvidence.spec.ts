import { describe, expect, it } from 'vitest'
import type { DemoActivitySnapshot } from '@/api/types'
import { filterDemoActivity, resolveDemoEvidenceFocus, type DemoEvidenceContext } from './demoEvidence'

const context: DemoEvidenceContext = {
  scenario: 'inbound',
  startedAt: '2026-09-24T01:00:00.000Z',
  completedAt: '2026-09-24T01:00:05.000Z',
  summary: '入库单 IN-1 已完成',
  evidence: [],
  links: [
    { label: '查看入库单', path: '/inbound/orders/101' },
    { label: '查看任务', path: '/tasks?order_id=101' },
    { label: '查看库存流水', path: '/inventory?order_no=IN-1' },
  ],
}

const snapshot = {
  operations: [
    { id: '1', user_id: '1', username: 'demo', path: '/api/v1/inbound/orders', method: 'POST', ip: '127.0.0.1', cost_ms: 10, status: 200, created_at: '2026-09-24T01:00:01.000Z' },
    { id: '2', user_id: '1', username: 'demo', path: '/api/v1/demo/activity', method: 'GET', ip: '127.0.0.1', cost_ms: 3, status: 200, created_at: '2026-09-24T01:00:02.000Z' },
  ],
  inbound_orders: [
    { id: '101', order_no: 'IN-1', status: 'COMPLETED', expected_qty: 5, received_qty: 5, created_at: '2026-09-24T01:00:01.000Z' },
    { id: '100', order_no: 'IN-OLD', status: 'COMPLETED', expected_qty: 2, received_qty: 2, created_at: '2026-09-23T01:00:00.000Z' },
  ],
  outbound_orders: [],
  stocktake_orders: [],
  tasks: [
    { id: '201', task_no: 'PUT-1', task_type: 'PUTAWAY', status: 'COMPLETED', order_id: '101', order_no: 'IN-1', done_qty: 5, target_qty: 5, created_at: '2026-09-24T01:00:03.000Z' },
    { id: '202', task_no: 'PICK-OLD', task_type: 'PICK', status: 'COMPLETED', order_id: '99', order_no: 'OUT-OLD', done_qty: 1, target_qty: 1, created_at: '2026-09-24T01:00:03.000Z' },
  ],
  inventory_trans: [
    { id: '301', inventory_id: '1', trans_type: 'RECEIVE', quantity_change: 5, before_quantity: 0, after_quantity: 5, available_before: 0, available_after: 5, order_no: 'IN-1', created_at: '2026-09-24T01:00:04.000Z' },
    { id: '302', inventory_id: '1', trans_type: 'SHIP', quantity_change: -1, before_quantity: 5, after_quantity: 4, available_before: 5, available_after: 4, order_no: 'OUT-OLD', created_at: '2026-09-24T01:00:04.000Z' },
  ],
} as unknown as DemoActivitySnapshot

describe('demo evidence focus', () => {
  it('derives business ids from the latest execution links', () => {
    const focus = resolveDemoEvidenceFocus({}, context)

    expect(focus.scenario).toBe('inbound')
    expect(focus.orderIds.inbound).toEqual(['101'])
    expect(focus.orderNos).toEqual(['IN-1'])
  })

  it('shows only real evidence from the focused execution window', () => {
    const focus = resolveDemoEvidenceFocus({}, context)
    const filtered = filterDemoActivity(snapshot, focus)

    expect(filtered.inbound_orders.map((item) => item.order_no)).toEqual(['IN-1'])
    expect(filtered.tasks.map((item) => item.task_no)).toEqual(['PUT-1'])
    expect(filtered.inventory_trans.map((item) => item.order_no)).toEqual(['IN-1'])
    expect(filtered.operations.map((item) => item.path)).toEqual(['/api/v1/inbound/orders'])
  })
})
