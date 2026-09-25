import type {
  DemoActivitySnapshot,
  DemoOperationLog,
  EntityID,
} from '@/api/types'
import { formatTime } from '@/utils'

export interface DemoOperationDetail {
  label: string
  value: string
}

export interface DemoOperationRow {
  key: string
  operation: string
  objectType: string
  objectNo: string
  beforeStatus: string
  afterStatus: string
  quantityChange: string
  relatedTask: string
  createdAt: string
  raw: DemoOperationLog
  details: DemoOperationDetail[]
}

function parseJSON(value?: string): unknown {
  if (!value) return null
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

function payloadData(operation: DemoOperationLog): Record<string, unknown> | null {
  const parsed = parseJSON(operation.result) as { data?: unknown } | null
  return parsed?.data && typeof parsed.data === 'object' && !Array.isArray(parsed.data)
    ? parsed.data as Record<string, unknown>
    : null
}

function params(operation: DemoOperationLog): Record<string, unknown> | null {
  const parsed = parseJSON(operation.params)
  return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
    ? parsed as Record<string, unknown>
    : null
}

function pathID(path: string, pattern: RegExp): string {
  return path.match(pattern)?.[1] || ''
}

function shortID(value?: EntityID | null, prefix = '对象'): string {
  if (!value) return '-'
  return prefix + ' · ' + String(value).slice(-8)
}

function operationMeta(operation: DemoOperationLog, snapshot?: DemoActivitySnapshot | null) {
  const path = operation.path
  const inboundID = pathID(path, /\/inbound\/orders\/(\d+)/)
  const outboundID = pathID(path, /\/outbound\/orders\/(\d+)/)
  const taskID = pathID(path, /\/(?:inbound|outbound)\/tasks\/(\d+)/) || pathID(path, /\/tasks\/(\d+)/)
  const inbound = snapshot?.inbound_orders.find((item) => String(item.id) === inboundID)
  const outbound = snapshot?.outbound_orders.find((item) => String(item.id) === outboundID)
  const task = snapshot?.tasks.find((item) => String(item.id) === taskID)
  const data = payloadData(operation)
  const objectNo = String(data?.order_no || data?.task_no || inbound?.order_no || outbound?.order_no || task?.task_no || shortID(inboundID || outboundID || taskID))

  if (path.endsWith('/demo/run/concurrent_shortage')) return { title: '供给不足并发验证', type: '库存实验', objectNo: '-', before: '-', after: '实验完成', task: '-' }
  if (path.endsWith('/demo/run/concurrent')) return { title: '并发库存分配实验', type: '库存实验', objectNo: '-', before: '-', after: '实验完成', task: '-' }
  if (path.endsWith('/demo/run/picking')) return { title: 'PDA 拣货作业验证', type: '拣货实验', objectNo: '-', before: '-', after: '实验完成', task: '-' }
  if (path.endsWith('/demo/run/restock')) return { title: '演示库存补货', type: '库存', objectNo: '-', before: '-', after: '补货完成', task: '-' }
  if (path.endsWith('/demo/reset')) return { title: '一键重置演示数据', type: '演示会话', objectNo: '-', before: '-', after: '已恢复初始状态', task: '-' }
  if (path.endsWith('/demo/run/inbound')) return { title: '自动演示 · 入库流程', type: '演示任务', objectNo: '-', before: '-', after: '执行完成', task: '-' }
  if (path.endsWith('/demo/run/outbound')) return { title: '自动演示 · 出库流程', type: '演示任务', objectNo: '-', before: '-', after: '执行完成', task: '-' }
  if (path.endsWith('/demo/run/full')) return { title: '自动演示 · 完整业务闭环', type: '演示任务', objectNo: '-', before: '-', after: '执行完成', task: '-' }

  if (path.includes('/inbound/orders') && path.endsWith('/submit')) return { title: '提交入库单', type: '入库单', objectNo, before: '草稿', after: '已提交', task: '-' }
  if (path.includes('/inbound/orders') && path.endsWith('/approve')) return { title: '审核入库单', type: '入库单', objectNo, before: '已提交', after: '已审核', task: '-' }
  if (path.includes('/inbound/orders') && path.endsWith('/cancel')) return { title: '取消入库单', type: '入库单', objectNo, before: '处理中', after: '已取消', task: '-' }
  if (path.includes('/receive')) return { title: '完成收货', type: '入库单', objectNo, before: '已审核', after: '收货中', task: task?.task_no || '-' }
  if (path.includes('/putaway')) return { title: '完成上架', type: '上架任务', objectNo: task?.task_no || objectNo, before: '上架中', after: '已完成', task: task?.task_no || objectNo }
  if (path.includes('/inbound/orders') && operation.method === 'POST') return { title: '创建入库单', type: '入库单', objectNo, before: '-', after: '草稿', task: '-' }
  if (path.includes('/outbound/orders') && path.endsWith('/submit')) return { title: '提交出库单', type: '出库单', objectNo, before: '草稿', after: '已提交', task: '-' }
  if (path.includes('/outbound/orders') && path.endsWith('/approve')) return { title: '审核并 FIFO 分配', type: '出库单', objectNo, before: '已提交', after: '分配完成', task: '-' }
  if (path.includes('/outbound/orders') && path.endsWith('/cancel')) return { title: '取消出库单', type: '出库单', objectNo, before: '处理中', after: '已取消', task: '-' }
  if (path.includes('/outbound/orders') && operation.method === 'POST') return { title: '创建出库单', type: '出库单', objectNo, before: '-', after: '草稿', task: '-' }
  if (path.includes('/pick')) return { title: '完成拣货', type: '拣货任务', objectNo: task?.task_no || objectNo, before: '拣货中', after: '已发货', task: task?.task_no || objectNo }
  if (path.includes('/inventory')) return { title: '查询库存', type: '库存', objectNo, before: '-', after: '-', task: '-' }
  if (path.includes('/tasks')) return { title: '查询任务', type: '任务', objectNo, before: '-', after: '-', task: '-' }
  return { title: operation.method + ' 业务操作', type: '业务对象', objectNo, before: '-', after: '-', task: '-' }
}

function quantityChange(operation: DemoOperationLog): string {
  const source = { ...(params(operation) || {}), ...(payloadData(operation) || {}) }
  if (operation.path.endsWith('/demo/run/concurrent') || operation.path.endsWith('/demo/run/concurrent_shortage')) {
    const concurrency = Number(source.concurrency || 0)
    const qty = Number(source.qty_per_order || 0)
    if (concurrency || qty) return concurrency + ' 请求 × ' + qty + ' 件'
  }
  if (operation.path.endsWith('/demo/run/picking')) {
    const workers = Number(source.workers || 0)
    const contenders = Number(source.contenders || 0)
    return workers + ' 拣货员 / ' + contenders + ' 竞争请求'
  }
  if (operation.path.endsWith('/demo/run/restock') && source.qty !== undefined) return '+' + String(source.qty) + ' 件'
  for (const key of ['quantity_change', 'actual_qty', 'qty', 'received_qty', 'defect_qty']) {
    const value = source[key]
    if (typeof value === 'number') return (value > 0 ? '+' : '') + String(value)
    if (typeof value === 'string' && value) return value
  }
  return '-'
}

export function buildDemoOperationRows(
  operations: DemoOperationLog[],
  snapshot?: DemoActivitySnapshot | null,
): DemoOperationRow[] {
  return operations
    .map((operation) => {
      const meta = operationMeta(operation, snapshot)
      const failed = operation.status < 200 || operation.status >= 300
      return {
        key: 'operation-' + operation.id,
        operation: meta.title,
        objectType: meta.type,
        objectNo: meta.objectNo,
        beforeStatus: meta.before,
        afterStatus: failed ? '处理失败' : meta.after,
        quantityChange: quantityChange(operation),
        relatedTask: meta.task,
        createdAt: operation.created_at,
        raw: operation,
        details: [
          { label: '操作类型', value: meta.title },
          { label: '业务对象', value: meta.type },
          { label: '对象编号', value: meta.objectNo },
          { label: '前置状态', value: meta.before },
          { label: '后置状态', value: failed ? '处理失败' : meta.after },
          { label: '数量变化', value: quantityChange(operation) },
          { label: '关联任务', value: meta.task },
          { label: '请求', value: operation.method + ' ' + operation.path },
          { label: 'HTTP 状态', value: String(operation.status) },
          { label: '操作时间', value: formatTime(operation.created_at) },
        ],
      }
    })
    .sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))
}
