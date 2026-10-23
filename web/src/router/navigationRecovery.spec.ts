import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { failedPage, installNavigationRecovery, isPageAssetError, refreshFailedPage } from './navigationRecovery'

afterEach(() => { failedPage.value = null; vi.unstubAllGlobals(); vi.restoreAllMocks() })

describe('page loading recovery', () => {
  it.each([
    'Failed to fetch dynamically imported module: /assets/Order-OLD.js',
    'Importing a module script failed.',
    'error loading dynamically imported module: /assets/Order-OLD.js',
    'Unable to preload CSS for /assets/Order-OLD.css',
    'Loading chunk 13 failed.',
  ])('recognizes browser asset errors: %s', message => {
    expect(isPageAssetError(new TypeError(message))).toBe(true)
  })

  it('keeps runtime and business errors out of the update flow', () => {
    expect(isPageAssetError(new TypeError("Cannot read properties of null (reading 'id')"))).toBe(false)
    expect(isPageAssetError(new Error('Request failed with status code 403'))).toBe(false)
    expect(isPageAssetError('Failed to fetch dynamically imported module')).toBe(false)
  })

  it('retains the failed destination and clears recovery after another successful navigation', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const error = new TypeError('Failed to fetch dynamically imported module: /assets/Old.js')
    const router = createRouter({ history: createMemoryHistory(), routes: [
      { path: '/orders', component: {} },
      { path: '/orders/:id', component: () => Promise.reject(error) },
      { path: '/returns', component: {} },
    ] })
    const remove = installNavigationRecovery(router)
    await router.push('/orders')
    await expect(router.push('/orders/17?tab=returns#receipt')).rejects.toThrow(error)
    expect(router.currentRoute.value.fullPath).toBe('/orders')
    expect(failedPage.value?.fullPath).toBe('/orders/17?tab=returns#receipt')
    failedPage.value = null
    await router.push('/orders/18')
    expect(failedPage.value).toEqual({ fullPath: '/orders/18' })
    expect(router.currentRoute.value.fullPath).toBe('/orders')
    await router.push('/returns')
    expect(failedPage.value).toBeNull()
    remove()
  })

  it('only refreshes on user request, without changing stored business operations', () => {
    const assign = vi.fn(), reload = vi.fn(), replaceState = vi.fn()
    vi.stubGlobal('window', { location: { origin: 'https://wms.test', pathname: '/orders', search: '', assign, reload }, history: { state: {}, replaceState } })
    failedPage.value = { fullPath: '/outbound/orders/17?tab=returns#receipt' }
    expect(assign).not.toHaveBeenCalled()
    refreshFailedPage()
    expect(assign).toHaveBeenCalledWith('https://wms.test/outbound/orders/17?tab=returns#receipt')
    expect(reload).not.toHaveBeenCalled()
  })

  it('reloads an initial failed page and refuses an external destination', () => {
    const assign = vi.fn(), reload = vi.fn(), replaceState = vi.fn()
    vi.stubGlobal('window', { location: { origin: 'https://wms.test', pathname: '/orders/17', search: '?tab=returns', assign, reload }, history: { state: {}, replaceState } })
    failedPage.value = { fullPath: '/orders/17?tab=returns#receipt' }
    refreshFailedPage()
    expect(replaceState).toHaveBeenCalledWith({}, '', '/orders/17?tab=returns#receipt')
    expect(reload).toHaveBeenCalledOnce()
    failedPage.value = { fullPath: '//other.test/path' }
    refreshFailedPage()
    expect(assign).not.toHaveBeenCalled()
    expect(reload).toHaveBeenCalledOnce()
  })
})
