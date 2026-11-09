import { expect, test, type BrowserContext, type Page } from '@playwright/test'
import { dashboardFixture } from './support/dashboard'

// 真正双标签页、真实登录UI与浏览器存储，HTTP为受控夹具，不代表服务端权限验收。
async function fixture(context: BrowserContext) {
  await context.route('**/api/v1/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname.replace('/api/v1', '')
    const username = path === '/login' ? request.postDataJSON().username : request.headers().authorization?.replace('Bearer token-', '') || 'A'
    const user = { user_id: username === 'A' ? '1' : '2', username, nickname: `用户${username}`, roles: [], perms: ['*'] }
    let data: unknown = { list: [], total: 0 }
    if (path === '/version') data = { demo_enabled: false, personal_enabled: false }
    else if (path === '/login') data = { ...user, token: `token-${username}` }
    else if (path === '/profile') data = user
    else if (path === '/dashboard/overview') data = dashboardFixture()
    else if (path === '/inventory/alerts') data = { list: [], total: 0, warehouses: [], summary: { configured: 0, out_of_stock: 0, low: 0 }, checked_at: '' }
    else if (path === '/inbound/imports') data = []
    await route.fulfill({ json: { code: 0, msg: 'ok', data } })
  })
}
async function login(page: Page, username: string) {
  await page.goto('/login')
  await page.getByPlaceholder('用户名', { exact: true }).fill(username)
  await page.getByPlaceholder('密码', { exact: true }).fill('password')
  await page.getByRole('button', { name: /登\s*录/ }).click()
  await expect(page.locator('.user-name')).toHaveText(`用户${username}`)
}
async function logout(page: Page) {
  await page.getByRole('button', { name: '用户菜单' }).click()
  await page.getByRole('menuitem', { name: '退出登录' }).click()
  await page.getByRole('button', { name: '确定', exact: true }).click()
  await expect(page).toHaveURL(/\/login/)
}

test('共享普通登录：另一页退出和切换账号后旧页刷新身份', async ({ context, page }) => {
  await fixture(context); await login(page, 'A')
  const second = await context.newPage(); await second.goto('/dashboard')
  await expect(second.locator('.user-name')).toHaveText('用户A')
  await logout(second); await expect(page).toHaveURL(/\/login/)
  await login(second, 'B')
  await expect(page.locator('.user-name')).toHaveText('用户B')
  await page.reload(); await expect(page.locator('.user-name')).toHaveText('用户B')
})

test('storage事件延迟时旧401仍不能删除新登录凭据', async ({ context, page }) => {
  await fixture(context)
  // 模拟后台标签页暂未处理共享存储事件；响应处理本身必须再次核对存储。
  await page.addInitScript(() => window.addEventListener('storage', event => event.stopImmediatePropagation(), true))
  let release!: () => void, started!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  const requestStarted = new Promise<void>(resolve => { started = resolve })
  let delayed = false
  await page.route('**/api/v1/profile', async route => {
    if (!delayed) {
      delayed = true; started(); await gate
      await route.fulfill({ status: 401, json: { code: 40100, msg: 'expired' } })
    } else await route.fallback()
  })
  await login(page, 'A'); await requestStarted
  const second = await context.newPage(); await second.goto('/dashboard')
  await logout(second); await login(second, 'B')
  await expect(page.locator('.user-name')).toHaveText('用户A')
  release()
  await expect(page.locator('.user-name')).toHaveText('用户B')
  await second.reload(); await expect(second.locator('.user-name')).toHaveText('用户B')
})
