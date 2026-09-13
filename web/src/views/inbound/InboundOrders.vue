<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, genFileId } from 'element-plus'
import type { UploadFile, UploadRawFile } from 'element-plus'
import {
  approveInboundOrder,
  cancelInboundOrder,
  createInboundOrder,
  deleteInboundOrder,
  getImportStatus,
  getInboundOrder,
  importInboundExcel,
  listInboundOrders,
  submitInboundOrder,
  updateInboundOrder,
} from '@/api/inbound'
import type { ImportTaskItem, InboundOrderItem } from '@/api/types'
import { INBOUND_STATUS_OPTIONS, statusTag, statusText } from '@/constants'
import { cleanParams, formatTime } from '@/utils'
import { loadSkuMap, loadWarehouseOptions, toOptionMap, type IdOption } from '@/utils/options'
import ReceiveDialog from '@/components/ReceiveDialog.vue'
import PutawayDialog from '@/components/PutawayDialog.vue'

const router = useRouter()

// ---------- 基础选项 ----------
const warehouseOptions = ref<IdOption[]>([])
const warehouseMap = ref<Record<number, string>>({})
const skuOptions = ref<IdOption[]>([])

onMounted(async () => {
  warehouseOptions.value = await loadWarehouseOptions()
  warehouseMap.value = toOptionMap(warehouseOptions.value)
  const map = await loadSkuMap()
  skuOptions.value = Object.entries(map).map(([id, sku]) => ({
    id: Number(id),
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
  warehouse_id: '' as number | '',
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
    await ElMessageBox.confirm(`确定取消入库单「${row.order_no}」吗？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await cancelInboundOrder(row.id)
  ElMessage.success('已取消')
  load()
}

async function onDelete(row: InboundOrderItem) {
  try {
    await ElMessageBox.confirm(`确定删除入库单「${row.order_no}」吗？`, '提示', { type: 'warning' })
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

// ---------- 新建 / 编辑 ----------
const editDialog = reactive({ visible: false, loading: false, editingId: 0 })
const editForm = reactive({
  warehouse_id: undefined as number | undefined,
  remark: '',
  details: [] as { sku_id: number | undefined; expected_qty: number }[],
})

function openCreate() {
  editDialog.editingId = 0
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
      if (info.status === 'SUCCESS' || info.status === 'FAILED') {
        stopPolling()
        if (info.status === 'SUCCESS') {
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
      <el-button type="primary" @click="openCreate">新建入库单</el-button>
      <el-button type="success" plain @click="openImport">Excel 导入</el-button>
    </div>

    <el-table v-loading="loading" :data="list" border stripe>
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
      <el-table-column label="来源" width="90">
        <template #default="{ row }">{{ row.source === 'IMPORT' ? '导入' : '手动' }}</template>
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
              <el-button size="small" type="primary" plain @click="openEdit(row)">编辑</el-button>
              <el-button size="small" type="success" plain @click="onSubmit(row)">提交</el-button>
              <el-button size="small" type="danger" plain @click="onDelete(row)">删除</el-button>
            </template>
            <template v-else-if="row.status === 'SUBMITTED'">
              <el-button size="small" type="success" plain @click="onApprove(row)">审核</el-button>
              <el-button size="small" type="danger" plain @click="onCancel(row)">取消</el-button>
            </template>
            <template v-else-if="row.status === 'APPROVED' || row.status === 'RECEIVING'">
              <el-button size="small" type="primary" plain @click="openReceive(row)">收货</el-button>
              <el-button size="small" type="warning" plain @click="openPutaway(row)">上架</el-button>
            </template>
            <template v-else-if="row.status === 'PUTAWAY'">
              <el-button size="small" type="warning" plain @click="openPutaway(row)">上架</el-button>
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
        <el-button @click="editDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="editDialog.loading" @click="submitEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 收货 -->
    <ReceiveDialog ref="receiveRef" @success="load" />

    <!-- 上架 -->
    <PutawayDialog ref="putawayRef" @success="load" />

    <!-- Excel 导入 -->
    <el-dialog v-model="importDialog.visible" title="Excel 导入入库单" width="560px" @close="closeImport">
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
