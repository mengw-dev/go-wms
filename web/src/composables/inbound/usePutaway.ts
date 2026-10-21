import { reactive, ref } from 'vue'
import {
  classifyOperationError, getOperationContext, onOperationStorageChange,
  operationErrorMessage, operationStateKey, prepareOperation,
} from '@/composables/operationLifecycle'
import { putawayInboundTask } from '@/api/inbound'
import type { EntityID, PutawayParams } from '@/api/types'
import {
  findPending,
  isStale,
  loadPending,
  removePending,
  upsertPending,
  type PendingOperation,
} from '@/utils/pendingOperations'

/**
 * 上架操作生命周期（复用 utils/pendingOperations，与收货/拣货同一套规则）：
 * 一次真实上架 = 一个新 key；提交前先落盘「key + 业务参数」，结果未知保留原 key
 * 原参数，确认成功后删除记录、下次上架（即使参数相同）生成新 key。
 *
 * 状态按任务隔离，objectId = task_id。存储读写非原子，不提供跨标签页互斥。
 */
export function usePutaway() {
  const PUTAWAY_SCOPE = 'inbound.putaway'

  type PutawayBusinessPayload = { location_id: EntityID; qty: number }

  interface PendingPutawayInfo {
    operationKey: string
    businessPayload: PutawayBusinessPayload
    createdAt: string
    stale: boolean
  }

  interface TaskState {
    submitting: boolean
    flight: number
    pending: PendingPutawayInfo | null
    blocked: boolean
    blockedReason: string
    lastError: string
  }

  const states = reactive(new Map<string, TaskState>())
  const storageError = ref('')
  let knownTaskIds: EntityID[] = []

  const currentContext = () => getOperationContext('上架')

  function stateOf(taskId: EntityID): TaskState {
    const key = operationStateKey(taskId)
    let st = states.get(key)
    if (!st) {
      st = { submitting: false, flight: 0, pending: null, blocked: false, blockedReason: '', lastError: '' }
      states.set(key, st)
    }
    return st
  }

  function findOp(taskId: EntityID): PendingOperation | null {
    const c = currentContext()
    if (!c.ctx) return null
    const loaded = loadPending(c.ctx.identity, c.ctx.storage)
    if (loaded.error) {
      storageError.value = loaded.error
      throw new Error(loaded.error)
    }
    return findPending(c.ctx.identity, c.ctx.storage, PUTAWAY_SCOPE, taskId)
  }

  function endOperation(taskId: EntityID, operationKey: string) {
    const c = currentContext()
    if (!c.ctx) return
    const removed = removePending(c.ctx.identity, c.ctx.storage, PUTAWAY_SCOPE, taskId, operationKey)
    if (!removed.ok) storageError.value = removed.error
  }

  function markUncertain(taskId: EntityID) {
    const c = currentContext()
    if (!c.ctx) return
    const existing = findPending(c.ctx.identity, c.ctx.storage, PUTAWAY_SCOPE, taskId)
    if (!existing) return
    const saved = upsertPending(c.ctx.identity, c.ctx.storage, { ...existing, state: 'uncertain' })
    if (!saved.ok) storageError.value = saved.error
  }

  function samePayload(a: unknown, b: PutawayBusinessPayload): boolean {
    if (!a || typeof a !== 'object' || Array.isArray(a)) return false
    const pa = a as Record<string, unknown>
    return pa.location_id === b.location_id && pa.qty === b.qty
  }

  function toPendingInfo(operation: PendingOperation, stale: boolean): PendingPutawayInfo {
    return {
      operationKey: operation.operationKey,
      businessPayload: (operation.businessPayload as PutawayBusinessPayload) ?? { location_id: '', qty: 0 },
      createdAt: operation.createdAt,
      stale,
    }
  }

  /** 提交上架：有未确认记录且参数一致则原 key 重试；参数不同/超窗口拒绝发送。 */
  async function putaway(taskId: EntityID, data: PutawayParams): Promise<void> {
    const st = stateOf(taskId)
    if (st.submitting) {
      throw new Error('该任务的上架请求正在进行，请勿重复提交')
    }
    const c = currentContext()
    if (!c.ctx) {
      st.lastError = c.error
      throw new Error(c.error)
    }
    if (storageError.value) {
      st.lastError = storageError.value
      throw new Error(storageError.value)
    }
    if (st.blocked) {
      st.lastError = st.blockedReason
      throw new Error(st.blockedReason)
    }
    const existing = findOp(taskId)
    if (existing) {
      if (isStale(existing)) {
        st.pending = toPendingInfo(existing, true)
        st.lastError = '该任务的上次上架已超过保留期，请核对库存与任务进度后再操作'
        throw new Error(st.lastError)
      }
      if (!samePayload(existing.businessPayload, data)) {
        st.pending = toPendingInfo(existing, false)
        st.lastError = '该任务存在未确认的上架操作，请先恢复重试或核对后放弃，不能更改库位或数量后直接提交'
        throw new Error(st.lastError)
      }
      return await sendPutaway(taskId, existing.operationKey, data)
    }
    const operationKey = crypto.randomUUID()
    const saveError = prepareOperation(c.ctx, PUTAWAY_SCOPE, taskId, operationKey, data as PutawayBusinessPayload)
    if (saveError) {
      storageError.value = saveError
      st.lastError = saveError
      throw new Error(saveError)
    }
    return await sendPutaway(taskId, operationKey, data)
  }

  /** 恢复未确认的上架（原 key 原参数重试）。 */
  async function recoverPutaway(taskId: EntityID): Promise<void> {
    const st = stateOf(taskId)
    const existing = findOp(taskId)
    if (!existing) {
      st.pending = null
      st.lastError = '没有待恢复的上架操作'
      throw new Error(st.lastError)
    }
    if (isStale(existing)) {
      st.pending = toPendingInfo(existing, true)
      st.lastError = '该任务的上次上架已超过保留期，请核对库存与任务进度后再操作'
      throw new Error(st.lastError)
    }
    return await putaway(taskId, existing.businessPayload as PutawayBusinessPayload)
  }

  /** 核对后放弃未确认的上架：仅删除本地记录，不向服务端发送任何请求。 */
  function discardPutaway(taskId: EntityID) {
    const st = stateOf(taskId)
    if (st.submitting) return
    const existing = findOp(taskId)
    if (!existing) {
      st.pending = null
      return
    }
    const c = currentContext()
    if (!c.ctx) return
    const removed = removePending(c.ctx.identity, c.ctx.storage, PUTAWAY_SCOPE, taskId, existing.operationKey)
    if (!removed.ok) {
      storageError.value = removed.error
      st.lastError = removed.error
      return
    }
    st.pending = null
    st.blocked = false
    st.blockedReason = ''
  }

  async function sendPutaway(taskId: EntityID, operationKey: string, data: PutawayBusinessPayload): Promise<void> {
    const st = stateOf(taskId)
    const seq = ++st.flight
    st.submitting = true
    st.lastError = ''
    try {
      await putawayInboundTask(taskId, data, operationKey)
      if (stateOf(taskId) !== st) throw new Error('登录身份已变更，请在原账号核对该操作')
      endOperation(taskId, operationKey)
      st.pending = null
    } catch (error) {
      if (stateOf(taskId) !== st) throw error
      const outcome = classifyOperationError(error, PUTAWAY_SCOPE)
      if (st.flight === seq) {
        st.lastError = operationErrorMessage(error, '上架失败')
      }
      if (outcome === 'rejected') {
        endOperation(taskId, operationKey)
        st.pending = null
      } else if (outcome === 'conflict') {
        markUncertain(taskId)
        const stored = findOp(taskId)
        st.pending = stored
          ? toPendingInfo(stored, false)
          : { operationKey, businessPayload: data, createdAt: new Date().toISOString(), stale: false }
        st.blocked = true
        st.blockedReason = '该请求编号已被用于其他内容，请核对原上架操作后放弃重试'
      } else {
        markUncertain(taskId)
        const stored = findOp(taskId)
        st.pending = stored
          ? toPendingInfo(stored, isStale(stored))
          : { operationKey, businessPayload: data, createdAt: new Date().toISOString(), stale: false }
      }
      throw error
    } finally {
      if (st.flight === seq) st.submitting = false
    }
  }

  /** 打开弹窗时恢复这批任务的待确认操作；返回存储层错误（如有）。 */
  function restoreForTasks(taskIds: EntityID[]): string {
    knownTaskIds = taskIds
    for (const id of taskIds) {
      const st = stateOf(id)
      if (st) {
        st.pending = null
        st.blocked = false
        st.blockedReason = ''
      }
    }
    const c = currentContext()
    if (!c.ctx) return c.error
    const loaded = loadPending(c.ctx.identity, c.ctx.storage)
    if (loaded.error) {
      storageError.value = loaded.error
      return loaded.error
    }
    storageError.value = ''
    const idSet = new Set(taskIds.map(String))
    for (const item of loaded.items) {
      if (!idSet.has(String(item.objectId)) || item.scope !== PUTAWAY_SCOPE) continue
      stateOf(item.objectId as EntityID).pending = toPendingInfo(item, isStale(item))
    }
    return ''
  }

  // 其他标签页写入存储时，本地横幅同步绑定最新状态；这不是跨标签页互斥。
  onOperationStorageChange(() => {
    if (knownTaskIds.length > 0) restoreForTasks(knownTaskIds)
  })

  return {
    stateOf,
    storageError,
    putaway,
    recoverPutaway,
    discardPutaway,
    restoreForTasks,
  }
}
