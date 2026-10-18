import type { PageData } from '@/api/types'

/** 分批加载选项；任一页失败则整体失败，不能把不完整数据当作全部选项。 */
export async function loadAllPages<T>(fetchPage: (page: number, size: number) => Promise<PageData<T>>): Promise<T[]> {
  const rows: T[] = []
  for (let page = 1; ; page++) {
    const data = await fetchPage(page, 100)
    rows.push(...data.list)
    if (data.list.length === 0 || rows.length >= data.total) return rows
  }
}
