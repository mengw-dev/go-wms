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

  it('manual focus does not inherit stale automatic scenario context', () => {
    const manualSnapshot = {
      ...snapshot,
      inbound_orders: [
        ...snapshot.inbound_orders,
        { id: '202', order_no: 'IN-2', status: 'COMPLETED', expected_qty: 3, received_qty: 3, created_at: '2026-09-24T02:00:01.000Z' },
      ],
    } as unknown as DemoActivitySnapshot
    const focus = resolveDemoEvidenceFocus({
      scenario: 'inbound',
      source: 'manual',
      order_id: '202',
      order_no: 'IN-2',
      started_at: '2026-09-24T02:00:00.000Z',
      completed_at: '2026-09-24T02:00:05.000Z',
    }, context)

    expect(focus.source).toBe('manual')
    expect(focus.orderIds.inbound).toEqual(['202'])
    expect(focus.orderNos).toEqual(['IN-2'])
    expect(filterDemoActivity(manualSnapshot, focus).inbound_orders.map((item) => item.order_no)).toEqual(['IN-2'])
  })

  it('staged focus reuses business identities only from the exact same execution', () => {
    const query = { source: 'staged', scenario: 'inbound', started_at: context.startedAt, completed_at: context.completedAt }
    expect(resolveDemoEvidenceFocus(query, context).orderIds.inbound).toEqual(['101'])
    expect(resolveDemoEvidenceFocus({ ...query, started_at: '2026-09-24T03:00:00.000Z' }, context).orderIds).toEqual({})
    expect(resolveDemoEvidenceFocus({ ...query, scenario: 'full' }, context).orderIds).toEqual({})
    expect(resolveDemoEvidenceFocus({ ...query, completed_at: '2026-09-24T03:00:05.000Z' }, context).orderIds).toEqual({})
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

  it('distinguishes consecutive executions by business id even when timestamps overlap', () => {
    const secondContext: DemoEvidenceContext = {
      ...context,
      links: [
        { label: '查看入库单', path: '/inbound/orders/202' },
        { label: '查看任务', path: '/tasks?task_id=202' },
        { label: '查看库存流水', path: '/inventory?order_no=IN-2' },
      ],
    }
    const closeSnapshot = {
      ...snapshot,
      inbound_orders: [
        ...snapshot.inbound_orders,
        { id: '202', order_no: 'IN-2', status: 'COMPLETED', expected_qty: 3, received_qty: 3, created_at: '2026-09-24T01:00:01.000Z' },
      ],
      tasks: [...snapshot.tasks],
    } as unknown as DemoActivitySnapshot

    const first = filterDemoActivity(closeSnapshot, resolveDemoEvidenceFocus({}, context))
    const second = filterDemoActivity(closeSnapshot, resolveDemoEvidenceFocus({}, secondContext))
    expect(first.inbound_orders.map((item) => item.order_no)).toEqual(['IN-1'])
    expect(second.inbound_orders.map((item) => item.order_no)).toEqual(['IN-2'])
  })

  it('prefers explicit task ids over order number when both are available', () => {
    const focus = resolveDemoEvidenceFocus({ task_id: '201' }, context)
    const tasks = snapshot.tasks.map((item) => ({ ...item, order_no: 'IN-1' }))
    const filtered = filterDemoActivity({ ...snapshot, tasks } as DemoActivitySnapshot, focus)
    expect(filtered.tasks.map((item) => item.id)).toEqual(['201'])
  })

  it('excludes adjacent-run order operations inside the same execution time window', () => {
    const operation = snapshot.operations[0]!
    const closeSnapshot = {
      ...snapshot,
      operations: [
        { ...operation, id: '1', result: JSON.stringify({ data: { id: '101', order_no: 'IN-1' } }) },
        { ...operation, id: '2', result: JSON.stringify({ data: { id: '100', order_no: 'IN-OLD' } }) },
        { ...operation, id: '3', path: '/api/v1/inbound/orders/101/submit' },
        { ...operation, id: '4', path: '/api/v1/inbound/orders/100/submit' },
        { ...operation, id: '5', path: '/api/v1/inbound/tasks/201/putaway' },
        { ...operation, id: '6', path: '/api/v1/inbound/tasks/202/putaway' },
      ],
    } as DemoActivitySnapshot
    const filtered = filterDemoActivity(closeSnapshot, resolveDemoEvidenceFocus({}, context))
    expect(filtered.operations.map(item => item.id)).toEqual(['1', '3', '5'])
  })

  it('uses string ids for focused operations and retains the unscoped history', () => {
    const id = '9007199254740993'
    const operations = [
      { ...snapshot.operations[0]!, id: '1', path: `/api/v1/inbound/orders/${id}/approve` },
      { ...snapshot.operations[0]!, id: '2', path: '/api/v1/inbound/orders/9007199254740992/approve' },
    ]
    const current = { ...snapshot, operations }
    const focus = resolveDemoEvidenceFocus({ source: 'manual', scenario: 'inbound', order_id: id }, null)
    expect(filterDemoActivity(current, focus).operations.map(item => item.id)).toEqual(['1'])
    expect(filterDemoActivity(current, resolveDemoEvidenceFocus({}, null)).operations).toEqual(operations)
  })

  it('full execution keeps outbound tasks and ledger when only inbound has a ledger link', () => {
    const fullContext: DemoEvidenceContext = {
      ...context, scenario: 'full', links: [...(context.links ?? []), { label: '出库单', path: '/outbound/orders/401' }],
    }
    const outboundTask = { ...snapshot.tasks[1]!, id: '402', order_id: '401', order_no: 'OUT-1' }
    const outboundLedger = { ...snapshot.inventory_trans[1]!, order_no: 'OUT-1' }
    const current = {
      ...snapshot,
      outbound_orders: [{ id: '401', order_no: 'OUT-1', created_at: '2026-09-24T01:00:02.000Z' }],
      tasks: [...snapshot.tasks, outboundTask],
      inventory_trans: [...snapshot.inventory_trans, outboundLedger],
      operations: [
        { ...snapshot.operations[0]!, id: '3', path: '/api/v1/outbound/tasks/402/pick' },
        { ...snapshot.operations[0]!, id: '4', path: '/api/v1/outbound/tasks/202/pick' },
      ],
    } as DemoActivitySnapshot
    const focus = resolveDemoEvidenceFocus({ source: 'staged', scenario: 'full', started_at: context.startedAt, completed_at: context.completedAt }, fullContext)
    const result = filterDemoActivity(current, focus)
    expect(result.tasks.map(task => task.id)).toEqual(['201', '402'])
    expect(result.inventory_trans.map(row => row.order_no)).toEqual(['IN-1', 'OUT-1'])
    expect(result.operations.map(row => row.id)).toEqual(['3'])
  })
