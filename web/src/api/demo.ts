import type { DemoActivitySnapshot, DemoConcurrentResult, DemoPerformanceSnapshot, DemoScenarioResult, DemoSessionInfo } from './types'
import { get, post } from './request'

export function acquireDemoSession() {
  return post<DemoSessionInfo>('/demo/session/acquire')
}

export function heartbeatDemoSession() {
  return post<DemoSessionInfo>('/demo/session/heartbeat')
}

export function releaseDemoSession() {
  return post<void>('/demo/session/release')
}

export function getDemoSessionStatus() {
  return get<DemoSessionInfo>('/demo/session/status')
}

export interface DemoScenarioOptions {
  count?: number
  qty?: number
}

export function runDemoScenario(
  scenario: 'inbound' | 'outbound' | 'stocktake' | 'full' | 'inbound_drafts' | 'outbound_drafts' | 'stocktake_drafts',
  options: DemoScenarioOptions = {},
) {
  return post<DemoScenarioResult>(`/demo/run/${scenario}`, options)
}

export function runConcurrentDemo(concurrency = 20, qtyPerOrder = 1) {
  return post<DemoConcurrentResult>('/demo/run/concurrent', { concurrency, qty_per_order: qtyPerOrder })
}

export function getDemoPerformance() {
  return get<DemoPerformanceSnapshot>('/demo/performance')
}

export function getDemoActivity(limit = 20) {
  return get<DemoActivitySnapshot>('/demo/activity', { limit })
}

export function resetDemoData() {
  return post<void>('/demo/reset')
}