import { expect, test } from '@playwright/test'
import {
  baseURL,
  loginByApi,
  loginByUi,
  requestApi,
  uniqueSuffix,
} from './support/api'

test('admin login, protected account and centered confirmation dialog', async ({ page, request }) => {
  await page.goto('/login')
  await expect(page.getByText('三数量库存模型')).toHaveCount(0)

  await loginByUi(page)
  await expect(page.getByText('技术亮点')).toHaveCount(0)

  await page.goto('/system/users')
  const adminRow = page.getByRole('row').filter({ hasText: 'admin' }).first()
  await expect(adminRow).toContainText('内置管理员')
  await expect(adminRow.getByRole('switch')).toBeDisabled()
  await expect(adminRow.getByRole('button', { name: '编辑' })).toBeDisabled()
  await expect(adminRow.getByRole('button', { name: '删除' })).toBeDisabled()

  const token = await loginByApi(request)
  const userResponse = await request.fetch('/api/v1/system/users/1/status', {
    method: 'PUT',
    headers: { Authorization: `Bearer ${token}` },
    data: { status: 0 },
  })
  expect(userResponse.status()).toBe(400)
  expect((await userResponse.json()).code).toBe(10011)

  const roleResponse = await request.fetch('/api/v1/system/roles/1', {
    method: 'PUT',
    headers: { Authorization: `Bearer ${token}` },
    data: { name: 'admin', perms: '*', remark: 'protected' },
  })
  expect(roleResponse.status()).toBe(400)
  expect((await roleResponse.json()).code).toBe(10013)

  await page.locator('.user-entry').click()
  await page.getByText('退出登录', { exact: true }).click()

  const messageBox = page.locator('.el-message-box')
  await expect(messageBox).toBeVisible()
  const box = await messageBox.boundingBox()
  const viewport = page.viewportSize()
  expect(box, 'confirmation dialog should have a layout box').toBeTruthy()
  expect(viewport, 'viewport should be available').toBeTruthy()
  expect(Math.abs(box!.x + box!.width / 2 - viewport!.width / 2)).toBeLessThan(8)
  expect(Math.abs(box!.y + box!.height / 2 - viewport!.height / 2)).toBeLessThan(24)
  await messageBox.getByRole('button', { name: '取消' }).click()
})

test('personal space entry lists persistent accounts and signs in with the chosen one', async ({ page }) => {
  await page.goto('/login')
  // "个人空间"入口：访客自己挑选持久账号（user1..userN），数据长期保留
  await page.getByRole('button', { name: '个人空间' }).click()
  const account = page.getByRole('button', { name: 'user1', exact: true })
  await expect(account).toBeVisible()
  await account.click()
  await page.getByRole('button', { name: '进入 user1' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByRole('heading', { name: /欢迎回来/ })).toBeVisible()
  // 持久账号不含 wms:demo：不应出现演示中心的悬浮入口
  await expect(page.getByText('业务流程中心')).toHaveCount(0)
})

test('limited user cannot see or open system management', async ({ browser, request }) => {
  const suffix = uniqueSuffix()
  const adminToken = await loginByApi(request)
  const roleName = `E2E-LIMITED-${suffix}`
  const username = `e2e_limited_${suffix.toLowerCase()}`
  const password = 'e2e-pass-123'

  await requestApi(request, adminToken, 'POST', '/system/roles', {
    name: roleName,
    perms: 'wms:inventory',
    remark: 'Playwright limited role',
  })
  const roles = await requestApi<{ list: Array<{ id: string; name: string }> }>(
    request,
    adminToken,
    'GET',
    `/system/roles?page=1&page_size=100&keyword=${encodeURIComponent(roleName)}`,
  )
  const role = roles.list.find((item) => item.name === roleName)
  expect(role, 'limited role should exist').toBeTruthy()

  await requestApi(request, adminToken, 'POST', '/system/users', {
    username,
    password,
    nickname: '受限用户',
    role_ids: [role!.id],
  })
  const users = await requestApi<{ list: Array<{ id: string; username: string }> }>(
    request,
    adminToken,
    'GET',
    `/system/users?page=1&page_size=100&keyword=${encodeURIComponent(username)}`,
  )
  const user = users.list.find((item) => item.username === username)
  expect(user, 'limited user should exist').toBeTruthy()

  const context = await browser.newContext({ baseURL })
  const limitedPage = await context.newPage()
  try {
    await loginByUi(limitedPage, username, password)
    await expect(limitedPage.getByText('系统管理', { exact: true })).toHaveCount(0)
    await limitedPage.goto('/system/users')
    await expect(limitedPage).toHaveURL(/\/dashboard$/)
  } finally {
    await context.close()
    await requestApi(request, adminToken, 'DELETE', `/system/users/${user!.id}`)
    await requestApi(request, adminToken, 'DELETE', `/system/roles/${role!.id}`)
  }
})
