/**
 * Guide 运行态持久化。
 *
 * Store 不直接碰 sessionStorage / JSON 序列化，统一走这里，
 * 便于测试替换和后续调整存储策略。
 */
import type { GuideFact, GuideScenario } from './types'

export const GUIDE_STORAGE_KEY = 'wms-manual-guide-v1'

export interface PersistedGuideState {
  active: boolean
  scenario: GuideScenario | null
  currentStep: number
  orderId: string
  orderNo: string
  taskId: string
  taskNo: string
  startedAt: number
  completed: boolean
  /** 真实业务事件逐步完成的步骤。 */
  verifiedStepIds: string[]
  /** 业务状态领先时推断为已完成的中间步骤，不等同于真实验证。 */
  inferredStepIds: string[]
  facts: GuideFact[]
  lastOutcome: string
  mismatch: string
}

export function loadGuideState(): PersistedGuideState | null {
  if (typeof window === 'undefined') return null
  const raw = window.sessionStorage.getItem(GUIDE_STORAGE_KEY)
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as Partial<PersistedGuideState>
    if (value.scenario !== 'inbound' && value.scenario !== 'outbound' && value.scenario !== 'stocktake') return null
    if (typeof value.active !== 'boolean' || typeof value.currentStep !== 'number') return null
    return {
      active: value.active,
      scenario: value.scenario,
      currentStep: Math.max(0, value.currentStep),
      orderId: value.orderId || '',
      orderNo: value.orderNo || '',
      taskId: value.taskId || '',
      taskNo: value.taskNo || '',
      startedAt: typeof value.startedAt === 'number' ? value.startedAt : Date.now(),
      completed: Boolean(value.completed),
      verifiedStepIds: Array.isArray(value.verifiedStepIds) ? value.verifiedStepIds.filter((item): item is string => typeof item === 'string') : [],
      inferredStepIds: Array.isArray(value.inferredStepIds) ? value.inferredStepIds.filter((item): item is string => typeof item === 'string') : [],
      facts: Array.isArray(value.facts) ? value.facts.filter((item): item is GuideFact => Boolean(item && typeof item.label === 'string' && typeof item.value === 'string')) : [],
      lastOutcome: value.lastOutcome || '',
      mismatch: value.mismatch || '',
    }
  } catch {
    return null
  }
}

export function saveGuideState(state: PersistedGuideState): void {
  if (typeof window === 'undefined') return
  window.sessionStorage.setItem(GUIDE_STORAGE_KEY, JSON.stringify(state))
}

export function clearGuideState(): void {
  if (typeof window === 'undefined') return
  window.sessionStorage.removeItem(GUIDE_STORAGE_KEY)
}
