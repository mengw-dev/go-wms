<script setup lang="ts">
import { computed, nextTick, onScopeDispose, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { encodeLabelBarcode, MAX_LABELS, type BarcodeLabel } from '@/utils/barcodeLabels'

const auth = useAuthStore()
const visible = ref(false)
const copies = ref(1)
const printing = ref(false)
const items = ref<BarcodeLabel[]>([])
let owner = ''
let sequence = 0
const identity = () => JSON.stringify([auth.sessionVersion, auth.user?.user_id, auth.demoSessionId])

watch(identity, () => {
  sequence++
  visible.value = false
  items.value = []
})

onScopeDispose(() => {
  sequence++
  visible.value = false
})

function open(rows: BarcodeLabel[]) {
  if (!auth.hasPerm('wms:basic')) return
  sequence++
  owner = identity()
  items.value = rows.map(row => ({ ...row }))
  copies.value = 1
  printing.value = false
  visible.value = true
}

const prepared = computed(() => items.value.map(item => {
  try {
    return { item, barcode: encodeLabelBarcode(item.value), error: '' }
  } catch (error) {
    return { item, barcode: null, error: error instanceof Error ? error.message : '条码编码失败' }
  }
}))
const total = computed(() => items.value.length * copies.value)
const canPrint = computed(() => visible.value && owner === identity() && auth.hasPerm('wms:basic')
  && total.value > 0 && total.value <= MAX_LABELS && prepared.value.every(row => !!row.barcode))
const sheets = computed(() => total.value > MAX_LABELS ? []
  : prepared.value.flatMap(row => Array.from({ length: copies.value }, (_, copy) => ({
    ...row, key: `${row.item.key}:${copy}`,
  }))))

// A4每页固定五行两列，显式分页，避免浏览器把一个标签拆到两页。
const pages = computed(() => {
  const result = []
  for (let i = 0; i < sheets.value.length; i += 10) result.push(sheets.value.slice(i, i + 10))
  return result
})

async function print() {
  if (!canPrint.value || printing.value) return
  const current = sequence
  printing.value = true
  try {
    await nextTick()
    await document.fonts.ready
    if (current === sequence && canPrint.value) window.print()
  } finally {
    if (current === sequence) printing.value = false
  }
}

defineExpose({open})
</script>

<template>
  <el-dialog v-model="visible" append-to-body title="条码标签预览" class="label-preview business-dialog" :class="{ 'label-print-active': visible }" width="1040px" :close-on-click-modal="false" destroy-on-close>
    <div class="label-controls"><span>A4 标签样板 · 单张90×45mm · 货品打印原条码值，库位打印原库位编码</span><label>每条记录 <el-input-number v-model="copies" :min="1" :max="20" :precision="0" /> 张</label></div>
    <p class="label-controls">来自本次列表查询快照；资料变更后请刷新列表重印。请选择实际纸张并按100%比例打印，实机扫描结果需现场核对。</p>
    <el-alert v-if="total>MAX_LABELS" class="label-controls" :title="`单次最多${MAX_LABELS}张，请减少记录或份数`" type="warning" :closable="false" />
    <el-alert v-for="row in prepared.filter(r=>r.error)" :key="row.item.key" class="label-controls" :title="`${row.item.title}：${row.error}`" type="error" :closable="false" />
    <div class="label-sheets">
      <section v-for="(page,index) in pages" :key="index" class="label-sheet">
      <article v-for="row in page" :key="row.key" class="barcode-label">
        <small>{{ row.item.kind==='sku' ? '货品标签' : '库位标签' }}</small><strong>{{ row.item.title }}</strong><div class="label-context">{{ row.item.subtitle }}</div>
        <svg v-if="row.barcode" :viewBox="`0 0 ${row.barcode.modules} 40`" :style="{width:`${row.barcode.widthMM}mm`}" height="14mm" preserveAspectRatio="none" role="img" :aria-label="`条码 ${row.item.value}`" shape-rendering="crispEdges">
          <rect x="0" y="0" :width="row.barcode.modules" height="40" fill="white" /><rect v-for="bar in row.barcode.bars" :key="bar.x" :x="bar.x" y="0" :width="bar.width" height="40" fill="black" />
        </svg>
        <div v-else class="label-invalid">当前编码无法生成此规格条码</div>
        <code>{{ row.item.value }}</code>
      </article>
      </section>
    </div>
    <template #footer><span>本次 {{ total }} 张，不记录“已打印”业务状态</span><el-button @click="visible=false">返回</el-button><el-button type="primary" :disabled="!canPrint" :loading="printing" @click="print">打印 / 保存 PDF</el-button></template>
  </el-dialog>
</template>

<style scoped>
.label-controls { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 12px; font-size: 12px; color: var(--el-text-color-secondary); }.label-controls label { display: flex; align-items: center; gap: 8px; }
.label-sheets { display: block; }.label-sheet + .label-sheet { margin-top: 16px; }
.label-sheet { display: grid; grid-template-columns: repeat(2,90mm); gap: 4mm; width: max-content; margin: 0 auto; page: labels; }
.barcode-label { box-sizing: border-box; width: 90mm; height: 45mm; border: 1px dashed #aaa; background: #fff; color: #111; padding: 3mm; display: flex; align-items: center; flex-direction: column; justify-content: space-between; gap: 1mm; break-inside: avoid; overflow: hidden; }
.barcode-label small { align-self: flex-start; font-size: 9px; color: #555; }.barcode-label strong { font-size: 13px; width: 100%; text-align: center; white-space: nowrap; text-overflow: ellipsis; overflow: hidden; }.label-context { font-size: 10px; color: #555; width: 100%; text-align: center; white-space: nowrap; text-overflow: ellipsis; overflow: hidden; }.barcode-label code { font-size: 11px; overflow-wrap: anywhere; text-align: center; line-height: 1.1; }.barcode-label svg { flex: none; max-width: none; }.label-invalid { font-size: 11px; color: #b91c1c; }
@media print { .label-controls { display: none !important; }.label-sheet { margin: 0; gap: 4mm; break-inside: avoid; break-after: page; }.label-sheet:last-child { break-after: auto; }.barcode-label { border: 0; } }
</style>

<style>
@page labels { size: A4; margin: 10mm; }
.label-preview .el-dialog__body { overflow: auto; }
@media print {
  body:has(.label-print-active) > :not(.el-overlay:has(.label-print-active)) { display: none !important; }
  body:has(.label-print-active), .el-overlay:has(.label-print-active), .el-overlay-dialog:has(.label-print-active) { position: static !important; overflow: visible !important; height: auto !important; background: white !important; margin: 0 !important; }
  .el-dialog.label-print-active { display: block !important; position: static !important; width: auto !important; max-width: none !important; max-height: none !important; margin: 0 !important; padding: 0 !important; border: 0 !important; box-shadow: none !important; background: white !important; }
  .label-print-active .el-dialog__header, .label-print-active .el-dialog__footer { display: none !important; }
  .label-print-active .el-dialog__body { flex: none !important; height: auto !important; max-height: none !important; overflow: visible !important; padding: 0 !important; }
}
</style>
