<script setup lang="ts">
import { ref } from 'vue'
import { ArrowRight } from '@element-plus/icons-vue'
import DemoHeader from '@/components/demo/DemoHeader.vue'
import DemoResultPanel from '@/components/demo/DemoResultPanel.vue'
import ScenarioSelector from '@/components/demo/ScenarioSelector.vue'
import { useDemoEvidence } from '@/composables/demo/useDemoEvidence'
import { engineeringChecks, useDemoHome, type EngineeringCheck } from '@/composables/demo/useDemoHome'

const {
  businessSteps,
  selectedStep,
  selectStep,
  currentStep,
  previewSteps,
  runModeDialogVisible,
  startAutomaticDemo,
  startOneClickDemo,
  startStepByStepDemo,
  startSelectedAutomaticDemo,
  startGuidedExperience,
  startSelectedGuide,
  openArchitecture,
  openSelectedBusinessPage,
  openRecords,
  goPerformance,
} = useDemoHome()
const { recentEvidence, currentObject, currentStatus } = useDemoEvidence(currentStep)

// 异步导入机制是纯说明弹窗，开关留在页面内维护。
const mechanismVisible = ref(false)

function openEngineeringCheck(check: EngineeringCheck): void {
  if (check.key === 'import') {
    mechanismVisible.value = true
    return
  }
  goPerformance(check.key)
}
</script>

<template>
  <div class="demo-home">
    <DemoHeader
      :preview-steps="previewSteps"
      @start-automatic="startAutomaticDemo('full')"
      @start-guided-inbound="startGuidedExperience('inbound')"
      @open-architecture="openArchitecture"
    />

    <section class="app-card flow-section">
      <div class="section-head">
        <div>
          <h2>业务流程</h2>
          <p>点击流程节点查看说明，也可以进入真实业务页面</p>
        </div>
        <span>本次演示 · 5 个关键步骤</span>
      </div>

      <div class="flow-layout">
        <ScenarioSelector :steps="businessSteps" :selected="selectedStep" @select="selectStep" />

        <DemoResultPanel
          :step="currentStep"
          :status="currentStatus"
          :object-text="currentObject"
          :has-evidence="Boolean(recentEvidence)"
          @open-business="openSelectedBusinessPage"
          @start-guide="startSelectedGuide"
          @start-auto="startSelectedAutomaticDemo"
          @open-records="openRecords"
        />
      </div>
    </section>

    <section class="app-card engineering-section">
      <div class="section-head">
        <div>
          <h2>工程验证</h2>
          <p>点击卡片后配置参数，结果以简洁方式呈现</p>
        </div>
        <el-button link type="primary" @click="goPerformance()">查看全部 →</el-button>
      </div>

      <div class="engineering-grid">
        <article v-for="check in engineeringChecks" :key="check.key" class="app-card engineering-card">
          <div>
            <h3>{{ check.title }}</h3>
            <p>{{ check.description }}</p>
            <span>{{ check.tag }}</span>
          </div>
          <el-button text type="primary" @click="openEngineeringCheck(check)">
            {{ check.action }} <ArrowRight />
          </el-button>
        </article>
      </div>
    </section>

    <div class="deep-links">
      <el-button text type="primary" @click="openRecords">业务证据 →</el-button>
      <el-button text type="primary" @click="goPerformance()">工程验证 →</el-button>
    </div>

    <el-dialog
      v-model="runModeDialogVisible"
      title="选择自动演示方式"
      width="min(560px, 94vw)"
      append-to-body
      :close-on-click-modal="true"
    >
      <div class="run-mode-grid">
        <button type="button" class="run-mode-card" @click="startOneClickDemo">
          <b>一键自动完成</b>
          <span>连续调用真实入库和出库接口，完成后展示业务结果。</span>
          <small>适合快速查看完整闭环</small>
        </button>
        <button type="button" class="run-mode-card" @click="startStepByStepDemo">
          <b>分步执行</b>
          <span>点击一次“执行下一步”，系统才执行当前真实业务步骤。</span>
          <small>每一步都可打开真实页面核对</small>
        </button>
      </div>
    </el-dialog>

    <el-dialog
      v-model="mechanismVisible"
      title="异步导入可靠性"
      width="min(520px, 92vw)"
      append-to-body
      :close-on-click-modal="true"
    >
      <ul class="mechanism-list">
        <li>导入任务持久化为待处理状态，由单个后台消费者领取，避免多实例重复执行。</li>
        <li>每次执行使用独立执行标识，旧消费者不能覆盖新任务状态。</li>
        <li>失败行保留源文件用于排查和重试，成功任务完成后才清理文件。</li>
        <li>每行业务写入仍受租户、校验和事务边界保护。</li>
      </ul>
      <template #footer>
        <el-button @click="mechanismVisible = false">关闭</el-button>
        <el-button type="primary" @click="openRecords">查看操作记录</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.demo-home {
  width: min(1180px, 100%);
  height: 100%;
  margin: 0 auto;
  padding-bottom: 52px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  color: var(--el-text-color-primary);
}

.flow-section,
.engineering-section {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
  background: var(--el-bg-color);
}

.flow-section {
  flex: 0 0 auto;
  padding: 14px 16px;
}

.section-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.section-head h2 {
  margin: 0 0 2px;
  font-size: 16px;
}

.section-head p,
.section-head > span {
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.flow-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(360px, 0.85fr);
  gap: 12px;
}

.engineering-section {
  flex: 1 1 auto;
  min-height: 160px;
  padding: 14px 16px;
}

.engineering-grid {
  height: calc(100% - 36px);
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.engineering-card {
  min-width: 0;
  padding: 13px 14px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 11px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  background: var(--el-fill-color-extra-light);
}

.engineering-card h3 {
  margin: 0 0 5px;
  font-size: 15px;
}

.engineering-card p {
  margin: 0 0 8px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.55;
}

.engineering-card span {
  display: inline-block;
  padding: 3px 7px;
  border-radius: 999px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  font-size: 10px;
}

.engineering-card .el-button {
  justify-content: flex-start;
  margin: 8px 0 0 -4px;
}

.deep-links {
  display: flex;
  justify-content: flex-end;
  gap: 4px;
  min-height: 24px;
}

.deep-links .el-button {
  margin-left: 0;
}

.mechanism-list {
  margin: 0;
  padding-left: 20px;
  color: var(--el-text-color-secondary);
  line-height: 1.8;
}

@media (max-height: 760px) and (min-width: 761px) {
  .demo-home {
    gap: 8px;
    padding-bottom: 24px;
  }

  .deep-links {
    display: none;
  }

  .engineering-section {
    min-height: 142px;
  }

  .engineering-card {
    padding: 10px 12px;
  }

  .engineering-card p,
  .engineering-card span {
    display: none;
  }

  .engineering-card h3 {
    margin-bottom: 0;
  }

  .engineering-card .el-button {
    margin-top: 2px;
  }
}

@media (max-width: 980px) {
  .flow-layout {
    grid-template-columns: 1fr;
  }

  .demo-home {
    height: auto;
    padding-bottom: 0;
  }
}

@media (max-width: 680px) {
  .engineering-grid {
    height: auto;
    grid-template-columns: 1fr;
  }

  .section-head {
    align-items: flex-start;
    flex-direction: column;
  }
}

.run-mode-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.run-mode-card {
  min-width: 0;
  padding: 16px;
  border: 1px solid var(--el-border-color);
  border-radius: 11px;
  display: grid;
  gap: 7px;
  text-align: left;
  color: var(--el-text-color-primary);
  background: var(--el-bg-color);
  cursor: pointer;
}

.run-mode-card:hover {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.run-mode-card b {
  font-size: 16px;
}

.run-mode-card span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.run-mode-card small {
  color: var(--el-color-primary);
  font-size: 11px;
}

@media (max-width: 680px) {
  .run-mode-grid {
    grid-template-columns: 1fr;
  }
}
</style>