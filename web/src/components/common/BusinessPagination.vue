<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

withDefaults(defineProps<{ layout?: string; pagerCount?: number }>(), {
  layout: 'total, sizes, prev, pager, next, jumper', pagerCount: 7,
})
defineOptions({ inheritAttrs: false })
const media = window.matchMedia('(max-width: 768px)')
const compact = ref(media.matches)
const sync = () => { compact.value = media.matches }
onMounted(() => media.addEventListener('change', sync))
onBeforeUnmount(() => media.removeEventListener('change', sync))
</script>

<template>
  <el-pagination v-bind="$attrs" class="business-pagination" :layout="layout" :pager-count="compact ? 5 : pagerCount" />
</template>
