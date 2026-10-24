import { effectScope, ref } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useLeaseActive } from './useLeaseActive'

const scopes: ReturnType<typeof effectScope>[] = []
function setup(expires = new Date(Date.now() + 1000).toISOString()) {
  const source = ref(expires), scope = effectScope(); scopes.push(scope)
  return { source, scope, active: scope.run(() => useLeaseActive(source))! }
}
beforeEach(() => {
  vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-24T00:00:00Z'))
  vi.stubGlobal('window', new EventTarget()); vi.stubGlobal('document', new EventTarget())
})
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.useRealTimers(); vi.unstubAllGlobals() })
it('expires without waiting for a server response or another reactive change', () => {
  const flow = setup(); expect(flow.active.value).toBe(true)
  vi.advanceTimersByTime(1001); expect(flow.active.value).toBe(false)
})
it('renewal replaces the old expiry timer', () => {
  const flow = setup(); vi.advanceTimersByTime(500)
  flow.source.value = new Date(Date.now() + 2000).toISOString()
  vi.advanceTimersByTime(501); expect(flow.active.value).toBe(true)
  vi.advanceTimersByTime(1500); expect(flow.active.value).toBe(false)
})
it('rejects empty, invalid and already expired leases', () => {
  for (const value of ['', 'invalid', '2026-10-23T00:00:00Z']) expect(setup(value).active.value).toBe(false)
  expect(vi.getTimerCount()).toBe(0)
})
it('rechecks a sleeping tab on visibility or focus', () => {
  const flow = setup(); vi.setSystemTime(new Date('2026-10-24T00:10:00Z'))
  document.dispatchEvent(new Event('visibilitychange')); expect(flow.active.value).toBe(false)
  flow.source.value = new Date(Date.now() + 1000).toISOString()
  vi.setSystemTime(new Date('2026-10-24T00:20:00Z'))
  window.dispatchEvent(new Event('focus')); expect(flow.active.value).toBe(false)
})
it('cleans up the timer and wake listeners with its component', () => {
  const flow = setup(); flow.scope.stop(); expect(vi.getTimerCount()).toBe(0)
  window.dispatchEvent(new Event('focus')); document.dispatchEvent(new Event('visibilitychange'))
  expect(vi.getTimerCount()).toBe(0)
})
