import { onScopeDispose, ref } from 'vue'
import { moveStock, listStockMoves, type MovePayload, type StockMove } from '@/api/stockMoves'
import { classifyOperationError, getOperationContext, onOperationStorageChange, operationErrorMessage, operationStateKey, prepareOperation } from '@/composables/operationLifecycle'
import { isStale, loadPending, removePending, upsertPending, type PendingOperation } from '@/utils/pendingOperations'

const scope = 'inventory.move'
export function movePayload(item: PendingOperation): MovePayload | null {
  const p = item.businessPayload
  if (!p || typeof p.source_inventory_id !== 'string' || p.source_inventory_id !== item.objectId || !/^[1-9]\d*$/.test(p.source_inventory_id) || typeof p.target_location_id !== 'string' || !/^[1-9]\d*$/.test(p.target_location_id) || typeof p.quantity !== 'number' || !Number.isInteger(p.quantity) || p.quantity <= 0 || p.quantity > 2147483647 || typeof p.reason !== 'string' || !p.reason.trim() || [...p.reason].length > 255) return null
  return { source_inventory_id: p.source_inventory_id, target_location_id: p.target_location_id, quantity: p.quantity, reason: p.reason }
}

export function useStockMove() {
  const pending = ref<PendingOperation[]>([]), error = ref(''), busy = ref(false)
  let alive = true
  onScopeDispose(() => { alive = false })
  function restore() {
    const c = getOperationContext('移库')
    if (!c.ctx) { error.value = c.error; pending.value = []; return }
    const saved = loadPending(c.ctx.identity, c.ctx.storage)
    pending.value = saved.items.filter(item => item.scope === scope)
    if (saved.error) error.value = saved.error
    else if (pending.value.some(item => !movePayload(item))) error.value = '移库待确认参数已损坏，请通过记录核对，已暂停新建与重试'
    else error.value = ''
  }
  onOperationStorageChange(() => { if (!busy.value) restore() })
  function forget(item: PendingOperation) {
    if (busy.value) return
    const c = getOperationContext('移库'); if (!c.ctx) { error.value = c.error; return }
    const removed = removePending(c.ctx.identity, c.ctx.storage, scope, item.objectId, item.operationKey)
    if (!removed.ok) error.value = removed.error; else restore()
  }
  async function send(payload: MovePayload, existing?: PendingOperation): Promise<StockMove | undefined> {
    if (!alive || busy.value) return
    restore()
    if (error.value) return
    if (existing && (!pending.value.some(item => item.operationKey === existing.operationKey) || !movePayload(existing) || isStale(existing))) { error.value = '原请求不存在、已损坏或超过7天，请核对移库记录后处理'; return }
    if (!existing && pending.value.length) { error.value = '请先核对或重试上次移库，避免再次移动'; return }
    const c = getOperationContext('移库'); if (!c.ctx) { error.value = c.error; return }
    const state = operationStateKey('stock-move'), key = existing?.operationKey || crypto.randomUUID()
    const written = existing
      ? upsertPending(c.ctx.identity, c.ctx.storage, { ...existing, state: 'sending' }).error
      : prepareOperation(c.ctx, scope, payload.source_inventory_id, key, { ...payload })
    if (written) { error.value = written; return }
    busy.value = true
    try {
      const result = await moveStock(payload, key)
      if (!alive || operationStateKey('stock-move') !== state) return
      // A malformed success is uncertain, never permission to issue another key.
      if (!result?.id || !result.move_no || result.source_inventory_id !== payload.source_inventory_id || result.target_location_id !== payload.target_location_id || result.quantity !== payload.quantity || result.reason !== payload.reason.trim()) throw Error('移库响应无法核对，请保留原请求编号')
      const removed = removePending(c.ctx.identity, c.ctx.storage, scope, payload.source_inventory_id, key)
      restore()
      if (!removed.ok) error.value = '移库已完成，但本地记录未能清除，请核对后处理'
      return result
    } catch (e) {
      if (!alive || operationStateKey('stock-move') !== state) return
      if (classifyOperationError(e, scope) === 'rejected') removePending(c.ctx.identity, c.ctx.storage, scope, payload.source_inventory_id, key)
      restore(); error.value = operationErrorMessage(e, '移库结果未确认，请保留原请求核对或重试')
    } finally { if (alive) busy.value = false }
  }
  async function retry(item: PendingOperation) { const payload = movePayload(item); if (payload) return send(payload, item); error.value = '待确认参数已损坏，不能重试' }
  async function check(item: PendingOperation): Promise<StockMove | undefined> {
    if (busy.value || !alive) return
    restore()
    if (!pending.value.some(p => p.objectId === item.objectId && p.operationKey === item.operationKey)) return
    const state = operationStateKey('stock-move'), c = getOperationContext('核对移库')
    if (!c.ctx) { error.value = c.error; return }
    busy.value = true
    try {
      const rows = await listStockMoves({ operation_key: item.operationKey, page: 1, page_size: 1 })
      if (!alive || operationStateKey('stock-move') !== state) return
      const found = rows.list?.[0], p = movePayload(item)
      if (!found) { error.value = '未找到已提交记录；若仍无法确认，请按原请求重试或人工核对'; return }
      if (!p || !found.id || !found.move_no || found.source_inventory_id !== p.source_inventory_id || found.target_location_id !== p.target_location_id || found.quantity !== p.quantity || found.reason !== p.reason.trim()) { error.value = '原请求与移库记录不一致，已保留本地记录，请人工核对'; return }
      const removed = removePending(c.ctx.identity, c.ctx.storage, scope, item.objectId, item.operationKey)
      restore(); if (!removed.ok) error.value = removed.error
      return found
    } catch (e) { if (alive && operationStateKey('stock-move') === state) error.value = operationErrorMessage(e, '核对失败，保留原请求') }
    finally { if (alive) busy.value = false }
  }
  return { pending, error, busy, restore, send, retry, check, forget }
}
