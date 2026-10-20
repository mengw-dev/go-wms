<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, ArrowRight } from '@element-plus/icons-vue'
import { getDashboardOverview, type DashboardOverview, type DashboardOrders, type DashboardTask } from '@/api/dashboard'
import { useAutoRefresh } from '@/composables/autoRefresh'
import { useAuthStore } from '@/stores/auth'
import { statusText, statusTag, taskTypeText } from '@/constants'

const auth = useAuthStore()
const router = useRouter()
const warehouseId = ref('')
const warehouses = ref<DashboardOverview['warehouses']>([])
const data = ref<DashboardOverview>()
const loading = ref(false)
const error = ref(false)
const trendDetails = ref(false)
let sequence = 0

async function load(silent = false) {
  const seq = ++sequence
  if (!silent) loading.value = true
  try {
    const result = await getDashboardOverview(warehouseId.value)
    if (seq !== sequence) return
    data.value = result
    warehouses.value = result.warehouses || []
    error.value = false
  } catch {
    if (seq === sequence) error.value = true
  } finally {
    if (seq === sequence) loading.value = false
  }
}
watch(warehouseId, () => { data.value = undefined; void load() })
onMounted(() => { void load() })
onBeforeUnmount(() => { sequence++ })
useAutoRefresh(() => load(true), 30000, () => document.visibilityState === 'visible')

const count = (orders: DashboardOrders | undefined, status?: string, today = false) =>
  (orders?.statuses || []).filter(row => !status || row.status === status).reduce((sum, row) => sum + (today ? row.today : row.total), 0)
const pending = computed(() => (data.value?.task_counts || []).filter(row => row.status === 'CREATED').reduce((n, row) => n + row.total, 0))
const running = computed(() => (data.value?.task_counts || []).filter(row => row.status === 'IN_PROGRESS').reduce((n, row) => n + row.total, 0))
const aged = computed(() => (data.value?.task_counts || []).reduce((n, row) => n + row.older_24h, 0))
const occupied = computed(() => data.value?.locations?.enabled ? Math.round(data.value.locations.occupied / data.value.locations.enabled * 100) : 0)
const updated = computed(() => data.value ? new Date(data.value.generated_at).toLocaleTimeString('zh-CN', { hour12: false, timeZone: 'Asia/Shanghai' }) : '尚未更新')
const date = computed(() => data.value?.today_from.slice(0, 10) || '')
const inboundStates = ['DRAFT', 'SUBMITTED', 'APPROVED', 'RECEIVING', 'PUTAWAY', 'COMPLETED']
const outboundStates = ['DRAFT', 'SUBMITTED', 'APPROVED', 'PICKING', 'SHIPPED']

function go(path: string, extra: Record<string, string> = {}) {
  void router.push({ path, query: { ...(warehouseId.value ? { warehouse_id: warehouseId.value } : {}), ...extra } })
}
function todayOrders(path: string) {
  if (data.value) go(path, { created_at_from: data.value.today_from, created_at_to: data.value.today_to })
}
function cancelledOrders(path: string) {
  if (data.value) go(path, { status: 'CANCELLED', created_at_from: data.value.today_from, created_at_to: data.value.today_to })
}
function openTask(task: DashboardTask) {
  const outbound = task.task_type === 'PICK'
  if (task.order_id !== '0' && auth.hasPerm(outbound ? 'wms:outbound:view' : 'wms:inbound:view')) {
    go(`${outbound ? '/outbound' : '/inbound'}/orders/${task.order_id}`)
  } else go('/tasks', { order_id: task.order_id, task_type: task.task_type, status: task.status })
}
function age(value: string) {
  const minutes = Math.max(0, Math.floor((Date.parse(data.value?.generated_at || '') - Date.parse(value)) / 60000))
  if (!Number.isFinite(minutes)) return '—'
  if (minutes < 60) return `${minutes} 分钟`
  if (minutes < 1440) return `${Math.floor(minutes / 60)} 小时`
  return `${Math.floor(minutes / 1440)} 天`
}
function time(value: string) {
  return new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}
const taskTypes = computed(() => ['RECEIVE', 'PUTAWAY', 'PICK'].map(type => ({ type,
  pending: (data.value?.task_counts || []).filter(row => row.task_type === type && row.status === 'CREATED').reduce((n, row) => n + row.total, 0) })))
const recent = computed(() => [
  ...(data.value?.inbound?.recent || []).map(row => ({ ...row, type: '入库', path: '/inbound/orders/' + row.id })),
  ...(data.value?.outbound?.recent || []).map(row => ({ ...row, type: '出库', path: '/outbound/orders/' + row.id })),
].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at)).slice(0, 8))
const trend = computed(() => {
  if (!data.value) return []
  const start = data.value.trend_from.slice(0, 10)
  return Array.from({ length: 7 }, (_, i) => {
    const d = new Date(`${start}T00:00:00+08:00`)
    d.setUTCDate(d.getUTCDate() + i)
    const date = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(d)
    return { date, inbound: data.value?.inbound?.days.find(row => row.date === date)?.total || 0,
      outbound: data.value?.outbound?.days.find(row => row.date === date)?.total || 0 }
  })
})
const max = computed(() => Math.max(2, Math.ceil(Math.max(0, ...trend.value.flatMap(row => [row.inbound, row.outbound])) / 2) * 2))
const x = (i: number) => 38 + i * 88
const y = (value: number) => 122 - value / max.value * 96
const points = (key: 'inbound' | 'outbound') => trend.value.map((row, i) => `${x(i)},${y(row[key])}`).join(' ')
const shortcuts = [
  { label: '入库单', path: '/inbound/orders', perm: 'wms:inbound:view' },
  { label: '出库单', path: '/outbound/orders', perm: 'wms:outbound:view' },
  { label: '盘点单', path: '/stocktake/orders', perm: 'wms:stocktake:view' },
  { label: '库存查询', path: '/inventory', perm: 'wms:inventory' },
  { label: '任务中心', path: '/tasks', perm: 'wms:task' },
  { label: '货品管理', path: '/basic/skus', perm: 'wms:basic' },
]
</script>

<template>
  <div class="operations-dashboard">
    <header class="dashboard-heading">
      <div><span class="eyebrow">WAREHOUSE / OVERVIEW</span><h1>仓储运营总览</h1><p>{{ date }} · 掌握进度，安排作业</p></div>
      <div class="dashboard-tools">
        <span class="updated" :class="{ stale: error }" role="status">{{ error ? '更新失败 · ' : '更新于 ' }}{{ updated }}</span>
        <el-select v-if="auth.hasPerm('wms:basic')" v-model="warehouseId" aria-label="仪表盘仓库" placeholder="全部仓库" clearable filterable :disabled="!warehouses.length" style="width: 190px">
          <el-option v-for="w in warehouses" :key="w.id" :label="w.code + ' · ' + w.name" :value="w.id" />
        </el-select>
        <el-button :icon="Refresh" :loading="loading" aria-label="刷新仪表盘" @click="load()">刷新</el-button>
      </div>
    </header>

    <div v-if="!data" class="dashboard-placeholder">
      <el-skeleton v-if="loading" :rows="10" animated />
      <el-empty v-else description="仪表盘暂时无法加载，请刷新重试"><el-button type="primary" @click="load()">重新加载</el-button></el-empty>
    </div>
    <template v-else>
      <div class="dashboard-metrics">
        <button v-if="data.access.inbound" class="metric" @click="todayOrders('/inbound/orders')"><span>今日新建入库单 <el-icon><ArrowRight /></el-icon></span><strong>{{ count(data.inbound, undefined, true) }}<small>单</small></strong><p>按创建时间统计</p></button>
        <button v-if="data.access.outbound" class="metric" @click="todayOrders('/outbound/orders')"><span>今日新建出库单 <el-icon><ArrowRight /></el-icon></span><strong>{{ count(data.outbound, undefined, true) }}<small>单</small></strong><p>按创建时间统计</p></button>
        <button v-if="data.access.tasks" class="metric" @click="go('/tasks', { status: 'CREATED' })"><span>当前待执行任务 <el-icon><ArrowRight /></el-icon></span><strong>{{ pending }}<small>项</small></strong><p>另有 {{ running }} 项进行中</p></button>
        <button v-if="data.access.basic" class="metric" title="查看占用库位；占用率 = 占用库位 / 启用库位" @click="go('/basic/locations', { status: '2' })"><span>启用库位占用率 <el-icon><ArrowRight /></el-icon></span><strong>{{ data.locations?.enabled ? occupied : '—' }}<small>%</small></strong><p>{{ data.locations?.occupied || 0 }} / {{ data.locations?.enabled || 0 }} 个启用库位</p></button>
      </div>

      <div class="dashboard-grid">
        <section v-if="data.access.inbound || data.access.outbound" class="overview-panel workflow-panel">
          <div class="panel-heading"><h2>作业状态分布</h2><span>当前单据 · 可点击核对</span></div>
          <div class="panel-scroll workflow-body">
            <div v-if="data.inbound" class="workflow"><b>入库</b><div class="phase-list"><button v-for="state in inboundStates" :key="state" @click="go('/inbound/orders', { status: state })"><span>{{ statusText(state) }}</span><strong>{{ count(data.inbound, state) }}</strong></button></div></div>
            <div v-if="data.outbound" class="workflow"><b>出库</b><div class="phase-list"><button v-for="state in outboundStates" :key="state" @click="go('/outbound/orders', { status: state })"><span>{{ statusText(state) }}</span><strong>{{ count(data.outbound, state) }}</strong></button></div></div>
            <div class="cancelled-notes"><span>今日新建后已取消</span><button v-if="data.inbound" class="text-action" @click="cancelledOrders('/inbound/orders')">入库 {{ count(data.inbound, 'CANCELLED', true) }} 单 ↗</button><button v-if="data.outbound" class="text-action" @click="cancelledOrders('/outbound/orders')">出库 {{ count(data.outbound, 'CANCELLED', true) }} 单 ↗</button></div>
          </div>
        </section>

        <section v-if="data.access.tasks" class="overview-panel tasks-panel">
          <div class="panel-heading"><h2>作业待办</h2><el-button link type="primary" @click="go('/tasks')">任务中心 ↗</el-button></div>
          <div class="task-type-strip"><button v-for="item in taskTypes" :key="item.type" @click="go('/tasks', { task_type: item.type, status: 'CREATED' })">{{ taskTypeText(item.type) }} <b>{{ item.pending }}</b></button><button @click="go('/tasks', { status: 'IN_PROGRESS' })">进行中 <b>{{ running }}</b></button></div>
          <div class="panel-scroll task-list">
            <p v-if="aged" class="attention-note">{{ aged }} 项任务已创建超过 24 小时，尚未完成</p>
            <button v-for="task in data.oldest_tasks || []" :key="task.id" class="pending-task" @click="openTask(task)"><span class="task-mark">{{ taskTypeText(task.task_type).slice(0, 1) }}</span><span class="task-copy"><b>{{ task.order_no || task.task_no }}</b><small>{{ statusText(task.status) }} · 已创建 {{ age(task.created_at) }}</small></span><el-icon><ArrowRight /></el-icon></button>
            <p v-if="!data.oldest_tasks?.length" class="quiet-empty">当前没有待办，作业已处理完毕</p>
          </div>
        </section>

        <section v-if="data.access.inbound || data.access.outbound" class="overview-panel trend-panel">
          <div class="panel-heading"><h2>近 7 日业务趋势</h2><button class="text-action" @click="trendDetails = true">查看数据 ↗</button></div>
          <div class="trend-legend"><span v-if="data.inbound" class="inbound-legend">入库新建单量</span><span v-if="data.outbound" class="outbound-legend">出库新建单量</span><small>单位：单 · 含取消单</small></div>
          <svg class="trend-chart" viewBox="0 0 600 150" preserveAspectRatio="none" role="img" aria-label="近七日入库与出库新建单量，详细数值可点击查看数据">
            <g v-for="fraction in [0, 0.5, 1]" :key="fraction"><line x1="38" :y1="y(max * fraction)" x2="566" :y2="y(max * fraction)" class="chart-grid" /><text x="28" :y="y(max * fraction) + 4" text-anchor="end">{{ Math.ceil(max * fraction) }}</text></g>
            <polyline v-if="data.inbound" :points="points('inbound')" class="chart-inbound" />
            <polyline v-if="data.outbound" :points="points('outbound')" class="chart-outbound" />
            <g v-for="(day, i) in trend" :key="day.date"><text :x="x(i)" y="145" text-anchor="middle">{{ day.date.slice(5) }}</text><circle v-if="data.inbound" :cx="x(i)" :cy="y(day.inbound)" r="3" class="point-inbound"><title>{{ day.date }} 入库 {{ day.inbound }} 单</title></circle><circle v-if="data.outbound" :cx="x(i)" :cy="y(day.outbound)" r="3" class="point-outbound"><title>{{ day.date }} 出库 {{ day.outbound }} 单</title></circle></g>
          </svg>
        </section>

        <section v-if="data.access.inventory || data.access.basic || data.access.stocktake" class="overview-panel inventory-panel">
          <div class="panel-heading"><h2>库存与核对</h2><el-button v-if="data.access.inventory" link type="primary" @click="go('/inventory')">查看库存 ↗</el-button></div>
          <div class="panel-scroll inventory-body">
            <div v-if="data.inventory" class="inventory-facts"><div><strong>{{ data.inventory.skus }}</strong><span>有货 SKU</span></div><div><strong>{{ data.inventory.rows }}</strong><span>有货库存记录</span></div><div><strong>{{ data.inventory.allocated_rows }}</strong><span>含分配量记录</span></div></div>
            <div v-if="data.locations" class="location-meter"><div><span>启用库位使用情况</span><b>{{ data.locations.enabled ? occupied + '%' : '暂无启用库位' }}</b></div><div class="meter-track"><i :style="{ width: occupied + '%' }"></i></div><small>占用 {{ data.locations.occupied }} 个 · 空闲 {{ data.locations.enabled - data.locations.occupied }} 个</small></div>
            <button v-if="data.draft_stocktakes !== undefined" class="stocktake-link" @click="go('/stocktake/orders', { status: 'DRAFT' })"><span>待完成盘点单</span><b>{{ data.draft_stocktakes }} 单 ↗</b></button>
          </div>
        </section>
        <p v-if="!Object.values(data.access).some(Boolean)" class="quiet-empty">当前账号暂无仓储业务查看权限</p>
      </div>

      <section class="overview-panel recent-panel">
        <div class="panel-heading"><h2>最近新建单据</h2><nav class="shortcuts" aria-label="业务快捷入口"><button v-for="item in shortcuts.filter(item => auth.hasPerm(item.perm))" :key="item.path" class="text-action" @click="go(item.path)">{{ item.label }} ↗</button></nav></div>
        <div class="panel-scroll recent-list"><button v-for="item in recent" :key="item.path" class="recent-row" @click="go(item.path)"><span class="record-type">{{ item.type }}</span><b>{{ item.order_no }}</b><el-tag :type="statusTag(item.status)" size="small" effect="plain">{{ statusText(item.status) }}</el-tag><time>{{ time(item.created_at) }}</time></button><p v-if="!recent.length" class="quiet-empty">当前范围暂无入出库单据</p></div>
      </section>
    </template>

    <el-dialog v-model="trendDetails" class="business-dialog" title="近 7 日新建单量" width="600px" append-to-body>
      <p class="trend-explanation">{{ warehouseId ? '当前所选仓库' : '全部仓库' }} · 按北京时间创建日期统计，包含已取消单据，不代表实际收发货数量。</p>
      <el-table :data="trend" max-height="400"><el-table-column prop="date" label="日期" /><el-table-column v-if="data?.inbound" prop="inbound" label="新建入库单" /><el-table-column v-if="data?.outbound" prop="outbound" label="新建出库单" /></el-table>
      <template #footer><el-button @click="trendDetails = false">关闭</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.operations-dashboard { --chart-secondary: var(--gowms-metric-outbound, #38bdf8); height: 100%; min-height: 0; display: grid; grid-template-rows: auto auto minmax(0, 1fr) 112px; gap: 14px; color: var(--el-text-color-primary); }
.dashboard-heading { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.eyebrow { color: var(--el-color-primary); font-size: 10px; font-weight: 650; letter-spacing: .13em; }
h1 { font-size: 23px; margin: 5px 0 4px; letter-spacing: -.035em; } .dashboard-heading p { margin: 0; font-size: 12px; color: var(--el-text-color-secondary); }
.dashboard-tools { display: flex; gap: 10px; align-items: center; } .updated { font-size: 11px; color: var(--el-text-color-secondary); } .updated.stale { color: var(--el-color-warning); }
.dashboard-metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 14px; }
button { font: inherit; } .metric { min-width: 0; text-align: left; padding: 14px 18px; background: var(--el-bg-color); border: 1px solid var(--el-border-color-light); border-radius: 10px; color: inherit; cursor: pointer; }
.metric > span { display: flex; justify-content: space-between; font-size: 12px; color: var(--el-text-color-secondary); } .metric strong { display: block; margin-top: 7px; font-family: var(--gowms-num-font); font-size: 29px; line-height: 1.1; font-weight: 650; font-variant-numeric: tabular-nums; } .metric strong small { font-family: inherit; font-size: 12px; font-weight: 400; margin-left: 7px; color: var(--el-text-color-secondary); } .metric p { margin: 7px 0 0; font-size: 11px; color: var(--el-text-color-secondary); }
.metric:hover:not(:disabled) { border-color: var(--el-color-primary-light-5); } .metric:disabled { cursor: default; } button:focus-visible { outline: 2px solid var(--el-color-primary); outline-offset: 2px; }
.dashboard-grid { display: grid; min-height: 0; gap: 14px; grid-template-columns: minmax(0, 1.65fr) minmax(0, 1fr); grid-template-rows: minmax(0, 1fr) minmax(0, 1.1fr); }
.overview-panel { min-width: 0; min-height: 0; display: flex; flex-direction: column; border: 1px solid var(--el-border-color-light); border-radius: 10px; background: var(--el-bg-color); overflow: hidden; }
.panel-heading { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex: none; padding: 12px 16px 8px; min-height: 26px; } .panel-heading h2 { font-size: 13px; font-weight: 650; margin: 0; white-space: nowrap; } .panel-heading > span { font-size: 10px; color: var(--el-text-color-secondary); } .panel-heading .el-button { height: 20px; font-size: 11px; }
.panel-scroll { flex: 1; min-height: 0; overflow: auto; overscroll-behavior: contain; scrollbar-gutter: stable; } .workflow-body { display: grid; align-content: safe center; gap: 10px; padding: 4px 16px 12px; }
.workflow { display: flex; align-items: center; gap: 14px; } .workflow > b { font-size: 12px; font-weight: 500; flex: none; color: var(--el-text-color-secondary); }
.phase-list { display: flex; gap: 5px; flex: 1; } .phase-list button { min-width: 0; flex: 1; padding: 7px 4px; border: 0; border-radius: 6px; color: var(--el-text-color-primary); background: var(--el-fill-color-lighter); cursor: pointer; } .phase-list button:hover { background: var(--el-color-primary-light-9); } .phase-list span { display: block; font-size: 10px; color: var(--el-text-color-secondary); } .phase-list strong { display: block; margin-top: 4px; font-size: 17px; font-weight: 600; font-variant-numeric: tabular-nums; }
.cancelled-notes { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding-left: 38px; font-size: 10px; color: var(--el-text-color-secondary); }
.task-type-strip { display: flex; gap: 8px; padding: 0 16px 6px; flex: none; } .task-type-strip button { flex: 1; padding: 4px 8px; font-size: 11px; border: 0; color: var(--el-text-color-secondary); background: var(--el-fill-color-lighter); border-radius: 5px; cursor: pointer; } .task-type-strip b { margin-left: 7px; color: var(--el-text-color-primary); }
.task-list { padding: 0 16px 8px; } .attention-note { margin: 4px 0 6px; padding: 6px 8px; font-size: 11px; border-radius: 5px; background: var(--el-color-warning-light-9); color: var(--el-color-warning-dark-2, var(--el-color-warning)); }
.pending-task { display: flex; align-items: center; gap: 9px; text-align: left; width: 100%; padding: 8px 0; color: inherit; background: transparent; border: 0; border-bottom: 1px solid var(--el-border-color-lighter); cursor: pointer; } .task-mark { width: 27px; height: 27px; display: grid; place-items: center; background: var(--el-color-primary-light-9); color: var(--el-color-primary); border-radius: 6px; font-size: 11px; flex: none; } .task-copy { flex: 1; min-width: 0; } .task-copy b { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; font-weight: 500; } .task-copy small { display: block; font-size: 10px; margin-top: 4px; color: var(--el-text-color-secondary); } .pending-task > .el-icon { color: var(--el-text-color-placeholder); }
.trend-legend { display: flex; align-items: center; gap: 12px; padding: 0 16px; font-size: 10px; flex: none; } .trend-legend span::before { content: ''; display: inline-block; width: 7px; height: 7px; margin-right: 5px; border-radius: 50%; background: var(--el-color-primary); } .trend-legend .outbound-legend::before { background: var(--chart-secondary); } .trend-legend small { margin-left: auto; color: var(--el-text-color-secondary); }
.trend-chart { width: calc(100% - 24px); margin: 4px 12px 8px; flex: 1; min-height: 0; overflow: visible; } .trend-chart text { fill: var(--el-text-color-secondary); font-size: 10px; } .chart-grid { stroke: var(--el-border-color-lighter); stroke-dasharray: 3 4; } .chart-inbound, .chart-outbound { fill: none; stroke: var(--el-color-primary); stroke-width: 2; vector-effect: non-scaling-stroke; stroke-linejoin: round; } .chart-outbound { stroke: var(--chart-secondary); } .point-inbound { fill: var(--el-color-primary); } .point-outbound { fill: var(--chart-secondary); }
.inventory-body { padding: 4px 16px 12px; } .inventory-facts { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; } .inventory-facts strong { display: block; font-size: 22px; font-weight: 600; font-variant-numeric: tabular-nums; } .inventory-facts span { display: block; margin-top: 5px; font-size: 10px; color: var(--el-text-color-secondary); }
.location-meter { margin-top: 14px; } .location-meter > div:first-child { display: flex; justify-content: space-between; font-size: 10px; color: var(--el-text-color-secondary); } .location-meter b { color: var(--el-text-color-primary); font-weight: 500; } .meter-track { height: 6px; border-radius: 4px; background: var(--el-fill-color); overflow: hidden; margin: 7px 0 4px; } .meter-track i { display: block; height: 100%; background: var(--el-color-primary); border-radius: inherit; } .location-meter small { font-size: 10px; color: var(--el-text-color-secondary); }
.stocktake-link { width: 100%; display: flex; justify-content: space-between; padding: 8px 0 0; margin-top: 9px; border: 0; border-top: 1px solid var(--el-border-color-lighter); background: transparent; color: var(--el-text-color-secondary); font-size: 11px; cursor: pointer; } .stocktake-link b { color: var(--el-color-primary); font-weight: 500; }
.shortcuts { display: flex; flex-wrap: wrap; gap: 12px; } .text-action { background: transparent; border: 0; padding: 0; color: var(--el-color-primary); font-size: 11px; cursor: pointer; } .recent-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-content: start; gap: 0 24px; padding: 0 16px 8px; } .recent-row { display: flex; align-items: center; gap: 8px; min-width: 0; padding: 8px 0; border: 0; border-bottom: 1px solid var(--el-border-color-lighter); text-align: left; background: transparent; color: inherit; cursor: pointer; } .record-type { font-size: 10px; color: var(--el-text-color-secondary); flex: none; } .recent-row b { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; font-weight: 500; } .recent-row time { color: var(--el-text-color-secondary); font-size: 10px; flex: none; } .recent-row .el-tag { font-size: 10px; height: 18px; }
.quiet-empty { margin: auto; padding: 16px 0; font-size: 12px; color: var(--el-text-color-secondary); } .dashboard-placeholder { grid-column: 1 / -1; grid-row: 2 / -1; border-radius: 10px; padding: 24px; background: var(--el-bg-color); } .trend-explanation { font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.7; }
@media (min-width: 981px) and (max-height: 740px) { .operations-dashboard { gap: 10px; grid-template-rows: auto 76px minmax(0, 1fr) 72px; } .eyebrow, .dashboard-heading p { display: none; } h1 { margin: 0; font-size: 21px; } .metric { padding: 8px 14px; } .metric strong { margin-top: 4px; font-size: 24px; } .metric p { margin-top: 3px; } .dashboard-grid { gap: 10px; grid-template-rows: minmax(0, 1fr) minmax(0, 1.1fr); } .panel-heading { padding: 8px 12px 5px; min-height: 22px; } .workflow-body { padding: 0 12px 8px; gap: 7px; } .phase-list button { padding: 5px 2px; } .phase-list strong { font-size: 15px; margin-top: 2px; } .inventory-facts strong { font-size: 19px; } .location-meter { margin-top: 9px; } .recent-row { padding: 6px 0; } }
@media (max-width: 980px) { .operations-dashboard { height: auto; grid-template-rows: auto auto auto auto; } .dashboard-heading { align-items: flex-start; flex-direction: column; } .dashboard-tools { flex-wrap: wrap; width: 100%; } .dashboard-tools .updated { margin-right: auto; } .dashboard-grid { grid-template-rows: 205px 225px; grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr); } .recent-panel { max-height: 210px; } .recent-list { grid-template-columns: 1fr; } .shortcuts { gap: 8px; } }
@media (max-width: 680px) { .dashboard-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; } .metric { padding: 12px; } .metric strong { font-size: 26px; } .dashboard-grid { grid-template-columns: 1fr; grid-template-rows: none; grid-auto-rows: 220px; } .dashboard-tools .updated { flex-basis: 100%; } .panel-heading > span { font-size: 9px; } .recent-panel .panel-heading { align-items: flex-start; flex-direction: column; } .recent-row time { display: none; } }
</style>
