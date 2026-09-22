import type {
  DemoActivitySnapshot,
  DemoConcurrentResult,
  DemoPerformanceSnapshot,
  DemoPickingResult,
  DemoScenarioResult,
  DemoSessionInfo,
  EntityID,
} from './types'
import { get, post } from './request'

export function acquireDemoSession() {
  return post<DemoSessionInfo>('/demo/session/acquire')
}

/** 空闲演示账号信息（多租户多账号，登录页"在线体验"自动分配） */
export interface DemoAccountInfo {
  tenant_id: EntityID
  username: string
  password: string
  total: number
  occupied: number
}

/** 领取一个空闲演示账号（免登录接口；全部占用时后端返回 70002） */
export function claimDemoAccount() {
  return post<DemoAccountInfo>('/demo/account')
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

export function runConcurrentPicking(workers = 10, contenders = 5) {
  return post<DemoPickingResult>('/demo/run/picking', { workers, contenders })
}

export function restockDemo(qty = 500) {
  return post<DemoScenarioResult>('/demo/run/restock', { qty })
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
