<script setup lang="ts">
import { failedPage, refreshFailedPage } from '@/router/navigationRecovery'
</script>

<template>
  <el-dialog :model-value="!!failedPage" title="页面暂时无法打开" width="440px" class="business-dialog" :close-on-click-modal="false" @update:model-value="failedPage = null">
    <p>页面文件加载失败，可能是系统刚完成更新或网络中断。检查网络后，刷新即可重新打开目标页面。</p>
    <p class="recovery-note">刷新前请保存当前未提交的填写内容；刷新不会重新提交业务操作。</p>
    <template #footer>
      <el-button @click="failedPage = null">留在当前页</el-button>
      <el-button type="primary" @click="refreshFailedPage">刷新并打开</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
p { margin: 0 0 12px; line-height: 1.7; overflow-wrap: anywhere; }
.recovery-note { color: var(--el-text-color-secondary); font-size: 13px; }
</style>
