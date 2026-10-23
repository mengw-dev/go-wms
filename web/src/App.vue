<script setup lang="ts">
import { ElConfigProvider } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import PageLoadRecovery from '@/components/common/PageLoadRecovery.vue'
</script>

<template>
  <ElConfigProvider :locale="zhCn">
    <router-view />
    <PageLoadRecovery />
  </ElConfigProvider>
</template>
