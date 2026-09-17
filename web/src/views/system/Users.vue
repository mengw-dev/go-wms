<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import {
  createUser,
  deleteUser,
  listAllRoles,
  listUsers,
  resetUserPassword,
  updateUser,
  updateUserStatus,
} from '@/api/system'
import type { EntityID, RoleItem, UserItem } from '@/api/types'
import { cleanParams, formatTime } from '@/utils'

// ---------- 列表 ----------
const loading = ref(false)
const list = ref<UserItem[]>([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 10, keyword: '' })

async function load() {
  loading.value = true
  try {
    const data = await listUsers(cleanParams({ ...query }))
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

onMounted(() => {
  load()
  loadRoleOptions()
})

// ---------- 角色选项 ----------
const roleOptions = ref<RoleItem[]>([])

async function loadRoleOptions() {
  roleOptions.value = (await listAllRoles()) ?? []
}

function roleName(id: EntityID): string {
  return roleOptions.value.find((role) => role.id === id)?.name || String(id)
}

function isBuiltinAdmin(row: UserItem): boolean {
  return row.id === '1'
}

// ---------- 状态开关 ----------
const statusUpdatingId = ref<EntityID>('')

async function onToggleStatus(row: UserItem, value: string | number | boolean) {
  if (isBuiltinAdmin(row)) return
  const status = value ? 1 : 0
  if (status === 0) {
    try {
      await ElMessageBox.confirm(
        `停用后「${row.username}」将立即退出登录，确定继续吗？`,
        '停用用户',
        {
          type: 'warning',
          confirmButtonText: '确认停用',
          cancelButtonText: '取消',
          confirmButtonClass: 'el-button--danger',
        },
      )
    } catch {
      return
    }
  }
  statusUpdatingId.value = row.id
  try {
    await updateUserStatus(row.id, status)
    ElMessage.success(status === 1 ? '已启用' : '已停用')
  } finally {
    statusUpdatingId.value = ''
    load()
  }
}

// ---------- 新增 / 编辑 ----------
const dialog = reactive({ visible: false, loading: false, editingId: '' as EntityID })
const formRef = ref<FormInstance>()
const form = reactive({
  username: '',
  password: '',
  nickname: '',
  role_ids: [] as EntityID[],
})
const editStatus = ref(1)

const createRules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [
    { required: true, message: '请输入初始密码', trigger: 'blur' },
    { min: 6, max: 32, message: '密码长度 6-32 位', trigger: 'blur' },
  ],
  nickname: [{ max: 64, message: '昵称最长 64 字符', trigger: 'blur' }],
}

function openCreate() {
  dialog.editingId = ''
  form.username = ''
  form.password = ''
  form.nickname = ''
  form.role_ids = []
  dialog.visible = true
}

function openEdit(row: UserItem) {
  dialog.editingId = row.id
  form.username = row.username
  form.password = ''
  form.nickname = row.nickname
  form.role_ids = [...(row.role_ids ?? [])]
  editStatus.value = row.status
  dialog.visible = true
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  dialog.loading = true
  try {
    if (dialog.editingId) {
      const data = {
        nickname: form.nickname,
        status: editStatus.value,
        role_ids: [...form.role_ids],
      }
      await updateUser(dialog.editingId, data)
      ElMessage.success('修改成功')
    } else {
      await createUser({
        username: form.username,
        password: form.password,
        nickname: form.nickname,
        role_ids: [...form.role_ids],
      })
      ElMessage.success('创建成功')
    }
    dialog.visible = false
    load()
  } finally {
    dialog.loading = false
  }
}

// ---------- 重置密码 ----------
const pwdDialog = reactive({ visible: false, loading: false, userId: '' as EntityID, username: '' })
const pwdFormRef = ref<FormInstance>()
const pwdForm = reactive({ password: '' })
const pwdRules: FormRules = {
  password: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, max: 32, message: '密码长度 6-32 位', trigger: 'blur' },
  ],
}

function openResetPwd(row: UserItem) {
  pwdDialog.userId = row.id
  pwdDialog.username = row.username
  pwdForm.password = ''
  pwdDialog.visible = true
}

async function submitResetPwd() {
  const valid = await pwdFormRef.value?.validate().catch(() => false)
  if (!valid) return
  pwdDialog.loading = true
  try {
    await resetUserPassword(pwdDialog.userId, pwdForm.password)
    ElMessage.success('密码重置成功')
    pwdDialog.visible = false
  } finally {
    pwdDialog.loading = false
  }
}

// ---------- 删除 ----------
async function onDelete(row: UserItem) {
  try {
    await ElMessageBox.confirm(
      `删除后账号「${row.username}」将无法登录，确定继续吗？`,
      '删除用户',
      {
        type: 'warning',
        confirmButtonText: '确认删除',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger',
      },
    )
  } catch {
    return
  }
  await deleteUser(row.id)
  ElMessage.success('删除成功')
  load()
}
</script>

<template>
  <div class="page-card">
    <el-form inline class="query-form" @submit.prevent="search">
      <el-form-item label="关键字">
        <el-input v-model="query.keyword" placeholder="用户名/昵称" clearable @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
      </el-form-item>
    </el-form>

    <div class="toolbar">
      <el-button v-permission="'wms:system:user'" type="primary" @click="openCreate">新增用户</el-button>
    </div>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column label="用户名" min-width="150">
        <template #default="{ row }">
          <span class="user-name">{{ row.username }}</span>
          <el-tag v-if="isBuiltinAdmin(row)" type="danger" size="small" effect="plain">内置管理员</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="nickname" label="昵称" min-width="120" />
      <el-table-column label="角色" min-width="180">
        <template #default="{ row }">
          <div v-if="row.role_ids?.length" class="role-tags">
            <el-tag v-for="roleId in row.role_ids" :key="roleId" type="info" size="small">
              {{ roleName(roleId) }}
            </el-tag>
          </div>
          <span v-else class="empty-text">未分配</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tooltip :disabled="!isBuiltinAdmin(row)" content="内置管理员不可禁用" placement="top">
            <span class="switch-wrap">
              <el-switch
                v-permission="'wms:system:user'"
                :model-value="row.status === 1"
                :disabled="isBuiltinAdmin(row)"
                :loading="statusUpdatingId === row.id"
                inline-prompt
                active-text="启"
                inactive-text="停"
                @change="onToggleStatus(row, $event)"
              />
            </span>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <el-tooltip v-if="isBuiltinAdmin(row)" content="内置管理员受保护，不可编辑、重置密码或删除" placement="top">
            <span class="table-oper table-oper--disabled">
              <el-button size="small" disabled>编辑</el-button>
              <el-button size="small" type="warning" plain disabled>重置密码</el-button>
              <el-button size="small" type="danger" plain disabled>删除</el-button>
            </span>
          </el-tooltip>
          <div v-else class="table-oper">
            <el-button v-permission="'wms:system:user'" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button v-permission="'wms:system:user'" size="small" type="warning" plain @click="openResetPwd(row)">
              重置密码
            </el-button>
            <el-button v-permission="'wms:system:user'" size="small" type="danger" plain @click="onDelete(row)">
              删除
            </el-button>
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
      :title="dialog.editingId ? '编辑用户' : '新增用户'"
      width="480px"
      destroy-on-close
      :close-on-click-modal="false"
      :close-on-press-escape="!dialog.loading"
      :show-close="!dialog.loading"
    >
      <el-form ref="formRef" :model="form" :rules="dialog.editingId ? {} : createRules" label-width="90px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" :disabled="!!dialog.editingId" placeholder="登录账号" />
        </el-form-item>
        <el-form-item v-if="!dialog.editingId" label="初始密码" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="6-32 位" />
        </el-form-item>
        <el-form-item label="昵称" prop="nickname">
          <el-input v-model="form.nickname" placeholder="显示名称" />
        </el-form-item>
        <el-form-item v-if="dialog.editingId" label="状态">
          <el-switch v-model="editStatus" :active-value="1" :inactive-value="0" active-text="启用" inactive-text="停用" />
        </el-form-item>
        <el-form-item :label="dialog.editingId ? '角色' : '初始角色'">
          <el-select
            v-model="form.role_ids"
            multiple
            placeholder="选择角色，可多选"
            style="width: 100%"
          >
            <el-option v-for="role in roleOptions" :key="role.id" :label="role.name" :value="role.id" />
          </el-select>
          <div class="form-tip">留空表示不授予角色；修改后立即覆盖该用户原有角色。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="dialog.loading" @click="dialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="dialog.loading" @click="submit">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="pwdDialog.visible"
      :title="`重置密码 - ${pwdDialog.username}`"
      width="420px"
      destroy-on-close
      :close-on-click-modal="false"
      :close-on-press-escape="!pwdDialog.loading"
      :show-close="!pwdDialog.loading"
    >
      <el-form ref="pwdFormRef" :model="pwdForm" :rules="pwdRules" label-width="90px">
        <el-form-item label="新密码" prop="password">
          <el-input v-model="pwdForm.password" type="password" show-password placeholder="6-32 位" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="pwdDialog.loading" @click="pwdDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="pwdDialog.loading" @click="submitResetPwd">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.user-name {
  margin-right: 8px;
  font-weight: 600;
}

.role-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.empty-text {
  color: var(--el-text-color-secondary);
}

.switch-wrap {
  display: inline-flex;
}

.table-oper--disabled {
  cursor: not-allowed;
}

.form-tip {
  margin-top: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
</style>
