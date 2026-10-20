<script setup lang="ts">
import { ArrowRight, Document, TopRight, VideoPlay } from '@element-plus/icons-vue'
import type { BusinessStep } from '@/composables/demo/useDemoHome'
defineProps<{ step: BusinessStep; status: string; objectText: string; hasEvidence: boolean }>()
const emit = defineEmits<{ 'open-business': []; 'start-guide': []; 'start-auto': []; 'open-records': [] }>()
</script>

<template>
  <article class="detail-panel" aria-live="polite">
    <div class="detail-body">
      <div class="detail-copy"><span class="detail-eyebrow">当前阶段</span><div class="detail-title"><h3>{{ step.name }}</h3><el-tag :type="hasEvidence ? 'success' : 'info'" effect="plain" size="small">{{ status }}</el-tag></div><p>{{ step.description }}</p></div>
      <div class="detail-evidence"><div class="evidence-label">本阶段的业务证据</div><dl><div><dt>关联对象</dt><dd>{{ objectText }}</dd></div><div><dt>数量变化</dt><dd>{{ step.quantity }}</dd></div><div><dt>核对入口</dt><dd>{{ step.evidence }}</dd></div></dl></div>
    </div>
    <div class="detail-actions">
      <el-button size="small" class="business-action" @click="emit('open-business')">{{ step.actionLabel }} <el-icon><TopRight /></el-icon></el-button>
      <el-button v-if="step.manualTarget !== 'inventory'" size="small" text @click="emit('start-guide')">引导演示 <el-icon><ArrowRight /></el-icon></el-button>
      <el-button size="small" text type="primary" :icon="VideoPlay" @click="emit('start-auto')">自动演示</el-button>
      <el-button size="small" text class="records-action" :icon="Document" @click="emit('open-records')">查看操作日志</el-button>
    </div>
  </article>
</template>

<style scoped>
.detail-panel { min-width: 0; min-height: 0; display: grid; grid-template-rows: minmax(0, 1fr) 32px; gap: 16px; padding-top: 16px; border-top: 1px solid var(--el-border-color-light); }
.detail-body { min-width: 0; display: grid; grid-template-columns: minmax(0, 1.05fr) minmax(0, 1fr); align-items: center; gap: 24px; }
.detail-copy { min-width: 0; }
.detail-eyebrow { color: var(--el-text-color-secondary); font-size: 11px; }
.detail-title { display: flex; align-items: center; flex-wrap: wrap; gap: 9px; margin-top: 5px; }
h3 { margin: 0; font-size: 26px; line-height: 1.3; letter-spacing: -0.02em; }
.detail-title .el-tag { font-size: 10px; }
.detail-copy p { margin: 7px 0 0; color: var(--el-text-color-secondary); font-size: 13px; line-height: 1.75; }
.detail-evidence { min-width: 0; border-left: 1px solid var(--el-border-color-light); padding-left: 22px; }
.evidence-label { color: var(--el-text-color-secondary); font-size: 11px; margin-bottom: 10px; }
dl { display: grid; gap: 8px; margin: 0; }
dl > div { display: flex; align-items: baseline; justify-content: space-between; gap: 14px; min-width: 0; }
dt { flex-shrink: 0; font-size: 12px; color: var(--el-text-color-secondary); }
dd { margin: 0; font-size: 12px; text-align: right; overflow-wrap: anywhere; }
.detail-actions { min-width: 0; display: flex; align-items: center; flex-wrap: wrap; gap: 9px; }
.detail-actions .el-button { margin: 0; font-size: 12px; padding: 6px 9px; }
.business-action { color: var(--el-color-primary); background: var(--el-color-primary-light-9); border-color: var(--el-color-primary-light-8); }
.detail-actions .el-icon { margin-left: 4px; }
.detail-actions .records-action { margin-left: auto; color: var(--el-text-color-secondary); }
@media (max-height: 720px) and (min-width: 981px) { .detail-panel { padding-top: 11px; gap: 12px; grid-template-rows: minmax(0, 1fr) 30px; } h3 { font-size: 22px; } .detail-copy p { font-size: 12px; margin-top: 5px; } .detail-title { margin-top: 4px; } .detail-body { gap: 18px; } .detail-evidence { padding-left: 17px; } .evidence-label { margin-bottom: 7px; } dl { gap: 6px; } dt, dd { font-size: 11px; } .detail-actions { gap: 5px; } .detail-actions .el-button { font-size: 11px; padding: 5px 7px; } }
@media (max-width: 1180px) and (min-width: 981px) { .detail-body { gap: 14px; } .detail-evidence { padding-left: 14px; } .detail-actions { gap: 3px; } .detail-actions .el-button { font-size: 11px; padding: 5px; } dt, dd { font-size: 11px; } .detail-copy p { font-size: 12px; } }
@media (max-width: 980px) { .detail-panel { min-height: 235px; grid-template-rows: 1fr auto; } .detail-body { gap: 18px; } .detail-actions { gap: 5px; } }
@media (max-width: 640px) { .detail-body { grid-template-columns: 1fr; gap: 17px; } .detail-evidence { border-left: 0; border-top: 1px solid var(--el-border-color-light); padding: 13px 0 0; } .detail-actions .el-button { font-size: 11px; padding: 5px 7px; } .detail-actions .records-action { margin-left: 0; } h3 { font-size: 25px; } }
</style>
