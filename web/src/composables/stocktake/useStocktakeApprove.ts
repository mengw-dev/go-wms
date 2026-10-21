import { reactive, ref } from 'vue'
import {
  classifyOperationError, getOperationContext, onOperationStorageChange,
  operationErrorMessage, operationStateKey, prepareOperation,
} from '@/composables/operationLifecycle'
import { approveStocktakeOrder } from '@/api/stocktake'
import type { EntityID } from '@/api/types'
import {
  findPending,
  isStale,
  loadPending,
  removePending,
  upsertPending,
  type PendingOperation,
} from '@/utils/pendingOperations'

/**
 * 盘点审核操作生命周期（复用 utils/pendingOperations，与拣货/收货/上架同一套规则）。
 * 审核接口没有业务请求体（业务参数为空对象），同 key 即同内容：
 * 存在未确认记录时点击审核会自动按原 key 重试，成功才生成下一个 key。
 *
 * objectId = 盘点单 ID。存储读写非原子，不提供跨标签页互斥。
 */
export function useStocktakeApprove() {
  const APPROVE_SCOPE = 'stocktake.approve'

  interface PendingApproveInfo {
    operationKey: string
    createdAt: string
    stale: boolean
  }

  interface OrderState {
    submitting: boolean
    flight: number
    pending: PendingApproveInfo | null
    blocked: boolean
    blockedReason: string
    lastError: string
  }

  const states = reactive(new Map<string, OrderState>())
  const storageError = ref('')
  let knownOrderIds: EntityID[] = []

  const currentContext = () => getOperationContext('审核')

  function stateOf(orderId: EntityID): OrderState {
    const key = operationStateKey(orderId)
    let st = states.get(key)
    if (!st) {
      st = { submitting: false, flight: 0, pending: null, blocked: false, blockedReason: '', lastError: '' }
      states.set(key, st)
    }
    return st
  }

  function findOp(orderId: EntityID): PendingOperation | null {
    const c = currentContext()
    if (!c.ctx) return null
    const loaded = loadPending(c.ctx.identity, c.ctx.storage)
    if (loaded.error) {
      storageError.value = loaded.error
      throw new Error(loaded.error)
    }
    return findPending(c.ctx.identity, c.ctx.storage, APPROVE_SCOPE, orderId)
  }

  function endOperation(orderId: EntityID, operationKey: string) {
    const c = currentContext()
    if (!c.ctx) return
    const removed = removePending(c.ctx.identity, c.ctx.storage, APPROVE_SCOPE, orderId, operationKey)
    if (!removed.ok) storageError.value = removed.error
  }

  function markUncertain(orderId: EntityID) {
    const c = currentContext()
    if (!c.ctx) return
    const existing = findPending(c.ctx.identity, c.ctx.storage, APPROVE_SCOPE, orderId)
    if (!existing) return
    const saved = upsertPending(c.ctx.identity, c.ctx.storage, { ...existing, state: 'uncertain' })
    if (!saved.ok) storageError.value = saved.error
  }

  function toPendingInfo(operation: PendingOperation, stale: boolean): PendingApproveInfo {
    return { operationKey: operation.operationKey, createdAt: operation.createdAt, stale }
  }

  /** 审核：存在未确认记录则自动按原 key 重试（无业务参数，原 key 即原内容）；超窗口拒绝发送。 */
  async function approve(orderId: EntityID): Promise<void> {
    const st = stateOf(orderId)
    if (st.submitting) {
      throw new Error('该盘点单的审核请求正在进行，请勿重复提交')
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
    const existing = findOp(orderId)
    if (existing) {
      if (isStale(existing)) {
        st.pending = toPendingInfo(existing, true)
        st.lastError = '该盘点单的上次审核已超过保留期，请核对待确认记录后再操作'
        throw new Error(st.lastError)
      }
      return await sendApprove(orderId, existing.operationKey)
    }
    const operationKey = crypto.randomUUID()
    const saveError = prepareOperation(c.ctx, APPROVE_SCOPE, orderId, operationKey, {})
    if (saveError) {
      storageError.value = saveError
      st.lastError = saveError
      throw new Error(saveError)
    }
    return await sendApprove(orderId, operationKey)
  }

  /** 核对后放弃未确认的审核记录：仅删除本地记录，不向服务端发送任何请求。 */
  function discard(orderId: EntityID) {
    const st = stateOf(orderId)
    if (st.submitting) return
    const existing = findOp(orderId)
    if (!existing) {
      st.pending = null
      return
    }
    const c = currentContext()
    if (!c.ctx) return
    const removed = removePending(c.ctx.identity, c.ctx.storage, APPROVE_SCOPE, orderId, existing.operationKey)
    if (!removed.ok) {
      storageError.value = removed.error
      st.lastError = removed.error
      return
    }
    st.pending = null
    st.blocked = false
    st.blockedReason = ''
  }

  async function sendApprove(orderId: EntityID, operationKey: string): Promise<void> {
    const st = stateOf(orderId)
    const seq = ++st.flight
    st.submitting = true
    st.lastError = ''
    try {
      await approveStocktakeOrder(orderId, operationKey)
      if (stateOf(orderId) !== st) throw new Error('登录身份已变更，请在原账号核对该操作')
      endOperation(orderId, operationKey)
      st.pending = null
    } catch (error) {
      if (stateOf(orderId) !== st) throw error
      const outcome = classifyOperationError(error, APPROVE_SCOPE)
      if (st.flight === seq) {
        st.lastError = operationErrorMessage(error, '审核失败')
      }
      if (outcome === 'rejected') {
        endOperation(orderId, operationKey)
        st.pending = null
      } else if (outcome === 'conflict') {
        markUncertain(orderId)
        const stored = findOp(orderId)
        st.pending = stored
          ? toPendingInfo(stored, false)
          : { operationKey, createdAt: new Date().toISOString(), stale: false }
        st.blocked = true
        st.blockedReason = '该请求编号已被用于其他盘点单，请核对原审核操作后放弃重试'
      } else {
        markUncertain(orderId)
        const stored = findOp(orderId)
        st.pending = stored
          ? toPendingInfo(stored, isStale(stored))
          : { operationKey, createdAt: new Date().toISOString(), stale: false }
      }
      throw error
    } finally {
      if (st.flight === seq) st.submitting = false
    }
  }

  /** 进入列表/详情时恢复待确认操作；返回存储层错误（如有）。 */
  function restoreForOrders(orderIds: EntityID[]): string {
    knownOrderIds = orderIds
    for (const id of orderIds) {
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
    const idSet = new Set(orderIds.map(String))
    for (const item of loaded.items) {
      if (!idSet.has(String(item.objectId)) || item.scope !== APPROVE_SCOPE) continue
      stateOf(item.objectId as EntityID).pending = toPendingInfo(item, isStale(item))
    }
    return ''
  }

  // 其他标签页写入存储时，本地状态同步绑定最新记录；这不是跨标签页互斥。
  onOperationStorageChange(() => {
    if (knownOrderIds.length > 0) restoreForOrders(knownOrderIds)
  })

  return {
    stateOf,
    storageError,
    approve,
    discard,
    restoreForOrders,
  }
}