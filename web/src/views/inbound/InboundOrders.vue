<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, genFileId } from 'element-plus'
import type { UploadFile, UploadRawFile } from 'element-plus'
import { ArrowDown, CloseBold, Delete, Files, Promotion, Select } from '@element-plus/icons-vue'
import {
  approveInboundOrder,
  batchApproveInboundOrders,
  batchCancelInboundOrders,
  batchDeleteInboundOrders,
  batchSubmitInboundOrders,
  cancelInboundOrder,
  createInboundOrder,
  deleteInboundByImportTask,
  deleteInboundOrder,
  getImportStatus,
  getInboundOrder,
  importInboundExcel,
  listInboundOrders,
  submitInboundOrder,
  updateInboundOrder,
} from '@/api/inbound'
import type { BatchOperResult, EntityID, ImportTaskItem, InboundOrderItem } from '@/api/types'
import { INBOUND_STATUS_OPTIONS, statusTag, statusText } from '@/constants'
import { cleanParams, formatTime } from '@/utils'
import { loadSkuMap, loadWarehouseOptions, toOptionMap, type IdOption } from '@/utils/options'
import ReceiveDialog from '@/components/ReceiveDialog.vue'
import PutawayDialog from '@/components/PutawayDialog.vue'

const router = useRouter()

// ---------- 基础选项 ----------
const warehouseOptions = ref<IdOption[]>([])
const warehouseMap = ref<Record<EntityID, string>>({})
const skuOptions = ref<IdOption[]>([])

onMounted(async () => {
  warehouseOptions.value = await loadWarehouseOptions()
  warehouseMap.value = toOptionMap(warehouseOptions.value)
  const map = await loadSkuMap()
  skuOptions.value = Object.entries(map).map(([id, sku]) => ({
    id,
    label: `${sku.code} ${sku.name}`,
  }))
  load()
})

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
})

async function load() {
  loading.value = true
  try {
    const data = await listInboundOrders(cleanParams({ ...query }))
    list.value = data.list ?? []
    total.value = data.total ?? 0
  } finally {
    loading.value = false
  }
}

function search() {
  query.page = 1
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
}

function goDetail(row: InboundOrderItem) {
  router.push(`/inbound/orders/${row.id}`)
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
  load()
}

// ---------- 新建 / 编辑 ----------
const editDialog = reactive({ visible: false, loading: false, editingId: '' as EntityID })
const editForm = reactive({
  warehouse_id: undefined as EntityID | undefined,
  remark: '',
  details: [] as { sku_id: EntityID | undefined; expected_qty: number }[],
})

function openCreate() {
  editDialog.editingId = ''
  editForm.warehouse_id = undefined
  editForm.remark = ''
  editForm.details = [{ sku_id: undefined, expected_qty: 1 }]
  editDialog.visible = true
}

async function openEdit(row: InboundOrderItem) {
  editDialog.editingId = row.id
  editDialog.visible = true
  const detail = await getInboundOrder(row.id)
  editForm.warehouse_id = detail.order.warehouse_id
  editForm.remark = detail.order.remark
  editForm.details = (detail.details ?? []).map((d) => ({ sku_id: d.sku_id, expected_qty: d.expected_qty }))
  if (editForm.details.length === 0) editForm.details = [{ sku_id: undefined, expected_qty: 1 }]
}

function addDetail() {
  editForm.details.push({ sku_id: undefined, expected_qty: 1 })
}

function removeDetail(index: number) {
  editForm.details.splice(index, 1)
}

async function submitEdit() {
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
  editDialog.loading = true
  try {
    if (editDialog.editingId) {
      await updateInboundOrder(editDialog.editingId, payload)
      ElMessage.success('保存成功')
    } else {
      await createInboundOrder(payload)
      ElMessage.success('创建成功')
    }
    editDialog.visible = false
    load()
  } finally {
    editDialog.loading = false
  }
}

// ---------- 收货 / 上架 ----------
const receiveRef = ref<InstanceType<typeof ReceiveDialog>>()
const putawayRef = ref<InstanceType<typeof PutawayDialog>>()

function openReceive(row: InboundOrderItem) {
  receiveRef.value?.open(row.id)
}

function openPutaway(row: InboundOrderItem) {
  putawayRef.value?.open(row.id)
}

// ---------- Excel 导入 ----------
const importDialog = reactive({ visible: false, uploading: false })
const importFile = ref<File | null>(null)
const importInfo = ref<ImportTaskItem | null>(null)
let pollTimer = 0

function onFileChange(file: UploadFile) {
  importFile.value = (file.raw as File) ?? null
}

function onFileRemove() {
  importFile.value = null
}

function handleExceed(files: File[]) {
  const raw = files[0] as UploadRawFile
  raw.uid = genFileId()
  importFile.value = raw as unknown as File
}

function stopPolling() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
    pollTimer = 0
  }
}

function openImport() {
  importFile.value = null
  importInfo.value = null
  importDialog.visible = true
}

function closeImport() {
  stopPolling()
  importDialog.visible = false
}

async function startImport() {
  if (!importFile.value) {
    ElMessage.warning('请先选择 Excel 文件')
    return
  }
  importDialog.uploading = true
  importInfo.value = null
  try {
    const resp = await importInboundExcel(importFile.value)
    ElMessage.success('文件已上传，开始解析导入')
    importInfo.value = {
      task_id: resp.task_id,
      status: 'PENDING',
      file_name: importFile.value.name,
      total_rows: 0,
      success_rows: 0,
      fail_rows: 0,
      error_msg: '',
    }
    startPolling(resp.task_id)
  } finally {
    importDialog.uploading = false
  }
}

function startPolling(taskId: string) {
  stopPolling()
  pollTimer = window.setInterval(async () => {
    try {
      const info = await getImportStatus(taskId)
      importInfo.value = info
      if (info.status === 'COMPLETED' || info.status === 'FAILED') {
        stopPolling()
        if (info.status === 'COMPLETED') {
          ElMessage.success(`导入完成：成功 ${info.success_rows} 条，失败 ${info.fail_rows} 条`)
        }
        load()
      }
    } catch {
      stopPolling()
    }
  }, 2000)
}

onUnmounted(stopPolling)
</script>

<template>
  <div class="page-card">
    <el-form inline class="query-form" @submit.prevent="search">
      <el-form-item label="仓库">
        <el-select v-model="query.warehouse_id" placeholder="全部" clearable style="width: 200px" @change="search">
          <el-option v-for="w in warehouseOptions" :key="w.id" :label="w.label" :value="w.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="query.status" placeholder="全部" clearable style="width: 130px" @change="search">
          <el-option v-for="s in INBOUND_STATUS_OPTIONS" :key="s" :label="statusText(s)" :value="s" />
        </el-select>
      </el-form-item>
      <el-form-item label="单号">
        <el-input v-model="query.keyword" placeholder="单号模糊搜索" clearable style="width: 180px" @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
      </el-form-item>
    </el-form>

    <div class="toolbar">
      <el-button v-permission="'wms:inbound:create'" type="primary" @click="openCreate">新建入库单</el-button>
      <el-button v-permission="'wms:inbound:create'" type="success" plain @click="openImport">Excel 导入</el-button>
      <el-dropdown trigger="click" @command="onBatchCommand">
        <el-button plain>
          批量操作
          <el-icon class="el-icon--right"><arrow-down /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="delete" :disabled="!availableBatchOps.delete">
              <el-icon><Delete /></el-icon>批量删除<span v-if="availableBatchOps.delete" class="badge-hint">(DRAFT)</span>
            </el-dropdown-item>
            <el-dropdown-item command="submit" :disabled="!availableBatchOps.submit">
              <el-icon><Promotion /></el-icon>批量提交<span v-if="availableBatchOps.submit" class="badge-hint">(DRAFT)</span>
            </el-dropdown-item>
            <el-dropdown-item command="approve" :disabled="!availableBatchOps.approve">
              <el-icon><Select /></el-icon>批量审核<span v-if="availableBatchOps.approve" class="badge-hint">(SUBMITTED)</span>
            </el-dropdown-item>
            <el-dropdown-item command="cancel" :disabled="!availableBatchOps.cancel">
              <el-icon><CloseBold /></el-icon>批量作废
            </el-dropdown-item>
            <el-dropdown-item v-if="singleImportBatch" command="batch-by-task" :divider="true">
              <el-icon><Files /></el-icon>按批次删除（{{ singleImportBatch }}）
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <span v-if="selectedRows.length > 0" class="selected-hint">已选 {{ selectedRows.length }} 项</span>
    </div>

    <el-alert
      v-if="selectedRows.length > 0"
      type="info"
      class="batch-alert"
      :closable="false"
      show-icon
    >
      <template #title>
        <span>已勾选 <strong>{{ selectedRows.length }}</strong> 张入库单</span>
        <span class="batch-alert-actions">
          <el-button v-if="availableBatchOps.delete" link type="danger" @click="onBatchDelete">删除</el-button>
          <el-button v-if="availableBatchOps.submit" link type="success" @click="onBatchSubmit">提交</el-button>
          <el-button v-if="availableBatchOps.approve" link type="primary" @click="onBatchApprove">审核</el-button>
          <el-button v-if="availableBatchOps.cancel" link type="danger" @click="onBatchCancel">作废</el-button>
          <el-button link @click="tableRef?.clearSelection()">清除选择</el-button>
        </span>
      </template>
    </el-alert>

    <el-table ref="tableRef" v-loading="loading" :data="list" border stripe @selection-change="onSelectionChange">
      <el-table-column type="selection" width="42" />
      <el-table-column prop="order_no" label="入库单号" min-width="170">
        <template #default="{ row }">
          <el-link type="primary" @click="goDetail(row)">{{ row.order_no }}</el-link>
        </template>
      </el-table-column>
      <el-table-column label="仓库" min-width="150">
        <template #default="{ row }">{{ warehouseMap[row.warehouse_id] || row.warehouse_id }}</template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="来源" width="130">
        <template #default="{ row }">
          <template v-if="row.source === 'IMPORT'">
            <el-tag type="info" size="small">导入</el-tag>
            <div class="task-id" v-if="row.import_task_id">{{ row.import_task_id }}</div>
          </template>
          <span v-else>手动</span>
        </template>
      </el-table-column>
      <el-table-column prop="expected_qty" label="应收数量" width="100" align="right" />
      <el-table-column prop="received_qty" label="已收数量" width="100" align="right" />
      <el-table-column prop="defective_qty" label="不良品" width="90" align="right" />
      <el-table-column prop="remark" label="备注" min-width="120" show-overflow-tooltip />
      <el-table-column prop="created_by" label="创建人" width="100" />
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="260" fixed="right">
        <template #default="{ row }">
          <div class="table-oper">
            <el-button size="small" @click="goDetail(row)">详情</el-button>
            <template v-if="row.status === 'DRAFT'">
              <el-button v-permission="'wms:inbound:create'" size="small" type="primary" plain @click="openEdit(row)">编辑</el-button>
              <el-button v-permission="'wms:inbound:submit'" size="small" type="success" plain @click="onSubmit(row)">提交</el-button>
              <el-button v-permission="'wms:inbound:create'" size="small" type="danger" plain @click="onDelete(row)">删除</el-button>
            </template>
            <template v-else-if="row.status === 'SUBMITTED'">
              <el-button v-permission="'wms:inbound:approve'" size="small" type="success" plain @click="onApprove(row)">审核</el-button>
              <el-button v-permission="'wms:inbound:cancel'" size="small" type="danger" plain @click="onCancel(row)">取消</el-button>
            </template>
            <template v-else-if="row.status === 'APPROVED' || row.status === 'RECEIVING'">
              <el-button v-permission="'wms:inbound:receive'" size="small" type="primary" plain @click="openReceive(row)">收货</el-button>
              <el-button v-permission="'wms:inbound:putaway'" size="small" type="warning" plain @click="openPutaway(row)">上架</el-button>
            </template>
            <template v-else-if="row.status === 'PUTAWAY'">
              <el-button v-permission="'wms:inbound:putaway'" size="small" type="warning" plain @click="openPutaway(row)">上架</el-button>
            </template>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="query.page"
      v-model:page-size="query.page_size"
      class="pagination"
      layout="total, sizes, prev, pager, next, jumper"
      :total="total"
      :page-sizes="[10, 20, 50]"
      @current-change="load"
      @size-change="search"
    />

    <!-- 新建 / 编辑 -->
    <el-dialog
      v-model="editDialog.visible"
      :title="editDialog.editingId ? '编辑入库单' : '新建入库单'"
      width="720px"
      destroy-on-close
      :close-on-click-modal="false"
      :close-on-press-escape="!editDialog.loading"
      :show-close="!editDialog.loading"
    >
      <el-form label-width="90px">
        <el-form-item label="仓库" required>
          <el-select v-model="editForm.warehouse_id" placeholder="选择仓库" style="width: 300px">
            <el-option v-for="w in warehouseOptions" :key="w.id" :label="w.label" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="editForm.remark" type="textarea" :rows="2" placeholder="备注（可选）" />
        </el-form-item>
        <el-form-item label="明细" required>
          <div class="detail-editor">
            <div v-for="(item, index) in editForm.details" :key="index" class="detail-row">
              <el-select v-model="item.sku_id" placeholder="选择货品" filterable style="width: 320px">
                <el-option v-for="s in skuOptions" :key="s.id" :label="s.label" :value="s.id" />
              </el-select>
              <el-input-number v-model="item.expected_qty" :min="1" controls-position="right" style="width: 140px" />
              <el-button type="danger" plain circle size="small" @click="removeDetail(index)">
                <el-icon><Delete /></el-icon>
              </el-button>
            </div>
            <el-button type="primary" plain size="small" @click="addDetail">+ 添加明细行</el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="editDialog.loading" @click="editDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="editDialog.loading" @click="submitEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 收货 -->
    <ReceiveDialog ref="receiveRef" @success="load" />

    <!-- 上架 -->
    <PutawayDialog ref="putawayRef" @success="load" />

    <!-- Excel 导入 -->
    <el-dialog
      v-model="importDialog.visible"
      title="Excel 导入入库单"
      width="560px"
      :close-on-click-modal="false"
      :close-on-press-escape="!importDialog.uploading"
      :show-close="!importDialog.uploading"
      @close="closeImport"
    >
      <el-upload
        drag
        accept=".xlsx,.xls"
        :auto-upload="false"
        :limit="1"
        :on-change="onFileChange"
        :on-remove="onFileRemove"
        :on-exceed="handleExceed"
      >
        <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
        <div class="el-upload__text">拖拽文件到此处，或 <em>点击选择文件</em></div>
        <template #tip>
          <div class="el-upload__tip">支持 .xlsx / .xls，上传后自动解析创建入库单，可在此查看导入进度。</div>
        </template>
      </el-upload>

      <div class="import-actions">
        <el-button type="primary" :loading="importDialog.uploading" @click="startImport">开始导入</el-button>
      </div>

      <el-descriptions v-if="importInfo" :column="2" border size="small" class="import-progress">
        <el-descriptions-item label="任务状态">
          <el-tag :type="statusTag(importInfo.status)" size="small">{{ statusText(importInfo.status) }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="文件名">{{ importInfo.file_name || importFile?.name || '-' }}</el-descriptions-item>
        <el-descriptions-item label="总行数">{{ importInfo.total_rows }}</el-descriptions-item>
        <el-descriptions-item label="成功 / 失败">{{ importInfo.success_rows }} / {{ importInfo.fail_rows }}</el-descriptions-item>
        <el-descriptions-item v-if="importInfo.error_msg" label="错误信息" :span="2">
          <span class="import-error">{{ importInfo.error_msg }}</span>
        </el-descriptions-item>
      </el-descriptions>
      <div v-else class="import-waiting">上传并点击“开始导入”后，此处每 2 秒刷新导入进度。</div>
    </el-dialog>
  </div>
</template>

<style scoped>
.selected-hint {
  margin-left: 12px;
  font-size: 13px;
  color: var(--el-color-primary);
  font-weight: 500;
}

.batch-alert {
  margin-bottom: 12px;
}

.batch-alert-actions {
  margin-left: 16px;
  display: inline-flex;
  gap: 4px;
}

.badge-hint {
  margin-left: 4px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.task-id {
  font-size: 11px;
  color: var(--el-text-color-secondary);
  font-family: monospace;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-editor {
  width: 100%;
}

.detail-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.import-actions {
  margin-top: 12px;
}

.import-progress {
  margin-top: 16px;
}

.import-error {
  color: var(--el-color-danger);
}

.import-waiting {
  margin-top: 16px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
