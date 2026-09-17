<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { listInboundOrders } from '@/api/inbound'
import { listOutboundOrders } from '@/api/outbound'
import { listTasks } from '@/api/task'
import type { TaskItem } from '@/api/types'
import { Download, Upload, Clock, Warning } from '@element-plus/icons-vue'

const router = useRouter()
const auth = useAuthStore()

// ---------- 今日零点时间（本地时间，避免 toISOString 的 UTC 偏移）----------
function pad(n: number) { return String(n).padStart(2, '0') }
function todayRange() {
  const now = new Date()
  const from = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} 00:00:00`
  const to = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`
  return { from, to }
}
const today = todayRange()

// ---------- 统计卡 ----------
interface StatCard {
  key: string
  label: string
  icon: typeof Download
  color: string
  accent: string
  queryPromise: Promise<number>
  path: string
  /** 路径附加 query 用于点击跳转带筛选 */
  pathQuery?: string
}

const cards = ref<StatCard[]>([])

async function buildStatCards() {
  const cards: StatCard[] = []
  if (auth.hasPerm('wms:inbound:view')) {
    cards.push({
      key: 'todayInbound', label: '今日入库单', icon: Download, color: '#6366f1',
      accent: '#818cf8', path: '/inbound/orders',
      queryPromise: listInboundOrders({ page: 1, page_size: 1, created_at_from: today.from, created_at_to: today.to }).then(r => r.total ?? 0),
    })
    cards.push({
      key: 'abnormalInbound', label: '异常入库', icon: Warning, color: '#ef4444',
      accent: '#f87171', path: '/inbound/orders', pathQuery: '?status=CANCELLED',
      queryPromise: listInboundOrders({ page: 1, page_size: 1, status: 'CANCELLED', created_at_from: today.from, created_at_to: today.to }).then(r => r.total ?? 0),
    })
  }
  if (auth.hasPerm('wms:outbound:view')) {
    cards.push({
      key: 'todayOutbound', label: '今日出库单', icon: Upload, color: '#0ea5e9',
      accent: '#38bdf8', path: '/outbound/orders',
      queryPromise: listOutboundOrders({ page: 1, page_size: 1, created_at_from: today.from, created_at_to: today.to }).then(r => r.total ?? 0),
    })
    cards.push({
      key: 'abnormalOutbound', label: '异常出库', icon: Warning, color: '#ef4444',
      accent: '#f87171', path: '/outbound/orders', pathQuery: '?status=CANCELLED',
      queryPromise: listOutboundOrders({ page: 1, page_size: 1, status: 'CANCELLED', created_at_from: today.from, created_at_to: today.to }).then(r => r.total ?? 0),
    })
  }
  if (auth.hasPerm('wms:task')) {
    cards.push({
      key: 'runningTasks', label: '进行中任务', icon: Clock, color: '#f59e0b',
      accent: '#fbbf24', path: '/tasks', pathQuery: '?status=IN_PROGRESS',
      queryPromise: listTasks({ page: 1, page_size: 1, status: 'IN_PROGRESS' }).then(r => r.total ?? 0),
    })
    cards.push({
      key: 'pendingTasks', label: '待办任务', icon: Clock, color: '#f97316',
      accent: '#fb923c', path: '/tasks', pathQuery: '?status=CREATED',
      queryPromise: listTasks({ page: 1, page_size: 1, status: 'CREATED' }).then(r => r.total ?? 0),
    })
  }
  return cards
}

const stats = reactive<Record<string, number | null>>({})

async function loadStats() {
  const built = await buildStatCards()
  cards.value = built
  await Promise.all(built.map(async c => {
    try { stats[c.key] = await c.queryPromise } catch { stats[c.key] = null }
  }))
}

function go(card: StatCard) {
  router.push(card.path + (card.pathQuery ?? ''))
}

// ---------- 待办任务 ----------
const pendingTasks = ref<TaskItem[]>([])
const pendingTotal = ref(0)
const runningTasks = ref<TaskItem[]>([])

const TASK_TYPE_LABEL: Record<string, string> = {
  RECEIVE: '收货',
  PUTAWAY: '上架',
  PICK: '拣货',
}

const taskGroups = computed(() => {
  const groups: Record<string, TaskItem[]> = {}
  for (const t of pendingTasks.value) {
    const k = TASK_TYPE_LABEL[t.task_type] ?? t.task_type
    if (!groups[k]) groups[k] = []
    groups[k].push(t)
  }
  return Object.entries(groups)
})

async function loadTasks() {
  try {
    if (auth.hasPerm('wms:task')) {
      const [cre, run] = await Promise.all([
        listTasks({ page: 1, page_size: 10, status: 'CREATED' }),
        listTasks({ page: 1, page_size: 5, status: 'IN_PROGRESS' }),
      ])
      pendingTasks.value = cre.list ?? []
      // 未完成总数 = 待办 + 进行中（不含 CANCELLED / COMPLETED）
      pendingTotal.value = (cre.total ?? 0) + (run.total ?? 0)
      runningTasks.value = run.list ?? []
    }
  } catch {
    // 任务数据加载失败时保持空态，不影响仪表盘其余区域展示
  }
}

function goTaskList(status?: string) {
  router.push('/tasks' + (status ? `?status=${status}` : ''))
}

// ---------- 最近单据 ----------
const recentInbound = ref<{ id: string; order_no: string; status: string; created_at: string }[]>([])
const recentOutbound = ref<{ id: string; order_no: string; status: string; created_at: string }[]>([])

const STATUS_TAG: Record<string, string> = {
  DRAFT: '草稿', SUBMITTED: '待审核', APPROVED: '已审核', RECEIVING: '收货中',
  PUTAWAY: '上架中', SHIPPED: '已发货', COMPLETED: '已完成', CANCELLED: '已取消',
}

const STATUS_TAG_CLASS: Record<string, string> = {
  DRAFT: 'info', SUBMITTED: 'warning', APPROVED: 'primary', RECEIVING: 'primary',
  PUTAWAY: 'warning', SHIPPED: 'success', COMPLETED: 'success', CANCELLED: 'danger',
}

function formatTime(s: string) {
  if (!s) return ''
  const d = new Date(s)
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

async function loadRecent() {
  try {
    const jobs: Promise<void>[] = []
    if (auth.hasPerm('wms:inbound:view')) {
      jobs.push(listInboundOrders({ page: 1, page_size: 5 }).then(r => {
        recentInbound.value = (r.list ?? []).map(o => ({
          id: String(o.id), order_no: o.order_no, status: o.status, created_at: o.created_at,
        }))
      }))
    }
    if (auth.hasPerm('wms:outbound:view')) {
      jobs.push(listOutboundOrders({ page: 1, page_size: 5 }).then(r => {
        recentOutbound.value = (r.list ?? []).map(o => ({
          id: String(o.id), order_no: o.order_no, status: o.status, created_at: o.created_at,
        }))
      }))
    }
    await Promise.all(jobs)
  } catch {
    // 最近单据加载失败时保持空列表展示
  }
}

onMounted(async () => {
  await Promise.all([loadStats(), loadTasks(), loadRecent()])
})

// ---------- 欢迎区 ----------
const todayDate = new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' })
</script>

<template>
  <div class="dash">
    <!-- 欢迎区 -->
    <div class="welcome">
      <div>
        <h2>欢迎回来，{{ auth.displayName }}</h2>
        <p>{{ todayDate }} · 祝你工作顺利</p>
      </div>
      <div v-if="auth.hasPerm('wms:task')" class="welcome-right">
        <el-button type="primary" @click="router.push('/tasks')">
          <el-icon class="btn-icon"><Clock /></el-icon>
          任务中心
          <el-badge v-if="pendingTotal > 0" :value="pendingTotal" class="task-badge" />
        </el-button>
      </div>
    </div>

    <!-- 统计卡：今日口径 -->
    <div class="stats">
      <div
        v-for="card in cards"
        :key="card.key"
        class="stat"
        :style="{ '--accent': card.color }"
        role="button"
        tabindex="0"
        @click="go(card)"
        @keyup.enter="go(card)"
      >
        <div class="stat-icon" :style="{ background: card.color + '1f', color: card.color }">
          <el-icon :size="20"><component :is="card.icon" /></el-icon>
        </div>
        <div class="stat-body">
          <div class="label">{{ card.label }}</div>
          <div class="num" :style="{ color: card.color }">{{ stats[card.key] ?? '—' }}</div>
        </div>
      </div>
    </div>

    <!-- 下区：左待办 / 中最近入库 / 右最近出库 -->
    <div class="grid-3">
      <!-- 待办任务 -->
      <div class="card">
        <div class="card-head">
          <b>待办任务</b>
          <el-button v-if="auth.hasPerm('wms:task')" link type="primary" @click="goTaskList('CREATED')">全部 →</el-button>
        </div>
        <div class="card-body">
          <template v-if="taskGroups.length > 0">
            <div v-for="[type, list] in taskGroups" :key="type" class="task-group">
              <div class="task-type">
                {{ type }}
                <el-tag size="small" type="warning">{{ list.length }}</el-tag>
              </div>
              <div
                v-for="t in list"
                :key="t.id"
                class="task-row"
                @click="router.push('/tasks')"
              >
                <span class="task-no">{{ t.task_no }}</span>
                <span class="task-order">{{ t.order_no }}</span>
                <el-tag size="small" type="info">{{ TASK_TYPE_LABEL[t.task_type] ?? t.task_type }}</el-tag>
              </div>
            </div>
          </template>
          <el-empty v-else description="暂无待办任务" :image-size="80" />
        </div>
      </div>

      <!-- 最近入库单 -->
      <div class="card">
        <div class="card-head">
          <b>最近入库单</b>
          <el-button v-if="auth.hasPerm('wms:inbound:view')" link type="primary" @click="router.push('/inbound/orders')">全部 →</el-button>
        </div>
        <div class="card-body">
          <template v-if="recentInbound.length > 0">
            <div
              v-for="o in recentInbound"
              :key="o.id"
              class="list-row"
              @click="router.push('/inbound/orders/' + o.id)"
            >
              <span class="row-no">{{ o.order_no }}</span>
              <el-tag size="small" :type="STATUS_TAG_CLASS[o.status] ?? 'info'">{{ STATUS_TAG[o.status] ?? o.status }}</el-tag>
              <span class="row-time">{{ formatTime(o.created_at) }}</span>
            </div>
          </template>
          <el-empty v-else description="暂无入库单" :image-size="80" />
        </div>
      </div>

      <!-- 最近出库单 -->
      <div class="card">
        <div class="card-head">
          <b>最近出库单</b>
          <el-button v-if="auth.hasPerm('wms:outbound:view')" link type="primary" @click="router.push('/outbound/orders')">全部 →</el-button>
        </div>
        <div class="card-body">
          <template v-if="recentOutbound.length > 0">
            <div
              v-for="o in recentOutbound"
              :key="o.id"
              class="list-row"
              @click="router.push('/outbound/orders/' + o.id)"
            >
              <span class="row-no">{{ o.order_no }}</span>
              <el-tag size="small" :type="STATUS_TAG_CLASS[o.status] ?? 'info'">{{ STATUS_TAG[o.status] ?? o.status }}</el-tag>
              <span class="row-time">{{ formatTime(o.created_at) }}</span>
            </div>
          </template>
          <el-empty v-else description="暂无出库单" :image-size="80" />
        </div>
      </div>
    </div>

    <!-- 快捷入口（保留，放在最下面） -->
    <div class="card shortcuts-card">
      <div class="card-head"><b>快捷入口</b></div>
      <div class="shortcuts">
        <div
          v-for="s in shortcuts.filter((item) => auth.hasPerm(item.perm))"
          :key="s.path"
          class="shortcut"
          role="button"
          tabindex="0"
          @click="router.push(s.path)"
        >
          <el-icon :size="22"><component :is="s.icon" /></el-icon>
          <b>{{ s.title }}</b>
          <span>{{ s.desc }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
// shortcuts 数据放在外层避免 template 里用 reactive
const shortcuts = [
  { title: '入库单', desc: '创建 / 收货 / 上架', path: '/inbound/orders', icon: 'Download', perm: 'wms:inbound:view' },
  { title: '出库单', desc: '审核分配 / 拣货', path: '/outbound/orders', icon: 'Upload', perm: 'wms:outbound:view' },
  { title: '盘点单', desc: '快照 / 实盘 / 调整', path: '/stocktake/orders', icon: 'Tickets', perm: 'wms:stocktake:view' },
  { title: '库存查询', desc: '明细 / 汇总 / 流水', path: '/inventory', icon: 'Coin', perm: 'wms:inventory' },
  { title: '任务中心', desc: '收货 / 上架 / 拣货', path: '/tasks', icon: 'List', perm: 'wms:task' },
  { title: '货品管理', desc: 'SKU / 条码', path: '/basic/skus', icon: 'Box', perm: 'wms:basic' },
]
</script>

<style scoped>
.welcome {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.welcome h2 {
  margin: 0 0 4px;
  font-size: 20px;
  color: var(--el-text-color-primary);
}

.welcome p {
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.task-badge {
  margin-left: 6px;
}
.task-badge :deep(.el-badge__content) {
  border: none !important;
}

/* ---------- 统计卡 ---------- */
.stats {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 14px;
  margin-bottom: 16px;
}

.stat {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: var(--gowms-radius-card);
  padding: 18px;
  display: flex;
  gap: 14px;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
  border-top: 3px solid var(--accent);
}

.stat:hover {
  transform: translateY(-2px);
  box-shadow: var(--el-box-shadow);
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.stat-body { min-width: 0; }
.stat .label { font-size: 12px; color: var(--el-text-color-secondary); }
.stat .num {
  font-family: var(--gowms-num-font);
  font-size: 26px;
  font-weight: 700;
  line-height: 1.3;
  font-variant-numeric: tabular-nums;
}

/* ---------- 3 列网格 ---------- */
.grid-3 {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 14px;
  margin-bottom: 16px;
}

@media (max-width: 1100px) {
  .grid-3 { grid-template-columns: repeat(2, 1fr); }
}

@media (max-width: 700px) {
  .grid-3 { grid-template-columns: 1fr; }
}

/* ---------- 卡片 ---------- */
.card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: var(--gowms-radius-card);
  box-shadow: var(--el-box-shadow-light);
  min-height: 240px;
  display: flex;
  flex-direction: column;
}

.card-head {
  padding: 14px 18px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-head b {
  font-size: 14px;
  color: var(--el-text-color-primary);
}

.card-body {
  padding: 12px 0;
  flex: 1;
  overflow-y: auto;
}

/* ---------- 待办任务 ---------- */
.task-group { margin-bottom: 10px; }
.task-group:last-child { margin-bottom: 0; }

.task-type {
  padding: 4px 18px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  display: flex;
  align-items: center;
  gap: 6px;
}

.task-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 18px;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s;
}

.task-row:hover {
  background: var(--el-color-primary-light-9);
}

.task-no {
  font-family: monospace;
  color: var(--el-text-color-primary);
}

.task-order {
  flex: 1;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ---------- 列表行（最近单据） ---------- */
.list-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 18px;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s;
}

.list-row:hover {
  background: var(--el-color-primary-light-9);
}

.row-no {
  flex: 1;
  font-family: monospace;
  color: var(--el-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-time {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}

/* ---------- 快捷入口 ---------- */
.shortcuts-card {
  min-height: auto;
}

.shortcuts {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 12px;
  padding: 16px;
}

@media (max-width: 1100px) {
  .shortcuts { grid-template-columns: repeat(3, 1fr); }
}

@media (max-width: 640px) {
  .shortcuts { grid-template-columns: repeat(2, 1fr); }
}

.shortcut {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 18px 8px 14px;
  border-radius: 10px;
  border: 1px solid var(--el-border-color-lighter);
  cursor: pointer;
  transition: all 0.2s;
  color: var(--el-color-primary);
}

.shortcut:hover {
  border-color: var(--el-color-primary-light-5);
  background: var(--el-color-primary-light-9);
  transform: translateY(-2px);
}

.shortcut b {
  font-size: 13px;
  color: var(--el-text-color-primary);
}

.shortcut span {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
</style>
