<script setup lang="ts">
import { CircleCheckFilled, Clock, Document, VideoPlay } from '@element-plus/icons-vue'
import {
  STAGED_EXECUTION_STEPS as EXECUTION_STEPS,
  STAGED_GROUPS as GROUPS,
  useStagedDemoRunner,
} from '@/composables/demo/useStagedDemoRunner'
import { formatTime } from '@/utils'

const emit = defineEmits<{
  navigate: [path: string]
  'state-change': [state: { started: boolean; active: boolean; completed: boolean; progress: number; running: boolean }]
}>()

const {
  mode,
  currentIndex,
  results,
  running,
  error,
  runStartedAt,
  runCompletedAt,
  currentStep,
  completed,
  progress,
  lastResult,
  groupStates,
  start,
  executeNext,
  validateCurrentPage,
} = useStagedDemoRunner({
  onStateChange: (state) => emit('state-change', state),
})

async function openCurrentPage(): Promise<void> {
  if (await validateCurrentPage()) {
    emit('navigate', lastResult.value?.route || '/demo')
  }
}

function openLogs(): void {
  const params = new globalThis.URLSearchParams({ tab: 'operations', source: 'staged', scenario: 'full' })
  if (runStartedAt.value) params.set('started_at', runStartedAt.value)
  if (runCompletedAt.value) params.set('completed_at', runCompletedAt.value)
  emit('navigate', '/demo/activity?' + params.toString())
}

defineExpose({ start })
</script>

<template>
  <section class="staged-runner" aria-label="分步执行自动演示">
    <header class="staged-head">
      <div>
        <span>自动演示 · 分步执行</span>
        <h3>{{ completed ? '全部流程已完成' : currentStep?.title }}</h3>
        <p>{{ completed ? '每一步都通过真实业务接口执行，可以继续打开页面核对。' : currentStep?.description }}</p>
      </div>
      <el-tag :type="completed ? 'success' : 'primary'" effect="plain">{{ completed ? '已完成' : `${currentIndex} / ${EXECUTION_STEPS.length}` }}</el-tag>
    </header>

    <el-progress :percentage="progress" :status="completed ? 'success' : undefined" :show-text="false" :stroke-width="5" />

    <div class="staged-layout">
      <aside class="stage-list">
        <span class="panel-title">业务步骤</span>
        <div v-for="(group, index) in GROUPS" :key="group.key" class="stage-item" :class="groupStates[index]">
          <span>{{ String(index + 1).padStart(2, '0') }}</span>
          <div><b>{{ group.title }}</b><small>{{ group.subtitle }}</small></div>
          <el-icon v-if="groupStates[index] === 'completed'"><CircleCheckFilled /></el-icon>
          <el-icon v-else><Clock /></el-icon>
        </div>
      </aside>

      <main class="stage-focus">
        <span class="panel-title">当前执行</span>
        <div v-if="!completed" class="next-step-card">
          <b>{{ currentStep?.title }}</b>
          <p>{{ currentStep?.description }}</p>
          <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
          <el-button v-if="mode === 'step'" type="primary" :icon="VideoPlay" :loading="running" @click="executeNext">执行下一步</el-button>
          <el-tag v-else type="primary" effect="plain">自动执行中，每步间隔约 0.9 秒</el-tag>
        </div>
        <el-result v-else icon="success" title="分步流程已完成" sub-title="所有步骤均调用真实业务接口完成。" />

        <div v-if="results.length" class="executed-list">
          <div v-for="item in results" :key="item.key" class="executed-item">
            <el-icon><CircleCheckFilled /></el-icon>
            <div><b>{{ item.title }}</b><span>{{ item.objectNo }} · {{ formatTime(item.createdAt) }}</span></div>
          </div>
        </div>
      </main>

      <aside class="stage-facts">
        <span class="panel-title">最近一步结果</span>
        <template v-if="lastResult">
          <dl>
            <div><dt>业务对象</dt><dd>{{ lastResult.objectNo }}</dd></div>
            <div><dt>当前状态</dt><dd>{{ lastResult.status }}</dd></div>
            <div><dt>状态变化</dt><dd>{{ lastResult.before }} → {{ lastResult.after }}</dd></div>
            <div><dt>数量变化</dt><dd>{{ lastResult.quantity }}</dd></div>
            <div v-for="fact in lastResult.facts" :key="fact.label"><dt>{{ fact.label }}</dt><dd>{{ fact.value }}</dd></div>
          </dl>
          <div class="stage-actions">
            <el-button size="small" :disabled="running" @click="openCurrentPage">打开真实页面</el-button>
            <el-button size="small" type="primary" plain :icon="Document" @click="openLogs">业务操作记录</el-button>
          </div>
        </template>
        <el-empty v-else description="点击“执行下一步”后显示真实结果" :image-size="62" />
      </aside>
    </div>
  </section>
</template>

<style scoped>
.staged-runner { display: grid; gap: 14px; color: var(--el-text-color-primary); }
.staged-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
.staged-head span, .panel-title { color: var(--el-color-primary); font-size: 11px; font-weight: 700; letter-spacing: 0.05em; }
.staged-head h3 { margin: 4px 0 3px; font-size: 21px; }
.staged-head p { margin: 0; color: var(--el-text-color-secondary); font-size: 13px; line-height: 1.6; }
.staged-layout { min-height: 360px; display: grid; grid-template-columns: 205px minmax(0, 1fr) 270px; gap: 12px; }
.stage-list, .stage-focus, .stage-facts { min-width: 0; padding: 14px; border: 1px solid var(--el-border-color-light); border-radius: 12px; background: var(--el-fill-color-extra-light); }
.stage-list { display: grid; align-content: start; gap: 7px; }
.stage-item { padding: 10px; border-radius: 9px; display: grid; grid-template-columns: 26px minmax(0, 1fr) 16px; align-items: center; gap: 8px; background: var(--el-bg-color); color: var(--el-text-color-regular); }
.stage-item.active { border: 1px solid var(--el-color-primary-light-5); background: var(--el-color-primary-light-9); }
.stage-item.completed .el-icon { color: var(--el-color-success); }
.stage-item b, .stage-item small { display: block; }
.stage-item small { margin-top: 3px; color: var(--el-text-color-secondary); font-size: 11px; }
.stage-focus { display: flex; flex-direction: column; background: var(--el-bg-color); }
.next-step-card { margin-top: 10px; padding: 14px; border-radius: 10px; background: var(--el-fill-color-lighter); }
.next-step-card b { font-size: 18px; }
.next-step-card p { margin: 6px 0 14px; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.6; }
.executed-list { display: grid; gap: 7px; margin-top: 14px; }
.executed-item { padding: 8px 9px; border-radius: 8px; display: flex; gap: 8px; color: var(--el-color-success); background: var(--el-color-success-light-9); }
.executed-item div { min-width: 0; }
.executed-item b, .executed-item span { display: block; }
.executed-item b { color: var(--el-text-color-primary); font-size: 12px; }
.executed-item span { margin-top: 3px; color: var(--el-text-color-secondary); font-size: 11px; }
.stage-facts { display: flex; flex-direction: column; }
.stage-facts dl { margin: 8px 0 0; display: grid; gap: 7px; }
.stage-facts dl > div { padding: 8px 9px; border-radius: 8px; background: var(--el-bg-color); }
.stage-facts dt { color: var(--el-text-color-secondary); font-size: 11px; }
.stage-facts dd { margin: 3px 0 0; overflow-wrap: anywhere; font-size: 12px; font-weight: 600; }
.stage-actions { display: flex; flex-wrap: wrap; gap: 7px; margin-top: auto; padding-top: 12px; }
.stage-actions .el-button { margin-left: 0; }
@media (max-width: 980px) { .staged-layout { grid-template-columns: 1fr; } .stage-list { grid-template-columns: repeat(5, minmax(0, 1fr)); } }
</style>
