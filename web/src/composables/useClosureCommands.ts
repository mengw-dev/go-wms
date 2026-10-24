import { onScopeDispose, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { isClosureAck, isClosureCommand, sendClosureCommand, type ClosureAck, type ClosureCommand } from '@/api/closure'
import {
  classifyOperationError, getOperationContext, onOperationStorageChange, operationErrorMessage,
  operationStateKey, prepareOperation,
} from '@/composables/operationLifecycle'
import { isStale, loadPending, removePending, type PendingOperation } from '@/utils/pendingOperations'

const scope = 'warehouse.closure'
const object = 'closure-command'

/** One unresolved stock/closure write per browser identity; retry always uses its persisted command. */
export function useClosureCommands() {
  const auth = useAuthStore()
  const pending = ref<PendingOperation | null>(null)
  const busy = ref(false)
  const error = ref('')
  let flight = 0
  let disposed = false
  const identity = () => JSON.stringify([auth.sessionVersion, operationStateKey(object)])

  function refresh() {
    const current = getOperationContext('库存与异常')
    if (!current.ctx) { pending.value = null; error.value = current.error; return }
    const loaded = loadPending(current.ctx.identity, current.ctx.storage)
    pending.value = loaded.items.filter(i => i.scope === scope && i.objectId === object).sort((a, b) => b.createdAt.localeCompare(a.createdAt))[0] ?? null
    error.value = loaded.error || ''
    if (pending.value && !isClosureCommand(pending.value.businessPayload)) error.value = '待确认记录损坏，请先核对业务结果，禁止重发'
    else if (pending.value && isStale(pending.value)) error.value = '待确认操作已超过7天，请核对业务结果后处理，禁止自动重发'
  }
  watch(identity, () => { flight++; busy.value = false; refresh() }, { immediate: true })
  onOperationStorageChange(refresh)
  onScopeDispose(() => { disposed = true; flight++ })

  async function execute(command: ClosureCommand, key: string): Promise<ClosureAck | null> {
    const current = getOperationContext('库存与异常')
    if (!current.ctx || disposed || busy.value) { error.value = current.error || '正在提交，请勿重复操作'; return null }
    const owner = identity(), seq = ++flight, ctx = current.ctx
    const active = () => !disposed && seq === flight && identity() === owner
    busy.value = true
    try {
      const ack = await sendClosureCommand(command, key)
      if (!active()) return null
      if (!isClosureAck(ack, command)) throw new Error('操作回执无法核对，请保留原请求并恢复，勿重新提交')
      const removed = removePending(ctx.identity, ctx.storage, scope, object, key)
      if (!removed.ok) { error.value = removed.error; return null }
      refresh()
      return ack
    } catch (cause) {
      if (!active()) return null
      if (classifyOperationError(cause, scope) === 'rejected') {
        const removed = removePending(ctx.identity, ctx.storage, scope, object, key)
        if (!removed.ok) { error.value = removed.error; return null }
      }
      refresh()
      error.value = cause && typeof cause === 'object' && 'kind' in cause && cause.kind === 'network'
        ? '网络连接中断，操作结果待确认，请用原参数恢复'
        : operationErrorMessage(cause, '操作结果未知，请用原参数重试')
      return null
    } finally {
      if (active()) busy.value = false
    }
  }

  async function run(command: ClosureCommand) {
    if (disposed || busy.value) return null
    refresh()
    if (pending.value || error.value) { error.value ||= '请先恢复或核对待确认操作'; return null }
    if (!isClosureCommand(command)) { error.value = '请完整填写操作参数'; return null }
    const current = getOperationContext('库存与异常')
    if (!current.ctx) { error.value = current.error; return null }
    // Clone before persistence and await, so later form edits cannot change this operation.
    const snapshot = JSON.parse(JSON.stringify(command)) as ClosureCommand
    let key: string
    try { key = crypto.randomUUID() } catch { error.value = '无法生成安全操作编号，已暂停提交'; return null }
    const failure = prepareOperation(current.ctx, scope, object, key, snapshot as unknown as Record<string, unknown>)
    if (failure) { error.value = failure; return null }
    refresh()
    return execute(snapshot, key)
  }
  async function recover() {
    if (disposed || busy.value) return null
    refresh()
    const op = pending.value
    if (!op || error.value || !isClosureCommand(op.businessPayload)) return null
    return execute(op.businessPayload, op.operationKey)
  }
  function discard() {
    if (disposed || busy.value) return
    refresh()
    const current = getOperationContext('库存与异常'), op = pending.value
    if (!current.ctx || !op) return
    const removed = removePending(current.ctx.identity, current.ctx.storage, scope, object, op.operationKey)
    if (!removed.ok) { error.value = removed.error; return }
    refresh()
  }
  return { pending, busy, error, run, recover, discard, refresh }
}
