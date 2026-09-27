import { defineStore } from 'pinia'
import { getGuideStep, getGuideSteps } from '@/guide/definitions'
import { matchGuideStepByRoute, resolveGuideRoute } from '@/guide/routeMatcher'
import { clearGuideState, loadGuideState, saveGuideState } from '@/guide/persistence'
import type { GuideBusinessResult, GuideScenario, GuideStep } from '@/guide/types'

// 兼容层：Guide 相关定义已迁移到 src/guide/，这里保持原有入口不变。
export type { GuideBusinessResult, GuideFact, GuideScenario, GuideStep } from '@/guide/types'
export { GUIDE_EVENTS } from '@/guide/events'
export { GUIDE_SCENARIO_LABELS, getGuideStep } from '@/guide/definitions'
export { resolveGuideRoute } from '@/guide/routeMatcher'

export const useGuideStore = defineStore('guide', {
  state: () => {
    const persisted = loadGuideState()
    return {
      active: persisted?.active ?? false,
      scenario: persisted?.scenario ?? null as GuideScenario | null,
      currentStep: persisted?.currentStep ?? 0,
      orderId: persisted?.orderId ?? '',
      orderNo: persisted?.orderNo ?? '',
      taskId: persisted?.taskId ?? '',
      taskNo: persisted?.taskNo ?? '',
      startedAt: persisted?.startedAt ?? 0,
      completed: persisted?.completed ?? false,
      verifiedStepIds: persisted?.verifiedStepIds ?? [] as string[],
      inferredStepIds: persisted?.inferredStepIds ?? [] as string[],
      facts: persisted?.facts ?? [] as { label: string; value: string }[],
      lastOutcome: persisted?.lastOutcome ?? '',
      mismatch: persisted?.mismatch ?? '',
    }
  },
  getters: {
    steps(state): readonly GuideStep[] {
      return state.scenario ? getGuideSteps(state.scenario) : []
    },
    currentStepDefinition(state): GuideStep | null {
      return state.scenario ? getGuideStep(state.scenario, state.currentStep) : null
    },
    currentStepRoute(state): string {
      const step = state.scenario ? getGuideStep(state.scenario, state.currentStep) : null
      if (!step) return ''
      return resolveGuideRoute(step.route, state.orderId, state.orderNo)
    },
    currentStepNumber(state): number {
      return state.scenario ? state.currentStep + 1 : 0
    },
    totalSteps(state): number {
      return state.scenario ? getGuideSteps(state.scenario).length : 0
    },
    canGoPrevious(state): boolean {
      return state.active && !state.completed && state.currentStep > 0
    },
    canAdvance(state): boolean {
      const step = state.scenario ? getGuideStep(state.scenario, state.currentStep) : null
      return Boolean(state.active && !state.completed && step && state.verifiedStepIds.includes(step.id) && !state.mismatch)
    },
  },
  actions: {
    persist(): void {
      saveGuideState({
        active: this.active,
        scenario: this.scenario,
        currentStep: this.currentStep,
        orderId: this.orderId,
        orderNo: this.orderNo,
        taskId: this.taskId,
        taskNo: this.taskNo,
        startedAt: this.startedAt,
        completed: this.completed,
        verifiedStepIds: [...this.verifiedStepIds],
        inferredStepIds: [...this.inferredStepIds],
        facts: this.facts.map((fact) => ({ ...fact })),
        lastOutcome: this.lastOutcome,
        mismatch: this.mismatch,
      })
    },
    start(scenario: GuideScenario): GuideStep {
      this.active = true
      this.scenario = scenario
      this.currentStep = 0
      this.orderId = ''
      this.orderNo = ''
      this.taskId = ''
      this.taskNo = ''
      this.startedAt = Date.now()
      this.completed = false
      this.verifiedStepIds = []
      this.inferredStepIds = []
      this.facts = []
      this.lastOutcome = ''
      this.mismatch = ''
      this.persist()
      return getGuideSteps(scenario)[0]
    },
    recordBusinessResult(event: string, result: GuideBusinessResult = {}): boolean {
      const currentStep = this.currentStepDefinition
      if (!this.active || this.completed || !currentStep || !this.scenario) return false
      const steps = getGuideSteps(this.scenario)
      const targetIndex = steps.findIndex((item) => item.event === event)
      if (targetIndex < 0 || targetIndex < this.currentStep) {
        this.mismatch = `当前业务状态与引导不一致：当前步骤需要完成“${currentStep.title}”。`
        this.persist()
        return false
      }

      // 业务状态领先时，中间步骤只是“推断完成”，不能记为真实验证。
      if (targetIndex > this.currentStep) {
        for (let index = this.currentStep; index < targetIndex; index += 1) {
          const previousId = steps[index].id
          if (!this.verifiedStepIds.includes(previousId) && !this.inferredStepIds.includes(previousId)) {
            this.inferredStepIds.push(previousId)
          }
        }
      }

      const step = steps[targetIndex]
      if (!this.verifiedStepIds.includes(step.id)) this.verifiedStepIds.push(step.id)
      if (result.orderId) this.orderId = result.orderId
      if (result.orderNo) this.orderNo = result.orderNo
      if (result.taskId) this.taskId = result.taskId
      if (result.taskNo) this.taskNo = result.taskNo
      if (result.facts?.length) {
        const merged = new Map(this.facts.map((fact) => [fact.label, fact]))
        for (const fact of result.facts) {
          if (fact.label) merged.set(fact.label, fact)
        }
        this.facts = Array.from(merged.values())
      }

      const message = result.message || '当前步骤已在真实业务中完成。'
      this.lastOutcome = targetIndex > this.currentStep
        ? `检测到当前业务已经进入下一阶段，已自动同步引导进度。${message}`
        : message
      this.mismatch = ''

      if (targetIndex >= steps.length - 1) {
        this.currentStep = targetIndex
        this.completed = true
        this.active = false
      } else {
        this.currentStep = targetIndex + 1
      }
      this.persist()
      return true
    },

    next(): boolean {
      if (!this.canAdvance) return false
      if (this.currentStep >= this.totalSteps - 1) {
        this.completed = true
        this.active = false
        this.persist()
        return true
      }
      this.currentStep += 1
      this.mismatch = ''
      this.lastOutcome = ''
      this.persist()
      return true
    },
    previous(): boolean {
      if (!this.canGoPrevious) return false
      this.currentStep -= 1
      this.mismatch = ''
      this.lastOutcome = ''
      this.persist()
      return true
    },
    reposition(routePath: string): boolean {
      if (!this.active || !this.scenario) return false
      const doneStepIds = [...this.verifiedStepIds, ...this.inferredStepIds]
      const target = matchGuideStepByRoute(this.scenario, routePath, this.orderId, this.orderNo, doneStepIds)
      if (!target) {
        this.mismatch = '当前页面不在本次引导流程中，无法重新定位。'
        return false
      }
      this.currentStep = target.index
      this.mismatch = ''
      this.lastOutcome = ''
      this.persist()
      return true
    },
    setMismatch(message: string): void {
      if (!this.active || this.completed) return
      this.mismatch = message
      this.persist()
    },
    restart(): GuideStep | null {
      if (!this.scenario) return null
      return this.start(this.scenario)
    },
    cancel(): void {
      this.active = false
      this.completed = false
      this.scenario = null
      this.currentStep = 0
      this.orderId = ''
      this.orderNo = ''
      this.taskId = ''
      this.taskNo = ''
      this.startedAt = 0
      this.verifiedStepIds = []
      this.inferredStepIds = []
      this.facts = []
      this.lastOutcome = ''
      this.mismatch = ''
      clearGuideState()
    },
  },
})
