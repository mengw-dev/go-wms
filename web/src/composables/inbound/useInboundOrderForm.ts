/**
 * 入库单新建 / 编辑表单：表单模型、校验与提交。
 *
 * 仓库、货品选项由页面从列表 composable 取用并渲染，这里只负责表单本身。
 */
import { reactive, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { createInboundOrder, getInboundOrder, updateInboundOrder } from '@/api/inbound'
import type { EntityID, InboundOrderItem } from '@/api/types'
import { BUSINESS_EVENTS, emitBusinessEvent } from '@/events/businessEvents'

export function useInboundOrderForm(onSaved: () => void | Promise<void>) {
  const editDialog = reactive({ visible: false, loading: false, fetching: false, ready: false, editingId: '' as EntityID })
  let generation = 0
  const editForm = reactive({
    warehouse_id: undefined as EntityID | undefined,
    remark: '',
    details: [] as { _uid: string; sku_id: EntityID | undefined; expected_qty: number }[],
  })

  function makeDetail(sku_id?: EntityID, expected_qty = 1) {
    return { _uid: crypto.randomUUID(), sku_id, expected_qty }
  }

  function resetForm() {
    editForm.warehouse_id = undefined
    editForm.remark = ''
    editForm.details = [makeDetail()]
  }

  watch(() => editDialog.visible, (visible) => {
    if (!visible) {
      generation++
      editDialog.ready = false
      editDialog.fetching = false
    }
  }, { flush: 'sync' })

  function openCreate() {
    if (editDialog.loading) return
    generation++
    editDialog.editingId = ''
    editDialog.fetching = false
    resetForm()
    editDialog.ready = true
    editDialog.visible = true
  }

  async function openEdit(row: InboundOrderItem) {
    if (editDialog.loading) return
    const current = ++generation
    const id = row.id
    editDialog.editingId = id
    resetForm()
    editDialog.ready = false
    editDialog.fetching = true
    editDialog.visible = true
    try {
      const detail = await getInboundOrder(id)
      if (current !== generation || !editDialog.visible || editDialog.editingId !== id) return
      editForm.warehouse_id = detail.order.warehouse_id
      editForm.remark = detail.order.remark
      editForm.details = (detail.details ?? []).map((d) => makeDetail(d.sku_id, d.expected_qty))
      if (editForm.details.length === 0) editForm.details = [makeDetail()]
      editDialog.ready = true
    } catch {
      if (current === generation && editDialog.visible) ElMessage.error('入库单加载失败，请关闭后重新打开')
    } finally {
      if (current === generation) editDialog.fetching = false
    }
  }

  function addDetail() {
    editForm.details.push(makeDetail())
  }

  function removeDetail(index: number) {
    editForm.details.splice(index, 1)
  }

  async function submitEdit() {
    if (editDialog.loading || editDialog.fetching || !editDialog.ready || !editDialog.visible) return
    if (!editForm.warehouse_id) {
      ElMessage.warning('请选择仓库')
      return
    }
    const details = editForm.details.filter((d) => d.sku_id && d.expected_qty > 0)
    if (details.length === 0) {
      ElMessage.warning('请至少填写一行有效的明细（选择货品且数量大于 0）')
      return
    }
    const payload = {
      warehouse_id: editForm.warehouse_id,
      remark: editForm.remark,
      details: details.map((d) => ({ sku_id: d.sku_id!, expected_qty: d.expected_qty })),
    }
    const editingId = editDialog.editingId
    editDialog.loading = true
    try {
      if (editingId) {
        await updateInboundOrder(editingId, payload)
        ElMessage.success('保存成功')
      } else {
        const created = await createInboundOrder(payload)
        emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_CREATED, {
          orderId: String(created.id),
          orderNo: created.order_no,
        })
        ElMessage.success('创建成功')
      }
      editDialog.visible = false
      await onSaved()
    } finally {
      editDialog.loading = false
    }
  }

  return { editDialog, editForm, openCreate, openEdit, addDetail, removeDetail, submitEdit }
}
