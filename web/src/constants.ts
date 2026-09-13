export type TagType = 'primary' | 'success' | 'info' | 'warning' | 'danger'

/** 单据/任务/导入状态 → el-tag 颜色 */
const STATUS_TAG_MAP: Record<string, TagType> = {
  DRAFT: 'info',
  SUBMITTED: 'warning',
  APPROVED: 'warning',
  RECEIVING: 'primary',
  PUTAWAY: 'primary',
  PICKING: 'primary',
  IN_PROGRESS: 'primary',
  SHIPPED: 'success',
  COMPLETED: 'success',
  SUCCESS: 'success',
  PICKED: 'success',
  CANCELLED: 'danger',
  FAILED: 'danger',
  ALLOCATED: 'warning',
  CREATED: 'info',
  PENDING: 'info',
  RUNNING: 'primary',
  PROCESSING: 'primary',
}

export function statusTag(status?: string | null): TagType {
  return STATUS_TAG_MAP[status ?? ''] ?? 'info'
}

/** 状态中文文案 */
const STATUS_TEXT_MAP: Record<string, string> = {
  DRAFT: '草稿',
  SUBMITTED: '已提交',
  APPROVED: '已审核',
  RECEIVING: '收货中',
  PUTAWAY: '上架中',
  COMPLETED: '已完成',
  CANCELLED: '已取消',
  PICKING: '拣货中',
  SHIPPED: '已发货',
  CREATED: '待执行',
  IN_PROGRESS: '进行中',
  PENDING: '等待中',
  RUNNING: '执行中',
  PROCESSING: '执行中',
  SUCCESS: '成功',
  FAILED: '失败',
  ALLOCATED: '已分配',
  PICKED: '已拣货',
  RECEIVE: '入库',
  ALLOCATE: '分配',
  SHIP: '发货',
  RELEASE: '释放',
  ADJUST: '调整',
}

export function statusText(status?: string | null): string {
  if (!status) return '-'
  return STATUS_TEXT_MAP[status] ?? status
}

/** 任务类型 */
export const TASK_TYPE_TEXT: Record<string, string> = {
  RECEIVE: '收货',
  PUTAWAY: '上架',
  PICK: '拣货',
}

export function taskTypeText(type?: string | null): string {
  if (!type) return '-'
  return TASK_TYPE_TEXT[type] ?? type
}

/** 通用启停状态 */
export const COMMON_STATUS = {
  DISABLED: 0,
  ENABLED: 1,
} as const

/** 库位状态 */
export const LOCATION_STATUS = {
  DISABLED: 0,
  IDLE: 1,
  OCCUPIED: 2,
} as const

export const LOCATION_STATUS_TEXT: Record<number, string> = {
  0: '禁用',
  1: '空闲',
  2: '占用',
}

export function locationStatusText(status?: number | null): string {
  if (status === null || status === undefined) return '-'
  return LOCATION_STATUS_TEXT[status] ?? String(status)
}

export function locationStatusTag(status?: number | null): TagType {
  switch (status) {
    case LOCATION_STATUS.IDLE:
      return 'success'
    case LOCATION_STATUS.OCCUPIED:
      return 'warning'
    case LOCATION_STATUS.DISABLED:
      return 'danger'
    default:
      return 'info'
  }
}

/** 入库单状态集合 */
export const INBOUND_STATUS_OPTIONS = [
  'DRAFT',
  'SUBMITTED',
  'APPROVED',
  'RECEIVING',
  'PUTAWAY',
  'COMPLETED',
  'CANCELLED',
]

/** 出库单状态集合 */
export const OUTBOUND_STATUS_OPTIONS = [
  'DRAFT',
  'SUBMITTED',
  'APPROVED',
  'PICKING',
  'SHIPPED',
  'CANCELLED',
]

/** 盘点单状态集合 */
export const STOCKTAKE_STATUS_OPTIONS = ['DRAFT', 'COMPLETED', 'CANCELLED']

/** 任务状态集合 */
export const TASK_STATUS_OPTIONS = ['CREATED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED']

/** 任务类型集合 */
export const TASK_TYPE_OPTIONS = ['RECEIVE', 'PUTAWAY', 'PICK']

/** 流水类型集合 */
export const TRANS_TYPE_OPTIONS = ['RECEIVE', 'ALLOCATE', 'SHIP', 'RELEASE', 'ADJUST']
