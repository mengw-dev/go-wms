<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveInboundOrder,
  cancelInboundOrder,
  getInboundOrder,
  submitInboundOrder,
} from '@/api/inbound'
import type { EntityID, InboundOrderDetail as InboundDetailData } from '@/api/types'
import { GUIDE_EVENTS, useGuideStore, type GuideBusinessResult } from '@/stores/guide'
import { statusTag, statusText, taskTypeText } from '@/constants'
import { formatTime } from '@/utils'
import { loadWarehouseOptions, toOptionMap } from '@/utils/options'
import ReceiveDialog from '@/components/ReceiveDialog.vue'
import PutawayDialog from '@/components/PutawayDialog.vue'

const route = useRoute()
const router = useRouter()
const orderId = String(route.params.id)
const guide = useGuideStore()

const loading = ref(false)
const data = ref<InboundDetailData | null>(null)
const warehouseMap = ref<Record<EntityID, string>>({})
const isGuideOrder = computed(
  () => guide.active && guide.scenario === 'inbound' && guide.orderId === orderId,
)

function pendingPutawayTask() {
  return data.value?.tasks?.find(
    (task) => task.task_type === 'PUTAWAY' && (task.status === 'CREATED' || task.status === 'IN_PROGRESS'),
  )
}

function recordGuideEvent(event: string, result: GuideBusinessResult): void {
  if (!isGuideOrder.value || guide.currentStepDefinition?.event !== event) return
  guide.recordBusinessResult(event, result)
}

function syncGuideState(): void {
  const order = data.value?.order
  const step = guide.currentStepDefinition
  if (!isGuideOrder.value || !order || !step) return

  const task = pendingPutawayTask()
  const status = order.status
  const laterThanSubmitted = ['SUBMITTED', 'APPROVED', 'RECEIVING', 'PUTAWAY', 'COMPLETED'].includes(status)
  const laterThanApproved = ['APPROVED', 'RECEIVING', 'PUTAWAY', 'COMPLETED'].includes(status)
  const received = ['PUTAWAY', 'COMPLETED'].includes(status)

  if (step.id === 'inbound-submit') {
    if (status === 'DRAFT') {
      guide.setMismatch('')
      return
    }
    if (laterThanSubmitted) {
      recordGuideEvent(GUIDE_EVENTS.inboundOrderSubmitted, {
        orderId: String(order.id),
        orderNo: order.order_no,
        message: `提交完成：${order.order_no} 已从草稿变为已提交。`,
      })
      return
    }
  }

  if (step.id === 'inbound-approve') {
    if (status === 'SUBMITTED') {
      guide.setMismatch('')
      return
    }
    if (laterThanApproved) {
      recordGuideEvent(GUIDE_EVENTS.inboundOrderApproved, {
        orderId: String(order.id),
        orderNo: order.order_no,
        message: `审核完成：${order.order_no} 已从已提交变为已审核。下一步：收货。`,
      })
      return
    }
  }

  if (step.id === 'inbound-receive') {
    if (status === 'APPROVED' || status === 'RECEIVING') {
      guide.setMismatch('')
      return
    }
    if (received) {
      recordGuideEvent(GUIDE_EVENTS.inboundReceived, {
        orderId: String(order.id),
        orderNo: order.order_no,
        taskId: task ? String(task.id) : guide.taskId,
        taskNo: task?.task_no || guide.taskNo,
        message: task
          ? `收货完成：${order.order_no} 已生成上架任务 ${task.task_no}。下一步：查看并完成上架。`
          : `收货完成：${order.order_no} 已收齐。下一步：查看上架任务。`,
      })
      return
    }
  }

  if (step.id === 'inbound-tasks') {
    if (task) {
      recordGuideEvent(GUIDE_EVENTS.inboundPutawayReady, {
        taskId: String(task.id),
        taskNo: task.task_no,
        message: `上架任务已生成：${task.task_no}，待上架 ${Math.max(task.target_qty - task.done_qty, 0)}。`,
      })
      return
    }
    if (status === 'COMPLETED') {
      guide.setMismatch('当前入库单已完成上架，没有可查看的待上架任务。请重新开始本次引导。')
      return
    }
  }

  if (step.id === 'inbound-putaway') {
    if (task) {
      guide.setMismatch('')
      return
    }
    if (status === 'COMPLETED') {
      recordGuideEvent(GUIDE_EVENTS.inboundPutawayCompleted, {
        orderId: String(order.id),
        orderNo: order.order_no,
        message: `上架完成：${order.order_no} 的库存已在真实业务事务中增加。`,
      })
      return
    }
  }

  if (status === 'CANCELED') {
    guide.setMismatch(`入库单 ${order.order_no} 已取消，当前步骤无法继续。请重新开始本次引导。`)
    return
  }

  if (step.id !== 'inbound-create') {
    guide.setMismatch(`当前业务状态 ${status} 与引导步骤“${step.title}”不一致，请重新定位当前步骤。`)
  }
}

async function load() {
  loading.value = true
  try {
    data.value = await getInboundOrder(orderId)
    syncGuideState()
  } finally {
    loading.value = false
  }
}

watch(
  () => guide.currentStepDefinition?.id,
  async () => {
    await nextTick()
    syncGuideState()
  },
)

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
  await load()
}

async function onApprove() {
  try {
    await ElMessageBox.confirm('确定审核通过该入库单吗？', '提示', { type: 'warning' })
  } catch {
    return
  }
  await approveInboundOrder(orderId)
  ElMessage.success('审核通过')
  await load()
}

async function onCancel() {
  try {
    await ElMessageBox.confirm('确定取消该入库单吗？取消后不能继续收货或上架。', '取消入库单', {
      type: 'warning',
      confirmButtonText: '确认取消',
      cancelButtonText: '返回',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  await cancelInboundOrder(orderId)
  ElMessage.success('已取消')
  await load()
}

// ---------- 收货 / 上架 ----------
const receiveRef = ref<InstanceType<typeof ReceiveDialog>>()
const putawayRef = ref<InstanceType<typeof PutawayDialog>>()

function openReceive() {
  receiveRef.value?.open(orderId)
}

function openPutaway(taskId?: EntityID) {
  const task = data.value?.tasks?.find((t) => t.id === taskId)
  putawayRef.value?.open(orderId, task)
}

function canPutaway(task: { task_type: string; status: string }): boolean {
  return task.task_type === 'PUTAWAY' && (task.status === 'CREATED' || task.status === 'IN_PROGRESS')
}

async function onReceiveSuccess() {
  await load()
}

async function onPutawaySuccess() {
  await load()
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
            <el-button v-permission="'wms:inbound:submit'" data-tour="inbound-submit" type="success" plain @click="onSubmit">提交</el-button>
            <el-button v-permission="'wms:inbound:cancel'" type="danger" plain @click="onCancel">取消</el-button>
          </template>
          <template v-else-if="data.order.status === 'SUBMITTED'">
            <el-button v-permission="'wms:inbound:approve'" data-tour="inbound-approve" type="success" plain @click="onApprove">审核</el-button>
            <el-button v-permission="'wms:inbound:cancel'" type="danger" plain @click="onCancel">取消</el-button>
          </template>
          <template v-else-if="data.order.status === 'APPROVED' || data.order.status === 'RECEIVING'">
            <el-button v-permission="'wms:inbound:receive'" data-tour="inbound-receive" type="primary" plain @click="openReceive">收货</el-button>
            <el-button v-permission="'wms:inbound:putaway'" data-tour="inbound-putaway" type="warning" plain @click="openPutaway()">上架</el-button>
          </template>
          <template v-else-if="data.order.status === 'PUTAWAY'">
            <el-button v-permission="'wms:inbound:putaway'" data-tour="inbound-putaway" type="warning" plain @click="openPutaway()">上架</el-button>
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

      <div class="page-card section" data-tour="inbound-tasks">
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
              <el-button v-if="canPutaway(row)" v-permission="'wms:inbound:putaway'" type="warning" size="small" plain @click="openPutaway(row.id)">
                上架
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </template>

    <ReceiveDialog ref="receiveRef" @success="onReceiveSuccess" />
    <PutawayDialog ref="putawayRef" @success="onPutawaySuccess" />
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
