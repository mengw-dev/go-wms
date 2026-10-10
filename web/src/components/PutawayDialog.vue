<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getInboundOrder } from '@/api/inbound'
import type { EntityID, TaskItem } from '@/api/types'
import { usePutaway } from '@/composables/inbound/usePutaway'
import { loadIdleLocationOptions, type IdOption } from '@/utils/options'

const emit = defineEmits<{ (e: 'success'): void }>()

const visible = ref(false)
const loading = ref(false)
const orderId = ref<EntityID>('')
const tasks = ref<TaskItem[]>([])
const locationOptions = ref<IdOption[]>([])

const puter = usePutaway()

const form = reactive({
  task_id: undefined as EntityID | undefined,
  location_id: undefined as EntityID | undefined,
  qty: 1,
})

const selectedTask = computed(() => tasks.value.find((t) => t.id === form.task_id))

const pendingTasks = computed(() =>
  tasks.value.filter((t) => t.task_type === 'PUTAWAY' && (t.status === 'CREATED' || t.status === 'IN_PROGRESS')),
)

// 提交中与待确认横幅按当前任务读取：切换任务即切换展示。
const current = computed(() => (form.task_id ? puter.stateOf(form.task_id) : null))
const submitting = computed(() => current.value?.submitting ?? false)
const pendingPutaway = computed(() => current.value?.pending ?? null)
const lastError = computed(() => current.value?.lastError ?? '')
const storageError = computed(() => puter.storageError)

async function open(id: EntityID, task?: TaskItem) {
  orderId.value = id
  visible.value = true
  loading.value = true
  form.task_id = undefined
  form.location_id = undefined
  form.qty = 1
  try {
    const detail = await getInboundOrder(id)
    tasks.value = detail.tasks ?? []
    // 恢复这批任务上次未确认的上架操作（关闭弹窗、刷新后不丢）。
    const restoreError = puter.restoreForTasks(tasks.value.map((t) => t.id))
    if (restoreError) ElMessage.error(restoreError)
    if (detail.order?.warehouse_id) {
      locationOptions.value = await loadIdleLocationOptions(detail.order.warehouse_id)
    }
    const preselect = task ?? pendingTasks.value[0]
    if (preselect) {
      onTaskChange(preselect.id)
    } else {
      ElMessage.warning('暂无待执行的上架任务')
    }
  } finally {
    loading.value = false
  }
}

function onTaskChange(taskId: EntityID) {
  form.task_id = taskId
  const task = tasks.value.find((t) => t.id === taskId)
  form.qty = task ? Math.max(task.target_qty - task.done_qty, pendingPutaway.value?.businessPayload.qty || 1) : 1
  if (task) form.location_id = undefined
  const pending = puter.stateOf(taskId).pending
  if (pending && !pending.stale) {
    form.qty = pending.businessPayload.qty
    form.location_id = pending.businessPayload.location_id
  }
}

async function submit() {
  const taskId = form.task_id
  if (!taskId) {
    ElMessage.warning('请选择上架任务')
    return
  }
  if (!form.location_id) {
    ElMessage.warning('请选择目标库位')
    return
  }
  if (form.qty <= 0) {
    ElMessage.warning('上架数量必须大于 0')
    return
  }
  try {
    await puter.putaway(taskId, {
      location_id: form.location_id,
      qty: form.qty,
    })
    ElMessage.success('上架成功')
    visible.value = false
    emit('success')
  } catch {
    // 错误与待确认横幅已写入当前任务状态，由界面展示恢复/放弃入口。
  }
}

/** 恢复未确认的上架：原 key 原参数重试，服务端回放或重新执行同一操作。 */
async function recover() {
  const taskId = form.task_id
  if (!taskId) return
  try {
    await puter.recoverPutaway(taskId)
    ElMessage.success('上架操作已恢复')
    visible.value = false
    emit('success')
  } catch {
    // 错误与待确认横幅已写入当前任务状态。
  }
}

/** 核对后放弃未确认的上架：仅删除本地记录，不向服务端发送任何请求。 */
async function discard() {
  const taskId = form.task_id
  if (!taskId) return
  try {
    await ElMessageBox.confirm(
      '请确认已核对库存与任务进度：放弃后本次上架不会自动重试。',
      '核对并放弃待确认上架',
      { type: 'warning', confirmButtonText: '已核对，放弃', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  puter.discardPutaway(taskId)
  ElMessage.success('已放弃待确认的上架操作')
}

defineExpose({ open })
</script>

<template>
  <el-dialog
    v-model="visible"
    title="上架"
    width="520px"
    destroy-on-close
    :close-on-click-modal="!loading && !submitting"
    :close-on-press-escape="!submitting"
    :show-close="!submitting"
  >
    <el-form label-width="90px" v-loading="loading">
      <el-form-item v-if="storageError" label=" ">
        <el-alert :title="storageError" type="error" show-icon :closable="false" />
      </el-form-item>
      <el-form-item v-if="pendingPutaway || lastError" label=" ">
        <el-alert
          :title="
            pendingPutaway
              ? pendingPutaway.stale
                ? '任务存在未确认的上架操作（已超过保留期，请核对库存与任务进度）'
                : '任务存在未确认的上架操作'
              : lastError
          "
          :type="pendingPutaway ? 'warning' : 'error'"
          show-icon
          :closable="false"
        >
          <div v-if="pendingPutaway" class="pending-actions">
            <span class="pending-detail">
              库位 {{ pendingPutaway.businessPayload.location_id }}，数量 {{ pendingPutaway.businessPayload.qty }}
            </span>
            <el-button size="small" type="primary" :disabled="pendingPutaway.stale" @click="recover">恢复上架</el-button>
            <el-button size="small" @click="discard">已核对，放弃</el-button>
          </div>
        </el-alert>
      </el-form-item>
      <el-form-item label="上架任务">
        <el-select v-model="form.task_id" placeholder="选择待执行的上架任务" style="width: 100%" @change="onTaskChange">
          <el-option
            v-for="t in pendingTasks"
            :key="t.id"
            :value="t.id"
            :label="`${t.task_no}（目标 ${t.target_qty} 已完成 ${t.done_qty}）`"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="目标库位">
        <el-select
          v-model="form.location_id"
          placeholder="选择空闲库位"
          filterable
          style="width: 100%"
          :disabled="!!pendingPutaway"
        >
          <el-option v-for="loc in locationOptions" :key="loc.id" :label="loc.label" :value="loc.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="上架数量">
        <el-input-number v-model="form.qty" :min="1" :disabled="!!pendingPutaway" controls-position="right" />
        <span v-if="selectedTask" class="qty-tip">待上架：{{ selectedTask.target_qty - selectedTask.done_qty }}</span>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button :disabled="submitting" @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" :disabled="!!pendingPutaway" @click="submit">确定上架</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.qty-tip {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
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