<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowRight, VideoPlay } from '@element-plus/icons-vue'
import { getGuideStep, useGuideStore, type GuideScenario } from '@/stores/guide'
import { onDataChanged, runDemoAutomaticallyInConsole, runDemoStepByStepInConsole } from '@/utils/events'
import { readDemoEvidence } from '@/utils/demoEvidence'

type AutomaticScenario = GuideScenario | 'full'
type ManualTarget = 'inbound' | 'outbound' | 'inventory'

interface BusinessStep {
  key: string
  name: string
  short: string
  description: string
  object: string
  status: string
  quantity: string
  evidence: string
  actionLabel: string
  manualTarget: ManualTarget
  automaticScenario: GuideScenario
}

interface EngineeringCheck {
  key: 'allocation' | 'picking' | 'import'
  title: string
  description: string
  tag: string
  action: string
}

const router = useRouter()
const guide = useGuideStore()
const selectedStep = ref(0)
const mechanismVisible = ref(false)
const runModeDialogVisible = ref(false)
const recentEvidence = ref(readDemoEvidence())
let stopDataChanged: (() => void) | undefined

const businessSteps: BusinessStep[] = [
  {
    key: 'inbound-order',
    name: '入库单',
    short: '创建与审核',
    description: '创建入库单并完成审核，形成可执行的收货任务。',
    object: '入库单（执行后生成）',
    status: '等待执行',
    quantity: '按计划入库',
    evidence: '操作记录与业务单据',
    actionLabel: '进入入库页面',
    manualTarget: 'inbound',
    automaticScenario: 'inbound',
  },
  {
    key: 'inbound-receive',
    name: '收货上架',
    short: '收货与上架',
    description: '按实收数量完成收货，上架后写入库存并生成库存流水。',
    object: '收货 / 上架任务',
    status: '等待前序步骤',
    quantity: '按实收数量增加',
    evidence: '任务记录与库存流水',
    actionLabel: '进入入库页面',
    manualTarget: 'inbound',
    automaticScenario: 'inbound',
  },
  {
    key: 'inventory-ready',
    name: '库存形成',
    short: '库存与流水',
    description: '收货结果形成可用库存，可继续核对库存数量和流水。',
    object: '库存与库存流水',
    status: '等待前序步骤',
    quantity: '现存量与可用量同步',
    evidence: '库存流水',
    actionLabel: '查看库存页面',
    manualTarget: 'inventory',
    automaticScenario: 'inbound',
  },
  {
    key: 'outbound-order',
    name: '出库单',
    short: '审核与分配',
    description: '审核出库单，按 FIFO 规则分配库存并生成拣货任务。',
    object: '出库单与拣货任务',
    status: '等待执行',
    quantity: '按分配数量扣减可用量',
    evidence: '库存流水与任务记录',
    actionLabel: '进入出库页面',
    manualTarget: 'outbound',
    automaticScenario: 'outbound',
  },
  {
    key: 'outbound-ship',
    name: '拣货发货',
    short: '任务与发货',
    description: '拣货任务完成后发货，形成完整出库闭环和操作记录。',
    object: '拣货任务与发货单据',
    status: '等待前序步骤',
    quantity: '按实际拣货数量发货',
    evidence: '任务记录与发货流水',
    actionLabel: '进入出库页面',
    manualTarget: 'outbound',
    automaticScenario: 'outbound',
  },
]

const engineeringChecks: EngineeringCheck[] = [
  {
    key: 'allocation',
    title: '并发库存分配',
    description: '验证 FIFO 分配、库存边界和防超卖。',
    tag: '并发 · FIFO · 防超卖',
    action: '配置并运行',
  },
  {
    key: 'picking',
    title: '拣货作业验证',
    description: '观察并发拣货和重复扫码拦截。',
    tag: '并发拣货 · 重复扫码',
    action: '配置并运行',
  },
  {
    key: 'import',
    title: '异步导入可靠性',
    description: '查看任务恢复、重试和行级处理。',
    tag: '异步任务 · 可恢复',
    action: '查看机制',
  },
]

const currentStep = computed(() => businessSteps[selectedStep.value] ?? businessSteps[0])
const previewSteps = computed(() => [businessSteps[0], businessSteps[1], businessSteps[3], businessSteps[4]])
const currentEvidence = computed(() => {
  const context = recentEvidence.value
  if (!context) return null

  const labels: Record<string, string[]> = {
    'inbound-order': ['入库单'],
    'inbound-receive': ['收货', '上架'],
    'inventory-ready': ['库存'],
    'outbound-order': ['出库单', 'FIFO', '分配'],
    'outbound-ship': ['拣货', '发货'],
  }
  const candidates = labels[currentStep.value.key] ?? []
  return (context.evidence ?? []).find((item) => candidates.some((label) => item.label.includes(label))) ?? null
})
const currentObject = computed(() => {
  if (currentEvidence.value) return `${currentEvidence.value.label}：${currentEvidence.value.value}`
  return currentStep.value.object
})
const currentStatus = computed(() => (recentEvidence.value ? '已有执行记录' : currentStep.value.status))

function startAutomaticDemo(scenario: AutomaticScenario): void {
  if (scenario === 'full') {
    runModeDialogVisible.value = true
    return
  }
  if (scenario === 'stocktake') return
  runDemoAutomaticallyInConsole(scenario)
}

function startOneClickDemo(): void {
  runModeDialogVisible.value = false
  runDemoAutomaticallyInConsole('full')
}

function startStepByStepDemo(): void {
  runModeDialogVisible.value = false
  runDemoStepByStepInConsole()
}

function startSelectedAutomaticDemo(): void {
  startAutomaticDemo(currentStep.value.automaticScenario)
}

async function startGuidedExperience(scenario: Extract<GuideScenario, 'inbound' | 'outbound'>): Promise<void> {
  const firstStep = getGuideStep(scenario, 0)
  if (!firstStep) return
  await router.push(firstStep.route)
  guide.start(scenario)
}

function startSelectedGuide(): void {
  if (currentStep.value.manualTarget === 'inventory') return
  startGuidedExperience(currentStep.value.manualTarget)
}

function openSelectedBusinessPage(): void {
  if (currentStep.value.manualTarget === 'outbound') {
    void router.push('/outbound/orders')
    return
  }
  if (currentStep.value.manualTarget === 'inventory') {
    void router.push('/inventory')
    return
  }
  void router.push('/inbound/orders')
}

function openRecords(): void {
  void router.push({ path: '/demo/activity', query: { tab: 'operations' } })
}

function goPerformance(section?: string): void {
  if (section) {
    void router.push({ path: '/demo/performance', query: { section } })
    return
  }
  void router.push('/demo/performance')
}

function openEngineeringCheck(check: EngineeringCheck): void {
  if (check.key === 'import') {
    mechanismVisible.value = true
    return
  }
  goPerformance(check.key)
}

onMounted(() => {
  recentEvidence.value = readDemoEvidence()
  stopDataChanged = onDataChanged(() => {
    recentEvidence.value = readDemoEvidence()
  })
})

onUnmounted(() => stopDataChanged?.())
</script>

<template>
  <div class="demo-home">
    <section class="app-card home-hero">
      <div class="hero-copy">
        <span class="eyebrow">推荐体验 · 约 3 分钟</span>
        <h1>WMS 业务闭环</h1>
        <p>从入库、库存到出库，查看真实单据、库存变化与操作记录。</p>
        <div class="hero-actions">
          <el-button type="primary" :icon="VideoPlay" @click="startAutomaticDemo('full')">
            开始自动演示
          </el-button>
          <el-button @click="startGuidedExperience('inbound')">引导体验 · 入库</el-button>
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

    <section class="app-card flow-section">
      <div class="section-head">
        <div>
          <h2>业务流程</h2>
          <p>点击流程节点查看说明，也可以进入真实业务页面</p>
        </div>
        <span>本次演示 · 5 个关键步骤</span>
      </div>

      <div class="flow-layout">
        <div class="flow-panel">
          <div class="flow-strip" role="tablist" aria-label="业务流程步骤">
            <template v-for="(step, index) in businessSteps" :key="step.key">
              <button
                type="button"
                class="flow-step"
                :class="{ active: selectedStep === index }"
                role="tab"
                :aria-selected="selectedStep === index"
                @click="selectedStep = index"
              >
                <span>{{ String(index + 1).padStart(2, '0') }}</span>
                <b>{{ step.name }}</b>
                <small>{{ step.short }}</small>
              </button>
              <i v-if="index < businessSteps.length - 1" class="flow-arrow" aria-hidden="true">→</i>
            </template>
          </div>
          <div class="flow-meta">
            <b>真实业务链路</b>
            <i>·</i>
            <span>单据、库存、任务和流水都会留下记录</span>
          </div>
        </div>

        <article class="detail-panel" aria-live="polite">
          <div class="detail-head">
            <div>
              <span>当前步骤</span>
              <h3>{{ currentStep.name }}</h3>
            </div>
            <el-tag :type="recentEvidence ? 'success' : 'info'" effect="plain">{{ currentStatus }}</el-tag>
          </div>
          <p>{{ currentStep.description }}</p>
          <dl class="detail-lines">
            <div>
              <dt>关联对象</dt>
              <dd>{{ currentObject }}</dd>
            </div>
            <div>
              <dt>数量变化</dt>
              <dd>{{ currentStep.quantity }}</dd>
            </div>
            <div>
              <dt>证据</dt>
              <dd>{{ currentStep.evidence }}</dd>
            </div>
          </dl>
          <div class="detail-actions">
            <el-button size="small" @click="openSelectedBusinessPage">
              {{ currentStep.actionLabel }}
            </el-button>
            <el-button v-if="currentStep.manualTarget !== 'inventory'" size="small" @click="startSelectedGuide">引导演示</el-button>
            <el-button size="small" @click="startSelectedAutomaticDemo">自动演示</el-button>
            <el-button size="small" text type="primary" @click="openRecords">查看操作日志</el-button>
          </div>
        </article>
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

.home-hero,
.flow-section,
.engineering-section {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
  background: var(--el-bg-color);
}

.home-hero {
  flex: 0 0 176px;
  padding: 20px 22px;
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

.eyebrow,
.detail-head > div > span {
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

.flow-panel,
.detail-panel {
  min-width: 0;
  min-height: 176px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
}

.flow-panel {
  padding: 14px;
  display: flex;
  flex-direction: column;
  background: var(--el-fill-color-extra-light);
}

.flow-strip {
  display: flex;
  align-items: stretch;
  gap: 5px;
}

.flow-step {
  min-width: 0;
  flex: 1;
  padding: 9px 5px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 9px;
  color: var(--el-text-color-regular);
  background: var(--el-bg-color);
  text-align: left;
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease;
}

.flow-step:hover,
.flow-step.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.flow-step span,
.flow-step b,
.flow-step small {
  display: block;
  min-width: 0;
}

.flow-step span {
  margin-bottom: 9px;
  color: var(--el-color-primary);
  font-size: 11px;
  font-weight: 800;
}

.flow-step b {
  font-size: 12px;
}

.flow-step small {
  margin-top: 5px;
  color: var(--el-text-color-secondary);
  font-size: 10px;
  line-height: 1.35;
}

.flow-arrow {
  align-self: center;
  flex: none;
  color: var(--el-text-color-placeholder);
  font-style: normal;
}

.flow-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: auto;
  padding-top: 14px;
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.flow-meta b {
  color: var(--el-color-success);
}

.flow-meta i {
  font-style: normal;
}

.detail-panel {
  padding: 15px 16px;
  display: flex;
  flex-direction: column;
  background: var(--el-fill-color-extra-light);
}

.detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}

.detail-head h3 {
  margin: 4px 0 0;
  font-size: 18px;
}

.detail-panel > p {
  margin: 8px 0 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.55;
}

.detail-lines {
  margin: 0;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.detail-lines div {
  min-width: 0;
  padding: 8px 9px;
  border-radius: 8px;
  background: var(--el-bg-color);
}

.detail-lines dt {
  color: var(--el-text-color-secondary);
  font-size: 10px;
}

.detail-lines dd {
  margin: 4px 0 0;
  overflow-wrap: anywhere;
  font-size: 12px;
  font-weight: 700;
}

.detail-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: auto;
  padding-top: 12px;
}

.detail-actions .el-button {
  margin-left: 0;
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

  .home-hero {
    flex-basis: 154px;
    padding-top: 14px;
    padding-bottom: 14px;
  }

  .hero-side > p,
  .detail-panel > p {
    display: none;
  }

  .proof-line {
    padding-top: 6px;
  }

  .flow-panel,
  .detail-panel {
    min-height: 158px;
  }

  .detail-lines div {
    padding: 6px 8px;
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
  .home-hero,
  .flow-layout {
    grid-template-columns: 1fr;
  }

  .demo-home {
    height: auto;
    padding-bottom: 0;
  }

  .home-hero {
    flex-basis: auto;
  }

  .preview-flow,
  .flow-strip {
    flex-wrap: wrap;
  }

  .flow-arrow {
    display: none;
  }

  .flow-step {
    flex: 1 0 calc(33.333% - 5px);
  }
}

@media (max-width: 680px) {
  .engineering-grid {
    height: auto;
    grid-template-columns: 1fr;
  }

  .detail-lines {
    grid-template-columns: 1fr;
  }

  .flow-step {
    flex-basis: calc(50% - 5px);
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
