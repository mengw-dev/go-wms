<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { changePassword } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

const activeMenu = computed(() => {
  const menu = route.meta.activeMenu as string | undefined
  return menu || route.path
})

const pageTitle = computed(() => (route.meta.title as string) || '')

async function onCommand(command: string) {
  if (command === 'logout') {
    try {
      await ElMessageBox.confirm('确定退出登录吗？', '提示', { type: 'warning' })
    } catch {
      return
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
    <el-aside width="220px" class="gowms-aside aside">
      <div class="logo">
        <div class="logo-mark">W</div>
        <div class="logo-text">
          <b>GoWMS</b>
          <small>仓储管理系统</small>
        </div>
      </div>
      <el-menu :default-active="activeMenu" router class="menu">
        <el-menu-item index="/dashboard">
          <el-icon><Odometer /></el-icon>
          <span>仪表盘</span>
        </el-menu-item>
        <el-sub-menu index="inbound">
          <template #title>
            <el-icon><Download /></el-icon>
            <span>入库管理</span>
          </template>
          <el-menu-item index="/inbound/orders">入库单</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="outbound">
          <template #title>
            <el-icon><Upload /></el-icon>
            <span>出库管理</span>
          </template>
          <el-menu-item index="/outbound/orders">出库单</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="inventory">
          <template #title>
            <el-icon><Coin /></el-icon>
            <span>库存管理</span>
          </template>
          <el-menu-item index="/inventory">库存查询</el-menu-item>
          <el-menu-item index="/tasks">任务中心</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="stocktake">
          <template #title>
            <el-icon><Tickets /></el-icon>
            <span>盘点管理</span>
          </template>
          <el-menu-item index="/stocktake/orders">盘点单</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="basic">
          <template #title>
            <el-icon><OfficeBuilding /></el-icon>
            <span>基础数据</span>
          </template>
          <el-menu-item index="/basic/warehouses">仓库管理</el-menu-item>
          <el-menu-item index="/basic/locations">库位管理</el-menu-item>
          <el-menu-item index="/basic/skus">货品管理</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="system">
          <template #title>
            <el-icon><Setting /></el-icon>
            <span>系统管理</span>
          </template>
          <el-menu-item index="/system/users">用户管理</el-menu-item>
          <el-menu-item index="/system/roles">角色管理</el-menu-item>
          <el-menu-item index="/system/logs">操作日志</el-menu-item>
        </el-sub-menu>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-left">
          <el-breadcrumb separator="/">
            <el-breadcrumb-item>GoWMS</el-breadcrumb-item>
            <el-breadcrumb-item>{{ pageTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <el-tooltip :content="theme.theme === 'dark' ? '切换为浅色模式' : '切换为深色模式'" placement="bottom">
            <el-button class="theme-btn" circle text @click="theme.toggle()">
              <el-icon :size="18">
                <Sunny v-if="theme.theme === 'dark'" />
                <Moon v-else />
              </el-icon>
            </el-button>
          </el-tooltip>
          <el-dropdown @command="onCommand">
            <span class="user-entry">
              <span class="avatar">{{ auth.displayName.slice(0, 1) }}</span>
              {{ auth.displayName }}
              <el-icon><ArrowDown /></el-icon>
            </span>
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
        <router-view />
      </el-main>
    </el-container>

    <el-dialog v-model="pwdDialog.visible" title="修改密码" width="420px" destroy-on-close>
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
        <el-button @click="pwdDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="pwdDialog.loading" @click="submitPassword">确定</el-button>
      </template>
    </el-dialog>
  </el-container>
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

.logo {
  height: 60px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  border-bottom: 1px solid var(--gowms-sidebar-border);
  flex-shrink: 0;
}

.logo-mark {
  width: 32px;
  height: 32px;
  border-radius: 10px;
  background: var(--gowms-logo-gradient);
  color: #fff;
  font-weight: 800;
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px var(--gowms-sidebar-active-bg);
}

.logo-text {
  line-height: 1.2;
  color: var(--el-text-color-primary);
}

.logo-text b {
  font-size: 16px;
  letter-spacing: 0.5px;
}

.logo-text small {
  display: block;
  font-size: 11px;
  color: var(--el-text-color-secondary);
  font-weight: 400;
}

.menu {
  border-right: none;
  padding: 8px 10px;
  flex: 1;
}

/* 菜单行圆角 + 选中态底色与左侧色条 */
.gowms-aside .menu :deep(.el-menu-item),
.gowms-aside .menu :deep(.el-sub-menu__title) {
  border-radius: 8px;
  margin: 2px 0;
  height: 42px;
  line-height: 42px;
  position: relative;
}

.gowms-aside .menu :deep(.el-menu-item.is-active) {
  background: var(--gowms-sidebar-active-bg);
  font-weight: 600;
}

.gowms-aside .menu :deep(.el-menu-item.is-active)::before {
  content: '';
  position: absolute;
  left: 0;
  top: 20%;
  bottom: 20%;
  width: 3px;
  border-radius: 2px;
  background: var(--gowms-sidebar-active-text);
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
  outline: none;
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
  padding: 16px;
  overflow-y: auto;
  background: var(--el-bg-color-page);
}
</style>
