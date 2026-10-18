import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { createInboundOrder, getInboundOrder, updateInboundOrder } from '@/api/inbound'
import type { InboundOrderDetail, InboundOrderItem } from '@/api/types'
import { useInboundOrderForm } from './useInboundOrderForm'

vi.mock('@/api/inbound', () => ({ createInboundOrder: vi.fn(), getInboundOrder: vi.fn(), updateInboundOrder: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessage: { warning: vi.fn(), error: vi.fn(), success: vi.fn() } }))
vi.mock('@/events/businessEvents', () => ({ BUSINESS_EVENTS: {}, emitBusinessEvent: vi.fn() }))

function order(id: string): InboundOrderItem {
  return { id, order_no: id, warehouse_id: `WH-${id}`, remark: id, status: 'DRAFT', source: 'MANUAL', import_task_id: null, expected_qty: 5, received_qty: 0, defective_qty: 0, created_by: '', created_at: '', updated_at: '' }
}
function detail(id: string): InboundOrderDetail {
  return { order: order(id), details: [{ id: `D-${id}`, order_id: id, sku_id: `SKU-${id}`, sku_code: id, sku_name: id, expected_qty: 5, received_qty: 0, defective_qty: 0, batch_no: '' }], tasks: [] }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}

beforeEach(() => vi.resetAllMocks())

describe('入库编辑对象与加载生命周期', () => {
  it('打开 B 立即清空 A 内容，B 加载完成前不能保存', async () => {
    const form = useInboundOrderForm(vi.fn())
    vi.mocked(getInboundOrder).mockResolvedValueOnce(detail('A'))
    await form.openEdit(order('A'))
    const b = deferred<InboundOrderDetail>()
    vi.mocked(getInboundOrder).mockReturnValueOnce(b.promise)
    const loading = form.openEdit(order('B'))
    expect(form.editForm.warehouse_id).toBeUndefined()
    expect(form.editForm.remark).toBe('')
    await form.submitEdit()
    expect(updateInboundOrder).not.toHaveBeenCalled()
    b.resolve(detail('B'))
    await loading
    await form.submitEdit()
    expect(updateInboundOrder).toHaveBeenCalledWith('B', { warehouse_id: 'WH-B', remark: 'B', details: [{ sku_id: 'SKU-B', expected_qty: 5 }] })
  })

  it('A 的详情晚返回不能覆盖已加载的 B', async () => {
    const form = useInboundOrderForm(vi.fn())
    const a = deferred<InboundOrderDetail>()
    vi.mocked(getInboundOrder).mockReturnValueOnce(a.promise).mockResolvedValueOnce(detail('B'))
    const first = form.openEdit(order('A'))
    await form.openEdit(order('B'))
    a.resolve(detail('A'))
    await first
    expect(form.editForm.remark).toBe('B')
    expect(form.editDialog.editingId).toBe('B')
  })

  it('加载失败时不能保存，新建时隔离上一编辑请求的晚返回', async () => {
    const form = useInboundOrderForm(vi.fn())
    vi.mocked(getInboundOrder).mockRejectedValueOnce(new Error('offline'))
    await form.openEdit(order('A'))
    await form.submitEdit()
    expect(updateInboundOrder).not.toHaveBeenCalled()
    const b = deferred<InboundOrderDetail>()
    vi.mocked(getInboundOrder).mockReturnValueOnce(b.promise)
    const loading = form.openEdit(order('B'))
    form.openCreate()
    b.resolve(detail('B'))
    await loading
    expect(form.editDialog.editingId).toBe('')
    expect(form.editForm.warehouse_id).toBeUndefined()
  })

  it('关闭后晚返回不恢复表单，保存中禁止双击和切换目标', async () => {
    const scope = effectScope()
    const form = scope.run(() => useInboundOrderForm(vi.fn()))!
    const loading = deferred<InboundOrderDetail>()
    vi.mocked(getInboundOrder).mockReturnValueOnce(loading.promise)
    const opened = form.openEdit(order('A'))
    form.editDialog.visible = false
    loading.resolve(detail('A'))
    await opened
    expect(form.editDialog.ready).toBe(false)
    vi.mocked(getInboundOrder).mockResolvedValueOnce(detail('B'))
    await form.openEdit(order('B'))
    const saving = deferred<void>()
    vi.mocked(updateInboundOrder).mockReturnValueOnce(saving.promise)
    const saved = form.submitEdit()
    await form.submitEdit()
    await form.openEdit(order('C'))
    expect(form.editDialog.editingId).toBe('B')
    expect(updateInboundOrder).toHaveBeenCalledTimes(1)
    expect(createInboundOrder).not.toHaveBeenCalled()
    saving.resolve()
    await saved
    scope.stop()
  })
})
