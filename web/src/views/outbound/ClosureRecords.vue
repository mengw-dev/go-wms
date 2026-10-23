<script setup lang="ts">
import { onScopeDispose, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listExceptions, listReturns } from '@/api/closure'
import PageHeader from '@/components/common/PageHeader.vue'
import BusinessFilters from '@/components/common/BusinessFilters.vue'
import BusinessPagination from '@/components/common/BusinessPagination.vue'
import ResizableTableArea from '@/components/common/ResizableTableArea.vue'
import { formatTime } from '@/utils'
import { statusText, statusTag } from '@/constants'
import { mobileTableCellStyle } from '@/utils/mobileTable'
const props = defineProps<{ kind: 'exceptions' | 'returns' }>()
const route = useRoute(), router = useRouter()
const query = reactive({ keyword: '', order_id: typeof route.query.order_id === 'string' ? route.query.order_id : '', page: 1, page_size: 20 })
interface Row { id: string; order_id: string; order_no: string; receipt: string; status: string; quantity: string; reason: string; resolution: string; operator: string; created_at: string; inventory_id?: string; ledger_no?: string; product?: string; location?: string }
const rows = ref<Row[]>([]), total = ref(0), loading = ref(false), error = ref('')
let seq = 0, alive = true
onScopeDispose(() => { alive = false; seq++ })
async function load() {
  const current = ++seq, kind = props.kind
  loading.value = true; error.value = ''
  try {
    const params = { ...query, order_id: query.order_id || undefined }
    if (kind === 'exceptions') {
      const page = await listExceptions(params)
      if (!alive || seq !== current) return
      rows.value = page.list.map(r => ({ id: r.id, order_id: r.order_id, order_no: r.order_no, receipt: r.kind === 'DAMAGE' ? '货损冻结' : '实物短缺', status: r.status, quantity: String(r.quantity), reason: r.reason, resolution: r.resolution, operator: `${r.operator}${r.resolved_by ? ` → ${r.resolved_by}` : ''}`, created_at: r.created_at, inventory_id: r.inventory_id })); total.value = page.total
    } else {
      const page = await listReturns(params)
      if (!alive || seq !== current) return
      rows.value = page.list.map(r => ({ id: r.id, order_id: r.order_id, order_no: r.order_no, receipt: `${r.return_no} · 批次 ${r.batch_no || '—'}`, status: 'COMPLETED', product: `${r.sku_code} ${r.sku_name}`, location: r.location_code, ledger_no: r.return_no, quantity: `合格 ${r.good_qty} / 不良 ${r.bad_qty}`, reason: r.reason, resolution: '', operator: r.operator, created_at: r.created_at })); total.value = page.total
    }
  } catch { if (alive && seq === current) error.value = '记录加载失败，请重试' }
  finally { if (alive && seq === current) loading.value = false }
}
function search() { query.page = 1; void load() }
watch(() => [props.kind, route.query.order_id], () => { query.order_id = typeof route.query.order_id === 'string' ? route.query.order_id : ''; query.page = 1; void load() }, { immediate: true })
</script>
<template>
  <div class="app-page">
    <PageHeader :title="kind === 'exceptions' ? '出库异常记录' : '客户退货记录'" :description="kind === 'exceptions' ? '追踪短缺、货损与后续处理，保留原单需求和实际发货差额。' : '按原出库批次追溯退货验收；不良品已转入冻结库存。'"><template #actions><el-button @click="router.push('/outbound/orders')">前往出库单处理</el-button></template></PageHeader>
    <section class="page-card">
      <BusinessFilters><el-form inline class="query-form" @submit.prevent="search"><el-form-item label="单号"><el-input v-model="query.keyword" clearable placeholder="出库单号 / 退货单号" @keyup.enter="search" /></el-form-item><el-form-item><el-button type="primary" plain @click="search">查询</el-button><el-button @click="query.order_id = ''; query.keyword = ''; search()">重置</el-button></el-form-item></el-form></BusinessFilters>
      <el-alert v-if="error" :title="error" type="error" :closable="false" />
      <ResizableTableArea :key="kind" :storage-key="`outbound-${kind}`">
        <el-table v-loading="loading" :data="rows" class="mobile-record-table" :cell-style="mobileTableCellStyle" max-height="var(--business-table-height)" stripe>
          <el-table-column label="来源出库单" min-width="185"><template #default="{ row }"><el-button type="primary" link @click="router.push(`/outbound/orders/${row.order_id}`)">{{ row.order_no }}</el-button></template></el-table-column>
          <el-table-column v-if="kind === 'returns'" prop="product" label="货品" min-width="200" /><el-table-column v-if="kind === 'returns'" prop="location" label="验收库位" min-width="125" />
          <el-table-column prop="receipt" :label="kind === 'exceptions' ? '异常类型' : '退货单 / 批次'" min-width="230" />
          <el-table-column label="状态" width="110"><template #default="{ row }"><el-tag :type="statusTag(row.status)">{{ statusText(row.status) }}</el-tag></template></el-table-column>
          <el-table-column prop="quantity" label="数量" min-width="140" />
          <el-table-column prop="reason" label="原因" min-width="200" />
          <el-table-column v-if="kind === 'exceptions'" prop="resolution" label="处理结果" min-width="200" />
          <el-table-column prop="operator" label="操作员" min-width="140" />
          <el-table-column label="登记时间" width="175"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
          <el-table-column label="追溯" width="100"><template #default="{ row }"><el-button type="primary" link @click="router.push({ path: '/inventory', query: row.inventory_id ? { inventory_id: row.inventory_id } : { order_no: row.ledger_no } })">库存流水</el-button></template></el-table-column>
        </el-table>
      </ResizableTableArea>
      <BusinessPagination class="pagination" v-model:current-page="query.page" v-model:page-size="query.page_size" :total="total" @current-change="load" @size-change="search" />
    </section>
  </div>
</template>
