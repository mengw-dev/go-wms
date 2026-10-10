<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
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

const picker = usePdaPick()

const selectedTask = computed(() => tasks.value.find((t) => t.id === form.task_id))

const pendingTasks = computed(() =>
  tasks.value.filter((t) => t.task_type === 'PICK' && (t.status === 'CREATED' || t.status === 'IN_PROGRESS')),
)

// 任务已指定批次时必须扫码核对，避免同一库位混放多个批次时拣错。
const requireBatchScan = computed(() => !!selectedTask.value?.batch_no)

// 领取状态、进度快照、错误与待确认横幅全部按当前任务读取：切换任务即切换展示。
const current = computed(() => (form.task_id ? picker.stateOf(form.task_id) : null))
const claimToken = computed(() => current.value?.claimToken ?? '')
const leaseExpireAt = computed(() => current.value?.leaseExpireAt ?? '')
const claiming = computed(() => current.value?.claiming ?? false)
const submitting = computed(() => current.value?.submitting ?? false)
const snapshot = computed(() => current.value?.snapshot ?? null)
const lastError = computed(() => current.value?.lastError ?? '')
const pendingClaim = computed(() => current.value?.pendingClaim ?? null)
const pendingPick = computed(() => current.value?.pendingPick ?? null)
const storageError = computed(() => picker.storageError)
const pendingTip = computed(() => {
  if (pendingPick.value) {
    return pendingPick.value.stale
      ? '任务存在未确认的拣货操作（已超过保留期，请核对任务进度）'
      : '任务存在未确认的拣货操作'
  }
  if (pendingClaim.value) {
    return pendingClaim.value.stale
      ? '任务存在未确认的领取操作（已超过保留期，请核对任务状态）'
      : '任务存在未确认的领取操作'
  }
  return ''
})

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
  try {
    const detail = await getOutboundOrder(id)
    tasks.value = detail.tasks ?? []
    // 恢复这批任务上一次未确认的领取/拣货操作（关闭弹窗、刷新后不丢）。
    const restoreError = picker.restoreForTasks(tasks.value.map((t) => t.id))
    if (restoreError) ElMessage.error(restoreError)
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
  if (await picker.claim(form.task_id)) {
    ElMessage.success('任务领取成功，请在 10 分钟内完成作业')
  }
}

/** 恢复未确认的领取：原 key 原内容重试，服务端回放首次凭证。 */
async function handleRecoverClaim() {
  if (!form.task_id) return
  if (await picker.recoverClaim(form.task_id)) {
    ElMessage.success('领取操作已恢复，凭证以首次领取为准')
  }
}

/** 恢复未确认的拣货：原 key 原参数重试，服务端回放首次成功或重新执行同一操作。 */
async function handleRecoverPick() {
  const taskId = form.task_id
  if (!taskId) return
  try {
    const result = await picker.recoverPick(taskId)
    applySnapshot(result, taskId)
    ElMessage.success('拣货操作已恢复')
    if (form.task_id === taskId && (result.remaining_qty <= 0 || result.task_status === 'COMPLETED')) {
      visible.value = false
      emit('success')
    }
  } catch {
    // 错误与快照已写入当前任务状态，由界面横幅/提示展示。
  }
}

async function handleDiscardClaim() {
  if (!form.task_id) return
  try {
    await confirmDiscard('请确认已核对任务领取状态：放弃后本次领取不会自动重试。')
  } catch {
    return
  }
  picker.discardClaim(form.task_id)
  ElMessage.success('已放弃待确认的领取操作')
}

async function handleDiscardPick() {
  if (!form.task_id) return
  try {
    await confirmDiscard('请确认已核对任务进度：放弃后本次拣货不会自动重试。')
  } catch {
    return
  }
  picker.discardPick(form.task_id)
  ElMessage.success('已放弃待确认的拣货操作')
}

async function confirmDiscard(message: string) {
  try {
    await ElMessageBox.confirm(message, '核对并放弃待确认操作', {
      type: 'warning',
      confirmButtonText: '已核对，放弃',
      cancelButtonText: '取消',
    })
  } catch {
    throw new Error('已取消')
  }
}

async function submit() {
  const taskId = form.task_id
  if (!taskId) {
    ElMessage.warning('请选择拣货任务')
    return
  }
  if (!claimed.value) {
    if (pendingClaim.value) {
      ElMessage.warning('请先恢复或放弃上一次未确认的领取操作')
    } else {
      ElMessage.warning('请先领取任务')
    }
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
    const result = await picker.pick(taskId, {
      qty: form.qty,
      location_code: scannedLocation,
      batch_no: scannedBatch || undefined,
    })
    // 响应属于发送请求时的任务：切换任务后晚返回也只刷新对应任务的数据。
    applySnapshot(result, taskId)
    ElMessage.success('拣货成功')
    if (form.task_id === taskId && (result.remaining_qty <= 0 || result.task_status === 'COMPLETED')) {
      visible.value = false
      emit('success')
    }
  } catch {
    // 业务/网络错误与快照已写入对应任务状态；待确认横幅提示恢复或放弃动作。
  }
}

/** 用后端返回的快照更新发送请求时对应的本地任务数据（不重新拉详情）。 */
function applySnapshot(result: { task_status: string; done_qty: number }, taskId: EntityID) {
  const task = tasks.value.find((t) => t.id === taskId)
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
      <el-form-item v-if="storageError" label=" ">
        <el-alert :title="storageError" type="error" show-icon :closable="false" />
      </el-form-item>
      <el-form-item v-if="pendingPick || pendingClaim" label=" ">
        <el-alert :title="pendingTip" type="warning" show-icon :closable="false">
          <div class="pending-actions">
            <span v-if="pendingPick" class="pending-detail">
              数量 {{ pendingPick.businessPayload.qty }}，库位 {{ pendingPick.businessPayload.location_code || '未扫描' }}
            </span>
            <el-button v-if="pendingPick" size="small" type="primary" :disabled="pendingPick.stale" @click="handleRecoverPick">
              恢复拣货
            </el-button>
            <el-button v-if="pendingClaim" size="small" type="primary" :disabled="pendingClaim.stale" @click="handleRecoverClaim">
              恢复领取
            </el-button>
            <el-button size="small" @click="pendingPick ? handleDiscardPick() : handleDiscardClaim()">已核对，放弃</el-button>
          </div>
        </el-alert>
      </el-form-item>
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
      <el-form-item v-if="lastError && !pendingPick && !pendingClaim" label="提示">
        <el-alert :title="lastError" type="error" :closable="false" show-icon />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button :disabled="submitting" @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" :disabled="!claimed || !!pendingPick" @click="submit">确定拣货</el-button>
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

.pending-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
  flex-wrap: wrap;
}

.pending-detail {
  font-size: 12px;
  color: var(--el-text-color-regular);
}
</style>