<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'

const TOUR_SEEN_KEY_PREFIX = 'WMS_DEMO_TOUR_SEEN:'

const auth = useAuthStore()
const open = ref(false)
const current = ref(0)
const emit = defineEmits<{ complete: [] }>()

const steps = [
  {
    target: '[data-tour="demo-session"]',
    title: '欢迎体验 WMS',
    description: '当前会话使用独立演示租户。不同访客数据隔离。退出或超时后自动恢复演示数据。',
    placement: 'bottom',
  },
  {
    target: '[data-tour="complete-flow"]',
    title: '推荐业务闭环',
    description: '这里调用真实业务 Service，不是直接生成已完成状态的数据。',
    placement: 'bottom',
  },
  {
    target: '[data-tour="demo-evidence"]',
    title: '结果与证据',
    description: '可以查看单据、任务、库存变化和库存流水。',
    placement: 'top',
  },
  {
    target: '[data-tour="demo-verification"]',
    title: '工程验证',
    description: '用于观察并发库存分配、并发拣货、事务和一致性行为，不夸大为严格数学证明。',
    placement: 'bottom',
  },
  {
    target: '[data-tour="demo-project"]',
    title: '项目与源码',
    description: '可以继续查看真实调用关系和源码位置。',
    placement: 'top',
  },
] as const

const nextButtonProps = computed(() =>
  current.value === steps.length - 1 ? { children: '开始体验完整业务闭环' } : undefined,
)

function seenKey(): string {
  return auth.demoSessionId ? `${TOUR_SEEN_KEY_PREFIX}${auth.demoSessionId}` : ''
}

function hasSeenTour(): boolean {
  const key = seenKey()
  return !!key && sessionStorage.getItem(key) === '1'
}

function markTourSeen(): void {
  const key = seenKey()
  if (key) sessionStorage.setItem(key, '1')
}

function openTour(): void {
  current.value = 0
  open.value = true
}

function closeTour(): void {
  markTourSeen()
  open.value = false
}

function completeTour(): void {
  closeTour()
  emit('complete')
}

onMounted(async () => {
  await nextTick()
  if (auth.demoSessionId && !hasSeenTour()) openTour()
})

defineExpose({ open: openTour })
</script>

<template>
  <el-tour
    v-model="open"
    v-model:current="current"
    append-to="body"
    :z-index="3000"
    @close="closeTour"
    @finish="completeTour"
  >
    <template #indicators>
      <el-button link type="primary" @click="closeTour">
        跳过导览
      </el-button>
    </template>
    <el-tour-step
      v-for="(step, index) in steps"
      :key="step.title"
      :target="step.target"
      :title="step.title"
      :description="step.description"
      :placement="step.placement"
      :show-close="true"
      :next-button-props="index === steps.length - 1 ? nextButtonProps : undefined"
    />
  </el-tour>
</template>
