<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { CircleCheckFilled, CircleCloseFilled, Clock } from '@element-plus/icons-vue'
import type { DemoScenarioEvidence, DemoScenarioResult, DemoScenarioStep } from '@/api/types'

type StageStatus = 'completed' | 'active' | 'pending' | 'failed'

interface StageDefinition {
  key: string
  title: string
  subtitle: string
  description: string
}

interface StageView extends StageDefinition {
  steps: DemoScenarioStep[]
  status: StageStatus
}

const props = defineProps<{ result: DemoScenarioResult }>()
const emit = defineEmits<{ navigate: [path: string] }>()

const STAGE_DEFINITIONS: StageDefinition[] = [
  { key: 'inbound-order', title: '入库单', subtitle: '创建与审核', description: '创建并审核入库单，形成可执行的收货任务。' },
  { key: 'receiving', title: '收货上架', subtitle: '收货与上架', description: '真实登记收货批次并完成上架，库存事务同步落账。' },
  { key: 'inventory', title: '库存形成', subtitle: '库存与流水', description: '入库结果形成现存量、可用量，并保留可追溯流水。' },
  { key: 'outbound-order', title: '出库单', subtitle: '审核与 FIFO', description: '审核出库需求，并按 FIFO 锁定真实库存批次。' },
  { key: 'pick-ship', title: '拣货发货', subtitle: '任务与发货', description: '完成拣货任务并扣减库存，形成完整出库闭环。' },
]

const currentIndex = ref(0)

const runFailed = computed(() => props.result.status === 'failed' || props.result.steps.some((step) => step.status === 'failed'))
const businessTitle = computed(() => {
  if (runFailed.value) return '业务流程执行未完成'
  return props.result.name === 'full' ? '完整业务闭环已完成' : '本次业务执行已完成'
})

function stepIndexes(): number[][] {
  if (props.result.name === 'full') return [[0, 1, 2], [3, 4], [5], [6, 7, 8], [9]]
  if (props.result.name === 'outbound') return [[], [], [], [0, 1, 2], [3]]
  return [[0, 1, 2], [3, 4], [5], [], []]
}

function stageStatus(steps: DemoScenarioStep[]): StageStatus {
  if (!steps.length) return 'pending'
  if (steps.some((step) => step.status === 'failed')) return 'failed'
  if (steps.every((step) => step.status === 'completed')) return 'completed'
  if (steps.some((step) => step.status === 'completed')) return 'active'
  return 'pending'
}

const stages = computed<StageView[]>(() => {
  const indexes = stepIndexes()
  return STAGE_DEFINITIONS.map((definition, index) => {
    const steps = (indexes[index] ?? []).map((stepIndex) => props.result.steps[stepIndex]).filter(Boolean)
    return { ...definition, steps, status: stageStatus(steps) }
  })
})

const currentStage = computed(() => stages.value[currentIndex.value] ?? stages.value[0])
const currentStageLink = computed(() => {
  const links = props.result.links ?? []
  const stage = currentStage.value
  if (!stage) return ''
  if (stage.key === 'inbound-order') return links.find((link) => link.path.startsWith('/inbound/orders/'))?.path ?? '/inbound/orders'
  if (stage.key === 'receiving') return links.find((link) => link.label.includes('任务'))?.path ?? links.find((link) => link.path.startsWith('/tasks'))?.path ?? '/tasks'
  if (stage.key === 'inventory') return links.find((link) => link.path.startsWith('/inventory'))?.path ?? '/inventory'
  if (stage.key === 'outbound-order') return links.find((link) => link.path.startsWith('/outbound/orders/'))?.path ?? '/outbound/orders'
  return links.find((link) => link.path.startsWith('/tasks'))?.path ?? '/tasks'
})

function factValue(labels: string[]): string {
  for (const step of currentStage.value?.steps ?? []) {
    for (const fact of step.facts ?? []) {
      if (labels.some((label) => fact.label.includes(label))) return fact.value
    }
  }
  return '-'
}

const stageFacts = computed(() => {
  const stage = currentStage.value
  if (!stage?.steps.length) return []
  const completed = stage.steps.filter((step) => step.status === 'completed').length
  return [
    { label: '业务对象', value: stage.steps.find((step) => step.object)?.object || '-' },
    { label: '当前状态', value: statusText(stage.status) },
    { label: '数量变化', value: factValue(['入库数量', '收货数量', '上架数量', '出库需求', '分配汇总', '库存变化']) },
    { label: '库存变化', value: factValue(['库存变化', '现存量', '可用量']) },
    { label: '作业任务', value: factValue(['上架任务', 'PICK 任务数量', '流水任务']) },
    { label: '操作记录', value: completed + ' / ' + stage.steps.length + (completed === stage.steps.length ? ' 已记录' : ' 处理中') },
  ]
})

const summaryCards = computed(() => {
  const evidence = props.result.evidence ?? []
  const find = (labels: string[]) => evidence.find((item) => labels.some((label) => item.label.includes(label)))
  const findLast = (labels: string[]) => [...evidence].reverse().find((item) => labels.some((label) => item.label.includes(label)))

  return [
    { label: '入库单', item: find(['入库单']) },
    { label: '出库单', item: find(['出库单']) },
    { label: '作业任务', item: findLast(['作业任务', 'PICK 任务']) },
    { label: 'FIFO 分配', item: find(['FIFO 分配']) },
    { label: '库存流水', item: findLast(['库存流水']) },
    { label: '库存前后变化', item: findLast(['库存变化']) },
  ].filter((item): item is { label: string; item: DemoScenarioEvidence } => Boolean(item.item))
})

const progress = computed(() => Math.round(((currentIndex.value + 1) / stages.value.length) * 100))

function statusText(status: StageStatus): string {
  if (status === 'failed') return '失败'
  if (status === 'pending') return '待执行'
  if (status === 'active') return '进行中'
  return '已完成'
}

function startReplay(): void {
  currentIndex.value = Math.max(0, stages.value.length - 1)
}

function selectStage(index: number): void {
  currentIndex.value = index
}

function navigate(path: string): void {
  emit('navigate', path)
}

watch(
  () => [props.result.summary, props.result.status, props.result.steps.length] as const,
  startReplay,
  { immediate: true },
)

</script>

<template>
  <section class="run-viewer" :class="{ 'run-viewer--failed': runFailed }" aria-label="真实业务执行结果">
    <header class="run-header">
      <div>
        <span class="run-kicker">自动演示 · 真实业务执行</span>
        <h3>{{ businessTitle }}</h3>
        <p>{{ result.summary }}</p>
      </div>
      <el-tag :type="runFailed ? 'danger' : 'success'" effect="plain">{{ runFailed ? '执行失败' : '执行成功' }}</el-tag>
    </header>

    <el-progress :percentage="progress" :status="runFailed ? 'exception' : 'success'" :show-text="false" :stroke-width="5" />

    <div class="run-layout">
      <aside class="stage-list" aria-label="自动演示步骤">
        <span class="panel-title">业务步骤</span>
        <button
          v-for="(stage, index) in stages"
          :key="stage.key"
          type="button"
          class="stage-item"
          :class="{ active: currentIndex === index, completed: stage.status === 'completed', failed: stage.status === 'failed' }"
          @click="selectStage(index)"
        >
          <span class="stage-item__index">{{ String(index + 1).padStart(2, '0') }}</span>
          <span class="stage-item__copy"><b>{{ stage.title }}</b><small>{{ stage.subtitle }}</small></span>
          <el-icon v-if="stage.status === 'completed'"><CircleCheckFilled /></el-icon>
          <el-icon v-else-if="stage.status === 'failed'"><CircleCloseFilled /></el-icon>
          <el-icon v-else><Clock /></el-icon>
        </button>
      </aside>

      <main class="stage-focus">
        <div class="stage-focus__head">
          <span>当前步骤</span>
          <h4>{{ currentStage.title }}</h4>
          <p>{{ currentStage.description }}</p>
        </div>

        <div v-if="currentStage.steps.length" class="stage-step-list">
          <article v-for="step in currentStage.steps" :key="step.title" class="stage-step">
            <div>
              <b>{{ step.title }}</b>
              <el-tag size="small" :type="step.status === 'failed' ? 'danger' : step.status === 'pending' ? 'info' : 'success'" effect="light">
                {{ step.status === 'completed' ? '完成' : step.status === 'failed' ? '失败' : '待执行' }}
              </el-tag>
            </div>
            <p>{{ step.status_change || step.detail }}</p>
          </article>
        </div>
        <el-empty v-else description="当前流程尚未执行或不属于本次演示" :image-size="64" />
      </main>

      <aside class="stage-facts">
        <span class="panel-title">业务结果</span>
        <dl>
          <div v-for="item in stageFacts" :key="item.label"><dt>{{ item.label }}</dt><dd>{{ item.value }}</dd></div>
        </dl>
        <div class="stage-actions">
          <el-button size="small" :disabled="!currentStage.steps.length" @click="navigate(currentStageLink)">打开真实页面</el-button>
          <el-button size="small" type="primary" plain @click="navigate('/demo/activity?tab=operations')">查看操作日志</el-button>
        </div>
      </aside>
    </div>

    <footer class="run-footer">
      <div v-if="summaryCards.length" class="summary-strip">
        <div v-for="item in summaryCards" :key="item.label" :title="item.item.detail"><span>{{ item.label }}</span><b>{{ item.item.value }}</b></div>
      </div>

    </footer>
  </section>
</template>

<style scoped>
.run-viewer { display: grid; gap: 14px; color: var(--el-text-color-primary); }
.run-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
.run-kicker, .panel-title, .stage-focus__head > span { color: var(--el-color-primary); font-size: 11px; font-weight: 700; letter-spacing: 0.05em; }
.run-header h3 { margin: 4px 0 3px; font-size: 21px; }
.run-header p, .stage-focus__head p { margin: 0; color: var(--el-text-color-secondary); font-size: 13px; line-height: 1.6; }
.run-layout { min-height: 360px; display: grid; grid-template-columns: 210px minmax(0, 1fr) 260px; gap: 12px; }
.stage-list, .stage-focus, .stage-facts { min-width: 0; padding: 14px; border: 1px solid var(--el-border-color-light); border-radius: 12px; background: var(--el-fill-color-extra-light); }
.stage-list { display: grid; align-content: start; gap: 7px; }
.panel-title { display: block; margin-bottom: 4px; }
.stage-item { min-width: 0; padding: 10px; border: 1px solid transparent; border-radius: 9px; display: grid; grid-template-columns: 26px minmax(0, 1fr) 16px; align-items: center; gap: 8px; text-align: left; color: var(--el-text-color-regular); background: var(--el-bg-color); cursor: pointer; }
.stage-item.active { border-color: var(--el-color-primary-light-5); background: var(--el-color-primary-light-9); }
.stage-item.completed .stage-item__index, .stage-item.completed .el-icon { color: var(--el-color-success); }
.stage-item.failed .el-icon { color: var(--el-color-danger); }
.stage-item__index { color: var(--el-color-primary); font-family: var(--gowms-num-font); font-size: 12px; font-weight: 700; }
.stage-item__copy b, .stage-item__copy small { display: block; }
.stage-item__copy b { font-size: 13px; }
.stage-item__copy small { margin-top: 3px; color: var(--el-text-color-secondary); font-size: 11px; }
.stage-focus { background: var(--el-bg-color); }
.stage-focus__head h4 { margin: 4px 0 5px; font-size: 20px; }
.stage-step-list { display: grid; gap: 8px; margin-top: 14px; }
.stage-step { padding: 10px 11px; border-radius: 9px; background: var(--el-fill-color-lighter); }
.stage-step > div { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.stage-step b { font-size: 13px; }
.stage-step p { margin: 5px 0 0; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.55; }
.stage-facts { display: flex; flex-direction: column; }
.stage-facts dl { margin: 0; display: grid; gap: 7px; }
.stage-facts dl > div { min-width: 0; padding: 8px 9px; border-radius: 8px; background: var(--el-bg-color); }
.stage-facts dt { color: var(--el-text-color-secondary); font-size: 11px; }
.stage-facts dd { margin: 3px 0 0; overflow-wrap: anywhere; font-size: 12px; font-weight: 600; }
.stage-actions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: auto; padding-top: 12px; }
.stage-actions .el-button, .run-controls .el-button { margin-left: 0; }
.run-footer { display: grid; gap: 10px; }
.summary-strip { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 8px; }
.summary-strip > div { min-width: 0; padding: 9px 10px; border: 1px solid var(--el-border-color-lighter); border-radius: 9px; background: var(--el-fill-color-extra-light); }
.summary-strip span, .summary-strip b { display: block; }
.summary-strip span { color: var(--el-text-color-secondary); font-size: 10px; }
.summary-strip b { margin-top: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
.run-controls { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
@media (max-width: 980px) { .run-layout { grid-template-columns: 1fr; } .stage-list { grid-template-columns: repeat(5, minmax(0, 1fr)); } .stage-item { grid-template-columns: 1fr; } .stage-item__copy small, .stage-item .el-icon { display: none; } .summary-strip { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 640px) { .stage-list, .summary-strip { grid-template-columns: 1fr; } }
</style>
