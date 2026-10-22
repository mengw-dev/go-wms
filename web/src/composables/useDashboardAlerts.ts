import { onScopeDispose, ref } from 'vue'
import { listInventoryAlerts, type AlertList } from '@/api/inventoryAlerts'

// Reuse the alert page's exact-tenant read model and failure semantics. Warehouse
// changes clear old values; a same-warehouse refresh failure retains a dated value.
export function useDashboardAlerts(allowed: () => boolean) {
  const result = ref<AlertList>(), error = ref(false), loading = ref(false)
  let sequence = 0, alive = true, warehouse: string | undefined
  onScopeDispose(() => { alive = false; sequence++ })
  async function load(id: string) {
    if (!alive || !allowed()) return
    const current = ++sequence
    if (warehouse !== id) result.value = undefined
    warehouse = id
    loading.value = true
    error.value = false
    try {
      const data = await listInventoryAlerts({ warehouse_id: id, state: 'ALERT', sku_keyword: '', page: 1, page_size: 1 })
      if (alive && sequence === current) result.value = data
    } catch { if (alive && sequence === current) error.value = true }
    finally { if (alive && sequence === current) loading.value = false }
  }
  return { result, error, loading, load }
}
