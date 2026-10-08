<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { TrendCharts } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const emit = defineEmits<{ navigate: [] }>()
const route = useRoute()
const auth = useAuthStore()
const activeMenu = computed(() => (route.meta.activeMenu as string | undefined) || route.path)
</script>

<template>
  <nav class="gowms-aside navigation" aria-label="主导航">
    <div class="logo">
      <div class="logo-mark">W</div>
      <div class="logo-text">
        <b>WMS</b>
        <small>仓储管理系统</small>
      </div>
    </div>
    <el-menu :default-active="activeMenu" router class="menu" @select="emit('navigate')">
      <el-menu-item index="/dashboard">
        <el-icon><Odometer /></el-icon>
        <span>仪表盘</span>
      </el-menu-item>
      <el-sub-menu v-if="auth.isDemo" index="demo-center">
        <template #title>
          <el-icon><TrendCharts /></el-icon>
          <span>演示中心</span>
        </template>
        <el-menu-item index="/demo">开始演示</el-menu-item>
        <el-menu-item index="/demo/activity">业务证据</el-menu-item>
        <el-menu-item index="/demo/performance">工程验证</el-menu-item>
      </el-sub-menu>
      <el-sub-menu v-if="auth.hasPerm('wms:inbound:view')" index="inbound">
        <template #title>
          <el-icon><Download /></el-icon>
          <span>入库管理</span>
        </template>
        <el-menu-item index="/inbound/orders">入库单</el-menu-item>
      </el-sub-menu>
      <el-sub-menu v-if="auth.hasPerm('wms:outbound:view')" index="outbound">
        <template #title>
          <el-icon><Upload /></el-icon>
          <span>出库管理</span>
        </template>
        <el-menu-item index="/outbound/orders">出库单</el-menu-item>
      </el-sub-menu>
      <el-sub-menu v-if="auth.hasPerm('wms:inventory') || auth.hasPerm('wms:task')" index="inventory">
        <template #title>
          <el-icon><Coin /></el-icon>
          <span>库存管理</span>
        </template>
        <el-menu-item v-if="auth.hasPerm('wms:inventory')" index="/inventory">库存查询</el-menu-item>
        <el-menu-item v-if="auth.hasPerm('wms:inventory')" index="/ai">AI 问答</el-menu-item>
        <el-menu-item v-if="auth.hasPerm('wms:task')" index="/tasks">任务中心</el-menu-item>
      </el-sub-menu>
      <el-sub-menu v-if="auth.hasPerm('wms:stocktake:view')" index="stocktake">
        <template #title>
          <el-icon><Tickets /></el-icon>
          <span>盘点管理</span>
        </template>
        <el-menu-item index="/stocktake/orders">盘点单</el-menu-item>
      </el-sub-menu>
      <el-sub-menu v-if="auth.hasPerm('wms:basic')" index="basic">
        <template #title>
          <el-icon><OfficeBuilding /></el-icon>
          <span>基础数据</span>
        </template>
        <el-menu-item index="/basic/warehouses">仓库管理</el-menu-item>
        <el-menu-item index="/basic/locations">库位管理</el-menu-item>
        <el-menu-item index="/basic/skus">货品管理</el-menu-item>
      </el-sub-menu>
      <el-sub-menu v-if="auth.hasPerm('wms:system:user') || auth.hasPerm('wms:system:role') || auth.hasPerm('wms:system:log')" index="system">
        <template #title>
          <el-icon><Setting /></el-icon>
          <span>系统管理</span>
        </template>
        <el-menu-item v-if="auth.hasPerm('wms:system:user')" index="/system/users">用户管理</el-menu-item>
        <el-menu-item v-if="auth.hasPerm('wms:system:role')" index="/system/roles">角色管理</el-menu-item>
        <el-menu-item v-if="auth.hasPerm('wms:system:log')" index="/system/logs">操作日志</el-menu-item>
      </el-sub-menu>
    </el-menu>
  </nav>
</template>

<style scoped>
.navigation { min-height: 100%; display: flex; flex-direction: column; }
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
.menu :deep(.el-menu-item),
.menu :deep(.el-sub-menu__title) {
  border-radius: 8px;
  margin: 2px 0;
  height: 42px;
  line-height: 42px;
  position: relative;
}

.menu :deep(.el-menu-item.is-active) {
  background: var(--gowms-sidebar-active-bg);
  font-weight: 600;
}

.menu :deep(.el-menu-item.is-active)::before {
  content: '';
  position: absolute;
  left: 0;
  top: 20%;
  bottom: 20%;
  width: 3px;
  border-radius: 2px;
  background: var(--gowms-sidebar-active-text);
}

</style>
