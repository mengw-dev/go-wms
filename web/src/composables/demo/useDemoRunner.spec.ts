import { describe, expect, it, vi } from 'vitest'
import { runDemoScenario } from '@/api/demo'
import { ApiError } from '@/api/request'
import { rememberDemoEvidence } from '@/utils/demoEvidence'
import { useDemoRunner } from './useDemoRunner'

vi.mock('@/api/demo', () => ({ runDemoScenario: vi.fn() }))
vi.mock('@/router', () => ({ default: { push: vi.fn() } }))
vi.mock('@/utils/demoEvidence', () => ({ rememberDemoEvidence: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn() } }))

describe('旧场景入口的单次后端编排', () => {
  it('完整流程只调用一次 full，避免分段重置清掉前段证据', async () => {
    vi.mocked(runDemoScenario).mockReset()
    const result = { name: 'full', status: 'completed' as const, summary: '完成', steps: [], links: [] }
    vi.mocked(runDemoScenario).mockResolvedValue(result)
    const runner = useDemoRunner()
    await runner.runScenario('full')
    expect(runDemoScenario).toHaveBeenCalledExactlyOnceWith('full')
    expect(runner.result.value).toEqual(result)
    expect(rememberDemoEvidence).toHaveBeenCalledWith(result, expect.any(String))
  })

  it('失败仍展示后端已完成的部分结果，不再另调下一段', async () => {
    vi.mocked(runDemoScenario).mockReset()
    const partial = { name: 'full', status: 'failed' as const, summary: '部分完成', steps: [{ title: '入库', detail: '完成', status: 'completed' as const }] }
    vi.mocked(runDemoScenario).mockRejectedValue(new ApiError('失败', 400, 400, partial))
    const runner = useDemoRunner()
    await runner.runScenario('full')
    expect(runDemoScenario).toHaveBeenCalledExactlyOnceWith('full')
    expect(runner.result.value).toEqual(partial)
    expect(runner.resultDialogVisible.value).toBe(true)
    expect(runner.scenarioRunning.value).toBe(false)
  })
})
