<script setup lang="ts">
import { Box, Coin, DocumentAdd, DocumentRemove, Van } from '@element-plus/icons-vue'
import type { BusinessStep } from '@/composables/demo/useDemoHome'
defineProps<{ steps: BusinessStep[]; selected: number }>()
const emit = defineEmits<{ select: [index: number] }>()
const icons = [DocumentAdd, Box, Coin, DocumentRemove, Van]
</script>

<template>
  <div class="flow-strip" aria-label="业务流程步骤">
    <button v-for="(step, index) in steps" :key="step.key" type="button" class="flow-step" :class="{ active: selected === index }" :aria-pressed="selected === index" :aria-label="step.name" @click="emit('select', index)">
      <span class="step-icon"><el-icon><component :is="icons[index]" /></el-icon></span><b>{{ step.name }}</b>
    </button>
  </div>
</template>

<style scoped>
.flow-strip { min-width: 0; min-height: 0; display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); align-items: center; position: relative; }
.flow-strip::before { content: ''; position: absolute; left: 10%; right: 10%; top: calc(50% - 15px); height: 1px; background: var(--el-border-color-light); }
.flow-step { min-width: 0; position: relative; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 9px; padding: 4px 2px; border: 0; background: transparent; border-radius: 8px; font-family: inherit; cursor: pointer; }
.step-icon { display: grid; place-items: center; width: 42px; height: 42px; border: 1px solid var(--el-border-color-light); border-radius: 12px; color: var(--el-text-color-secondary); background: var(--el-bg-color); box-shadow: 0 0 0 7px var(--el-bg-color); transition: background-color 0.16s, color 0.16s, border-color 0.16s; }
.step-icon .el-icon { font-size: 21px; }
.flow-step b { font-size: 13px; font-weight: 500; color: var(--el-text-color-secondary); white-space: nowrap; }
.flow-step.active .step-icon { border-color: var(--el-color-primary); color: var(--gowms-primary-button-text); background: var(--el-color-primary); }
.flow-step.active b { color: var(--el-text-color-primary); font-weight: 600; }
.flow-step:hover .step-icon { border-color: var(--el-color-primary); }
.flow-step:focus-visible { outline: 2px solid var(--el-color-primary); outline-offset: 4px; }
@media (min-width: 1600px) and (min-height: 800px) { .step-icon { width: 48px; height: 48px; border-radius: 14px; } .step-icon .el-icon { font-size: 24px; } .flow-step b { font-size: 15px; } .flow-strip::before { top: calc(50% - 16px); } }
@media (max-height: 720px), (max-width: 1180px) { .step-icon { width: 34px; height: 34px; border-radius: 10px; } .step-icon .el-icon { font-size: 18px; } .flow-step { gap: 7px; } .flow-step b { font-size: 12px; } }
@media (max-width: 480px) { .flow-step b { font-size: 11px; } .step-icon { width: 30px; height: 30px; box-shadow: 0 0 0 4px var(--el-bg-color); } }
</style>
