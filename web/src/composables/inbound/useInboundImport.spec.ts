import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { ElMessage } from 'element-plus'
import type { ImportTaskItem } from '@/api/types'
import { getImportStatus, importInboundExcel } from '@/api/inbound'
import { useInboundImport } from './useInboundImport'

// element-plus 在 node 环境会触碰真实 DOM（消息组件），这里整体替换成受控桩。
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
  genFileId: vi.fn(() => 'generated-uid'),
}))

// 上传与状态查询全部换成受控实现，便于断言调用次数与返回时序。
vi.mock('@/api/inbound', () => ({
  importInboundExcel: vi.fn(),
  getImportStatus: vi.fn(),
}))

/** node 环境没有 window，补齐轮询用到的定时器 */
function createWindowStub() {
  return {
    setTimeout: (handler: () => void, timeout?: number) =>
      globalThis.setTimeout(handler, timeout) as unknown as number,
    clearTimeout: (id: number) => {
      globalThis.clearTimeout(id)
    },
  }
}

function createDeferred<T>() {
  let resolve: (value: T) => void = () => {}
  let reject: (reason?: unknown) => void = () => {}
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function taskInfo(overrides: Partial<ImportTaskItem> = {}): ImportTaskItem {
  return {
    task_id: 'T-1',
    status: 'PENDING',
    file_name: 'inbound.xlsx',
    total_rows: 0,
    success_rows: 0,
    fail_rows: 0,
    error_msg: '',
    ...overrides,
  }
}

/** 让 await 之后的逻辑跑完 */
async function flushMicrotasks() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

const POLL_INTERVAL_MS = 2000

describe('useInboundImport 轮询', () => {
  const uploadMock = vi.mocked(importInboundExcel)
  const statusMock = vi.mocked(getImportStatus)

  beforeEach(() => {
    vi.useFakeTimers()
    vi.resetAllMocks()
    vi.stubGlobal('window', createWindowStub())
    // 组件外调用 onUnmounted 只会产生 Vue 警告，静默掉以保持输出干净
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  function createImport() {
    const onCompleted = vi.fn()
    const imp = useInboundImport(onCompleted)
    imp.importFile.value = new File(['demo'], 'inbound.xlsx')
    return { imp, onCompleted }
  }

  it('PENDING → RUNNING → COMPLETED 正常推进，终态后停止轮询', async () => {
    const first = createDeferred<ImportTaskItem>()
    uploadMock.mockResolvedValue({ task_id: 'T-1' })
    statusMock
      .mockReturnValueOnce(first.promise)
      .mockResolvedValue(taskInfo({ status: 'COMPLETED', total_rows: 3, success_rows: 3, fail_rows: 0 }))

    const { imp, onCompleted } = createImport()
    await imp.startImport()

    expect(uploadMock).toHaveBeenCalledTimes(1)
    // 上传成功后先本地展示 PENDING，第一次状态请求仍悬挂
    expect(imp.importInfo.value?.status).toBe('PENDING')
    expect(statusMock).toHaveBeenCalledTimes(1)

    first.resolve(taskInfo({ status: 'RUNNING' }))
    await flushMicrotasks()
    expect(imp.importInfo.value?.status).toBe('RUNNING')
    expect(statusMock).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)
    await flushMicrotasks()
    expect(statusMock).toHaveBeenCalledTimes(2)
    expect(imp.importInfo.value?.status).toBe('COMPLETED')
    expect(onCompleted).toHaveBeenCalledTimes(1)
    expect(vi.mocked(ElMessage.success)).toHaveBeenCalledWith('导入完成：成功 3 条，失败 0 条')

    // 已到终态：继续推进时间也不会再发起轮询
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 5)
    expect(statusMock).toHaveBeenCalledTimes(2)
  })

  it('上一次状态请求未返回时不会发出第二个轮询请求', async () => {
    const first = createDeferred<ImportTaskItem>()
    uploadMock.mockResolvedValue({ task_id: 'T-2' })
    statusMock.mockReturnValueOnce(first.promise).mockResolvedValue(taskInfo({ status: 'COMPLETED' }))

    const { imp } = createImport()
    await imp.startImport()
    expect(statusMock).toHaveBeenCalledTimes(1)

    // 请求悬挂期间推进远超一个轮询间隔的时间，仍然只有一个在途请求
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 5)
    expect(statusMock).toHaveBeenCalledTimes(1)

    // 只有请求返回后，才会安排下一次
    first.resolve(taskInfo({ status: 'RUNNING' }))
    await flushMicrotasks()
    expect(statusMock).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)
    expect(statusMock).toHaveBeenCalledTimes(2)
  })

  it('FAILED 状态记录错误信息、停止轮询，且不弹成功提示', async () => {
    uploadMock.mockResolvedValue({ task_id: 'T-3' })
    statusMock.mockResolvedValue(taskInfo({ status: 'FAILED', error_msg: '第 3 行 SKU 不存在', fail_rows: 1 }))

    const { imp, onCompleted } = createImport()
    await imp.startImport()
    await flushMicrotasks()

    expect(imp.importInfo.value?.status).toBe('FAILED')
    expect(imp.importInfo.value?.error_msg).toBe('第 3 行 SKU 不存在')
    expect(statusMock).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 5)
    expect(statusMock).toHaveBeenCalledTimes(1)

    // 源码在终态（含 FAILED）后统一回调 onCompleted，但只在 COMPLETED 时弹成功提示：
    // 这里 success 只应被上传提示调用过一次。
    expect(onCompleted).toHaveBeenCalledTimes(1)
    expect(vi.mocked(ElMessage.success)).toHaveBeenCalledTimes(1)
    expect(vi.mocked(ElMessage.success)).toHaveBeenCalledWith('文件已上传，开始解析导入')
  })

  it('状态请求失败时静默停止轮询，不会持续刷错误', async () => {
    uploadMock.mockResolvedValue({ task_id: 'T-4' })
    statusMock.mockRejectedValue(new Error('网络错误'))

    const { imp, onCompleted } = createImport()
    await imp.startImport()
    await flushMicrotasks()

    expect(statusMock).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 10)
    expect(statusMock).toHaveBeenCalledTimes(1)
    expect(onCompleted).not.toHaveBeenCalled()
    // silentError 的轮询失败不打扰用户
    expect(vi.mocked(ElMessage.error)).not.toHaveBeenCalled()
  })

  it('closeImport() 取消后，迟到的响应被 token 作废且不再轮询', async () => {
    const first = createDeferred<ImportTaskItem>()
    uploadMock.mockResolvedValue({ task_id: 'T-5' })
    statusMock.mockReturnValueOnce(first.promise)

    const { imp } = createImport()
    await imp.startImport()
    expect(statusMock).toHaveBeenCalledTimes(1)

    imp.closeImport()
    expect(imp.importDialog.visible).toBe(false)

    // 迟到的 RUNNING 响应不应写入状态（token 已失效）
    first.resolve(taskInfo({ status: 'RUNNING' }))
    await flushMicrotasks()
    expect(imp.importInfo.value?.status).toBe('PENDING')

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 5)
    expect(statusMock).toHaveBeenCalledTimes(1)
  })

  it('closeImport() 会清掉已排定的下一次轮询', async () => {
    uploadMock.mockResolvedValue({ task_id: 'T-6' })
    statusMock.mockResolvedValue(taskInfo({ status: 'RUNNING' }))

    const { imp } = createImport()
    await imp.startImport()
    await flushMicrotasks()
    expect(statusMock).toHaveBeenCalledTimes(1)

    imp.closeImport()
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 5)
    expect(statusMock).toHaveBeenCalledTimes(1)
  })

  // onUnmounted 注册的 stopPolling 在 node 环境（无组件实例）无法触发，
  // closeImport 内部走的是同一个 stopPolling，上面的用例验证了等价的清理行为。
})
