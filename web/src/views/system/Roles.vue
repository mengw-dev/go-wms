<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { createRole, deleteRole, listRoles, updateRole } from '@/api/system'
import type { RoleItem } from '@/api/types'
import { cleanParams, formatTime } from '@/utils'

// ---------- 列表 ----------
const loading = ref(false)
const list = ref<RoleItem[]>([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 10, keyword: '' })

async function load() {
  loading.value = true
  try {
    const data = await listRoles(cleanParams({ ...query }))
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
const form = reactive({ name: '', perms: '', remark: '' })

const rules: FormRules = {
  name: [{ required: true, message: '请输入角色名称', trigger: 'blur' }],
}

function openCreate() {
  dialog.editingId = 0
  form.name = ''
  form.perms = ''
  form.remark = ''
  dialog.visible = true
}

function openEdit(row: RoleItem) {
  dialog.editingId = row.id
  form.name = row.name
  form.perms = row.perms
  form.remark = row.remark
  dialog.visible = true
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  dialog.loading = true
  try {
    if (dialog.editingId) {
      await updateRole(dialog.editingId, { ...form })
      ElMessage.success('修改成功')
    } else {
      await createRole({ ...form })
      ElMessage.success('创建成功')
    }
    dialog.visible = false
    load()
  } finally {
    dialog.loading = false
  }
}

async function onDelete(row: RoleItem) {
  try {
    await ElMessageBox.confirm(`确定删除角色「${row.name}」吗？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await deleteRole(row.id)
  ElMessage.success('删除成功')
  load()
}
</script>

<template>
  <div class="page-card">
    <el-form inline class="query-form" @submit.prevent="search">
      <el-form-item label="关键字">
        <el-input v-model="query.keyword" placeholder="角色名称" clearable @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
      </el-form-item>
    </el-form>

    <div class="toolbar">
      <span />
      <el-button type="primary" @click="openCreate">新增角色</el-button>
    </div>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="name" label="角色名称" min-width="140" />
      <el-table-column prop="perms" label="权限串" min-width="220" show-overflow-tooltip />
      <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip />
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
      :title="dialog.editingId ? '编辑角色' : '新增角色'"
      width="500px"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="角色名称" />
        </el-form-item>
        <el-form-item label="权限串" prop="perms">
          <el-input v-model="form.perms" placeholder="如 wms:inbound:approve" />
          <div class="form-tip">权限串，* 表示超管（全部权限），多个权限用英文逗号分隔</div>
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

<style scoped>
.form-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
  margin-top: 2px;
}
</style>
