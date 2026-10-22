import { onScopeDispose, reactive, ref } from 'vue'
import { listInventoryAlerts, type AlertList, type AlertQuery } from '@/api/inventoryAlerts'

export function useInventoryAlerts(warehouse = '') {
  const query = reactive<AlertQuery>({ warehouse_id: warehouse, sku_keyword: '', state: 'ALERT', page: 1, page_size: 10 })
  const result = ref<AlertList>({ list: [], total: 0, warehouses: [], summary: { configured: 0, out_of_stock: 0, low: 0 }, checked_at: '' })
  const loading = ref(false), error = ref('')
  let sequence = 0, alive = true
  onScopeDispose(() => { alive = false; sequence++ })
  async function load() {
    if (!alive) return
    const current = ++sequence
    loading.value = true; error.value = ''
    try {
      const data = await listInventoryAlerts({ ...query })
      if (alive && current === sequence) result.value = data
    } catch { if (alive && current === sequence) error.value = '预警查询失败，当前显示上次结果，请重试' }
    finally { if (alive && current === sequence) loading.value = false }
  }
  function search() { query.page = 1; return load() }
  return { query, result, loading, error, load, search }
}
