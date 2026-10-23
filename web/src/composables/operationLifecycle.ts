import { getCurrentScope, onScopeDispose } from 'vue'
import type { EntityID } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import {
  PENDING_SCHEMA_VERSION, removePending, upsertPending,
  type OperationIdentity, type PendingOperation,
} from '@/utils/pendingOperations'

// Keep per-command policies separate: a rejection safe for one command may be
// uncertain for another. These lists reflect the existing server contracts.
const safeRejectCodes = {
  'warehouse.closure': new Set([400, 43101, 43102, 43103, 50402, 50408, 30001, 30201, 30202, 30205, 20001, 20002, 20005, 20011, 20012, 20014, 40006, 40007, 50001, 50002, 50301, 50302]),
  'inventory.move': new Set([400, 30001, 30010, 30011]),
  'inbound.receive': new Set([400, 40001, 40002, 40004, 40008, 40014]),
  'inbound.putaway': new Set([400, 40001, 40002, 40005, 40006, 40007, 40008, 20002, 20005, 20011, 20014]),
  'stocktake.approve': new Set([400, 60001, 60002, 60004, 60006]),
  'outbound.pick': new Set([400, 40006, 40018, 40019, 50008, 50009, 50010, 50011, 50408]),
  'outbound.claim': new Set([400, 40006, 40007, 40017, 50408]),
  'outbound.wave': new Set([400, 50401, 50402, 50403, 50404, 50405, 50406, 50407, 50408, 50409]),
} as const

export type OperationScope = keyof typeof safeRejectCodes
export type OperationErrorOutcome = 'uncertain' | 'rejected' | 'conflict'
export interface OperationContext { identity: OperationIdentity; storage: Storage }

/** Re-read identity on every use; never capture an account across an await. */
export function getOperationContext(action: string): { ctx: OperationContext | null; error: string } {
  const auth = useAuthStore()
  if (!auth.token || !auth.user?.user_id) {
    return { ctx: null, error: `登录状态无效，无法进行${action}操作` }
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

/** Business states remain in their owning composable, with one identity rule. */
export function operationStateKey(objectId: EntityID): string {
  const auth = useAuthStore()
  return JSON.stringify([auth.user?.user_id, auth.demoSessionId, String(objectId)])
}

/** Persist prepared → sending before the caller may issue a business request. */
export function prepareOperation(ctx: OperationContext, scope: OperationScope, objectId: EntityID,
  operationKey: string, businessPayload: Record<string, unknown> | null): string {
  const base: PendingOperation = {
    schemaVersion: PENDING_SCHEMA_VERSION,
    userId: ctx.identity.userId,
    sessionNamespace: ctx.identity.sessionNamespace,
    scope, objectId, operationKey, businessPayload,
    createdAt: new Date().toISOString(),
    state: 'prepared',
  }
  const prepared = upsertPending(ctx.identity, ctx.storage, base)
  if (!prepared.ok) return prepared.error
  const sending = upsertPending(ctx.identity, ctx.storage, { ...base, state: 'sending' })
  if (!sending.ok) {
    removePending(ctx.identity, ctx.storage, scope, objectId, operationKey)
    return sending.error
  }
  return ''
}

/** Only a command's explicit safe rejection permits clearing its pending key. */
export function classifyOperationError(error: unknown, scope: OperationScope): OperationErrorOutcome {
  if (!error || typeof error !== 'object') return 'uncertain'
  const api = error as { code?: unknown; status?: unknown; kind?: unknown }
  if (api.code === 40901) return 'conflict'
  if (api.kind === 'network') return 'uncertain'
  if (api.status === 401 || api.status === 403) return 'uncertain'
  if (typeof api.status === 'number' && api.status >= 500) return 'uncertain'
  if (typeof api.code === 'number' && safeRejectCodes[scope].has(api.code)) return 'rejected'
  return 'uncertain'
}

export function operationErrorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object' && typeof (error as Pick<Error, 'message'>).message === 'string') {
    return (error as Pick<Error, 'message'>).message
  }
  return fallback
}

/** Storage synchronization is scoped to the owner; it does not provide a lock. */
export function onOperationStorageChange(callback: () => void): void {
  if (typeof window === 'undefined' || !getCurrentScope()) return
  window.addEventListener('storage', callback)
  onScopeDispose(() => window.removeEventListener('storage', callback))
}
