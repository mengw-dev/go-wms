<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getOutboundOrder, pickOutboundTask } from '@/api/outbound'
import type { TaskItem } from '@/api/types'

const emit = defineEmits<{ (e: 'success'): void }>()

const visible = ref(false)
const loading = ref(false)
const orderId = ref(0)
const tasks = ref<TaskItem[]>([])

const form = reactive({
  task_id: undefined as number | undefined,
  qty: 1,
})
const submitting = ref(false)

const selectedTask = computed(() => tasks.value.find((t) => t.id === form.task_id))

const pendingTasks = computed(() =>
  tasks.value.filter((t) => t.task_type === 'PICK' && (t.status === 'CREATED' || t.status === 'IN_PROGRESS')),
)

async function open(id: number, task?: TaskItem) {
  orderId.value = id
  visible.value = true
  loading.value = true
  form.task_id = undefined
  form.qty = 1
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

function onTaskChange(taskId: number) {
  form.task_id = taskId
  const task = tasks.value.find((t) => t.id === taskId)
  form.qty = task ? Math.max(task.target_qty - task.done_qty, 1) : 1
}

async function submit() {
  if (!form.task_id) {
    ElMessage.warning('请选择拣货任务')
    return
  }
  if (form.qty <= 0) {
    ElMessage.warning('拣货数量必须大于 0')
    return
  }
  submitting.value = true
  try {
    await pickOutboundTask(form.task_id, { task_id: form.task_id, qty: form.qty })
    ElMessage.success('拣货成功')
    visible.value = false
    emit('success')
  } finally {
    submitting.value = false
  }
}

defineExpose({ open })
</script>

<template>
  <el-dialog v-model="visible" title="拣货" width="520px" destroy-on-close>
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
      <el-form-item label="拣货数量">
        <el-input-number v-model="form.qty" :min="1" controls-position="right" />
        <span v-if="selectedTask" class="qty-tip">待拣：{{ selectedTask.target_qty - selectedTask.done_qty }}</span>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">确定拣货</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.qty-tip {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
