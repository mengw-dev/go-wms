export const MIN_TABLE_HEIGHT = 160
export function maxTableHeight(viewport: number) { return Math.max(MIN_TABLE_HEIGHT, Math.min(2400, Math.round(viewport * 2))) }
export function clampTableHeight(height: number, viewport: number) {
  return Math.max(MIN_TABLE_HEIGHT, Math.min(maxTableHeight(viewport), Math.round(height)))
}
export function tableHeightKey(name: string, compact: boolean) { return `gowms:table-height:v1:${name}:${compact ? 'compact' : 'wide'}` }
export function readTableHeight(key: string, viewport: number): number | null {
  try {
    const raw = localStorage.getItem(key)
    if (raw === null || !raw.trim()) return null
    const value = Number(raw)
    return Number.isFinite(value) && value > 0 ? clampTableHeight(value, viewport) : null
  } catch { return null }
}
export function saveTableHeight(key: string, value: number | null) {
  try {
    if (value === null) localStorage.removeItem(key)
    else localStorage.setItem(key, String(value))
  } catch { /* Layout remains usable when browser storage is unavailable. */ }
}
