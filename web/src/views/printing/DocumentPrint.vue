<script setup lang="ts">
import { nextTick, onScopeDispose, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getInboundOrder } from '@/api/inbound'
import { getOutboundOrder } from '@/api/outbound'
import { getStocktakeOrder } from '@/api/stocktake'
import { inboundPrint, outboundPrint, stocktakePrint, type PrintDocument } from '@/utils/documentPrint'
import { loadWarehouseOptions } from '@/utils/options'
import { statusText } from '@/constants'
import { formatTime } from '@/utils'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ kind: 'inbound' | 'outbound' | 'stocktake' }>()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const data = ref<PrintDocument | null>(null)
const loading = ref(false)
const error = ref('')
const printing = ref(false)
const warehouse = ref('')
const generatedAt = ref('')
let sequence = 0
let alive = true
const identity = () => JSON.stringify([
  props.kind, route.params.id, auth.sessionVersion, auth.user?.user_id, auth.demoSessionId,
])

async function load() {
  if (!alive) return
  const current = ++sequence
  const owner = identity()
  const id = String(route.params.id)
  data.value = null
  warehouse.value = ''
  error.value = ''
  loading.value = true
  printing.value = false
  try {
    const result = props.kind === 'inbound' ? inboundPrint(await getInboundOrder(id))
      : props.kind === 'outbound' ? outboundPrint(await getOutboundOrder(id))
        : stocktakePrint(await getStocktakeOrder(id))
    if (!alive || current !== sequence || identity() !== owner) return
    data.value = result
    generatedAt.value = new Date().toISOString()
    if (auth.hasPerm('wms:basic')) {
      try {
        const options = await loadWarehouseOptions()
        if (alive && current === sequence && owner === identity()) {
          warehouse.value = options.find(option => option.id === result.warehouseID)?.label || ''
        }
      } catch {
        // 仓库名称仅是辅助信息，失败时仍可打印单据中的仓库 ID。
      }
    }
  } catch {
    if (alive && current === sequence) {
      error.value = '单据加载失败或无权访问，请重试；不能打印上一次的内容。'
    }
  } finally {
    if (alive && current === sequence) loading.value = false
  }
}

watch(identity, () => { void load() }, { immediate: true })
onScopeDispose(() => { alive = false; sequence++ })

async function print() {
  if (!data.value || loading.value || error.value || printing.value) return
  const owner = identity()
  const current = sequence
  printing.value = true
  try {
    await nextTick()
    await document.fonts.ready
    if (alive && owner === identity() && current === sequence && data.value && !error.value) {
      window.print()
    }
  } finally {
    if (alive && owner === identity() && current === sequence) printing.value = false
  }
}

</script>

<template>
  <div class="document-preview">
    <header class="print-controls">
      <div><h1>单据打印预览</h1><span>A4 · 按已保存数据生成 · 打印不改变业务状态</span></div>
      <div><el-button @click="router.push(`/${kind}/orders/${route.params.id}`)">返回单据</el-button><el-button :disabled="loading || printing" @click="load">刷新数据</el-button><el-button type="primary" :disabled="!data || loading || !!error" :loading="printing" @click="print">打印 / 保存 PDF</el-button></div>
    </header>
    <el-alert v-if="error" class="print-error" :title="error" type="error" :closable="false" />
    <main v-loading="loading" class="print-scroll">
      <article v-if="data" class="print-paper">
        <div class="paper-brand">WMS · 仓储管理系统</div><h2>{{ data.title }}</h2>
        <dl><div><dt>单号</dt><dd>{{ data.orderNo }}</dd></div><div><dt>当前状态</dt><dd>{{ statusText(data.status) }}</dd></div><div><dt>{{ warehouse ? '仓库' : '仓库 ID' }}</dt><dd>{{ warehouse || data.warehouseID }}</dd></div><div><dt>创建时间</dt><dd>{{ formatTime(data.createdAt) }}</dd></div><div><dt>备注</dt><dd>{{ data.remark || '—' }}</dd></div></dl>
        <section v-for="section in data.sections" :key="section.title"><h3>{{ section.title }} · {{ section.rows.length }} 行</h3><table><thead><tr><th v-for="column in section.columns" :key="column">{{ column }}</th></tr></thead><tbody><tr v-for="(row,index) in section.rows" :key="index"><td v-for="(value,column) in row" :key="column">{{ value === null ? '________' : value }}</td></tr></tbody></table><p v-if="!section.rows.length">暂无明细</p></section>
        <div class="paper-notes"><p v-for="note in data.notes" :key="note">{{ note }}</p><p>预览生成于 {{ formatTime(generatedAt) }}，数据可能在此后变化，现场作业请再次核对。</p></div>
        <div class="signatures"><span>操作人：_______________</span><span>核对人：_______________</span><span>日期：_______________</span></div>
      </article>
      <div v-else-if="!loading" class="preview-empty">没有可打印的单据数据</div>
    </main>
  </div>
</template>

<style scoped>
.document-preview { height: 100dvh; display: flex; flex-direction: column; background: var(--el-bg-color-page); color: var(--el-text-color-primary); }
.print-controls { padding: 16px 24px; display: flex; gap: 12px; justify-content: space-between; flex-wrap: wrap; align-items: center; flex: none; border-bottom: 1px solid var(--el-border-color); background: var(--el-bg-color); }
.print-controls h1 { margin: 0 0 4px; font-size: 19px; }.print-controls span { font-size: 12px; color: var(--el-text-color-secondary); }
.print-scroll { flex: 1; min-height: 0; overflow: auto; padding: 20px; }.print-error { flex: none; }.preview-empty { text-align: center; padding: 60px; }
.print-paper { box-sizing: border-box; width: 210mm; min-height: 297mm; padding: 16mm 14mm; margin: 0 auto; background: #fff; color: #111; box-shadow: 0 4px 22px #0001; font: 12px/1.6 Arial,"Microsoft YaHei",sans-serif; }
.paper-brand { color: #555; font-size: 11px; letter-spacing: 1px; } h2 { text-align: center; font-size: 23px; margin: 10px 0 18px; } h3 { font-size: 13px; margin: 18px 0 8px; }
dl { display: grid; grid-template-columns: 1.5fr 1fr; gap: 8px 20px; margin: 0; } dl div { min-width: 0; display: flex; gap: 10px; }dt { color: #555; flex: none; }dd { margin: 0; overflow-wrap: anywhere; }
table { border-collapse: collapse; width: 100%; table-layout: fixed; }th,td { border: 1px solid #bbb; padding: 7px; text-align: left; vertical-align: top; overflow-wrap: anywhere; }th { background: #f2f3f5; font-weight: 600; }tr { break-inside: avoid; }
.paper-notes { margin-top: 20px; font-size: 11px; color: #555; }.paper-notes p { margin: 4px 0; }.signatures { margin-top: 26px; break-inside: avoid; display: flex; flex-wrap: wrap; gap: 16px; justify-content: space-between; }
@media (max-width: 767px) { .print-controls { padding: 12px; }.print-controls > div:last-child { display: flex; gap: 6px; flex-wrap: wrap; }.print-controls .el-button { margin: 0; }.print-scroll { padding: 10px; } }
@media print { .document-preview { height: auto; display: block; background: white; }.print-controls,.print-error { display: none; }.print-scroll { display: block; overflow: visible; padding: 0; }.print-paper { width: auto; min-height: 0; margin: 0; padding: 0; box-shadow: none; }thead { display: table-header-group; } }
</style>

<style>
@page { size: A4; margin: 12mm; }
@media print { body:has(.document-preview) { background: white; margin: 0; } body:has(.document-preview) #app { height: auto; overflow: visible; } }
</style>
