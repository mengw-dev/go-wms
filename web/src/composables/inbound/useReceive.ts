import { reactive, ref } from 'vue'
import { receiveInbound } from '@/api/inbound'
import type { EntityID, ReceiveParams } from '@/api/types'
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
 * 收货操作生命周期（复用 utils/pendingOperations，与 PDA 拣货同一套规则）：
 * 一次真实收货 = 一个新 key；提交前先落盘「key + 业务参数」，发送失败/结果未知
 * 保留原 key 原参数，确认成功后删除记录、下次收货（即使参数相同）生成新 key。
 *
 * 状态按明细隔离：pending 横幅、错误与提交中标记均绑定 detail_id。
 * 演示账号用 sessionStorage，普通账号用 localStorage；存储读写非原子，
 * 不提供跨标签页互斥（多标签页并发由服务端 key 级去重兜底）。
 */
export function useReceive() {
  const RECEIVE_SCOPE = 'inbound.receive'

  /** 收货业务参数（不含 detail_id：objectId=detailID 已承载归属）。 */
  type ReceiveBusinessPayload = { qty: number; defective_qty: number; batch_no: string }

  interface PendingReceiveInfo {
    operationKey: string
    businessPayload: ReceiveBusinessPayload
    createdAt: string
    stale: boolean
  }

  interface RowState {
    submitting: boolean
    flight: number
    pending: PendingReceiveInfo | null
    blocked: boolean
    blockedReason: string
    lastError: string
  }

  const states = reactive(new Map<string, RowState>())
  /** 存储层异常（损坏/不可写）：未解决前禁止创建任何新操作。 */
  const storageError = ref('')
  let knownDetailIds: EntityID[] = []

  /** 服务端语义可确认「本次内容没有执行任何写入」的静态校验类错误码。 */
  const RECEIVE_SAFE_REJECT_CODES = new Set([400, 40001, 40002, 40004, 40008, 40014])

  function currentContext(): { ctx: { identity: OperationIdentity; storage: Storage } | null; error: string } {
    const auth = useAuthStore()
    if (!auth.token || !auth.user?.user_id) {
      return { ctx: null, error: '登录状态无效，无法进行收货操作' }
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

  function stateOf(detailId: EntityID): RowState {
    const key = String(detailId)
    let st = states.get(key)
    if (!st) {
      st = { submitting: false, flight: 0, pending: null, blocked: false, blockedReason: '', lastError: '' }
      states.set(key, st)
    }
    return st
  }

  function findOp(detailId: EntityID): PendingOperation | null {
    const c = currentContext()
    if (!c.ctx) return null
    return findPending(c.ctx.identity, c.ctx.storage, RECEIVE_SCOPE, detailId)
  }

  function endOperation(detailId: EntityID, operationKey: string) {
    const c = currentContext()
    if (!c.ctx) return
    const removed = removePending(c.ctx.identity, c.ctx.storage, RECEIVE_SCOPE, detailId, operationKey)
    if (!removed.ok) storageError.value = removed.error
  }

  function markUncertain(detailId: EntityID) {
    const c = currentContext()
    if (!c.ctx) return
    const existing = findPending(c.ctx.identity, c.ctx.storage, RECEIVE_SCOPE, detailId)
    if (!existing) return
    const saved = upsertPending(c.ctx.identity, c.ctx.storage, { ...existing, state: 'uncertain' })
    if (!saved.ok) storageError.value = saved.error
  }

  function beginOperation(ctx: { identity: OperationIdentity; storage: Storage }, detailId: EntityID, operationKey: string, businessPayload: ReceiveBusinessPayload): string {
    const base: PendingOperation = {
      schemaVersion: PENDING_SCHEMA_VERSION,
      userId: ctx.identity.userId,
      sessionNamespace: ctx.identity.sessionNamespace,
      scope: RECEIVE_SCOPE,
      objectId: detailId,
      operationKey,
      businessPayload,
      createdAt: new Date().toISOString(),
      state: 'prepared',
    }
    const prepared = upsertPending(ctx.identity, ctx.storage, base)
    if (!prepared.ok) return prepared.error
    const sending = upsertPending(ctx.identity, ctx.storage, { ...base, state: 'sending' })
    if (!sending.ok) {
      removePending(ctx.identity, ctx.storage, RECEIVE_SCOPE, detailId, operationKey)
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
    if (typeof api.code === 'number' && RECEIVE_SAFE_REJECT_CODES.has(api.code)) return 'rejected'
    return 'uncertain'
  }

  function errorMessage(error: unknown): string {
    if (error && typeof error === 'object' && typeof (error as Pick<Error, 'message'>).message === 'string') {
      return (error as Pick<Error, 'message'>).message
    }
    return '收货失败'
  }

  /** 参数一致性按业务口径精确比较（批次不做 trim/大小写归一化，与服务端一致）。 */
  function samePayload(a: unknown, b: ReceiveBusinessPayload): boolean {
    if (!a || typeof a !== 'object' || Array.isArray(a)) return false
    const pa = a as Record<string, unknown>
    return pa.qty === b.qty && pa.defective_qty === b.defective_qty && pa.batch_no === b.batch_no
  }

  function toPendingInfo(operation: PendingOperation, stale: boolean): PendingReceiveInfo {
    return {
      operationKey: operation.operationKey,
      businessPayload: (operation.businessPayload as ReceiveBusinessPayload) ?? { qty: 0, defective_qty: 0, batch_no: '' },
      createdAt: operation.createdAt,
      stale,
    }
  }

  /** 提交收货：有未确认记录且参数一致则原 key 重试；参数不同/超窗口拒绝发送。 */
  async function receive(orderId: EntityID, detailId: EntityID, data: Omit<ReceiveParams, 'detail_id'>): Promise<void> {
    const st = stateOf(detailId)
    if (st.submitting) {
      throw new Error('该明细的收货请求正在进行，请勿重复提交')
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
    const existing = findOp(detailId)
    if (existing) {
      if (isStale(existing)) {
        st.pending = toPendingInfo(existing, true)
        st.lastError = '该明细的上次收货已超过保留期，请核对已收数量后再操作'
        throw new Error(st.lastError)
      }
      if (!samePayload(existing.businessPayload, data)) {
        st.pending = toPendingInfo(existing, false)
        st.lastError = '该明细存在未确认的收货操作，请先恢复重试或核对后放弃，不能更改数量后直接提交'
        throw new Error(st.lastError)
      }
      return await sendReceive(orderId, detailId, existing.operationKey, data)
    }
    const operationKey = crypto.randomUUID()
    const saveError = beginOperation(c.ctx, detailId, operationKey, data)
    if (saveError) {
      storageError.value = saveError
      st.lastError = saveError
      throw new Error(saveError)
    }
    return await sendReceive(orderId, detailId, operationKey, data)
  }

  /** 恢复未确认的收货（原 key 原参数重试）。 */
  async function recoverReceive(orderId: EntityID, detailId: EntityID): Promise<void> {
    const st = stateOf(detailId)
    const existing = findOp(detailId)
    if (!existing) {
      st.pending = null
      st.lastError = '没有待恢复的收货操作'
      throw new Error(st.lastError)
    }
    if (isStale(existing)) {
      st.pending = toPendingInfo(existing, true)
      st.lastError = '该明细的上次收货已超过保留期，请核对已收数量后再操作'
      throw new Error(st.lastError)
    }
    return await sendReceive(orderId, detailId, existing.operationKey, existing.businessPayload as ReceiveBusinessPayload)
  }

  /** 核对后放弃未确认的收货：仅删除本地记录，不向服务端发送任何请求。 */
  function discardReceive(detailId: EntityID) {
    const st = stateOf(detailId)
    const existing = findOp(detailId)
    if (!existing) {
      st.pending = null
      return
    }
    const c = currentContext()
    if (!c.ctx) return
    const removed = removePending(c.ctx.identity, c.ctx.storage, RECEIVE_SCOPE, detailId, existing.operationKey)
    if (!removed.ok) {
      storageError.value = removed.error
      st.lastError = removed.error
      return
    }
    st.pending = null
    st.blocked = false
    st.blockedReason = ''
  }

  async function sendReceive(orderId: EntityID, detailId: EntityID, operationKey: string, data: ReceiveBusinessPayload): Promise<void> {
    const st = stateOf(detailId)
    const seq = ++st.flight
    st.submitting = true
    st.lastError = ''
    try {
      await receiveInbound(orderId, { detail_id: detailId, ...data }, operationKey)
      endOperation(detailId, operationKey)
      st.pending = null
    } catch (error) {
      const outcome = classify(error)
      if (st.flight === seq) {
        st.lastError = errorMessage(error)
      }
      if (outcome === 'rejected') {
        endOperation(detailId, operationKey)
        st.pending = null
      } else if (outcome === 'conflict') {
        markUncertain(detailId)
        const stored = findOp(detailId)
        st.pending = stored
          ? toPendingInfo(stored, false)
          : { operationKey, businessPayload: data, createdAt: new Date().toISOString(), stale: false }
        st.blocked = true
        st.blockedReason = '该请求编号已被用于其他内容，请核对原收货操作后放弃重试'
      } else {
        markUncertain(detailId)
        const stored = findOp(detailId)
        st.pending = stored
          ? toPendingInfo(stored, isStale(stored))
          : { operationKey, businessPayload: data, createdAt: new Date().toISOString(), stale: false }
      }
      throw error
    } finally {
      if (st.flight === seq) st.submitting = false
    }
  }

  /** 打开弹窗时恢复这批明细的待确认操作；返回存储层错误（如有）。 */
  function restoreForDetails(detailIds: EntityID[]): string {
    knownDetailIds = detailIds
    for (const id of detailIds) {
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
    const idSet = new Set(detailIds.map(String))
    for (const item of loaded.items) {
      if (!idSet.has(String(item.objectId)) || item.scope !== RECEIVE_SCOPE) continue
      stateOf(item.objectId as EntityID).pending = toPendingInfo(item, isStale(item))
    }
    return ''
  }

  // 其他标签页写入存储时，本地横幅同步绑定最新状态；这不是跨标签页互斥。
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', () => {
      if (knownDetailIds.length > 0) restoreForDetails(knownDetailIds)
    })
  }

  return {
    stateOf,
    storageError,
    receive,
    recoverReceive,
    discardReceive,
    restoreForDetails,
  }
}