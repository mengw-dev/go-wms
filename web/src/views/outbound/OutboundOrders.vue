<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveOutboundOrder,
  cancelOutboundOrder,
  createOutboundOrder,
  deleteOutboundOrder,
  listOutboundOrders,
  submitOutboundOrder,
} from '@/api/outbound'
import type { OutboundOrderItem } from '@/api/types'
import { OUTBOUND_STATUS_OPTIONS, statusTag, statusText } from '@/constants'
import { cleanParams, formatTime } from '@/utils'
import { loadSkuMap, loadWarehouseOptions, toOptionMap, type IdOption } from '@/utils/options'
import PickDialog from '@/components/PickDialog.vue'

const router = useRouter()

// ---------- 基础选项 ----------
const warehouseOptions = ref<IdOption[]>([])
const warehouseMap = ref<Record<number, string>>({})
const skuOptions = ref<IdOption[]>([])

onMounted(async () => {
  warehouseOptions.value = await loadWarehouseOptions()
  warehouseMap.value = toOptionMap(warehouseOptions.value)
  const map = await loadSkuMap()
  skuOptions.value = Object.entries(map).map(([id, sku]) => ({
    id: Number(id),
    label: `${sku.code} ${sku.name}`,
  }))
  load()
})

// ---------- 列表 ----------
const loading = ref(false)
const list = ref<OutboundOrderItem[]>([])
const total = ref(0)
const query = reactive({
  page: 1,
  page_size: 10,
  warehouse_id: '' as number | '',
  status: '',
  keyword: '',
})

async function load() {
  loading.value = true
  try {
    const data = await listOutboundOrders(cleanParams({ ...query }))
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

// ---------- 行操作 ----------
async function onSubmit(row: OutboundOrderItem) {
  try {
    await ElMessageBox.confirm(`确定提交出库单「${row.order_no}」吗？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await submitOutboundOrder(row.id)
  ElMessage.success('提交成功')
  load()
}

async function onApprove(row: OutboundOrderItem) {
  try {
    await ElMessageBox.confirm(`确定审核（分配库存）出库单「${row.order_no}」吗？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await approveOutboundOrder(row.id)
  ElMessage.success('审核完成，库存已分配')
  load()
}

async function onCancel(row: OutboundOrderItem) {
  try {
    await ElMessageBox.confirm(`确定取消出库单「${row.order_no}」吗？取消后将释放已分配库存。`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await cancelOutboundOrder(row.id)
  ElMessage.success('已取消')
  load()
}

async function onDelete(row: OutboundOrderItem) {
  try {
    await ElMessageBox.confirm(`确定删除出库单「${row.order_no}」吗？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  await deleteOutboundOrder(row.id)
  ElMessage.success('删除成功')
  load()
}

function goDetail(row: OutboundOrderItem) {
  router.push(`/outbound/orders/${row.id}`)
}

// ---------- 新建 ----------
const createDialog = reactive({ visible: false, loading: false })
const createForm = reactive({
  warehouse_id: undefined as number | undefined,
  biz_order_no: '',
  remark: '',
  details: [] as { sku_id: number | undefined; expected_qty: number }[],
})

function openCreate() {
  createForm.warehouse_id = undefined
  createForm.biz_order_no = ''
  createForm.remark = ''
  createForm.details = [{ sku_id: undefined, expected_qty: 1 }]
  createDialog.visible = true
}

function addDetail() {
  createForm.details.push({ sku_id: undefined, expected_qty: 1 })
}

function removeDetail(index: number) {
  createForm.details.splice(index, 1)
}

async function submitCreate() {
  if (!createForm.warehouse_id) {
    ElMessage.warning('请选择仓库')
    return
  }
  if (!createForm.biz_order_no.trim()) {
    ElMessage.warning('请输入业务订单号')
    return
  }
  const details = createForm.details.filter((d) => d.sku_id && d.expected_qty > 0)
  if (details.length === 0) {
    ElMessage.warning('请至少填写一行有效的明细（选择货品且数量大于 0）')
    return
  }
  createDialog.loading = true
  try {
    await createOutboundOrder({
      warehouse_id: createForm.warehouse_id,
      biz_order_no: createForm.biz_order_no.trim(),
      remark: createForm.remark,
      details: details.map((d) => ({ sku_id: d.sku_id!, expected_qty: d.expected_qty })),
    })
    ElMessage.success('创建成功')
    createDialog.visible = false
    load()
  } finally {
    createDialog.loading = false
  }
}

// ---------- 拣货 ----------
const pickRef = ref<InstanceType<typeof PickDialog>>()

function openPick(row: OutboundOrderItem) {
  pickRef.value?.open(row.id)
}
</script>

<template>
  <div class="page-card">
    <el-form inline class="query-form" @submit.prevent="search">
      <el-form-item label="仓库">
        <el-select v-model="query.warehouse_id" placeholder="全部" clearable style="width: 200px" @change="search">
          <el-option v-for="w in warehouseOptions" :key="w.id" :label="w.label" :value="w.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="query.status" placeholder="全部" clearable style="width: 130px" @change="search">
          <el-option v-for="s in OUTBOUND_STATUS_OPTIONS" :key="s" :label="statusText(s)" :value="s" />
        </el-select>
      </el-form-item>
      <el-form-item label="单号">
        <el-input v-model="query.keyword" placeholder="单号模糊搜索" clearable style="width: 180px" @keyup.enter="search" @clear="search" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="search">查询</el-button>
      </el-form-item>
    </el-form>

    <div class="toolbar">
      <span />
      <el-button type="primary" @click="openCreate">新建出库单</el-button>
    </div>

    <el-table v-loading="loading" :data="list" border stripe>
      <el-table-column prop="order_no" label="出库单号" min-width="170">
        <template #default="{ row }">
          <el-link type="primary" @click="goDetail(row)">{{ row.order_no }}</el-link>
        </template>
      </el-table-column>
      <el-table-column prop="biz_order_no" label="业务订单号" min-width="150" show-overflow-tooltip />
      <el-table-column label="仓库" min-width="150">
        <template #default="{ row }">{{ warehouseMap[row.warehouse_id] || row.warehouse_id }}</template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusTag(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="expected_qty" label="需求数量" width="100" align="right" />
      <el-table-column prop="allocated_qty" label="已分配" width="90" align="right" />
      <el-table-column prop="picked_qty" label="已拣货" width="90" align="right" />
      <el-table-column prop="remark" label="备注" min-width="120" show-overflow-tooltip />
      <el-table-column prop="created_by" label="创建人" width="100" />
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="250" fixed="right">
        <template #default="{ row }">
          <div class="table-oper">
            <el-button size="small" @click="goDetail(row)">详情</el-button>
            <template v-if="row.status === 'DRAFT'">
              <el-button size="small" type="success" plain @click="onSubmit(row)">提交</el-button>
              <el-button size="small" type="danger" plain @click="onDelete(row)">删除</el-button>
            </template>
            <template v-else-if="row.status === 'SUBMITTED'">
              <el-button size="small" type="success" plain @click="onApprove(row)">审核</el-button>
              <el-button size="small" type="danger" plain @click="onCancel(row)">取消</el-button>
            </template>
            <template v-else-if="row.status === 'APPROVED' || row.status === 'PICKING'">
              <el-button size="small" type="primary" plain @click="openPick(row)">拣货</el-button>
              <el-button size="small" type="danger" plain @click="onCancel(row)">取消</el-button>
            </template>
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

    <!-- 新建 -->
    <el-dialog v-model="createDialog.visible" title="新建出库单" width="760px" destroy-on-close>
      <el-form label-width="90px">
        <el-form-item label="仓库" required>
          <el-select v-model="createForm.warehouse_id" placeholder="选择仓库" style="width: 300px">
            <el-option v-for="w in warehouseOptions" :key="w.id" :label="w.label" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="业务订单号" required>
          <el-input v-model="createForm.biz_order_no" placeholder="业务订单号（幂等键，重复将创建失败）" style="width: 300px" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="createForm.remark" type="textarea" :rows="2" placeholder="备注（可选）" />
        </el-form-item>
        <el-form-item label="明细" required>
          <div class="detail-editor">
            <div v-for="(item, index) in createForm.details" :key="index" class="detail-row">
              <el-select v-model="item.sku_id" placeholder="选择货品" filterable style="width: 320px">
                <el-option v-for="s in skuOptions" :key="s.id" :label="s.label" :value="s.id" />
              </el-select>
              <el-input-number v-model="item.expected_qty" :min="1" controls-position="right" style="width: 140px" />
              <el-button type="danger" plain circle size="small" @click="removeDetail(index)">
                <el-icon><Delete /></el-icon>
              </el-button>
            </div>
            <el-button type="primary" plain size="small" @click="addDetail">+ 添加明细行</el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="createDialog.loading" @click="submitCreate">保存</el-button>
      </template>
    </el-dialog>

    <!-- 拣货 -->
    <PickDialog ref="pickRef" @success="load" />
  </div>
</template>

<style scoped>
.detail-editor {
  width: 100%;
}

.detail-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
</style>
