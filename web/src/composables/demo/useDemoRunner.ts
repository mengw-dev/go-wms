/**
 * 演示场景执行器（TASK 19）。
 *
 * 负责发起真实的后端 Demo scenario 调用并保存执行结果：running 状态、当前场景、
 * 结果对象与结果弹窗开关。结果只来自后端返回（失败时后端也会带上已执行的部分结果），
 * 前端不做任何"模拟成功"处理；页面只负责选择场景、触发执行与展示结果。
 */
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { runDemoScenario as requestDemoScenario } from '@/api/demo'
import { ApiError } from '@/api/request'
import type { DemoScenarioResult } from '@/api/types'
import { rememberDemoEvidence } from '@/utils/demoEvidence'
import { mergeDemoScenarioResults } from '@/utils/demoScenario'
import type { DemoConsoleScenario } from '@/utils/events'

export interface UseDemoRunnerOptions {
  /** 执行前检查，返回 false 时不发起请求（例如会话正在忙） */
  canRun?: () => boolean
  /** 发起执行前的清理动作（例如取消进行中的引导演示） */
  onBeforeRun?: () => void
  /** 一次执行结束后触发（成功或失败），用于刷新页面数据 */
  onSettled?: () => void
}

/** 完整闭环由入出库两次真实调用拼成，第二次失败时保留第一次的结果。 */
async function runFullScenario(): Promise<DemoScenarioResult> {
  const inbound = await requestDemoScenario('inbound')
  try {
    const outbound = await requestDemoScenario('outbound')
    return mergeDemoScenarioResults([inbound, outbound])
  } catch (error) {
    if (error instanceof ApiError && isDemoScenarioResult(error.data)) {
      return mergeDemoScenarioResults([inbound, error.data])
    }
    throw error
  }
}

function isDemoScenarioResult(value: unknown): value is DemoScenarioResult {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<DemoScenarioResult>
  return typeof candidate.summary === 'string' && Array.isArray(candidate.steps)
}

export function useDemoRunner(options: UseDemoRunnerOptions = {}) {
  const scenarioRunning = ref(false)
  const runningScenario = ref<DemoConsoleScenario | null>(null)
  const result = ref<DemoScenarioResult | null>(null)
  const resultDialogVisible = ref(false)

  function applyResult(demoResult: DemoScenarioResult, startedAt: string) {
    result.value = demoResult
    resultDialogVisible.value = true
    rememberDemoEvidence(demoResult, startedAt)
    options.onSettled?.()
  }

  /** 清空当前结果（重置演示数据时调用）。 */
  function clearResult() {
    result.value = null
    resultDialogVisible.value = false
  }

  /**
   * 执行一次演示场景。执行期间再次调用会被忽略，因此"重试"就是重新触发本方法。
   */
  async function runScenario(scenario: DemoConsoleScenario) {
    if (scenarioRunning.value) return
    if (options.canRun && !options.canRun()) return
    options.onBeforeRun?.()
    scenarioRunning.value = true
    runningScenario.value = scenario
    result.value = null
    resultDialogVisible.value = false
    const startedAt = new Date().toISOString()
    try {
      const demoResult = scenario === 'full' ? await runFullScenario() : await requestDemoScenario(scenario)
      applyResult(demoResult, startedAt)
      if (demoResult.status !== 'failed') {
        ElMessage.success(demoResult.summary)
      }
    } catch (error) {
      if (error instanceof ApiError && isDemoScenarioResult(error.data)) {
        applyResult(error.data, startedAt)
      }
    } finally {
      scenarioRunning.value = false
      runningScenario.value = null
    }
  }

  return {
    scenarioRunning,
    runningScenario,
    result,
    resultDialogVisible,
    runScenario,
    clearResult,
  }
}
