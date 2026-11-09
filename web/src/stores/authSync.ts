import { watch } from 'vue'
import { useAuthStore } from './auth'

/** 普通登录跨页共享；先阻止旧页继续提交，再重建页面，避免旧表单套用新身份。 */
export function installAuthStorageSync(reload = () => window.location.reload()): () => void {
  const auth = useAuthStore()
  let reloading = false
  const reloadOnce = () => {
    if (!reloading && auth.externalSessionChanged) { reloading = true; reload() }
  }
  const check = () => { auth.invalidateIfStorageChanged(); reloadOnce() }
  const onStorage = (event: StorageEvent) => { if (event.storageArea === localStorage) check() }
  // focus/pageshow覆盖后台冻结与BFCache恢复；请求入口仍会独立核对，不能只等事件。
  window.addEventListener('storage', onStorage)
  window.addEventListener('focus', check)
  window.addEventListener('pageshow', check)
  const stop = watch(() => auth.externalSessionChanged, reloadOnce)
  check()
  return () => {
    stop()
    window.removeEventListener('storage', onStorage)
    window.removeEventListener('focus', check)
    window.removeEventListener('pageshow', check)
  }
}
