<script setup lang="ts">
import type { PendingOperation } from '@/utils/pendingOperations'
defineProps<{ pending: PendingOperation | null; error: string; busy: boolean }>()
defineEmits<{ recover: []; discard: [] }>()
</script>

<template>
  <div v-if="pending || error" class="wave-notice">
    <el-alert :title="error || '有一笔波次操作结果待确认，请恢复原操作后继续'" type="warning" :closable="false" show-icon />
    <div v-if="pending" class="wave-notice-actions">
      <span>刷新或关闭页面会保留原操作，重试不会新建一笔操作。</span>
      <el-button type="primary" plain :loading="busy" @click="$emit('recover')">恢复原操作</el-button>
      <el-button :disabled="busy" @click="$emit('discard')">已核对，放弃记录</el-button>
    </div>
  </div>
</template>

<style scoped>
.wave-notice { margin-bottom: 12px; }
.wave-notice-actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-top: 8px; }
.wave-notice-actions span { font-size: 12px; color: var(--el-text-color-secondary); }
</style>
