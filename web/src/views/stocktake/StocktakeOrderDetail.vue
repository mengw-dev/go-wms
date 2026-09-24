<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveStocktakeOrder,
  cancelStocktakeOrder,
  getStocktakeOrder,
  submitStocktakeActual,
} from '@/api/stocktake'
import type { EntityID, StocktakeDetailItem, StocktakeOrderDetail as StocktakeDetailData } from '@/api/types'
import { statusTag, statusText } from '@/constants'
import { formatTime } from '@/utils'
import { loadWarehouseOptions, toOptionMap } from '@/utils/options'
import { GUIDE_EVENTS, useGuideStore, type GuideBusinessResult, type GuideFact } from '@/stores/guide'

const route = useRoute()
const router = useRouter()
const orderId = String(route.params.id)
const guide = useGuideStore()

const loading = ref(false)
const savingIds = reactive<Record<EntityID, boolean>>({})
const data = ref<StocktakeDetailData | null>(null)
const warehouseMap = ref<Record<EntityID, string>>({})

/** 草稿状态下每行可编辑的实盘数 */
const actualInputs = reactive<Record<EntityID, number>>({})

const isDraft = () => data.value?.order?.status === 'DRAFT'
const isGuideOrder = computed(
  () => guide.active && guide.scenario === 'stocktake' && guide.orderId === orderId,
)
const countedDetails = computed(() =>
  (data.value?.details ?? []).filter((detail) => detail.actual_qty !== null),
)
const allActualEntered = computed(
  () => Boolean(data.value?.details?.length) && countedDetails.value.length === data.value?.details?.length,
)
const guideSummary = computed(() => {
  if (!allActualEntered.value) return null
  return summarizeDetails(countedDetails.value, false)
})
const adjustmentCompleted = computed(
  () =>
    data.value?.order?.status === 'COMPLETED' &&
    Boolean(data.value.details?.length) &&
    data.value.details.every((detail) => detail.adjusted),
)

function displayDiff(row: StocktakeDetailItem): number | null {
  if (row.actual_qty === null) return null
  return isDraft() ? row.actual_qty - row.book_qty : row.diff_qty
}

function summarizeDetails(details: StocktakeDetailItem[], persistedDiff: boolean): {
  book: number
  actual: number
  diff: number
} {
  return details.reduce(
    (sum, detail) => {
      sum.book += detail.book_qty
      sum.actual += detail.actual_qty ?? 0
      sum.diff += persistedDiff ? detail.diff_qty : (detail.actual_qty ?? 0) - detail.book_qty
      return sum
    },
    { book: 0, actual: 0, diff: 0 },
  )
}

function signed(value: number): string {
  return `${value > 0 ? '+' : ''}${value}`
}

function stocktakeFacts(persistedDiff: boolean): GuideFact[] {
  const details = isDraft() ? countedDetails.value : (data.value?.details ?? [])
  if (!details.length) return []
  const summary = summarizeDetails(details, persistedDiff)
  return [
    { label: '账面库存', value: String(summary.book) },
    { label: '实盘库存', value: String(summary.actual) },
    { label: '差异', value: signed(summary.diff) },
    { label: '调整数量', value: signed(summary.diff) },
    { label: '盘点明细', value: `${details.length} 行` },
  ]
}

function recordGuideEvent(event: string, result: GuideBusinessResult = {}): void {
  if (!isGuideOrder.value || guide.currentStepDefinition?.event !== event) return
  guide.recordBusinessResult(event, result)
}

function syncGuideState(): void {
  const order = data.value?.order
  const step = guide.currentStepDefinition
  if (!isGuideOrder.value || !order || !step) return

  if (order.status === 'CANCELLED') {
    guide.setMismatch(`盘点单 ${order.order_no} 已取消，当前步骤无法继续。请重新开始本次引导。`)
    return
  }

  const snapshotReady = Boolean(data.value?.details?.length)
  const base = {
    orderId: String(order.id),
    orderNo: order.order_no,
  }

  if (step.id === 'stocktake-snapshot') {
    if (!snapshotReady) {
      guide.setMismatch('当前盘点单没有账面快照，无法继续。')
      return
    }
    recordGuideEvent(GUIDE_EVENTS.stocktakeSnapshotReady, {
      ...base,
      facts: stocktakeFacts(false),
      message: `${order.order_no} 已生成账面快照，共 ${data.value?.details?.length ?? 0} 行。`,
    })
    return
  }

  if (step.id === 'stocktake-actual') {
    if (adjustmentCompleted.value) {
      recordGuideEvent(GUIDE_EVENTS.stocktakeActualCompleted, {
        ...base,
        facts: stocktakeFacts(true),
        message: '全部明细已完成实盘录入。',
      })
      return
    }
    if (allActualEntered.value) {
      recordGuideEvent(GUIDE_EVENTS.stocktakeActualCompleted, {
        ...base,
        facts: stocktakeFacts(false),
        message: `全部 ${data.value?.details?.length ?? 0} 行实盘数量已保存，可以查看差异。`,
      })
      return
    }
    guide.setMismatch('')
    return
  }

  if (step.id === 'stocktake-difference') {
    if (!allActualEntered.value) {
      guide.setMismatch('请先保存全部明细的实盘数量，再查看账实差异。')
      return
    }
    recordGuideEvent(GUIDE_EVENTS.stocktakeDifferenceReviewed, {
      ...base,
      facts: stocktakeFacts(adjustmentCompleted.value),
      message: '已核对账面、实盘与差异汇总。',
    })
    return
  }

  if (step.id === 'stocktake-approve') {
    if (adjustmentCompleted.value) {
      recordGuideEvent(GUIDE_EVENTS.stocktakeApproved, {
        ...base,
        facts: stocktakeFacts(true),
        message: `审核完成：${order.order_no} 已按实际盘点结果调整库存。`,
      })
      return
    }
    if (!allActualEntered.value) {
      guide.setMismatch('存在尚未录入实盘数量的明细，不能审核。')
      return
    }
    guide.setMismatch('')
    return
  }

  if (step.id === 'stocktake-inventory' && adjustmentCompleted.value) {
    guide.setMismatch('盘点已审核完成，请前往库存流水核对 ADJUST 调整记录。')
  }
}

async function load() {
  loading.value = true
  try {
    data.value = await getStocktakeOrder(orderId)
    for (const detail of data.value.details ?? []) {
      if (!(detail.id in actualInputs)) {
        actualInputs[detail.id] = detail.actual_qty ?? detail.book_qty
      }
    }
    syncGuideState()
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  warehouseMap.value = toOptionMap(await loadWarehouseOptions())
  await load()
})

watch(
  () => [guide.active, guide.currentStep, data.value] as const,
  () => syncGuideState(),
  { deep: true },
)

// ---------- 录入实盘 ----------
async function saveActual(row: StocktakeDetailItem) {
  const value = actualInputs[row.id]
  if (value === undefined || value < 0) {
    ElMessage.warning('请输入不小于 0 的实盘数量')
    return
  }
  savingIds[row.id] = true
  try {
    await submitStocktakeActual(orderId, { detail_id: row.id, actual_qty: value })
    ElMessage.success('实盘数已保存')
    await load()
  } finally {
    savingIds[row.id] = false
  }
}

// ---------- 审核 / 取消 ----------
async function onApprove() {
  try {
    await ElMessageBox.confirm('确定审核该盘点单吗？审核后将按差异自动调整库存。', '审核盘点单', {
      type: 'warning',
      confirmButtonText: '确认审核',
      cancelButtonText: '返回',
    })
  } catch {
    return
  }
  await approveStocktakeOrder(orderId)
  ElMessage.success('审核完成，库存调整流水已生成')
  await load()
}

async function onCancel() {
  try {
    await ElMessageBox.confirm('确定取消该盘点单吗？', '取消盘点单', {
      type: 'warning',
      confirmButtonText: '确认取消',
      cancelButtonText: '返回',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  await cancelStocktakeOrder(orderId)
  ElMessage.success('已取消')
  await load()
}
</script>

<template>
  <div v-loading="loading">
    <el-page-header class="detail-header" @back="router.back()">
      <template #content>
        <span class="header-title">盘点单详情</span>
      </template>
    </el-page-header>

    <template v-if="data?.order">
      <div class="page-card">
        <div class="detail-actions">
          <el-tag :type="statusTag(data.order.status)" size="large">{{ statusText(data.order.status) }}</el-tag>
          <template v-if="data.order.status === 'DRAFT'">
            <el-button
              v-permission="'wms:stocktake:approve'"
              data-tour="stocktake-approve"
              type="success"
              plain
              @click="onApprove"
            >
              审核
            </el-button>
            <el-button v-permission="'wms:stocktake:cancel'" type="danger" plain @click="onCancel">取消</el-button>
          </template>
        </div>

        <el-descriptions :column="3" border>
          <el-descriptions-item label="盘点单号">{{ data.order.order_no }}</el-descriptions-item>
          <el-descriptions-item label="仓库">{{ warehouseMap[data.order.warehouse_id] || data.order.warehouse_id }}</el-descriptions-item>
          <el-descriptions-item label="盘点范围">
            {{ data.order.location_id && data.order.location_id !== '0' ? (data.order.location_code || `库位#${data.order.location_id}`) : '整仓盘点' }}
          </el-descriptions-item>
          <el-descriptions-item label="创建人">{{ data.order.created_by || '-' }}</el-descriptions-item>
          <el-descriptions-item label="创建时间">{{ formatTime(data.order.created_at) }}</el-descriptions-item>
          <el-descriptions-item label="备注">{{ data.order.remark || '-' }}</el-descriptions-item>
        </el-descriptions>
      </div>

      <div v-if="guideSummary" class="page-card section difference-summary" data-tour="stocktake-difference">
        <div>
          <span>账面库存</span>
          <b>{{ guideSummary.book }}</b>
        </div>
        <div>
          <span>实盘库存</span>
          <b>{{ guideSummary.actual }}</b>
        </div>
        <div>
          <span>差异</span>
          <b :class="{ positive: guideSummary.diff > 0, negative: guideSummary.diff < 0 }">
            {{ signed(guideSummary.diff) }}
          </b>
        </div>
      </div>

      <div class="page-card section" data-tour="stocktake-snapshot">
        <h3 class="section-title">
          盘点明细
          <el-tag v-if="isDraft()" type="warning" size="small" class="draft-tip">草稿状态可直接录入实盘数</el-tag>
        </h3>
        <el-table :data="data.details ?? []" border stripe>
          <el-table-column label="库位" min-width="120">
            <template #default="{ row }">{{ row.location_code || '-' }}</template>
          </el-table-column>
          <el-table-column prop="sku_code" label="货品编码" min-width="130" />
          <el-table-column prop="sku_name" label="货品名称" min-width="150" show-overflow-tooltip />
          <el-table-column label="批次" min-width="100">
            <template #default="{ row }">{{ row.batch_no || '-' }}</template>
          </el-table-column>
          <el-table-column prop="book_qty" label="账面数量" width="100" align="right" />
          <el-table-column label="实盘数量" width="200">
            <template #default="{ row }">
              <template v-if="isDraft()">
                <div class="actual-editor">
                  <el-input-number
                    v-model="actualInputs[row.id]"
                    :min="0"
                    controls-position="right"
                    style="width: 120px"
                  />
                  <el-button
                    v-permission="'wms:stocktake:stocktake'"
                    data-tour="stocktake-actual"
                    type="primary"
                    size="small"
                    :loading="savingIds[row.id]"
                    @click="saveActual(row)"
                  >
                    保存
                  </el-button>
                </div>
              </template>
              <template v-else>
                <span :class="{ 'not-counted': row.actual_qty === null }">
                  {{ row.actual_qty ?? '未盘' }}
                </span>
              </template>
            </template>
          </el-table-column>
          <el-table-column label="差异数" width="100" align="right">
            <template #default="{ row }">
              <span v-if="displayDiff(row) === null">-</span>
              <span
                v-else
                :style="{ color: displayDiff(row) === 0 ? 'var(--el-text-color-secondary)' : displayDiff(row)! > 0 ? 'var(--el-color-success)' : 'var(--el-color-danger)' }"
              >
                {{ displayDiff(row)! > 0 ? '+' : '' }}{{ displayDiff(row) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="已调整" width="90">
            <template #default="{ row }">
              <el-tag v-if="row.adjusted" type="success" size="small">是</el-tag>
              <span v-else>否</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </template>

    <div v-else-if="!loading" class="empty-tip">盘点单不存在或已删除</div>
  </div>
</template>

<style scoped>
.detail-header {
  margin-bottom: 14px;
}

.header-title {
  font-size: 16px;
  font-weight: 600;
}

.detail-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.section {
  margin-top: 14px;
}

.section-title {
  margin: 0 0 12px;
  font-size: 15px;
  display: flex;
  align-items: center;
  gap: 10px;
}

.difference-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.difference-summary div {
  padding: 14px 16px;
  border-radius: 9px;
  background: var(--el-fill-color-light);
}

.difference-summary span,
.difference-summary b {
  display: block;
}

.difference-summary span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.difference-summary b {
  margin-top: 5px;
  color: var(--el-text-color-primary);
  font-family: var(--gowms-num-font);
  font-size: 22px;
}

.difference-summary b.positive {
  color: var(--el-color-success);
}

.difference-summary b.negative {
  color: var(--el-color-danger);
}

.actual-editor {
  display: flex;
  align-items: center;
  gap: 8px;
}

.not-counted {
  color: var(--el-text-color-disabled);
}

.empty-tip {
  text-align: center;
  color: var(--el-text-color-secondary);
  padding: 60px 0;
}

@media (max-width: 640px) {
  .difference-summary {
    grid-template-columns: 1fr;
  }
}
</style>
