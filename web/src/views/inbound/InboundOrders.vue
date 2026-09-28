<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAutoRefresh } from '@/composables/autoRefresh'
import { useRouter } from 'vue-router'
import { ArrowDown, CloseBold, Delete, Files, Promotion, Select } from '@element-plus/icons-vue'
import { useInboundImport } from '@/composables/inbound/useInboundImport'
import { useInboundOrderForm } from '@/composables/inbound/useInboundOrderForm'
import { useInboundOrders } from '@/composables/inbound/useInboundOrders'
import type { InboundOrderItem } from '@/api/types'
import { INBOUND_STATUS_OPTIONS, statusTag, statusText } from '@/constants'
import { formatTime } from '@/utils'
import PageHeader from '@/components/common/PageHeader.vue'
import ReceiveDialog from '@/components/ReceiveDialog.vue'
import PutawayDialog from '@/components/PutawayDialog.vue'

const router = useRouter()

const {
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
} = useInboundOrders()

const { editDialog, editForm, openCreate, openEdit, addDetail, removeDetail, submitEdit } = useInboundOrderForm(load)

const {
  importDialog,
  importFile,
  importInfo,
  openImport,
  closeImport,
  startImport,
  onFileChange,
  onFileRemove,
  handleExceed,
} = useInboundImport(load)

// ---------- 收货 / 上架 ----------
const receiveRef = ref<InstanceType<typeof ReceiveDialog>>()
const putawayRef = ref<InstanceType<typeof PutawayDialog>>()

function openReceive(row: InboundOrderItem) {
  receiveRef.value?.open(row.id)
}

function openPutaway(row: InboundOrderItem) {
  putawayRef.value?.open(row.id)
}

function goDetail(row: InboundOrderItem) {
  router.push(`/inbound/orders/${row.id}`)
}

onMounted(async () => {
  await loadOptions()
  await loadImports()
  load()
})

useAutoRefresh(() => load(true), 0, () => selectedRows.value.length === 0)
</script>

<template>
  <div class="app-page order-page">
    <PageHeader title="入库管理" description="管理入库单创建、审核、收货和上架，查看真实业务状态。">
      <template #actions>
        <el-button v-permission="'wms:inbound:create'" data-tour="inbound-create" type="primary" @click="openCreate">新建入库单</el-button>
      </template>
    </PageHeader>

    <section class="app-card app-card--flush">
    <el-form inline class="query-form app-filter-bar order-filters" @submit.prevent="search">
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
      <el-form-item label="创建时间">
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          value-format="YYYY-MM-DD HH:mm:ss"
          :default-time="dateRangeDefaultTime"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          unlink-panels
          style="width: 250px"
          @change="onDateRangeChange"
        />
      </el-form-item>
      <el-form-item label="批次号">
        <el-select v-model="query.import_task_id" placeholder="全部" clearable filterable style="width: 320px" @change="search">
          <el-option
            v-for="b in importBatchOptions"
            :key="b.task_id"
            :label="b.label"
            :value="b.task_id"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="单号">
        <el-input v-model="query.keyword" placeholder="单号模糊搜索" clearable style="width: 180px" @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
        <el-button @click="resetSearch">重置</el-button>
      </el-form-item>
    </el-form>

    <div class="app-table-region">
      <div class="app-toolbar">
        <div class="app-toolbar__actions">
          <el-button v-permission="'wms:inbound:create'" type="success" plain @click="openImport">Excel 导入</el-button>
        </div>
        <div class="app-toolbar__actions">
          <span class="selected-hint" :class="{ 'is-hidden': selectedRows.length === 0 }">已选 {{ selectedRows.length }} 项</span>
          <el-dropdown trigger="click" @command="onBatchCommand">
            <el-button plain>
              批量操作
              <el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="delete" :disabled="!availableBatchOps.delete"><el-icon><Delete /></el-icon>批量删除</el-dropdown-item>
                <el-dropdown-item command="submit" :disabled="!availableBatchOps.submit"><el-icon><Promotion /></el-icon>批量提交</el-dropdown-item>
                <el-dropdown-item command="approve" :disabled="!availableBatchOps.approve"><el-icon><Select /></el-icon>批量审核</el-dropdown-item>
                <el-dropdown-item command="cancel" :disabled="!availableBatchOps.cancel"><el-icon><CloseBold /></el-icon>批量作废</el-dropdown-item>
                <el-dropdown-item command="batch-by-task" :disabled="!singleImportBatch" divided><el-icon><Files /></el-icon>按批次删除</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button link :disabled="selectedRows.length === 0" @click="tableRef?.clearSelection()">清除选择</el-button>
        </div>
      </div>

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
      <el-table-column label="数量" width="130" align="right">
        <template #default="{ row }"><span class="quantity-main">{{ row.received_qty }} / {{ row.expected_qty }}</span><small v-if="row.defective_qty" class="quantity-note">不良 {{ row.defective_qty }}</small></template>
      </el-table-column>
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

      </div>

    <!-- 新建 / 编辑 -->
    <el-dialog
      v-model="editDialog.visible"
      :title="editDialog.editingId ? '编辑入库单' : '新建入库单'"
      width="720px"
      destroy-on-close
      :close-on-click-modal="true"
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
            <div v-for="(item, index) in editForm.details" :key="item._uid" class="detail-row">
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
      :close-on-click-modal="!importDialog.uploading"
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
    </section>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.selected-hint {
  font-size: 13px;
  color: var(--el-color-primary);
  font-weight: 500;
  white-space: nowrap;
  min-width: 62px;
  text-align: right;
}

.selected-hint.is-hidden {
  visibility: hidden;
  pointer-events: none;
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
.quantity-main {
  display: block;
  color: var(--el-text-color-primary);
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}

.quantity-note {
  display: block;
  margin-top: 2px;
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

</style>
