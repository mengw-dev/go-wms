<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { CircleCheckFilled, CircleCloseFilled, Clock, Document } from '@element-plus/icons-vue'
import type { DemoScenarioFact, DemoScenarioResult, DemoScenarioStep } from '@/api/types'

type StepStatus = 'pending' | 'completed' | 'failed'

const props = defineProps<{ result: DemoScenarioResult }>()
const emit = defineEmits<{ navigate: [path: string] }>()

const QUANTITY_LABELS = new Set(['现存量', '可用量', '已分配', '账面数量', '实盘数量', '差异', '确认差异', '库存变化'])

const steps = computed(() => props.result.steps || [])
const replayIndex = ref(0)
const replayDone = ref(false)
const technicalPanels = ref<string[]>([])
const stepsSection = ref<HTMLElement | null>(null)
let replayTimer: number | undefined

const runStatus = computed<'completed' | 'failed'>(() =>
  props.result.status === 'failed' || steps.value.some((step) => stepStatus(step) === 'failed')
    ? 'failed'
    : 'completed',
)
const businessTitle = computed(() => {
  if (runStatus.value === 'failed') return '业务流程执行失败'
  return props.result.name === 'full' ? '完整业务闭环已完成' : '本次业务执行已完成'
})
const failedStep = computed(() =>
  steps.value.find((step) => stepStatus(step) === 'failed') ?? null,
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
const technicalSteps = computed(() => steps.value.filter((step) => Boolean(step.technical)))

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
  const values = new Map<string, string>()
  for (const step of steps.value) {
    for (const fact of step.facts ?? []) {
      if (QUANTITY_LABELS.has(fact.label)) values.set(fact.label, fact.value)
    }
  }
  return Array.from(values, ([label, value]) => ({ label, value }))
})

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

const hasTechnicalDetails = computed(() =>
  Boolean(
    props.result.implementation ||
    technicalSteps.value.length ||
    fifoRows.value.length ||
    quantityRelations.value.length,
  ),
)

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

function businessFacts(step: DemoScenarioStep): DemoScenarioFact[] {
  return (step.facts ?? []).filter(
    (fact) => !fact.label.startsWith('FIFO ') && !QUANTITY_LABELS.has(fact.label),
  )
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

function showAllSteps(): void {
  finishReplay()
  void nextTick(() => stepsSection.value?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
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

function navigate(path: string): void {
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
    aria-label="真实业务执行结果"
  >
    <header class="summary-card" :class="{ failed: runStatus === 'failed' }">
      <div class="summary-copy">
        <span class="summary-kicker">真实业务执行结果</span>
        <h3>{{ businessTitle }}</h3>
        <p>{{ result.summary }}</p>
      </div>
      <el-tag :type="runStatus === 'failed' ? 'danger' : 'success'" effect="plain">
        {{ runStatus === 'failed' ? '执行失败' : '执行成功' }}
      </el-tag>
    </header>

    <el-alert
      v-if="failedStep"
      class="failure-alert"
      type="error"
      :closable="false"
      show-icon
      :title="failedStep.title"
      :description="failedStep.error || failedStep.detail"
    />

    <section class="business-summary">
      <div class="section-heading">
        <div>
          <span>业务摘要</span>
          <h4>{{ result.evidence_title || '本次执行产生' }}</h4>
        </div>
        <small>数据来自本次真实业务执行</small>
      </div>

      <div v-if="evidenceCards.length" class="evidence-grid">
        <article v-for="item in evidenceCards" :key="item.label" class="evidence-item">
          <span>{{ item.label }}</span>
          <b>{{ item.value }}</b>
          <small v-if="item.detail">{{ item.detail }}</small>
        </article>
      </div>
      <p v-else class="summary-empty">本次执行没有返回独立业务摘要，请继续查看业务步骤。</p>

      <div class="summary-actions">
        <el-button type="primary" :icon="Document" @click="navigate('/demo/activity')">
          查看业务证据
        </el-button>
        <el-button @click="showAllSteps">查看完整步骤</el-button>
      </div>
    </section>

    <section ref="stepsSection" class="business-steps">
      <div class="steps-head">
        <div>
          <span class="section-kicker">业务步骤</span>
          <h4>真实执行轨迹</h4>
        </div>
        <div class="progress-meta">
          <b>{{ completedCount }} / {{ steps.length }}</b>
          <el-button v-if="!replayDone" link type="primary" size="small" @click="finishReplay">
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

      <div v-if="currentReplayStep" class="current-step">
        <span>当前回放步骤</span>
        <b>{{ currentReplayStep.title }}</b>
        <small>{{ currentReplayStep.status_change || currentReplayStep.detail }}</small>
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

            <dl
              v-if="isStepReplayed(index) && (step.object || step.status_change || businessFacts(step).length)"
              class="step-meta"
            >
              <div v-if="step.object">
                <dt>业务对象</dt>
                <dd>{{ step.object }}</dd>
              </div>
              <div v-if="step.status_change">
                <dt>状态变化</dt>
                <dd>{{ step.status_change }}</dd>
              </div>
              <div v-for="fact in businessFacts(step)" :key="fact.label">
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
    </section>

    <section v-if="result.links?.length" class="object-links">
      <div>
        <span>继续核对</span>
        <b>业务对象</b>
      </div>
      <div class="object-link-actions">
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
    </section>

    <el-collapse v-if="hasTechnicalDetails" v-model="technicalPanels" class="technical-collapse">
      <el-collapse-item name="technical">
        <template #title>
          <div class="technical-title">
            <b>技术详情</b>
            <span>调用链、技术步骤、源码、FIFO、数量关系和接口记录</span>
          </div>
        </template>

        <p class="technical-intro">
          Demo 只负责编排，入库、出库、盘点和库存能力仍由现有业务 Service 完成。
        </p>

        <div v-if="result.implementation?.call_chain?.length" class="technical-section">
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
          <span class="technical-label">技术步骤</span>
          <ol class="technical-step-list">
            <li v-for="(step, index) in technicalSteps" :key="`${index}-${step.technical}`">
              <span class="technical-step-index">{{ index + 1 }}</span>
              <code>{{ step.technical }}</code>
            </li>
          </ol>
        </div>

        <div v-if="fifoRows.length" class="technical-section">
          <span class="technical-label">FIFO 详细拆解</span>
          <div class="table-scroll">
            <el-table :data="fifoRows" border stripe size="small">
              <el-table-column prop="batch" label="批次" min-width="110" />
              <el-table-column prop="stockIn" label="入库时间" min-width="140" />
              <el-table-column prop="available" label="分配后可用" width="105" align="right" />
              <el-table-column prop="allocated" label="本次分配" width="95" align="right" />
              <el-table-column prop="location" label="库位" min-width="90" />
            </el-table>
          </div>
        </div>

        <div v-if="quantityRelations.length" class="technical-section">
          <span class="technical-label">数量关系</span>
          <div class="quantity-grid">
            <div v-for="item in quantityRelations" :key="item.label">
              <span>{{ item.label }}</span>
              <b>{{ item.value }}</b>
            </div>
          </div>
        </div>

        <div class="technical-section interface-note">
          <span class="technical-label">接口记录</span>
          <p>接口调用属于底层排查信息，已放在业务证据页的“接口调用记录”中，不默认占用结果页首屏。</p>
          <el-button plain size="small" @click="navigate('/demo/activity')">
            查看接口调用记录
          </el-button>
        </div>
      </el-collapse-item>
    </el-collapse>
  </section>
</template>

<style scoped>
.run-viewer {
  min-width: 0;
  display: grid;
  gap: 16px;
}

.summary-card,
.business-summary,
.business-steps,
.object-links,
.technical-collapse {
  min-width: 0;
  border: 1px solid var(--el-border-color-light);
  border-radius: 14px;
  background: var(--el-bg-color);
}

.summary-card {
  padding: 20px;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  background:
    linear-gradient(135deg, var(--el-color-success-light-9), var(--el-bg-color));
}

.summary-card.failed {
  border-color: var(--el-color-danger-light-5);
  background: linear-gradient(135deg, var(--el-color-danger-light-9), var(--el-bg-color));
}

.summary-copy {
  min-width: 0;
}

.summary-kicker,
.section-kicker,
.section-heading > div > span,
.technical-label {
  color: var(--el-color-primary);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.05em;
}

.summary-card.failed .summary-kicker {
  color: var(--el-color-danger);
}

.summary-copy h3 {
  margin: 8px 0 6px;
  color: var(--el-text-color-primary);
  font-size: 23px;
}

.summary-copy p {
  margin: 0;
  color: var(--el-text-color-secondary);
  line-height: 1.7;
}

.failure-alert {
  margin: 0;
}

.business-summary,
.business-steps,
.object-links,
.technical-collapse {
  padding: 18px;
}

.section-heading,
.steps-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.section-heading h4,
.steps-head h4 {
  margin: 5px 0 0;
  color: var(--el-text-color-primary);
  font-size: 18px;
}

.section-heading > small {
  max-width: 360px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
  text-align: right;
}

.evidence-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
  margin-top: 16px;
}

.evidence-item {
  min-width: 0;
  padding: 14px;
  border-radius: 10px;
  background: var(--el-fill-color-extra-light);
}

.evidence-item span,
.evidence-item b,
.evidence-item small {
  display: block;
}

.evidence-item span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.evidence-item b {
  margin-top: 6px;
  overflow-wrap: anywhere;
  color: var(--el-text-color-primary);
  font-family: var(--gowms-num-font);
  font-size: 18px;
}

.evidence-item small {
  margin-top: 6px;
  overflow-wrap: anywhere;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.summary-empty {
  margin: 16px 0 0;
  padding: 14px;
  border-radius: 10px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-lighter);
  font-size: 13px;
}

.summary-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 16px;
}

.progress-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--el-text-color-secondary);
}

.progress-meta b {
  font-family: var(--gowms-num-font);
}

.current-step {
  display: grid;
  gap: 3px;
  margin: 14px 0 4px;
  padding: 12px 14px;
  border-left: 3px solid var(--el-color-primary);
  border-radius: 8px;
  background: var(--el-color-primary-light-9);
}

.current-step span,
.current-step small {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.current-step b {
  color: var(--el-text-color-primary);
}

.run-steps {
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
}

.run-step {
  position: relative;
  display: grid;
  grid-template-columns: 32px minmax(0, 1fr);
  gap: 12px;
  padding: 12px 0;
}

.run-step:not(:last-child)::after {
  content: '';
  position: absolute;
  left: 15px;
  top: 43px;
  bottom: -7px;
  width: 1px;
  background: var(--el-border-color-light);
}

.step-marker {
  width: 32px;
  height: 32px;
  z-index: 1;
  display: grid;
  place-items: center;
  border-radius: 50%;
  color: var(--el-text-color-placeholder);
  background: var(--el-fill-color-light);
}

.run-step.is-completed .step-marker {
  color: var(--el-color-success);
  background: var(--el-color-success-light-9);
}

.run-step.is-failed .step-marker {
  color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
}

.run-step.is-active .step-marker {
  box-shadow: 0 0 0 4px color-mix(in srgb, currentColor 14%, transparent);
}

.step-content {
  min-width: 0;
  padding: 12px 14px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  background: var(--el-fill-color-extra-light);
}

.run-step.is-replaying .step-content {
  border-color: var(--el-color-primary-light-5);
}

.run-step.is-awaiting .step-content {
  opacity: 0.64;
}

.step-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.step-head > div {
  min-width: 0;
}

.step-head b {
  color: var(--el-text-color-primary);
}

.step-duration {
  margin-left: 8px;
  color: var(--el-text-color-placeholder);
  font-size: 12px;
}

.step-content > p {
  margin: 9px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.step-meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 8px;
  margin: 12px 0 0;
}

.step-meta div {
  min-width: 0;
  padding: 9px 10px;
  border-radius: 8px;
  background: var(--el-bg-color);
}

.step-meta dt {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.step-meta dd {
  margin: 4px 0 0;
  overflow-wrap: anywhere;
  color: var(--el-text-color-regular);
  font-size: 12px;
}

.step-error {
  margin-top: 10px;
}

.object-links {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.object-links > div:first-child {
  display: grid;
  gap: 4px;
}

.object-links span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.object-link-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.object-link-actions .el-button {
  margin-left: 0;
}

.technical-title {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}

.technical-title b {
  color: var(--el-text-color-primary);
}

.technical-title span {
  min-width: 0;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.technical-intro {
  margin: 0 0 16px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.technical-section + .technical-section {
  margin-top: 18px;
}

.technical-label {
  display: block;
  margin-bottom: 8px;
}

.call-chain {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}

.call-chain span {
  padding: 6px 9px;
  border-radius: 7px;
  color: var(--el-text-color-regular);
  background: var(--el-fill-color-light);
  font-family: var(--gowms-num-font);
  font-size: 12px;
}

.source-groups {
  display: grid;
  gap: 10px;
}

.source-group {
  min-width: 0;
  padding: 11px;
  border-radius: 9px;
  background: var(--el-fill-color-extra-light);
}

.source-group b {
  display: block;
  margin-bottom: 7px;
  color: var(--el-text-color-primary);
  font-size: 13px;
}

.source-group code,
.technical-step-list code {
  display: block;
  overflow-wrap: anywhere;
  color: var(--el-text-color-regular);
  font-size: 12px;
}

.technical-step-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 7px;
}

.technical-step-list li {
  display: grid;
  grid-template-columns: 22px minmax(0, 1fr);
  align-items: start;
  gap: 8px;
}

.technical-step-index {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  border-radius: 6px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  font-size: 11px;
}

.table-scroll {
  max-width: 100%;
  overflow-x: auto;
}

.quantity-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 8px;
}

.quantity-grid div {
  padding: 11px;
  border-radius: 9px;
  background: var(--el-fill-color-extra-light);
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
  margin-top: 5px;
  overflow-wrap: anywhere;
  color: var(--el-text-color-primary);
}

.interface-note p {
  margin: 0 0 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.7;
}

@media (max-width: 700px) {
  .summary-card,
  .section-heading,
  .steps-head,
  .object-links,
  .technical-title {
    flex-direction: column;
  }

  .section-heading > small {
    max-width: none;
    text-align: left;
  }

  .object-link-actions {
    justify-content: flex-start;
  }

  .run-step {
    grid-template-columns: 28px minmax(0, 1fr);
    gap: 9px;
  }

  .step-marker {
    width: 28px;
    height: 28px;
  }

  .run-step:not(:last-child)::after {
    left: 13px;
    top: 39px;
  }
}
</style>