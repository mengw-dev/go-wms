<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { createSku, deleteSku, listSkus, updateSku } from '@/api/basic'
import type { SkuItem } from '@/api/types'
import { cleanParams, formatTime } from '@/utils'

// ---------- 列表 ----------
const loading = ref(false)
const list = ref<SkuItem[]>([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 10, keyword: '' })

async function load() {
  loading.value = true
  try {
    const data = await listSkus(cleanParams({ ...query }))
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

// ---------- 新增 / 编辑 ----------
const dialog = reactive({ visible: false, loading: false, editingId: 0 })
const formRef = ref<FormInstance>()
const form = reactive({ code: '', barcode: '', name: '', spec: '', unit: '' })

const rules: FormRules = {
  code: [{ required: true, message: '请输入货品编码', trigger: 'blur' }],
  barcode: [{ required: true, message: '请输入条码', trigger: 'blur' }],
  name: [{ required: true, message: '请输入货品名称', trigger: 'blur' }],
}

function openCreate() {
  dialog.editingId = 0
  form.code = ''
  form.barcode = ''
  form.name = ''
  form.spec = ''
  form.unit = ''
  dialog.visible = true
}

function openEdit(row: SkuItem) {
  dialog.editingId = row.id
  form.code = row.code
  form.barcode = row.barcode
  form.name = row.name
  form.spec = row.spec
  form.unit = row.unit
  dialog.visible = true
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  dialog.loading = true
  try {
    if (dialog.editingId) {
      await updateSku(dialog.editingId, { ...form })
      ElMessage.success('修改成功')
    } else {
      await createSku({ ...form })
      ElMessage.success('创建成功')
    }
    dialog.visible = false
    load()
  } finally {
    dialog.loading = false
  }
}

async function onDelete(row: SkuItem) {
  try {
    await ElMessageBox.confirm(`确定删除货品「${row.name}」吗？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await deleteSku(row.id)
  ElMessage.success('删除成功')
  load()
}
</script>

<template>
  <div class="page-card">
    <el-form inline class="query-form" @submit.prevent="search">
      <el-form-item label="关键字">
        <el-input v-model="query.keyword" placeholder="编码/条码/名称" clearable @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
      </el-form-item>
    </el-form>

    <div class="toolbar">
      <span />
      <el-button type="primary" @click="openCreate">新增货品</el-button>
    </div>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="code" label="编码" min-width="120" />
      <el-table-column prop="barcode" label="条码" min-width="140" />
      <el-table-column prop="name" label="名称" min-width="160" />
      <el-table-column prop="spec" label="规格" min-width="120" />
      <el-table-column prop="unit" label="单位" width="80" />
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
      :title="dialog.editingId ? '编辑货品' : '新增货品'"
      width="480px"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
        <el-form-item label="编码" prop="code">
          <el-input v-model="form.code" placeholder="货品编码" />
        </el-form-item>
        <el-form-item label="条码" prop="barcode">
          <el-input v-model="form.barcode" placeholder="条码" />
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="货品名称" />
        </el-form-item>
        <el-form-item label="规格" prop="spec">
          <el-input v-model="form.spec" placeholder="规格（可选）" />
        </el-form-item>
        <el-form-item label="单位" prop="unit">
          <el-input v-model="form.unit" placeholder="单位，如 件/箱（可选）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="dialog.loading" @click="submit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>
