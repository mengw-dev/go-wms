import { expect, test, type Page } from '@playwright/test'

// Layout and event-contract tests are isolated from real demo leases/business writes.
const user = { user_id: '9001', username: 'demo1', nickname: '演示访客1', roles: ['demo'], perms: ['wms:demo', 'wms:inbound:view', 'wms:outbound:view', 'wms:inventory', 'wms:basic', 'wms:task'] }
const runtimeErrors = new WeakMap<Page, string[]>()
test.beforeEach(({ page }) => { const errors: string[] = []; runtimeErrors.set(page, errors); page.on('pageerror', error => errors.push(error.message)) })
test.afterEach(({ page }) => { expect(runtimeErrors.get(page)).toEqual([]) })

async function openHome(page: Page, theme: 'light' | 'dark' = 'light') {
  await page.addInitScript(({ user, theme }) => {
    sessionStorage.setItem('WMS_TOKEN', 'layout-test-token')
    sessionStorage.setItem('WMS_USER', JSON.stringify(user))
    sessionStorage.setItem('WMS_DEMO_SESSION', 'layout-test-session')
    if (!localStorage.getItem('gowms_theme')) localStorage.setItem('gowms_theme', theme)
    const launches: unknown[] = []
    Object.assign(window, { layoutLaunches: launches })
    // Register before the app: inspect its public launch contract without starting writes.
    window.addEventListener('wms:run-demo-staged', (event) => { launches.push((event as CustomEvent).detail); event.stopImmediatePropagation() })
  }, { user, theme })
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown = { list: [], total: 0 }
    if (path.endsWith('/profile')) data = user
    else if (path.includes('/demo/session/')) data = { session_id: 'layout-test-session', expires_in: 300 }
    else if (path.endsWith('/demo/activity')) data = { operations: [], inbound_orders: [], outbound_orders: [], stocktake_orders: [], tasks: [], inventory_trans: [] }
    else if (path.endsWith('/demo/performance')) data = { checked_at: '', database: { status: 'ok' }, redis: { status: 'ok' }, pool: {}, runtime: {}, business: {} }
    await route.fulfill({ json: { code: 0, msg: 'ok', data } })
  })
  await page.goto('/demo')
  await expect(page.getByRole('heading', { name: 'WMS 业务闭环', exact: true })).toBeVisible()
  await expect(page.locator('.demo-statusbar.home-dock')).toBeVisible()
}

for (const theme of ['light', 'dark'] as const) {
  test(`demo home keeps all stages and controls in one screen in ${theme} theme`, async ({ page }, testInfo) => {
    await openHome(page, theme)
    // A real-length evidence label exercises wrapping, not just the empty state.
    await page.evaluate(() => {
      sessionStorage.setItem('wms-demo-evidence-context-v1', JSON.stringify({ scenario: 'inbound', startedAt: new Date().toISOString(), completedAt: new Date().toISOString(), summary: '入库执行记录', evidence: [{ label: '入库单', value: 'RK2026102012345678901234567890' }], links: [] }))
      window.dispatchEvent(new Event('wms:data-changed'))
    })
    for (const [width, height] of [[1920, 914], [1536, 730], [1366, 650], [1280, 620], [1024, 600]]) {
      await page.setViewportSize({ width, height })
      for (const stage of ['入库单', '收货上架', '库存形成', '出库单', '拣货发货']) {
        await page.locator('.flow-step').getByText(stage, { exact: true }).click()
        await expect(page.locator('.detail-copy p')).toBeVisible()
        await expect(page.locator('.detail-actions').getByRole('button', { name: '自动演示', exact: true })).toBeVisible()
        await expect(page.locator('.detail-actions').getByRole('button', { name: '查看操作日志', exact: true })).toBeVisible()
        const layout = await page.evaluate(() => {
          const main = document.querySelector('.main') as HTMLElement
          const clipped: string[] = []
          for (const selector of ['.flow-section', '.experience-panel', '.engineering-card']) {
            for (const container of document.querySelectorAll(selector)) {
              const r = container.getBoundingClientRect()
              for (const child of container.querySelectorAll('button, p, h2, h3, dd')) {
                const b = child.getBoundingClientRect()
                if (b.width && b.height && (b.bottom > r.bottom + 1 || b.right > r.right + 1 || b.left < r.left - 1)) clipped.push(child.textContent || '')
              }
            }
          }
          const footer = document.querySelector('.demo-statusbar')!.getBoundingClientRect()
          return { overflow: main.scrollHeight - main.clientHeight, horizontal: document.documentElement.scrollWidth - innerWidth, footerVisible: footer.bottom <= innerHeight, clipped }
        })
        expect(layout, `${theme} ${width}×${height} ${stage}`).toEqual({ overflow: 0, horizontal: 0, footerVisible: true, clipped: [] })
      }
    }
    const colors = await page.evaluate(() => ({
      primary: getComputedStyle(document.documentElement).getPropertyValue('--el-color-primary').trim(),
      button: getComputedStyle(document.querySelector('.experience-start')!).backgroundColor,
      surface: getComputedStyle(document.querySelector('.flow-section')!).backgroundColor,
    }))
    expect(colors.primary).toBe(theme === 'dark' ? '#10b981' : '#585be0')
    expect(colors.button).toBe(theme === 'dark' ? 'rgb(16, 185, 129)' : 'rgb(88, 91, 224)')
    expect(colors.surface).toBe(theme === 'dark' ? 'rgb(16, 26, 46)' : 'rgb(255, 255, 255)')
    // Evidence for inbound must not mark an unrelated outbound stage as executed.
    await expect(page.locator('.detail-title .el-tag')).toContainText('等待前序步骤')
    await page.setViewportSize({ width: 1366, height: 650 })
    await page.locator('.flow-step').getByText('入库单', { exact: true }).click()
    await page.screenshot({ path: testInfo.outputPath(`home-${theme}.png`), animations: 'disabled' })
    await page.getByRole('button', { name: theme === 'dark' ? '切换为浅色模式' : '切换为深色模式', exact: true }).click()
    expect(await page.evaluate(() => localStorage.getItem('gowms_theme'))).toBe(theme === 'dark' ? 'light' : 'dark')
    // Finish the desktop interaction before measuring the mobile layout.
    await page.getByRole('heading', { name: 'WMS 业务闭环', exact: true }).click()
    await page.keyboard.press('Escape')
    await page.setViewportSize({ width: 320, height: 700 })
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320)
  })
}

test('demo launch modes keep full, inbound and outbound event scopes', async ({ page }) => {
  await openHome(page)
  await page.getByRole('button', { name: '开始自动演示', exact: true }).click()
  await page.locator('.mode-tabs').getByRole('button', { name: '分步执行', exact: true }).click()
  await page.getByRole('button', { name: '开始分步执行', exact: true }).click()
  await page.locator('.detail-actions').getByRole('button', { name: '自动演示', exact: true }).click()
  await page.locator('.flow-step').getByText('出库单', { exact: true }).click()
  await page.locator('.detail-actions').getByRole('button', { name: '自动演示', exact: true }).click()
  expect(await page.evaluate(() => (window as unknown as { layoutLaunches: unknown[] }).layoutLaunches)).toEqual([{ mode: 'auto', scope: 'full' }, { mode: 'step', scope: 'full' }, { mode: 'auto', scope: 'inbound' }, { mode: 'auto', scope: 'outbound' }])
  await page.locator('.flow-step').getByText('库存形成', { exact: true }).click()
  await expect(page.locator('.detail-actions').getByRole('button', { name: '引导演示', exact: true })).toHaveCount(0)
  await page.locator('.detail-actions').getByRole('button', { name: '查看操作日志', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/activity\?tab=operations$/)
  await page.goto('/demo')
  await page.locator('.experience-footer').getByRole('button', { name: '业务证据 →', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/activity$/)
})

test('home footer preserves reset, resume drawer and exit actions', async ({ page }) => {
  await openHome(page)
  const resetRequest = page.waitForRequest(r => r.method() === 'POST' && r.url().endsWith('/api/v1/demo/reset'))
  await page.locator('.home-dock').getByRole('button', { name: '一键重置数据', exact: true }).click()
  await page.getByRole('button', { name: '确定重置', exact: true }).click()
  await resetRequest
  await page.locator('.home-dock').getByRole('button', { name: '演示控制', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: '演示控制', exact: true })
  await expect(drawer.getByRole('button', { name: '业务操作记录', exact: false })).toBeVisible()
  await drawer.getByRole('button', { name: '退出演示', exact: true }).click()
  const release = page.waitForRequest(r => r.method() === 'POST' && r.url().endsWith('/api/v1/demo/session/release'))
  await page.getByRole('button', { name: '退出并重置', exact: true }).click()
  await release
  await expect(page).toHaveURL(/\/login$/)
})
