/** Guide / Demo 外挂层的公共类型。静态配置见 definitions.ts。 */

export type GuideScenario = 'inbound' | 'outbound' | 'stocktake'

export interface GuideStep {
  id: string
  route: string
  target: string
  title: string
  description: string
  event: string
}

export interface GuideFact {
  label: string
  value: string
}

export interface GuideBusinessResult {
  orderId?: string
  orderNo?: string
  taskId?: string
  taskNo?: string
  message?: string
  facts?: GuideFact[]
}
