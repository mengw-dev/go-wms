<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, TrendCharts } from '@element-plus/icons-vue'
import { getDemoActivity } from '@/api/demo'
import type { DemoActivitySnapshot } from '@/api/types'
import { statusTag, statusText, taskTypeText } from '@/constants'
import { formatTime } from '@/utils'
import { useAutoRefresh } from '@/composables/autoRefresh'

const router = useRouter()
const loading = ref(false)
const data = ref<DemoActivitySnapshot | null>(null)
const activeTab = ref('operations')

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    data.value = await getDemoActivity(30)
  } finally {
    if (!silent) loading.value = false
  }
}

function httpStatusType(status: number) {
  if (status >= 200 && status < 300) return 'success'
  if (status >= 400 && status < 500) return 'warning'
  return 'danger'
}

onMounted(() => load())
useAutoRefresh(() => load(true), 5000)
</script>

<template>
  <div v-loading="loading" class="activity-page">
    <div class="page-head">
      <div>
        <h2>业务操作记录</h2>
        <p>展示当前演示账号最近发生的接口操作、业务单据、任务和库存流水。</p>
      </div>
      <div class="head-actions">
        <el-button :icon="TrendCharts" @click="router.push('/demo/performance')">性能指标</el-button>
        <el-button :icon="Refresh" type="primary" @click="load()">立即刷新</el-button>
      </div>
    </div>

    <el-tabs v-if="data" v-model="activeTab" class="activity-tabs">
      <el-tab-pane label="接口操作" name="operations">
        <el-table :data="data.operations" border stripe>
          <el-table-column label="时间" width="170">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="method" label="方法" width="80" />
          <el-table-column prop="path" label="接口路径" min-width="220" />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="httpStatusType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="耗时" width="100" align="right">
            <template #default="{ row }">{{ row.cost_ms }} ms</template>
          </el-table-column>
          <el-table-column prop="ip" label="IP" min-width="120" />
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="业务单据" name="orders">
        <div class="section-title"><b>最近入库单</b><span>共 {{ data.inbound_orders.length }} 条</span></div>
        <el-table :data="data.inbound_orders" border stripe>
          <el-table-column prop="order_no" label="入库单号" min-width="170" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="expected_qty" label="应收" width="90" align="right" />
          <el-table-column prop="received_qty" label="已收" width="90" align="right" />
          <el-table-column label="创建时间" width="170">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>

        <div class="section-title"><b>最近出库单</b><span>共 {{ data.outbound_orders.length }} 条</span></div>
        <el-table :data="data.outbound_orders" border stripe>
          <el-table-column prop="order_no" label="出库单号" min-width="170" />
          <el-table-column prop="biz_order_no" label="业务单号" min-width="150" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="picked_qty" label="已拣" width="90" align="right" />
          <el-table-column label="创建时间" width="170">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>

        <div class="section-title"><b>最近盘点单</b><span>共 {{ data.stocktake_orders.length }} 条</span></div>
        <el-table :data="data.stocktake_orders" border stripe>
          <el-table-column prop="order_no" label="盘点单号" min-width="170" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="location_code" label="库位" min-width="120" />
          <el-table-column label="创建时间" width="170">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="任务与库存流水" name="tasks">
        <div class="section-title"><b>最近任务</b><span>共 {{ data.tasks.length }} 条</span></div>
        <el-table :data="data.tasks" border stripe>
          <el-table-column prop="task_no" label="任务号" min-width="170" />
          <el-table-column label="类型" width="90">
            <template #default="{ row }">{{ taskTypeText(row.task_type) }}</template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="order_no" label="单据号" min-width="150" />
          <el-table-column label="完成/目标" width="110" align="right">
            <template #default="{ row }">{{ row.done_qty }} / {{ row.target_qty }}</template>
          </el-table-column>
          <el-table-column label="时间" width="170">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>

        <div class="section-title"><b>最近库存流水</b><span>共 {{ data.inventory_trans.length }} 条</span></div>
        <el-table :data="data.inventory_trans" border stripe>
          <el-table-column prop="order_no" label="关联单号" min-width="150" />
          <el-table-column label="类型" width="100">
            <template #default="{ row }">{{ statusText(row.trans_type) }}</template>
          </el-table-column>
          <el-table-column prop="quantity_change" label="数量变化" width="100" align="right" />
          <el-table-column label="库存变化" width="140">
            <template #default="{ row }">{{ row.before_quantity }} → {{ row.after_quantity }}</template>
          </el-table-column>
          <el-table-column label="可用量变化" width="140">
            <template #default="{ row }">{{ row.available_before }} → {{ row.available_after }}</template>
          </el-table-column>
          <el-table-column label="时间" width="170">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-empty v-else description="暂无演示记录" />
  </div>
</template>

<style scoped>
.activity-page {
  min-height: 100%;
}

.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
  margin-bottom: 16px;
}

.page-head h2 {
  margin: 0 0 6px;
  font-size: 22px;
}

.page-head p {
  margin: 0;
  color: var(--el-text-color-secondary);
}

.head-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.activity-tabs {
  padding: 16px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  background: var(--el-bg-color);
  box-shadow: var(--el-box-shadow-light);
}

.section-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 18px 0 10px;
  color: var(--el-text-color-secondary);
}

.section-title:first-child {
  margin-top: 0;
}

.section-title b {
  color: var(--el-text-color-primary);
}

@media (max-width: 768px) {
  .page-head {
    display: block;
  }

  .head-actions {
    margin-top: 12px;
  }
}
</style>