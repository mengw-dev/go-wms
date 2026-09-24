<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { CircleCheckFilled, CircleCloseFilled, Clock } from '@element-plus/icons-vue'
import type { DemoScenarioResult, DemoScenarioStep } from '@/api/types'

type StepStatus = 'pending' | 'completed' | 'failed'

const props = defineProps<{ result: DemoScenarioResult }>()
const emit = defineEmits<{ navigate: [path: string] }>()

const steps = computed(() => props.result.steps || [])
const replayIndex = ref(0)
const replayDone = ref(false)
let replayTimer: number | undefined
const runStatus = computed<'completed' | 'failed'>(() =>
  props.result.status === 'failed' || steps.value.some((step) => stepStatus(step) === 'failed')
    ? 'failed'
    : 'completed',
)
const completedCount = computed(() => {
  if (!steps.value.length) return 0
  return Math.min(replayIndex.value + 1, steps.value.length)
})
const progress = computed(() =>
  steps.value.length ? Math.round((completedCount.value / steps.value.length) * 100) : 0,
)
const currentReplayStep = computed(() => steps.value[replayIndex.value] ?? null)
const evidenceCards = computed(() => props.result.evidence ?? [])
const fifoRows = computed(() => {
  const rows: Array<{ batch: string; stockIn: string; available: string; allocated: string; location: string }> = []
  for (const step of steps.value) {
    for (const fact of step.facts ?? []) {
      if (!fact.label.startsWith('FIFO ')) continue
      rows.push({
        batch: fact.value.match(/批次 ([^/]+)/)?.[1]?.trim() || '-',
        stockIn: fact.value.match(/入库 ([^/]+)/)?.[1]?.trim() || '-',
        available: fact.value.match(/分配后可用 (\d+)/)?.[1] || '-',
        allocated: fact.value.match(/本次分配 (\d+)/)?.[1] || '-',
        location: fact.value.match(/库位 ([^/]+)/)?.[1]?.trim() || '-',
      })
    }
  }
  return rows
})
const quantityRelations = computed(() => {
  const labels = new Set(['现存量', '可用量', '已分配', '账面数量', '实盘数量', '差异', '确认差异', '库存变化'])
  const values = new Map<string, string>()
  for (const step of steps.value) {
    for (const fact of step.facts ?? []) {
      if (labels.has(fact.label)) values.set(fact.label, fact.value)
    }
  }
  return Array.from(values, ([label, value]) => ({ label, value }))
})

const technicalSteps = computed(() => steps.value.filter((step) => Boolean(step.technical)))
const technicalSourceGroups = computed(() => {
  const implementation = props.result.implementation
  if (!implementation) return []

  const businessFiles: string[] = []
  const capabilityFiles: string[] = []
  for (const file of implementation.business_files || []) {
    if (isCapabilityFile(file)) {
      capabilityFiles.push(file)
    } else {
      businessFiles.push(file)
    }
  }

  return [
    { label: 'Demo 编排', files: uniqueFiles([implementation.orchestration]) },
    { label: '真实业务', files: uniqueFiles(businessFiles) },
    { label: '库存 / 任务能力', files: uniqueFiles(capabilityFiles) },
  ].filter((group) => group.files.length > 0)
})

function isCapabilityFile(file: string): boolean {
  return file.includes('/inventory/service/') || file.includes('/task/service/')
}

function uniqueFiles(files: string[]): string[] {
  return Array.from(new Set(files.filter(Boolean)))
}

function stepStatus(step: DemoScenarioStep): StepStatus {
  if (step.status === 'pending' || step.status === 'failed' || step.status === 'completed') {
    return step.status
  }
  return 'completed'
}

function statusText(status: StepStatus): string {
  if (status === 'failed') return '失败'
  if (status === 'pending') return '待执行'
  return '完成'
}

function tagType(status: StepStatus): 'success' | 'warning' | 'danger' {
  if (status === 'failed') return 'danger'
  if (status === 'pending') return 'warning'
  return 'success'
}

function durationText(duration?: number): string {
  if (duration === undefined) return ''
  if (duration < 1) return '<1 ms'
  return `${duration} ms`
}

function clearReplayTimer(): void {
  if (replayTimer !== undefined) {
    window.clearTimeout(replayTimer)
    replayTimer = undefined
  }
}

function replayIntervalMs(): number {
  if (steps.value.length <= 1) return 0
  return Math.min(500, Math.max(320, Math.round(2500 / (steps.value.length - 1))))
}

function finishReplay(): void {
  replayIndex.value = Math.max(0, steps.value.length - 1)
  replayDone.value = true
  clearReplayTimer()
}

function replayNextStep(): void {
  const next = replayIndex.value + 1
  if (next >= steps.value.length) {
    replayDone.value = true
    return
  }
  replayIndex.value = next
  if (stepStatus(steps.value[next]) === 'failed') {
    replayDone.value = true
    return
  }
  replayTimer = window.setTimeout(replayNextStep, replayIntervalMs())
}

function startReplay(): void {
  clearReplayTimer()
  replayIndex.value = 0
  replayDone.value = steps.value.length <= 1
  if (!steps.value.length || steps.value.length === 1) return
  if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
    finishReplay()
    return
  }
  if (stepStatus(steps.value[0]) === 'failed') {
    replayDone.value = true
    return
  }
  replayTimer = window.setTimeout(replayNextStep, replayIntervalMs())
}

function showAllResults(): void {
  finishReplay()
}

function isStepReplayed(index: number): boolean {
  return index <= replayIndex.value
}

function displayedStepStatus(step: DemoScenarioStep, index: number): StepStatus {
  return isStepReplayed(index) ? stepStatus(step) : 'pending'
}

function displayedStatusText(step: DemoScenarioStep, index: number): string {
  if (isStepReplayed(index)) return statusText(stepStatus(step))
  if (replayDone.value && runStatus.value === 'failed') return '未执行'
  return '未回放'
}

function navigate(path: string) {
  emit('navigate', path)
}

watch(
  () => [props.result.summary, props.result.status, props.result.steps.length] as const,
  startReplay,
  { immediate: true },
)

onBeforeUnmount(clearReplayTimer)
</script>

<template>
  <section
    class="run-viewer"
    :class="{ 'run-viewer--failed': runStatus === 'failed' }"
    aria-label="真实执行结果回放"
  >
    <header class="run-head">
      <div>
        <span class="run-kicker">业务视角 · 真实执行结果回放</span>
        <h3>{{ result.summary }}</h3>
      </div>
      <el-tag :type="runStatus === 'failed' ? 'danger' : 'success'" effect="plain">
        {{ replayDone ? (runStatus === 'failed' ? '执行失败' : '结果已回放') : '结果回放中' }}
      </el-tag>
    </header>

    <div class="run-progress">
      <span>真实结果回放进度</span>
      <div>
        <b>{{ completedCount }} / {{ steps.length }}</b>
        <el-button v-if="!replayDone" link type="primary" size="small" @click="showAllResults">
          显示全部结果
        </el-button>
      </div>
    </div>
    <el-progress
      :percentage="progress"
      :status="runStatus === 'failed' ? 'exception' : 'success'"
      :show-text="false"
      :stroke-width="7"
    />

    <div v-if="currentReplayStep" class="replay-context">
      <span>当前回放步骤</span>
      <b>{{ currentReplayStep.title }}</b>
      <small>{{ currentReplayStep.status_change || currentReplayStep.detail }}</small>
    </div>

    <div v-if="replayDone" class="run-evidence">
      <div class="evidence-head">
        <span>{{ result.evidence_title || '本次执行产生' }}</span>
        <small>以下数据来自本次真实业务执行</small>
      </div>
      <div v-if="evidenceCards.length" class="evidence-grid">
        <div v-for="item in evidenceCards" :key="item.label" class="evidence-item">
          <span>{{ item.label }}</span>
          <b>{{ item.value }}</b>
          <small v-if="item.detail">{{ item.detail }}</small>
        </div>
      </div>

      <div v-if="fifoRows.length" class="business-subsection">
        <div class="business-subtitle"><b>FIFO 批次分配</b><span>按真实入库时间顺序展开</span></div>
        <el-table :data="fifoRows" border stripe size="small">
          <el-table-column prop="batch" label="批次" min-width="110" />
          <el-table-column prop="stockIn" label="入库时间" min-width="140" />
          <el-table-column prop="available" label="分配后可用" width="105" align="right" />
          <el-table-column prop="allocated" label="本次分配" width="95" align="right" />
          <el-table-column prop="location" label="库位" min-width="90" />
        </el-table>
      </div>

      <div v-if="quantityRelations.length" class="business-subsection">
        <div class="business-subtitle"><b>数量关系</b><span>执行前 / 执行后来自真实业务结果</span></div>
        <div class="quantity-grid">
          <div v-for="item in quantityRelations" :key="item.label">
            <span>{{ item.label }}</span>
            <b>{{ item.value }}</b>
          </div>
        </div>
      </div>
    </div>

    <ol class="run-steps">
      <li
        v-for="(step, index) in steps"
        :key="`${step.title}-${index}`"
        class="run-step"
        :class="[
          `is-${displayedStepStatus(step, index)}`,
          {
            'is-active': index === replayIndex,
            'is-replaying': !replayDone && index === replayIndex,
            'is-awaiting': !isStepReplayed(index),
          },
        ]"
      >
        <div class="step-marker" aria-hidden="true">
          <el-icon v-if="displayedStepStatus(step, index) === 'completed'"><CircleCheckFilled /></el-icon>
          <el-icon v-else-if="displayedStepStatus(step, index) === 'failed'"><CircleCloseFilled /></el-icon>
          <el-icon v-else><Clock /></el-icon>
        </div>

        <article class="step-content">
          <div class="step-head">
            <div>
              <b>{{ step.title }}</b>
              <span v-if="isStepReplayed(index) && durationText(step.duration_ms)" class="step-duration">
                {{ durationText(step.duration_ms) }}
              </span>
            </div>
            <el-tag size="small" :type="tagType(displayedStepStatus(step, index))" effect="light">
              {{ displayedStatusText(step, index) }}
            </el-tag>
          </div>

          <p v-if="isStepReplayed(index)">{{ step.detail }}</p>

          <dl v-if="isStepReplayed(index) && (step.object || step.status_change || step.facts?.length)" class="step-meta">
            <div v-if="step.object">
              <dt>业务对象</dt>
              <dd>{{ step.object }}</dd>
            </div>
            <div v-if="step.status_change">
              <dt>状态变化</dt>
              <dd>{{ step.status_change }}</dd>
            </div>
            <div v-for="fact in step.facts || []" :key="fact.label">
              <dt>{{ fact.label }}</dt>
              <dd>{{ fact.value }}</dd>
            </div>
          </dl>

          <el-alert
            v-if="isStepReplayed(index) && step.error"
            class="step-error"
            type="error"
            :closable="false"
            show-icon
            :title="step.error"
          />
        </article>
      </li>
    </ol>

    <div v-if="replayDone && result.links?.length" class="run-actions">
      <span>继续核对</span>
      <div>
        <el-button
          v-for="link in result.links"
          :key="link.path"
          type="primary"
          plain
          size="small"
          @click="navigate(link.path)"
        >
          {{ link.label }}
        </el-button>
      </div>
    </div>

    <el-collapse v-if="replayDone && result.implementation" class="run-implementation">
      <el-collapse-item title="技术视角 / 查看技术实现" name="implementation">
        <p class="technical-intro">
          以下入口对应本次真实调用。Demo 只负责编排，入库、出库、盘点和库存能力仍由现有业务 Service 完成。
        </p>

        <div v-if="result.implementation.call_chain?.length" class="technical-section">
          <span class="technical-label">调用链</span>
          <div class="call-chain">
            <span v-for="stepName in result.implementation.call_chain" :key="stepName">
              {{ stepName }}
            </span>
          </div>
        </div>

        <div v-if="technicalSourceGroups.length" class="technical-section">
          <span class="technical-label">关键源码入口</span>
          <div class="source-groups">
            <div v-for="group in technicalSourceGroups" :key="group.label" class="source-group">
              <b>{{ group.label }}</b>
              <code v-for="file in group.files" :key="file">{{ file }}</code>
            </div>
          </div>
        </div>

        <div v-if="technicalSteps.length" class="technical-section">
          <span class="technical-label">步骤调用</span>
          <ol class="technical-step-list">
            <li v-for="(step, index) in technicalSteps" :key="`${index}-${step.technical}`">
              <span class="technical-step-index">{{ index + 1 }}</span>
              <code>{{ step.technical }}</code>
            </li>
          </ol>
        </div>
      </el-collapse-item>
    </el-collapse>
  </section>
</template>

<style scoped>
.run-viewer {
  padding: 16px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  background: var(--el-bg-color);
}

.run-viewer--failed {
  border-color: var(--el-color-danger-light-5);
}

.run-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
}

.run-kicker {
  color: var(--el-color-primary);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.run-head h3 {
  margin: 5px 0 0;
  color: var(--el-text-color-primary);
  font-size: 15px;
  line-height: 1.55;
}

.run-progress {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 14px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.run-progress > div {
  display: flex;
  align-items: center;
  gap: 8px;
}

.run-progress b {
  color: var(--el-text-color-primary);
  font-family: var(--gowms-num-font);
  font-variant-numeric: tabular-nums;
}

.replay-context {
  margin-top: 12px;
  padding: 10px 12px;
  border-left: 3px solid var(--el-color-primary);
  border-radius: 6px;
  background: var(--el-fill-color-extra-light);
}

.replay-context span,
.replay-context b,
.replay-context small {
  display: block;
}

.replay-context span {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.replay-context b {
  margin-top: 3px;
  color: var(--el-text-color-primary);
  font-size: 13px;
}

.replay-context small {
  margin-top: 3px;
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.business-subsection {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--el-color-primary-light-8);
}

.business-subtitle {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 8px;
}

.business-subtitle b {
  font-size: 13px;
}

.business-subtitle span {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.quantity-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.quantity-grid div {
  padding: 10px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}

.quantity-grid span,
.quantity-grid b {
  display: block;
}

.quantity-grid span {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.quantity-grid b {
  margin-top: 4px;
  color: var(--el-text-color-primary);
  font-family: var(--gowms-num-font);
  font-size: 14px;
}

.run-steps {
  display: grid;
  gap: 0;
  margin: 14px 0 0;
  padding: 0;
  list-style: none;
}

.run-step {
  position: relative;
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  gap: 10px;
  padding-bottom: 14px;
  transition: opacity 180ms ease;
}

.run-step.is-awaiting {
  opacity: 0.48;
}

.run-step.is-replaying .step-content {
  margin: -6px;
  padding: 6px;
  border-radius: 8px;
  background: var(--el-color-primary-light-9);
  animation: run-step-fade 320ms ease both;
}

.run-step:not(:last-child)::before {
  position: absolute;
  top: 23px;
  bottom: 0;
  left: 11px;
  width: 2px;
  background: var(--el-border-color-lighter);
  content: '';
}

.run-step.is-completed:not(:last-child)::before {
  background: var(--el-color-success-light-5);
}

.run-step.is-failed:not(:last-child)::before {
  background: var(--el-color-danger-light-5);
}

.step-marker {
  position: relative;
  z-index: 1;
  display: grid;
  width: 24px;
  height: 24px;
  place-items: center;
  border: 1px solid var(--el-border-color);
  border-radius: 50%;
  color: var(--el-text-color-placeholder);
  background: var(--el-bg-color);
  font-size: 14px;
}

.is-completed .step-marker {
  border-color: var(--el-color-success);
  color: var(--el-color-success);
}

.is-failed .step-marker {
  border-color: var(--el-color-danger);
  color: var(--el-color-danger);
}

.is-active .step-marker {
  box-shadow: 0 0 0 4px var(--el-color-primary-light-9);
}

.is-failed.is-active .step-marker {
  box-shadow: 0 0 0 4px var(--el-color-danger-light-9);
}

.step-content {
  min-width: 0;
  padding: 2px 0 0;
}

.step-head,
.step-head > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.step-head > div {
  min-width: 0;
  justify-content: flex-start;
}

.step-head b {
  color: var(--el-text-color-primary);
  font-size: 14px;
}

.step-duration {
  flex: none;
  color: var(--el-text-color-placeholder);
  font-family: var(--gowms-num-font);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.step-content p {
  margin: 5px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.is-pending .step-content p,
.is-pending .step-head b {
  color: var(--el-text-color-placeholder);
}

.step-meta {
  display: grid;
  gap: 6px;
  margin: 9px 0 0;
}

.step-meta div {
  display: grid;
  grid-template-columns: 64px minmax(0, 1fr);
  gap: 8px;
  font-size: 12px;
  line-height: 1.5;
}

.step-meta dt {
  color: var(--el-text-color-placeholder);
}

.step-meta dd {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--el-text-color-regular);
}

.step-error {
  margin-top: 9px;
}

.run-evidence {
  margin-top: 15px;
  padding: 13px;
  border: 1px solid var(--el-color-primary-light-7);
  border-radius: 10px;
  background: var(--el-color-primary-light-9);
}

.evidence-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
}

.evidence-head span {
  color: var(--el-color-primary);
  font-size: 13px;
  font-weight: 700;
}

.evidence-head small {
  color: var(--el-text-color-secondary);
  font-size: 11px;
  text-align: right;
}

.evidence-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 11px;
}

.evidence-item {
  min-width: 0;
  padding: 10px;
  border: 1px solid var(--el-color-primary-light-8);
  border-radius: 8px;
  background: var(--el-bg-color);
}

.evidence-item span,
.evidence-item small {
  display: block;
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.evidence-item b {
  display: block;
  margin: 3px 0;
  overflow-wrap: anywhere;
  color: var(--el-text-color-primary);
  font-size: 13px;
}

.run-actions {
  display: grid;
  gap: 8px;
  margin-top: 14px;
  padding-top: 13px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.run-actions > span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.run-actions > div {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}

.run-actions .el-button {
  margin-left: 0;
}

.run-implementation {
  margin-top: 12px;
  padding: 0 12px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  background: var(--el-fill-color-extra-light);
}

.run-implementation :deep(.el-collapse-item__header) {
  height: 44px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  font-weight: 600;
}

.run-implementation :deep(.el-collapse-item__wrap) {
  border-bottom: 0;
  background: transparent;
}

.run-implementation :deep(.el-collapse-item__content) {
  padding-bottom: 14px;
}

.technical-intro {
  margin: 0 0 14px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.technical-section + .technical-section {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.technical-label {
  display: block;
  margin-bottom: 8px;
  color: var(--el-text-color-placeholder);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.03em;
}

.source-groups {
  display: grid;
  gap: 9px;
}

.source-group {
  display: grid;
  grid-template-columns: 86px minmax(0, 1fr);
  gap: 8px;
}

.source-group b {
  padding-top: 2px;
  color: var(--el-text-color-secondary);
  font-size: 11px;
  font-weight: 500;
}

.source-group code,
.technical-step-list code {
  display: block;
  min-width: 0;
  overflow-wrap: anywhere;
  color: var(--el-text-color-regular);
  font-family: var(--gowms-num-font);
  font-size: 11px;
  line-height: 1.6;
}

.technical-step-list {
  display: grid;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.technical-step-list li {
  display: grid;
  grid-template-columns: 86px minmax(0, 1fr);
  gap: 8px;
}

.technical-step-index {
  padding-top: 2px;
  color: var(--el-text-color-placeholder);
  font-family: var(--gowms-num-font);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.call-chain {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
}

.call-chain span {
  padding: 3px 6px;
  border-radius: 5px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
  font-size: 11px;
}

.call-chain span:not(:last-child)::after {
  margin-left: 5px;
  color: var(--el-text-color-placeholder);
  content: '→';
}

@keyframes run-step-fade {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .run-step,
  .step-content {
    transition: none;
    animation: none;
  }
}
</style>
