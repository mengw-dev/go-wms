import type { Directive } from 'vue'
import { useAuthStore } from '@/stores/auth'

function updatePermission(el: HTMLElement, permission: string) {
  const auth = useAuthStore()
  el.hidden = !auth.hasPerm(permission)
}

export const permission: Directive<HTMLElement, string> = {
  mounted(el, binding) {
    updatePermission(el, binding.value)
  },
  updated(el, binding) {
    updatePermission(el, binding.value)
  },
}
