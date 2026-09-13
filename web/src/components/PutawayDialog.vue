<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getInboundOrder, putawayInboundTask } from '@/api/inbound'
import type { TaskItem } from '@/api/types'
import { loadIdleLocationOptions, type IdOption } from '@/utils/options'

const emit = defineEmits<{ (e: 'success'): void }>()

const visible = ref(false)
const loading = ref(false)
const orderId = ref(0)
const tasks = ref<TaskItem[]>([])
const locationOptions = ref<IdOption[]>([])

const form = reactive({
  task_id: undefined as number | undefined,
  location_id: undefined as number | undefined,
  qty: 1,
})
const submitting = ref(false)

const selectedTask = computed(() => tasks.value.find((t) => t.id === form.task_id))

const pendingTasks = computed(() =>
  tasks.value.filter((t) => t.task_type === 'PUTAWAY' && (t.status === 'CREATED' || t.status === 'IN_PROGRESS')),
)

async function open(id: number, task?: TaskItem) {
  orderId.value = id
  visible.value = true
  loading.value = true
  form.task_id = undefined
  form.location_id = undefined
  form.qty = 1
  try {
    const detail = await getInboundOrder(id)
    tasks.value = detail.tasks ?? []
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

function onTaskChange(taskId: number) {
  form.task_id = taskId
  const task = tasks.value.find((t) => t.id === taskId)
  form.qty = task ? Math.max(task.target_qty - task.done_qty, 1) : 1
  form.location_id = undefined
}

async function submit() {
  if (!form.task_id) {
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
  submitting.value = true
  try {
    await putawayInboundTask(form.task_id, {
      task_id: form.task_id,
      location_id: form.location_id,
      qty: form.qty,
    })
    ElMessage.success('上架成功')
    visible.value = false
    emit('success')
  } finally {
    submitting.value = false
  }
}

defineExpose({ open })
</script>

<template>
  <el-dialog v-model="visible" title="上架" width="520px" destroy-on-close>
    <el-form label-width="90px" v-loading="loading">
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
        <el-select v-model="form.location_id" placeholder="选择空闲库位" filterable style="width: 100%">
          <el-option v-for="loc in locationOptions" :key="loc.id" :label="loc.label" :value="loc.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="上架数量">
        <el-input-number v-model="form.qty" :min="1" controls-position="right" />
        <span v-if="selectedTask" class="qty-tip">待上架：{{ selectedTask.target_qty - selectedTask.done_qty }}</span>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">确定上架</el-button>
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
