<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowRight, Clock, Connection, Document, MagicStick, VideoPlay } from '@element-plus/icons-vue'
import type { DemoEvidenceContext } from '@/utils/demoEvidence'
const props = defineProps<{ evidence: DemoEvidenceContext | null }>()
const emit = defineEmits<{ 'start-automatic': []; 'start-step': []; 'open-evidence': [] }>()
const mode = ref<'auto' | 'step'>('auto')
const recentScenario = computed(() => {
  const labels: Record<string, string> = { full: '完整业务闭环', inbound: '入库业务流程', outbound: '出库业务流程' }
  return props.evidence ? labels[props.evidence.scenario] || '最近一次业务执行' : '完整业务闭环'
})
function start() { if (mode.value === 'auto') emit('start-automatic'); else emit('start-step') }
</script>

<template>
  <aside class="experience-panel" aria-label="演示体验">
    <div class="experience-top"><span><el-icon><MagicStick /></el-icon>推荐体验</span><small><el-icon><Clock /></el-icon>约 3 分钟</small></div>
    <h2>演示体验</h2><p class="experience-copy">选择运行方式，<br>走完一次完整的仓储业务链路。</p>
    <div class="experience-context"><el-icon><Connection /></el-icon><div><small>{{ evidence ? '最近执行记录' : '体验流程' }}</small><b>{{ recentScenario }}</b></div><span class="context-state">{{ evidence ? '已有记录' : '尚未开始' }}</span></div>
    <div class="experience-actions"><div class="mode-tabs" aria-label="选择完整演示方式"><button type="button" :aria-pressed="mode === 'auto'" @click="mode = 'auto'"><el-icon><VideoPlay /></el-icon>自动演示</button><button type="button" :aria-pressed="mode === 'step'" @click="mode = 'step'"><el-icon><Document /></el-icon>分步执行</button></div><el-button class="experience-start" type="primary" @click="start">{{ mode === 'auto' ? '开始自动演示' : '开始分步执行' }}<el-icon><ArrowRight /></el-icon></el-button></div>
    <div class="experience-footer"><span><i aria-hidden="true"></i>真实业务 · 操作可追溯</span><el-button link type="primary" @click="emit('open-evidence')">业务证据 →</el-button></div>
  </aside>
</template>

<style scoped>
.experience-panel { display: flex; flex-direction: column; min-width: 0; min-height: 0; padding: 23px; border: 1px solid var(--el-border-color-light); border-radius: 12px; background: linear-gradient(145deg, var(--el-bg-color) 25%, var(--el-color-primary-light-9)); color: var(--el-text-color-primary); }
.experience-top { display: flex; align-items: center; justify-content: space-between; gap: 10px; font-size: 11px; color: var(--el-color-primary); }
.experience-top > span, .experience-top small { display: flex; align-items: center; gap: 5px; }
.experience-top small { color: var(--el-text-color-secondary); font-size: 11px; }
h2 { font-size: 26px; line-height: 1.4; margin: 13px 0 0; letter-spacing: -0.02em; }
.experience-copy { font-size: 13px; line-height: 1.9; color: var(--el-text-color-secondary); margin: 7px 0 0; }
.experience-context { display: flex; align-items: center; gap: 10px; margin-top: 21px; padding: 13px 0; border-block: 1px solid var(--el-border-color-light); }
.experience-context > .el-icon { font-size: 20px; color: var(--el-color-primary); }
.experience-context small { display: block; font-size: 11px; color: var(--el-text-color-secondary); }
.experience-context b { display: block; font-size: 13px; font-weight: 500; margin-top: 3px; }
.context-state { margin-left: auto; white-space: nowrap; font-size: 11px; color: var(--el-text-color-secondary); }
.experience-actions { margin-top: auto; padding-top: 18px; }
.mode-tabs { display: flex; gap: 3px; padding: 3px; border-radius: 8px; border: 1px solid var(--el-border-color-light); background: var(--el-fill-color-extra-light); }
.mode-tabs button { flex: 1; display: flex; align-items: center; justify-content: center; gap: 6px; min-height: 33px; padding: 5px; border: 1px solid transparent; border-radius: 5px; background: transparent; color: var(--el-text-color-secondary); font-family: inherit; font-size: 12px; cursor: pointer; white-space: nowrap; }
.mode-tabs button[aria-pressed='true'] { color: var(--el-color-primary); background: var(--el-bg-color); border-color: var(--el-border-color-light); }
.mode-tabs button:focus-visible { outline: 2px solid var(--el-color-primary); outline-offset: 2px; }
.experience-start { width: 100%; margin: 11px 0 0; min-height: 38px; }
.experience-start :deep(> span) { width: 100%; display: flex; align-items: center; justify-content: space-between; }
.experience-footer { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 17px; padding-top: 12px; border-top: 1px solid var(--el-border-color-light); color: var(--el-text-color-secondary); font-size: 11px; }
.experience-footer > span { display: flex; align-items: center; gap: 5px; }
.experience-footer i { width: 5px; height: 5px; background: var(--el-color-success); border-radius: 50%; }
.experience-footer .el-button { font-size: 11px; margin: 0; padding: 0; }
@media (min-width: 981px) and (max-height: 800px) { .experience-panel { padding: 16px 19px; } h2 { font-size: 22px; margin-top: 8px; } .experience-copy { font-size: 12px; line-height: 1.75; margin-top: 5px; } .experience-context { margin-top: 12px; padding: 8px 0; } .experience-context small { display: none; } .experience-context b { font-size: 12px; margin: 0; } .context-state { font-size: 10px; } .experience-actions { padding-top: 11px; } .mode-tabs button { min-height: 28px; font-size: 11px; } .experience-start { min-height: 33px; margin-top: 8px; font-size: 12px; } .experience-footer { margin-top: 11px; padding-top: 9px; font-size: 10px; } .experience-footer .el-button { font-size: 10px; } }
@media (max-width: 980px) { .experience-panel { min-height: 330px; padding: 22px; } }
@media (min-width: 981px) and (max-height: 640px) {
  .experience-panel { padding: 14px 18px; }
  h2 { margin-top: 5px; }
  .experience-copy { font-size: 11px; line-height: 1.65; margin-top: 4px; }
  .experience-context { margin-top: 9px; padding: 7px 0; }
  .experience-actions { padding-top: 8px; }
  .mode-tabs button { padding: 3px 4px; }
  .experience-footer { margin-top: 8px; padding-top: 7px; }
}
@media (max-width: 640px) { .experience-panel { min-height: 340px; } }
</style>
