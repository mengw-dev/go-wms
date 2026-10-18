export const MAX_QUANTITY = 2147483647

/** 单据总量也落在 INT 字段中，不能只限制每一行输入。 */
export function validOrderQuantities(rows: { expected_qty: number }[]): boolean {
  let remaining = MAX_QUANTITY
  for (const row of rows) {
    if (!Number.isInteger(row.expected_qty) || row.expected_qty < 1 || row.expected_qty > remaining) return false
    remaining -= row.expected_qty
  }
  return true
}
