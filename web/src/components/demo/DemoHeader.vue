<script setup lang="ts">
import { Share, VideoPlay } from '@element-plus/icons-vue'
import type { BusinessStep } from '@/composables/demo/useDemoHome'

defineProps<{ previewSteps: BusinessStep[] }>()
const emit = defineEmits<{
  'start-automatic': []
  'start-guided-inbound': []
  'open-architecture': []
}>()
</script>

<template>
  <section class="app-card home-hero">
    <div class="hero-copy">
      <span class="eyebrow">推荐体验 · 约 3 分钟</span>
      <h1>WMS 业务闭环</h1>
      <p>从入库、库存到出库，查看真实单据、库存变化与操作记录。</p>
      <div class="hero-actions">
        <el-button type="primary" :icon="VideoPlay" @click="emit('start-automatic')">
          开始自动演示
        </el-button>
        <el-button @click="emit('start-guided-inbound')">引导体验 · 入库</el-button>
        <el-button :icon="Share" @click="emit('open-architecture')">系统架构图</el-button>
      </div>
      <div class="proof-line" aria-label="演示环境能力">
        <span>真实业务接口</span>
        <span>独立演示数据</span>
        <span>操作记录可追溯</span>
      </div>
    </div>

    <aside class="hero-side">
      <h2>你将看到什么</h2>
      <p>选择业务步骤，右侧会显示对应说明、单据和操作记录。</p>
      <div class="preview-flow" aria-label="业务闭环预览">
        <template v-for="(step, index) in previewSteps" :key="step.key">
          <span class="preview-node">
            <b>{{ String(index + 1).padStart(2, '0') }}</b>
            {{ step.name }}
          </span>
          <i v-if="index < previewSteps.length - 1" aria-hidden="true">→</i>
        </template>
      </div>
    </aside>
  </section>
</template>

<style scoped>
.home-hero {
  flex: 0 0 176px;
  padding: 20px 22px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(340px, 0.85fr);
  gap: 24px;
  background: var(--el-fill-color-extra-light);
}

.hero-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
}

.eyebrow {
  color: var(--el-color-primary);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.06em;
}

.hero-copy h1 {
  margin: 7px 0 5px;
  font-size: clamp(26px, 2.2vw, 32px);
  line-height: 1.16;
  letter-spacing: -0.02em;
}

.hero-copy > p,
.hero-side > p {
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.65;
}

.hero-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}

.hero-actions .el-button {
  margin-left: 0;
}

.proof-line {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: auto;
  padding-top: 10px;
}

.proof-line span {
  padding: 3px 8px;
  border: 1px solid var(--el-color-success-light-7);
  border-radius: 999px;
  color: var(--el-color-success);
  background: var(--el-color-success-light-9);
  font-size: 11px;
}

.hero-side {
  min-width: 0;
  padding: 15px 16px;
  border: 1px solid var(--el-color-primary-light-8);
  border-radius: 12px;
  background: var(--el-color-primary-light-9);
}

.hero-side h2 {
  margin: 0 0 4px;
  font-size: 15px;
}

.preview-flow {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-top: 14px;
}

.preview-flow i {
  flex: none;
  color: var(--el-text-color-placeholder);
  font-style: normal;
}

.preview-node {
  min-width: 0;
  flex: 1;
  padding: 8px 6px;
  border: 1px solid var(--el-color-primary-light-8);
  border-radius: 8px;
  text-align: center;
  color: var(--el-text-color-regular);
  background: var(--el-bg-color);
  font-size: 11px;
}

.preview-node b {
  display: block;
  margin-bottom: 2px;
  color: var(--el-color-primary);
  font-size: 11px;
}

@media (max-height: 760px) and (min-width: 761px) {
  .home-hero {
    flex-basis: 154px;
    padding-top: 14px;
    padding-bottom: 14px;
  }

  .hero-side > p {
    display: none;
  }

  .proof-line {
    padding-top: 6px;
  }
}

@media (max-width: 980px) {
  .home-hero {
    grid-template-columns: 1fr;
    flex-basis: auto;
  }

  .preview-flow {
    flex-wrap: wrap;
  }
}
</style>