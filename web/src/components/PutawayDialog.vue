<script setup lang="ts">
import { computed, onScopeDispose, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getInboundOrder } from '@/api/inbound'
import type { EntityID, TaskItem } from '@/api/types'
import { usePutaway } from '@/composables/inbound/usePutaway'
import type { IdOption } from '@/utils/options'
import ScanField from '@/components/common/ScanField.vue'
import { putawayLocations } from '@/api/scanning'
import { useSkuScan } from '@/composables/useSkuScan'
import { operationErrorMessage, operationStateKey } from '@/composables/operationLifecycle'
import { scanCode } from '@/utils/scanCode'

const emit = defineEmits<{ (e: 'success'): void }>()

const visible = ref(false)
const loading = ref(false)
const orderId = ref<EntityID>('')
const orderNo = ref('')
const tasks = ref<TaskItem[]>([])
const locationOptions = ref<IdOption[]>([])

const puter = usePutaway()

const form = reactive({
  task_id: undefined as EntityID | undefined,
  location_id: undefined as EntityID | undefined,
  qty: 1,
})

const selectedTask = computed(() => tasks.value.find((t) => t.id === form.task_id))

const pendingTasks = computed(() =>
  tasks.value.filter((t) => t.task_type === 'PUTAWAY' && (t.status === 'CREATED' || t.status === 'IN_PROGRESS' || puter.stateOf(t.id).pending)),
)

// 提交中与待确认横幅按当前任务读取：切换任务即切换展示。
const current = computed(() => (form.task_id ? puter.stateOf(form.task_id) : null))
const submitting = computed(() => current.value?.submitting ?? false)
const pendingPutaway = computed(() => current.value?.pending ?? null)
const lastError = computed(() => current.value?.lastError ?? '')
const storageError = computed(() => puter.storageError.value)

const skuScan = useSkuScan('inbound', () => orderId.value, () => `${visible.value}:${form.task_id}`, () => selectedTask.value?.sku_id || '')
const locationScan = ref(''), locationError = ref(''), locationLoading = ref(false), locationTotal = ref(0)
let loadSeq = 0, locationSeq = 0, alive = true
onScopeDispose(() => { alive = false; loadSeq++; locationSeq++ })
watch(visible, value => { if (!value) { loadSeq++; locationSeq++; skuScan.reset() } })
watch(() => operationStateKey('putaway-view'), () => { loadSeq++; locationSeq++; visible.value = false; tasks.value = []; locationOptions.value = [] })
watch(locationScan, () => { locationSeq++; form.location_id = undefined; locationError.value = ''; locationLoading.value = false }, { flush: 'sync' })
async function loadLocations(keyword = '') {
  if (!visible.value || !orderId.value || pendingPutaway.value) return
  const seq = ++locationSeq, view = loadSeq, identity = operationStateKey('putaway-view')
  locationLoading.value = true; locationError.value = ''
  try {
    const data = await putawayLocations(orderId.value, { keyword })
    if (!alive || seq !== locationSeq || view !== loadSeq || !visible.value || identity !== operationStateKey('putaway-view')) return
    locationOptions.value = data.list.map(l => ({ id: l.id, label: `${l.code}（${l.status === 1 ? '空闲' : '已占用'}）` }))
    locationTotal.value = data.total
  } catch (e) { if (alive && seq === locationSeq && view === loadSeq) { locationOptions.value = []; locationError.value = operationErrorMessage(e, '库位加载失败，请重试') } }
  finally { if (alive && seq === locationSeq) locationLoading.value = false }
}
async function scanLocation() {
  if (locationLoading.value || loading.value || submitting.value || pendingPutaway.value) return
  form.location_id = undefined; locationError.value = ''
  let code: string
  try { code = scanCode(locationScan.value) } catch(e) { locationError.value = operationErrorMessage(e, '编码无效'); return }
  const seq = ++locationSeq, view = loadSeq, identity = operationStateKey('putaway-view')
  locationLoading.value = true
  try {
    const data = await putawayLocations(orderId.value, { code })
    if (!alive || seq !== locationSeq || view !== loadSeq || !visible.value || identity !== operationStateKey('putaway-view')) return
    const row = data.list[0]
    if (data.total !== 1 || !row || row.code !== code) { locationError.value = '当前单据仓库没有此编码的启用库位，请核对仓库和标签'; return }
    if (!locationOptions.value.some(l => l.id === row.id)) locationOptions.value.push({ id: row.id, label: `${row.code}（${row.status === 1 ? '空闲' : '已占用'}）` })
    form.location_id = row.id
  } catch (e) { if (alive && seq === locationSeq && view === loadSeq) locationError.value = operationErrorMessage(e, '库位核对失败，请重试') }
  finally { if (alive && seq === locationSeq) locationLoading.value = false }
}
function selectLocation(id: EntityID) { locationScan.value = ''; form.location_id = id; locationError.value = '' }

async function open(id: EntityID, task?: TaskItem) {
  if (submitting.value) return
  const seq = ++loadSeq, identity = operationStateKey('putaway-view')
  locationSeq++; locationLoading.value = false; locationError.value = ''; locationTotal.value = 0
  tasks.value = []; locationOptions.value = []; locationScan.value = ''; skuScan.reset()
  orderId.value = id
  orderNo.value = ''
  visible.value = true
  loading.value = true
  form.task_id = undefined
  form.location_id = undefined
  form.qty = 1
  try {
    const detail = await getInboundOrder(id)
    if (!alive || seq !== loadSeq || !visible.value || identity !== operationStateKey('putaway-view')) return
    orderNo.value = detail.order.order_no
    tasks.value = detail.tasks ?? []
    // 恢复这批任务上次未确认的上架操作（关闭弹窗、刷新后不丢）。
    const restoreError = puter.restoreForTasks(tasks.value.map((t) => t.id))
    if (restoreError) ElMessage.error(restoreError)

    const preselect = task ?? pendingTasks.value.find((t) => puter.stateOf(t.id).pending) ?? pendingTasks.value[0]
    if (preselect) {
      onTaskChange(preselect.id)
    } else {
      ElMessage.warning('暂无待执行的上架任务')
    }
    await loadLocations()
  } finally {
    if (alive && seq === loadSeq) loading.value = false
  }
}

function onTaskChange(taskId: EntityID) {
  locationSeq++; locationScan.value = ''; locationError.value = ''; locationLoading.value = false
  form.task_id = taskId
  const task = tasks.value.find((t) => t.id === taskId)
  form.qty = task ? Math.max(task.target_qty - task.done_qty, pendingPutaway.value?.businessPayload.qty || 1) : 1
  if (task) form.location_id = undefined
  const pending = puter.stateOf(taskId).pending
  if (pending && !pending.stale) {
    form.qty = pending.businessPayload.qty
    form.location_id = pending.businessPayload.location_id
  }
}

async function submit() {
  if (loading.value || locationLoading.value || locationError.value || skuScan.blocked.value || (locationScan.value && !form.location_id)) return
  const seq = loadSeq, identity = operationStateKey('putaway-view')
  const taskId = form.task_id
  if (!taskId) {
    ElMessage.warning('请选择上架任务')
    return
  }
  if (!form.location_id) {
    ElMessage.warning('请选择目标库位')
    return
  }
  if (form.qty <= 0) {
    ElMessage.warning('上架数量必须大于 0')
    return
  }
  try {
    await puter.putaway(taskId, {
      location_id: form.location_id,
      qty: form.qty,
    })
    if (!alive || seq !== loadSeq || !visible.value || identity !== operationStateKey('putaway-view')) return
    ElMessage.success('上架成功')
    visible.value = false
    emit('success')
  } catch {
    // 错误与待确认横幅已写入当前任务状态，由界面展示恢复/放弃入口。
  }
}

/** 恢复未确认的上架：原 key 原参数重试，服务端回放或重新执行同一操作。 */
async function recover() {
  const taskId = form.task_id
  if (!taskId) return
  try {
    await puter.recoverPutaway(taskId)
    ElMessage.success('上架操作已恢复')
    visible.value = false
    emit('success')
  } catch {
    // 错误与待确认横幅已写入当前任务状态。
  }
}

/** 核对后放弃未确认的上架：仅删除本地记录，不向服务端发送任何请求。 */
async function discard() {
  const taskId = form.task_id
  if (!taskId) return
  try {
    await ElMessageBox.confirm(
      '请确认已核对库存与任务进度：放弃后本次上架不会自动重试。',
      '核对并放弃待确认上架',
      { type: 'warning', confirmButtonText: '已核对，放弃', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  puter.discardPutaway(taskId)
  ElMessage.success('已放弃待确认的上架操作')
}

defineExpose({ open })
</script>

<template>
  <el-dialog class="business-dialog business-operation"
    v-model="visible"
    title="上架"
    width="520px"
    destroy-on-close
    :close-on-click-modal="!loading && !submitting"
    :close-on-press-escape="!submitting"
    :show-close="!submitting"
  >
    <div class="operation-context">
      <span>入库单</span><strong>{{ orderNo || '加载中…' }}</strong>
      <p>选择上架任务与目标库位，核对待上架数量后确认。</p>
    </div>
    <el-form label-width="90px" v-loading="loading" @submit.prevent>
      <el-form-item v-if="storageError" label=" ">
        <el-alert :title="storageError" type="error" show-icon :closable="false" />
      </el-form-item>
      <el-form-item v-if="pendingPutaway || lastError" label=" ">
        <el-alert
          :title="
            pendingPutaway
              ? pendingPutaway.stale
                ? '任务存在未确认的上架操作（已超过保留期，请核对库存与任务进度）'
                : '任务存在未确认的上架操作'
              : lastError
          "
          :type="pendingPutaway ? 'warning' : 'error'"
          show-icon
          :closable="false"
        >
          <div v-if="pendingPutaway" class="pending-actions">
            <span class="pending-detail">
              库位 {{ pendingPutaway.businessPayload.location_id }}，数量 {{ pendingPutaway.businessPayload.qty }}
            </span>
            <el-button size="small" type="primary" :disabled="pendingPutaway.stale" @click="recover">恢复上架</el-button>
            <el-button size="small" @click="discard">已核对，放弃</el-button>
          </div>
        </el-alert>
      </el-form-item>
      <el-form-item label="上架任务">
        <el-select v-model="form.task_id" placeholder="选择待执行的上架任务" style="width: 100%" @change="onTaskChange">
          <el-option
            v-for="t in pendingTasks"
            :key="t.id"
            :value="t.id"
            :label="`${t.task_no}（目标 ${t.target_qty} 已完成 ${t.done_qty}）`"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="货品核对">
        <ScanField v-model="skuScan.code.value" label="上架货品扫码" placeholder="扫描货品条码或编码（可选）" :disabled="loading || submitting || !!pendingPutaway || !form.task_id" :loading="skuScan.loading.value" @scan="skuScan.scan" />
        <div v-if="skuScan.match.value" class="qty-tip">已核对 {{ skuScan.match.value.name }} · {{ skuScan.match.value.code }}</div>
        <el-alert v-if="skuScan.error.value" :title="skuScan.error.value" type="error" :closable="false" />
      </el-form-item>
      <el-form-item label="库位扫码">
        <ScanField v-model="locationScan" label="上架库位扫码" placeholder="扫描同仓库位编码，回车核对" :disabled="loading || submitting || !!pendingPutaway" :loading="locationLoading" @scan="scanLocation" />
        <div class="scan-hint">扫码只选择库位；确认上架时仍会重新校验。也可直接从下方选择。</div>
        <el-alert v-if="locationError" :title="locationError" type="error" :closable="false" />
      </el-form-item>
      <el-form-item label="目标库位">
        <el-select
          v-model="form.location_id"
          placeholder="搜索或选择启用库位"
          remote
          :remote-method="loadLocations"
          :loading="locationLoading"
          @change="selectLocation"
          filterable
          style="width: 100%"
          :disabled="!!pendingPutaway || submitting || loading"
        >
          <el-option v-for="loc in locationOptions" :key="loc.id" :label="loc.label" :value="loc.id" />
        </el-select>
      </el-form-item>
      <div v-if="locationTotal > 100" class="scan-hint">显示前100个库位，可搜索完整编码或使用扫码核对。</div>
      <el-form-item label="上架数量">
        <el-input-number v-model="form.qty" :min="1" :disabled="!!pendingPutaway" controls-position="right" />
        <span v-if="selectedTask" class="qty-tip">待上架：{{ selectedTask.target_qty - selectedTask.done_qty }}</span>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button :disabled="submitting" @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" :disabled="!!pendingPutaway || skuScan.blocked.value || locationLoading || !!locationError || !form.location_id || loading" @click="submit">确定上架</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.scan-hint { font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.6; }
.qty-tip {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.pending-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
  flex-wrap: wrap;
}

.pending-detail {
  font-size: 12px;
  color: var(--el-text-color-regular);
}
</style>