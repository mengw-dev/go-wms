<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Refresh, TrendCharts } from '@element-plus/icons-vue'
import { getDemoActivity } from '@/api/demo'
import type {
  DemoActivitySnapshot,
  EntityID,
  InboundOrderItem,
  InventoryTransItem,
  OutboundOrderItem,
  TaskItem,
} from '@/api/types'
import { statusTag, statusText, taskTypeText } from '@/constants'
import { formatTime } from '@/utils'
import { clearDemoEvidence, filterDemoActivity, readDemoEvidence, resolveDemoEvidenceFocus } from '@/utils/demoEvidence'
import { buildDemoOperationRows, type DemoOperationRow } from '@/utils/demoOperations'
import { useAutoRefresh } from '@/composables/autoRefresh'

interface DetailRow {
  label: string
  value: string
}

interface BusinessObjectRow {
  key: string
  type: string
  objectNo: string
  status: string
  quantity: string
  createdAt: string
  route: string
  details: DetailRow[]
}

interface InventoryRow {
  key: string
  type: string
  objectNo: string
  quantityChange: number
  createdAt: string
  orderNo: string
  details: DetailRow[]
}

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const data = ref<DemoActivitySnapshot | null>(null)
const activeTab = ref(route.query.tab === 'operations' ? 'operations' : 'objects')
const context = ref(readDemoEvidence())
const objectPage = ref(1)
const inventoryPage = ref(1)
const operationPage = ref(1)
const pageSize = 5
const detailVisible = ref(false)
const detailTitle = ref('')
const detailRows = ref<DetailRow[]>([])

const focus = computed(() => resolveDemoEvidenceFocus(route.query as Record<string, unknown>, context.value))
const focused = computed(() => Boolean(focus.value.scenario || focus.value.startedAt || focus.value.orderNos.length))
const evidence = computed(() => {
  if (!data.value) return null
  return { ...filterDemoActivity(data.value, focus.value), stocktake_orders: [] }
})
const scenarioLabel = computed(() => {
  const labels: Record<string, string> = {
    inbound: '入库自动演示',
    outbound: '出库自动演示',
    stocktake: '盘点自动演示',
    full: '完整业务闭环',
  }
  const label = labels[focus.value.scenario] || '最近一次演示'
  return focus.value.source === 'manual' ? label.replace('自动演示', '手动体验') : label
})

function parseLeadingNumber(value: string): number | null {
  const matched = value.match(/-?\d+/)?.[0]
  if (!matched) return null
  const parsed = Number(matched)
  return Number.isFinite(parsed) ? parsed : null
}

const evidenceStats = computed(() => {
  const items = context.value?.evidence ?? []
  const labels = (targets: string[]) => items.filter((item) => targets.some((target) => item.label.includes(target)))
  const taskItems = labels(['作业任务', 'PICK 任务'])
  const transItems = labels(['库存流水'])
  return {
    inbound: labels(['入库单']).length,
    outbound: labels(['出库单']).length,
    tasks: taskItems.reduce((total, item) => total + (item.value.includes('个') ? parseLeadingNumber(item.value) ?? 1 : 1), 0),
    inventoryTrans: transItems.reduce((total, item) => total + (item.value.includes('条') ? parseLeadingNumber(item.value) ?? 1 : 1), 0),
    inventoryChange: items
      .filter((item) => item.label.includes('库存变化'))
      .reduce((total, item) => total + (parseLeadingNumber(item.value) ?? 0), 0),
  }
})

function quantityFromDetail(detail?: string): string {
  const matched = detail?.match(/×\s*(\d+)/)
  return matched ? matched[1] + ' 件' : detail || '-'
}

function evidenceFallbackObject(label: string, type: string): BusinessObjectRow | null {
  const item = context.value?.evidence?.find((entry) => entry.label === label)
  if (!item) return null
  const route = type === '入库单'
    ? context.value?.links?.find((link) => link.path.startsWith('/inbound/orders/'))?.path
    : context.value?.links?.find((link) => link.path.startsWith('/outbound/orders/'))?.path
  if (!route) return null
  return {
    key: `evidence-${type}-${item.value}`,
    type,
    objectNo: item.value,
    status: 'COMPLETED',
    quantity: quantityFromDetail(item.detail),
    createdAt: context.value?.completedAt || '',
    route,
    details: [
      { label: type + '号', value: item.value },
      { label: '业务明细', value: item.detail || '-' },
      { label: '结果来源', value: '本次真实自动演示结果' },
      { label: '完成时间', value: formatTime(context.value?.completedAt || '') },
    ],
  }
}

const businessObjects = computed<BusinessObjectRow[]>(() => {
  const current = evidence.value
  if (!current) return []
  const rows = [
    ...current.inbound_orders.map(inboundObject),
    ...current.outbound_orders.map(outboundObject),
    ...current.tasks.map(taskObject),
  ]
  const fallbackInbound = evidenceFallbackObject('入库单', '入库单')
  const fallbackOutbound = evidenceFallbackObject('出库单', '出库单')
  for (const fallback of [fallbackInbound, fallbackOutbound]) {
    if (fallback && !rows.some((row) => row.type === fallback.type && row.objectNo === fallback.objectNo)) rows.push(fallback)
  }
  return rows.sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))
})
const inventoryRows = computed<InventoryRow[]>(() => (evidence.value?.inventory_trans ?? []).map(inventoryObject))
const operationRows = computed<DemoOperationRow[]>(() => buildDemoOperationRows(evidence.value?.operations ?? [], evidence.value))

const summaryCards = computed(() => {
  const current = evidence.value
  if (!current) return []
  const stats = evidenceStats.value
  const orders = [...current.inbound_orders, ...current.outbound_orders]
  const completedOrders = orders.filter((order) => order.status === 'COMPLETED' || order.status === 'SHIPPED').length
  const snapshotQuantityChange = current.inventory_trans.reduce((total, item) => total + item.quantity_change, 0)
  const inboundCount = Math.max(current.inbound_orders.length, stats.inbound)
  const outboundCount = Math.max(current.outbound_orders.length, stats.outbound)
  const taskCount = Math.max(current.tasks.length, stats.tasks)
  const transCount = Math.max(current.inventory_trans.length, stats.inventoryTrans)
  const quantityChange = context.value?.evidence?.length ? stats.inventoryChange : snapshotQuantityChange
  return [
    { label: '入库单', value: String(inboundCount) },
    { label: '出库单', value: String(outboundCount) },
    { label: '作业任务', value: String(taskCount) },
    { label: '库存流水', value: String(transCount) },
    { label: '业务状态', value: context.value?.completedAt && focus.value.scenario === 'full' ? '闭环已完成' : `${completedOrders} / ${orders.length} 已完成` },
    { label: '库存变化', value: `${quantityChange > 0 ? '+' : ''}${quantityChange} 件` },
  ]
})

function shortId(value?: EntityID | null, prefix = '编号'): string {
  if (!value) return '-'
  const text = String(value)
  return `${prefix} · ${text.slice(-8)}`
}

function inboundObject(order: InboundOrderItem): BusinessObjectRow {
  return {
    key: `inbound-${order.id}`,
    type: '入库单',
    objectNo: order.order_no,
    status: order.status,
    quantity: `${order.received_qty} / ${order.expected_qty}`,
    createdAt: order.created_at,
    route: `/inbound/orders/${order.id}`,
    details: [
      { label: '完整对象 ID', value: String(order.id) },
      { label: '入库单号', value: order.order_no },
      { label: '仓库 ID', value: String(order.warehouse_id) },
      { label: '应收数量', value: String(order.expected_qty) },
      { label: '已收数量', value: String(order.received_qty) },
      { label: '不良品数量', value: String(order.defective_qty) },
      { label: '状态', value: statusText(order.status) },
      { label: '创建时间', value: formatTime(order.created_at) },
    ],
  }
}

function outboundObject(order: OutboundOrderItem): BusinessObjectRow {
  return {
    key: `outbound-${order.id}`,
    type: '出库单',
    objectNo: order.order_no,
    status: order.status,
    quantity: `${order.picked_qty} / ${order.expected_qty}`,
    createdAt: order.created_at,
    route: `/outbound/orders/${order.id}`,
    details: [
      { label: '完整对象 ID', value: String(order.id) },
      { label: '出库单号', value: order.order_no },
      { label: '业务单号', value: order.biz_order_no || '-' },
      { label: '仓库 ID', value: String(order.warehouse_id) },
      { label: '需求数量', value: String(order.expected_qty) },
      { label: '已分配数量', value: String(order.allocated_qty) },
      { label: '已拣数量', value: String(order.picked_qty) },
      { label: '状态', value: statusText(order.status) },
      { label: '创建时间', value: formatTime(order.created_at) },
    ],
  }
}

function taskObject(task: TaskItem): BusinessObjectRow {
  return {
    key: `task-${task.id}`,
    type: taskTypeText(task.task_type),
    objectNo: task.task_no,
    status: task.status,
    quantity: `${task.done_qty} / ${task.target_qty}`,
    createdAt: task.created_at,
    route: task.order_id ? `/tasks?order_id=${task.order_id}` : '/tasks',
    details: [
      { label: '完整对象 ID', value: String(task.id) },
      { label: '任务号', value: task.task_no },
      { label: '任务类型', value: taskTypeText(task.task_type) },
      { label: '关联单号', value: task.order_no || '-' },
      { label: '库位', value: task.location_code || '-' },
      { label: '批次', value: task.batch_no || '-' },
      { label: '完成 / 目标', value: `${task.done_qty} / ${task.target_qty}` },
      { label: '状态', value: statusText(task.status) },
      { label: '创建时间', value: formatTime(task.created_at) },
    ],
  }
}

function inventoryObject(item: InventoryTransItem): InventoryRow {
  const objectNo = item.task_no || item.order_no || shortId(item.id, '流水')
  return {
    key: `inventory-${item.id}`,
    type: statusText(item.trans_type),
    objectNo,
    quantityChange: item.quantity_change,
    createdAt: item.created_at,
    orderNo: item.order_no || '-',
    details: [
      { label: '完整流水 ID', value: String(item.id) },
      { label: '流水类型', value: statusText(item.trans_type) },
      { label: '关联单号', value: item.order_no || '-' },
      { label: '关联任务', value: item.task_no || '-' },
      { label: '数量变化', value: String(item.quantity_change) },
      { label: '现存量', value: `${item.before_quantity} → ${item.after_quantity}` },
      { label: '可用量', value: `${item.available_before} → ${item.available_after}` },
      { label: '操作时间', value: formatTime(item.created_at) },
    ],
  }
}

function paginate<T>(items: T[], page: number): T[] {
  return items.slice((page - 1) * pageSize, page * pageSize)
}

const pagedObjects = computed(() => paginate(businessObjects.value, objectPage.value))
const pagedInventory = computed(() => paginate(inventoryRows.value, inventoryPage.value))
const pagedOperations = computed(() => paginate(operationRows.value, operationPage.value))

async function load(silent = false): Promise<void> {
  if (!silent) loading.value = true
  try {
    data.value = await getDemoActivity(50)
  } finally {
    if (!silent) loading.value = false
  }
}

function showAll(): void {
  clearDemoEvidence()
  context.value = null
  void router.replace('/demo/activity')
}

function openDetail(title: string, rows: DetailRow[]): void {
  detailTitle.value = title
  detailRows.value = rows
  detailVisible.value = true
}

function openOperationDetail(row: DemoOperationRow): void {
  openDetail('操作记录详情', row.details)
}

function openBusinessDetail(row: BusinessObjectRow): void {
  openDetail(row.type + '详情', row.details)
}

function openInventoryDetail(row: InventoryRow): void {
  openDetail('库存流水详情', row.details)
}

function viewObject(row: BusinessObjectRow): void {
  void router.push(row.route)
}

function onTabChange(): void {
  objectPage.value = 1
  inventoryPage.value = 1
  operationPage.value = 1
}

watch(
  () => route.query.tab,
  (tab) => {
    activeTab.value = tab === 'operations' ? 'operations' : 'objects'
  },
)

onMounted(() => load())
useAutoRefresh(() => load(true), 5000)
</script>

<template>
  <div v-loading="loading" class="app-page activity-page">
    <header class="page-header">
      <div class="page-header__main">
        <span class="page-header__eyebrow">结果核对</span>
        <h1 class="page-header__title">{{ focused ? '本次业务执行证据' : '业务证据' }}</h1>
        <p class="page-header__description">
          <template v-if="focused"><span class="scenario-name">{{ scenarioLabel }}</span><span> · 只展示本次执行关联的真实业务对象。</span></template>
          <template v-else>展示当前演示账号最近产生的业务对象、库存流水和操作记录。</template>
        </p>
      </div>
      <div class="page-header__actions">
        <el-button v-if="focused" @click="showAll">查看全部记录</el-button>
        <el-button :icon="TrendCharts" @click="router.push('/demo/performance')">运行状态</el-button>
        <el-button :icon="Refresh" type="primary" @click="load()">刷新</el-button>
      </div>
    </header>

    <section class="app-card summary-card">
      <div v-for="item in summaryCards" :key="item.label">
        <span>{{ item.label }}</span>
        <b>{{ item.value }}</b>
      </div>
    </section>

    <section class="app-card app-card--flush evidence-card">
      <el-tabs v-model="activeTab" class="evidence-tabs" @tab-change="onTabChange">
        <el-tab-pane label="业务对象" name="objects">
          <el-table :data="pagedObjects" size="small" border stripe empty-text="本次执行没有业务对象">
            <el-table-column prop="type" label="类型" width="100" />
            <el-table-column prop="objectNo" label="单号" min-width="170" />
            <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag></template></el-table-column>
            <el-table-column prop="quantity" label="数量" width="100" align="right" />
            <el-table-column label="创建时间" width="170"><template #default="{ row }">{{ formatTime(row.createdAt) }}</template></el-table-column>
            <el-table-column label="查看" width="140" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openBusinessDetail(row)">详情</el-button><el-button link type="primary" @click="viewObject(row)">打开</el-button></template></el-table-column>
          </el-table>
          <el-pagination v-if="businessObjects.length > pageSize" v-model:current-page="objectPage" small background layout="prev, pager, next" :page-size="pageSize" :total="businessObjects.length" />
        </el-tab-pane>

        <el-tab-pane label="库存流水" name="inventory">
          <el-table :data="pagedInventory" size="small" border stripe empty-text="本次执行没有库存流水" @row-click="openInventoryDetail">
            <el-table-column prop="type" label="操作类型" width="100" />
            <el-table-column prop="objectNo" label="业务对象" min-width="170" />
            <el-table-column label="数量变化" width="100" align="right"><template #default="{ row }"><span :class="row.quantityChange >= 0 ? 'up' : 'down'">{{ row.quantityChange > 0 ? '+' : '' }}{{ row.quantityChange }}</span></template></el-table-column>
            <el-table-column label="操作时间" width="170"><template #default="{ row }">{{ formatTime(row.createdAt) }}</template></el-table-column>
            <el-table-column prop="orderNo" label="关联单据" min-width="160" />
          </el-table>
          <el-pagination v-if="inventoryRows.length > pageSize" v-model:current-page="inventoryPage" small background layout="prev, pager, next" :page-size="pageSize" :total="inventoryRows.length" />
        </el-tab-pane>

        <el-tab-pane label="操作日志" name="operations">
          <el-table :data="pagedOperations" size="small" border stripe empty-text="本次执行没有操作记录" @row-click="openOperationDetail">
            <el-table-column prop="operation" label="操作" min-width="150" />
            <el-table-column prop="objectNo" label="对象" min-width="170" />
            <el-table-column prop="beforeStatus" label="前置状态" width="110" />
            <el-table-column prop="afterStatus" label="后置状态" width="110" />
            <el-table-column label="时间" width="170"><template #default="{ row }">{{ formatTime(row.createdAt) }}</template></el-table-column>
          </el-table>
          <el-pagination v-if="operationRows.length > pageSize" v-model:current-page="operationPage" small background layout="prev, pager, next" :page-size="pageSize" :total="operationRows.length" />
        </el-tab-pane>
      </el-tabs>
    </section>

    <el-drawer v-model="detailVisible" :title="detailTitle" size="min(520px, 94vw)" append-to-body>
      <el-descriptions :column="1" border>
        <el-descriptions-item v-for="item in detailRows" :key="item.label" :label="item.label">{{ item.value }}</el-descriptions-item>
      </el-descriptions>
    </el-drawer>
  </div>
</template>

<style scoped>
.activity-page { width: min(1180px, 100%); margin: 0 auto; }
.scenario-name { font-weight: 600; color: var(--el-text-color-primary); }
.summary-card { padding: 14px 16px; display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 10px; }
.summary-card > div { min-width: 0; padding: 10px 12px; border-radius: 9px; background: var(--el-fill-color-lighter); }
.summary-card span, .summary-card b { display: block; }
.summary-card span { color: var(--el-text-color-secondary); font-size: 11px; }
.summary-card b { margin-top: 5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 17px; }
.evidence-card { min-height: 0; }
.evidence-tabs { padding: 0 16px 14px; }
.evidence-tabs :deep(.el-tabs__header) { margin-bottom: 12px; }
.evidence-tabs :deep(.el-pagination) { justify-content: flex-end; margin-top: 12px; }
.evidence-tabs :deep(.el-table__row) { cursor: pointer; }
.up { color: var(--el-color-success); font-weight: 600; }
.down { color: var(--el-color-danger); font-weight: 600; }
@media (max-width: 900px) { .summary-card { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 600px) { .summary-card { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
