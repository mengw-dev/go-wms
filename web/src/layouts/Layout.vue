<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Fold, Expand } from '@element-plus/icons-vue'
import WmsNavigation from './WmsNavigation.vue'
import { changePassword, getProfile } from '@/api/auth'
import { releaseDemoSession } from '@/api/demo'
import DemoConsole from '@/components/DemoConsole.vue'
import ManualGuide from '@/components/demo/ManualGuide.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

const compactMedia = window.matchMedia('(max-width: 900px)')
const compact = ref(compactMedia.matches)
const sidebarVisible = ref(true)
const mobileMenuVisible = ref(false)
const navigationExpanded = computed(() => compact.value ? mobileMenuVisible.value : sidebarVisible.value)

function syncViewport() {
  compact.value = compactMedia.matches
  mobileMenuVisible.value = false
}
function toggleNavigation() {
  if (compact.value) mobileMenuVisible.value = !mobileMenuVisible.value
  else sidebarVisible.value = !sidebarVisible.value
}
onMounted(() => compactMedia.addEventListener('change', syncViewport))
onUnmounted(() => compactMedia.removeEventListener('change', syncViewport))
watch(() => route.fullPath, () => { mobileMenuVisible.value = false })

const pageTitle = computed(() => (route.meta.title as string) || '')

onMounted(async () => {
  const profile = await getProfile().catch(() => null)
  if (profile) auth.setProfile(profile)
})

async function onCommand(command: string) {
  if (command === 'logout') {
    try {
      await ElMessageBox.confirm('确定退出登录吗？', '提示', { type: 'warning' })
    } catch {
      return
    }
    if (auth.isDemo) {
      await releaseDemoSession().catch(() => undefined)
    }
    auth.clear()
    ElMessage.success('已退出登录')
    router.push('/login')
  } else if (command === 'password') {
    pwdDialog.visible = true
  }
}

// ---------- 修改密码 ----------
const pwdDialog = reactive({ visible: false, loading: false })
const pwdFormRef = ref<FormInstance>()
const pwdForm = reactive({ old_password: '', new_password: '', confirm_password: '' })

const pwdRules: FormRules = {
  old_password: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  new_password: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, max: 32, message: '密码长度 6-32 位', trigger: 'blur' },
  ],
  confirm_password: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        if (value !== pwdForm.new_password) {
          callback(new Error('两次输入的密码不一致'))
        } else {
          callback()
        }
      },
      trigger: 'blur',
    },
  ],
}

async function submitPassword() {
  const valid = await pwdFormRef.value?.validate().catch(() => false)
  if (!valid) return
  pwdDialog.loading = true
  try {
    await changePassword({
      old_password: pwdForm.old_password,
      new_password: pwdForm.new_password,
    })
    ElMessage.success('密码修改成功，请重新登录')
    pwdDialog.visible = false
    auth.clear()
    router.push('/login')
  } finally {
    pwdDialog.loading = false
  }
}
</script>

<template>
  <el-container class="layout">
    <el-aside v-if="!compact && sidebarVisible" id="desktop-navigation" width="220px" class="gowms-aside aside">
      <WmsNavigation />
    </el-aside>

    <el-container class="workspace">
      <el-header class="header">
        <div class="header-left">
          <el-button
            class="navigation-toggle" text circle
            :aria-label="navigationExpanded ? '收起导航' : '展开导航'"
            :aria-expanded="navigationExpanded"
            :aria-controls="compact ? 'mobile-navigation' : 'desktop-navigation'"
            @click="toggleNavigation"
          >
            <el-icon :size="20"><Fold v-if="navigationExpanded" /><Expand v-else /></el-icon>
          </el-button>
          <el-breadcrumb separator="/">
            <el-breadcrumb-item>WMS</el-breadcrumb-item>
            <el-breadcrumb-item>{{ pageTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <el-tooltip :content="theme.theme === 'dark' ? '切换为浅色模式' : '切换为深色模式'" placement="bottom">
            <el-button class="theme-btn" :aria-label="theme.theme === 'dark' ? '切换为浅色模式' : '切换为深色模式'" circle text @click="theme.toggle()">
              <el-icon :size="18">
                <Sunny v-if="theme.theme === 'dark'" />
                <Moon v-else />
              </el-icon>
            </el-button>
          </el-tooltip>
          <el-dropdown @command="onCommand">
            <button type="button" class="user-entry" aria-label="用户菜单">
              <span class="avatar">{{ auth.displayName.slice(0, 1) }}</span>
              <span class="user-name">{{ auth.displayName }}</span>
              <el-icon><ArrowDown /></el-icon>
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="password">修改密码</el-dropdown-item>
                <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>
      <el-main class="main">
        <div class="page-stage">
          <router-view />
        </div>
      </el-main>
    </el-container>

    <el-drawer
      v-model="mobileMenuVisible" title="导航菜单" direction="ltr"
      size="min(280px, 86vw)" class="navigation-drawer" destroy-on-close
    >
      <div id="mobile-navigation"><WmsNavigation @navigate="mobileMenuVisible = false" /></div>
    </el-drawer>

    <el-dialog
      v-model="pwdDialog.visible"
      title="修改密码"
      width="420px"
      destroy-on-close
      :close-on-click-modal="false"
      :close-on-press-escape="!pwdDialog.loading"
      :show-close="!pwdDialog.loading"
    >
      <el-form ref="pwdFormRef" :model="pwdForm" :rules="pwdRules" label-width="90px">
        <el-form-item label="原密码" prop="old_password">
          <el-input v-model="pwdForm.old_password" type="password" show-password placeholder="请输入原密码" />
        </el-form-item>
        <el-form-item label="新密码" prop="new_password">
          <el-input v-model="pwdForm.new_password" type="password" show-password placeholder="6-32 位" />
        </el-form-item>
        <el-form-item label="确认密码" prop="confirm_password">
          <el-input v-model="pwdForm.confirm_password" type="password" show-password placeholder="再次输入新密码" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="pwdDialog.loading" @click="pwdDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="pwdDialog.loading" @click="submitPassword">确定</el-button>
      </template>
    </el-dialog>
  </el-container>
  <DemoConsole />
  <ManualGuide />
</template>

<style scoped>
.layout {
  height: 100%;
}

/* ---------- 侧边栏（全部由布局令牌驱动，双主题自动切换） ---------- */
.aside {
  background-color: var(--gowms-sidebar-bg);
  border-right: 1px solid var(--gowms-sidebar-border);
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  transition: background-color 0.25s ease;
}

.aside::-webkit-scrollbar {
  width: 4px;
}

/* ---------- 顶栏 ---------- */
.header {
  background: var(--gowms-header-bg);
  border-bottom: 1px solid var(--gowms-header-border);
  box-shadow: var(--gowms-header-shadow);
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 60px;
  z-index: 1;
  transition: background-color 0.25s ease;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.theme-btn {
  color: var(--el-text-color-secondary);
}

.theme-btn:hover {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.user-entry {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  color: var(--el-text-color-primary);
  border: 0;
  background: transparent;
  padding: 4px;
  border-radius: 8px;
  font-family: inherit;
  font-size: 14px;
}

.avatar {
  width: 30px;
  height: 30px;
  border-radius: 50%;
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  font-size: 13px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}

.main {
  padding: 24px;
  min-width: 0;
  overflow-y: auto;
  background: var(--el-bg-color-page);
}

.page-stage {
  width: 100%;
  max-width: 1600px;
  height: 100%;
  margin: 0 auto;
}
.workspace { min-width: 0; }
.header-left { display: flex; align-items: center; gap: 12px; min-width: 0; }
.user-name { max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 900px) {
  .main { padding: 16px; }
  .header { padding: 0 12px; }
}
@media (max-width: 480px) {
  .main { padding: 12px; }
  .header-left { gap: 6px; }
  .header-right { gap: 4px; }
  .user-name { display: none; }
}
</style>
