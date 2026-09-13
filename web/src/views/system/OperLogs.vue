<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { listOperLogs } from '@/api/system'
import type { OperLogItem } from '@/api/types'
import { cleanParams, formatTime } from '@/utils'

const loading = ref(false)
const list = ref<OperLogItem[]>([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 10, username: '' })

async function load() {
  loading.value = true
  try {
    const data = await listOperLogs(cleanParams({ ...query }))
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
      <el-form-item label="用户名">
        <el-input v-model="query.username" placeholder="按操作人筛选" clearable @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
      </el-form-item>
    </el-form>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="username" label="操作人" width="120" />
      <el-table-column prop="method" label="方法" width="90" />
      <el-table-column prop="path" label="请求路径" min-width="220" show-overflow-tooltip />
      <el-table-column prop="ip" label="IP" width="140" />
      <el-table-column prop="cost_ms" label="耗时(ms)" width="100">
        <template #default="{ row }">{{ row.cost_ms ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.status === 200 ? 'success' : 'danger'" size="small">
            {{ row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作时间" width="170">
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
