<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getInboundOrder } from '@/api/inbound'
import type { EntityID, InboundOrderDetailRow } from '@/api/types'
import { useReceive } from '@/composables/inbound/useReceive'

const emit = defineEmits<{ (e: 'success'): void }>()

const visible = ref(false)
const loading = ref(false)
const orderId = ref<EntityID>('')
const details = ref<InboundOrderDetailRow[]>([])

const receiver = useReceive()

interface RowForm {
  qty: number
  defective_qty: number
  batch_no: string
}
const forms = reactive<Record<EntityID, RowForm>>({})

const storageError = computed(() => receiver.storageError)
const hasSubmitting = computed(() =>
  details.value.some((row) => receiver.stateOf(row.id).submitting),
)

function remaining(row: InboundOrderDetailRow): number {
  return Math.max(row.expected_qty - row.received_qty, 0)
}

function stateOf(row: InboundOrderDetailRow) {
  return receiver.stateOf(row.id)
}

async function open(id: EntityID) {
  orderId.value = id
  visible.value = true
  loading.value = true
  try {
    const detail = await getInboundOrder(id)
    details.value = detail.details ?? []
    Object.keys(forms).forEach((key) => delete forms[key])
    for (const row of details.value) {
      forms[row.id] = {
        qty: remaining(row) || 1,
        defective_qty: 0,
        batch_no: row.batch_no || '',
      }
    }
    // 恢复这批明细上次未确认的收货操作（关闭弹窗、刷新后不丢）。
    const restoreError = receiver.restoreForDetails(details.value.map((row) => row.id))
    if (restoreError) ElMessage.error(restoreError)
  } finally {
    loading.value = false
  }
}

async function submit(row: InboundOrderDetailRow) {
  const form = forms[row.id]
  if (!form || form.qty <= 0) {
    ElMessage.warning('请输入大于 0 的收货数量')
    return
  }
  if (form.defective_qty < 0 || form.defective_qty > form.qty) {
    ElMessage.warning('不良品数量不能大于收货数量')
    return
  }
  if (!row.batch_no && !form.batch_no.trim()) {
    ElMessage.warning('首次收货必须填写批次号')
    return
  }
  try {
    await receiver.receive(orderId.value, row.id, {
      qty: form.qty,
      defective_qty: form.defective_qty,
      batch_no: form.batch_no,
    })
    ElMessage.success('收货成功')
    emit('success')
    await open(orderId.value)
  } catch {
    // 错误与待确认横幅已写入对应明细状态，由界面展示恢复/放弃入口。
  }
}

/** 恢复未确认的收货：原 key 原参数重试，服务端回放或重新执行同一操作。 */
async function recover(row: InboundOrderDetailRow) {
  try {
    await receiver.recoverReceive(orderId.value, row.id)
    ElMessage.success('收货操作已恢复')
    emit('success')
    await open(orderId.value)
  } catch {
    // 错误与待确认横幅已写入对应明细状态。
  }
}

/** 核对后放弃未确认的收货：仅删除本地记录，不向服务端发送任何请求。 */
async function discard(row: InboundOrderDetailRow) {
  try {
    await ElMessageBox.confirm(
      '请确认已核对该明细的已收数量：放弃后本次收货不会自动重试。',
      '核对并放弃待确认收货',
      { type: 'warning', confirmButtonText: '已核对，放弃', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  receiver.discardReceive(row.id)
  ElMessage.success('已放弃待确认的收货操作')
}

defineExpose({ open })
</script>

<template>
  <el-dialog
    v-model="visible"
    title="收货"
    width="960px"
    destroy-on-close
    :close-on-click-modal="!loading && !hasSubmitting"
    :close-on-press-escape="!loading && !hasSubmitting"
    :show-close="!loading && !hasSubmitting"
  >
    <el-alert v-if="storageError" :title="storageError" type="error" show-icon :closable="false" class="storage-tip" />
    <el-table v-loading="loading" :data="details" border max-height="420">
      <el-table-column prop="sku_code" label="货品编码" min-width="110" />
      <el-table-column prop="sku_name" label="货品名称" min-width="130" show-overflow-tooltip />
      <el-table-column prop="expected_qty" label="应收" width="70" align="right" />
      <el-table-column prop="received_qty" label="已收" width="70" align="right" />
      <el-table-column label="本次收货" width="130">
        <template #default="{ row }">
          <el-input-number
            v-model="forms[row.id]!.qty"
            :min="1"
            :disabled="!!stateOf(row).pending"
            controls-position="right"
            style="width: 100px"
          />
        </template>
      </el-table-column>
      <el-table-column label="不良品" width="130">
        <template #default="{ row }">
          <el-input-number
            v-model="forms[row.id]!.defective_qty"
            :min="0"
            :disabled="!!stateOf(row).pending"
            controls-position="right"
            style="width: 100px"
          />
        </template>
      </el-table-column>
      <el-table-column label="批次号" width="140">
        <template #default="{ row }">
          <el-input v-model="forms[row.id]!.batch_no" placeholder="首次收货必填" :disabled="!!stateOf(row).pending" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="170" fixed="right">
        <template #default="{ row }">
          <template v-if="stateOf(row).pending">
            <span v-if="stateOf(row).pending!.stale" class="pending-tip">已超保留期，请核对后放弃</span>
            <el-button
              size="small"
              type="primary"
              :disabled="stateOf(row).pending!.stale"
              :loading="stateOf(row).submitting"
              @click="recover(row)"
            >
              恢复收货
            </el-button>
            <el-button size="small" @click="discard(row)">放弃</el-button>
          </template>
          <el-button
            v-else
            type="primary"
            size="small"
            :disabled="remaining(row) <= 0"
            :loading="stateOf(row).submitting"
            @click="submit(row)"
          >
            收货
          </el-button>
        </template>
      </el-table-column>
      <el-table-column label="待确认" width="130">
        <template #default="{ row }">
          <span v-if="stateOf(row).pending" class="pending-tip">
            {{ stateOf(row).pending!.stale ? '超期待核对' : `待确认 ${stateOf(row).pending!.businessPayload.qty} 件` }}
          </span>
        </template>
      </el-table-column>
    </el-table>
    <template #footer>
      <el-button :disabled="loading || hasSubmitting" @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.pending-tip {
  font-size: 12px;
  color: var(--el-color-warning);
}

.storage-tip {
  margin-bottom: 10px;
}
</style>