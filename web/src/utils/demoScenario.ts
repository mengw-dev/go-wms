import type {
  DemoScenarioEvidence,
  DemoScenarioImplementation,
  DemoScenarioLink,
  DemoScenarioResult,
  DemoScenarioStep,
} from '@/api/types'

function uniqueBy<T>(items: T[], key: (item: T) => string): T[] {
  const seen = new Set<string>()
  const result: T[] = []
  for (const item of items) {
    const value = key(item)
    if (seen.has(value)) continue
    seen.add(value)
    result.push(item)
  }
  return result
}

function mergeImplementation(results: DemoScenarioResult[]): DemoScenarioImplementation | undefined {
  const implementations = results
    .map((result) => result.implementation)
    .filter((item): item is DemoScenarioImplementation => Boolean(item))
  if (!implementations.length) return undefined

  return {
    orchestration: 'web/src/components/DemoConsole.vue',
    business_files: [...new Set(implementations.flatMap((item) => item.business_files))],
    call_chain: ['Vue Demo Orchestrator', 'Inbound Service', 'Inventory / Task', 'Outbound Service', 'MySQL'],
  }
}

function mergeEvidence(results: DemoScenarioResult[]): DemoScenarioEvidence[] {
  return uniqueBy(
    results.flatMap((result) => result.evidence ?? []),
    (item) => `${item.label}:${item.value}`,
  )
}

function mergeLinks(results: DemoScenarioResult[]): DemoScenarioLink[] {
  return uniqueBy(
    results.flatMap((result) => result.links ?? []),
    (item) => item.path,
  )
}

function mergeSteps(results: DemoScenarioResult[]): DemoScenarioStep[] {
  return results.flatMap((result) => result.steps ?? [])
}

export function mergeDemoScenarioResults(results: DemoScenarioResult[]): DemoScenarioResult {
  const completedResults = results.filter(Boolean)
  const failed = completedResults.some((result) => result.status === 'failed')

  return {
    name: 'full',
    status: failed ? 'failed' : 'completed',
    summary: failed
      ? '入库、库存和出库业务闭环未完全成功'
      : '入库、库存和出库业务闭环已完成',
    evidence_title: '本次完整业务闭环产生',
    steps: mergeSteps(completedResults),
    evidence: mergeEvidence(completedResults),
    links: mergeLinks(completedResults),
    implementation: mergeImplementation(completedResults),
  }
}
