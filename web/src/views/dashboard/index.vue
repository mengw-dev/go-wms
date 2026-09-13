<script setup lang="ts">
import { onMounted, reactive } from 'vue'
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

function pickTotal(list: Promise<unknown>): Promise<number | null> {
  return list
    .then((res) => (res as { total: number }).total ?? 0)
    .catch(() => null)
}

onMounted(async () => {
  const results = await Promise.allSettled([
    pickTotal(listInboundOrders({ page: 1, page_size: 1 })),
    pickTotal(listOutboundOrders({ page: 1, page_size: 1 })),
    pickTotal(listTasks({ page: 1, page_size: 1, status: 'IN_PROGRESS' })),
    pickTotal(listInventory({ page: 1, page_size: 1 })),
  ])
  const [inbound, outbound, running, inventory] = results.map((r) =>
    r.status === 'fulfilled' ? r.value : null,
  )
  stats.inbound = inbound
  stats.outbound = outbound
  stats.runningTasks = running
  stats.inventory = inventory
})

const shortcuts = [
  { title: '入库单', desc: '创建 / 收货 / 上架', path: '/inbound/orders', icon: 'Download' },
  { title: '出库单', desc: '审核分配 / 拣货', path: '/outbound/orders', icon: 'Upload' },
  { title: '盘点单', desc: '快照 / 实盘 / 调整', path: '/stocktake/orders', icon: 'Tickets' },
  { title: '库存查询', desc: '明细 / 汇总 / 流水', path: '/inventory', icon: 'Coin' },
  { title: '任务中心', desc: '收货 / 上架 / 拣货', path: '/tasks', icon: 'List' },
  { title: '货品管理', desc: 'SKU / 条码', path: '/basic/skus', icon: 'Box' },
]

const highlights = [
  { title: '三数量库存模型', desc: '存量 = 可用 + 分配，每笔变动均有流水可追溯' },
  { title: '双重防超卖', desc: 'SELECT FOR UPDATE 行锁 + WHERE 条件更新兜底' },
  { title: '审核即 FIFO 分配', desc: '按批次先进先出锁库，出库按分配明细执行' },
  { title: 'Excel 异步导入', desc: '状态机 + CAS 抢占 + 悬挂任务定时补偿' },
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
      <el-button type="primary" @click="$router.push('/inbound/orders')">
        <el-icon class="btn-icon"><Plus /></el-icon>新建入库单
      </el-button>
    </div>

    <!-- 统计卡 -->
    <div class="stats">
      <div class="stat stat-1" @click="$router.push('/inbound/orders')">
        <div class="stat-icon"><el-icon :size="20"><Download /></el-icon></div>
        <div class="stat-body">
          <div class="label">入库单总数</div>
          <div class="num">{{ stats.inbound ?? '—' }}</div>
        </div>
      </div>
      <div class="stat stat-2" @click="$router.push('/outbound/orders')">
        <div class="stat-icon"><el-icon :size="20"><Upload /></el-icon></div>
        <div class="stat-body">
          <div class="label">出库单总数</div>
          <div class="num">{{ stats.outbound ?? '—' }}</div>
        </div>
      </div>
      <div class="stat stat-3" @click="$router.push('/tasks')">
        <div class="stat-icon"><el-icon :size="20"><Clock /></el-icon></div>
        <div class="stat-body">
          <div class="label">进行中任务</div>
          <div class="num">{{ stats.runningTasks ?? '—' }}</div>
        </div>
      </div>
      <div class="stat stat-4" @click="$router.push('/inventory')">
        <div class="stat-icon"><el-icon :size="20"><Coin /></el-icon></div>
        <div class="stat-body">
          <div class="label">库存记录数</div>
          <div class="num">{{ stats.inventory ?? '—' }}</div>
        </div>
      </div>
    </div>

    <el-row :gutter="16">
      <!-- 快捷入口 -->
      <el-col :xs="24" :md="14">
        <div class="card">
          <div class="card-head"><b>快捷入口</b></div>
          <div class="shortcuts">
            <div v-for="s in shortcuts" :key="s.path" class="shortcut" @click="$router.push(s.path)">
              <el-icon :size="22"><component :is="s.icon" /></el-icon>
              <b>{{ s.title }}</b>
              <span>{{ s.desc }}</span>
            </div>
          </div>
        </div>
      </el-col>
      <!-- 技术亮点 -->
      <el-col :xs="24" :md="10">
        <div class="card">
          <div class="card-head"><b>技术亮点</b><span class="head-sub">面试可讲</span></div>
          <ul class="hl-list">
            <li v-for="h in highlights" :key="h.title">
              <span class="hl-dot"></span>
              <div>
                <b>{{ h.title }}</b>
                <p>{{ h.desc }}</p>
              </div>
            </li>
          </ul>
        </div>
      </el-col>
    </el-row>
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

.head-sub {
  font-size: 12px;
  color: var(--el-color-primary);
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

/* 亮点列表 */
.hl-list {
  list-style: none;
  margin: 0;
  padding: 8px 18px 14px;
}

.hl-list li {
  display: flex;
  gap: 12px;
  padding: 11px 0;
  border-bottom: 1px dashed var(--el-border-color-lighter);
}

.hl-list li:last-child {
  border-bottom: none;
}

.hl-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-top: 6px;
  background: var(--el-color-primary);
  box-shadow: 0 0 6px var(--el-color-primary-light-5);
  flex-shrink: 0;
}

.hl-list b {
  font-size: 13px;
  color: var(--el-text-color-primary);
}

.hl-list p {
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
</style>
