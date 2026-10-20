import { describe, expect, it, vi } from 'vitest'
import { post } from './request'
import { executeStagedRequest } from './demoExecution'

vi.mock('./request', () => ({ post: vi.fn() }))

describe('分步演示实际业务请求', () => {
  it.each([['receive', '/inbound/orders/123/receive'], ['putaway', '/inbound/tasks/123/putaway'], ['pick', '/outbound/tasks/123/pick']] as const)('%s 携带原操作键和取消信号', async (kind, path) => {
    vi.mocked(post).mockClear()
    const controller = new AbortController()
    const payload = { qty: 2 }
    await executeStagedRequest({ kind, objectId: '123', payload }, 'original-key', controller.signal)
    expect(post).toHaveBeenCalledExactlyOnceWith(path, payload, { headers: { 'Idempotency-Key': 'original-key' }, signal: controller.signal })
  })
})
