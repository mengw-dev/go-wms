<script setup lang="ts">
import { computed, onMounted, onScopeDispose, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import BusinessListPage from '@/components/common/BusinessListPage.vue'
import BusinessPagination from '@/components/common/BusinessPagination.vue'
import { useInventoryAlerts } from '@/composables/useInventoryAlerts'
import { useAutoRefresh } from '@/composables/autoRefresh'
import { ALERT_STATE_TEXT, deleteInventoryAlertRule, listAlertOptions, saveInventoryAlertRule, type InventoryAlert, type AlertState } from '@/api/inventoryAlerts'
import { loadAllPages } from '@/utils/pagination'
import type { IdOption } from '@/utils/options'
import { useAuthStore } from '@/stores/auth'
import { formatTime } from '@/utils'
import { mobileTableCellStyle } from '@/utils/mobileTable'

const route = useRoute(), router = useRouter()
const auth = useAuthStore(), canManage = computed(() => auth.hasPerm('wms:inventory:alert:manage'))
const { query, result, loading, error, load, search } = useInventoryAlerts(typeof route.query.warehouse_id === 'string' ? route.query.warehouse_id : '')
// Dashboard drill-downs use the same state names as the filter and API.
if (typeof route.query.state === 'string' && Object.hasOwn(ALERT_STATE_TEXT, route.query.state)) query.state = route.query.state as AlertState
const warehouses = ref<IdOption[]>([]), skus = ref<{ id: string; label: string; unit: string }[]>([])
const open = ref(false), saving = ref(false), editing = ref(false), optionsError = ref(false)
const form = reactive({ warehouse_id: '', sku_id: '', min_available_qty: 10, enabled: true })
useAutoRefresh(() => load(), 30000, () => !open.value && !saving.value && document.visibilityState === 'visible')
let alive = true
onScopeDispose(() => { alive = false })
onMounted(async () => {
  void load()
  if (!canManage.value) return
  try {
    const [w, s] = await Promise.all([loadAllPages((page,size) => listAlertOptions('warehouse',page,size)), loadAllPages((page,size) => listAlertOptions('sku',page,size))])
    if (!alive) return
    warehouses.value = w.map(v => ({id:v.id,label:`${v.code}（${v.name}）`}))
    skus.value = s.map(value => ({ id:value.id, label: `${value.code} ${value.name}`, unit: value.unit }))
  } catch { if (alive) optionsError.value = true }
})
function edit(row?: InventoryAlert) {
  editing.value = !!row
  Object.assign(form, row ? { warehouse_id: row.warehouse_id, sku_id: row.sku_id, min_available_qty: row.min_available_qty, enabled: row.enabled } : { warehouse_id: query.warehouse_id, sku_id: '', min_available_qty: 10, enabled: true })
  open.value = true
}
async function save() {
  if (saving.value || !form.warehouse_id || !form.sku_id || !Number.isInteger(form.min_available_qty) || form.min_available_qty < 1) return
  saving.value = true
  try {
    await saveInventoryAlertRule({ ...form })
    if (alive) { open.value = false; ElMessage.success('规则已保存，同一仓库和货品的规则会更新'); await load() }
  } finally { if (alive) saving.value = false }
}
async function remove(row: InventoryAlert) {
  try { await ElMessageBox.confirm(`删除「${row.sku_name || row.sku_id}」在该仓库的预警规则？不会改变库存或单据。`, '删除预警规则', { type: 'warning' }) } catch { return }
  if (!alive || saving.value) return
  saving.value = true
  try { await deleteInventoryAlertRule(row.id); if (alive) { if (result.value.list.length === 1 && query.page > 1) query.page--; await load() } }
  finally { if (alive) saving.value = false }
}
function inventory(row: InventoryAlert) {
  void router.push({ path: '/inventory', query: { warehouse_id: row.warehouse_id, sku_keyword: row.sku_code, in_stock_only: 'false' } })
}
function changeState(state: AlertState) { query.state = state; void search() }
</script>

<template>
  <BusinessListPage title="库存预警" description="按仓库与货品监控可用库存，及时发现缺货和低库存。" :total="result.total">
    <template #actions><el-button v-if="canManage" type="primary" :disabled="optionsError || saving" @click="edit()">配置预警规则</el-button></template>
    <template #filters>
      <el-form inline label-position="top" class="query-form" @submit.prevent="search">
        <el-form-item label="仓库"><el-select v-model="query.warehouse_id" clearable placeholder="全部仓库"><el-option v-for="w in result.warehouses" :key="w.id" :value="w.id" :label="`${w.code}（${w.name}）`" /></el-select></el-form-item>
        <el-form-item label="货品"><el-input v-model="query.sku_keyword" placeholder="编码 / 名称 / 条码" clearable /></el-form-item>
        <el-form-item label="状态"><el-select v-model="query.state"><el-option v-for="(label, value) in ALERT_STATE_TEXT" :key="value" :label="label" :value="value" /></el-select></el-form-item>
        <el-form-item><el-button native-type="submit" type="primary" plain>查询</el-button><el-button @click="load">刷新</el-button></el-form-item>
      </el-form>
    </template>
    <div class="alert-overview">
      <button type="button" @click="changeState('OUT_OF_STOCK')"><span>可用库存为零</span><strong>{{ result.summary.out_of_stock }}</strong></button>
      <button type="button" @click="changeState('LOW')"><span>低于预警下限</span><strong>{{ result.summary.low }}</strong></button>
      <button type="button" @click="changeState('ALL')"><span>已配置规则</span><strong>{{ result.summary.configured }}</strong></button>
      <span>查询于 {{ result.checked_at ? formatTime(result.checked_at) : '—' }}</span>
    </div>
    <el-alert v-if="error || optionsError" :title="error || '基础资料选项加载失败，请刷新页面后配置规则'" type="error" :closable="false" />
    <p class="alert-note">仅监控已配置且启用的规则；可用库存低于下限时预警，等于下限即正常。未生成库存行按零计算，已分配库存不计入可用量。</p>
    <el-table v-loading="loading" :data="result.list" class="mobile-record-table" :cell-style="mobileTableCellStyle" :max-height="480" row-key="id">
      <el-table-column label="货品" min-width="190"><template #default="{ row }"><strong>{{ row.sku_name || '货品已删除' }}</strong><div class="secondary">{{ row.sku_code || row.sku_id }}</div></template></el-table-column>
      <el-table-column label="仓库" min-width="170"><template #default="{ row }">{{ row.warehouse_name || '仓库已删除' }}<div class="secondary">{{ row.warehouse_code }}</div></template></el-table-column>
      <el-table-column label="状态" min-width="165"><template #default="{ row }"><el-tag :type="row.state === 'OUT_OF_STOCK' ? 'danger' : row.state === 'LOW' ? 'warning' : row.state === 'NORMAL' ? 'success' : 'info'">{{ ALERT_STATE_TEXT[row.state as AlertState] }}</el-tag></template></el-table-column>
      <el-table-column label="可用 / 下限" min-width="130"><template #default="{ row }"><strong>{{ row.available_qty }}</strong> / {{ row.min_available_qty }} {{ row.unit }}</template></el-table-column>
      <el-table-column label="现存 / 已分配" min-width="140"><template #default="{ row }">{{ row.stock_quantity }} / {{ row.allocated_qty }} {{ row.unit }}</template></el-table-column>
      <el-table-column label="距下限差额" min-width="125"><template #default="{ row }">{{ ['LOW', 'OUT_OF_STOCK'].includes(row.state) ? row.gap_qty + ' ' + row.unit : '—' }}</template></el-table-column>
      <el-table-column label="操作" min-width="200"><template #default="{ row }"><el-button link type="primary" :disabled="row.state === 'UNAVAILABLE'" @click="inventory(row)">库存明细</el-button><el-button v-if="canManage" link type="primary" :disabled="saving || optionsError || row.state === 'UNAVAILABLE'" @click="edit(row)">规则</el-button><el-button v-if="canManage" link type="danger" :disabled="saving" @click="remove(row)">删除</el-button></template></el-table-column>
      <template #empty>{{ result.summary.configured ? '当前筛选条件下暂无预警记录' : '尚未配置预警规则，请先按仓库和货品设置下限' }}</template>
    </el-table>
    <BusinessPagination v-model:current-page="query.page" v-model:page-size="query.page_size" class="pagination" :total="result.total" @change="load" />
    <el-dialog v-model="open" :title="editing ? '修改预警规则' : '配置预警规则'" width="520px" class="business-dialog" :close-on-click-modal="false" :close-on-press-escape="!saving">
      <el-form label-position="top" @submit.prevent="save">
        <el-form-item label="仓库"><el-select v-model="form.warehouse_id" :disabled="editing || saving" filterable placeholder="选择仓库"><el-option v-for="w in warehouses" :key="w.id" :value="w.id" :label="w.label" /></el-select></el-form-item>
        <el-form-item label="货品"><el-select v-model="form.sku_id" :disabled="editing || saving" filterable placeholder="选择货品"><el-option v-for="s in skus" :key="s.id" :value="s.id" :label="s.label" /></el-select></el-form-item>
        <el-form-item label="可用库存下限"><el-input-number v-model="form.min_available_qty" :min="1" :max="2147483647" :precision="0" :disabled="saving" /><span class="rule-unit">{{ skus.find(s => s.id === form.sku_id)?.unit }}</span></el-form-item>
        <el-form-item label="启用规则"><el-switch v-model="form.enabled" :disabled="saving" /></el-form-item>
      </el-form>
      <el-alert title="以货品的库存计量单位设置；这是预警下限，不是采购建议量。保存只修改规则，不会增加或扣减库存。" type="info" :closable="false" />
      <template #footer><el-button :disabled="saving" @click="open = false">返回</el-button><el-button type="primary" :loading="saving" :disabled="!form.warehouse_id || !form.sku_id" @click="save">保存规则</el-button></template>
    </el-dialog>
  </BusinessListPage>
</template>

<style scoped>
.alert-overview { display: flex; gap: 12px; flex-wrap: wrap; align-items: center; margin-bottom: 12px; flex: none; }
.alert-overview button { display: flex; align-items: center; gap: 20px; padding: 10px 14px; border: 1px solid var(--el-border-color-lighter); background: var(--el-fill-color-lighter); color: var(--el-text-color-regular); border-radius: 8px; cursor: pointer; }
.alert-overview strong { font-size: 21px; color: var(--el-text-color-primary); }
.alert-overview > span, .alert-note, .secondary { color: var(--el-text-color-secondary); font-size: 12px; }
.alert-note { margin: 0 0 12px; flex: none; line-height: 1.7; } .rule-unit { margin-left: 10px; }
@media (max-width: 767px) { .alert-overview { gap: 8px; } .alert-overview button { gap: 10px; padding: 8px; } }
</style>
