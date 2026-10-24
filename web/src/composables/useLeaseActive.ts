import { onScopeDispose, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'

/** Refresh the local lease indicator at expiry and when returning to a sleeping tab. */
export function useLeaseActive(expires: MaybeRefOrGetter<string | null | undefined>) {
  const active = ref(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  function sync() {
    clearTimeout(timer)
    const remaining = Date.parse(toValue(expires) || '') - Date.now()
    active.value = remaining > 0
    if (active.value) timer = setTimeout(sync, Math.min(remaining + 1, 2147483647))
  }
  watch(() => toValue(expires), sync, { immediate: true, flush: 'sync' })
  window.addEventListener('focus', sync)
  document.addEventListener('visibilitychange', sync)
  onScopeDispose(() => {
    clearTimeout(timer)
    window.removeEventListener('focus', sync)
    document.removeEventListener('visibilitychange', sync)
  })
  return active
}
