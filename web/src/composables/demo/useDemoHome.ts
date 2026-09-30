/**
 * 演示中心首页的页面逻辑：业务步骤定义、当前步骤选择、演示启动入口与页面跳转。
 *
 * 步骤数据只描述真实业务链路（入库 → 库存 → 出库），所有执行都会调用真实接口，
 * 这里不包含任何模拟行为。执行结果与证据展示见 useDemoEvidence。
 */
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { getGuideStep, useGuideStore, type GuideScenario } from '@/stores/guide'
import { runDemoAutomaticallyInConsole, runDemoStepByStepInConsole } from '@/utils/events'

export type AutomaticScenario = GuideScenario | 'full'
export type ManualTarget = 'inbound' | 'outbound' | 'inventory'

export interface BusinessStep {
  key: string
  name: string
  short: string
  description: string
  object: string
  status: string
  quantity: string
  evidence: string
  actionLabel: string
  manualTarget: ManualTarget
  automaticScenario: GuideScenario
}

export interface EngineeringCheck {
  key: 'allocation' | 'picking' | 'import'
  title: string
  description: string
  tag: string
  action: string
}

const ARCHITECTURE_URL = '/overview.html'

export const businessSteps: BusinessStep[] = [
  {
    key: 'inbound-order',
    name: '入库单',
    short: '创建与审核',
    description: '创建入库单并完成审核，形成可执行的收货任务。',
    object: '入库单（执行后生成）',
    status: '等待执行',
    quantity: '按计划入库',
    evidence: '操作记录与业务单据',
    actionLabel: '进入入库页面',
    manualTarget: 'inbound',
    automaticScenario: 'inbound',
  },
  {
    key: 'inbound-receive',
    name: '收货上架',
    short: '收货与上架',
    description: '按实收数量完成收货，上架后写入库存并生成库存流水。',
    object: '收货 / 上架任务',
    status: '等待前序步骤',
    quantity: '按实收数量增加',
    evidence: '任务记录与库存流水',
    actionLabel: '进入入库页面',
    manualTarget: 'inbound',
    automaticScenario: 'inbound',
  },
  {
    key: 'inventory-ready',
    name: '库存形成',
    short: '库存与流水',
    description: '收货结果形成可用库存，可继续核对库存数量和流水。',
    object: '库存与库存流水',
    status: '等待前序步骤',
    quantity: '现存量与可用量同步',
    evidence: '库存流水',
    actionLabel: '查看库存页面',
    manualTarget: 'inventory',
    automaticScenario: 'inbound',
  },
  {
    key: 'outbound-order',
    name: '出库单',
    short: '审核与分配',
    description: '审核出库单，按 FIFO 规则分配库存并生成拣货任务。',
    object: '出库单与拣货任务',
    status: '等待执行',
    quantity: '按分配数量扣减可用量',
    evidence: '库存流水与任务记录',
    actionLabel: '进入出库页面',
    manualTarget: 'outbound',
    automaticScenario: 'outbound',
  },
  {
    key: 'outbound-ship',
    name: '拣货发货',
    short: '任务与发货',
    description: '拣货任务完成后发货，形成完整出库闭环和操作记录。',
    object: '拣货任务与发货单据',
    status: '等待前序步骤',
    quantity: '按实际拣货数量发货',
    evidence: '任务记录与发货流水',
    actionLabel: '进入出库页面',
    manualTarget: 'outbound',
    automaticScenario: 'outbound',
  },
]

export const engineeringChecks: EngineeringCheck[] = [
  {
    key: 'allocation',
    title: '并发库存分配',
    description: '验证 FIFO 分配、库存边界和防超卖。',
    tag: '并发 · FIFO · 防超卖',
    action: '配置并运行',
  },
  {
    key: 'picking',
    title: '拣货作业验证',
    description: '观察并发拣货和重复扫码拦截。',
    tag: '并发拣货 · 重复扫码',
    action: '配置并运行',
  },
  {
    key: 'import',
    title: '异步导入可靠性',
    description: '查看任务恢复、重试和行级处理。',
    tag: '异步任务 · 可恢复',
    action: '查看机制',
  },
]

export function useDemoHome() {
  const router = useRouter()
  const guide = useGuideStore()
  const selectedStep = ref(0)
  const runModeDialogVisible = ref(false)

  const currentStep = computed(() => businessSteps[selectedStep.value] ?? businessSteps[0])
  const previewSteps = computed(() => [businessSteps[0], businessSteps[1], businessSteps[3], businessSteps[4]])

  function selectStep(index: number): void {
    selectedStep.value = index
  }

  function startAutomaticDemo(scenario: AutomaticScenario): void {
    if (scenario === 'full') {
      runModeDialogVisible.value = true
      return
    }
    if (scenario === 'stocktake') return
    runDemoAutomaticallyInConsole(scenario)
  }

  function startOneClickDemo(): void {
    runModeDialogVisible.value = false
    runDemoAutomaticallyInConsole('full')
  }

  function startStepByStepDemo(): void {
    runModeDialogVisible.value = false
    runDemoStepByStepInConsole()
  }

  function startSelectedAutomaticDemo(): void {
    startAutomaticDemo(currentStep.value.automaticScenario)
  }

  async function startGuidedExperience(scenario: Extract<GuideScenario, 'inbound' | 'outbound'>): Promise<void> {
    const firstStep = getGuideStep(scenario, 0)
    if (!firstStep) return
    await router.push(firstStep.route)
    guide.start(scenario)
  }

  function openArchitecture(): void {
    window.open(ARCHITECTURE_URL, '_blank', 'noopener,noreferrer')
  }

  function startSelectedGuide(): void {
    if (currentStep.value.manualTarget === 'inventory') return
    startGuidedExperience(currentStep.value.manualTarget)
  }

  function openSelectedBusinessPage(): void {
    if (currentStep.value.manualTarget === 'outbound') {
      void router.push('/outbound/orders')
      return
    }
    if (currentStep.value.manualTarget === 'inventory') {
      void router.push('/inventory')
      return
    }
    void router.push('/inbound/orders')
  }

  function openRecords(): void {
    void router.push({ path: '/demo/activity', query: { tab: 'operations' } })
  }

  function goPerformance(section?: string): void {
    if (section) {
      void router.push({ path: '/demo/performance', query: { section } })
      return
    }
    void router.push('/demo/performance')
  }

  return {
    businessSteps,
    engineeringChecks,
    selectedStep,
    selectStep,
    currentStep,
    previewSteps,
    runModeDialogVisible,
    startAutomaticDemo,
    startOneClickDemo,
    startStepByStepDemo,
    startSelectedAutomaticDemo,
    startGuidedExperience,
    startSelectedGuide,
    openArchitecture,
    openSelectedBusinessPage,
    openRecords,
    goPerformance,
  }
}