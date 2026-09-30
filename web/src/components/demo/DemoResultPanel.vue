<script setup lang="ts">
import type { BusinessStep } from '@/composables/demo/useDemoHome'

defineProps<{
  step: BusinessStep
  status: string
  objectText: string
  hasEvidence: boolean
}>()
const emit = defineEmits<{
  'open-business': []
  'start-guide': []
  'start-auto': []
  'open-records': []
}>()
</script>

<template>
  <article class="detail-panel" aria-live="polite">
    <div class="detail-head">
      <div>
        <span>当前步骤</span>
        <h3>{{ step.name }}</h3>
      </div>
      <el-tag :type="hasEvidence ? 'success' : 'info'" effect="plain">{{ status }}</el-tag>
    </div>
    <p>{{ step.description }}</p>
    <dl class="detail-lines">
      <div>
        <dt>关联对象</dt>
        <dd>{{ objectText }}</dd>
      </div>
      <div>
        <dt>数量变化</dt>
        <dd>{{ step.quantity }}</dd>
      </div>
      <div>
        <dt>证据</dt>
        <dd>{{ step.evidence }}</dd>
      </div>
    </dl>
    <div class="detail-actions">
      <el-button size="small" @click="emit('open-business')">
        {{ step.actionLabel }}
      </el-button>
      <el-button v-if="step.manualTarget !== 'inventory'" size="small" @click="emit('start-guide')">引导演示</el-button>
      <el-button size="small" @click="emit('start-auto')">自动演示</el-button>
      <el-button size="small" text type="primary" @click="emit('open-records')">查看操作日志</el-button>
    </div>
  </article>
</template>

<style scoped>
.detail-panel {
  min-width: 0;
  min-height: 176px;
  padding: 15px 16px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  background: var(--el-fill-color-extra-light);
}

.detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}

.detail-head > div > span {
  color: var(--el-color-primary);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.06em;
}

.detail-head h3 {
  margin: 4px 0 0;
  font-size: 18px;
}

.detail-panel > p {
  margin: 8px 0 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.55;
}

.detail-lines {
  margin: 0;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.detail-lines div {
  min-width: 0;
  padding: 8px 9px;
  border-radius: 8px;
  background: var(--el-bg-color);
}

.detail-lines dt {
  color: var(--el-text-color-secondary);
  font-size: 10px;
}

.detail-lines dd {
  margin: 4px 0 0;
  overflow-wrap: anywhere;
  font-size: 12px;
  font-weight: 700;
}

.detail-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: auto;
  padding-top: 12px;
}

.detail-actions .el-button {
  margin-left: 0;
}

@media (max-height: 760px) and (min-width: 761px) {
  .detail-panel {
    min-height: 158px;
  }

  .detail-panel > p {
    display: none;
  }

  .detail-lines div {
    padding: 6px 8px;
  }
}

@media (max-width: 680px) {
  .detail-lines {
    grid-template-columns: 1fr;
  }
}
</style>