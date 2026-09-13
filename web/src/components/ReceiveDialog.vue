<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getInboundOrder, receiveInbound } from '@/api/inbound'
import type { InboundOrderDetailRow } from '@/api/types'

const emit = defineEmits<{ (e: 'success'): void }>()

const visible = ref(false)
const loading = ref(false)
const orderId = ref(0)
const details = ref<InboundOrderDetailRow[]>([])

interface RowForm {
  qty: number
  defective_qty: number
  batch_no: string
  submitting: boolean
}
const forms = reactive<Record<number, RowForm>>({})

function remaining(row: InboundOrderDetailRow): number {
  return Math.max(row.expected_qty - row.received_qty, 0)
}

async function open(id: number) {
  orderId.value = id
  visible.value = true
  loading.value = true
  try {
    const detail = await getInboundOrder(id)
    details.value = detail.details ?? []
    for (const row of details.value) {
      if (!forms[row.id]) {
        forms[row.id] = {
          qty: remaining(row) || 1,
          defective_qty: 0,
          batch_no: '',
          submitting: false,
        }
      }
    }
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
  form.submitting = true
  try {
    await receiveInbound(orderId.value, {
      detail_id: row.id,
      qty: form.qty,
      defective_qty: form.defective_qty,
      batch_no: form.batch_no,
    })
    ElMessage.success('收货成功')
    emit('success')
    await open(orderId.value)
  } finally {
    form.submitting = false
  }
}

defineExpose({ open })
</script>

<template>
  <el-dialog v-model="visible" title="收货" width="820px" destroy-on-close>
    <el-table v-loading="loading" :data="details" border max-height="420">
      <el-table-column prop="sku_code" label="货品编码" min-width="110" />
      <el-table-column prop="sku_name" label="货品名称" min-width="130" show-overflow-tooltip />
      <el-table-column prop="expected_qty" label="应收" width="70" align="right" />
      <el-table-column prop="received_qty" label="已收" width="70" align="right" />
      <el-table-column label="本次收货" width="130">
        <template #default="{ row }">
          <el-input-number v-model="forms[row.id]!.qty" :min="1" controls-position="right" style="width: 100px" />
        </template>
      </el-table-column>
      <el-table-column label="不良品" width="130">
        <template #default="{ row }">
          <el-input-number v-model="forms[row.id]!.defective_qty" :min="0" controls-position="right" style="width: 100px" />
        </template>
      </el-table-column>
      <el-table-column label="批次号" width="140">
        <template #default="{ row }">
          <el-input v-model="forms[row.id]!.batch_no" placeholder="可选" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="90" fixed="right">
        <template #default="{ row }">
          <el-button
            type="primary"
            size="small"
            :disabled="remaining(row) <= 0"
            :loading="forms[row.id]?.submitting"
            @click="submit(row)"
          >
            收货
          </el-button>
        </template>
      </el-table-column>
    </el-table>
    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>
