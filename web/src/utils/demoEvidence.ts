import type { DemoActivitySnapshot, DemoScenarioResult } from '@/api/types'
import { isBusinessOperation } from '@/utils/demoOperations'

const STORAGE_KEY = 'wms-demo-evidence-context-v1'

export interface DemoEvidenceContext {
  scenario: string
  startedAt: string
  completedAt: string
  summary: string
  evidence: DemoScenarioResult['evidence']
  links: DemoScenarioResult['links']
}

export interface DemoEvidenceFocus {
  scenario: string
  source: string
  startedAt: string
  completedAt: string
  summary: string
  orderIds: Record<string, string[]>
  orderNos: string[]
  taskIds: string[]
}

export function rememberDemoEvidence(result: DemoScenarioResult, startedAt: string): DemoEvidenceContext {
  const context: DemoEvidenceContext = {
    scenario: result.name,
    startedAt,
    completedAt: new Date().toISOString(),
    summary: result.summary,
    evidence: result.evidence ?? [],
    links: result.links ?? [],
  }
  if (typeof window !== 'undefined') {
    window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(context))
  }
  return context
}

export function rememberDemoExecutionWindow(
  scenario: string,
  startedAt: string,
  completedAt: string,
  links: DemoScenarioResult['links'] = [],
): DemoEvidenceContext {
  const context: DemoEvidenceContext = {
    scenario,
    startedAt,
    completedAt,
    summary: '最近一次演示执行完成',
    evidence: [],
    links,
  }
  if (typeof window !== 'undefined') {
    window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(context))
  }
  return context
}

export function clearDemoEvidence(): void {
  if (typeof window !== 'undefined') window.sessionStorage.removeItem(STORAGE_KEY)
}

export function readDemoEvidence(): DemoEvidenceContext | null {
  if (typeof window === 'undefined') return null
  const raw = window.sessionStorage.getItem(STORAGE_KEY)
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as Partial<DemoEvidenceContext>
    if (!value.scenario || !value.startedAt || !value.completedAt || !value.summary) return null
    return {
      scenario: value.scenario,
      startedAt: value.startedAt,
      completedAt: value.completedAt,
      summary: value.summary,
      evidence: Array.isArray(value.evidence) ? value.evidence : [],
      links: Array.isArray(value.links) ? value.links : [],
    }
  } catch {
    return null
  }
}

function queryText(query: Record<string, unknown>, key: string): string {
  const value = query[key]
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

function addUnique(target: string[], value: string): void {
  if (value && !target.includes(value)) target.push(value)
}

function focusFromContext(context: DemoEvidenceContext): DemoEvidenceFocus {
  const focus: DemoEvidenceFocus = {
    scenario: context.scenario,
    source: 'auto',
    startedAt: context.startedAt,
    completedAt: context.completedAt,
    summary: context.summary,
    orderIds: {},
    orderNos: [],
    taskIds: [],
  }

  for (const link of context.links ?? []) {
    const path = link.path
    for (const [scenario, prefix] of [
      ['inbound', '/inbound/orders/'],
      ['outbound', '/outbound/orders/'],
      ['stocktake', '/stocktake/orders/'],
    ] as const) {
      if (path.startsWith(prefix)) {
        addUnique((focus.orderIds[scenario] ??= []), path.slice(prefix.length).split(/[/?#]/)[0])
      }
    }
    const url = new URL(path, 'https://demo.local')
    const orderNo = url.searchParams.get('order_no')
    if (orderNo) addUnique(focus.orderNos, orderNo)
    const taskId = url.searchParams.get('task_id')
    if (taskId) addUnique(focus.taskIds, taskId)
    const orderId = url.searchParams.get('order_id')
    if (orderId && !Object.values(focus.orderIds).some((ids) => ids.includes(orderId))) {
      addUnique((focus.orderIds[context.scenario] ??= []), orderId)
    }
  }
  return focus
}

export function resolveDemoEvidenceFocus(
  query: Record<string, unknown>,
  context: DemoEvidenceContext | null,
): DemoEvidenceFocus {
  const requestedSource = queryText(query, 'source')
  const useContext = requestedSource !== 'manual' && requestedSource !== 'staged' && Boolean(context)
  const base = useContext && context ? focusFromContext(context) : {
    scenario: '',
    source: '',
    startedAt: '',
    completedAt: '',
    summary: '',
    orderIds: {},
    orderNos: [],
    taskIds: [],
  }

  const scenario = queryText(query, 'scenario') || base.scenario
  const source = requestedSource || base.source
  const startedAt = queryText(query, 'started_at') || base.startedAt
  const completedAt = queryText(query, 'completed_at') || base.completedAt
  const orderId = queryText(query, 'order_id')
  const orderNo = queryText(query, 'order_no')
  const taskId = queryText(query, 'task_id')
  const taskNo = queryText(query, 'task_no')

  if (scenario && scenario !== base.scenario) {
    base.orderIds = {}
    base.orderNos = []
    base.taskIds = []
  }
  if (scenario === 'inbound' || scenario === 'outbound' || scenario === 'stocktake') {
    if (orderId) addUnique((base.orderIds[scenario] ??= []), orderId)
  }
  if (orderNo) addUnique(base.orderNos, orderNo)
  if (taskId) addUnique(base.taskIds, taskId)
  if (taskNo && !base.taskIds.includes(taskNo)) addUnique(base.taskIds, taskNo)

  return {
    ...base,
    scenario,
    source,
    startedAt,
    completedAt,
    orderIds: { ...base.orderIds },
    orderNos: [...base.orderNos],
    taskIds: [...base.taskIds],
  }
}

function inExecutionWindow(createdAt: string, focus: DemoEvidenceFocus): boolean {
  if (!focus.startedAt || !createdAt) return true
  const created = Date.parse(createdAt)
  const started = Date.parse(focus.startedAt)
  if (!Number.isFinite(created) || !Number.isFinite(started)) return true
  const completed = focus.completedAt ? Date.parse(focus.completedAt) : Number.NaN
  return created >= started - 2000 && (!Number.isFinite(completed) || created <= completed + 5000)
}

function scenarioMatches(scenario: string, type: string): boolean {
  return !scenario || scenario === 'full' || scenario === type
}

function orderMatches(
  item: { id: string; order_no: string; created_at?: string },
  type: string,
  focus: DemoEvidenceFocus,
): boolean {
  if (!scenarioMatches(focus.scenario, type)) return false
  const ids = focus.orderIds[type] ?? []
  if (ids.length > 0 && !ids.includes(String(item.id))) return false
  if (ids.length === 0 && focus.orderNos.length > 0 && !focus.orderNos.includes(item.order_no)) return false
  return inExecutionWindow(item.created_at ?? '', focus)
}

export function filterDemoActivity(
  snapshot: DemoActivitySnapshot,
  focus: DemoEvidenceFocus,
): DemoActivitySnapshot {
  const taskTypes: Record<string, string[]> = {
    inbound: ['PUTAWAY'],
    outbound: ['PICK'],
    stocktake: [],
  }
  const transTypes: Record<string, string[]> = {
    inbound: ['RECEIVE'],
    outbound: ['ALLOCATE', 'RELEASE', 'SHIP'],
    stocktake: ['ADJUST'],
  }
  const operationPrefixes: Record<string, string[]> = {
    inbound: ['/api/v1/inbound', '/inbound'],
    outbound: ['/api/v1/outbound', '/outbound'],
    stocktake: ['/api/v1/stocktake', '/stocktake'],
  }

  const inboundOrders = snapshot.inbound_orders.filter((item) => orderMatches(item, 'inbound', focus))
  const outboundOrders = snapshot.outbound_orders.filter((item) => orderMatches(item, 'outbound', focus))
  const stocktakeOrders = snapshot.stocktake_orders.filter((item) => orderMatches(item, 'stocktake', focus))
  const tasks = snapshot.tasks.filter((item) => {
    const allowedTypes = taskTypes[focus.scenario]
    if (allowedTypes && allowedTypes.length > 0 && !allowedTypes.includes(item.task_type)) return false
    if (focus.scenario === 'stocktake') return false
    if (focus.taskIds.length > 0) {
      return (focus.taskIds.includes(String(item.id)) || focus.taskIds.includes(item.task_no)) && inExecutionWindow(item.created_at, focus)
    }
    if (focus.orderNos.length > 0 && !focus.orderNos.includes(item.order_no)) return false
    return inExecutionWindow(item.created_at, focus)
  })
  const inventoryTrans = snapshot.inventory_trans.filter((item) => {
    const allowedTypes = transTypes[focus.scenario]
    if (allowedTypes && allowedTypes.length > 0 && !allowedTypes.includes(item.trans_type)) return false
    if (focus.scenario === 'stocktake' && item.trans_type !== 'ADJUST') return false
    if (focus.orderNos.length > 0 && !focus.orderNos.includes(item.order_no)) return false
    if (focus.orderNos.length === 0 && focus.taskIds.length > 0 && !focus.taskIds.includes(item.task_no)) return false
    return inExecutionWindow(item.created_at, focus)
  })
  const operations = snapshot.operations.filter((item) => {
    if (!isBusinessOperation(item)) return false
    if (!inExecutionWindow(item.created_at, focus)) return false
    const prefixes = operationPrefixes[focus.scenario]
    if (prefixes?.length) return prefixes.some((prefix) => item.path.startsWith(prefix))
    return true
  })

  return {
    operations,
    inbound_orders: inboundOrders,
    outbound_orders: outboundOrders,
    stocktake_orders: stocktakeOrders,
    tasks,
    inventory_trans: inventoryTrans,
  }
}
