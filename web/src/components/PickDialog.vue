<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { getOutboundOrder } from '@/api/outbound'
import type { EntityID, TaskItem } from '@/api/types'
import { usePdaPick } from '@/composables/outbound/usePdaPick'

const emit = defineEmits<{ (e: 'success'): void }>()

const visible = ref(false)
const loading = ref(false)
const orderId = ref<EntityID>('')
const tasks = ref<TaskItem[]>([])

const form = reactive({
  task_id: undefined as EntityID | undefined,
  qty: 1,
  location_scan: '',
  batch_scan: '',
})

const { claimToken, leaseExpireAt, claiming, submitting, snapshot, lastError, reset, claim, pick } = usePdaPick()

const selectedTask = computed(() => tasks.value.find((t) => t.id === form.task_id))

const pendingTasks = computed(() =>
  tasks.value.filter((t) => t.task_type === 'PICK' && (t.status === 'CREATED' || t.status === 'IN_PROGRESS')),
)

// 任务已指定批次时必须扫码核对，避免同一库位混放多个批次时拣错。
const requireBatchScan = computed(() => !!selectedTask.value?.batch_no)

const claimed = computed(() => claimToken.value !== '')

watch(
  () => form.task_id,
  () => {
    form.batch_scan = ''
    form.location_scan = selectedTask.value?.location_code ?? ''
  },
)

async function open(id: EntityID, task?: TaskItem) {
  orderId.value = id
  visible.value = true
  loading.value = true
  form.task_id = undefined
  form.qty = 1
  form.batch_scan = ''
  reset()
  try {
    const detail = await getOutboundOrder(id)
    tasks.value = detail.tasks ?? []
    const preselect = task ?? pendingTasks.value[0]
    if (preselect) {
      onTaskChange(preselect.id)
    } else {
      ElMessage.warning('暂无待执行的拣货任务')
    }
  } finally {
    loading.value = false
  }
}

function onTaskChange(taskId: EntityID) {
  form.task_id = taskId
  const task = tasks.value.find((t) => t.id === taskId)
  form.qty = task ? Math.max(task.target_qty - task.done_qty, 1) : 1
  form.location_scan = task?.location_code ?? ''
}

async function handleClaim() {
  if (!form.task_id) {
    ElMessage.warning('请先选择拣货任务')
    return
  }
  if (await claim(form.task_id)) {
    ElMessage.success('任务领取成功，请在 10 分钟内完成作业')
  }
}

async function submit() {
  if (!form.task_id) {
    ElMessage.warning('请选择拣货任务')
    return
  }
  if (!claimed.value) {
    ElMessage.warning('请先领取任务')
    return
  }
  if (form.qty <= 0) {
    ElMessage.warning('拣货数量必须大于 0')
    return
  }
  const expectedBatch = selectedTask.value?.batch_no ?? ''
  const scannedBatch = form.batch_scan.trim()
  const scannedLocation = form.location_scan.trim()
  if (!scannedLocation) {
    ElMessage.warning('请扫描或输入作业库位')
    return
  }
  if (requireBatchScan.value && !scannedBatch) {
    ElMessage.warning('请扫描或输入批次号核对')
    return
  }
  if (scannedBatch && scannedBatch.toLowerCase() !== expectedBatch.toLowerCase()) {
    ElMessage.error(`批次不符：该任务应拣批次 ${expectedBatch}`)
    return
  }
  try {
    const result = await pick(form.task_id, {
      qty: form.qty,
      location_code: scannedLocation,
      batch_no: scannedBatch || undefined,
    })
    // 用返回快照就地刷新任务进度，拣完自动关闭，否则继续拣下一件。
    applySnapshot(result)
    ElMessage.success('拣货成功')
    if (result.remaining_qty <= 0 || result.task_status === 'COMPLETED') {
      visible.value = false
      emit('success')
    }
  } catch {
    // 业务拒绝：错误与快照已在界面展示，保持对话框打开以便重试（复用同一 Idempotency-Key）。
  }
}

/** 用后端返回的快照更新本地任务数据（不重新拉详情）。 */
function applySnapshot(result: { task_status: string; done_qty: number }) {
  const task = tasks.value.find((t) => t.id === form.task_id)
  if (task) {
    task.done_qty = result.done_qty
    task.status = result.task_status
  }
}

defineExpose({ open })
</script>

<template>
  <el-dialog
    v-model="visible"
    title="PDA 拣货"
    width="560px"
    destroy-on-close
    :close-on-click-modal="!loading && !submitting"
    :close-on-press-escape="!submitting"
    :show-close="!submitting"
  >
    <el-form label-width="90px" v-loading="loading">
      <el-form-item label="拣货任务">
        <el-select v-model="form.task_id" placeholder="选择待执行的拣货任务" style="width: 100%" @change="onTaskChange">
          <el-option
            v-for="t in pendingTasks"
            :key="t.id"
            :value="t.id"
            :label="`${t.task_no}（目标 ${t.target_qty} 已完成 ${t.done_qty}）`"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="任务领取">
        <template v-if="claimed">
          <el-tag type="success">已领取</el-tag>
          <span class="lease-tip">租约至 {{ new Date(leaseExpireAt).toLocaleTimeString() }}，成功拣货自动续租</span>
        </template>
        <template v-else>
          <el-button :loading="claiming" :disabled="!form.task_id" @click="handleClaim">领取任务</el-button>
          <div class="hint">领取后获得作业凭证，10 分钟内完成，过期需重新领取</div>
        </template>
      </el-form-item>
      <el-form-item label="作业库位">
        <el-input
          v-model="form.location_scan"
          placeholder="扫描或输入作业库位"
          :disabled="!claimed"
          clearable
        />
      </el-form-item>
      <el-form-item label="应拣批次">
        <el-input :model-value="selectedTask?.batch_no || '-'" readonly />
      </el-form-item>
      <el-form-item label="批次核对">
        <el-input
          v-model="form.batch_scan"
          placeholder="扫描或输入批次号核对"
          :disabled="!requireBatchScan || !claimed"
          clearable
        />
        <div class="hint">核对一致后才会提交，防止同库位不同批次拣错</div>
      </el-form-item>
      <el-form-item label="拣货数量">
        <el-input-number v-model="form.qty" :min="1" :disabled="!claimed" controls-position="right" />
        <span v-if="selectedTask" class="qty-tip">待拣：{{ selectedTask.target_qty - selectedTask.done_qty }}</span>
      </el-form-item>
      <el-form-item v-if="snapshot" label="任务进度">
        <div class="snapshot">
          <div>任务状态：{{ snapshot.task_status }}</div>
          <div>已拣数量：{{ snapshot.done_qty }}</div>
          <div>剩余数量：{{ snapshot.remaining_qty }}</div>
          <div>出库单状态：{{ snapshot.order_status }}</div>
        </div>
      </el-form-item>
      <el-form-item v-if="lastError" label="提示">
        <el-alert :title="lastError" type="error" :closable="false" show-icon />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button :disabled="submitting" @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" :disabled="!claimed" @click="submit">确定拣货</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.qty-tip {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.hint {
  margin-top: 4px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.4;
}

.lease-tip {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.snapshot {
  font-size: 12px;
  line-height: 1.8;
  color: var(--el-text-color-regular);
}
</style>
