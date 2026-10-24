import type { PickResult } from '@/api/types'

/** JSON IDs must retain their exact signed BIGINT value as strings. */
export function isReceiptID(value: unknown): value is string {
  return typeof value === 'string' && /^[1-9]\d{0,18}$/.test(value) && (value.length < 19 || value <= '9223372036854775807')
}

export function isPickReceipt(value: unknown): value is PickResult {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const row = value as Record<string, unknown>
  const quantity = (n: unknown): n is number => typeof n === 'number' && Number.isInteger(n) && n >= 0 && n <= 2147483647
  return typeof row.task_status === 'string' && ['CREATED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED'].includes(row.task_status) &&
    typeof row.order_status === 'string' && ['DRAFT', 'SUBMITTED', 'APPROVED', 'PICKING', 'SHIPPED', 'CANCELLED', 'EXCEPTION', 'CLOSED'].includes(row.order_status) &&
    quantity(row.done_qty) && quantity(row.remaining_qty) && row.done_qty + row.remaining_qty <= 2147483647
}
