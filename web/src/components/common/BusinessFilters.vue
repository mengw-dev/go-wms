<script setup lang="ts">
import { ref, useId } from 'vue'
import { Search, ArrowDown } from '@element-plus/icons-vue'

const expanded = ref(false)
const id = useId()
</script>

<template>
  <div class="business-filters" :class="{ 'is-expanded': expanded }">
    <button class="mobile-filter-toggle" type="button" :aria-expanded="expanded" :aria-controls="id" @click="expanded = !expanded">
      <el-icon><Search /></el-icon><span>搜索与筛选</span><small>{{ expanded ? '收起' : '展开' }}</small><el-icon><ArrowDown /></el-icon>
    </button>
    <div :id="id" class="business-filters__body"><slot /></div>
  </div>
</template>
