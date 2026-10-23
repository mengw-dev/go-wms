import { shallowRef } from 'vue'
import type { Router } from 'vue-router'

export const failedPage = shallowRef<{ fullPath: string } | null>(null)

// A deployed page chunk can disappear while a user still has the previous app open.
// Keep the current page and pending business operations until the user chooses to refresh.
export function isPageAssetError(error: unknown): boolean {
  if (!(error instanceof Error)) return false
  return /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module|Unable to preload CSS|Loading (?:CSS )?chunk .+ failed/i.test(error.message)
}

export function installNavigationRecovery(router: Router): () => void {
  const failedRoutes = new Set<string>()
  // Vite caches attempted CSS preloads even after failure. Re-entering the same
  // route without refreshing could otherwise open a page with missing styles.
  const removeBefore = router.beforeEach(to => {
    if (!failedRoutes.has(to.matched.at(-1)?.path ?? to.path)) return
    failedPage.value = { fullPath: to.fullPath }
    return false
  })
  const removeError = router.onError((error, to) => {
    if (!isPageAssetError(error)) return
    failedRoutes.add(to.matched.at(-1)?.path ?? to.path)
    failedPage.value = { fullPath: to.fullPath }
  })
  const removeAfter = router.afterEach((_to, _from, failure) => {
    if (!failure) failedPage.value = null
  })
  return () => { removeBefore(); removeError(); removeAfter() }
}

export function refreshFailedPage(): void {
  if (!failedPage.value) return
  const target = new URL(failedPage.value.fullPath, window.location.origin)
  if (target.origin !== window.location.origin) return
  if (target.pathname + target.search === window.location.pathname + window.location.search) {
    window.history.replaceState(window.history.state, '', target.pathname + target.search + target.hash)
    window.location.reload()
  } else window.location.assign(target.href)
}
