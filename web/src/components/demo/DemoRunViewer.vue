<script setup lang="ts">
import { computed } from 'vue'
import { CircleCheckFilled, CircleCloseFilled, Clock } from '@element-plus/icons-vue'
import type { DemoScenarioResult, DemoScenarioStep } from '@/api/types'

type StepStatus = 'pending' | 'completed' | 'failed'

const props = defineProps<{ result: DemoScenarioResult }>()
const emit = defineEmits<{ navigate: [path: string] }>()

const steps = computed(() => props.result.steps || [])
const runStatus = computed<'completed' | 'failed'>(() =>
  props.result.status === 'failed' || steps.value.some((step) => stepStatus(step) === 'failed')
    ? 'failed'
    : 'completed',
)
const completedCount = computed(
  () => steps.value.filter((step) => stepStatus(step) === 'completed').length,
)
const progress = computed(() =>
  steps.value.length ? Math.round((completedCount.value / steps.value.length) * 100) : 0,
)
const activeIndex = computed(() => {
  const failedIndex = steps.value.findIndex((step) => stepStatus(step) === 'failed')
  if (failedIndex >= 0) return failedIndex
  return steps.value.length - 1
})

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

function navigate(path: string) {
  emit('navigate', path)
}
</script>

<template>
  <section
    class="run-viewer"
    :class="{ 'run-viewer--failed': runStatus === 'failed' }"
    aria-label="真实执行结果回放"
  >
    <header class="run-head">
      <div>
        <span class="run-kicker">真实执行结果回放</span>
        <h3>{{ result.summary }}</h3>
      </div>
      <el-tag :type="runStatus === 'failed' ? 'danger' : 'success'" effect="plain">
        {{ runStatus === 'failed' ? '执行失败' : '执行完成' }}
      </el-tag>
    </header>

    <div class="run-progress">
      <span>步骤完成</span>
      <b>{{ completedCount }} / {{ steps.length }}</b>
    </div>
    <el-progress
      :percentage="progress"
      :status="runStatus === 'failed' ? 'exception' : 'success'"
      :show-text="false"
      :stroke-width="7"
    />

    <div v-if="result.evidence?.length" class="run-evidence">
      <div class="evidence-head">
        <span>{{ result.evidence_title || '本次执行产生' }}</span>
        <small>以下数据来自本次真实业务执行</small>
      </div>
      <div class="evidence-grid">
        <div v-for="item in result.evidence" :key="item.label" class="evidence-item">
          <span>{{ item.label }}</span>
          <b>{{ item.value }}</b>
          <small v-if="item.detail">{{ item.detail }}</small>
        </div>
      </div>
    </div>

    <ol class="run-steps">
      <li
        v-for="(step, index) in steps"
        :key="`${step.title}-${index}`"
        class="run-step"
        :class="[`is-${stepStatus(step)}`, { 'is-active': index === activeIndex }]"
        :style="{ animationDelay: `${index * 45}ms` }"
      >
        <div class="step-marker" aria-hidden="true">
          <el-icon v-if="stepStatus(step) === 'completed'"><CircleCheckFilled /></el-icon>
          <el-icon v-else-if="stepStatus(step) === 'failed'"><CircleCloseFilled /></el-icon>
          <el-icon v-else><Clock /></el-icon>
        </div>

        <article class="step-content">
          <div class="step-head">
            <div>
              <b>{{ step.title }}</b>
              <span v-if="durationText(step.duration_ms)" class="step-duration">
                {{ durationText(step.duration_ms) }}
              </span>
            </div>
            <el-tag size="small" :type="tagType(stepStatus(step))" effect="light">
              {{ statusText(stepStatus(step)) }}
            </el-tag>
          </div>

          <p>{{ step.detail }}</p>

          <dl v-if="step.object || step.status_change || step.facts?.length" class="step-meta">
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

          <el-collapse v-if="step.technical" class="step-technical">
            <el-collapse-item title="查看技术实现" :name="`${index}`">
              <code>{{ step.technical }}</code>
            </el-collapse-item>
          </el-collapse>

          <el-alert
            v-if="step.error"
            class="step-error"
            type="error"
            :closable="false"
            show-icon
            :title="step.error"
          />
        </article>
      </li>
    </ol>

    <div v-if="result.links?.length" class="run-actions">
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

    <el-collapse v-if="result.implementation" class="run-implementation">
      <el-collapse-item title="查看实现" name="implementation">
        <dl>
          <div>
            <dt>场景编排文件</dt>
            <dd><code>{{ result.implementation.orchestration }}</code></dd>
          </div>
          <div>
            <dt>真实业务文件</dt>
            <dd>
              <code v-for="file in result.implementation.business_files" :key="file">{{ file }}</code>
            </dd>
          </div>
          <div>
            <dt>核心调用链</dt>
            <dd class="call-chain">
              <span v-for="stepName in result.implementation.call_chain" :key="stepName">
                {{ stepName }}
              </span>
            </dd>
          </div>
        </dl>
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

.run-progress b {
  color: var(--el-text-color-primary);
  font-family: var(--gowms-num-font);
  font-variant-numeric: tabular-nums;
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

.step-technical {
  margin-top: 8px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.step-technical :deep(.el-collapse-item__header) {
  height: 34px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.step-technical :deep(.el-collapse-item__wrap) {
  border-bottom: 0;
}

.step-technical code {
  color: var(--el-text-color-regular);
  font-size: 12px;
  overflow-wrap: anywhere;
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
  margin-top: 10px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.run-implementation :deep(.el-collapse-item__header) {
  height: 36px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.run-implementation :deep(.el-collapse-item__wrap) {
  border-bottom: 0;
}

.run-implementation dl {
  display: grid;
  gap: 11px;
  margin: 0;
}

.run-implementation dl > div {
  display: grid;
  grid-template-columns: 84px minmax(0, 1fr);
  gap: 8px;
}

.run-implementation dt {
  color: var(--el-text-color-placeholder);
  font-size: 12px;
}

.run-implementation dd {
  min-width: 0;
  margin: 0;
}

.run-implementation code {
  display: block;
  margin-bottom: 4px;
  overflow-wrap: anywhere;
  color: var(--el-text-color-regular);
  font-size: 11px;
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
  .run-step {
    animation: none;
  }
}
</style>
