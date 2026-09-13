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
        path: 'system/users',
        name: 'system-users',
        component: () => import('@/views/system/Users.vue'),
        meta: { title: '用户管理' },
      },
      {
        path: 'system/roles',
        name: 'system-roles',
        component: () => import('@/views/system/Roles.vue'),
        meta: { title: '角色管理' },
      },
      {
        path: 'system/logs',
        name: 'system-logs',
        component: () => import('@/views/system/OperLogs.vue'),
        meta: { title: '操作日志' },
      },
      {
        path: 'basic/warehouses',
        name: 'basic-warehouses',
        component: () => import('@/views/basic/Warehouses.vue'),
        meta: { title: '仓库管理' },
      },
      {
        path: 'basic/locations',
        name: 'basic-locations',
        component: () => import('@/views/basic/Locations.vue'),
        meta: { title: '库位管理' },
      },
      {
        path: 'basic/skus',
        name: 'basic-skus',
        component: () => import('@/views/basic/Skus.vue'),
        meta: { title: '货品管理' },
      },
      {
        path: 'inventory',
        name: 'inventory',
        component: () => import('@/views/inventory/Inventory.vue'),
        meta: { title: '库存查询' },
      },
      {
        path: 'inbound/orders',
        name: 'inbound-orders',
        component: () => import('@/views/inbound/InboundOrders.vue'),
        meta: { title: '入库单' },
      },
      {
        path: 'inbound/orders/:id',
        name: 'inbound-order-detail',
        component: () => import('@/views/inbound/InboundOrderDetail.vue'),
        meta: { title: '入库单详情', activeMenu: '/inbound/orders' },
      },
      {
        path: 'outbound/orders',
        name: 'outbound-orders',
        component: () => import('@/views/outbound/OutboundOrders.vue'),
        meta: { title: '出库单' },
      },
      {
        path: 'outbound/orders/:id',
        name: 'outbound-order-detail',
        component: () => import('@/views/outbound/OutboundOrderDetail.vue'),
        meta: { title: '出库单详情', activeMenu: '/outbound/orders' },
      },
      {
        path: 'stocktake/orders',
        name: 'stocktake-orders',
        component: () => import('@/views/stocktake/StocktakeOrders.vue'),
        meta: { title: '盘点单' },
      },
      {
        path: 'stocktake/orders/:id',
        name: 'stocktake-order-detail',
        component: () => import('@/views/stocktake/StocktakeOrderDetail.vue'),
        meta: { title: '盘点单详情', activeMenu: '/stocktake/orders' },
      },
      {
        path: 'tasks',
        name: 'tasks',
        component: () => import('@/views/task/Tasks.vue'),
        meta: { title: '任务中心' },
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
  return true
})

router.afterEach((to) => {
  const title = to.meta.title as string | undefined
  document.title = title ? `${title} - WMS 仓储管理系统` : 'WMS 仓储管理系统'
})

export default router
