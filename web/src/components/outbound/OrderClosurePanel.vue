<script setup lang="ts">
import { computed, onScopeDispose, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getReturnable, getReturnLocations, listExceptions, type ClosureCommand, type ReturnableRow, type WorkException } from '@/api/closure'
import type { OutboundOrderDetail } from '@/api/types'
import { loadAllPages } from '@/utils/pagination'
import { formatTime } from '@/utils'
import { useClosureCommands } from '@/composables/useClosureCommands'
import ClosurePendingNotice from '@/components/common/ClosurePendingNotice.vue'

const props = defineProps<{ data: OutboundOrderDetail; blocked?: boolean }>()
const emit = defineEmits<{ changed: []; pendingChange: [blocked: boolean] }>()
const router = useRouter(), commands = useClosureCommands()
const { pending, busy, error } = commands
const dialog = ref(false), mode = ref<'exception' | 'resolve' | 'return'>('exception'), records = ref<WorkException[]>([])
const loading = ref(false), loadError = ref(''), locations = ref<{ id: string; code: string }[]>([]), returnable = ref<ReturnableRow[]>([])
const form = reactive({ task_id: '', kind: 'SHORTAGE' as 'SHORTAGE' | 'DAMAGE', quantity: 1, reason: '', confirm_returned: false, exception_id: '', close: false, allocation_id: '', location_id: '', good_qty: 1, bad_qty: 0 })
let seq = 0, alive = true
onScopeDispose(() => { alive = false; seq++ })
const tasks = computed(() => props.data.tasks.filter(t => t.task_type === 'PICK' && ['CREATED', 'IN_PROGRESS'].includes(t.status) && (!t.wave_id || t.wave_id === '0')))
const selectedTask = computed(() => tasks.value.find(t => t.id === form.task_id))
const maxException = computed(() => selectedTask.value ? selectedTask.value.target_qty - selectedTask.value.done_qty : 0)
const maxReturn = computed(() => returnable.value.find(r => r.allocation_id === form.allocation_id)?.returnable_qty ?? 0)
const openException = computed(() => records.value.find(r => r.status === 'OPEN'))
const disabled = computed(() => busy.value || !!pending.value || !!props.blocked || loading.value)
watch(() => !!pending.value || busy.value, value => emit('pendingChange', value), { immediate: true })
watch(() => [props.data.order.id, props.data.order.status], async () => {
  const current = ++seq, id = props.data.order.id
  records.value = []; dialog.value = false; loading.value = false
  try { const data = await listExceptions({ order_id: id, page: 1, page_size: 100 }); if (alive && seq === current) { records.value = data.list; loadError.value = '' } }
  catch { if (alive && seq === current) loadError.value = '异常记录加载失败，请刷新后再处理' }
}, { immediate: true })

async function open(next: typeof mode.value) {
  mode.value = next
  Object.assign(form, { task_id: tasks.value[0]?.id || '', kind: 'SHORTAGE', quantity: 1, reason: '', confirm_returned: false, exception_id: openException.value?.id || '', close: false, allocation_id: '', location_id: '', good_qty: 1, bad_qty: 0 })
  dialog.value = true; loadError.value = ''
  if (next !== 'return') return
  const current = ++seq, id = props.data.order.id
  loading.value = true
  try {
    const [rows, bins] = await Promise.all([getReturnable(id), loadAllPages((page, page_size) => getReturnLocations(id, page, page_size))])
    if (!alive || seq !== current || id !== props.data.order.id) return
    returnable.value = rows; locations.value = bins; form.allocation_id = rows.find(r => r.returnable_qty > 0)?.allocation_id || ''
  } catch { if (alive && seq === current) loadError.value = '可退批次或库位加载失败，请关闭后重试' }
  finally { if (alive && seq === current) loading.value = false }
}
const valid = computed(() => {
  if (disabled.value || !form.reason.trim() || loadError.value) return false
  if (mode.value === 'exception') return !!form.task_id && form.confirm_returned && form.quantity > 0 && form.quantity <= maxException.value
  if (mode.value === 'resolve') return !!form.exception_id
  return !!form.allocation_id && !!form.location_id && form.good_qty >= 0 && form.bad_qty >= 0 && form.good_qty + form.bad_qty > 0 && form.good_qty + form.bad_qty <= maxReturn.value
})
async function submit() {
  if (!valid.value) return
  const order_id = props.data.order.id, reason = form.reason.trim()
  let command: ClosureCommand
  if (mode.value === 'exception') command = { action: 'exception', order_id, data: { task_id: form.task_id, kind: form.kind, quantity: form.quantity, reason, confirm_returned: form.confirm_returned } }
  else if (mode.value === 'resolve') command = { action: 'resolve', order_id, exception_id: form.exception_id, data: { close: form.close, reason } }
  else command = { action: 'return', order_id, data: { allocation_id: form.allocation_id, location_id: form.location_id, good_qty: form.good_qty, bad_qty: form.bad_qty, reason } }
  if (await commands.run({ ...command, label: props.data.order.order_no })) { dialog.value = false; ElMessage.success('处理成功'); emit('changed') }
}
async function recover() { if (await commands.recover()) { dialog.value = false; emit('changed') } }
</script>

<template>
  <section class="app-card closure-panel">
    <ClosurePendingNotice v-if="!dialog" :pending="pending" :busy="busy" :error="error" @recover="recover" @discard="commands.discard" />
    <div class="closure-toolbar">
      <div><strong>异常与售后</strong><span>按真实库存与原单处理，保留完整记录</span></div>
      <div class="closure-buttons">
        <el-button v-if="data.order.status === 'PICKING' && tasks.length" v-permission="'wms:outbound:exception'" :disabled="disabled" @click="open('exception')">登记短缺 / 货损</el-button>
        <el-button v-if="data.order.status === 'EXCEPTION'" v-permission="'wms:outbound:exception'" type="primary" :disabled="disabled || !openException || !!loadError" @click="open('resolve')">处理异常</el-button>
        <el-button v-if="['SHIPPED', 'CLOSED'].includes(data.order.status) && data.order.picked_qty > 0" v-permission="'wms:outbound:return'" type="primary" plain :disabled="disabled" @click="open('return')">验收客户退货</el-button>
        <el-button text @click="router.push({ path: '/outbound/exceptions', query: { order_id: data.order.id } })">异常记录 {{ records.length || '' }}</el-button>
        <el-button text @click="router.push({ path: '/outbound/returns', query: { order_id: data.order.id } })">退货记录</el-button>
      </div>
    </div>
    <el-alert v-if="data.order.status === 'EXCEPTION'" type="warning" :closable="false" :title="openException ? `${openException.kind === 'DAMAGE' ? '货损' : '短缺'} ${openException.quantity}：${openException.reason}。可补货后重新分配，或关闭剩余作业。` : '该单等待异常处理，请核对异常记录。'" />
    <el-alert v-if="loadError" type="error" :title="loadError" :closable="false" />
    <el-dialog v-model="dialog" class="business-dialog" :title="mode === 'exception' ? '登记出库异常' : mode === 'resolve' ? '处理出库异常' : '客户退货验收'" width="580px" :close-on-click-modal="!busy">
      <ClosurePendingNotice :pending="pending" :busy="busy" :error="error" @recover="recover" @discard="commands.discard" />
      <div v-loading="loading">
        <p>{{ data.order.order_no }} · {{ formatTime(data.order.created_at) }}</p>
        <el-form label-position="top" @submit.prevent="submit">
          <template v-if="mode === 'exception'">
            <el-alert title="将停止本单全部未发任务。未拣满分配行的实物须退回原位，进度将保留为历史记录；已拣满发货部分保持。短缺扣减实物账，货损转为冻结。" type="warning" :closable="false" />
            <el-form-item label="异常任务"><el-select v-model="form.task_id" style="width: 100%" :disabled="disabled"><el-option v-for="t in tasks" :key="t.id" :value="t.id" :label="`${t.task_no} · ${t.location_code} · ${t.batch_no}（未拣 ${t.target_qty - t.done_qty}）`" /></el-select></el-form-item>
            <el-form-item label="异常类型"><el-radio-group v-model="form.kind" :disabled="disabled"><el-radio-button value="SHORTAGE">实物短缺</el-radio-button><el-radio-button value="DAMAGE">货损冻结</el-radio-button></el-radio-group></el-form-item>
            <el-form-item :label="`异常数量（最多 ${maxException}）`"><el-input-number v-model="form.quantity" :min="1" :max="Math.max(1, maxException)" :precision="0" :disabled="disabled" /></el-form-item>
            <el-form-item><el-checkbox v-model="form.confirm_returned" :disabled="disabled">已确认本单所有未发实物退回原库位，并与现场作业员核对</el-checkbox></el-form-item>
          </template>
          <template v-else-if="mode === 'resolve'">
            <el-form-item label="处理方式"><el-radio-group v-model="form.close" :disabled="disabled"><el-radio-button :value="false">重新分配未发数量</el-radio-button><el-radio-button :value="true">关闭剩余作业</el-radio-button></el-radio-group></el-form-item>
            <el-alert :title="form.close ? '保留原需求与已发量，未发差额不再执行，单据标记异常关闭。' : '只分配尚未发货数量。库存不足时整笔回滚，异常保持待处理，可补货后重试。'" type="info" :closable="false" />
          </template>
          <template v-else>
            <el-alert title="按原出库批次验收，累计不可超过实发量。合格品可用，不良品冻结。如需分库位隔离，请分两次验收；不涉及退款。" type="info" :closable="false" />
            <el-form-item label="原单已发批次"><el-select v-model="form.allocation_id" style="width: 100%" :disabled="disabled"><el-option v-for="r in returnable" :key="r.allocation_id" :value="r.allocation_id" :disabled="r.returnable_qty <= 0" :label="`${r.sku_code} ${r.sku_name} · 批次 ${r.batch_no || '—'} · 可退 ${r.returnable_qty} / 实发 ${r.shipped_qty}`" /></el-select></el-form-item>
            <el-form-item label="验收入库库位（原仓库）"><el-select v-model="form.location_id" filterable style="width: 100%" :disabled="disabled"><el-option v-for="l in locations" :key="l.id" :value="l.id" :label="l.code" /></el-select></el-form-item>
            <div v-if="!loading && maxReturn > 0" :key="form.allocation_id" class="return-quantities"><el-form-item label="合格数量"><el-input-number v-model="form.good_qty" :min="0" :max="maxReturn" :precision="0" :disabled="disabled" /></el-form-item><el-form-item label="不良数量（冻结）"><el-input-number v-model="form.bad_qty" :min="0" :max="maxReturn" :precision="0" :disabled="disabled" /></el-form-item></div>
            <p>本次合计 {{ form.good_qty + form.bad_qty }} · 最多可退 {{ maxReturn }}</p>
          </template>
          <el-form-item label="处理原因"><el-input v-model="form.reason" type="textarea" maxlength="255" show-word-limit :disabled="disabled" /></el-form-item>
        </el-form>
        <el-alert v-if="loadError" :title="loadError" type="warning" :closable="false" />
      </div>
      <template #footer><el-button :disabled="busy" @click="dialog = false">返回</el-button><el-button type="primary" :loading="busy" :disabled="!valid" @click="submit">确认处理</el-button></template>
    </el-dialog>
  </section>
</template>

<style scoped>
.closure-panel { padding: 16px 20px; margin-bottom: 16px; }
.closure-toolbar, .closure-buttons { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.closure-toolbar { justify-content: space-between; }
.closure-toolbar span { margin-left: 12px; color: var(--el-text-color-secondary); font-size: 12px; }
.closure-panel :deep(.el-alert) { margin-top: 10px; }
.return-quantities { display: flex; gap: 20px; flex-wrap: wrap; }
@media (max-width: 767px) { .closure-panel { padding: 12px; } .closure-toolbar span { display: block; margin: 6px 0; } .closure-buttons { width: 100%; } .closure-panel :deep(.el-checkbox) { height: auto; white-space: normal; } .closure-panel :deep(.el-checkbox__label) { white-space: normal; line-height: 1.6; } }
</style>
