<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { login } from '@/api/auth'
import { acquireDemoSession, claimDemoAccount } from '@/api/demo'
import { claimPersonalAccount, listPersonalAccounts, type PersonalAccountInfo } from '@/api/personal'
import { getVersion } from '@/api/version'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const demoLoading = ref(false)
const personalLoading = ref(false)
// 特性开关：后端 demo.enabled / personal.enabled 决定展示哪些快捷入口（生产部署可能两者都关闭）。
const demoEnabled = ref(true)
const personalEnabled = ref(false)
// 快捷体验方式：'demo' 在线体验（自动分配、退出重置）/ 'personal' 个人空间（自选账号、数据保留）。
const entry = ref<'demo' | 'personal'>('demo')
const personalAccounts = ref<PersonalAccountInfo[]>([])
const selectedAccount = ref('')
const form = reactive({ username: '', password: '', tenant_id: '' })

onMounted(async () => {
  try {
    const version = await getVersion()
    demoEnabled.value = version.demo_enabled !== false
    personalEnabled.value = version.personal_enabled === true
  } catch {
    // /version 不可用时保持默认（演示开启），不影响登录功能
  }
  entry.value = demoEnabled.value ? 'demo' : 'personal'
  if (personalEnabled.value) {
    try {
      personalAccounts.value = await listPersonalAccounts()
    } catch {
      // 错误提示由 request.ts 拦截器统一弹出
    }
  }
})

const showEntries = computed(() => demoEnabled.value || personalEnabled.value)
const showSwitch = computed(() => demoEnabled.value && personalEnabled.value)

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  tenant_id: [{ pattern: /^\d{1,19}$/, message: '租户编号应为非负整数', trigger: 'blur' }],
}

async function submit() {
  if (loading.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    const result = await login({
      username: form.username,
      password: form.password,
      tenant_id: form.tenant_id || undefined,
    })
    auth.setAuth(result)
    if ((result.perms ?? []).includes('wms:demo')) {
      const session = await acquireDemoSession()
      auth.setDemoSession(session)
      sessionStorage.setItem('WMS_DEMO_AUTO_OPEN', '1')
    }
    ElMessage.success('登录成功')
    const redirect = route.query.redirect
    router.push(typeof redirect === 'string' && redirect.startsWith('/') ? redirect : '/')
  } catch {
    auth.clear()
  } finally {
    loading.value = false
  }
}

// 在线体验：自动领取一个空闲演示账号（demo1~demoN，各占独立租户）并登录。
// 多个访客同时点击会分配到不同账号，数据互不影响，退出后该账号数据自动重置。
async function startDemo() {
  if (demoLoading.value) return
  demoLoading.value = true
  try {
    const account = await claimDemoAccount()
    const result = await login({
      username: account.username,
      password: account.password,
      tenant_id: account.tenant_id,
    })
    auth.setAuth(result)
    if ((result.perms ?? []).includes('wms:demo')) {
      const session = await acquireDemoSession()
      auth.setDemoSession(session)
      sessionStorage.setItem('WMS_DEMO_AUTO_OPEN', '1')
    }
    ElMessage({
      message: `已为你分配空闲演示账号 ${account.username}（共 ${account.total} 席，数据独立、退出自动重置）`,
      type: 'success',
      duration: 5000,
    })
    router.push('/')
  } catch {
    auth.clear()
    // 错误提示由 request.ts 拦截器统一弹出（如 70002 演示席位已满）
  } finally {
    demoLoading.value = false
  }
}

// 个人空间：进入访客自己挑选的持久体验账号（user1..userN，各占独立租户）。
// 数据长期保留：不重置、不占席位，退出后可用同一账号再次登录继续操作。
async function startPersonal() {
  if (personalLoading.value || !selectedAccount.value) return
  personalLoading.value = true
  try {
    const account = await claimPersonalAccount(selectedAccount.value)
    const result = await login({
      username: account.username,
      password: account.password ?? '',
      tenant_id: account.tenant_id,
    })
    auth.setAuth(result)
    ElMessage({
      message: `已进入 ${account.username}（${account.nickname}），数据长期保留，退出后可用同一账号再次登录`,
      type: 'success',
      duration: 5000,
    })
    router.push('/')
  } catch {
    auth.clear()
  } finally {
    personalLoading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <!-- 左侧品牌区 -->
    <div class="brand">
      <div class="brand-head">
        <div class="brand-logo">W</div>
        <div>
          <b>WMS</b>
          <small>轻量级仓储管理系统</small>
        </div>
      </div>
      <div class="brand-body">
        <h1 class="brand-title">让每一个库存数字<br />都值得信赖</h1>
        <p class="brand-desc">覆盖入库、上架、出库、库存与盘点全流程，让仓储作业更清晰、更高效。</p>
        <div class="brand-modules">
          <span>入库管理</span>
          <span>出库管理</span>
          <span>库存查询</span>
          <span>盘点管理</span>
        </div>
      </div>
    </div>

    <!-- 右侧表单区 -->
    <div class="panel">
      <div class="login-box">
        <div class="mini-logo">W</div>
        <h2>欢迎登录</h2>
        <p class="sub">输入账号进入工作台，或选择下方快捷体验</p>

        <!-- 快捷体验：两种账号的区别在切换页里各自说明，避免堆在一起 -->
        <template v-if="showEntries">
          <div v-if="showSwitch" class="entry-switch">
            <button
              type="button"
              class="entry-tab"
              :class="{ active: entry === 'demo' }"
              @click="entry = 'demo'"
            >
              在线体验
            </button>
            <button
              type="button"
              class="entry-tab"
              :class="{ active: entry === 'personal' }"
              @click="entry = 'personal'"
            >
              个人空间
            </button>
          </div>

          <!-- 在线体验：系统自动分配演示账号，数据退出即重置 -->
          <div v-if="entry === 'demo' && demoEnabled" class="entry-panel">
            <ul class="entry-points">
              <li>自动分配空闲演示席位，多人同时体验互不影响</li>
              <li>退出或 5 分钟无操作后，数据<b>自动重置</b></li>
              <li>含“业务流程中心”：一键模拟完整业务流程</li>
            </ul>
            <el-button
              type="primary"
              size="large"
              class="login-btn"
              :loading="demoLoading"
              @click="startDemo"
            >
              一键进入演示
            </el-button>
          </div>

          <!-- 个人空间：访客自己挑账号，数据长期保留 -->
          <div v-else-if="entry === 'personal' && personalEnabled" class="entry-panel">
            <p class="entry-desc">选择一个账号进入，数据<b>长期保留</b>，退出后仍可再次登录</p>
            <div v-if="personalAccounts.length" class="account-list">
              <button
                v-for="item in personalAccounts"
                :key="item.username"
                type="button"
                class="account-chip"
                :class="{ active: selectedAccount === item.username }"
                @click="selectedAccount = item.username"
              >
                {{ item.username }}
              </button>
            </div>
            <p v-else class="entry-desc empty">暂无可用账号，请使用下方账号密码登录</p>
            <el-button
              type="primary"
              size="large"
              class="login-btn"
              :disabled="!selectedAccount"
              :loading="personalLoading"
              @click="startPersonal"
            >
              {{ selectedAccount ? `进入 ${selectedAccount}` : '请先选择账号' }}
            </el-button>
            <p class="entry-note">公开共享账号，请勿存放真实敏感数据</p>
          </div>
        </template>

        <div v-if="showEntries" class="divider"><span>或使用账号密码登录</span></div>

        <el-form ref="formRef" :model="form" :rules="rules" size="large" @keyup.enter="submit">
          <el-form-item prop="username">
            <el-input v-model="form.username" placeholder="用户名">
              <template #prefix><el-icon><User /></el-icon></template>
            </el-input>
          </el-form-item>
          <el-form-item prop="password">
            <el-input v-model="form.password" type="password" show-password placeholder="密码">
              <template #prefix><el-icon><Lock /></el-icon></template>
            </el-input>
          </el-form-item>
          <el-form-item prop="tenant_id">
            <el-input
              v-model="form.tenant_id"
              placeholder="租户编号（可选，同名账号必填）"
              inputmode="numeric"
              maxlength="19"
            />
          </el-form-item>
          <!-- 快捷体验可用时，账密登录降为次按钮，避免两个主按钮抢焦点 -->
          <el-button
            :type="showEntries ? 'default' : 'primary'"
            size="large"
            class="login-btn"
            :loading="loading"
            @click="submit"
          >
            登 录
          </el-button>
        </el-form>
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  height: 100%;
  display: flex;
  overflow: hidden;
}

/* ---------- 左侧品牌区 ---------- */
.brand {
  flex: 1.2;
  min-width: 0;
  background: var(--gowms-login-brand-bg);
  color: #fff;
  padding: 56px 64px;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
}

/* 装饰光斑 */
.brand::before,
.brand::after {
  content: '';
  position: absolute;
  border-radius: 50%;
  pointer-events: none;
}

.brand::before {
  width: 420px;
  height: 420px;
  right: -140px;
  bottom: -160px;
  background: radial-gradient(circle, rgba(34, 211, 238, 0.18), transparent 70%);
}

.brand::after {
  width: 320px;
  height: 320px;
  left: -100px;
  top: -100px;
  background: radial-gradient(circle, rgba(255, 255, 255, 0.08), transparent 70%);
}

.brand-head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

/* 中部内容块：与右侧登录卡垂直居中对齐 */
.brand-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 32px 0;
  position: relative;
  z-index: 1;
}

.brand-logo {
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.16);
  backdrop-filter: blur(4px);
  border: 1px solid rgba(255, 255, 255, 0.25);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 800;
  font-size: 20px;
}

.brand-head b {
  font-size: 18px;
  letter-spacing: 0.5px;
  display: block;
}

.brand-head small {
  font-size: 12px;
  opacity: 0.75;
}

.brand-title {
  font-size: clamp(24px, 2.6vw, 34px);
  line-height: 1.45;
  margin: 0 0 32px;
  font-weight: 700;
}

.brand-desc {
  max-width: 520px;
  margin: -12px 0 24px;
  font-size: 15px;
  line-height: 1.8;
  opacity: 0.78;
}

.brand-modules {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.brand-modules span {
  padding: 8px 14px;
  border: 1px solid rgba(255, 255, 255, 0.2);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  font-size: 13px;
}

/* ---------- 右侧表单区 ---------- */
.panel {
  flex: 1;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  background: var(--el-bg-color-page);
  padding: 24px;
  overflow-y: auto;
}

.login-box {
  width: 380px;
  /* margin:auto 让卡片空间富余时居中、空间不足时从顶部开始并可滚动
     （若用 align-items:center，矮窗口下卡片顶部会被裁掉且无法上滚） */
  margin: auto 0;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 16px;
  padding: 32px;
  box-shadow: var(--el-box-shadow);
}

.login-box h2 {
  margin: 0 0 4px;
  font-size: 22px;
  color: var(--el-text-color-primary);
}

.login-box .sub {
  margin: 0 0 18px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

/* 品牌标识：桌面端由左侧品牌区承担，小屏（隐藏品牌区后）才显示 */
.mini-logo {
  display: none;
  width: 40px;
  height: 40px;
  margin-bottom: 14px;
  border-radius: 12px;
  background: var(--gowms-login-brand-bg);
  color: #fff;
  align-items: center;
  justify-content: center;
  font-weight: 800;
  font-size: 20px;
}

/* ---------- 快捷体验切换 ---------- */
.entry-switch {
  display: flex;
  gap: 4px;
  padding: 4px;
  margin-bottom: 14px;
  border-radius: 10px;
  background: var(--el-fill-color-light);
}

.entry-tab {
  flex: 1;
  border: none;
  border-radius: 8px;
  padding: 8px 0;
  background: transparent;
  color: var(--el-text-color-regular);
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s;
}

.entry-tab:hover {
  color: var(--el-color-primary);
}

.entry-tab.active {
  background: var(--el-bg-color);
  color: var(--el-color-primary);
  font-weight: 600;
  box-shadow: var(--el-box-shadow-light);
}

.entry-panel {
  margin-bottom: 4px;
}

.entry-points {
  margin: 0 0 14px;
  padding: 0;
  list-style: none;
}

.entry-points li {
  position: relative;
  padding-left: 14px;
  font-size: 13px;
  line-height: 1.9;
  color: var(--el-text-color-secondary);
}

.entry-points li::before {
  content: '';
  position: absolute;
  left: 2px;
  top: 0.72em;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--el-color-primary-light-5);
}

.entry-points b,
.entry-desc b {
  color: var(--el-color-primary);
  font-weight: 600;
}

.entry-desc {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.8;
  color: var(--el-text-color-secondary);
}

.entry-desc.empty {
  padding: 10px 0 2px;
  text-align: center;
}

.account-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
}

.account-chip {
  min-width: 64px;
  padding: 7px 14px;
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  background: transparent;
  color: var(--el-text-color-regular);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
}

.account-chip:hover {
  border-color: var(--el-color-primary-light-5);
  color: var(--el-color-primary);
}

.account-chip.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  font-weight: 600;
}

.entry-note {
  margin: 8px 0 0;
  font-size: 12px;
  text-align: center;
  color: var(--el-text-color-placeholder);
}

.divider {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 18px 0 16px;
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}

.divider::before,
.divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--el-border-color-lighter);
}

.login-btn {
  width: 100%;
  margin-top: 4px;
  letter-spacing: 4px;
}

.entry-panel .login-btn {
  letter-spacing: normal;
}

/* 小屏隐藏品牌区，登录卡内显示品牌标识 */
@media (max-width: 900px) {
  .brand {
    display: none;
  }

  .mini-logo {
    display: flex;
  }
}
</style>
