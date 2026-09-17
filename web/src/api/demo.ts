import type { DemoScenarioResult, DemoSessionInfo } from './types'
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

export function resetDemoData() {
  return post<void>('/demo/reset')
}