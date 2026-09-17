<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { createRole, deleteRole, listRoles, updateRole } from '@/api/system'
import type { EntityID, RoleItem } from '@/api/types'
import { cleanParams, formatTime } from '@/utils'

interface PermissionOption {
  label: string
  value: string
}

interface PermissionGroup {
  label: string
  options: PermissionOption[]
}

const PERMISSION_GROUPS: PermissionGroup[] = [
  {
    label: '基础数据',
    options: [{ label: '仓库 / 库位 / 货品管理', value: 'wms:basic' }],
  },
  {
    label: '库存与任务',
    options: [
      { label: '库存查询', value: 'wms:inventory' },
      { label: '任务中心', value: 'wms:task' },
    ],
  },
  {
    label: '入库管理',
    options: [
      { label: '查看入库单', value: 'wms:inbound:view' },
      { label: '创建 / 编辑 / 删除', value: 'wms:inbound:create' },
      { label: '提交入库单', value: 'wms:inbound:submit' },
      { label: '审核入库单', value: 'wms:inbound:approve' },
      { label: '取消入库单', value: 'wms:inbound:cancel' },
      { label: '收货', value: 'wms:inbound:receive' },
      { label: '上架', value: 'wms:inbound:putaway' },
    ],
  },
  {
    label: '出库管理',
    options: [
      { label: '查看出库单', value: 'wms:outbound:view' },
      { label: '创建 / 删除', value: 'wms:outbound:create' },
      { label: '提交出库单', value: 'wms:outbound:submit' },
      { label: '审核并分配库存', value: 'wms:outbound:approve' },
      { label: '取消出库单', value: 'wms:outbound:cancel' },
      { label: '拣货', value: 'wms:outbound:pick' },
    ],
  },
  {
    label: '盘点管理',
    options: [
      { label: '查看盘点单', value: 'wms:stocktake:view' },
      { label: '创建盘点单', value: 'wms:stocktake:create' },
      { label: '录入实盘', value: 'wms:stocktake:stocktake' },
      { label: '审核盘点', value: 'wms:stocktake:approve' },
      { label: '取消盘点', value: 'wms:stocktake:cancel' },
    ],
  },
  {
    label: '系统管理',
    options: [
      { label: '用户管理', value: 'wms:system:user' },
      { label: '角色管理', value: 'wms:system:role' },
      { label: '操作日志', value: 'wms:system:log' },
    ],
  },
]

const PERMISSION_LABELS = new Map(
  PERMISSION_GROUPS.flatMap((group) => group.options.map((option) => [option.value, option.label] as const)),
)

function isBuiltinRole(row: RoleItem): boolean {
  return row.id === '1'
}

function splitPerms(raw: string): string[] {
  return raw
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

function permissionLabels(raw: string): string[] {
  if (raw.trim() === '*') return ['超级管理员（全部权限）']
  return splitPerms(raw).map((perm) => PERMISSION_LABELS.get(perm) || perm)
}

function visiblePermissionLabels(raw: string): string[] {
  return permissionLabels(raw).slice(0, 3)
}

function permissionTooltip(raw: string): string {
  return permissionLabels(raw).join('\n')
}

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
const dialog = reactive({ visible: false, loading: false, editingId: '' as EntityID })
const formRef = ref<FormInstance>()
const form = reactive({ name: '', remark: '' })
const selectedPerms = ref<string[]>([])
const customPerms = ref('')
const superAdmin = ref(false)

const rules: FormRules = {
  name: [{ required: true, message: '请输入角色名称', trigger: 'blur' }],
}

function resetPermissionForm() {
  selectedPerms.value = []
  customPerms.value = ''
  superAdmin.value = false
}

function openCreate() {
  dialog.editingId = ''
  form.name = ''
  form.remark = ''
  resetPermissionForm()
  dialog.visible = true
}

function openEdit(row: RoleItem) {
  if (isBuiltinRole(row)) return
  dialog.editingId = row.id
  form.name = row.name
  form.remark = row.remark
  const perms = splitPerms(row.perms)
  superAdmin.value = perms.includes('*')
  selectedPerms.value = perms.filter((perm) => PERMISSION_LABELS.has(perm))
  customPerms.value = perms.filter((perm) => !PERMISSION_LABELS.has(perm) && perm !== '*').join(', ')
  dialog.visible = true
}

function buildPerms(): string {
  if (superAdmin.value) return '*'
  return [...new Set([...selectedPerms.value, ...splitPerms(customPerms.value)])].join(',')
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  dialog.loading = true
  try {
    const data = { name: form.name.trim(), perms: buildPerms(), remark: form.remark.trim() }
    if (dialog.editingId) {
      await updateRole(dialog.editingId, data)
      ElMessage.success('修改成功')
    } else {
      await createRole(data)
      ElMessage.success('创建成功')
    }
    dialog.visible = false
    load()
  } finally {
    dialog.loading = false
  }
}

async function onDelete(row: RoleItem) {
  if (isBuiltinRole(row)) return
  try {
    await ElMessageBox.confirm(
      `删除角色「${row.name}」后不可恢复，确定继续吗？`,
      '删除角色',
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
      <el-button v-permission="'wms:system:role'" type="primary" @click="openCreate">新增角色</el-button>
    </div>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column label="角色名称" min-width="170">
        <template #default="{ row }">
          <span class="role-name">{{ row.name }}</span>
          <el-tag v-if="isBuiltinRole(row)" type="danger" size="small" effect="plain">内置</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="权限配置" min-width="300">
        <template #default="{ row }">
          <el-tag v-if="row.perms.trim() === '*'" type="danger" size="small">超级管理员</el-tag>
          <div v-else-if="splitPerms(row.perms).length" class="permission-tags">
            <el-tag v-for="label in visiblePermissionLabels(row.perms)" :key="label" type="info" size="small">
              {{ label }}
            </el-tag>
            <el-tooltip
              v-if="permissionLabels(row.perms).length > 3"
              :content="permissionTooltip(row.perms)"
              placement="top"
            >
              <el-tag type="info" size="small" effect="plain">
                +{{ permissionLabels(row.perms).length - 3 }}
              </el-tag>
            </el-tooltip>
          </div>
          <span v-else class="empty-text">未配置</span>
        </template>
      </el-table-column>
      <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip />
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-tooltip v-if="isBuiltinRole(row)" content="内置超级管理员角色不可修改或删除" placement="top">
            <span class="table-oper table-oper--disabled">
              <el-button size="small" disabled>编辑</el-button>
              <el-button size="small" type="danger" plain disabled>删除</el-button>
            </span>
          </el-tooltip>
          <div v-else class="table-oper">
            <el-button v-permission="'wms:system:role'" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button v-permission="'wms:system:role'" size="small" type="danger" plain @click="onDelete(row)">
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
      :title="dialog.editingId ? '编辑角色' : '新增角色'"
      width="760px"
      top="6vh"
      destroy-on-close
      :close-on-click-modal="false"
      :close-on-press-escape="!dialog.loading"
      :show-close="!dialog.loading"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" maxlength="64" show-word-limit placeholder="例如：仓库管理员" />
        </el-form-item>
        <el-form-item label="权限配置">
          <el-checkbox v-model="superAdmin">超级管理员权限（拥有系统全部权限）</el-checkbox>
          <el-alert
            v-if="superAdmin"
            class="permission-alert"
            type="warning"
            :closable="false"
            show-icon
            title="超级管理员可访问全部功能，请仅在受信任的账号上分配。"
          />
          <div v-else class="permission-groups">
            <section v-for="group in PERMISSION_GROUPS" :key="group.label" class="permission-group">
              <div class="permission-group-title">{{ group.label }}</div>
              <el-checkbox-group v-model="selectedPerms" class="permission-grid">
                <el-checkbox v-for="option in group.options" :key="option.value" :value="option.value">
                  {{ option.label }}
                </el-checkbox>
              </el-checkbox-group>
            </section>
          </div>
        </el-form-item>
        <el-form-item label="自定义权限">
          <el-input
            v-model="customPerms"
            :disabled="superAdmin"
            placeholder="可选，多个权限用英文逗号分隔，例如 wms:custom:action"
          />
          <div class="form-tip">用于兼容后续扩展权限；已勾选权限会自动合并并去重。</div>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" type="textarea" :rows="2" maxlength="255" show-word-limit placeholder="说明角色职责" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="dialog.loading" @click="dialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="dialog.loading" @click="submit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.role-name {
  margin-right: 8px;
  font-weight: 600;
}

.permission-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.empty-text {
  color: var(--el-text-color-secondary);
}

.table-oper--disabled {
  cursor: not-allowed;
}

.permission-groups {
  width: 100%;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  overflow: hidden;
}

.permission-group + .permission-group {
  border-top: 1px solid var(--el-border-color-lighter);
}

.permission-group-title {
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
  font-size: 13px;
  font-weight: 600;
}

.permission-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 16px;
  padding: 10px 12px;
}

.permission-grid :deep(.el-checkbox) {
  margin-right: 0;
}

.permission-alert {
  margin-top: 10px;
}

.form-tip {
  margin-top: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}

@media (max-width: 640px) {
  .permission-grid {
    grid-template-columns: 1fr;
  }
}
</style>
