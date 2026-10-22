import { computed, onScopeDispose, ref, watch } from 'vue'
import { scanOrderSKU, type ScannedSKU } from '@/api/scanning'
import { operationErrorMessage, operationStateKey } from './operationLifecycle'
import { scanCode } from '@/utils/scanCode'

/** Optional read-only assistance. A scan never claims a task or writes quantity. */
export function useSkuScan(kind: 'inbound' | 'outbound', orderID: () => string, scope: () => string, expectedSKU: () => string = () => '') {
  const code = ref(''), match = ref<ScannedSKU>(), error = ref(''), loading = ref(false)
  let sequence = 0, alive = true
  const state = () => JSON.stringify([operationStateKey('scan'), orderID(), scope(), expectedSKU()])
  function reset() { sequence++; code.value = ''; match.value = undefined; error.value = ''; loading.value = false }
  watch(state, reset, { flush: 'sync' })
  watch(code, () => { sequence++; match.value = undefined; error.value = ''; loading.value = false }, { flush: 'sync' })
  onScopeDispose(() => { alive = false; sequence++ })
  async function scan() {
    if (loading.value || !alive) return
    match.value = undefined; error.value = ''
    let value: string
    try { value = scanCode(code.value) } catch (e) { error.value = operationErrorMessage(e, '编码无效'); return }
    const seq = ++sequence, identity = state(), id = orderID()
    if (!id) return
    loading.value = true
    try {
      const rows = await scanOrderSKU(kind, id, value)
      if (!alive || seq !== sequence || identity !== state()) return
      if (rows.length !== 1) { error.value = rows.length ? '编码匹配多个货品，请使用无歧义的货品编码核对' : '当前单据没有此编码对应的启用货品，请检查标签'; return }
      const row = rows[0]!
      if (row.barcode !== value && row.code !== value) { error.value = '返回货品与扫描编码不符，请重新核对'; return }
      if (expectedSKU() && row.id !== expectedSKU()) { error.value = '货品不符，请核对当前任务对应的货品'; return }
      match.value = row
    } catch (e) { if (alive && seq === sequence && identity === state()) error.value = operationErrorMessage(e, '扫码核对失败，请重试') }
    finally { if (alive && seq === sequence) loading.value = false }
  }
  const blocked = computed(() => !!code.value && !match.value)
  return { code, match, error, loading, blocked, scan, reset }
}
