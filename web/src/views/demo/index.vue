<script setup lang="ts">
import { ref } from 'vue'
import { Aim, ArrowRight, Connection, Document, TopRight } from '@element-plus/icons-vue'
import DemoHeader from '@/components/demo/DemoHeader.vue'
import DemoExperience from '@/components/demo/DemoExperience.vue'
import DemoResultPanel from '@/components/demo/DemoResultPanel.vue'
import ScenarioSelector from '@/components/demo/ScenarioSelector.vue'
import { useDemoEvidence } from '@/composables/demo/useDemoEvidence'
import { engineeringChecks, useDemoHome, type EngineeringCheck } from '@/composables/demo/useDemoHome'

const {
  businessSteps, selectedStep, selectStep, currentStep,
  startOneClickDemo, startStepByStepDemo, startSelectedAutomaticDemo, startSelectedGuide,
  openArchitecture, openSelectedBusinessPage, openRecords, openEvidence, goPerformance,
} = useDemoHome()
const { recentEvidence, currentEvidence, currentObject, currentStatus } = useDemoEvidence(currentStep)
const mechanismVisible = ref(false)
const checkIcons = { allocation: Connection, picking: Aim, import: Document }

function openEngineeringCheck(check: EngineeringCheck): void {
  if (check.key === 'import') mechanismVisible.value = true
  else goPerformance(check.key)
}
</script>

<template>
  <div class="demo-home">
    <DemoHeader @open-architecture="openArchitecture" />
    <div class="demo-workspace">
      <section class="flow-section" aria-label="业务流程">
        <header class="section-head"><div><h2>业务全流程</h2><span>单据、任务与库存环环相扣</span></div><small>5 个关键阶段</small></header>
        <ScenarioSelector :steps="businessSteps" :selected="selectedStep" @select="selectStep" />
        <DemoResultPanel :step="currentStep" :status="currentStatus" :object-text="currentObject" :has-evidence="Boolean(currentEvidence)" @open-business="openSelectedBusinessPage" @start-guide="startSelectedGuide" @start-auto="startSelectedAutomaticDemo" @open-records="openRecords" />
      </section>
      <DemoExperience :evidence="recentEvidence" @start-automatic="startOneClickDemo" @start-step="startStepByStepDemo" @open-evidence="openEvidence" />
    </div>
    <section class="engineering-section" aria-label="工程验证">
      <header class="section-head"><div><h2>工程验证</h2><span>业务之外，经得起并发与异常的考验</span></div><el-button link @click="goPerformance()">查看全部 <el-icon><TopRight /></el-icon></el-button></header>
      <div class="engineering-grid">
        <article v-for="check in engineeringChecks" :key="check.key" class="engineering-card" :aria-label="`${check.title}：${check.tag}`">
          <span class="engineering-icon"><el-icon><component :is="checkIcons[check.key]" /></el-icon></span>
          <h3>{{ check.title }}</h3><el-icon class="card-corner"><TopRight /></el-icon><p>{{ check.description }}</p>
          <el-button link type="primary" @click="openEngineeringCheck(check)">{{ check.action }} <el-icon><ArrowRight /></el-icon></el-button>
        </article>
      </div>
    </section>
    <el-dialog v-model="mechanismVisible" title="异步导入可靠性" width="min(520px, 92vw)" append-to-body :close-on-click-modal="true">
      <ul class="mechanism-list"><li>导入任务持久化为待处理状态，由单个后台消费者领取，避免多实例重复执行。</li><li>每次执行使用独立执行标识，旧消费者不能覆盖新任务状态。</li><li>失败行保留源文件用于排查和重试，成功任务完成后才清理文件。</li><li>每行业务写入仍受租户、校验和事务边界保护。</li></ul>
      <template #footer><el-button @click="mechanismVisible = false">关闭</el-button><el-button type="primary" @click="openRecords">查看操作记录</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.demo-home { width: 100%; height: 100%; min-height: 0; display: grid; grid-template-rows: 68px minmax(300px, 1fr) 160px; gap: 18px; color: var(--el-text-color-primary); }
.demo-workspace { display: grid; grid-template-columns: minmax(0, 1fr) 290px; gap: 18px; min-width: 0; min-height: 0; }
.flow-section { min-width: 0; min-height: 0; display: grid; grid-template-rows: 27px minmax(75px, 0.55fr) minmax(150px, 1fr); gap: 15px; padding: 20px 22px; border: 1px solid var(--el-border-color-light); border-radius: 12px; background: var(--el-bg-color); }
.section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-width: 0; }
.section-head > div { display: flex; align-items: baseline; gap: 9px; min-width: 0; }
.section-head h2 { margin: 0; font-size: 16px; font-weight: 600; white-space: nowrap; }
.section-head span, .section-head small { font-size: 11px; color: var(--el-text-color-secondary); }
.flow-section > .section-head { align-items: flex-start; padding-bottom: 10px; border-bottom: 1px solid var(--el-border-color-light); }
.section-head small { white-space: nowrap; }
.engineering-section { min-height: 0; display: grid; grid-template-rows: 24px minmax(0, 1fr); gap: 11px; }
.engineering-section > .section-head .el-button { font-size: 11px; color: var(--el-text-color-secondary); margin: 0; }
.engineering-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; min-height: 0; }
.engineering-card { min-width: 0; min-height: 0; display: grid; grid-template-columns: 33px minmax(0, 1fr) 15px; grid-template-rows: auto 1fr auto; gap: 0 11px; padding: 16px 18px; border: 1px solid var(--el-border-color-light); border-radius: 10px; background: var(--el-bg-color); }
.engineering-icon { grid-column: 1; grid-row: 1 / 3; display: grid; place-items: center; width: 33px; height: 33px; border-radius: 9px; background: var(--el-color-primary-light-9); color: var(--el-color-primary); }
.engineering-icon .el-icon { font-size: 18px; }
.engineering-card h3 { grid-column: 2; grid-row: 1; margin: 3px 0 0; font-size: 15px; font-weight: 600; }
.card-corner { grid-column: 3; grid-row: 1; margin-top: 3px; color: var(--el-text-color-placeholder); }
.engineering-card p { grid-column: 2; grid-row: 2; margin: 6px 0 0; font-size: 12px; line-height: 1.7; color: var(--el-text-color-secondary); }
.engineering-card .el-button { grid-column: 2; grid-row: 3; justify-self: start; margin: 0; padding: 8px 0 0; font-size: 12px; }
.engineering-card .el-button .el-icon { margin-left: 5px; }
.mechanism-list { margin: 0; padding-left: 20px; line-height: 1.8; color: var(--el-text-color-secondary); }
@media (min-width: 1600px) and (min-height: 800px) {
  .demo-home { grid-template-rows: 78px minmax(330px, 1fr) 180px; gap: 21px; }
  .demo-workspace { grid-template-columns: minmax(0, 1fr) 330px; gap: 22px; }
  .flow-section { padding: 23px 26px; grid-template-rows: 28px minmax(85px, 0.55fr) minmax(164px, 1fr); gap: 17px; }
  .engineering-card { padding: 21px; grid-template-columns: 37px minmax(0, 1fr) 16px; gap: 0 13px; }
  .engineering-icon { width: 37px; height: 37px; }
  .engineering-card h3 { font-size: 16px; }
  .engineering-card p { font-size: 13px; }
}
@media (max-height: 720px) and (min-width: 981px) {
  .demo-home { grid-template-rows: 54px minmax(280px, 1fr) 126px; gap: 12px; }
  .flow-section { padding: 12px 17px; grid-template-rows: 24px minmax(62px, 0.5fr) minmax(146px, 1fr); gap: 10px; }
  .section-head h2 { font-size: 15px; }
  .engineering-section { grid-template-rows: 22px 1fr; gap: 7px; }
  .engineering-card { padding: 11px 14px; grid-template-columns: 28px minmax(0, 1fr) 14px; gap: 0 9px; }
  .engineering-icon { width: 28px; height: 28px; }
  .engineering-icon .el-icon { font-size: 15px; }
  .engineering-card h3 { font-size: 13px; }
  .engineering-card p, .engineering-card .el-button { font-size: 11px; }
  .engineering-card p { margin-top: 4px; }
  .engineering-card .el-button { padding-top: 4px; }
}
@media (max-width: 1180px) and (min-width: 981px) {
  .demo-workspace { grid-template-columns: minmax(0, 1fr) 235px; gap: 13px; }
  .flow-section { padding-inline: 14px; }
  .section-head span { display: none; }
  .engineering-grid { gap: 10px; }
  .engineering-card { padding-inline: 11px; gap: 0 8px; }
  .engineering-card h3 { font-size: 12px; }
}
@media (max-height: 640px) and (min-width: 981px) { .demo-home { gap: 8px; } }
@media (max-width: 980px) {
  .demo-home { height: auto; display: flex; flex-direction: column; gap: 18px; }
  .demo-workspace { grid-template-columns: 1fr; gap: 17px; }
  .flow-section { min-height: 390px; grid-template-rows: 27px 85px 1fr; gap: 15px; padding: 18px; }
  .engineering-section { min-height: 170px; }
  .engineering-card { min-height: 135px; padding: 13px; }
  .engineering-card h3 { font-size: 13px; }
  .engineering-card p { font-size: 11px; }
  .section-head span { display: none; }
}
@media (max-width: 640px) {
  .flow-section { min-height: 440px; padding: 15px; }
  .engineering-grid { grid-template-columns: 1fr; gap: 11px; }
  .engineering-card { min-height: 126px; padding: 16px; }
  .engineering-card h3 { font-size: 14px; }
  .engineering-card p { font-size: 12px; }
}
</style>
