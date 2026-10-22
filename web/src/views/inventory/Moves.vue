<script setup lang="ts">
import { computed, onScopeDispose, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import BusinessListPage from '@/components/common/BusinessListPage.vue'
import BusinessPagination from '@/components/common/BusinessPagination.vue'
import ResizableTableArea from '@/components/common/ResizableTableArea.vue'
import { getMoveSource, listMoveLocations, listStockMoves, type MoveSource, type StockMove } from '@/api/stockMoves'
import { useStockMove, movePayload } from '@/composables/useStockMove'
import { operationErrorMessage, operationStateKey } from '@/composables/operationLifecycle'
import type { PendingOperation } from '@/utils/pendingOperations'
import { useAuthStore } from '@/stores/auth'
import { formatTime } from '@/utils'
import { mobileTableCellStyle } from '@/utils/mobileTable'

const route = useRoute(), router = useRouter(), auth = useAuthStore()
const canMove = computed(() => auth.hasPerm('wms:inventory:move'))
const operation = useStockMove()
const { pending, error: operationError, busy } = operation
const query = reactive({ keyword: '', page: 1, page_size: 10 })
const rows = ref<StockMove[]>([]), total = ref(0), loading = ref(false), error = ref('')
const open = ref(false), source = ref<MoveSource>(), sourceLoading = ref(false), sourceError = ref('')
const locations = ref<{id: string; code: string}[]>([]), locationsLoading = ref(false), locationsError = ref(''), locationTotal = ref(0)
const form = reactive({ target_location_id: '', quantity: 1, reason: '' })
let alive = true, listSeq = 0, sourceSeq = 0, locationsSeq = 0
onScopeDispose(() => { alive = false; listSeq++; sourceSeq++; locationsSeq++ })
function identity() { return operationStateKey('stock-move') }
async function load() {
  const seq = ++listSeq, state = identity()
  loading.value = true; error.value = ''
  try {
    const data = await listStockMoves({ ...query })
    if (!alive || seq !== listSeq || state !== identity()) return
    rows.value = data.list ?? []; total.value = data.total
  } catch (e) { if (alive && seq === listSeq && state === identity()) { rows.value = []; total.value = 0; error.value = operationErrorMessage(e, '移库记录加载失败，请重试') } }
  finally { if (alive && seq === listSeq) loading.value = false }
}
function search() { query.page = 1; void load() }
async function searchLocations(keyword = '') {
  const id = source.value?.id
  if (!id || !open.value) return
  const seq = ++locationsSeq, state = identity()
  locationsLoading.value = true; locationsError.value = ''; locations.value = []
  // A new remote search also clears the selection, so a hidden old option is never submitted.
  form.target_location_id = ''
  try {
    const data = await listMoveLocations(id, keyword)
    if (!alive || seq !== locationsSeq || state !== identity() || !open.value) return
    locations.value = data.list ?? []; locationTotal.value = data.total
  } catch (e) { if (alive && seq === locationsSeq && state === identity()) locationsError.value = operationErrorMessage(e, '目标库位加载失败，请重试') }
  finally { if (alive && seq === locationsSeq) locationsLoading.value = false }
}
async function begin(id: string) {
  if (busy.value || !canMove.value) return
  const seq = ++sourceSeq, state = identity()
  source.value = undefined; sourceError.value = ''; sourceLoading.value = true; open.value = true
  locationsSeq++; locations.value = []; locationsError.value = ''; locationTotal.value = 0
  Object.assign(form, { target_location_id: '', quantity: 1, reason: '' })
  try {
    const data = await getMoveSource(id)
    if (!alive || seq !== sourceSeq || state !== identity() || !open.value) return
    source.value = data
    if (!data.enabled || data.available_qty < 1) { sourceError.value = '该库存或关联资料不可用，或没有可移动的可用量'; return }
    await searchLocations()
  } catch (e) { if (alive && seq === sourceSeq && state === identity()) sourceError.value = operationErrorMessage(e, '来源库存加载失败，请返回库存查询重试') }
  finally { if (alive && seq === sourceSeq) sourceLoading.value = false }
}
function closeDialog(done: () => void) { if (!busy.value) done() }
const valid = computed(() => !!source.value?.enabled && !sourceError.value && !locationsError.value && !sourceLoading.value && !locationsLoading.value && locations.value.some(l => l.id === form.target_location_id) && Number.isInteger(form.quantity) && form.quantity > 0 && form.quantity <= (source.value?.available_qty ?? 0) && !!form.reason.trim() && [...form.reason.trim()].length <= 255 && !pending.value.length)
function completed(result?: StockMove) {
  if (!result || !alive) return
  open.value = false; ElMessage.success(`移库已完成：${result.move_no}`); void load()
}
async function submit() {
  if (!valid.value || busy.value || !canMove.value || !source.value) return
  completed(await operation.send({ source_inventory_id: source.value.id, target_location_id: form.target_location_id, quantity: form.quantity, reason: form.reason.trim() }))
}
async function recover(item: PendingOperation, retry: boolean) {
  if (!canMove.value && retry) return
  completed(await (retry ? operation.retry(item) : operation.check(item)))
}
async function forget(item: PendingOperation) {
  try { await ElMessageBox.confirm('请先核对移库记录和库存流水。清除只删除本机待确认记录，不会撤销库存变动；再次发起将使用新请求编号。确认已人工核对？', '清除本地待确认记录', { type: 'warning', confirmButtonText: '已核对，清除记录' }) } catch { return }
  if (alive) operation.forget(item)
}
function ledger(row: StockMove) { void router.push({ path: '/inventory', query: { order_no: row.move_no } }) }
watch(identity, () => { sourceSeq++; locationsSeq++; open.value = false; operation.restore(); void load() }, { immediate: true })
watch(() => route.query.source_inventory_id, id => { if (typeof id === 'string' && /^[1-9]\d{0,18}$/.test(id)) void begin(id) }, { immediate: true })
</script>

<template>
  <BusinessListPage title="仓内移库" description="同仓库内移动可用库存，保留批次与入库时间，记录每次库位变动。" :total="total">
    <template #actions><el-button type="primary" @click="router.push('/inventory')">从库存查询发起移库</el-button></template>
    <template #filters><el-form inline class="query-form" @submit.prevent="search"><el-form-item label="移库记录"><el-input v-model="query.keyword" clearable placeholder="移库号 / 货品 / 批次" maxlength="128" /></el-form-item><el-form-item><el-button native-type="submit" type="primary" plain>查询</el-button><el-button @click="load">刷新</el-button></el-form-item></el-form></template>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <el-alert v-if="operationError" :title="operationError" type="warning" :closable="false" />
    <section v-if="pending.length" class="move-pending" aria-label="待确认移库">
      <strong>待确认移库 · {{ pending.length }} 笔</strong>
      <p>响应未确认时，请先核对结果；重试使用原请求编号和原参数。</p>
      <div v-for="item in pending" :key="item.operationKey" class="pending-item">
        <span>来源库存 {{ item.objectId }} → 库位 {{ movePayload(item)?.target_location_id || '参数待核对' }} · 数量 {{ movePayload(item)?.quantity ?? '—' }}<br><small>请求编号 {{ item.operationKey }}</small></span>
        <div><el-button :disabled="busy" @click="recover(item, false)">核对结果</el-button><el-button v-if="canMove" :disabled="busy" @click="recover(item, true)">按原请求重试</el-button><el-button :disabled="busy" link type="danger" @click="forget(item)">核对后清除</el-button></div>
      </div>
    </section>
    <ResizableTableArea storage-key="stock-moves">
      <el-table v-loading="loading" :data="rows" class="mobile-record-table" :cell-style="mobileTableCellStyle" max-height="var(--business-table-height)" row-key="id">
        <el-table-column label="移库号" min-width="215"><template #default="{ row }"><strong>{{ row.move_no }}</strong><div class="secondary">{{ formatTime(row.created_at) }}</div></template></el-table-column>
        <el-table-column label="货品 / 批次" min-width="200"><template #default="{ row }">{{ row.sku_name }}<div class="secondary">{{ row.sku_code }} · {{ row.batch_no }}</div></template></el-table-column>
        <el-table-column prop="warehouse_code" label="仓库" min-width="100" />
        <el-table-column label="库位变动" min-width="210"><template #default="{ row }">{{ row.source_location_code }} → {{ row.target_location_code }}</template></el-table-column>
        <el-table-column label="移动数量" min-width="115" align="right"><template #default="{ row }"><strong>{{ row.quantity }}</strong> {{ row.unit }}</template></el-table-column>
        <el-table-column prop="operator" label="操作人" min-width="115" />
        <el-table-column prop="reason" label="移库原因" min-width="210" show-overflow-tooltip />
        <el-table-column label="操作" width="110" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="ledger(row)">库存流水</el-button></template></el-table-column>
        <template #empty>{{ error ? '记录加载失败，请刷新重试' : '暂无移库记录，可从库存明细中选择可用库存发起' }}</template>
      </el-table>
    </ResizableTableArea>
    <BusinessPagination v-model:current-page="query.page" v-model:page-size="query.page_size" class="pagination" :total="total" @change="load" />
    <el-dialog v-model="open" title="仓内移库" width="580px" class="business-dialog" :close-on-click-modal="false" :close-on-press-escape="!busy" :before-close="closeDialog">
      <div v-loading="sourceLoading">
        <el-alert v-if="sourceError || operationError" :title="sourceError || operationError" type="warning" :closable="false" />
        <div v-if="source" class="move-source"><strong>{{ source.sku_name }} · {{ source.sku_code }}</strong><p>{{ source.warehouse_code }} / {{ source.location_code }} · 批次 {{ source.batch_no }}</p><p>可移动 {{ source.available_qty }} {{ source.unit }} · 原入库 {{ formatTime(source.stock_in_time) }}</p></div>
        <el-form label-position="top" @submit.prevent="submit">
          <el-form-item label="目标库位"><el-select v-model="form.target_location_id" filterable remote :remote-method="searchLocations" :loading="locationsLoading" :disabled="busy || !source?.enabled || !!sourceError" placeholder="搜索同仓库的其他启用库位"><el-option v-for="l in locations" :key="l.id" :label="l.code" :value="l.id" /></el-select><small v-if="locationTotal > 100">显示前 100 个库位，请输入库位编码缩小范围。</small></el-form-item>
          <el-alert v-if="locationsError" :title="locationsError" type="error" :closable="false" /><el-button v-if="locationsError" :disabled="busy" @click="searchLocations()">重新加载库位</el-button>
          <el-form-item v-if="source && !sourceLoading" label="移动数量"><el-input-number :key="source.id" v-model="form.quantity" :min="1" :max="Math.max(1, source.available_qty)" :precision="0" :disabled="busy || !source.enabled" /><span class="move-unit">{{ source.unit }}</span></el-form-item>
          <el-form-item label="移库原因"><el-input v-model="form.reason" type="textarea" :rows="2" maxlength="255" show-word-limit :disabled="busy" placeholder="例如：库位整理、调整存放位置" /></el-form-item>
        </el-form>
        <p class="secondary">仅移动可用库存，已分配数量保留。确认后立即完成，请核实目标库位和数量。</p>
      </div>
      <template #footer><el-button :disabled="busy" @click="open = false">返回</el-button><el-button type="primary" :loading="busy" :disabled="!valid || !canMove" @click="submit">确认移库</el-button></template>
    </el-dialog>
  </BusinessListPage>
</template>

<style scoped>
.secondary, .move-pending p, small { color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.7; }
.move-source, .move-pending { padding: 12px 16px; border: 1px solid var(--el-border-color-lighter); background: var(--el-fill-color-lighter); border-radius: 8px; margin-bottom: 16px; }
.move-source p, .move-pending p { margin: 6px 0; }
.move-pending { flex: none; max-height: 240px; overflow: auto; }
.pending-item { display: flex; justify-content: space-between; gap: 12px; flex-wrap: wrap; padding: 8px 0; overflow-wrap: anywhere; }
.pending-item > div { display: flex; flex-wrap: wrap; gap: 8px; } .pending-item .el-button { margin: 0; }
.move-unit { margin-left: 10px; }
</style>
