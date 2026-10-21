import { effectScope } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { receiveInbound, putawayInboundTask } from '@/api/inbound'
import { claimPdaTask, pickPdaTask } from '@/api/outbound'
import { approveStocktakeOrder } from '@/api/stocktake'
import { useAuthStore } from '@/stores/auth'
import { loadPending } from '@/utils/pendingOperations'
import { useReceive } from './inbound/useReceive'
import { usePutaway } from './inbound/usePutaway'
import { useStocktakeApprove } from './stocktake/useStocktakeApprove'
import { usePdaPick } from './outbound/usePdaPick'
import { classifyOperationError, getOperationContext, operationErrorMessage, operationStateKey } from './operationLifecycle'

vi.mock('@/api/inbound', () => ({ receiveInbound: vi.fn(), putawayInboundTask: vi.fn() }))
vi.mock('@/api/outbound', () => ({ claimPdaTask: vi.fn(), pickPdaTask: vi.fn() }))
vi.mock('@/api/stocktake', () => ({ approveStocktakeOrder: vi.fn() }))

function memoryStorage(failAtPendingWrite?: number): Storage {
  const data = new Map<string, string>()
  let pendingWrites = 0
  return {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      if (key === 'WMS_PENDING_OPS' && ++pendingWrites === failAtPendingWrite) throw new Error('quota')
      data.set(key, value)
    },
    removeItem: (key) => { data.delete(key) },
    clear: () => data.clear(),
    key: (index) => [...data.keys()][index] ?? null,
    get length() { return data.size },
  }
}

function login(userId: string, session = '') {
  const auth = useAuthStore()
  auth.setAuth({ token: 'test', user_id: userId, username: 'operator', nickname: 'Operator', roles: [], perms: session ? ['wms:demo'] : [] })
  if (session) auth.setDemoSession({ session_id: session, expires_in: 600 })
}

function pending(userId = '1', session = '') {
  return loadPending({ userId, sessionNamespace: session }, session ? sessionStorage : localStorage).items
}

const cases = [
  {
    scope: 'inbound.receive', safeCode: 40004, foreignCode: 60004,
    create: () => {
      const flow = useReceive(), request = vi.mocked(receiveInbound)
      return {
        send: () => flow.receive('1', '10', { qty: 2, defective_qty: 0, batch_no: ' B-1 ' }),
        restore: () => flow.restoreForDetails(['10']), state: () => flow.stateOf('10'),
        reject: (error: unknown) => request.mockRejectedValueOnce(error), resolve: () => request.mockResolvedValueOnce(undefined),
        key: () => request.mock.calls.at(-1)?.[2], calls: () => request.mock.calls.length,
      }
    },
  },
  {
    scope: 'inbound.putaway', safeCode: 40005, foreignCode: 40004,
    create: () => {
      const flow = usePutaway(), request = vi.mocked(putawayInboundTask)
      return {
        send: () => flow.putaway('10', { location_id: '9223372036854775800', qty: 2 }),
        restore: () => flow.restoreForTasks(['10']), state: () => flow.stateOf('10'),
        reject: (error: unknown) => request.mockRejectedValueOnce(error), resolve: () => request.mockResolvedValueOnce(undefined),
        key: () => request.mock.calls.at(-1)?.[2], calls: () => request.mock.calls.length,
      }
    },
  },
  {
    scope: 'stocktake.approve', safeCode: 60004, foreignCode: 40004,
    create: () => {
      const flow = useStocktakeApprove(), request = vi.mocked(approveStocktakeOrder)
      return {
        send: () => flow.approve('10'), restore: () => flow.restoreForOrders(['10']), state: () => flow.stateOf('10'),
        reject: (error: unknown) => request.mockRejectedValueOnce(error), resolve: () => request.mockResolvedValueOnce(undefined),
        key: () => request.mock.calls.at(-1)?.[1], calls: () => request.mock.calls.length,
      }
    },
  },
  {
    scope: 'outbound.pick', safeCode: 50008, foreignCode: 40004,
    create: () => {
      const flow = usePdaPick(), request = vi.mocked(pickPdaTask)
      return {
        send: () => flow.pick('10', { qty: 2, location_code: ' A-1 ', batch_no: ' B-1 ' }),
        restore: () => flow.restoreForTasks(['10']), state: () => flow.stateOf('10'),
        reject: (error: unknown) => request.mockRejectedValueOnce(error),
        resolve: () => request.mockResolvedValueOnce({ task_status: 'IN_PROGRESS', done_qty: 2, remaining_qty: 1, order_status: 'PICKING' }),
        key: () => request.mock.calls.at(-1)?.[2], calls: () => request.mock.calls.length,
      }
    },
  },
  {
    scope: 'outbound.claim', safeCode: 40017, foreignCode: 40004,
    create: () => {
      const flow = usePdaPick(), request = vi.mocked(claimPdaTask)
      return {
        send: () => flow.claim('10'), restore: () => flow.restoreForTasks(['10']), state: () => flow.stateOf('10'),
        reject: (error: unknown) => request.mockRejectedValueOnce(error),
        resolve: () => request.mockResolvedValueOnce({ claim_token: 'lease', lease_expire_at: '2030-01-01T00:00:00Z', task_status: 'IN_PROGRESS', done_qty: 0, remaining_qty: 3, order_status: 'PICKING' }),
        key: () => request.mock.calls.at(-1)?.[1], calls: () => request.mock.calls.length,
      }
    },
  },
] as const

beforeEach(() => {
  vi.resetAllMocks()
  vi.stubGlobal('localStorage', memoryStorage())
  vi.stubGlobal('sessionStorage', memoryStorage())
  setActivePinia(createPinia())
  login('1')
})
afterEach(() => vi.unstubAllGlobals())

for (const entry of cases) {
  describe(`${entry.scope} shared lifecycle contract`, () => {
    it('keeps the original key/payload for uncertain responses and releases it only after success', async () => {
      const flow = entry.create()
      let originalKey: string | undefined
      let originalPayload: unknown
      const errors = [
        { kind: 'network', code: entry.safeCode },
        { status: 401, code: entry.safeCode }, { status: 403, code: entry.safeCode },
        { status: 503, code: entry.safeCode }, { status: 503, code: 50302 },
        { code: entry.foreignCode }, { code: String(entry.safeCode) }, { code: 99999 },
      ]
      for (const error of errors) {
        flow.reject(error)
        await flow.send().catch(() => undefined)
        const saved = pending()[0]
        expect(saved?.scope).toBe(entry.scope)
        expect(saved?.state).toBe('uncertain')
        originalKey ??= flow.key()
        originalPayload ??= saved?.businessPayload
        expect(flow.key()).toBe(originalKey)
        expect(saved?.businessPayload).toEqual(originalPayload)
      }
      flow.resolve()
      await flow.send()
      expect(flow.key()).toBe(originalKey)
      expect(pending()).toHaveLength(0)
    })

    it('retains and blocks a conflicting key; a different command safe code cannot clear it', async () => {
      const flow = entry.create()
      flow.reject({ code: 40901, status: 409 })
      await flow.send().catch(() => undefined)
      expect(pending()).toHaveLength(1)
      expect(flow.state().blocked).toBe(true)
      await flow.send().catch(() => undefined)
      expect(flow.calls()).toBe(1)
    })

    it('allows a fresh key only after its own confirmed safe rejection', async () => {
      const flow = entry.create()
      flow.reject({ code: entry.safeCode, status: 400 })
      await flow.send().catch(() => undefined)
      const previous = flow.key()
      expect(pending()).toHaveLength(0)
      flow.resolve()
      await flow.send()
      expect(flow.key()).not.toBe(previous)
    })

    it.each([1, 2])('does not send when pending persistence step %i fails', async (step) => {
      vi.stubGlobal('localStorage', memoryStorage(step))
      const flow = entry.create()
      await flow.send().catch(() => undefined)
      expect(flow.calls()).toBe(0)
      expect(pending()).toHaveLength(0)
    })

    it('an old account response cannot clear or change the new account operation', async () => {
      const flow = entry.create()
      const request = entry.scope === 'inbound.receive' ? vi.mocked(receiveInbound) :
        entry.scope === 'inbound.putaway' ? vi.mocked(putawayInboundTask) :
          entry.scope === 'stocktake.approve' ? vi.mocked(approveStocktakeOrder) :
            entry.scope === 'outbound.pick' ? vi.mocked(pickPdaTask) : vi.mocked(claimPdaTask)
      let rejectOld!: (reason: unknown) => void
      let rejectNew!: (reason: unknown) => void
      request.mockReturnValueOnce(new Promise<never>((_, reject) => { rejectOld = reject }))
      request.mockReturnValueOnce(new Promise<never>((_, reject) => { rejectNew = reject }))
      const old = flow.send().catch(() => undefined)
      const oldRecord = pending()[0]
      expect(oldRecord).toBeDefined()
      login('2')
      const newer = flow.send().catch(() => undefined)
      const newRecord = pending('2')[0]
      expect(newRecord).toBeDefined()
      rejectOld({ code: entry.safeCode, status: 400 })
      await old
      expect(pending('1')[0]?.operationKey).toBe(oldRecord?.operationKey)
      expect(pending('2')[0]?.operationKey).toBe(newRecord?.operationKey)
      const state = flow.state()
      expect(state.submitting || ('claiming' in state && state.claiming)).toBe(true)
      rejectNew({ kind: 'network' })
      await newer
    })

    it('removes the exact storage listener when its Vue owner is disposed', () => {
      const target = new EventTarget()
      vi.stubGlobal('window', target)
      const add = vi.spyOn(target, 'addEventListener'), remove = vi.spyOn(target, 'removeEventListener')
      const owner = effectScope()
      const flow = owner.run(() => entry.create())!
      flow.restore()
      expect(add).toHaveBeenCalledTimes(1)
      owner.stop()
      expect(remove).toHaveBeenCalledWith('storage', add.mock.calls[0]?.[1])
    })
  })
}

it('re-reads ordinary/demo identities and preserves large string object IDs', () => {
  const objectId = '9223372036854775800'
  const ordinary = getOperationContext('收货')
  expect(ordinary.ctx?.storage).toBe(localStorage)
  expect(ordinary.ctx?.identity.sessionNamespace).toBe('')
  const original = operationStateKey(objectId)
  login('1', 'visitor-a')
  expect(getOperationContext('收货').ctx?.storage).toBe(sessionStorage)
  const firstVisitor = operationStateKey(objectId)
  login('1', 'visitor-b')
  expect(operationStateKey(objectId)).not.toBe(firstVisitor)
  expect(operationStateKey(objectId)).not.toBe(original)
  expect(JSON.parse(operationStateKey(objectId))[2]).toBe(objectId)
  useAuthStore().clear()
  expect(getOperationContext('收货').ctx).toBeNull()
})

it('keeps unknown error shapes uncertain and command-specific fallback messages intact', () => {
  for (const value of [null, undefined, 'failed', 40004, {}, { code: NaN }]) {
    expect(classifyOperationError(value, 'inbound.receive')).toBe('uncertain')
  }
  expect(classifyOperationError({ code: 40007 }, 'outbound.claim')).toBe('rejected')
  expect(classifyOperationError({ code: 40007 }, 'inbound.receive')).toBe('uncertain')
  expect(operationErrorMessage({}, '审核失败')).toBe('审核失败')
  expect(operationErrorMessage(new Error('保留原原因'), '审核失败')).toBe('保留原原因')
})
