import { reactive, ref } from 'vue'
import { putawayInboundTask } from '@/api/inbound'
import type { EntityID, PutawayParams } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import {
  PENDING_SCHEMA_VERSION,
  findPending,
  isStale,
  loadPending,
  removePending,
  upsertPending,
  type OperationIdentity,
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

  /** 服务端语义可确认「本次内容没有执行任何写入」的静态校验类错误码。 */
  const PUTAWAY_SAFE_REJECT_CODES = new Set([400, 40001, 40002, 40005, 40006, 40007, 40008, 20002, 20005, 20011, 20014])

  function currentContext(): { ctx: { identity: OperationIdentity; storage: Storage } | null; error: string } {
    const auth = useAuthStore()
    if (!auth.token || !auth.user?.user_id) {
      return { ctx: null, error: '登录状态无效，无法进行上架操作' }
    }
    const isDemo = (auth.user.perms ?? []).includes('wms:demo')
    let storage: Storage
    try {
      storage = isDemo ? sessionStorage : localStorage
    } catch {
      return { ctx: null, error: '浏览器存储不可用，无法记录待确认操作' }
    }
    return {
      ctx: {
        identity: { userId: auth.user.user_id, sessionNamespace: isDemo ? auth.demoSessionId || '' : '' },
        storage,
      },
      error: '',
    }
  }

  function stateOf(taskId: EntityID): TaskState {
    const key = String(taskId)
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

  function beginOperation(ctx: { identity: OperationIdentity; storage: Storage }, taskId: EntityID, operationKey: string, businessPayload: PutawayBusinessPayload): string {
    const base: PendingOperation = {
      schemaVersion: PENDING_SCHEMA_VERSION,
      userId: ctx.identity.userId,
      sessionNamespace: ctx.identity.sessionNamespace,
      scope: PUTAWAY_SCOPE,
      objectId: taskId,
      operationKey,
      businessPayload,
      createdAt: new Date().toISOString(),
      state: 'prepared',
    }
    const prepared = upsertPending(ctx.identity, ctx.storage, base)
    if (!prepared.ok) return prepared.error
    const sending = upsertPending(ctx.identity, ctx.storage, { ...base, state: 'sending' })
    if (!sending.ok) {
      removePending(ctx.identity, ctx.storage, PUTAWAY_SCOPE, taskId, operationKey)
      return sending.error
    }
    return ''
  }

  type ErrorOutcome = 'uncertain' | 'rejected' | 'conflict'

  function classify(error: unknown): ErrorOutcome {
    if (!error || typeof error !== 'object') return 'uncertain'
    const api = error as { code?: unknown; status?: unknown; kind?: unknown }
    if (api.code === 40901) return 'conflict'
    if (api.kind === 'network') return 'uncertain'
    if (api.status === 401 || api.status === 403) return 'uncertain'
    if (typeof api.status === 'number' && api.status >= 500) return 'uncertain'
    if (typeof api.code === 'number' && PUTAWAY_SAFE_REJECT_CODES.has(api.code)) return 'rejected'
    return 'uncertain'
  }

  function errorMessage(error: unknown): string {
    if (error && typeof error === 'object' && typeof (error as Pick<Error, 'message'>).message === 'string') {
      return (error as Pick<Error, 'message'>).message
    }
    return '上架失败'
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
    const saveError = beginOperation(c.ctx, taskId, operationKey, data)
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
    return await sendPutaway(taskId, existing.operationKey, existing.businessPayload as PutawayBusinessPayload)
  }

  /** 核对后放弃未确认的上架：仅删除本地记录，不向服务端发送任何请求。 */
  function discardPutaway(taskId: EntityID) {
    const st = stateOf(taskId)
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
      endOperation(taskId, operationKey)
      st.pending = null
    } catch (error) {
      const outcome = classify(error)
      if (st.flight === seq) {
        st.lastError = errorMessage(error)
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
      const st = states.get(String(id))
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
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', () => {
      if (knownTaskIds.length > 0) restoreForTasks(knownTaskIds)
    })
  }

  return {
    stateOf,
    storageError,
    putaway,
    recoverPutaway,
    discardPutaway,
    restoreForTasks,
  }
}