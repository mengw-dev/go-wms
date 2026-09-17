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

export function runDemoScenario(scenario: 'inbound' | 'outbound' | 'stocktake' | 'full') {
  return post<DemoScenarioResult>(`/demo/run/${scenario}`)
}

export function runConcurrentDemo(concurrency = 20) {
  return post<DemoConcurrentResult>('/demo/run/concurrent', { concurrency })
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