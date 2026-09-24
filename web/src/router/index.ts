import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import Layout from '@/layouts/Layout.vue'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/login/index.vue'),
    meta: { title: '登录' },
  },
  {
    path: '/',
    component: Layout,
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import('@/views/dashboard/index.vue'),
        meta: { title: '仪表盘' },
      },
      {
        path: 'demo',
        name: 'demo-home',
        component: () => import('@/views/demo/index.vue'),
        meta: { title: '演示中心', perm: 'wms:demo' },
      },
      {
        path: 'demo/performance',
        name: 'demo-performance',
        component: () => import('@/views/demo/Performance.vue'),
        meta: { title: '工程验证', perm: 'wms:demo' },
      },
      {
        path: 'demo/activity',
        name: 'demo-activity',
        component: () => import('@/views/demo/Activity.vue'),
        meta: { title: '业务证据', perm: 'wms:demo' },
      },
      {
        path: 'system/users',
        name: 'system-users',
        component: () => import('@/views/system/Users.vue'),
        meta: { title: '用户管理', perm: 'wms:system:user' },
      },
      {
        path: 'system/roles',
        name: 'system-roles',
        component: () => import('@/views/system/Roles.vue'),
        meta: { title: '角色管理', perm: 'wms:system:role' },
      },
      {
        path: 'system/logs',
        name: 'system-logs',
        component: () => import('@/views/system/OperLogs.vue'),
        meta: { title: '操作日志', perm: 'wms:system:log' },
      },
      {
        path: 'basic/warehouses',
        name: 'basic-warehouses',
        component: () => import('@/views/basic/Warehouses.vue'),
        meta: { title: '仓库管理', perm: 'wms:basic' },
      },
      {
        path: 'basic/locations',
        name: 'basic-locations',
        component: () => import('@/views/basic/Locations.vue'),
        meta: { title: '库位管理', perm: 'wms:basic' },
      },
      {
        path: 'basic/skus',
        name: 'basic-skus',
        component: () => import('@/views/basic/Skus.vue'),
        meta: { title: '货品管理', perm: 'wms:basic' },
      },
      {
        path: 'inventory',
        name: 'inventory',
        component: () => import('@/views/inventory/Inventory.vue'),
        meta: { title: '库存查询', perm: 'wms:inventory' },
      },
      {
        path: 'ai',
        name: 'ai',
        component: () => import('@/views/ai/index.vue'),
        meta: { title: 'AI 问答', perm: 'wms:inventory' },
      },
      {
        path: 'inbound/orders',
        name: 'inbound-orders',
        component: () => import('@/views/inbound/InboundOrders.vue'),
        meta: { title: '入库单', perm: 'wms:inbound:view' },
      },
      {
        path: 'inbound/orders/:id',
        name: 'inbound-order-detail',
        component: () => import('@/views/inbound/InboundOrderDetail.vue'),
        meta: { title: '入库单详情', activeMenu: '/inbound/orders', perm: 'wms:inbound:view' },
      },
      {
        path: 'outbound/orders',
        name: 'outbound-orders',
        component: () => import('@/views/outbound/OutboundOrders.vue'),
        meta: { title: '出库单', perm: 'wms:outbound:view' },
      },
      {
        path: 'outbound/orders/:id',
        name: 'outbound-order-detail',
        component: () => import('@/views/outbound/OutboundOrderDetail.vue'),
        meta: { title: '出库单详情', activeMenu: '/outbound/orders', perm: 'wms:outbound:view' },
      },
      {
        path: 'stocktake/orders',
        name: 'stocktake-orders',
        component: () => import('@/views/stocktake/StocktakeOrders.vue'),
        meta: { title: '盘点单', perm: 'wms:stocktake:view' },
      },
      {
        path: 'stocktake/orders/:id',
        name: 'stocktake-order-detail',
        component: () => import('@/views/stocktake/StocktakeOrderDetail.vue'),
        meta: { title: '盘点单详情', activeMenu: '/stocktake/orders', perm: 'wms:stocktake:view' },
      },
      {
        path: 'tasks',
        name: 'tasks',
        component: () => import('@/views/task/Tasks.vue'),
        meta: { title: '任务中心', perm: 'wms:task' },
      },
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/dashboard',
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!auth.isLoggedIn && to.path !== '/login') {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (auth.isLoggedIn && to.path === '/login') {
    return { path: '/' }
  }
  const requiredPerm = to.meta.perm as string | undefined
  if (requiredPerm && !auth.hasPerm(requiredPerm)) {
    return { path: '/dashboard' }
  }
  return true
})

router.afterEach((to) => {
  const title = to.meta.title as string | undefined
  document.title = title ? `${title} - WMS 仓储管理系统` : 'WMS 仓储管理系统'
})

export default router
