<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import {
  createWarehouse,
  deleteWarehouse,
  listWarehouses,
  updateWarehouse,
  updateWarehouseStatus,
} from '@/api/basic'
import type { WarehouseItem } from '@/api/types'
import { COMMON_STATUS } from '@/constants'
import { cleanParams, formatTime } from '@/utils'

// ---------- 列表 ----------
const loading = ref(false)
const list = ref<WarehouseItem[]>([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 10, keyword: '', status: '' as number | '' })

async function load() {
  loading.value = true
  try {
    const data = await listWarehouses(cleanParams({ ...query }))
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

// ---------- 状态开关 ----------
async function onToggleStatus(row: WarehouseItem, value: string | number | boolean) {
  const status = value ? COMMON_STATUS.ENABLED : COMMON_STATUS.DISABLED
  try {
    await updateWarehouseStatus(row.id, status)
    ElMessage.success(status === COMMON_STATUS.ENABLED ? '已启用' : '已停用')
  } finally {
    load()
  }
}

// ---------- 新增 / 编辑 ----------
const dialog = reactive({ visible: false, loading: false, editingId: 0 })
const formRef = ref<FormInstance>()
const form = reactive({ code: '', name: '', remark: '' })

const rules: FormRules = {
  code: [{ required: true, message: '请输入仓库编码', trigger: 'blur' }],
  name: [{ required: true, message: '请输入仓库名称', trigger: 'blur' }],
}

function openCreate() {
  dialog.editingId = 0
  form.code = ''
  form.name = ''
  form.remark = ''
  dialog.visible = true
}

function openEdit(row: WarehouseItem) {
  dialog.editingId = row.id
  form.code = row.code
  form.name = row.name
  form.remark = row.remark
  dialog.visible = true
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  dialog.loading = true
  try {
    if (dialog.editingId) {
      await updateWarehouse(dialog.editingId, { ...form })
      ElMessage.success('修改成功')
    } else {
      await createWarehouse({ ...form })
      ElMessage.success('创建成功')
    }
    dialog.visible = false
    load()
  } finally {
    dialog.loading = false
  }
}

async function onDelete(row: WarehouseItem) {
  try {
    await ElMessageBox.confirm(`确定删除仓库「${row.name}」吗？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await deleteWarehouse(row.id)
  ElMessage.success('删除成功')
  load()
}
</script>

<template>
  <div class="page-card">
    <el-form inline class="query-form" @submit.prevent="search">
      <el-form-item label="关键字">
        <el-input v-model="query.keyword" placeholder="编码/名称" clearable @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="query.status" placeholder="全部" clearable style="width: 120px" @change="search">
          <el-option label="启用" :value="COMMON_STATUS.ENABLED" />
          <el-option label="停用" :value="COMMON_STATUS.DISABLED" />
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
      </el-form-item>
    </el-form>

    <div class="toolbar">
      <span />
      <el-button type="primary" @click="openCreate">新增仓库</el-button>
    </div>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="code" label="编码" min-width="110" />
      <el-table-column prop="name" label="名称" min-width="140" />
      <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip />
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-switch
            :model-value="row.status === COMMON_STATUS.ENABLED"
            inline-prompt
            active-text="启"
            inactive-text="停"
            @change="onToggleStatus(row, $event)"
          />
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <div class="table-oper">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" plain @click="onDelete(row)">删除</el-button>
          </div>
        </template>
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

    <el-dialog
      v-model="dialog.visible"
      :title="dialog.editingId ? '编辑仓库' : '新增仓库'"
      width="480px"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
        <el-form-item label="编码" prop="code">
          <el-input v-model="form.code" placeholder="仓库编码，如 WH01" :disabled="!!dialog.editingId" />
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="仓库名称" />
        </el-form-item>
        <el-form-item label="备注" prop="remark">
          <el-input v-model="form.remark" type="textarea" :rows="2" placeholder="备注" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="dialog.loading" @click="submit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>
