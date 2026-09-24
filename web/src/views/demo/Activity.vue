<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Refresh, TrendCharts } from '@element-plus/icons-vue'
import { getDemoActivity } from '@/api/demo'
import type { DemoActivitySnapshot } from '@/api/types'
import { statusTag, statusText, taskTypeText } from '@/constants'
import { formatTime } from '@/utils'
import {
  clearDemoEvidence,
  filterDemoActivity,
  readDemoEvidence,
  resolveDemoEvidenceFocus,
} from '@/utils/demoEvidence'
import { useAutoRefresh } from '@/composables/autoRefresh'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const data = ref<DemoActivitySnapshot | null>(null)
const activeTab = ref('evidence')
const context = ref(readDemoEvidence())

const focus = computed(() => resolveDemoEvidenceFocus(route.query as Record<string, unknown>, context.value))
const focused = computed(() => Boolean(focus.value.scenario || focus.value.startedAt || focus.value.orderNos.length))
const evidence = computed(() => (data.value ? filterDemoActivity(data.value, focus.value) : null))
const evidenceCount = computed(() => {
  if (!evidence.value) return 0
  return (
    evidence.value.inbound_orders.length +
    evidence.value.outbound_orders.length +
    evidence.value.stocktake_orders.length +
    evidence.value.tasks.length +
    evidence.value.inventory_trans.length
  )
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
const relatedNumbers = computed(() => {
  const values = [...focus.value.orderNos]
  for (const ids of Object.values(focus.value.orderIds)) values.push(...ids)
  return Array.from(new Set(values.filter(Boolean)))
})
const evidenceOverview = computed(() => {
  const current = evidence.value
  if (!current) return []

  const orders = [
    ...current.inbound_orders,
    ...current.outbound_orders,
    ...current.stocktake_orders,
  ]
  const completedOrders = orders.filter((order) => order.status === 'COMPLETED').length
  const quantityChange = current.inventory_trans.reduce(
    (total, item) => total + item.quantity_change,
    0,
  )

  return [
    { label: '入库单', value: String(current.inbound_orders.length) },
    { label: '出库单', value: String(current.outbound_orders.length) },
    { label: '盘点单', value: String(current.stocktake_orders.length) },
    { label: '作业任务', value: String(current.tasks.length) },
    { label: '库存流水', value: String(current.inventory_trans.length) },
    { label: '业务状态', value: `${completedOrders} / ${orders.length} 已完成` },
    { label: '数量变化', value: `${quantityChange > 0 ? '+' : ''}${quantityChange} 件` },
  ]
})

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    data.value = await getDemoActivity(50)
  } finally {
    if (!silent) loading.value = false
  }
}

function showAll() {
  clearDemoEvidence()
  context.value = null
  void router.replace('/demo/activity')
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
        <h2>{{ focused ? '本次业务执行证据' : '业务证据' }}</h2>
        <p v-if="focused">
          {{ focus.summary || '从最近一次自动演示结果进入，页面只聚焦该次执行产生的真实业务对象。' }}
        </p>
        <p v-else>完成一次自动演示或手动业务后，可在这里核对对应单据、任务和库存流水。</p>
      </div>
      <div class="head-actions">
        <el-button v-if="focused" @click="showAll">查看全部记录</el-button>
        <el-button :icon="TrendCharts" @click="router.push('/demo/performance')">运行状态与指标</el-button>
        <el-button :icon="Refresh" type="primary" @click="load()">立即刷新</el-button>
      </div>
    </div>

    <section v-if="focused" class="focus-card">
      <div>
        <span>本次演示</span>
        <b>{{ scenarioLabel }}</b>
      </div>
      <div>
        <span>执行时间</span>
        <b>{{ formatTime(focus.startedAt) }}</b>
      </div>
      <div>
        <span>相关业务编号</span>
        <b>{{ relatedNumbers.length ? relatedNumbers.join('、') : '按执行时间聚焦' }}</b>
      </div>
    </section>

    <template v-if="evidence">
      <el-tabs v-model="activeTab" class="activity-tabs">
        <el-tab-pane label="业务证据" name="evidence">
          <section class="evidence-overview">
            <div class="section-title">
              <b>业务结果概览</b>
              <span>优先展示与本次执行关联的业务对象</span>
            </div>
            <div class="overview-grid">
              <div v-for="item in evidenceOverview" :key="item.label">
                <span>{{ item.label }}</span>
                <b>{{ item.value }}</b>
              </div>
            </div>
          </section>

          <div v-if="focused" class="evidence-rule">
            <el-alert
              title="优先按本次业务编号关联；缺少直接编号的记录再使用执行时间窗辅助过滤。"
              type="success"
              :closable="false"
              show-icon
            />
            <p>业务单据、任务和库存流水优先按业务编号关联；接口调用等无法直接绑定业务对象的记录使用本次执行时间窗辅助过滤。</p>
          </div>
          <el-alert
            v-else
            title="当前展示本演示账号最近的业务记录；从自动演示结果点击“查看业务证据”可自动聚焦单次执行。"
            type="info"
            :closable="false"
            show-icon
          />

          <section class="evidence-section">
            <div class="section-title"><b>业务单据</b><span>本次关联 {{ evidence.inbound_orders.length + evidence.outbound_orders.length + evidence.stocktake_orders.length }} 条</span></div>
            <el-table v-if="evidence.inbound_orders.length" :data="evidence.inbound_orders" border stripe>
              <el-table-column label="类型" width="90"><template #default>入库单</template></el-table-column>
              <el-table-column label="单号" min-width="170">
                <template #default="{ row }"><el-link type="primary" @click="router.push(`/inbound/orders/${row.id}`)">{{ row.order_no }}</el-link></template>
              </el-table-column>
              <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag></template></el-table-column>
              <el-table-column prop="expected_qty" label="应收" width="90" align="right" />
              <el-table-column prop="received_qty" label="已收" width="90" align="right" />
              <el-table-column label="创建时间" width="170"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
            </el-table>

            <el-table v-if="evidence.outbound_orders.length" :data="evidence.outbound_orders" border stripe>
              <el-table-column label="类型" width="90"><template #default>出库单</template></el-table-column>
              <el-table-column label="单号" min-width="170">
                <template #default="{ row }"><el-link type="primary" @click="router.push(`/outbound/orders/${row.id}`)">{{ row.order_no }}</el-link></template>
              </el-table-column>
              <el-table-column prop="biz_order_no" label="业务单号" min-width="150" />
              <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag></template></el-table-column>
              <el-table-column prop="allocated_qty" label="已分配" width="90" align="right" />
              <el-table-column prop="picked_qty" label="已拣" width="90" align="right" />
              <el-table-column label="创建时间" width="170"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
            </el-table>

            <el-table v-if="evidence.stocktake_orders.length" :data="evidence.stocktake_orders" border stripe>
              <el-table-column label="类型" width="90"><template #default>盘点单</template></el-table-column>
              <el-table-column label="单号" min-width="170">
                <template #default="{ row }"><el-link type="primary" @click="router.push(`/stocktake/orders/${row.id}`)">{{ row.order_no }}</el-link></template>
              </el-table-column>
              <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag></template></el-table-column>
              <el-table-column prop="location_code" label="库位" min-width="120" />
              <el-table-column label="创建时间" width="170"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
            </el-table>
          </section>

          <section class="evidence-section">
            <div class="section-title"><b>作业任务</b><span>本次关联 {{ evidence.tasks.length }} 条</span></div>
            <el-table v-if="evidence.tasks.length" :data="evidence.tasks" border stripe>
              <el-table-column prop="task_no" label="任务号" min-width="170" />
              <el-table-column label="类型" width="90"><template #default="{ row }">{{ taskTypeText(row.task_type) }}</template></el-table-column>
              <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag></template></el-table-column>
              <el-table-column prop="order_no" label="关联单号" min-width="150" />
              <el-table-column label="完成/目标" width="110" align="right"><template #default="{ row }">{{ row.done_qty }} / {{ row.target_qty }}</template></el-table-column>
              <el-table-column label="创建时间" width="170"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
            </el-table>
            <div v-else class="empty-inline">本次执行没有单独生成作业任务。</div>
          </section>

          <section class="evidence-section">
            <div class="section-title"><b>库存流水</b><span>本次关联 {{ evidence.inventory_trans.length }} 条</span></div>
            <el-table v-if="evidence.inventory_trans.length" :data="evidence.inventory_trans" border stripe>
              <el-table-column prop="order_no" label="关联单号" min-width="150" />
              <el-table-column label="类型" width="100"><template #default="{ row }">{{ statusText(row.trans_type) }}</template></el-table-column>
              <el-table-column prop="quantity_change" label="数量变化" width="100" align="right" />
              <el-table-column label="库存变化" width="140"><template #default="{ row }">{{ row.before_quantity }} → {{ row.after_quantity }}</template></el-table-column>
              <el-table-column label="可用量变化" width="140"><template #default="{ row }">{{ row.available_before }} → {{ row.available_after }}</template></el-table-column>
              <el-table-column prop="task_no" label="任务号" min-width="150" />
              <el-table-column label="时间" width="170"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
            </el-table>
            <div v-else class="empty-inline">本次执行没有库存数量变更。</div>
          </section>

          <el-empty
            v-if="evidenceCount === 0"
            :description="focused ? '暂未找到这次执行的业务对象。请确认演示已成功完成，或点击“查看全部记录”。' : '完成一次自动演示或手动流程后，可以在这里查看对应业务证据。'"
          />
        </el-tab-pane>

        <el-tab-pane label="接口调用记录" name="operations">
          <p class="tab-note">接口记录用于排查调用过程，不作为业务结果的第一层证据。</p>
          <el-table :data="evidence.operations" border stripe>
            <el-table-column label="时间" width="170"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
            <el-table-column prop="method" label="方法" width="80" />
            <el-table-column prop="path" label="接口路径" min-width="260" />
            <el-table-column label="状态" width="90"><template #default="{ row }"><el-tag :type="httpStatusType(row.status)" size="small">{{ row.status }}</el-tag></template></el-table-column>
            <el-table-column label="耗时" width="100" align="right"><template #default="{ row }">{{ row.cost_ms }} ms</template></el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </template>
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
  max-width: 760px;
  margin: 0;
  color: var(--el-text-color-secondary);
  line-height: 1.7;
}

.head-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
  flex-shrink: 0;
}

.focus-card {
  display: grid;
  grid-template-columns: 0.8fr 1fr 1.6fr;
  gap: 12px;
  margin-bottom: 14px;
  padding: 16px;
  border: 1px solid var(--el-color-success-light-7);
  border-radius: 12px;
  background: var(--el-color-success-light-9);
}

.focus-card div {
  min-width: 0;
}

.focus-card span,
.focus-card b {
  display: block;
}

.focus-card span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.focus-card b {
  margin-top: 5px;
  overflow-wrap: anywhere;
  color: var(--el-text-color-primary);
}

.activity-tabs {
  padding: 16px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  background: var(--el-bg-color);
  box-shadow: var(--el-box-shadow-light);
}

.evidence-overview {
  margin-bottom: 18px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 8px;
}

.overview-grid div {
  min-width: 0;
  padding: 12px;
  border-radius: 9px;
  background: var(--el-fill-color-light);
}

.overview-grid span,
.overview-grid b {
  display: block;
}

.overview-grid span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.overview-grid b {
  margin-top: 5px;
  overflow-wrap: anywhere;
  color: var(--el-text-color-primary);
  font-family: var(--gowms-num-font);
  font-size: 17px;
}

.evidence-section {
  margin-top: 18px;
}

.evidence-rule {
  display: grid;
  gap: 8px;
}

.evidence-rule p {
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.65;
}

.section-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 0 0 10px;
  color: var(--el-text-color-secondary);
}

.section-title b {
  color: var(--el-text-color-primary);
}

.empty-inline,
.tab-note {
  padding: 12px 14px;
  border-radius: 8px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-lighter);
  font-size: 13px;
}

.tab-note {
  margin: 0 0 12px;
}

@media (max-width: 768px) {
  .page-head {
    display: block;
  }

  .head-actions {
    justify-content: flex-start;
    margin-top: 12px;
  }

  .focus-card,
  .overview-grid {
    grid-template-columns: 1fr;
  }
}
</style>
