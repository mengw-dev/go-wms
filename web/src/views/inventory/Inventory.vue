<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useAutoRefresh } from '@/composables/autoRefresh'
import { listInventory, listInventorySummary, listInventoryTrans } from '@/api/inventory'
import type {
  EntityID,
  InventoryItem,
  InventorySummaryItem,
  InventoryTransItem,
} from '@/api/types'
import { statusTag, statusText, TRANS_TYPE_OPTIONS } from '@/constants'
import { cleanParams, formatTime } from '@/utils'
import { GUIDE_EVENTS, useGuideStore } from '@/stores/guide'
import { loadSkuMap, loadWarehouseOptions, toOptionMap, type IdOption } from '@/utils/options'

const activeTab = ref('detail')
const route = useRoute()
const guide = useGuideStore()

// ---------- 仓库下拉 / 货品映射 ----------
const warehouseOptions = ref<IdOption[]>([])
const warehouseMap = ref<Record<EntityID, string>>({})
const skuMap = ref<Record<EntityID, string>>({})

// ---------- 明细 ----------
const detailLoading = ref(false)
const detailList = ref<InventoryItem[]>([])
const detailTotal = ref(0)
const detailQuery = reactive({
  page: 1,
  page_size: 10,
  warehouse_id: '' as EntityID | '',
  sku_keyword: '',
  // 默认只看有货，隐藏已经清空的批次
  in_stock_only: true,
})

async function loadDetail(silent = false) {
  if (!silent) detailLoading.value = true
  try {
    const data = await listInventory(cleanParams({ ...detailQuery }))
    detailList.value = data.list ?? []
    detailTotal.value = data.total ?? 0
  } finally {
    if (!silent) detailLoading.value = false
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
  warehouse_id: '' as EntityID | '',
})

async function loadSummary(silent = false) {
  if (!silent) summaryLoading.value = true
  try {
    const data = await listInventorySummary(cleanParams({ ...summaryQuery }))
    summaryList.value = data.list ?? []
    summaryTotal.value = data.total ?? 0
  } finally {
    if (!silent) summaryLoading.value = false
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
  inventory_id: '' as EntityID | '',
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

async function loadTrans(silent = false) {
  if (!silent) transLoading.value = true
  try {
    const data = await listInventoryTrans(cleanParams({ ...transQuery }))
    transList.value = data.list ?? []
    transTotal.value = data.total ?? 0
    syncInboundGuide()
    syncOutboundGuide()
    syncStocktakeGuide()
  } finally {
    if (!silent) transLoading.value = false
  }
}

function syncInboundGuide(): void {
  if (
    !guide.active ||
    guide.scenario !== 'inbound' ||
    guide.currentStepDefinition?.event !== GUIDE_EVENTS.inboundInventoryReviewed
  ) {
    return
  }
  if (!guide.orderNo || guide.orderNo !== transQuery.order_no) {
    guide.setMismatch('当前库存流水与引导中的入库单不一致，请重新定位当前步骤。')
    return
  }
  if (transList.value.length === 0) {
    guide.setMismatch(`暂未找到入库单 ${guide.orderNo} 的库存流水，请确认上架是否完成。`)
    return
  }
  guide.recordBusinessResult(GUIDE_EVENTS.inboundInventoryReviewed, {
    message: `已查看入库单 ${guide.orderNo} 的 ${transTotal.value} 条库存流水。`,
  })
}

function syncOutboundGuide(): void {
  if (
    !guide.active ||
    guide.scenario !== 'outbound' ||
    guide.currentStepDefinition?.event !== GUIDE_EVENTS.outboundInventoryReviewed
  ) {
    return
  }
  if (!guide.orderNo || guide.orderNo !== transQuery.order_no) {
    guide.setMismatch('当前库存流水与引导中的出库单不一致，请重新定位当前步骤。')
    return
  }
  if (!transList.value.some((item) => item.trans_type === 'SHIP')) {
    guide.setMismatch(`暂未找到出库单 ${guide.orderNo} 的发货扣减流水，请确认拣货发货是否完成。`)
    return
  }
  guide.recordBusinessResult(GUIDE_EVENTS.outboundInventoryReviewed, {
    message: `已查看出库单 ${guide.orderNo} 的 ${transTotal.value} 条库存流水，其中包含 SHIP 发货扣减。`,
  })
}

function syncStocktakeGuide(): void {
  if (
    !guide.active ||
    guide.scenario !== 'stocktake' ||
    guide.currentStepDefinition?.event !== GUIDE_EVENTS.stocktakeInventoryReviewed
  ) {
    return
  }
  if (!guide.orderNo || guide.orderNo !== transQuery.order_no) {
    guide.setMismatch('当前库存流水与引导中的盘点单不一致，请重新定位当前步骤。')
    return
  }
  if (!transList.value.some((item) => item.trans_type === 'ADJUST')) {
    guide.setMismatch(`暂未找到盘点单 ${guide.orderNo} 的 ADJUST 调整流水，请确认审核是否完成。`)
    return
  }
  guide.recordBusinessResult(GUIDE_EVENTS.stocktakeInventoryReviewed, {
    message: `已查看盘点单 ${guide.orderNo} 的库存调整流水。`,
  })
}

function searchTrans() {
  transQuery.page = 1
  loadTrans()
}

function skuLabel(skuId: EntityID): string {
  return skuMap.value[skuId] || String(skuId)
}

onMounted(async () => {
  const skuKeyword = typeof route.query.sku_keyword === 'string' ? route.query.sku_keyword : ''
  const orderNo = typeof route.query.order_no === 'string' ? route.query.order_no : ''
  if (skuKeyword) detailQuery.sku_keyword = skuKeyword
  if (orderNo) {
    transQuery.order_no = orderNo
    drawerVisible.value = true
  }

  warehouseOptions.value = await loadWarehouseOptions()
  warehouseMap.value = toOptionMap(warehouseOptions.value)
  const map = await loadSkuMap()
  skuMap.value = Object.fromEntries(Object.entries(map).map(([k, v]) => [k, v.name]))
  await loadDetail()
  if (drawerVisible.value) await loadTrans()
})

function refreshActiveTab() {
  if (activeTab.value === 'summary') {
    loadSummary(true)
  } else {
    loadDetail(true)
  }
  if (drawerVisible.value) loadTrans(true)
}

useAutoRefresh(refreshActiveTab, 0)
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
          <el-form-item label="SKU关键字">
            <el-input v-model="detailQuery.sku_keyword" placeholder="编码/名称/条码" clearable style="width: 160px" @keyup.enter="searchDetail" @clear="searchDetail" />
          </el-form-item>
          <el-form-item label="只看有货">
            <el-switch v-model="detailQuery.in_stock_only" @change="searchDetail" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="searchDetail">查询</el-button>
          </el-form-item>
        </el-form>

        <el-table v-loading="detailLoading" :data="detailList" border stripe>
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
      <div data-tour="inventory-evidence">
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
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.trans-summary {
  color: var(--el-text-color-regular);
  margin-bottom: 12px;
}
</style>
