import { expect, test, type Page } from '@playwright/test'

// 只模拟登录相关 API，不启动或写入真实业务数据库。
async function mockLoginPage(page: Page, options = { demo: false, personal: false }) {
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown
    switch (path) {
      case '/api/v1/version':
        data = { version: 'test', demo_enabled: options.demo, personal_enabled: options.personal }
        break
      case '/api/v1/demo/account':
        data = { username: 'demo1', password: 'demo-password', tenant_id: '10001', total: 1, occupied: 0 }
        break
      case '/api/v1/personal/accounts':
        data = [{ username: 'user1', nickname: '测试账号', tenant_id: '20001' }]
        break
      case '/api/v1/personal/login':
        data = { username: 'user1', nickname: '测试账号', password: 'personal-password', tenant_id: '20001' }
        break
      default:
        await route.abort()
        return
    }
    await route.fulfill({ json: { code: 0, msg: 'ok', data } })
  })
}

for (const tenantID of ['', '0', '9007199254740993']) {
  test(`manual login preserves tenant selection: ${tenantID || 'omitted'}`, async ({ page }) => {
    await mockLoginPage(page)
    await page.route('**/api/v1/login', (route) => route.fulfill({
      status: 400,
      json: { code: 10002, msg: '用户名或密码错误' },
    }))
    await page.goto('/login')
    await page.getByPlaceholder('用户名', { exact: true }).fill('tester')
    await page.getByPlaceholder('密码', { exact: true }).fill('test-password')
    await page.getByPlaceholder('租户编号（可选，同名账号必填）').fill(tenantID)
    const requestPromise = page.waitForRequest('**/api/v1/login')
    await page.getByRole('button', { name: /登\s*录/ }).click()
    const request = await requestPromise
    expect(request.postDataJSON()).toEqual({
      username: 'tester', password: 'test-password', ...(tenantID ? { tenant_id: tenantID } : {}),
    })
    await expect(page.getByText('用户名或密码错误', { exact: true })).toBeVisible()
    await expect(page).toHaveURL(/\/login$/)
  })
}

test('demo login sends its tenant and clears auth if session acquisition fails', async ({ page }) => {
  await mockLoginPage(page, { demo: true, personal: false })
  await page.route('**/api/v1/login', (route) => route.fulfill({
    json: { code: 0, msg: 'ok', data: {
      token: 'test-demo-token', user_id: '42', username: 'demo1', nickname: '演示', roles: ['demo'], perms: ['wms:demo'],
    } },
  }))
  await page.route('**/api/v1/demo/session/acquire', (route) => route.fulfill({
    status: 423, json: { code: 70002, msg: '演示席位已满' },
  }))
  await page.goto('/login')
  const requestPromise = page.waitForRequest('**/api/v1/login')
  await page.getByRole('button', { name: '一键进入演示' }).click()
  expect((await requestPromise).postDataJSON()).toEqual({
    username: 'demo1', password: 'demo-password', tenant_id: '10001',
  })
  await expect(page.getByText('演示席位已满', { exact: true })).toBeVisible()
  await expect.poll(() => page.evaluate(() => ({
    local: localStorage.getItem('WMS_TOKEN'), session: sessionStorage.getItem('WMS_TOKEN'),
  }))).toEqual({ local: null, session: null })
  await expect(page).toHaveURL(/\/login$/)
})

test('personal login sends the selected account tenant', async ({ page }) => {
  await mockLoginPage(page, { demo: false, personal: true })
  await page.route('**/api/v1/login', (route) => route.fulfill({
    status: 400, json: { code: 10002, msg: '用户名或密码错误' },
  }))
  await page.goto('/login')
  await page.getByRole('button', { name: 'user1', exact: true }).click()
  const requestPromise = page.waitForRequest('**/api/v1/login')
  await page.getByRole('button', { name: '进入 user1' }).click()
  expect((await requestPromise).postDataJSON()).toEqual({
    username: 'user1', password: 'personal-password', tenant_id: '20001',
  })
})

test('demo login enters the dedicated home with the first-session tour', async ({ page }) => {
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown
    switch (path) {
      case '/api/v1/version':
        data = { version: 'test', demo_enabled: true, personal_enabled: false }
        break
      case '/api/v1/demo/account':
        data = { username: 'demo1', password: 'demo-password', tenant_id: '10001', total: 1, occupied: 0 }
        break
      case '/api/v1/login':
        data = {
          token: 'test-demo-token',
          user_id: '42',
          username: 'demo1',
          nickname: '演示用户',
          roles: ['demo'],
          perms: ['wms:demo'],
        }
        break
      case '/api/v1/demo/session/acquire':
      case '/api/v1/demo/session/heartbeat':
        data = { session_id: 'test-session', expires_in: 300 }
        break
      case '/api/v1/profile':
        data = {
          user_id: '42',
          username: 'demo1',
          nickname: '演示用户',
          roles: ['demo'],
          perms: ['wms:demo'],
        }
        break
      case '/api/v1/demo/activity':
        data = {
          operations: [],
          inbound_orders: [],
          outbound_orders: [],
          stocktake_orders: [],
          tasks: [],
          inventory_trans: [],
        }
        break
      default:
        await route.abort()
        return
    }
    await route.fulfill({ json: { code: 0, msg: 'ok', data } })
  })

  await page.goto('/login')
  await page.getByRole('button', { name: '一键进入演示' }).click()

  await expect(page).toHaveURL(/\/demo$/)
  await expect(page.getByRole('heading', { name: '从真实业务流程理解这套 WMS' })).toBeVisible()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toBeVisible()
  const dialog = page.getByRole('dialog', { name: 'Demo 快捷控制器' })
  await expect(dialog).not.toBeVisible()

  await page.getByRole('button', { name: '跳过导览', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toHaveCount(0)
  await expect.poll(() => page.evaluate(() => sessionStorage.getItem('WMS_DEMO_TOUR_SEEN:test-session'))).toBe('1')
  await page.getByRole('button', { name: '快速导览', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toBeVisible()
  for (const title of ['推荐业务闭环', '结果与证据', '工程验证', '项目与源码']) {
    await page.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(page.getByRole('heading', { name: title })).toBeVisible()
  }
  await page.getByRole('button', { name: '开始体验完整业务闭环', exact: true }).click()
  await expect(dialog).toBeVisible()
})
