/** 过滤掉空字符串 / undefined / null 的查询参数（保留 0 / false 等有效值） */
export function cleanParams<T extends Record<string, unknown>>(params: T): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(params)) {
    if (value !== '' && value !== undefined && value !== null) {
      out[key] = value
    }
  }
  return out
}

/** 格式化时间字符串（后端返回 RFC3339，仅展示到秒） */
export function formatTime(value?: string | null): string {
  if (!value) return '-'
  return value.replace('T', ' ').slice(0, 19)
}
