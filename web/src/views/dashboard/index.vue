<script setup lang="ts">
import { computed, onMounted, reactive } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { listInboundOrders } from '@/api/inbound'
import { listOutboundOrders } from '@/api/outbound'
import { listTasks } from '@/api/task'
import { listInventory } from '@/api/inventory'

const auth = useAuthStore()

const stats = reactive({
  inbound: null as number | null,
  outbound: null as number | null,
  runningTasks: null as number | null,
  inventory: null as number | null,
})

type StatKey = keyof typeof stats

const statCards = [
  { key: 'inbound', label: '入库单总数', path: '/inbound/orders', perm: 'wms:inbound:view', icon: 'Download', className: 'stat-1' },
  { key: 'outbound', label: '出库单总数', path: '/outbound/orders', perm: 'wms:outbound:view', icon: 'Upload', className: 'stat-2' },
  { key: 'runningTasks', label: '进行中任务', path: '/tasks', perm: 'wms:task', icon: 'Clock', className: 'stat-3' },
  { key: 'inventory', label: '库存记录数', path: '/inventory', perm: 'wms:inventory', icon: 'Coin', className: 'stat-4' },
] as const

const visibleStatCards = computed(() => statCards.filter((card) => auth.hasPerm(card.perm)))

async function loadStat(key: StatKey, request: Promise<{ total: number }>) {
  try {
    const result = await request
    stats[key] = result.total ?? 0
  } catch {
    stats[key] = null
  }
}

onMounted(async () => {
  const jobs: Promise<void>[] = []
  if (auth.hasPerm('wms:inbound:view')) {
    jobs.push(loadStat('inbound', listInboundOrders({ page: 1, page_size: 1 })))
  }
  if (auth.hasPerm('wms:outbound:view')) {
    jobs.push(loadStat('outbound', listOutboundOrders({ page: 1, page_size: 1 })))
  }
  if (auth.hasPerm('wms:task')) {
    jobs.push(loadStat('runningTasks', listTasks({ page: 1, page_size: 1, status: 'IN_PROGRESS' })))
  }
  if (auth.hasPerm('wms:inventory')) {
    jobs.push(loadStat('inventory', listInventory({ page: 1, page_size: 1 })))
  }
  await Promise.all(jobs)
})

const shortcuts = [
  { title: '入库单', desc: '创建 / 收货 / 上架', path: '/inbound/orders', icon: 'Download', perm: 'wms:inbound:view' },
  { title: '出库单', desc: '审核分配 / 拣货', path: '/outbound/orders', icon: 'Upload', perm: 'wms:outbound:view' },
  { title: '盘点单', desc: '快照 / 实盘 / 调整', path: '/stocktake/orders', icon: 'Tickets', perm: 'wms:stocktake:view' },
  { title: '库存查询', desc: '明细 / 汇总 / 流水', path: '/inventory', icon: 'Coin', perm: 'wms:inventory' },
  { title: '任务中心', desc: '收货 / 上架 / 拣货', path: '/tasks', icon: 'List', perm: 'wms:task' },
  { title: '货品管理', desc: 'SKU / 条码', path: '/basic/skus', icon: 'Box', perm: 'wms:basic' },
]

const today = new Date().toLocaleDateString('zh-CN', {
  year: 'numeric',
  month: 'long',
  day: 'numeric',
  weekday: 'long',
})
</script>

<template>
  <div class="dash">
    <!-- 欢迎区 -->
    <div class="welcome">
      <div>
        <h2>欢迎回来，{{ auth.displayName }}</h2>
        <p>{{ today }} · 祝你工作顺利</p>
      </div>
      <el-button v-if="auth.hasPerm('wms:inbound:view')" type="primary" @click="$router.push('/inbound/orders')">
        <el-icon class="btn-icon"><Download /></el-icon>进入入库管理
      </el-button>
    </div>

    <!-- 统计卡 -->
    <div class="stats">
      <div
        v-for="card in visibleStatCards"
        :key="card.key"
        class="stat"
        :class="card.className"
        role="button"
        tabindex="0"
        :aria-label="card.label"
        @click="$router.push(card.path)"
        @keyup.enter="$router.push(card.path)"
      >
        <div class="stat-icon"><el-icon :size="20"><component :is="card.icon" /></el-icon></div>
        <div class="stat-body">
          <div class="label">{{ card.label }}</div>
          <div class="num">{{ stats[card.key] ?? '—' }}</div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-head"><b>快捷入口</b></div>
      <div class="shortcuts">
        <div
          v-for="s in shortcuts.filter((item) => auth.hasPerm(item.perm))"
          :key="s.path"
          class="shortcut"
          role="button"
          tabindex="0"
          :aria-label="s.title"
          @click="$router.push(s.path)"
          @keyup.enter="$router.push(s.path)"
        >
          <el-icon :size="22"><component :is="s.icon" /></el-icon>
          <b>{{ s.title }}</b>
          <span>{{ s.desc }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

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

.btn-icon {
  margin-right: 4px;
}

/* ---------- 统计卡：彩色顶边 + 图标 ---------- */
.stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
  margin-bottom: 16px;
}

@media (max-width: 1100px) {
  .stats {
    grid-template-columns: repeat(2, 1fr);
  }
}

.stat {
  position: relative;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: var(--gowms-radius-card);
  padding: 18px;
  display: flex;
  gap: 14px;
  cursor: pointer;
  overflow: hidden;
  transition: transform 0.2s, box-shadow 0.2s;
}

.stat:hover {
  transform: translateY(-2px);
  box-shadow: var(--el-box-shadow);
}

.stat::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: var(--accent);
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--accent);
  background: color-mix(in srgb, var(--accent) 12%, transparent);
  flex-shrink: 0;
}

.stat-body {
  min-width: 0;
}

.stat .label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.stat .num {
  font-family: var(--gowms-num-font);
  font-size: 26px;
  font-weight: 700;
  line-height: 1.3;
  color: var(--el-text-color-primary);
  font-variant-numeric: tabular-nums;
}

.stat-1 {
  --accent: #6366f1;
}

.stat-2 {
  --accent: #0ea5e9;
}

.stat-3 {
  --accent: #f59e0b;
}

.stat-4 {
  --accent: #10b981;
}

html.dark .stat-1 {
  --accent: #818cf8;
}

html.dark .stat-2 {
  --accent: #38bdf8;
}

html.dark .stat-3 {
  --accent: #fbbf24;
}

html.dark .stat-4 {
  --accent: #34d399;
}

/* ---------- 卡片通用 ---------- */
.card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: var(--gowms-radius-card);
  box-shadow: var(--el-box-shadow-light);
  height: 100%;
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

/* 快捷入口宫格 */
.shortcuts {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  padding: 16px;
}

@media (max-width: 640px) {
  .shortcuts {
    grid-template-columns: repeat(2, 1fr);
  }
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
