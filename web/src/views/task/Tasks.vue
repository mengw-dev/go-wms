<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { listTasks } from '@/api/task'
import type { TaskItem } from '@/api/types'
import {
  statusTag,
  statusText,
  taskTypeText,
  TASK_STATUS_OPTIONS,
  TASK_TYPE_OPTIONS,
} from '@/constants'
import { cleanParams, formatTime } from '@/utils'

const loading = ref(false)
const list = ref<TaskItem[]>([])
const total = ref(0)
const query = reactive({
  page: 1,
  page_size: 10,
  task_type: '',
  status: '',
  order_id: '' as number | '',
})

async function load() {
  loading.value = true
  try {
    const data = await listTasks(cleanParams({ ...query }))
    list.value = data.list ?? []
    total.value = data.total ?? 0
  } finally {
    loading.value = false
  }
}

function search() {
  query.page = 1
  load()
}

onMounted(load)
</script>

<template>
  <div class="page-card">
    <el-form inline class="query-form" @submit.prevent="search">
      <el-form-item label="任务类型">
        <el-select v-model="query.task_type" placeholder="全部" clearable style="width: 120px" @change="search">
          <el-option v-for="t in TASK_TYPE_OPTIONS" :key="t" :label="taskTypeText(t)" :value="t" />
        </el-select>
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="query.status" placeholder="全部" clearable style="width: 130px" @change="search">
          <el-option v-for="s in TASK_STATUS_OPTIONS" :key="s" :label="statusText(s)" :value="s" />
        </el-select>
      </el-form-item>
      <el-form-item label="单据ID">
        <el-input v-model="query.order_id" placeholder="来源单据 ID" clearable style="width: 130px" @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
      </el-form-item>
    </el-form>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column prop="task_no" label="任务号" min-width="170" />
      <el-table-column label="类型" width="90">
        <template #default="{ row }">{{ taskTypeText(row.task_type) }}</template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="order_no" label="单据号" min-width="160">
        <template #default="{ row }">{{ row.order_no || '-' }}</template>
      </el-table-column>
      <el-table-column prop="target_qty" label="目标数量" width="100" align="right" />
      <el-table-column prop="done_qty" label="完成数量" width="100" align="right" />
      <el-table-column label="操作员" width="110">
        <template #default="{ row }">{{ row.operator || '-' }}</template>
      </el-table-column>
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="query.page"
      v-model:page-size="query.page_size"
      class="pagination"
      layout="total, sizes, prev, pager, next, jumper"
      :total="total"
      :page-sizes="[10, 20, 50]"
      @current-change="load"
      @size-change="search"
    />
  </div>
</template>
