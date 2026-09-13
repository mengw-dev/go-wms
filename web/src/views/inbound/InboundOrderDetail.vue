<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveInboundOrder,
  cancelInboundOrder,
  getInboundOrder,
  submitInboundOrder,
} from '@/api/inbound'
import type { InboundOrderDetail as InboundDetailData } from '@/api/types'
import { statusTag, statusText, taskTypeText } from '@/constants'
import { formatTime } from '@/utils'
import { loadWarehouseOptions, toOptionMap } from '@/utils/options'
import ReceiveDialog from '@/components/ReceiveDialog.vue'
import PutawayDialog from '@/components/PutawayDialog.vue'

const route = useRoute()
const router = useRouter()
const orderId = Number(route.params.id)

const loading = ref(false)
const data = ref<InboundDetailData | null>(null)
const warehouseMap = ref<Record<number, string>>({})

async function load() {
  loading.value = true
  try {
    data.value = await getInboundOrder(orderId)
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
    await ElMessageBox.confirm('确定提交该入库单吗？', '提示', { type: 'warning' })
  } catch {
    return
  }
  await submitInboundOrder(orderId)
  ElMessage.success('提交成功')
  load()
}

async function onApprove() {
  try {
    await ElMessageBox.confirm('确定审核通过该入库单吗？', '提示', { type: 'warning' })
  } catch {
    return
  }
  await approveInboundOrder(orderId)
  ElMessage.success('审核通过')
  load()
}

async function onCancel() {
  try {
    await ElMessageBox.confirm('确定取消该入库单吗？', '提示', { type: 'warning' })
  } catch {
    return
  }
  await cancelInboundOrder(orderId)
  ElMessage.success('已取消')
  load()
}

// ---------- 收货 / 上架 ----------
const receiveRef = ref<InstanceType<typeof ReceiveDialog>>()
const putawayRef = ref<InstanceType<typeof PutawayDialog>>()

function openReceive() {
  receiveRef.value?.open(orderId)
}

function openPutaway(taskId?: number) {
  const task = data.value?.tasks?.find((t) => t.id === taskId)
  putawayRef.value?.open(orderId, task)
}

function canPutaway(task: { task_type: string; status: string }): boolean {
  return task.task_type === 'PUTAWAY' && (task.status === 'CREATED' || task.status === 'IN_PROGRESS')
}
</script>

<template>
  <div v-loading="loading">
    <el-page-header class="detail-header" @back="router.back()">
      <template #content>
        <span class="header-title">入库单详情</span>
      </template>
    </el-page-header>

    <template v-if="data?.order">
      <div class="page-card">
        <div class="detail-actions">
          <el-tag :type="statusTag(data.order.status)" size="large">{{ statusText(data.order.status) }}</el-tag>
          <template v-if="data.order.status === 'DRAFT'">
            <el-button type="success" plain @click="onSubmit">提交</el-button>
            <el-button type="danger" plain @click="onCancel">取消</el-button>
          </template>
          <template v-else-if="data.order.status === 'SUBMITTED'">
            <el-button type="success" plain @click="onApprove">审核</el-button>
            <el-button type="danger" plain @click="onCancel">取消</el-button>
          </template>
          <template v-else-if="data.order.status === 'APPROVED' || data.order.status === 'RECEIVING'">
            <el-button type="primary" plain @click="openReceive">收货</el-button>
            <el-button type="warning" plain @click="openPutaway()">上架</el-button>
          </template>
          <template v-else-if="data.order.status === 'PUTAWAY'">
            <el-button type="warning" plain @click="openPutaway()">上架</el-button>
          </template>
        </div>

        <el-descriptions :column="3" border>
          <el-descriptions-item label="入库单号">{{ data.order.order_no }}</el-descriptions-item>
          <el-descriptions-item label="仓库">{{ warehouseMap[data.order.warehouse_id] || data.order.warehouse_id }}</el-descriptions-item>
          <el-descriptions-item label="来源">{{ data.order.source === 'IMPORT' ? '导入' : '手动' }}</el-descriptions-item>
          <el-descriptions-item label="应收数量">{{ data.order.expected_qty }}</el-descriptions-item>
          <el-descriptions-item label="已收数量">{{ data.order.received_qty }}</el-descriptions-item>
          <el-descriptions-item label="不良品数量">{{ data.order.defective_qty }}</el-descriptions-item>
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
          <el-table-column prop="expected_qty" label="应收数量" width="100" align="right" />
          <el-table-column prop="received_qty" label="已收数量" width="100" align="right" />
          <el-table-column prop="defective_qty" label="不良品" width="90" align="right" />
          <el-table-column label="批次号" min-width="120">
            <template #default="{ row }">{{ row.batch_no || '-' }}</template>
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
              <el-button v-if="canPutaway(row)" type="warning" size="small" plain @click="openPutaway(row.id)">
                上架
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </template>

    <ReceiveDialog ref="receiveRef" @success="load" />
    <PutawayDialog ref="putawayRef" @success="load" />
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
