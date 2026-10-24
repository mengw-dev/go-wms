import { onScopeDispose, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { useAuthStore } from '@/stores/auth'

/** A business confirmation belongs to the page and login that opened it. */
export function useScopedConfirm() {
  const route = useRoute(), auth = useAuthStore()
  let alive = true, opened = false
  const identity = () => JSON.stringify([route?.fullPath, auth.sessionVersion, auth.token, auth.demoSessionId])
  function close() { if (opened) { opened = false; ElMessageBox.close() } }
  watch(identity, close)
  onScopeDispose(() => { alive = false; close() })
  return async (...args: Parameters<typeof ElMessageBox.confirm>) => {
    if (!alive || opened) throw new Error('当前操作已取消或仍在等待确认')
    const owner = identity()
    opened = true
    try {
      const result = await ElMessageBox.confirm(...args)
      if (!alive || identity() !== owner) throw new Error('页面或账号已切换，原操作已取消')
      return result
    } finally { opened = false }
  }
}
