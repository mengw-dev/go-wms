/**
 * 演示首页的业务证据读取：订阅业务数据变更事件，把最近一次真实执行结果
 * 匹配到当前业务步骤，供详情面板展示"关联对象 / 执行状态"。
 *
 * 证据由 rememberDemoEvidence 从后端返回结果写入，这里只做读取和匹配。
 */
import { computed, onMounted, onUnmounted, ref, type ComputedRef } from 'vue'
import { onDataChanged } from '@/utils/events'
import { readDemoEvidence } from '@/utils/demoEvidence'
import type { BusinessStep } from './useDemoHome'

// 步骤 → 证据标签：命中任一标签即认为该证据属于当前步骤。
const STEP_EVIDENCE_LABELS: Record<string, string[]> = {
  'inbound-order': ['入库单'],
  'inbound-receive': ['收货', '上架'],
  'inventory-ready': ['库存'],
  'outbound-order': ['出库单', 'FIFO', '分配'],
  'outbound-ship': ['拣货', '发货'],
}

export function useDemoEvidence(currentStep: ComputedRef<BusinessStep>) {
  const recentEvidence = ref(readDemoEvidence())
  let stopDataChanged: (() => void) | undefined

  // 订阅跟随页面生命周期；重新进入页面时先重读一次，避免展示过期证据。
  onMounted(() => {
    recentEvidence.value = readDemoEvidence()
    stopDataChanged = onDataChanged(() => {
      recentEvidence.value = readDemoEvidence()
    })
  })
  onUnmounted(() => stopDataChanged?.())

  const currentEvidence = computed(() => {
    const context = recentEvidence.value
    if (!context) return null
    const candidates = STEP_EVIDENCE_LABELS[currentStep.value.key] ?? []
    return (context.evidence ?? []).find((item) => candidates.some((label) => item.label.includes(label))) ?? null
  })
  const currentObject = computed(() => {
    if (currentEvidence.value) return `${currentEvidence.value.label}：${currentEvidence.value.value}`
    return currentStep.value.object
  })
  const currentStatus = computed(() => (recentEvidence.value ? '已有执行记录' : currentStep.value.status))

  return { recentEvidence, currentEvidence, currentObject, currentStatus }
}