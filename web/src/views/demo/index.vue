<script setup lang="ts">
import { reactive, ref, type Component } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ArrowRight,
  Document,
  Download,
  Link,
  Monitor,
  Tickets,
  Upload,
  VideoPlay,
} from '@element-plus/icons-vue'
import { releaseDemoSession, resetDemoData } from '@/api/demo'
import DemoTour from '@/components/demo/DemoTour.vue'
import { useAuthStore } from '@/stores/auth'
import { useGuideStore, type GuideScenario } from '@/stores/guide'
import { runDemoScenarioInConsole } from '@/utils/events'

interface ScenarioCard {
  key: GuideScenario
  title: string
  description: string
  icon: Component
}

const router = useRouter()
const auth = useAuthStore()
const guide = useGuideStore()

const resetLoading = ref(false)
const exitLoading = ref(false)
const scenarioDialog = reactive<{
  visible: boolean
  scenario: GuideScenario | null
  title: string
}>({
  visible: false,
  scenario: null,
  title: '',
})

const scenarios: ScenarioCard[] = [
  {
    key: 'inbound',
    title: '入库流程',
    description: '从创建入库单、收货到上架，完成一批货进入库存的真实闭环。',
    icon: Download,
  },
  {
    key: 'outbound',
    title: '出库流程',
    description: '从出库审核、FIFO 分配到拣货发货，观察库存如何被真实扣减。',
    icon: Upload,
  },
  {
    key: 'stocktake',
    title: '库存盘点',
    description: '生成账面快照、录入实盘差异并审核调整，核对最终库存状态。',
    icon: Tickets,
  },
]

function openScenarioDialog(scenario: ScenarioCard): void {
  scenarioDialog.scenario = scenario.key
  scenarioDialog.title = scenario.title
  scenarioDialog.visible = true
}

function startFullDemo(): void {
  runDemoScenarioInConsole('full')
}

function startAutomaticScenario(): void {
  if (!scenarioDialog.scenario) return
  runDemoScenarioInConsole(scenarioDialog.scenario)
  scenarioDialog.visible = false
}

function startManualGuide(): void {
  if (!scenarioDialog.scenario) return
  const firstStep = guide.start(scenarioDialog.scenario)
  scenarioDialog.visible = false
  void router.push(firstStep.route)
}

function goActivity(): void {
  void router.push('/demo/activity')
}

function goPerformance(): void {
  void router.push('/demo/performance')
}

function openOverview(): void {
  window.open('/overview.html', '_blank', 'noopener,noreferrer')
}

function openSource(): void {
  window.open('https://github.com/mengw-dev/go-wms', '_blank', 'noopener,noreferrer')
}

async function resetData(): Promise<void> {
  try {
    await ElMessageBox.confirm(
      '将恢复仓库、货品、库存和单据到初始演示数据，确定继续吗？',
      '重置演示数据',
      { type: 'warning', confirmButtonText: '确定重置', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  resetLoading.value = true
  try {
    await resetDemoData()
    ElMessage.success('演示数据已恢复为初始状态')
  } finally {
    resetLoading.value = false
  }
}

async function releaseAndExit(): Promise<void> {
  try {
    await ElMessageBox.confirm(
      '退出后当前演示数据会立即恢复初始状态，确定退出吗？',
      '退出演示',
      { type: 'warning', confirmButtonText: '退出并重置', cancelButtonText: '继续演示' },
    )
  } catch {
    return
  }
  exitLoading.value = true
  try {
    await releaseDemoSession().catch(() => undefined)
    auth.clear()
    await router.push('/login')
  } finally {
    exitLoading.value = false
  }
}

async function onSessionCommand(command: string): Promise<void> {
  if (resetLoading.value || exitLoading.value) return
  if (command === 'reset') await resetData()
  if (command === 'exit') await releaseAndExit()
}
</script>

<template>
  <div class="demo-home">
    <section class="demo-hero" data-tour="demo-session">
      <div class="hero-topline">
        <span class="hero-kicker">WMS 演示中心</span>
        <div class="session-tools">
          <span class="session-ready"><i></i>演示环境已就绪</span>
          <el-dropdown trigger="click" @command="onSessionCommand">
            <el-button text :disabled="resetLoading || exitLoading">会话操作</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="reset" :disabled="resetLoading || exitLoading">
                  重置演示数据
                </el-dropdown-item>
                <el-dropdown-item command="exit" divided :disabled="resetLoading || exitLoading">
                  退出演示
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>

      <div class="hero-copy" data-tour="complete-flow">
        <h1>用一个真实业务闭环，看懂这套 WMS</h1>
        <p>
          从入库、库存到出库和盘点，所有步骤都调用真实业务 Service。完成后可以继续查看单据、任务、
          库存变化和技术实现。
        </p>
        <div class="hero-flow" aria-label="完整业务闭环">
          <span>入库</span>
          <ArrowRight />
          <span>库存</span>
          <ArrowRight />
          <span>出库</span>
          <ArrowRight />
          <span>盘点</span>
        </div>
        <el-button
          class="primary-cta"
          type="primary"
          size="large"
          :icon="VideoPlay"
          @click="startFullDemo"
        >
          开始 3 分钟演示
        </el-button>
        <small>一键执行完整闭环，过程与结果均来自真实后端。</small>
      </div>
    </section>

    <section class="home-section business-section">
      <div class="section-heading">
        <div>
          <span>核心业务</span>
          <h2>选择一项业务开始体验</h2>
        </div>
        <p>每项业务只保留一个入口，再选择自动演示或亲自操作。</p>
      </div>

      <div class="scenario-grid">
        <article v-for="scenario in scenarios" :key="scenario.key" class="scenario-card">
          <div class="scenario-icon">
            <component :is="scenario.icon" />
          </div>
          <h3>{{ scenario.title }}</h3>
          <p>{{ scenario.description }}</p>
          <el-button type="primary" plain @click="openScenarioDialog(scenario)">
            开始体验
            <ArrowRight />
          </el-button>
        </article>
      </div>
    </section>

    <section class="home-section capability-section">
      <div class="section-heading">
        <div>
          <span>项目能力</span>
          <h2>不只是页面演示</h2>
        </div>
      </div>

      <ul class="capability-grid">
        <li>
          <span>01</span>
          <b>真实业务 Service</b>
          <p>自动流程复用现有入库、出库、库存和盘点业务能力。</p>
        </li>
        <li>
          <span>02</span>
          <b>独立演示租户</b>
          <p>访客使用独立演示账号，不会接触普通业务数据。</p>
        </li>
        <li>
          <span>03</span>
          <b>结果可追溯</b>
          <p>执行后可核对单据、任务状态、库存流水和接口记录。</p>
        </li>
        <li>
          <span>04</span>
          <b>库存状态可核对</b>
          <p>现存量、可用量和分配量均可在业务页面继续验证。</p>
        </li>
      </ul>
    </section>

    <section class="home-section deep-section" data-tour="demo-project">
      <div class="section-heading">
        <div>
          <span>深入查看</span>
          <h2>需要继续了解时</h2>
        </div>
        <p>业务结果给第一次访问的人看，技术和源码细节留在这里继续展开。</p>
      </div>

      <div class="deep-grid">
        <button type="button" data-tour="demo-evidence" @click="goActivity">
          <Document />
          <span>
            <b>业务证据</b>
            <small>查看单据、任务和库存变化</small>
          </span>
          <ArrowRight />
        </button>
        <button type="button" data-tour="demo-verification" @click="goPerformance">
          <Monitor />
          <span>
            <b>工程验证</b>
            <small>查看并发实验和运行状态</small>
          </span>
          <ArrowRight />
        </button>
        <button type="button" @click="openOverview">
          <Tickets />
          <span>
            <b>项目概览</b>
            <small>业务流程、架构和可靠性设计</small>
          </span>
          <ArrowRight />
        </button>
        <button type="button" @click="openSource">
          <Link />
          <span>
            <b>源码仓库</b>
            <small>查看完整实现、测试和提交记录</small>
          </span>
          <ArrowRight />
        </button>
      </div>
    </section>

    <DemoTour @complete="startFullDemo" />

    <el-dialog
      v-model="scenarioDialog.visible"
      :title="`选择体验方式：${scenarioDialog.title}`"
      width="min(540px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <p class="dialog-description">
        自动演示会直接调用真实业务接口完成闭环；亲自操作会进入业务页面，由引导协助逐步完成。
      </p>
      <div class="mode-grid">
        <button type="button" class="mode-card" @click="startAutomaticScenario">
          <VideoPlay />
          <b>自动演示</b>
          <span>快速看完整结果，适合首次了解。</span>
        </button>
        <button type="button" class="mode-card" @click="startManualGuide">
          <ArrowRight />
          <b>亲自操作</b>
          <span>进入真实业务页面，按步骤完成。</span>
        </button>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.demo-home {
  width: min(1120px, 100%);
  margin: 0 auto;
  padding-bottom: 32px;
  display: grid;
  gap: 24px;
}

.demo-hero {
  position: relative;
  overflow: hidden;
  padding: 28px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 24px;
  background:
    radial-gradient(circle at 88% 18%, color-mix(in srgb, var(--el-color-primary) 18%, transparent), transparent 34%),
    linear-gradient(135deg, var(--el-color-primary-light-9), var(--el-bg-color));
  box-shadow: var(--el-box-shadow-light);
}

.hero-topline,
.section-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
}

.hero-kicker,
.section-heading > div > span {
  color: var(--el-color-primary);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.session-tools {
  display: flex;
  align-items: center;
  gap: 6px;
}

.session-ready {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.session-ready i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--el-color-success);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--el-color-success) 14%, transparent);
}

.hero-copy {
  max-width: 800px;
  padding: 58px 0 42px;
}

.hero-copy h1 {
  margin: 0;
  color: var(--el-text-color-primary);
  font-size: clamp(32px, 5vw, 52px);
  line-height: 1.12;
  letter-spacing: -0.03em;
}

.hero-copy > p {
  max-width: 720px;
  margin: 18px 0 0;
  color: var(--el-text-color-regular);
  font-size: 16px;
  line-height: 1.8;
}

.hero-flow {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin: 26px 0;
  color: var(--el-text-color-secondary);
}

.hero-flow span {
  padding: 7px 12px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 999px;
  color: var(--el-text-color-primary);
  background: var(--el-bg-color);
  font-size: 13px;
  font-weight: 700;
}

.hero-flow svg {
  width: 15px;
}

.primary-cta {
  min-width: 190px;
}

.hero-copy > small {
  display: block;
  margin-top: 12px;
  color: var(--el-text-color-secondary);
}

.home-section {
  padding: 26px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 20px;
  background: var(--el-bg-color);
  box-shadow: var(--el-box-shadow-light);
}

.section-heading {
  margin-bottom: 20px;
}

.section-heading h2 {
  margin: 6px 0 0;
  color: var(--el-text-color-primary);
  font-size: 24px;
}

.section-heading > p {
  max-width: 480px;
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
  text-align: right;
}

.scenario-grid,
.deep-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
}

.scenario-card {
  min-height: 230px;
  padding: 20px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  background: var(--el-fill-color-extra-light);
  transition:
    transform 0.18s ease,
    border-color 0.18s ease,
    box-shadow 0.18s ease;
}

.scenario-card:hover {
  transform: translateY(-2px);
  border-color: var(--el-color-primary-light-5);
  box-shadow: var(--el-box-shadow-light);
}

.scenario-icon {
  width: 42px;
  height: 42px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.scenario-icon svg {
  width: 22px;
}

.scenario-card h3 {
  margin: 18px 0 8px;
  color: var(--el-text-color-primary);
  font-size: 19px;
}

.scenario-card p {
  margin: 0 0 20px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.75;
}

.scenario-card .el-button {
  width: fit-content;
  margin-top: auto;
}

.scenario-card .el-button svg {
  margin-left: 4px;
}

.capability-grid {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.capability-grid li {
  padding: 18px;
  border-radius: 14px;
  background: var(--el-fill-color-extra-light);
}

.capability-grid li > span {
  display: block;
  margin-bottom: 12px;
  color: var(--el-color-primary);
  font-family: var(--gowms-num-font);
  font-size: 13px;
  font-weight: 800;
}

.capability-grid b {
  color: var(--el-text-color-primary);
  font-size: 15px;
}

.capability-grid p {
  margin: 8px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.deep-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.deep-grid button {
  min-width: 0;
  min-height: 128px;
  padding: 18px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  text-align: left;
  color: var(--el-text-color-primary);
  background: var(--el-fill-color-extra-light);
  cursor: pointer;
  transition:
    border-color 0.18s ease,
    background-color 0.18s ease;
}

.deep-grid button:hover {
  border-color: var(--el-color-primary-light-5);
  background: var(--el-color-primary-light-9);
}

.deep-grid button > svg:first-child {
  width: 22px;
  color: var(--el-color-primary);
}

.deep-grid button > svg:last-child {
  width: 16px;
  color: var(--el-text-color-placeholder);
}

.deep-grid button span,
.deep-grid button b,
.deep-grid button small {
  display: block;
  min-width: 0;
}

.deep-grid button b {
  font-size: 15px;
}

.deep-grid button small {
  margin-top: 6px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.dialog-description {
  margin: 0 0 18px;
  color: var(--el-text-color-secondary);
  font-size: 14px;
  line-height: 1.75;
}

.mode-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.mode-card {
  min-height: 150px;
  padding: 20px;
  border: 1px solid var(--el-border-color);
  border-radius: 14px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  text-align: left;
  color: var(--el-text-color-primary);
  background: var(--el-bg-color);
  cursor: pointer;
  transition:
    border-color 0.18s ease,
    background-color 0.18s ease,
    transform 0.18s ease;
}

.mode-card:hover {
  transform: translateY(-2px);
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.mode-card svg {
  width: 24px;
  color: var(--el-color-primary);
}

.mode-card b {
  margin-top: 6px;
  font-size: 16px;
}

.mode-card span {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.6;
}

@media (max-width: 920px) {
  .scenario-grid,
  .capability-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .deep-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .demo-home {
    gap: 16px;
  }

  .demo-hero,
  .home-section {
    padding: 20px;
    border-radius: 16px;
  }

  .hero-topline,
  .section-heading {
    flex-direction: column;
    gap: 10px;
  }

  .session-tools {
    width: 100%;
    justify-content: space-between;
  }

  .hero-copy {
    padding: 38px 0 22px;
  }

  .hero-copy h1 {
    font-size: 34px;
  }

  .section-heading > p {
    text-align: left;
  }

  .scenario-grid,
  .capability-grid,
  .deep-grid,
  .mode-grid {
    grid-template-columns: 1fr;
  }

  .scenario-card {
    min-height: 0;
  }

  .deep-grid button {
    min-height: 104px;
  }
}
</style>