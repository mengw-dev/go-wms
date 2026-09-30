/**
 * 入库单列表：查询条件、分页、选择与批量/行级操作。
 *
 * 页面只做展示与交互，这里集中承担列表相关的请求编排。
 */
import { computed, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveInboundOrder,
  batchApproveInboundOrders,
  batchCancelInboundOrders,
  batchDeleteInboundOrders,
  batchSubmitInboundOrders,
  cancelInboundOrder,
  deleteInboundByImportTask,
  deleteInboundOrder,
  listImports,
  listInboundOrders,
  submitInboundOrder,
} from '@/api/inbound'
import type { BatchOperResult, EntityID, ImportTaskItem, InboundOrderItem } from '@/api/types'
import { BUSINESS_EVENTS, emitBusinessEvent } from '@/events/businessEvents'
import { cleanParams } from '@/utils'
import { loadSkuMap, loadWarehouseOptions, toOptionMap, type IdOption } from '@/utils/options'

export function useInboundOrders() {
  // ---------- 基础选项 ----------
  const warehouseOptions = ref<IdOption[]>([])
  const warehouseMap = ref<Record<EntityID, string>>({})
  const skuOptions = ref<IdOption[]>([])
  const importBatchOptions = ref<{ task_id: string; label: string }[]>([])

  async function loadOptions() {
    warehouseOptions.value = await loadWarehouseOptions()
    warehouseMap.value = toOptionMap(warehouseOptions.value)
    const map = await loadSkuMap()
    skuOptions.value = Object.entries(map).map(([id, sku]) => ({
      id,
      label: `${sku.code} ${sku.name}`,
    }))
  }

  async function loadImports() {
    try {
      const imports = await listImports(20)
      const statusMap: Record<string, string> = { PENDING: '待处理', PROCESSING: '处理中', COMPLETED: '完成', FAILED: '失败' }
      importBatchOptions.value = imports.map((t: ImportTaskItem) => {
        const date = t.created_at ? new Date(Date.parse(t.created_at)).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : ''
        return { task_id: t.task_id, label: `${t.task_id} (${date} · ${statusMap[t.status] || t.status} · ${t.success_rows}/${t.total_rows})` }
      })
    } catch {
      // 后端未部署或没有导入历史时静默忽略
    }
  }

  // ---------- 列表 ----------
  const loading = ref(false)
  const list = ref<InboundOrderItem[]>([])
  const total = ref(0)
  const query = reactive({
    page: 1,
    page_size: 10,
    warehouse_id: '' as EntityID | '',
    status: '',
    keyword: '',
    import_task_id: '',
    created_at_from: '',
    created_at_to: '',
  })
  const dateRange = ref<[string, string] | null>(null)
  const dateRangeDefaultTime: [Date, Date] = [new Date(2000, 0, 1, 0, 0, 0), new Date(2000, 0, 1, 23, 59, 59)]

  let requestSeq = 0

  async function load(silent = false) {
    const seq = ++requestSeq
    if (!silent) loading.value = true
    try {
      const data = await listInboundOrders(cleanParams({ ...query }))
      if (seq !== requestSeq) return
      list.value = data.list ?? []
      total.value = data.total ?? 0
    } finally {
      if (!silent && seq === requestSeq) loading.value = false
    }
  }

  function search() {
    query.page = 1
    load()
  }

  function onDateRangeChange(value: [string, string] | null): void {
    query.created_at_from = value?.[0] ?? ''
    query.created_at_to = value?.[1] ?? ''
    search()
  }

  function resetSearch() {
    query.page = 1
    query.page_size = 10
    query.warehouse_id = ''
    query.status = ''
    query.keyword = ''
    query.import_task_id = ''
    query.created_at_from = ''
    query.created_at_to = ''
    dateRange.value = null
    load()
  }

  // ---------- 行操作 ----------
  async function onSubmit(row: InboundOrderItem) {
    try {
      await ElMessageBox.confirm(`确定提交入库单「${row.order_no}」吗？`, '提示', { type: 'warning' })
    } catch {
      return
    }
    await submitInboundOrder(row.id)
    emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_SUBMITTED, {
      orderId: String(row.id),
      orderNo: row.order_no,
    })
    ElMessage.success('提交成功')
    load()
  }

  async function onApprove(row: InboundOrderItem) {
    try {
      await ElMessageBox.confirm(`确定审核通过入库单「${row.order_no}」吗？`, '提示', { type: 'warning' })
    } catch {
      return
    }
    await approveInboundOrder(row.id)
    emitBusinessEvent(BUSINESS_EVENTS.INBOUND_ORDER_APPROVED, {
      orderId: String(row.id),
      orderNo: row.order_no,
    })
    ElMessage.success('审核通过')
    load()
  }

  async function onCancel(row: InboundOrderItem) {
    try {
      await ElMessageBox.confirm(
        `确定取消入库单「${row.order_no}」吗？取消后不能继续收货或上架。`,
        '取消入库单',
        {
          type: 'warning',
          confirmButtonText: '确认取消',
          cancelButtonText: '返回',
          confirmButtonClass: 'el-button--danger',
        },
      )
    } catch {
      return
    }
    await cancelInboundOrder(row.id)
    ElMessage.success('已取消')
    load()
  }

  async function onDelete(row: InboundOrderItem) {
    try {
      await ElMessageBox.confirm(
        `确定删除草稿入库单「${row.order_no}」吗？删除后不可恢复。`,
        '删除入库单',
        {
          type: 'warning',
          confirmButtonText: '确认删除',
          cancelButtonText: '返回',
          confirmButtonClass: 'el-button--danger',
        },
      )
    } catch {
      return
    }
    await deleteInboundOrder(row.id)
    ElMessage.success('删除成功')
    load()
    loadImports()
  }

  // ---------- 批量操作 ----------
  const selectedRows = ref<InboundOrderItem[]>([])
  const tableRef = ref()

  function onSelectionChange(rows: InboundOrderItem[]) {
    selectedRows.value = rows
  }

  function onBatchCommand(cmd: string) {
    if (cmd === 'delete') onBatchDelete()
    else if (cmd === 'submit') onBatchSubmit()
    else if (cmd === 'approve') onBatchApprove()
    else if (cmd === 'cancel') onBatchCancel()
    else if (cmd === 'batch-by-task' && singleImportBatch.value) onBatchDeleteByTask(singleImportBatch.value)
  }

  // 动态计算可用批量操作：只要选中单据中"至少有一张能做"就显示按钮
  // 执行时后端会跳过状态不匹配的，返回部分成功/失败
  const availableBatchOps = computed(() => {
    const rows = selectedRows.value
    if (rows.length === 0) return { delete: false, submit: false, approve: false, cancel: false }
    const hasDraft = rows.some(r => r.status === 'DRAFT')
    const hasSubmitted = rows.some(r => r.status === 'SUBMITTED')
    const hasCanCancel = rows.some(r => ['DRAFT', 'SUBMITTED', 'APPROVED'].includes(r.status))
    return {
      delete: hasDraft,
      submit: hasDraft,
      approve: hasSubmitted,
      cancel: hasCanCancel,
    }
  })

  // 是否存在可按批次删除的选中（全部来自同一次导入 + 都是 DRAFT）
  const singleImportBatch = computed(() => {
    const rows = selectedRows.value
    if (rows.length < 2) return null
    const taskId = rows[0].import_task_id
    if (!taskId) return null
    if (!rows.every(r => r.import_task_id === taskId && r.status === 'DRAFT')) return null
    return taskId
  })

  function showBatchResult(resp: BatchOperResult, action: string) {
    if (resp.fail === 0) {
      ElMessage.success(`${action}：全部成功 ${resp.success} 张`)
    } else {
      ElMessageBox.alert(
        `${action}完成：成功 ${resp.success} 张，失败 ${resp.fail} 张`,
        '批量操作结果',
        { type: 'warning' },
      )
    }
  }

  async function onBatchDelete() {
    const rows = selectedRows.value
    try {
      await ElMessageBox.confirm(`确定批量删除选中的 ${rows.length} 张草稿入库单吗？`, '批量删除', { type: 'warning', confirmButtonClass: 'el-button--danger' })
    } catch { return }
    const resp = await batchDeleteInboundOrders(rows.map(r => r.id))
    showBatchResult(resp, '批量删除')
    tableRef.value?.clearSelection()
    load()
    loadImports()
  }

  async function onBatchSubmit() {
    const rows = selectedRows.value
    try {
      await ElMessageBox.confirm(`确定批量提交选中的 ${rows.length} 张草稿入库单吗？`, '批量提交', { type: 'warning' })
    } catch { return }
    const resp = await batchSubmitInboundOrders(rows.map(r => r.id))
    showBatchResult(resp, '批量提交')
    tableRef.value?.clearSelection()
    load()
  }

  async function onBatchApprove() {
    const rows = selectedRows.value
    try {
      await ElMessageBox.confirm(`确定批量审核选中的 ${rows.length} 张入库单吗？`, '批量审核', { type: 'warning' })
    } catch { return }
    const resp = await batchApproveInboundOrders(rows.map(r => r.id))
    showBatchResult(resp, '批量审核')
    tableRef.value?.clearSelection()
    load()
  }

  async function onBatchCancel() {
    const rows = selectedRows.value
    try {
      await ElMessageBox.confirm(`确定批量作废选中的 ${rows.length} 张入库单吗？`, '批量作废', { type: 'warning', confirmButtonClass: 'el-button--danger' })
    } catch { return }
    const resp = await batchCancelInboundOrders(rows.map(r => r.id))
    showBatchResult(resp, '批量作废')
    tableRef.value?.clearSelection()
    load()
  }

  async function onBatchDeleteByTask(taskId: string) {
    try {
      await ElMessageBox.confirm(`确定按批次号 ${taskId} 删除全部 DRAFT 入库单吗？`, '按批次删除', { type: 'warning', confirmButtonClass: 'el-button--danger' })
    } catch { return }
    const resp = await deleteInboundByImportTask(taskId)
    showBatchResult(resp, `批次 ${taskId} 删除`)
    tableRef.value?.clearSelection()
    query.import_task_id = ''
    load()
    loadImports()
  }

  return {
    warehouseOptions,
    warehouseMap,
    skuOptions,
    importBatchOptions,
    loadOptions,
    loadImports,
    loading,
    list,
    total,
    query,
    dateRange,
    dateRangeDefaultTime,
    load,
    search,
    onDateRangeChange,
    resetSearch,
    onSubmit,
    onApprove,
    onCancel,
    onDelete,
    selectedRows,
    tableRef,
    onSelectionChange,
    availableBatchOps,
    singleImportBatch,
    onBatchCommand,
  }
}
