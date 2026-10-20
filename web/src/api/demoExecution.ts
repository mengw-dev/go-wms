import { post } from './request'

export type StagedRequestKind = 'inbound-create' | 'inbound-submit' | 'inbound-approve' |
  'receive' | 'putaway' | 'outbound-create' | 'outbound-submit' | 'outbound-approve' | 'pick' | 'location-create'

export interface StagedRequest {
  kind: StagedRequestKind
  objectId: string
  payload: Record<string, unknown>
}

/** 演示仍调用真实业务接口；取消只停止客户端等待，已发出的写请求保留待确认记录。 */
export function executeStagedRequest(request: StagedRequest, operationKey: string, signal: AbortSignal): Promise<unknown> {
  const id = encodeURIComponent(request.objectId)
  const paths: Record<StagedRequestKind, string> = {
    'inbound-create': '/inbound/orders',
    'inbound-submit': `/inbound/orders/${id}/submit`,
    'inbound-approve': `/inbound/orders/${id}/approve`,
    receive: `/inbound/orders/${id}/receive`,
    putaway: `/inbound/tasks/${id}/putaway`,
    'outbound-create': '/outbound/orders',
    'outbound-submit': `/outbound/orders/${id}/submit`,
    'outbound-approve': `/outbound/orders/${id}/approve`,
    pick: `/outbound/tasks/${id}/pick`,
    'location-create': '/basic/locations/batch',
  }
  const keyed = ['receive', 'putaway', 'pick'].includes(request.kind)
  return post(paths[request.kind], request.payload, {
    signal,
    headers: keyed ? { 'Idempotency-Key': operationKey } : undefined,
  })
}
