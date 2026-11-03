import { onScopeDispose, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { useAuthStore } from '@/stores/auth'

/** A business confirmation belongs to the page and login that opened it. */
export function useScopedConfirm(target: () => unknown = () => null) {
  const route = useRoute(), auth = useAuthStore()
  let alive = true, active: symbol | null = null
  const identity = () => JSON.stringify([route?.fullPath, auth.sessionVersion, auth.token, auth.demoSessionId, target()])
  function close() { if (active) { active = null; ElMessageBox.close() } }
  watch(identity, close, { flush: 'sync' })
  onScopeDispose(() => { alive = false; close() })
  return async (...args: Parameters<typeof ElMessageBox.confirm>) => {
    if (!alive || active || !auth.storageIsCurrent()) throw new Error('当前操作已取消或仍在等待确认')
    const owner = identity()
    const confirmation = Symbol('confirmation')
    active = confirmation
    try {
      const result = await ElMessageBox.confirm(...args)
      if (!alive || active !== confirmation || identity() !== owner || !auth.storageIsCurrent()) throw new Error('页面、账号或操作目标已切换，原操作已取消')
      return result
    } finally { if (active === confirmation) active = null }
  }
}
