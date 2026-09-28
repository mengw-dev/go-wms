/**
 * Guide 路由匹配。
 *
 * 步骤路由模板（含 :orderId / :orderNo 占位）与实际地址的换算，以及
 * “当前 URL 对应场景中的哪一步”的判断，集中在这里，不散落在 Store / 组件中。
 */
import { getGuideSteps } from './definitions'
import type { GuideScenario, GuideStep } from './types'

export function resolveGuideRoute(route: string, orderId: string, orderNo: string): string {
  return route
    .replace(':orderId', encodeURIComponent(orderId))
    .replace(':orderNo', encodeURIComponent(orderNo))
}

export interface GuideRouteMatch {
  index: number
  step: GuideStep
}

/**
 * 在给定场景中按路由路径定位步骤。
 * 同一路径可能对应多步（例如单据详情页），优先返回第一个尚未完成的步骤；
 * 全部完成时返回最后一步，便于停留在终点。
 */
export function matchGuideStepByRoute(
  scenario: GuideScenario,
  routePath: string,
  orderId: string,
  orderNo: string,
  doneStepIds: readonly string[],
): GuideRouteMatch | null {
  const matches = getGuideSteps(scenario)
    .map((step, index) => ({ step, index }))
    .filter(({ step }) => resolveGuideRoute(step.route, orderId, orderNo).split('?')[0] === routePath)
  return matches.find(({ step }) => !doneStepIds.includes(step.id)) ?? matches.at(-1) ?? null
}

/** 当前路径是否属于该场景的任一步骤，用于检测是否偏离引导流程。 */
export function isGuideFlowRoute(
  scenario: GuideScenario,
  routePath: string,
  orderId: string,
  orderNo: string,
): boolean {
  return getGuideSteps(scenario).some(
    (step) => resolveGuideRoute(step.route, orderId, orderNo).split('?')[0] === routePath,
  )
}
