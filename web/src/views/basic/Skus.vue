<script setup lang="ts">
import BusinessPagination from '@/components/common/BusinessPagination.vue'
import { mobileTableCellStyle } from '@/utils/mobileTable'
import BusinessListPage from '@/components/common/BusinessListPage.vue'
import { onMounted, onScopeDispose, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { createSku, deleteSku, listSkus, updateSku } from '@/api/basic'
import type { EntityID,  SkuItem } from '@/api/types'
import BarcodeLabelPreview from '@/components/printing/BarcodeLabelPreview.vue'
import { skuBarcodeLabel } from '@/utils/barcodeLabels'
import { cleanParams, formatTime } from '@/utils'

// ---------- 列表 ----------
const loading = ref(false)
const list = ref<SkuItem[]>([])
const total = ref(0)
const selectedRows = ref<SkuItem[]>([])
const labels = ref<InstanceType<typeof BarcodeLabelPreview>>()
function previewLabels(rows: SkuItem[]) { labels.value?.open(rows.map(skuBarcodeLabel)) }
const query = reactive({ page: 1, page_size: 10, keyword: '' })

let loadSeq = 0
let disposed = false
onScopeDispose(() => {
  disposed = true
  loadSeq++
})

async function load() {
  if (disposed) return
  const seq = ++loadSeq
  loading.value = true
  try {
    const data = await listSkus(cleanParams({ ...query }))
    if (seq !== loadSeq) return
    list.value = data.list ?? []
    total.value = data.total ?? 0
  } finally {
    if (seq === loadSeq) loading.value = false
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
const form = reactive({ code: '', barcode: '', name: '', spec: '', unit: '' })

const rules: FormRules = {
  code: [{ required: true, message: '请输入货品编码', trigger: 'blur' }],
  barcode: [{ required: true, message: '请输入条码', trigger: 'blur' }],
  name: [{ required: true, message: '请输入货品名称', trigger: 'blur' }],
}

function openCreate() {
  dialog.editingId = ''
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
    await ElMessageBox.confirm(
      `删除货品「${row.name}」前请确认该货品没有库存，确定继续吗？`,
      '删除货品',
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
  await deleteSku(row.id)
  ElMessage.success('删除成功')
  load()
}
</script>

<template>
  <BusinessListPage title="货品管理" description="维护货品编码、条码与规格，建立统一的业务资料。" :total="total">
    <template #actions>
      <el-button :disabled="!list.length" @click="previewLabels(selectedRows.length ? selectedRows : list)">{{ selectedRows.length ? `所选标签（${selectedRows.length}）` : '本页标签预览' }}</el-button>
      <el-button v-permission="'wms:basic'" type="primary" @click="openCreate">新增货品</el-button>
    </template>
    <template #filters>
      <el-form inline class="query-form" @submit.prevent="search">
        <el-form-item label="关键字">
          <el-input v-model="query.keyword" placeholder="编码/条码/名称" clearable @keyup.enter="search" @clear="search" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" plain @click="search">查询</el-button>
        </el-form-item>
      </el-form>
    </template>

    <el-table class="mobile-record-table" :cell-style="mobileTableCellStyle" max-height="var(--business-table-height)" v-loading="loading" :data="list" row-key="id" border stripe @selection-change="selectedRows = $event">
      <el-table-column type="selection" width="42" />
      <el-table-column prop="code" label="编码" min-width="120" />
      <el-table-column prop="barcode" label="条码" min-width="140" />
      <el-table-column prop="name" label="名称" min-width="160" />
      <el-table-column prop="spec" label="规格" min-width="120" />
      <el-table-column prop="unit" label="单位" width="80" />
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="210" fixed="right">
        <template #default="{ row }">
          <div class="table-oper">
            <el-button size="small" @click="previewLabels([row])">标签</el-button>
            <el-button v-permission="'wms:basic'" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button v-permission="'wms:basic'" size="small" type="danger" plain @click="onDelete(row)">删除</el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <BusinessPagination
      v-model:current-page="query.page"
      v-model:page-size="query.page_size"
      class="pagination"
      layout="total, sizes, prev, pager, next, jumper"
      :total="total"
      :page-sizes="[10, 20, 50]"
      @current-change="load"
      @size-change="search"
    />

    <BarcodeLabelPreview ref="labels" />
    <el-dialog class="business-dialog"
      v-model="dialog.visible"
      :title="dialog.editingId ? '编辑货品' : '新增货品'"
      width="480px"
      destroy-on-close
      :close-on-click-modal="false"
      :close-on-press-escape="!dialog.loading"
      :show-close="!dialog.loading"
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
        <el-button :disabled="dialog.loading" @click="dialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="dialog.loading" @click="submit">确定</el-button>
      </template>
    </el-dialog>
  </BusinessListPage>
</template>
