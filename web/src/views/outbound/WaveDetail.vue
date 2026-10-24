<script setup lang="ts">
import { computed, onScopeDispose, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { getWave, WAVE_STATUS_TEXT, type WaveDetail, type WaveCommand, type WaveLine, type WaveItem, type WaveOrder } from '@/api/wave'
import WavePendingNotice from '@/components/outbound/WavePendingNotice.vue'
import WaveAbortPanel from '@/components/outbound/WaveAbortPanel.vue'
const closureBlocked = ref(false)
import { useWaveCommands } from '@/composables/outbound/useWaveCommands'
import { useAuthStore } from '@/stores/auth'
import { formatTime } from '@/utils'
import { useLeaseActive } from '@/composables/useLeaseActive'

const route = useRoute(), router = useRouter(), auth = useAuthStore()
const commands = useWaveCommands(), { pending, busy, error } = commands
const data = ref<WaveDetail | null>(null), loading = ref(false), loadError = ref(''), tab = ref('collect')
const dialog = ref(false), selected = ref<{ action: 'collect' | 'sort'; id: string; title: string; max: number } | null>(null)
const form = reactive({ sku_code: '', location_code: '', batch_no: '', slot_code: '', qty: 1 })
let seq = 0, alive = true
const waveID = computed(() => String(route.params.id))
const terminal = computed(() => !data.value || ['COMPLETED', 'CANCELLED', 'ABORTED'].includes(data.value.wave.status))
const leaseActive = useLeaseActive(() => data.value?.wave.lease_expire_at)
const owner = computed(() => !!data.value && data.value.wave.claimed_by === auth.user?.username && leaseActive.value)
const canWrite = computed(() => !terminal.value && !closureBlocked.value && !busy.value && !pending.value && owner.value && auth.hasPerm('wms:outbound:pick'))
const pickedLines = computed(() => data.value?.lines.filter(l => l.picked_qty === l.target_qty).length ?? 0)
const sortedItems = computed(() => data.value?.items.filter(i => i.sorted_qty === i.target_qty).length ?? 0)
const shipped = computed(() => data.value?.orders.filter(o => o.shipped_at).length ?? 0)
const lineMap = computed(() => new Map(data.value?.lines.map(l => [l.id, l]) ?? []))
const orderMap = computed(() => new Map(data.value?.orders.map(o => [o.id, o]) ?? []))
const sortRows = computed(() => data.value?.items.map(item => ({ ...item, line: lineMap.value.get(item.line_id), order: orderMap.value.get(item.wave_order_id) })) ?? [])
function orderSorted(order: WaveOrder) { const rows = data.value?.items.filter(i => i.wave_order_id === order.id) ?? []; return rows.length > 0 && rows.every(i => i.sorted_qty === i.target_qty) }
async function load() {
  if (!alive) return
  const current = ++seq, id = waveID.value
  loading.value = true; loadError.value = ''
  try {
    const result = await getWave(id)
    if (!alive || seq !== current || waveID.value !== id) return
    data.value = result
  } catch { if (alive && current === seq) loadError.value = '波次加载失败，请重试' }
  finally { if (alive && seq === current) loading.value = false }
}
watch(waveID, () => { data.value = null; dialog.value = false; tab.value = 'collect'; void load() }, { immediate: true })
onScopeDispose(() => { alive = false; seq++ })
async function perform(command: WaveCommand) {
  if (closureBlocked.value) return
  const id = waveID.value
  const ack = await commands.run(command)
  if (ack && alive && waveID.value === id) { dialog.value = false; await load() }
}
function openCollect(line: WaveLine) {
  selected.value = { action: 'collect', id: line.id, title: `${line.sku_name} · 来源库位 ${line.location_code} · 批次 ${line.batch_no || '无批次'}`, max: Math.min(line.target_qty - line.picked_qty, 2147483647) }
  Object.assign(form, { sku_code: '', location_code: '', batch_no: '', slot_code: '', qty: Math.min(line.target_qty - line.picked_qty, 2147483647) }); dialog.value = true
}
function openSort(item: WaveItem) {
  const line = lineMap.value.get(item.line_id), member = orderMap.value.get(item.wave_order_id)
  if (!line || !member) return
  selected.value = { action: 'sort', id: item.id, title: `${line.sku_name} · 批次 ${line.batch_no || '无批次'} → ${member.slot_code}（${member.order_no}）`, max: item.target_qty - item.sorted_qty }
  Object.assign(form, { sku_code: '', location_code: '', batch_no: '', slot_code: '', qty: item.target_qty - item.sorted_qty }); dialog.value = true
}
async function submitQuantity() {
  if (!selected.value) return
  await perform({ action: selected.value.action, wave_id: waveID.value, part_id: selected.value.id, data: { ...form } })
}
async function ship(order: WaveOrder) {
  const id = waveID.value
  try { await ElMessageBox.confirm(`确认槽位 ${order.slot_code} 的 ${order.order_no} 已核对并实际发货？确认后将扣减库存并完成原拣货任务。`, '确认发货', { type: 'warning' }) } catch { return }
  if (alive && id === waveID.value) await perform({ action: 'ship', wave_id: id, part_id: order.id })
}
async function cancel() {
  const id = waveID.value
  try { await ElMessageBox.confirm('只能撤销尚未拣货的波次。撤销后订单恢复普通拣货，原订单和库存占用保持。', '撤销波次', { type: 'warning' }) } catch { return }
  if (alive && id === waveID.value) await perform({ action: 'cancel', wave_id: id })
}
async function recover() {
  const ack = await commands.recover()
  if (!ack || !alive) return
  if (ack.wave_id !== waveID.value) await router.push(`/outbound/waves/${ack.wave_id}`)
  else { dialog.value = false; await load() }
}
async function discard() {
  try { await ElMessageBox.confirm('先核对波次和订单结果。此操作仅清除本机待确认记录，不会撤销业务。', '放弃待确认记录', { type: 'warning' }) } catch { return }
  if (alive) commands.discard()
}
function printList() { window.print() }
</script>

<template>
  <div v-loading="loading" class="app-page wave-workbench">
    <div class="wave-heading no-print"><el-button @click="router.push('/outbound/waves')">返回波次</el-button><el-button @click="load">刷新</el-button><el-button :disabled="!data" @click="printList">打印拣货单</el-button></div>
    <WavePendingNotice v-if="!dialog" class="no-print" :pending="pending" :error="error" :busy="busy" @recover="recover" @discard="discard" />
    <el-alert v-if="loadError" class="no-print" type="error" :title="loadError" :closable="false" />
    <template v-if="data">
      <section class="app-card wave-identity">
        <div><h1>{{ data.wave.wave_no }}</h1><p>{{ data.wave.remark || '汇总拣货 → 按订单槽位分货 → 确认发货' }}</p></div>
        <div class="wave-heading no-print"><el-tag>{{ WAVE_STATUS_TEXT[data.wave.status] }}</el-tag><el-button v-if="!terminal" v-permission="'wms:outbound:pick'" :disabled="busy || !!pending || closureBlocked" type="primary" @click="perform({ action: 'claim', wave_id: waveID })">{{ owner ? '续领波次' : '领取波次' }}</el-button><el-button v-if="data.wave.status === 'CREATED'" v-permission="'wms:outbound:approve'" :disabled="busy || !!pending || closureBlocked" @click="cancel">撤销波次</el-button></div>
      </section>
      <WaveAbortPanel :wave="data.wave" :blocked="busy || !!pending" @changed="load" @pending-change="closureBlocked = $event" />
      <section class="wave-progress app-card">
        <div><small>汇总拣货</small><strong>{{ pickedLines }} / {{ data.lines.length }} 行</strong></div>
        <div><small>播种分货</small><strong>{{ sortedItems }} / {{ data.items.length }} 项</strong></div>
        <div><small>确认发货</small><strong>{{ shipped }} / {{ data.orders.length }} 单</strong></div>
        <div><small>作业员 · 租约到期</small><span>{{ data.wave.claimed_by || '尚未领取' }} · {{ formatTime(data.wave.lease_expire_at || '') || '—' }}</span></div>
      </section>
      <el-alert class="no-print" title="汇总拣货和分货记录现场进度，账面库存仍保持原库位占用；确认发货后才扣减。请按实际批次操作，领取后10分钟内有效，可续领。" type="info" :closable="false" />
      <section class="app-card wave-body">
        <h2 class="print-only">汇总拣货单（来源库位）</h2>
        <el-tabs v-model="tab" class="no-print">
          <el-tab-pane label="01 汇总拣货" name="collect" />
          <el-tab-pane label="02 播种分货" name="sort" />
          <el-tab-pane label="03 确认发货" name="ship" />
        </el-tabs>
        <div class="wave-scroll" :class="{ 'only-print': tab !== 'collect' }">
          <table class="wave-grid"><thead><tr><th>来源库位</th><th>货品</th><th>批次</th><th>已拣 / 应拣</th><th class="no-print">操作</th><th class="print-only">实拣签字</th></tr></thead><tbody><tr v-for="line in data.lines" :key="line.id"><td>{{ line.location_code }}</td><td><strong>{{ line.sku_name }}</strong><small>{{ line.sku_code }}</small></td><td>{{ line.batch_no || '—' }}</td><td>{{ line.picked_qty }} / {{ line.target_qty }}</td><td class="no-print"><el-button :disabled="!canWrite || !['CREATED', 'PICKING'].includes(data.wave.status) || line.picked_qty >= line.target_qty" type="primary" plain @click="openCollect(line)">拣货</el-button></td><td class="print-only">________</td></tr></tbody></table>
        </div>
        <div v-if="tab === 'sort'" class="wave-scroll no-print"><table class="wave-grid"><thead><tr><th>目标槽位 / 出库单</th><th>货品 / 批次</th><th>已分 / 应分</th><th>操作</th></tr></thead><tbody><tr v-for="item in sortRows" :key="item.id"><td><strong class="slot">{{ item.order?.slot_code }}</strong><small>{{ item.order?.order_no }}</small></td><td>{{ item.line?.sku_name }}<small>{{ item.line?.sku_code }} · {{ item.line?.batch_no || '无批次' }}</small></td><td>{{ item.sorted_qty }} / {{ item.target_qty }}</td><td><el-button :disabled="!canWrite || data.wave.status !== 'SORTING' || item.sorted_qty >= item.target_qty" type="primary" plain @click="openSort(item)">分货</el-button></td></tr></tbody></table></div>
        <div v-if="tab === 'ship'" class="wave-slots no-print"><article v-for="order in data.orders" :key="order.id"><strong class="slot">{{ order.slot_code }}</strong><el-link type="primary" @click="router.push(`/outbound/orders/${order.order_id}`)">{{ order.order_no }}</el-link><span>{{ order.shipped_at ? '已发货 · ' + formatTime(order.shipped_at) : orderSorted(order) ? '分货完成，待确认发货' : '等待分货' }}</span><el-button :disabled="!canWrite || !['SORTING', 'READY'].includes(data.wave.status) || !orderSorted(order) || !!order.shipped_at" type="primary" @click="ship(order)">确认发货</el-button></article></div>
      </section>
      <section class="print-only"><h2>订单槽位</h2><p v-for="order in data.orders" :key="order.id">{{ order.slot_code }} · {{ order.order_no }}</p><p>请核对来源库位、货品、批次；实物拣出后在工作台登记，分货完成再确认发货。本单不是发货凭证。</p></section>
    </template>
    <el-dialog v-model="dialog" :title="selected?.action === 'collect' ? '登记汇总拣货' : '登记播种分货'" class="business-dialog" width="520px" :close-on-click-modal="false" :close-on-press-escape="!busy">
      <WavePendingNotice :pending="pending" :error="error" :busy="busy" @recover="recover" @discard="discard" />
      <p>{{ selected?.title }}</p><p>支持扫码枪输入；无设备时手动核对填写。</p>
      <el-form label-position="top" @submit.prevent="submitQuantity">
        <el-form-item label="货品编码或条码"><el-input v-model="form.sku_code" autofocus maxlength="64" placeholder="扫描或输入货品编码 / 条码" /></el-form-item>
        <el-form-item v-if="selected?.action === 'collect'" label="来源库位"><el-input v-model="form.location_code" maxlength="64" placeholder="扫描或输入来源库位" /></el-form-item>
        <el-form-item label="批次（无批次留空）"><el-input v-model="form.batch_no" maxlength="64" /></el-form-item>
        <el-form-item v-if="selected?.action === 'sort'" label="目标槽位"><el-input v-model="form.slot_code" maxlength="16" placeholder="例如 S01" /></el-form-item>
        <el-form-item label="本次数量"><el-input-number v-model="form.qty" :min="1" :max="selected?.max || 1" :precision="0" /></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="busy" @click="dialog = false">返回</el-button><el-button type="primary" :loading="busy" :disabled="!canWrite" @click="submitQuantity">确认登记</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.wave-workbench { gap: 14px; min-height: 0; }
.wave-workbench > :not(.wave-body) { flex-shrink: 0; }
.wave-heading, .wave-identity { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; justify-content: space-between; }
h1 { font-size: 21px; margin: 0; overflow-wrap: anywhere; } h2 { font-size: 17px; }
p, small { color: var(--el-text-color-secondary); } p { margin: 8px 0; }
.wave-progress { display: grid; grid-template-columns: repeat(3, 1fr) 1.5fr; gap: 16px; }
.wave-progress div, .wave-slots article { display: flex; flex-direction: column; gap: 8px; }
.wave-progress strong { font-size: 21px; }
.wave-body { flex: 1; min-height: 180px; display: flex; flex-direction: column; overflow: hidden; }
.wave-scroll { overflow: auto; min-height: 0; flex: 1; }
.wave-grid { width: 100%; border-collapse: collapse; text-align: left; min-width: 640px; }
.wave-grid th { position: sticky; top: 0; background: var(--el-fill-color-light); z-index: 1; }
.wave-grid th, .wave-grid td { padding: 12px; border-bottom: 1px solid var(--el-border-color-lighter); }
.wave-grid small { display: block; margin-top: 5px; }
.wave-slots { display: grid; grid-template-columns: repeat(auto-fill, minmax(230px, 1fr)); gap: 14px; overflow: auto; }
.wave-slots article { border: 1px solid var(--el-border-color); border-radius: 8px; padding: 16px; }
.slot { color: var(--el-color-primary); font-size: 20px; }
.print-only, .only-print { display: none; }
@media (min-width: 981px) and (min-height: 620px) { .wave-workbench { height: 100%; } }
@media (max-width: 980px), (max-height: 619px) { .wave-body { flex: none; height: min(52dvh, 520px); } }
@media (max-width: 767px) { .wave-progress { grid-template-columns: 1fr 1fr; } .wave-body { min-height: 240px; } .wave-heading { justify-content: flex-start; } }
@media print {
  .no-print { display: none !important; }
  .only-print, .print-only { display: block; }
  .wave-body, .wave-scroll { height: auto; min-height: 0; overflow: visible; }
  .wave-grid { min-width: 0; table-layout: fixed; font-size: 11px; }
  .wave-grid th, .wave-grid td { padding: 7px; overflow-wrap: anywhere; }
  .wave-grid .print-only { display: table-cell; }
  .wave-grid th { position: static; background: #f2f3f5; color: #111; }
  .wave-grid thead { display: table-header-group; }
  .wave-grid tr { break-inside: avoid; }
}
</style>

<style>
@page { size: A4; margin: 12mm; }
@media print {
  html:has(.wave-workbench), body:has(.wave-workbench) {
    color-scheme: light;
    --el-bg-color: #fff;
    --el-bg-color-page: #fff;
    --el-text-color-primary: #111;
    --el-text-color-regular: #333;
    --el-text-color-secondary: #555;
    --el-fill-color-light: #f2f3f5;
    --el-border-color-lighter: #ddd;
    transition: none !important;
    color: #111 !important; background: white !important;
  }
  body:has(.wave-workbench) .gowms-aside, body:has(.wave-workbench) .el-header, body:has(.wave-workbench) .demo-statusbar, body:has(.wave-workbench) .el-overlay { display: none !important; }
  body:has(.wave-workbench) #app, body:has(.wave-workbench) .layout, body:has(.wave-workbench) .el-main, body:has(.wave-workbench) .el-container, body:has(.wave-workbench) .app-page, body:has(.wave-workbench) .page-stage { display: block !important; height: auto !important; min-height: 0 !important; overflow: visible !important; padding: 0 !important; max-width: none !important; }
  body:has(.wave-workbench) .app-card { color: #111; background: white; box-shadow: none; border: 0; border-radius: 0; margin-bottom: 12px; }
}
</style>
