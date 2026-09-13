<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import {
  batchCreateLocations,
  deleteLocation,
  listLocations,
  listWarehouses,
  updateLocationStatus,
} from '@/api/basic'
import type { LocationItem, WarehouseItem } from '@/api/types'
import { COMMON_STATUS, LOCATION_STATUS, locationStatusTag, locationStatusText } from '@/constants'
import { cleanParams, formatTime } from '@/utils'

// ---------- 仓库下拉 ----------
const warehouseOptions = ref<WarehouseItem[]>([])
const warehouseMap = ref<Record<number, string>>({})

async function loadWarehouses() {
  const data = await listWarehouses({ page: 1, page_size: 100, status: COMMON_STATUS.ENABLED })
  warehouseOptions.value = data.list ?? []
  const map: Record<number, string> = {}
  for (const w of warehouseOptions.value) {
    map[w.id] = `${w.code}（${w.name}）`
  }
  warehouseMap.value = map
}

// ---------- 列表 ----------
const loading = ref(false)
const list = ref<LocationItem[]>([])
const total = ref(0)
const query = reactive({
  page: 1,
  page_size: 10,
  warehouse_id: '' as number | '',
  zone: '',
  code: '',
  status: '' as number | '',
})

async function load() {
  if (query.warehouse_id === '' ) {
    ElMessage.warning('请先选择仓库')
    return
  }
  loading.value = true
  try {
    const data = await listLocations(cleanParams({ ...query }))
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

onMounted(async () => {
  await loadWarehouses()
  if (warehouseOptions.value.length > 0) {
    query.warehouse_id = warehouseOptions.value[0]!.id
  }
  load()
})

// ---------- 批量生成 ----------
const batchDialog = reactive({ visible: false, loading: false })
const batchFormRef = ref<FormInstance>()
const batchForm = reactive({
  warehouse_id: undefined as number | undefined,
  zone: '',
  row_from: 1,
  row_to: 1,
  col_from: 1,
  col_to: 1,
})

const batchRules: FormRules = {
  warehouse_id: [{ required: true, message: '请选择仓库', trigger: 'change' }],
  zone: [{ required: true, message: '请输入库区', trigger: 'blur' }],
}

function openBatch() {
  batchForm.warehouse_id = query.warehouse_id === '' ? undefined : query.warehouse_id
  batchForm.zone = ''
  batchForm.row_from = 1
  batchForm.row_to = 1
  batchForm.col_from = 1
  batchForm.col_to = 1
  batchDialog.visible = true
}

async function submitBatch() {
  const valid = await batchFormRef.value?.validate().catch(() => false)
  if (!valid) return
  if (!batchForm.warehouse_id) return
  if (
    batchForm.row_from < 1 ||
    batchForm.row_to < batchForm.row_from ||
    batchForm.col_from < 1 ||
    batchForm.col_to < batchForm.col_from
  ) {
    ElMessage.warning('请检查排/列范围：结束值不能小于起始值')
    return
  }
  batchDialog.loading = true
  try {
    await batchCreateLocations({
      warehouse_id: batchForm.warehouse_id,
      zone: batchForm.zone,
      row_from: batchForm.row_from,
      row_to: batchForm.row_to,
      col_from: batchForm.col_from,
      col_to: batchForm.col_to,
    })
    ElMessage.success('批量生成完成')
    batchDialog.visible = false
    load()
  } finally {
    batchDialog.loading = false
  }
}

// ---------- 状态 / 删除 ----------
async function onToggleStatus(row: LocationItem, value: string | number | boolean) {
  const status = value ? LOCATION_STATUS.IDLE : LOCATION_STATUS.DISABLED
  try {
    await updateLocationStatus(row.id, status)
    ElMessage.success('状态已更新')
  } finally {
    load()
  }
}

async function onDelete(row: LocationItem) {
  try {
    await ElMessageBox.confirm(`确定删除库位「${row.code}」吗？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await deleteLocation(row.id)
  ElMessage.success('删除成功')
  load()
}
</script>

<template>
  <div class="page-card">
    <el-form inline class="query-form" @submit.prevent="search">
      <el-form-item label="仓库">
        <el-select
          v-model="query.warehouse_id"
          placeholder="选择仓库"
          style="width: 220px"
          @change="search"
        >
          <el-option v-for="w in warehouseOptions" :key="w.id" :label="`${w.code}（${w.name}）`" :value="w.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="库区">
        <el-input v-model="query.zone" placeholder="库区" clearable style="width: 120px" @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item label="编码">
        <el-input v-model="query.code" placeholder="库位编码" clearable style="width: 150px" @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="query.status" placeholder="全部" clearable style="width: 110px" @change="search">
          <el-option label="禁用" :value="LOCATION_STATUS.DISABLED" />
          <el-option label="空闲" :value="LOCATION_STATUS.IDLE" />
          <el-option label="占用" :value="LOCATION_STATUS.OCCUPIED" />
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
      </el-form-item>
    </el-form>

    <div class="toolbar">
      <span />
      <el-button type="primary" @click="openBatch">批量生成库位</el-button>
    </div>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column label="仓库" min-width="160">
        <template #default="{ row }">{{ warehouseMap[row.warehouse_id] || row.warehouse_id }}</template>
      </el-table-column>
      <el-table-column prop="zone" label="库区" width="100" />
      <el-table-column prop="code" label="库位编码" min-width="140" />
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="locationStatusTag(row.status)" size="small">{{ locationStatusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="启停" width="90">
        <template #default="{ row }">
          <el-switch
            :model-value="row.status !== LOCATION_STATUS.DISABLED"
            inline-prompt
            active-text="启"
            inactive-text="停"
            @change="onToggleStatus(row, $event)"
          />
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="90" fixed="right">
        <template #default="{ row }">
          <el-button size="small" type="danger" plain @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="query.page"
      v-model:page-size="query.page_size"
      class="pagination"
      layout="total, sizes, prev, pager, next, jumper"
      :total="total"
      :page-sizes="[10, 20, 50, 100]"
      @current-change="load"
      @size-change="search"
    />

    <el-dialog v-model="batchDialog.visible" title="批量生成库位" width="480px" destroy-on-close>
      <el-alert
        type="info"
        :closable="false"
        show-icon
        title="按 库区-排-列 规则批量生成，编码格式：{库区}-{排}-{列}，如 A01-02-03；已存在的编码自动跳过。"
        class="batch-tip"
      />
      <el-form ref="batchFormRef" :model="batchForm" :rules="batchRules" label-width="90px">
        <el-form-item label="仓库" prop="warehouse_id">
          <el-select v-model="batchForm.warehouse_id" placeholder="选择仓库" style="width: 100%">
            <el-option v-for="w in warehouseOptions" :key="w.id" :label="`${w.code}（${w.name}）`" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="库区" prop="zone">
          <el-input v-model="batchForm.zone" placeholder="如 A01" />
        </el-form-item>
        <el-form-item label="排范围">
          <el-input-number v-model="batchForm.row_from" :min="1" controls-position="right" style="width: 120px" />
          <span class="range-sep">至</span>
          <el-input-number v-model="batchForm.row_to" :min="1" controls-position="right" style="width: 120px" />
        </el-form-item>
        <el-form-item label="列范围">
          <el-input-number v-model="batchForm.col_from" :min="1" controls-position="right" style="width: 120px" />
          <span class="range-sep">至</span>
          <el-input-number v-model="batchForm.col_to" :min="1" controls-position="right" style="width: 120px" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="batchDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="batchDialog.loading" @click="submitBatch">生成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.batch-tip {
  margin-bottom: 16px;
}

.range-sep {
  margin: 0 8px;
  color: var(--el-text-color-secondary);
}
</style>
