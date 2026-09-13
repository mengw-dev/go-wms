<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveOutboundOrder,
  cancelOutboundOrder,
  getOutboundOrder,
  submitOutboundOrder,
} from '@/api/outbound'
import type { OutboundOrderDetail as OutboundDetailData } from '@/api/types'
import { statusTag, statusText, taskTypeText } from '@/constants'
import { formatTime } from '@/utils'
import { loadWarehouseOptions, toOptionMap } from '@/utils/options'
import PickDialog from '@/components/PickDialog.vue'

const route = useRoute()
const router = useRouter()
const orderId = Number(route.params.id)

const loading = ref(false)
const data = ref<OutboundDetailData | null>(null)
const warehouseMap = ref<Record<number, string>>({})

async function load() {
  loading.value = true
  try {
    data.value = await getOutboundOrder(orderId)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  warehouseMap.value = toOptionMap(await loadWarehouseOptions())
  load()
})

// ---------- 单据操作 ----------
async function onSubmit() {
  try {
    await ElMessageBox.confirm('确定提交该出库单吗？', '提示', { type: 'warning' })
  } catch {
    return
  }
  await submitOutboundOrder(orderId)
  ElMessage.success('提交成功')
  load()
}

async function onApprove() {
  try {
    await ElMessageBox.confirm('确定审核（分配库存）该出库单吗？', '提示', { type: 'warning' })
  } catch {
    return
  }
  await approveOutboundOrder(orderId)
  ElMessage.success('审核完成，库存已分配')
  load()
}

async function onCancel() {
  try {
    await ElMessageBox.confirm('确定取消该出库单吗？取消后将释放已分配库存。', '提示', { type: 'warning' })
  } catch {
    return
  }
  await cancelOutboundOrder(orderId)
  ElMessage.success('已取消')
  load()
}

// ---------- 拣货 ----------
const pickRef = ref<InstanceType<typeof PickDialog>>()

function openPick(taskId?: number) {
  const task = data.value?.tasks?.find((t) => t.id === taskId)
  pickRef.value?.open(orderId, task)
}

function canPick(task: { task_type: string; status: string }): boolean {
  return task.task_type === 'PICK' && (task.status === 'CREATED' || task.status === 'IN_PROGRESS')
}
</script>

<template>
  <div v-loading="loading">
    <el-page-header class="detail-header" @back="router.back()">
      <template #content>
        <span class="header-title">出库单详情</span>
      </template>
    </el-page-header>

    <template v-if="data?.order">
      <div class="page-card">
        <div class="detail-actions">
          <el-tag :type="statusTag(data.order.status)" size="large">{{ statusText(data.order.status) }}</el-tag>
          <template v-if="data.order.status === 'DRAFT'">
            <el-button type="success" plain @click="onSubmit">提交</el-button>
          </template>
          <template v-else-if="data.order.status === 'SUBMITTED'">
            <el-button type="success" plain @click="onApprove">审核（分配）</el-button>
            <el-button type="danger" plain @click="onCancel">取消</el-button>
          </template>
          <template v-else-if="data.order.status === 'APPROVED' || data.order.status === 'PICKING'">
            <el-button type="primary" plain @click="openPick()">拣货</el-button>
            <el-button type="danger" plain @click="onCancel">取消</el-button>
          </template>
        </div>

        <el-descriptions :column="3" border>
          <el-descriptions-item label="出库单号">{{ data.order.order_no }}</el-descriptions-item>
          <el-descriptions-item label="业务订单号">{{ data.order.biz_order_no }}</el-descriptions-item>
          <el-descriptions-item label="仓库">{{ warehouseMap[data.order.warehouse_id] || data.order.warehouse_id }}</el-descriptions-item>
          <el-descriptions-item label="需求数量">{{ data.order.expected_qty }}</el-descriptions-item>
          <el-descriptions-item label="已分配数量">{{ data.order.allocated_qty }}</el-descriptions-item>
          <el-descriptions-item label="已拣货数量">{{ data.order.picked_qty }}</el-descriptions-item>
          <el-descriptions-item label="创建人">{{ data.order.created_by || '-' }}</el-descriptions-item>
          <el-descriptions-item label="创建时间">{{ formatTime(data.order.created_at) }}</el-descriptions-item>
          <el-descriptions-item label="备注">{{ data.order.remark || '-' }}</el-descriptions-item>
        </el-descriptions>
      </div>

      <div class="page-card section">
        <h3 class="section-title">单据明细</h3>
        <el-table :data="data.details ?? []" border stripe>
          <el-table-column prop="id" label="明细ID" width="80" />
          <el-table-column prop="sku_code" label="货品编码" min-width="130" />
          <el-table-column prop="sku_name" label="货品名称" min-width="160" show-overflow-tooltip />
          <el-table-column prop="expected_qty" label="需求数量" width="100" align="right" />
          <el-table-column prop="allocated_qty" label="已分配" width="90" align="right" />
          <el-table-column prop="picked_qty" label="已拣货" width="90" align="right" />
        </el-table>
      </div>

      <div class="page-card section">
        <h3 class="section-title">分配明细</h3>
        <el-table :data="data.allocations ?? []" border stripe>
          <el-table-column prop="id" label="分配ID" width="80" />
          <el-table-column prop="detail_id" label="明细ID" width="80" />
          <el-table-column prop="location_code" label="库位" min-width="120">
            <template #default="{ row }">{{ row.location_code || row.location_id }}</template>
          </el-table-column>
          <el-table-column label="批次" min-width="110">
            <template #default="{ row }">{{ row.batch_no || '-' }}</template>
          </el-table-column>
          <el-table-column prop="allocated_qty" label="分配数量" width="100" align="right" />
          <el-table-column prop="picked_qty" label="已拣数量" width="100" align="right" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div class="page-card section">
        <h3 class="section-title">关联任务</h3>
        <el-table :data="data.tasks ?? []" border stripe>
          <el-table-column prop="task_no" label="任务号" min-width="160" />
          <el-table-column label="类型" width="90">
            <template #default="{ row }">{{ taskTypeText(row.task_type) }}</template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="target_qty" label="目标数量" width="100" align="right" />
          <el-table-column prop="done_qty" label="完成数量" width="100" align="right" />
          <el-table-column label="操作员" width="110">
            <template #default="{ row }">{{ row.operator || '-' }}</template>
          </el-table-column>
          <el-table-column label="创建时间" width="170">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="100" fixed="right">
            <template #default="{ row }">
              <el-button v-if="canPick(row)" type="primary" size="small" plain @click="openPick(row.id)">
                拣货
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </template>

    <PickDialog ref="pickRef" @success="load" />
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
}
</style>
