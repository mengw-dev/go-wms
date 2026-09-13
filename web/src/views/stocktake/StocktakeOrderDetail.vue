<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveStocktakeOrder,
  cancelStocktakeOrder,
  getStocktakeOrder,
  submitStocktakeActual,
} from '@/api/stocktake'
import type { StocktakeDetailItem, StocktakeOrderDetail as StocktakeDetailData } from '@/api/types'
import { statusTag, statusText } from '@/constants'
import { formatTime } from '@/utils'
import { loadWarehouseOptions, toOptionMap } from '@/utils/options'

const route = useRoute()
const router = useRouter()
const orderId = Number(route.params.id)

const loading = ref(false)
const savingIds = reactive<Record<number, boolean>>({})
const data = ref<StocktakeDetailData | null>(null)
const warehouseMap = ref<Record<number, string>>({})

/** 草稿状态下每行可编辑的实盘数 */
const actualInputs = reactive<Record<number, number>>({})

const isDraft = () => data.value?.order?.status === 'DRAFT'

async function load() {
  loading.value = true
  try {
    data.value = await getStocktakeOrder(orderId)
    for (const d of data.value.details ?? []) {
      if (!(d.id in actualInputs)) {
        actualInputs[d.id] = d.actual_qty ?? d.book_qty
      }
    }
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  warehouseMap.value = toOptionMap(await loadWarehouseOptions())
  load()
})

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
    load()
  } finally {
    savingIds[row.id] = false
  }
}

// ---------- 审核 / 取消 ----------
async function onApprove() {
  try {
    await ElMessageBox.confirm('确定审核该盘点单吗？审核后将按差异自动调整库存。', '提示', { type: 'warning' })
  } catch {
    return
  }
  await approveStocktakeOrder(orderId)
  ElMessage.success('审核完成')
  load()
}

async function onCancel() {
  try {
    await ElMessageBox.confirm('确定取消该盘点单吗？', '提示', { type: 'warning' })
  } catch {
    return
  }
  await cancelStocktakeOrder(orderId)
  ElMessage.success('已取消')
  load()
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
            <el-button type="success" plain @click="onApprove">审核</el-button>
            <el-button type="danger" plain @click="onCancel">取消</el-button>
          </template>
        </div>

        <el-descriptions :column="3" border>
          <el-descriptions-item label="盘点单号">{{ data.order.order_no }}</el-descriptions-item>
          <el-descriptions-item label="仓库">{{ warehouseMap[data.order.warehouse_id] || data.order.warehouse_id }}</el-descriptions-item>
          <el-descriptions-item label="盘点范围">
            {{ data.order.location_id > 0 ? (data.order.location_code || `库位#${data.order.location_id}`) : '整仓盘点' }}
          </el-descriptions-item>
          <el-descriptions-item label="创建人">{{ data.order.created_by || '-' }}</el-descriptions-item>
          <el-descriptions-item label="创建时间">{{ formatTime(data.order.created_at) }}</el-descriptions-item>
          <el-descriptions-item label="备注">{{ data.order.remark || '-' }}</el-descriptions-item>
        </el-descriptions>
      </div>

      <div class="page-card section">
        <h3 class="section-title">
          盘点明细
          <el-tag v-if="isDraft()" type="warning" size="small" class="draft-tip">草稿状态可直接录入实盘数</el-tag>
        </h3>
        <el-table :data="data.details ?? []" border stripe>
          <el-table-column prop="id" label="明细ID" width="80" />
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
              <span v-if="row.actual_qty === null">-</span>
              <span v-else :style="{ color: row.diff_qty === 0 ? 'var(--el-text-color-secondary)' : row.diff_qty > 0 ? 'var(--el-color-success)' : 'var(--el-color-danger)' }">
                {{ row.diff_qty > 0 ? '+' : '' }}{{ row.diff_qty }}
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
</style>
