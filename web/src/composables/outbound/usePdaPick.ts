import { reactive, ref } from 'vue'
import { claimPdaTask, pickPdaTask } from '@/api/outbound'
import type { ApiError } from '@/api/request'
import type { ClaimResult, EntityID, PickParams, PickResult } from '@/api/types'
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
 * PDA 拣货闭环（领取 → 扫码 → 幂等提交 → 快照刷新）的共享逻辑。
 *
 * 操作生命周期（一次真实操作 = 一个新 key）：
 * - 提交前先落盘「key + 业务参数」，持久化失败不发送请求；
 * - 网络重试复用原 key、原参数；结果未确认期间禁止改参数作为新操作发送；
 * - 明确成功后删除记录，下一次真实操作即使参数相同也生成新 key；
 * - 明确未执行（服务端语义可确认的静态拒绝）才删除记录；
 * - 关闭弹窗/刷新不清记录，重开时按任务恢复待确认操作。
 *
 * 状态按任务隔离：凭证、租约、快照、key、pending 均绑定 task_id；
 * 每个任务维护请求序号，旧请求的晚返回只更新它自己的任务，不清新请求的 loading。
 */
export function usePdaPick() {
  /** 幂等 scope 与后端 wms_idempotency 保持一致。 */
  const CLAIM_SCOPE = 'outbound.claim'
  const PICK_SCOPE = 'outbound.pick'

  /** 拣货业务参数（不含 claim_token：换人接手/续领后重放仍按原参数命中指纹）。 */
  type PickBusinessPayload = Omit<PickParams, 'claim_token'>

  /** 恢复出的待确认操作信息（用于横幅展示与重试）。 */
  interface PendingClaimInfo {
    operationKey: string
    createdAt: string
    stale: boolean
  }
  interface PendingPickInfo {
    operationKey: string
    businessPayload: PickBusinessPayload
    createdAt: string
    stale: boolean
  }

  /** 单个任务的操作与展示状态。 */
  interface TaskPickState {
    claimToken: string
    leaseExpireAt: string
    snapshot: PickResult | null
    lastError: string
    claiming: boolean
    submitting: boolean
    claimFlight: number
    pickFlight: number
    pendingClaim: PendingClaimInfo | null
    pendingPick: PendingPickInfo | null
    blocked: boolean
    blockedReason: string
  }

  const states = reactive(new Map<string, TaskPickState>())
  /** 存储层异常（损坏/不可写）：未解决前禁止新建任何操作。 */
  const storageError = ref('')
  let knownTaskIds: EntityID[] = []

  function emptyState(): TaskPickState {
    return {
      claimToken: '',
      leaseExpireAt: '',
      snapshot: null,
      lastError: '',
      claiming: false,
      submitting: false,
      claimFlight: 0,
      pickFlight: 0,
      pendingClaim: null,
      pendingPick: null,
      blocked: false,
      blockedReason: '',
    }
  }

  /** 取（或创建）任务状态：所有展示字段都从这里读，保证切换任务即切换展示。 */
  function stateOf(taskId: EntityID): TaskPickState {
    const key = String(taskId)
    let st = states.get(key)
    if (!st) {
      st = emptyState()
      states.set(key, st)
    }
    return st
  }

  /** 当前登录身份与对应存储介质：演示账号用 sessionStorage（随会话失效），普通账号用 localStorage（支持关闭重开恢复）。 */
  function currentContext(): { ctx: { identity: OperationIdentity; storage: Storage } | null; error: string } {
    const auth = useAuthStore()
    if (!auth.token || !auth.user?.user_id) {
      return { ctx: null, error: '登录状态无效，无法进行拣货操作' }
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

  function findOp(taskId: EntityID, scope: string): PendingOperation | null {
    const c = currentContext()
    if (!c.ctx) return null
    return findPending(c.ctx.identity, c.ctx.storage, scope, taskId)
  }

  function endOperation(taskId: EntityID, operationKey: string, scope: string) {
    const c = currentContext()
    if (!c.ctx) return
    const removed = removePending(c.ctx.identity, c.ctx.storage, scope, taskId, operationKey)
    if (!removed.ok) storageError.value = removed.error
  }

  function markUncertain(taskId: EntityID, scope: string) {
    const c = currentContext()
    if (!c.ctx) return
    const existing = findPending(c.ctx.identity, c.ctx.storage, scope, taskId)
    if (!existing) return
    const saved = upsertPending(c.ctx.identity, c.ctx.storage, { ...existing, state: 'uncertain' })
    if (!saved.ok) storageError.value = saved.error
  }

  /** 新建操作：先落盘（prepared → sending），任一步失败都不发送请求。 */
  function beginOperation(ctx: { identity: OperationIdentity; storage: Storage }, scope: string, taskId: EntityID, operationKey: string, businessPayload: Record<string, unknown> | null): string {
    const base: PendingOperation = {
      schemaVersion: PENDING_SCHEMA_VERSION,
      userId: ctx.identity.userId,
      sessionNamespace: ctx.identity.sessionNamespace,
      scope,
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
      removePending(ctx.identity, ctx.storage, scope, taskId, operationKey)
      return sending.error
    }
    return ''
  }

  /**
   * 错误分类：服务端语义可以确认「本次内容没有执行任何写入」才进入 rejected；
   * 401/403、5xx、网络错误、以及可能与同 key 并发成功竞争的状态类/数量类错误，
   * 一律按 uncertain 保留原 key 原参数，绝不借错误分类替用户删记录。
   */
  type ErrorOutcome = 'uncertain' | 'rejected' | 'conflict'

  /** 拣货：静态校验类错误码，这些校验在执行任何写入之前、且不受并发提交影响。 */
  const PICK_SAFE_REJECT_CODES = new Set([400, 40006, 40018, 40019, 50008, 50009, 50010, 50011])
  /** 领取：参数/任务不存在/任务状态/已被他人领取，均发生在写入之前。 */
  const CLAIM_SAFE_REJECT_CODES = new Set([400, 40006, 40007, 40017])

  function classify(error: unknown, safeCodes: Set<number>): ErrorOutcome {
    if (!error || typeof error !== 'object') return 'uncertain'
    const api = error as { code?: unknown; status?: unknown; kind?: unknown; message?: unknown }
    if (api.code === 40901) return 'conflict'
    if (api.kind === 'network') return 'uncertain'
    if (api.status === 401 || api.status === 403) return 'uncertain'
    if (typeof api.status === 'number' && api.status >= 500) return 'uncertain'
    if (typeof api.code === 'number' && safeCodes.has(api.code)) return 'rejected'
    return 'uncertain'
  }

  function errorMessage(error: unknown): string {
    if (error && typeof error === 'object' && typeof (error as Pick<ApiError, 'message'>).message === 'string') {
      return (error as Pick<ApiError, 'message'>).message
    }
    return '操作失败'
  }

  function samePickPayload(a: unknown, b: PickBusinessPayload): boolean {
    if (!a || typeof a !== 'object' || Array.isArray(a)) return false
    const pa = a as Record<string, unknown>
    if (typeof pa.qty !== 'number' || !Number.isFinite(pa.qty)) return false
    if (b.qty !== pa.qty) return false
    const norm = (v: unknown) => (typeof v === 'string' ? v.trim().toLowerCase() : '')
    return norm(pa.location_code) === norm(b.location_code ?? '') && norm(pa.batch_no) === norm(b.batch_no ?? '')
  }

  function toPickInfo(operation: PendingOperation, stale: boolean): PendingPickInfo {
    return {
      operationKey: operation.operationKey,
      businessPayload: (operation.businessPayload as PickBusinessPayload) ?? { qty: 0 },
      createdAt: operation.createdAt,
      stale,
    }
  }

  /** 本次任务待恢复的领取信息补齐（不确定结果时保留原 key）。 */
  function pendingClaimInfo(taskId: EntityID, fallbackKey: string, fallbackCreatedAt: string): PendingClaimInfo {
    const stored = findOp(taskId, CLAIM_SCOPE)
    return {
      operationKey: stored?.operationKey ?? fallbackKey,
      createdAt: stored?.createdAt ?? fallbackCreatedAt,
      stale: stored ? isStale(stored) : false,
    }
  }

  /** 领取（续领）任务租约：有未确认记录则原 key 重试，否则创建新 key。 */
  async function claim(taskId: EntityID): Promise<boolean> {
    const st = stateOf(taskId)
    if (st.claiming || st.submitting) return false
    const c = currentContext()
    if (!c.ctx) {
      st.lastError = c.error
      return false
    }
    if (storageError.value) {
      st.lastError = storageError.value
      return false
    }
    if (st.blocked) {
      st.lastError = st.blockedReason
      return false
    }
    const existing = findOp(taskId, CLAIM_SCOPE)
    if (existing) {
      if (isStale(existing)) {
        st.pendingClaim = { operationKey: existing.operationKey, createdAt: existing.createdAt, stale: true }
        st.lastError = '领取操作已超过保留期，请核对任务是否已被领取后再操作'
        return false
      }
      return await sendClaim(taskId, existing.operationKey)
    }
    const operationKey = crypto.randomUUID()
    const saveError = beginOperation(c.ctx, CLAIM_SCOPE, taskId, operationKey, {})
    if (saveError) {
      storageError.value = saveError
      st.lastError = saveError
      return false
    }
    return await sendClaim(taskId, operationKey)
  }

  /** 恢复未确认的领取操作（原 key 原内容重试）。 */
  async function recoverClaim(taskId: EntityID): Promise<boolean> {
    const st = stateOf(taskId)
    const existing = findOp(taskId, CLAIM_SCOPE)
    if (!existing) {
      st.pendingClaim = null
      st.lastError = '没有待恢复的领取操作'
      return false
    }
    if (isStale(existing)) {
      st.pendingClaim = { operationKey: existing.operationKey, createdAt: existing.createdAt, stale: true }
      st.lastError = '领取操作已超过保留期，请核对任务是否已被领取后再操作'
      return false
    }
    return await sendClaim(taskId, existing.operationKey)
  }

  /** 核对后放弃未确认的领取操作：仅删除本地记录，不向服务端发送任何请求。 */
  function discardClaim(taskId: EntityID) {
    const st = stateOf(taskId)
    const existing = findOp(taskId, CLAIM_SCOPE)
    if (!existing) {
      st.pendingClaim = null
      return
    }
    const c = currentContext()
    if (!c.ctx) return
    const removed = removePending(c.ctx.identity, c.ctx.storage, CLAIM_SCOPE, taskId, existing.operationKey)
    if (!removed.ok) {
      storageError.value = removed.error
      st.lastError = removed.error
      return
    }
    st.pendingClaim = null
    st.blocked = false
    st.blockedReason = ''
  }

  async function sendClaim(taskId: EntityID, operationKey: string): Promise<boolean> {
    const st = stateOf(taskId)
    const seq = ++st.claimFlight
    st.claiming = true
    st.lastError = ''
    try {
      const result: ClaimResult = await claimPdaTask(taskId, operationKey)
      if (st.claimFlight === seq) {
        st.claimToken = result.claim_token
        st.leaseExpireAt = result.lease_expire_at
        st.snapshot = toPickResult(result)
      }
      endOperation(taskId, operationKey, CLAIM_SCOPE)
      st.pendingClaim = null
      return true
    } catch (error) {
      const outcome = classify(error, CLAIM_SAFE_REJECT_CODES)
      const message = errorMessage(error)
      if (st.claimFlight === seq) {
        const data = (error as { data?: unknown } | null)?.data
        if (data && typeof data === 'object') st.snapshot = data as PickResult
        st.lastError = message
      }
      if (outcome === 'rejected') {
        endOperation(taskId, operationKey, CLAIM_SCOPE)
        st.pendingClaim = null
      } else if (outcome === 'conflict') {
        markUncertain(taskId, CLAIM_SCOPE)
        st.pendingClaim = pendingClaimInfo(taskId, operationKey, new Date().toISOString())
        st.blocked = true
        st.blockedReason = '该请求编号已被用于不同内容，请核对原领取操作后放弃重试'
      } else {
        markUncertain(taskId, CLAIM_SCOPE)
        st.pendingClaim = pendingClaimInfo(taskId, operationKey, new Date().toISOString())
      }
      return false
    } finally {
      if (st.claimFlight === seq) st.claiming = false
    }
  }

  /**
   * 提交拣货：
   * - 存在未确认记录且参数一致 → 原 key 原参数重试；
   * - 参数不一致 / 已超保留期 → 拒绝发送并提示核对，绝不换 key 重发；
   * - 无记录 → 新 key 落盘后发送。
   */
  async function pick(taskId: EntityID, data: PickBusinessPayload): Promise<PickResult> {
    const st = stateOf(taskId)
    if (st.claiming || st.submitting) {
      throw new Error('操作正在进行中，请勿重复提交')
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
    const existing = findOp(taskId, PICK_SCOPE)
    if (existing) {
      if (isStale(existing)) {
        st.pendingPick = toPickInfo(existing, true)
        st.lastError = '上次拣货已超过保留期，请核对任务进度后再操作'
        throw new Error(st.lastError)
      }
      if (!samePickPayload(existing.businessPayload, data)) {
        st.pendingPick = toPickInfo(existing, false)
        st.lastError = '该任务存在未确认的拣货操作，请先恢复重试或核对后放弃，不能更改数量后直接提交'
        throw new Error(st.lastError)
      }
      return await sendPick(taskId, existing.operationKey, data)
    }
    const operationKey = crypto.randomUUID()
    const saveError = beginOperation(c.ctx, PICK_SCOPE, taskId, operationKey, data as Record<string, unknown>)
    if (saveError) {
      storageError.value = saveError
      st.lastError = saveError
      throw new Error(saveError)
    }
    return await sendPick(taskId, operationKey, data)
  }

  /** 恢复未确认的拣货操作（原 key 原参数重试）。 */
  async function recoverPick(taskId: EntityID): Promise<PickResult> {
    const st = stateOf(taskId)
    const existing = findOp(taskId, PICK_SCOPE)
    if (!existing) {
      st.pendingPick = null
      st.lastError = '没有待恢复的拣货操作'
      throw new Error(st.lastError)
    }
    if (isStale(existing)) {
      st.pendingPick = toPickInfo(existing, true)
      st.lastError = '上次拣货已超过保留期，请核对任务进度后再操作'
      throw new Error(st.lastError)
    }
    return await sendPick(taskId, existing.operationKey, existing.businessPayload as PickBusinessPayload)
  }

  /** 核对后放弃未确认的拣货操作：仅删除本地记录，不向服务端发送任何请求。 */
  function discardPick(taskId: EntityID) {
    const st = stateOf(taskId)
    const existing = findOp(taskId, PICK_SCOPE)
    if (!existing) {
      st.pendingPick = null
      return
    }
    const c = currentContext()
    if (!c.ctx) return
    const removed = removePending(c.ctx.identity, c.ctx.storage, PICK_SCOPE, taskId, existing.operationKey)
    if (!removed.ok) {
      storageError.value = removed.error
      st.lastError = removed.error
      return
    }
    st.pendingPick = null
    st.blocked = false
    st.blockedReason = ''
  }

  async function sendPick(taskId: EntityID, operationKey: string, data: PickBusinessPayload): Promise<PickResult> {
    const st = stateOf(taskId)
    const seq = ++st.pickFlight
    st.submitting = true
    st.lastError = ''
    try {
      const result = await pickPdaTask(taskId, { ...data, claim_token: st.claimToken }, operationKey)
      if (st.pickFlight === seq) {
        st.snapshot = result
        st.lastError = ''
      }
      endOperation(taskId, operationKey, PICK_SCOPE)
      st.pendingPick = null
      return result
    } catch (error) {
      const outcome = classify(error, PICK_SAFE_REJECT_CODES)
      const message = errorMessage(error)
      if (st.pickFlight === seq) {
        const dataPayload = (error as { data?: unknown } | null)?.data
        if (dataPayload && typeof dataPayload === 'object') st.snapshot = dataPayload as PickResult
        st.lastError = message
      }
      if (outcome === 'rejected') {
        endOperation(taskId, operationKey, PICK_SCOPE)
        st.pendingPick = null
        // 凭证失效类拒绝：确认旧凭证不可用，清掉本地凭证让界面回到「重新领取」，
        // 重新领取会按新操作生成新的领取 key。
        const code = (error as { code?: unknown } | null)?.code
        if (code === 40018 || code === 40019) {
          st.claimToken = ''
          st.leaseExpireAt = ''
        }
      } else if (outcome === 'conflict') {
        markUncertain(taskId, PICK_SCOPE)
        const stored = findOp(taskId, PICK_SCOPE)
        st.pendingPick = stored
          ? toPickInfo(stored, false)
          : { operationKey, businessPayload: data, createdAt: new Date().toISOString(), stale: false }
        st.blocked = true
        st.blockedReason = '该请求编号已被用于其他内容，请核对原拣货操作后放弃重试'
      } else {
        markUncertain(taskId, PICK_SCOPE)
        const stored = findOp(taskId, PICK_SCOPE)
        st.pendingPick = stored
          ? toPickInfo(stored, isStale(stored))
          : { operationKey, businessPayload: data, createdAt: new Date().toISOString(), stale: false }
      }
      throw error
    } finally {
      if (st.pickFlight === seq) st.submitting = false
    }
  }

  /**
   * 打开弹窗/进入任务时恢复待确认操作；
   * 只绑定展示标记（pendingClaim/pendingPick），不触碰进行中的请求状态。
   * 返回存储层错误（如有），调用方必须阻止后续操作并提示。
   */
  function restoreForTasks(taskIds: EntityID[]): string {
    knownTaskIds = taskIds
    for (const id of taskIds) {
      const st = states.get(String(id))
      if (st) {
        st.pendingClaim = null
        st.pendingPick = null
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
      if (!idSet.has(String(item.objectId))) continue
      const st = stateOf(item.objectId as EntityID)
      const stale = isStale(item)
      if (item.scope === PICK_SCOPE) {
        st.pendingPick = toPickInfo(item, stale)
      } else if (item.scope === CLAIM_SCOPE) {
        st.pendingClaim = { operationKey: item.operationKey, createdAt: item.createdAt, stale }
      }
    }
    return ''
  }

  // 其他标签页写入存储时，本地横幅同步绑定最新状态；
  // 这不是跨标签页互斥，两个标签页仍然可能各自发送请求，最终由服务端 key 级去重兜底。
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', () => {
      if (knownTaskIds.length > 0) restoreForTasks(knownTaskIds)
    })
  }

  return {
    stateOf,
    storageError,
    claim,
    recoverClaim,
    discardClaim,
    pick,
    recoverPick,
    discardPick,
    restoreForTasks,
  }
}

/** 领取响应在 ClaimResult 上展开同一份快照字段，转成 PickResult 供界面统一消费。 */
function toPickResult(result: ClaimResult): PickResult {
  return {
    task_status: result.task_status,
    done_qty: result.done_qty,
    remaining_qty: result.remaining_qty,
    order_status: result.order_status,
  }
}