<script setup lang="ts">
import type { BusinessStep } from '@/composables/demo/useDemoHome'

defineProps<{ steps: BusinessStep[]; selected: number }>()
const emit = defineEmits<{ select: [index: number] }>()
</script>

<template>
  <div class="flow-panel">
    <div class="flow-strip" role="tablist" aria-label="业务流程步骤">
      <template v-for="(step, index) in steps" :key="step.key">
        <button
          type="button"
          class="flow-step"
          :class="{ active: selected === index }"
          role="tab"
          :aria-selected="selected === index"
          @click="emit('select', index)"
        >
          <span>{{ String(index + 1).padStart(2, '0') }}</span>
          <b>{{ step.name }}</b>
          <small>{{ step.short }}</small>
        </button>
        <i v-if="index < steps.length - 1" class="flow-arrow" aria-hidden="true">→</i>
      </template>
    </div>
    <div class="flow-meta">
      <b>真实业务链路</b>
      <i>·</i>
      <span>单据、库存、任务和流水都会留下记录</span>
    </div>
  </div>
</template>

<style scoped>
.flow-panel {
  min-width: 0;
  min-height: 176px;
  padding: 14px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  background: var(--el-fill-color-extra-light);
}

.flow-strip {
  display: flex;
  align-items: stretch;
  gap: 5px;
}

.flow-step {
  min-width: 0;
  flex: 1;
  padding: 9px 5px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 9px;
  color: var(--el-text-color-regular);
  background: var(--el-bg-color);
  text-align: left;
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease;
}

.flow-step:hover,
.flow-step.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.flow-step span,
.flow-step b,
.flow-step small {
  display: block;
  min-width: 0;
}

.flow-step span {
  margin-bottom: 9px;
  color: var(--el-color-primary);
  font-size: 11px;
  font-weight: 800;
}

.flow-step b {
  font-size: 12px;
}

.flow-step small {
  margin-top: 5px;
  color: var(--el-text-color-secondary);
  font-size: 10px;
  line-height: 1.35;
}

.flow-arrow {
  align-self: center;
  flex: none;
  color: var(--el-text-color-placeholder);
  font-style: normal;
}

.flow-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: auto;
  padding-top: 14px;
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.flow-meta b {
  color: var(--el-color-success);
}

.flow-meta i {
  font-style: normal;
}

@media (max-height: 760px) and (min-width: 761px) {
  .flow-panel {
    min-height: 158px;
  }
}

@media (max-width: 980px) {
  .flow-strip {
    flex-wrap: wrap;
  }

  .flow-arrow {
    display: none;
  }

  .flow-step {
    flex: 1 0 calc(33.333% - 5px);
  }
}

@media (max-width: 680px) {
  .flow-step {
    flex-basis: calc(50% - 5px);
  }
}
</style>