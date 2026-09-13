<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveStocktakeOrder,
  cancelStocktakeOrder,
  createStocktakeOrder,
  listStocktakeOrders,
} from '@/api/stocktake'
import type { StocktakeOrderItem } from '@/api/types'
import { STOCKTAKE_STATUS_OPTIONS, statusTag, statusText } from '@/constants'
import { cleanParams, formatTime } from '@/utils'
import { loadLocationOptions, loadWarehouseOptions, toOptionMap, type IdOption } from '@/utils/options'

const router = useRouter()

// ---------- 基础选项 ----------
const warehouseOptions = ref<IdOption[]>([])
const warehouseMap = ref<Record<number, string>>({})

onMounted(async () => {
  warehouseOptions.value = await loadWarehouseOptions()
  warehouseMap.value = toOptionMap(warehouseOptions.value)
  load()
})

// ---------- 列表 ----------
const loading = ref(false)
const list = ref<StocktakeOrderItem[]>([])
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
    const data = await listStocktakeOrders(cleanParams({ ...query }))
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

function goDetail(row: StocktakeOrderItem) {
  router.push(`/stocktake/orders/${row.id}`)
}

// ---------- 行操作 ----------
async function onApprove(row: StocktakeOrderItem) {
  try {
    await ElMessageBox.confirm(
      `确定审核盘点单「${row.order_no}」吗？审核后将按差异自动调整库存。`,
      '提示',
      { type: 'warning' },
    )
  } catch {
    return
  }
  await approveStocktakeOrder(row.id)
  ElMessage.success('审核完成')
  load()
}

async function onCancel(row: StocktakeOrderItem) {
  try {
    await ElMessageBox.confirm(`确定取消盘点单「${row.order_no}」吗？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await cancelStocktakeOrder(row.id)
  ElMessage.success('已取消')
  load()
}

// ---------- 新建 ----------
const createDialog = reactive({ visible: false, loading: false })
const createForm = reactive({
  warehouse_id: undefined as number | undefined,
  location_id: 0,
  remark: '',
})
const locationOptions = ref<IdOption[]>([])
const locationLoading = ref(false)

function openCreate() {
  createForm.warehouse_id = undefined
  createForm.location_id = 0
  createForm.remark = ''
  locationOptions.value = []
  createDialog.visible = true
}

async function onWarehouseChange(warehouseId: number | undefined) {
  createForm.location_id = 0
  locationOptions.value = []
  if (warehouseId) {
    locationLoading.value = true
    try {
      locationOptions.value = await loadLocationOptions(warehouseId)
    } finally {
      locationLoading.value = false
    }
  }
}

async function submitCreate() {
  if (!createForm.warehouse_id) {
    ElMessage.warning('请选择仓库')
    return
  }
  createDialog.loading = true
  try {
    await createStocktakeOrder({
      warehouse_id: createForm.warehouse_id,
      location_id: createForm.location_id,
      location_code: locationOptions.value.find((l) => l.id === createForm.location_id)?.label,
      remark: createForm.remark,
    })
    ElMessage.success('盘点单创建成功，已生成账面快照')
    createDialog.visible = false
    load()
  } finally {
    createDialog.loading = false
  }
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
          <el-option v-for="s in STOCKTAKE_STATUS_OPTIONS" :key="s" :label="statusText(s)" :value="s" />
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
      <span />
      <el-button type="primary" @click="openCreate">新建盘点单</el-button>
    </div>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column prop="order_no" label="盘点单号" min-width="170">
        <template #default="{ row }">
          <el-link type="primary" @click="goDetail(row)">{{ row.order_no }}</el-link>
        </template>
      </el-table-column>
      <el-table-column label="仓库" min-width="150">
        <template #default="{ row }">{{ warehouseMap[row.warehouse_id] || row.warehouse_id }}</template>
      </el-table-column>
      <el-table-column label="盘点范围" min-width="130">
        <template #default="{ row }">
          {{ row.location_id > 0 ? (row.location_code || `库位#${row.location_id}`) : '整仓盘点' }}
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="remark" label="备注" min-width="130" show-overflow-tooltip />
      <el-table-column prop="created_by" label="创建人" width="100" />
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <div class="table-oper">
            <el-button size="small" @click="goDetail(row)">详情</el-button>
            <template v-if="row.status === 'DRAFT'">
              <el-button size="small" type="primary" plain @click="goDetail(row)">录入实盘</el-button>
              <el-button size="small" type="success" plain @click="onApprove(row)">审核</el-button>
              <el-button size="small" type="danger" plain @click="onCancel(row)">取消</el-button>
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
    <el-dialog v-model="createDialog.visible" title="新建盘点单" width="520px" destroy-on-close>
      <el-form label-width="90px">
        <el-form-item label="仓库" required>
          <el-select
            v-model="createForm.warehouse_id"
            placeholder="选择仓库"
            style="width: 100%"
            @change="onWarehouseChange"
          >
            <el-option v-for="w in warehouseOptions" :key="w.id" :label="w.label" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="盘点范围">
          <el-select
            v-model="createForm.location_id"
            placeholder="不选则为整仓盘点"
            clearable
            filterable
            :loading="locationLoading"
            style="width: 100%"
          >
            <el-option v-for="loc in locationOptions" :key="loc.id" :label="loc.label" :value="loc.id" />
          </el-select>
          <div class="form-tip">选择库位则只盘点该库位；留空（0）表示整仓盘点</div>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="createForm.remark" type="textarea" :rows="2" placeholder="备注（可选）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="createDialog.loading" @click="submitCreate">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.form-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
  margin-top: 2px;
}
</style>
