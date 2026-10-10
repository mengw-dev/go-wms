/**
 * 待确认操作持久化（前端幂等第 1 层：操作 key 生命周期）。
 *
 * 职责：把「一次真实操作」的 key + 原始业务参数 + 状态落盘，
 * 使刷新、关闭重开、断网重试都能找回同一操作身份，避免重新生成 key 造成重复执行。
 *
 * 明确声明的能力边界：
 * - localStorage/sessionStorage 的读改写不是原子操作，本模块不提供跨标签页互斥；
 *   多标签页并发操作同一任务时，各自可能创建自己的记录，最终由服务端
 *   wms_idempotency 的 key 级去重兜底，不能宣称已解决跨标签页重复操作。
 * - 前端登录响应不含 tenant_id，记录暂无法填写租户，仅按 user_id +
 *   sessionNamespace 隔离；跨租户同 key 的隔离由服务端唯一键保证。
 * - 服务端幂等记录保留 7 天；超过保留期的待确认操作不允许自动重发，只能核对后处理。
 */
import type { EntityID } from '@/api/types'

/** 记录与容器版本：不兼容即视为损坏，禁止静默丢弃。 */
export const PENDING_SCHEMA_VERSION = 1
export const PENDING_CONTAINER_VERSION = 1

/** 与服务端幂等记录保留期对齐的窗口（毫秒）。 */
export const PENDING_RETENTION_MS = 7 * 24 * 60 * 60 * 1000

const STORAGE_KEY = 'WMS_PENDING_OPS'

/** 操作状态：succeeded/rejected 为终态，确认后即删除记录；prepared/sending 刷新后按 uncertain 恢复。 */
export type PendingState = 'prepared' | 'sending' | 'uncertain' | 'succeeded' | 'rejected'

/**
 * 一条待确认操作。ID 全部保持字符串，不转 JS number；
 * 不保存 JWT、密码等凭证；businessPayload 只放影响后端指纹的业务参数。
 */
export interface PendingOperation {
  schemaVersion: number
  userId: EntityID
  /** 前端登录响应未返回租户信息，暂不填充；隔离由 user_id 承担。 */
  tenantId?: EntityID
  /** 演示账号隔离不同访客会话；普通账号为空字符串。 */
  sessionNamespace: string
  /** 与后端 wms_idempotency.scope 一致，如 outbound.pick / outbound.claim。 */
  scope: string
  /** 业务对象 ID（任务/单据等），字符串。 */
  objectId: EntityID
  operationKey: string
  businessPayload: Record<string, unknown> | null
  createdAt: string
  state: PendingState
}

/** 待确认操作所属身份。 */
export interface OperationIdentity {
  userId: EntityID
  sessionNamespace: string
  tenantId?: EntityID
}

interface Container {
  version: number
  items: PendingOperation[]
}

export interface PendingLoadResult {
  items: PendingOperation[]
  /** 非空表示存储损坏/版本不支持/不可用，调用方必须阻止新建操作。 */
  error: string | null
}

export interface PendingWriteResult {
  ok: boolean
  error: string
}

const PENDING_STATES: readonly PendingState[] = ['prepared', 'sending', 'uncertain', 'succeeded', 'rejected']

function emptyContainer(): Container {
  return { version: PENDING_CONTAINER_VERSION, items: [] }
}

/** 运行时校验单条记录：不依赖 TypeScript 类型断言，字段非法视整份存储为损坏。 */
function isValidOperation(raw: unknown): raw is PendingOperation {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return false
  const item = raw as Record<string, unknown>
  if (item.schemaVersion !== PENDING_SCHEMA_VERSION) return false
  if (typeof item.userId !== 'string' || !item.userId) return false
  if (typeof item.sessionNamespace !== 'string') return false
  if (item.tenantId !== undefined && typeof item.tenantId !== 'string') return false
  if (typeof item.scope !== 'string' || !item.scope) return false
  if (typeof item.objectId !== 'string' || !item.objectId) return false
  if (typeof item.operationKey !== 'string' || !item.operationKey) return false
  if (item.businessPayload !== null && (typeof item.businessPayload !== 'object' || Array.isArray(item.businessPayload))) {
    return false
  }
  if (typeof item.createdAt !== 'string' || Number.isNaN(Date.parse(item.createdAt))) return false
  if (typeof item.state !== 'string' || !PENDING_STATES.includes(item.state as PendingState)) return false
  return true
}

/** 读取并校验整份存储；任何异常都返回明确 error，调用方不得静默继续。 */
function readContainer(storage: Storage): { container: Container | null; error: string } {
  let raw: string | null
  try {
    raw = storage.getItem(STORAGE_KEY)
  } catch {
    return { container: null, error: '浏览器存储不可用，无法记录待确认操作，已暂停提交' }
  }
  if (!raw) return { container: emptyContainer(), error: '' }
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return { container: null, error: '待确认操作记录已损坏，为避免重复操作已暂停提交，请核对任务进度后清理' }
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return { container: null, error: '待确认操作记录已损坏，为避免重复操作已暂停提交，请核对任务进度后清理' }
  }
  const body = parsed as Record<string, unknown>
  if (body.version !== PENDING_CONTAINER_VERSION) {
    return { container: null, error: '待确认操作记录版本不支持，为避免重复操作已暂停提交，请核对任务进度后清理' }
  }
  if (!Array.isArray(body.items) || !body.items.every(isValidOperation)) {
    return { container: null, error: '待确认操作记录存在损坏条目，为避免重复操作已暂停提交，请核对任务进度后清理' }
  }
  return { container: { version: body.version, items: body.items as PendingOperation[] }, error: '' }
}

function writeContainer(storage: Storage, container: Container): PendingWriteResult {
  try {
    storage.setItem(STORAGE_KEY, JSON.stringify(container))
    return { ok: true, error: '' }
  } catch {
    return { ok: false, error: '无法保存待确认操作记录（存储空间不足或不可写），本次操作未发送' }
  }
}

function sameIdentity(identity: OperationIdentity, item: PendingOperation): boolean {
  return item.userId === identity.userId && item.sessionNamespace === identity.sessionNamespace
}

/**
 * 读取当前身份的待确认操作。
 * 刷新恢复语义：persisted 的 prepared/sending 一律按 uncertain 返回并回写，
 * 不能把「已落盘未确认」当成「未发送」。
 */
export function loadPending(identity: OperationIdentity, storage: Storage): PendingLoadResult {
  const read = readContainer(storage)
  if (!read.container) return { items: [], error: read.error }
  const mine = read.container.items.filter((item) => sameIdentity(identity, item))
  let changed = false
  const normalized = mine.map((item) => {
    if (item.state === 'prepared' || item.state === 'sending') {
      changed = true
      return { ...item, state: 'uncertain' as const }
    }
    return item
  })
  if (changed) {
    const merged = read.container.items.map((item) => {
      if (sameIdentity(identity, item) && (item.state === 'prepared' || item.state === 'sending')) {
        return { ...item, state: 'uncertain' as const }
      }
      return item
    })
    const written = writeContainer(storage, { ...read.container, items: merged })
    if (!written.ok) return { items: normalized, error: written.error }
  }
  return { items: normalized, error: '' }
}

/** 查找 (scope, objectId) 下最新的待确认操作；存储异常时返回 null（调用方另行检查 storageError）。 */
export function findPending(identity: OperationIdentity, storage: Storage, scope: string, objectId: EntityID): PendingOperation | null {
  const read = readContainer(storage)
  if (!read.container) return null
  return findIn(read.container.items, identity, scope, objectId)
}

function findIn(items: PendingOperation[], identity: OperationIdentity, scope: string, objectId: EntityID): PendingOperation | null {
  let found: PendingOperation | null = null
  for (const item of items) {
    if (!sameIdentity(identity, item) || item.scope !== scope || item.objectId !== objectId) continue
    if (!found || item.createdAt > found.createdAt) found = item
  }
  return found
}

/** 写入（创建）或按 key 更新同一条操作；不会覆盖同对象下其他 key 的记录。 */
export function upsertPending(identity: OperationIdentity, storage: Storage, operation: PendingOperation): PendingWriteResult {
  const read = readContainer(storage)
  if (!read.container) return { ok: false, error: read.error }
  const items = read.container.items.filter(
    (item) => !(sameIdentity(identity, item) && item.scope === operation.scope && item.objectId === operation.objectId && item.operationKey === operation.operationKey),
  )
  items.push(operation)
  return writeContainer(storage, { ...read.container, items })
}

/** 删除指定操作的记录（确认成功或确认未执行后调用）。 */
export function removePending(identity: OperationIdentity, storage: Storage, scope: string, objectId: EntityID, operationKey: string): PendingWriteResult {
  const read = readContainer(storage)
  if (!read.container) return { ok: false, error: read.error }
  const items = read.container.items.filter(
    (item) => !(sameIdentity(identity, item) && item.scope === scope && item.objectId === objectId && item.operationKey === operationKey),
  )
  return writeContainer(storage, { ...read.container, items })
}

/** 是否超过服务端幂等保留窗口：超期操作不允许自动重发，只能核对后处理。 */
export function isStale(operation: Pick<PendingOperation, 'createdAt'>, now = Date.now()): boolean {
  const createdAt = Date.parse(operation.createdAt)
  if (Number.isNaN(createdAt)) return true
  return now - createdAt > PENDING_RETENTION_MS
}