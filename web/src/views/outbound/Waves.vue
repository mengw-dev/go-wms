<script setup lang="ts">
import { nextTick, onMounted, onScopeDispose, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import BusinessListPage from '@/components/common/BusinessListPage.vue'
import BusinessPagination from '@/components/common/BusinessPagination.vue'
import WavePendingNotice from '@/components/outbound/WavePendingNotice.vue'
import { listWaves, waveCandidates, WAVE_STATUS_TEXT, type Wave } from '@/api/wave'
import type { OutboundOrderItem } from '@/api/types'
import { useWaveCommands } from '@/composables/outbound/useWaveCommands'
import { loadWarehouseOptions, type IdOption } from '@/utils/options'
import { formatTime } from '@/utils'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()
const commands = useWaveCommands()
const { pending, busy, error } = commands
const list = ref<Wave[]>([]), total = ref(0), loading = ref(false)
const warehouses = ref<IdOption[]>([])
const optionsLoading = ref(false), optionsError = ref('')
const query = reactive({ warehouse_id: '', keyword: '', page: 1, page_size: 10 })
const createOpen = ref(false), previewLoading = ref(false)
const form = reactive({ warehouse_id: '', keyword: '', limit: 20, remark: '' })
const candidates = ref<OutboundOrderItem[]>([]), selected = ref<OutboundOrderItem[]>([])
const table = ref<{ toggleAllSelection: () => void; clearSelection: () => void }>()
let seq = 0, previewSeq = 0, alive = true
onScopeDispose(() => { alive = false; seq++; previewSeq++ })
async function load() {
  if (!alive) return
  const current = ++seq
  loading.value = true
  try {
    const data = await listWaves({ ...query, warehouse_id: query.warehouse_id || undefined })
    if (alive && current === seq) { list.value = data.list ?? []; total.value = data.total }
  } finally { if (alive && current === seq) loading.value = false }
}
function invalidatePreview() { previewSeq++; previewLoading.value = false; candidates.value = []; selected.value = [] }
async function preview(byRule: boolean) {
  if (!form.warehouse_id) return
  const current = ++previewSeq, warehouse = form.warehouse_id
  candidates.value = []; selected.value = []; previewLoading.value = true
  try {
    const data = await waveCandidates({ warehouse_id: warehouse, keyword: form.keyword, limit: byRule ? form.limit : 50 })
    if (!alive || current !== previewSeq || !createOpen.value) return
    candidates.value = data ?? []
    // Selection is explicit in the same preview table; confirmation posts these IDs.
    if (byRule) {
      await nextTick()
      if (alive && current === previewSeq && createOpen.value) table.value?.toggleAllSelection()
    }
  } finally { if (alive && current === previewSeq) previewLoading.value = false }
}
async function create() {
  const ack = await commands.run({ action: 'create', data: { order_ids: selected.value.map(o => o.id), remark: form.remark } })
  if (ack && alive) { createOpen.value = false; await router.push(`/outbound/waves/${ack.wave_id}`) }
}
async function recover() {
  const ack = await commands.recover()
  if (ack && alive) await router.push(`/outbound/waves/${ack.wave_id}`)
}
async function discard() {
  try { await ElMessageBox.confirm('请先核对波次列表与业务结果。放弃只清除本机待确认记录，不会撤销已完成的业务。', '放弃待确认记录', { type: 'warning' }) } catch { return }
  if (alive) commands.discard()
}
async function loadWarehouses() {
  if (!auth.hasPerm('wms:basic')) return
  optionsLoading.value = true; optionsError.value = ''
  try {
    const options = await loadWarehouseOptions()
    if (alive) warehouses.value = options
  } catch { if (alive) optionsError.value = '仓库选项加载失败，可重新加载后再创建波次' }
  finally { if (alive) optionsLoading.value = false }
}
onMounted(() => {
  void load()
  void loadWarehouses()
})
</script>

<template>
  <BusinessListPage title="波次与拣货" description="按仓库组织订单，汇总拣货、按槽位分货，最后逐单确认发货。" :total="total">
    <template #actions><el-button v-permission="'wms:outbound:approve'" type="primary" :disabled="busy || !!pending || optionsLoading || !!optionsError" @click="createOpen = true">新建波次</el-button></template>
    <template #filters>
      <el-form inline label-position="top" class="query-form" @submit.prevent="query.page = 1; load()">
        <el-form-item label="仓库"><el-select v-model="query.warehouse_id" clearable placeholder="全部仓库" style="width: 220px"><el-option v-for="w in warehouses" :key="w.id" :value="w.id" :label="w.label" /></el-select></el-form-item>
        <el-form-item label="波次号"><el-input v-model="query.keyword" clearable placeholder="搜索波次号" /></el-form-item>
        <el-form-item label="操作"><el-button native-type="submit" type="primary" plain>查询</el-button><el-button @click="load">刷新</el-button></el-form-item>
      </el-form>
    </template>
    <WavePendingNotice v-if="!createOpen" :pending="pending" :error="error" :busy="busy" @recover="recover" @discard="discard" />
    <el-alert v-if="optionsError" :title="optionsError" type="error" :closable="false"><el-button :loading="optionsLoading" @click="loadWarehouses">重新加载仓库</el-button></el-alert>
    <el-table v-loading="loading" :data="list" max-height="100%" class="wave-table mobile-record-table">
      <el-table-column label="波次号" min-width="250"><template #default="{ row }"><el-link type="primary" @click="router.push(`/outbound/waves/${row.id}`)">{{ row.wave_no }}</el-link></template></el-table-column>
      <el-table-column label="仓库" min-width="180"><template #default="{ row }">{{ warehouses.find(w => w.id === row.warehouse_id)?.label || row.warehouse_id }}</template></el-table-column>
      <el-table-column label="状态" min-width="110"><template #default="{ row }"><el-tag>{{ WAVE_STATUS_TEXT[row.status as keyof typeof WAVE_STATUS_TEXT] }}</el-tag></template></el-table-column>
      <el-table-column prop="claimed_by" label="当前作业员" min-width="130" />
      <el-table-column label="创建时间" min-width="165"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
      <el-table-column label="操作" width="110"><template #default="{ row }"><el-button link type="primary" @click="router.push(`/outbound/waves/${row.id}`)">工作台</el-button></template></el-table-column>
    </el-table>
    <BusinessPagination v-model:current-page="query.page" v-model:page-size="query.page_size" :total="total" @change="load" />
    <el-dialog v-model="createOpen" title="新建波次" class="business-dialog" width="960px" :close-on-click-modal="false" @closed="invalidatePreview">
      <WavePendingNotice :pending="pending" :error="error" :busy="busy" @recover="recover" @discard="discard" />
      <el-alert title="仅可选择同仓、已审核分配且尚未开始拣货的订单。每波最多50单、1000条拣货任务。" type="info" :closable="false" />
      <el-form inline label-position="top" class="preview-form" @submit.prevent="preview(false)">
        <el-form-item label="仓库"><el-select v-model="form.warehouse_id" placeholder="请选择仓库" style="width: 230px" @change="invalidatePreview"><el-option v-for="w in warehouses" :key="w.id" :value="w.id" :label="w.label" /></el-select></el-form-item>
        <el-form-item label="订单号"><el-input v-model="form.keyword" @input="invalidatePreview" /></el-form-item>
        <el-form-item label="规则选单上限"><el-input-number v-model="form.limit" :min="1" :max="50" @change="invalidatePreview" /></el-form-item>
        <el-form-item><el-button :disabled="!form.warehouse_id || previewLoading" @click="preview(false)">手动选择订单</el-button><el-button :disabled="!form.warehouse_id || previewLoading" type="primary" plain @click="preview(true)">按创建时间选单</el-button></el-form-item>
      </el-form>
      <p>规则：按创建时间由早到晚选择。下方是本次预览，确认时会重新校验订单状态。</p>
      <el-table ref="table" v-loading="previewLoading" :data="candidates" row-key="id" max-height="300" @selection-change="selected = $event">
        <el-table-column type="selection" width="45" /><el-table-column prop="order_no" label="出库单号" min-width="220" /><el-table-column prop="biz_order_no" label="业务订单号" min-width="160" /><el-table-column label="创建时间" min-width="160"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
      </el-table>
      <el-input v-model="form.remark" class="preview-form" maxlength="255" placeholder="波次备注（可选）" />
      <template #footer><span>已选择 {{ selected.length }} 单</span><el-button :disabled="busy" @click="createOpen = false">返回</el-button><el-button type="primary" :loading="busy" :disabled="!selected.length || !!pending || previewLoading" @click="create">确认创建波次</el-button></template>
    </el-dialog>
  </BusinessListPage>
</template>

<style scoped>
.preview-form { margin-top: 16px; }
.wave-table { flex: 1; min-height: 160px; }
@media (max-width: 767px) { :deep(.el-form-item) { width: 100%; margin-right: 0; } :deep(.el-select) { width: 100% !important; } }
</style>
