<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, CloseBold, Delete, Promotion, Select } from '@element-plus/icons-vue'
import {
  approveOutboundOrder,
  batchApproveOutboundOrders,
  batchCancelOutboundOrders,
  batchDeleteOutboundOrders,
  batchSubmitOutboundOrders,
  cancelOutboundOrder,
  createOutboundOrder,
  deleteOutboundOrder,
  listOutboundOrders,
  submitOutboundOrder,
} from '@/api/outbound'
import type { BatchOperResult, EntityID, OutboundOrderItem } from '@/api/types'
import { OUTBOUND_STATUS_OPTIONS, statusTag, statusText } from '@/constants'
import { cleanParams, formatTime } from '@/utils'
import { loadSkuMap, loadWarehouseOptions, toOptionMap, type IdOption } from '@/utils/options'
import PickDialog from '@/components/PickDialog.vue'

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
const list = ref<OutboundOrderItem[]>([])
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
    const data = await listOutboundOrders(cleanParams({ ...query }))
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
async function onSubmit(row: OutboundOrderItem) {
  try {
    await ElMessageBox.confirm(`确定提交出库单「${row.order_no}」吗？`, '提示', { type: 'warning' })
  } catch { return }
  await submitOutboundOrder(row.id)
  ElMessage.success('提交成功')
  load()
}

async function onApprove(row: OutboundOrderItem) {
  try {
    await ElMessageBox.confirm(`确定审核（分配库存）出库单「${row.order_no}」吗？`, '提示', { type: 'warning' })
  } catch { return }
  await approveOutboundOrder(row.id)
  ElMessage.success('审核完成，库存已分配')
  load()
}

async function onCancel(row: OutboundOrderItem) {
  try {
    await ElMessageBox.confirm(
      `确定取消出库单「${row.order_no}」吗？取消后将释放已分配库存。`,
      '取消出库单', { type: 'warning', confirmButtonText: '确认取消', cancelButtonText: '返回', confirmButtonClass: 'el-button--danger' },
    )
  } catch { return }
  await cancelOutboundOrder(row.id)
  ElMessage.success('已取消')
  load()
}

async function onDelete(row: OutboundOrderItem) {
  try {
    await ElMessageBox.confirm(
      `确定删除草稿出库单「${row.order_no}」吗？删除后不可恢复。`,
      '删除出库单', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '返回', confirmButtonClass: 'el-button--danger' },
    )
  } catch { return }
  await deleteOutboundOrder(row.id)
  ElMessage.success('删除成功')
  load()
}

function goDetail(row: OutboundOrderItem) {
  router.push(`/outbound/orders/${row.id}`)
}

// ---------- 批量操作 ----------
const selectedRows = ref<OutboundOrderItem[]>([])
const tableRef = ref()

function onSelectionChange(rows: OutboundOrderItem[]) {
  selectedRows.value = rows
}

function onBatchCommand(cmd: string) {
  if (cmd === 'delete') onBatchDelete()
  else if (cmd === 'submit') onBatchSubmit()
  else if (cmd === 'approve') onBatchApprove()
  else if (cmd === 'cancel') onBatchCancel()
}

// 动态计算可用批量操作：只要"至少有一张能做"就显示按钮
// 执行时后端会跳过状态不匹配的，返回部分成功/失败
const availableBatchOps = computed(() => {
  const rows = selectedRows.value
  if (rows.length === 0) return { delete: false, submit: false, approve: false, cancel: false }
  const hasDraft = rows.some(r => r.status === 'DRAFT')
  const hasSubmitted = rows.some(r => r.status === 'SUBMITTED')
  // 作废允许 DRAFT / SUBMITTED / PICKING（未实际拣货）
  const hasCanCancel = rows.some(r => ['DRAFT', 'SUBMITTED', 'PICKING'].includes(r.status))
  return { delete: hasDraft, submit: hasDraft, approve: hasSubmitted, cancel: hasCanCancel }
})

function showBatchResult(resp: BatchOperResult, action: string) {
  if (resp.fail === 0) {
    ElMessage.success(`${action}：全部成功 ${resp.success} 张`)
  } else {
    ElMessageBox.alert(`${action}完成：成功 ${resp.success} 张，失败 ${resp.fail} 张`, '批量操作结果', { type: 'warning' })
  }
}

async function onBatchDelete() {
  const rows = selectedRows.value
  try {
    await ElMessageBox.confirm(`确定批量删除选中的 ${rows.length} 张草稿出库单吗？`, '批量删除', { type: 'warning', confirmButtonClass: 'el-button--danger' })
  } catch { return }
  const resp = await batchDeleteOutboundOrders(rows.map(r => r.id))
  showBatchResult(resp, '批量删除')
  tableRef.value?.clearSelection()
  load()
}

async function onBatchSubmit() {
  const rows = selectedRows.value
  try {
    await ElMessageBox.confirm(`确定批量提交选中的 ${rows.length} 张草稿出库单吗？`, '批量提交', { type: 'warning' })
  } catch { return }
  const resp = await batchSubmitOutboundOrders(rows.map(r => r.id))
  showBatchResult(resp, '批量提交')
  tableRef.value?.clearSelection()
  load()
}

async function onBatchApprove() {
  const rows = selectedRows.value
  try {
    await ElMessageBox.confirm(`确定批量审核选中的 ${rows.length} 张出库单吗？审核将分配库存。`, '批量审核', { type: 'warning' })
  } catch { return }
  const resp = await batchApproveOutboundOrders(rows.map(r => r.id))
  showBatchResult(resp, '批量审核')
  tableRef.value?.clearSelection()
  load()
}

async function onBatchCancel() {
  const rows = selectedRows.value
  try {
    await ElMessageBox.confirm(`确定批量作废选中的 ${rows.length} 张出库单吗？已分配的库存将被释放。`, '批量作废', { type: 'warning', confirmButtonClass: 'el-button--danger' })
  } catch { return }
  const resp = await batchCancelOutboundOrders(rows.map(r => r.id))
  showBatchResult(resp, '批量作废')
  tableRef.value?.clearSelection()
  load()
}

// ---------- 新建 ----------
const createDialog = reactive({ visible: false, loading: false })
const createForm = reactive({
  warehouse_id: undefined as EntityID | undefined,
  biz_order_no: '',
  remark: '',
  details: [] as { sku_id: EntityID | undefined; expected_qty: number }[],
})

function openCreate() {
  createForm.warehouse_id = undefined
  createForm.biz_order_no = ''
  createForm.remark = ''
  createForm.details = [{ sku_id: undefined, expected_qty: 1 }]
  createDialog.visible = true
}

function addDetail() {
  createForm.details.push({ sku_id: undefined, expected_qty: 1 })
}

function removeDetail(index: number) {
  createForm.details.splice(index, 1)
}

async function submitCreate() {
  if (!createForm.warehouse_id) { ElMessage.warning('请选择仓库'); return }
  if (!createForm.biz_order_no.trim()) { ElMessage.warning('请输入业务订单号'); return }
  const details = createForm.details.filter((d) => d.sku_id && d.expected_qty > 0)
  if (details.length === 0) { ElMessage.warning('请至少填写一行有效的明细'); return }
  createDialog.loading = true
  try {
    await createOutboundOrder({
      warehouse_id: createForm.warehouse_id,
      biz_order_no: createForm.biz_order_no.trim(),
      remark: createForm.remark,
      details: details.map((d) => ({ sku_id: d.sku_id!, expected_qty: d.expected_qty })),
    })
    ElMessage.success('创建成功')
    createDialog.visible = false
    load()
  } finally {
    createDialog.loading = false
  }
}

// ---------- 拣货 ----------
const pickRef = ref<InstanceType<typeof PickDialog>>()

function openPick(row: OutboundOrderItem) {
  pickRef.value?.open(row.id)
}
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
          <el-option v-for="s in OUTBOUND_STATUS_OPTIONS" :key="s" :label="statusText(s)" :value="s" />
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
      <el-button v-permission="'wms:outbound:create'" type="primary" @click="openCreate">新建出库单</el-button>
      <el-dropdown trigger="click" @command="onBatchCommand">
        <el-button plain>
          批量操作
          <el-icon class="el-icon--right"><arrow-down /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="delete" :disabled="!availableBatchOps.delete">
              <el-icon><Delete /></el-icon>批量删除
            </el-dropdown-item>
            <el-dropdown-item command="submit" :disabled="!availableBatchOps.submit">
              <el-icon><Promotion /></el-icon>批量提交
            </el-dropdown-item>
            <el-dropdown-item command="approve" :disabled="!availableBatchOps.approve">
              <el-icon><Select /></el-icon>批量审核
            </el-dropdown-item>
            <el-dropdown-item command="cancel" :disabled="!availableBatchOps.cancel">
              <el-icon><CloseBold /></el-icon>批量作废
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
        <span>已勾选 <strong>{{ selectedRows.length }}</strong> 张出库单</span>
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
      <el-table-column prop="order_no" label="出库单号" min-width="170">
        <template #default="{ row }">
          <el-link type="primary" @click="goDetail(row)">{{ row.order_no }}</el-link>
        </template>
      </el-table-column>
      <el-table-column prop="biz_order_no" label="业务订单号" min-width="150" show-overflow-tooltip />
      <el-table-column label="仓库" min-width="150">
        <template #default="{ row }">{{ warehouseMap[row.warehouse_id] || row.warehouse_id }}</template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="expected_qty" label="需求数量" width="100" align="right" />
      <el-table-column prop="allocated_qty" label="已分配" width="90" align="right" />
      <el-table-column prop="picked_qty" label="已拣货" width="90" align="right" />
      <el-table-column prop="remark" label="备注" min-width="120" show-overflow-tooltip />
      <el-table-column prop="created_by" label="创建人" width="100" />
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="250" fixed="right">
        <template #default="{ row }">
          <div class="table-oper">
            <el-button size="small" @click="goDetail(row)">详情</el-button>
            <template v-if="row.status === 'DRAFT'">
              <el-button v-permission="'wms:outbound:submit'" size="small" type="success" plain @click="onSubmit(row)">提交</el-button>
              <el-button v-permission="'wms:outbound:create'" size="small" type="danger" plain @click="onDelete(row)">删除</el-button>
            </template>
            <template v-else-if="row.status === 'SUBMITTED'">
              <el-button v-permission="'wms:outbound:approve'" size="small" type="success" plain @click="onApprove(row)">审核</el-button>
              <el-button v-permission="'wms:outbound:cancel'" size="small" type="danger" plain @click="onCancel(row)">取消</el-button>
            </template>
            <template v-else-if="row.status === 'APPROVED' || row.status === 'PICKING'">
              <el-button v-permission="'wms:outbound:pick'" size="small" type="primary" plain @click="openPick(row)">拣货</el-button>
              <el-button v-permission="'wms:outbound:cancel'" size="small" type="danger" plain @click="onCancel(row)">取消</el-button>
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

    <!-- 新建 -->
    <el-dialog
      v-model="createDialog.visible"
      title="新建出库单"
      width="760px"
      destroy-on-close
      :close-on-click-modal="false"
      :close-on-press-escape="!createDialog.loading"
      :show-close="!createDialog.loading"
    >
      <el-form label-width="90px">
        <el-form-item label="仓库" required>
          <el-select v-model="createForm.warehouse_id" placeholder="选择仓库" style="width: 300px">
            <el-option v-for="w in warehouseOptions" :key="w.id" :label="w.label" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="业务订单号" required>
          <el-input v-model="createForm.biz_order_no" placeholder="业务订单号（幂等键，重复将创建失败）" style="width: 300px" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="createForm.remark" type="textarea" :rows="2" placeholder="备注（可选）" />
        </el-form-item>
        <el-form-item label="明细" required>
          <div class="detail-editor">
            <div v-for="(item, index) in createForm.details" :key="index" class="detail-row">
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
        <el-button :disabled="createDialog.loading" @click="createDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="createDialog.loading" @click="submitCreate">保存</el-button>
      </template>
    </el-dialog>

    <!-- 拣货 -->
    <PickDialog ref="pickRef" @success="load" />
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

.detail-editor {
  width: 100%;
}

.detail-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
</style>
