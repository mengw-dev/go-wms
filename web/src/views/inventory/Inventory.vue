<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { listInventory, listInventorySummary, listInventoryTrans } from '@/api/inventory'
import type {
  InventoryItem,
  InventorySummaryItem,
  InventoryTransItem,
} from '@/api/types'
import { statusTag, statusText, TRANS_TYPE_OPTIONS } from '@/constants'
import { cleanParams, formatTime } from '@/utils'
import { loadSkuMap, loadWarehouseOptions, toOptionMap, type IdOption } from '@/utils/options'

const activeTab = ref('detail')

// ---------- 仓库下拉 / 货品映射 ----------
const warehouseOptions = ref<IdOption[]>([])
const warehouseMap = ref<Record<number, string>>({})
const skuMap = ref<Record<number, string>>({})

onMounted(async () => {
  warehouseOptions.value = await loadWarehouseOptions()
  warehouseMap.value = toOptionMap(warehouseOptions.value)
  const map = await loadSkuMap()
  skuMap.value = Object.fromEntries(Object.entries(map).map(([k, v]) => [Number(k), v.name]))
})

// ---------- 明细 ----------
const detailLoading = ref(false)
const detailList = ref<InventoryItem[]>([])
const detailTotal = ref(0)
const detailQuery = reactive({
  page: 1,
  page_size: 10,
  warehouse_id: '' as number | '',
  location_id: '' as number | '',
  sku_id: '' as number | '',
  sku_keyword: '',
})

async function loadDetail() {
  detailLoading.value = true
  try {
    const data = await listInventory(cleanParams({ ...detailQuery }))
    detailList.value = data.list ?? []
    detailTotal.value = data.total ?? 0
  } finally {
    detailLoading.value = false
  }
}

function searchDetail() {
  detailQuery.page = 1
  loadDetail()
}

// ---------- 汇总 ----------
const summaryLoading = ref(false)
const summaryList = ref<InventorySummaryItem[]>([])
const summaryTotal = ref(0)
const summaryQuery = reactive({
  page: 1,
  page_size: 10,
  warehouse_id: '' as number | '',
})

async function loadSummary() {
  summaryLoading.value = true
  try {
    const data = await listInventorySummary(cleanParams({ ...summaryQuery }))
    summaryList.value = data.list ?? []
    summaryTotal.value = data.total ?? 0
  } finally {
    summaryLoading.value = false
  }
}

function searchSummary() {
  summaryQuery.page = 1
  loadSummary()
}

function onTabChange(name: string | number) {
  if (name === 'summary' && summaryList.value.length === 0) loadSummary()
}

// ---------- 流水抽屉 ----------
const drawerVisible = ref(false)
const transLoading = ref(false)
const transList = ref<InventoryTransItem[]>([])
const transTotal = ref(0)
const transQuery = reactive({
  page: 1,
  page_size: 10,
  inventory_id: 0,
  order_no: '',
  trans_type: '',
})
const transInventory = ref<InventoryItem | null>(null)

function openTrans(row: InventoryItem) {
  transInventory.value = row
  transQuery.inventory_id = row.id
  transQuery.order_no = ''
  transQuery.trans_type = ''
  transQuery.page = 1
  drawerVisible.value = true
  loadTrans()
}

async function loadTrans() {
  transLoading.value = true
  try {
    const data = await listInventoryTrans(cleanParams({ ...transQuery }))
    transList.value = data.list ?? []
    transTotal.value = data.total ?? 0
  } finally {
    transLoading.value = false
  }
}

function searchTrans() {
  transQuery.page = 1
  loadTrans()
}

function skuLabel(skuId: number): string {
  return skuMap.value[skuId] || String(skuId)
}
</script>

<template>
  <div class="page-card">
    <el-tabs v-model="activeTab" @tab-change="onTabChange">
      <el-tab-pane label="库存明细" name="detail">
        <el-form inline class="query-form" @submit.prevent="searchDetail">
          <el-form-item label="仓库">
            <el-select v-model="detailQuery.warehouse_id" placeholder="全部" clearable style="width: 200px" @change="searchDetail">
              <el-option v-for="w in warehouseOptions" :key="w.id" :label="w.label" :value="w.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="库位ID">
            <el-input v-model="detailQuery.location_id" placeholder="库位ID" clearable style="width: 110px" @keyup.enter="searchDetail" @clear="searchDetail" />
          </el-form-item>
          <el-form-item label="SKU ID">
            <el-input v-model="detailQuery.sku_id" placeholder="货品ID" clearable style="width: 110px" @keyup.enter="searchDetail" @clear="searchDetail" />
          </el-form-item>
          <el-form-item label="SKU关键字">
            <el-input v-model="detailQuery.sku_keyword" placeholder="编码/名称/条码" clearable style="width: 160px" @keyup.enter="searchDetail" @clear="searchDetail" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="searchDetail">查询</el-button>
          </el-form-item>
        </el-form>

        <el-table v-loading="detailLoading" :data="detailList" border stripe>
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column label="仓库" min-width="150">
            <template #default="{ row }">{{ warehouseMap[row.warehouse_id] || row.warehouse_id }}</template>
          </el-table-column>
          <el-table-column prop="location_code" label="库位" min-width="110">
            <template #default="{ row }">{{ row.location_code || row.location_id }}</template>
          </el-table-column>
          <el-table-column label="货品" min-width="160">
            <template #default="{ row }">{{ skuLabel(row.sku_id) }}</template>
          </el-table-column>
          <el-table-column prop="batch_no" label="批次" min-width="100">
            <template #default="{ row }">{{ row.batch_no || '-' }}</template>
          </el-table-column>
          <el-table-column prop="stock_quantity" label="现存量" width="90" align="right" />
          <el-table-column prop="available_quantity" label="可用量" width="90" align="right" />
          <el-table-column prop="allocated_quantity" label="分配量" width="90" align="right" />
          <el-table-column label="入库时间" width="170">
            <template #default="{ row }">{{ formatTime(row.stock_in_time) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="90" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="primary" plain @click="openTrans(row)">流水</el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-pagination
          v-model:current-page="detailQuery.page"
          v-model:page-size="detailQuery.page_size"
          class="pagination"
          layout="total, sizes, prev, pager, next, jumper"
          :total="detailTotal"
          :page-sizes="[10, 20, 50]"
          @current-change="loadDetail"
          @size-change="searchDetail"
        />
      </el-tab-pane>

      <el-tab-pane label="按SKU汇总" name="summary">
        <el-form inline class="query-form" @submit.prevent="searchSummary">
          <el-form-item label="仓库">
            <el-select v-model="summaryQuery.warehouse_id" placeholder="全部" clearable style="width: 200px" @change="searchSummary">
              <el-option v-for="w in warehouseOptions" :key="w.id" :label="w.label" :value="w.id" />
            </el-select>
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="searchSummary">查询</el-button>
          </el-form-item>
        </el-form>

        <el-table v-loading="summaryLoading" :data="summaryList" border stripe>
          <el-table-column prop="sku_code" label="货品编码" min-width="130" />
          <el-table-column prop="sku_name" label="货品名称" min-width="180" show-overflow-tooltip />
          <el-table-column prop="unit" label="单位" width="80">
            <template #default="{ row }">{{ row.unit || '-' }}</template>
          </el-table-column>
          <el-table-column prop="stock_quantity" label="现存量" width="110" align="right" />
          <el-table-column prop="available_quantity" label="可用量" width="110" align="right" />
          <el-table-column prop="allocated_quantity" label="分配量" width="110" align="right" />
        </el-table>

        <el-pagination
          v-model:current-page="summaryQuery.page"
          v-model:page-size="summaryQuery.page_size"
          class="pagination"
          layout="total, sizes, prev, pager, next, jumper"
          :total="summaryTotal"
          :page-sizes="[10, 20, 50]"
          @current-change="loadSummary"
          @size-change="searchSummary"
        />
      </el-tab-pane>
    </el-tabs>

    <el-drawer v-model="drawerVisible" title="库存流水" size="60%">
      <div v-if="transInventory" class="trans-summary">
        库位：{{ transInventory.location_code || transInventory.location_id }}
        ，货品：{{ skuLabel(transInventory.sku_id) }}
        ，批次：{{ transInventory.batch_no || '-' }}
      </div>
      <el-form inline @submit.prevent="searchTrans">
        <el-form-item label="单据号">
          <el-input v-model="transQuery.order_no" placeholder="单据号" clearable style="width: 180px" @keyup.enter="searchTrans" @clear="searchTrans" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="transQuery.trans_type" placeholder="全部" clearable style="width: 130px" @change="searchTrans">
            <el-option v-for="t in TRANS_TYPE_OPTIONS" :key="t" :label="statusText(t)" :value="t" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="searchTrans">查询</el-button>
        </el-form-item>
      </el-form>

      <el-table v-loading="transLoading" :data="transList" border stripe size="small">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.trans_type)" size="small">{{ statusText(row.trans_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="数量变化" width="100" align="right">
          <template #default="{ row }">
            <span :style="{ color: row.quantity_change >= 0 ? 'var(--el-color-success)' : 'var(--el-color-danger)' }">
              {{ row.quantity_change >= 0 ? '+' : '' }}{{ row.quantity_change }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="现存量变化" min-width="110">
          <template #default="{ row }">{{ row.before_quantity }} → {{ row.after_quantity }}</template>
        </el-table-column>
        <el-table-column label="可用量变化" min-width="110">
          <template #default="{ row }">{{ row.available_before }} → {{ row.available_after }}</template>
        </el-table-column>
        <el-table-column prop="order_no" label="来源单据" min-width="150">
          <template #default="{ row }">{{ row.order_no || '-' }}</template>
        </el-table-column>
        <el-table-column prop="task_no" label="任务号" min-width="140">
          <template #default="{ row }">{{ row.task_no || '-' }}</template>
        </el-table-column>
        <el-table-column prop="operator" label="操作员" width="100">
          <template #default="{ row }">{{ row.operator || '-' }}</template>
        </el-table-column>
        <el-table-column label="时间" width="160">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="transQuery.page"
        v-model:page-size="transQuery.page_size"
        class="pagination"
        layout="total, prev, pager, next"
        :total="transTotal"
        @current-change="loadTrans"
      />
    </el-drawer>
  </div>
</template>

<style scoped>
.trans-summary {
  color: var(--el-text-color-regular);
  margin-bottom: 12px;
}
</style>
