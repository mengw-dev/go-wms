<script setup lang="ts">
import { Connection, Lock, TopRight } from '@element-plus/icons-vue'
const emit = defineEmits<{ 'open-architecture': [] }>()
</script>

<template>
  <header class="demo-heading">
    <div><h1>WMS 业务闭环</h1><p>从第一张入库单，到最后一笔发货流水。</p></div>
    <div class="heading-actions">
      <span class="environment-note"><el-icon><Lock /></el-icon>独立演示环境</span>
      <el-button :icon="Connection" @click="emit('open-architecture')">系统架构图 <el-icon><TopRight /></el-icon></el-button>
    </div>
  </header>
</template>

<style scoped>
.demo-heading { display: flex; align-items: center; justify-content: space-between; gap: 20px; min-width: 0; }
h1 { margin: 0; font-size: clamp(24px, 2vw, 32px); line-height: 1.3; letter-spacing: -0.03em; }
p { margin: 6px 0 0; color: var(--el-text-color-secondary); font-size: 13px; line-height: 1.6; }
.heading-actions { display: flex; align-items: center; gap: 16px; flex-shrink: 0; }
.heading-actions .el-button { margin: 0; font-size: 12px; gap: 6px; }
.environment-note { display: flex; align-items: center; gap: 6px; color: var(--el-text-color-secondary); font-size: 11px; }
@media (max-width: 1180px) { .environment-note { display: none; } }
@media (max-width: 640px) { .demo-heading { align-items: flex-start; gap: 12px; } h1 { font-size: 24px; } p { font-size: 12px; max-width: 25em; } .heading-actions .el-button { padding: 8px; font-size: 11px; } }
</style>
