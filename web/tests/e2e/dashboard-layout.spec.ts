import { expect, test, type Page } from '@playwright/test'
import { dashboardFixture } from './support/dashboard'

async function setup(page: Page, theme: string, limited = false, demo = false) {
  const user = { user_id: '42', username: 'operator', nickname: '仓库管理员', roles: [], perms: demo ? ['*', 'wms:demo'] : limited ? ['wms:task'] : ['*'] }
  const state = { fail: false }
  await page.addInitScript(({ user, theme, demo }) => {
    localStorage.setItem('WMS_TOKEN', 'dashboard-test')
    localStorage.setItem('WMS_USER', JSON.stringify(user))
    localStorage.setItem('gowms_theme', theme)
    if (demo) {
      sessionStorage.setItem('WMS_TOKEN', 'dashboard-test')
      sessionStorage.setItem('WMS_USER', JSON.stringify(user))
      sessionStorage.setItem('WMS_DEMO_SESSION', 'dashboard-demo-session')
    }
  }, { user, theme, demo })
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url())
    const path = url.pathname.replace('/api/v1', '')
    if (!path.startsWith('/demo/session/')) expect(route.request().method()).toBe('GET')
    let data: unknown = { list: [], total: 0 }
    if (path === '/profile') data = user
    else if (path.startsWith('/demo/session/')) data = { session_id: 'dashboard-demo-session', expires_in: 300 }
    else if (path === '/dashboard/overview') {
      if (state.fail) { await route.fulfill({ status: 503, json: { code: 50300, msg: 'unavailable' } }); return }
      const fixture = dashboardFixture(url.searchParams.get('warehouse_id') || '0')
      data = limited ? { ...fixture, access: { ...fixture.access, inbound: false, outbound: false, inventory: false, basic: false, stocktake: false }, inbound: undefined, outbound: undefined, inventory: undefined, locations: undefined, draft_stocktakes: undefined, warehouses: [] } : fixture
    } else if (path === '/basic/warehouses') data = { list: dashboardFixture().warehouses.map(w => ({ ...w, status: 1 })), total: 2 }
    else if (path === '/inbound/imports') data = []
    await route.fulfill({ json: { code: 0, msg: 'ok', data } })
  })
  return state
}

for (const theme of ['light', 'dark']) {
  test(`${theme}: occupied location metric opens matching locations for all or selected warehouses`, async ({ page }) => {
    await setup(page, theme)
    for (const warehouseId of ['', '2']) {
      await page.goto('/dashboard')
      if (warehouseId) {
        await page.getByRole('combobox', { name: '仪表盘仓库' }).click()
        await page.getByRole('option', { name: 'WH-HZ · 杭州仓' }).click()
        await expect(page.locator('.metric').first()).toContainText('3')
      }
      const metric = page.getByRole('button', { name: /启用库位占用率/ })
      await expect(metric).toBeEnabled()
      const request = page.waitForRequest(req => new URL(req.url()).pathname.endsWith('/basic/locations'))
      await metric.click()
      const url = new URL((await request).url())
      expect(url.searchParams.get('warehouse_id')).toBe(warehouseId || null)
      expect(url.searchParams.get('status')).toBe('2')
      await expect(page.locator('.query-form .el-select').last()).toContainText('占用')
      if (warehouseId) {
        const select = page.locator('.query-form .el-select').first()
        await expect(select).toContainText('WH-HZ（杭州仓）')
        await select.hover()
        const cleared = page.waitForRequest(req => new URL(req.url()).pathname.endsWith('/basic/locations') && !new URL(req.url()).searchParams.has('warehouse_id'))
        await select.locator('.el-select__clear').click()
        expect(new URL((await cleared).url()).searchParams.get('status')).toBe('2')
      } else {
        await expect(page.locator('.query-form .el-select').first()).toContainText('全部仓库')
      }
    }
  })

  test(`${theme}: dashboard reserves the demo dock lane on smaller desktop screens`, async ({ page }) => {
    await setup(page, theme, false, true)
    await page.goto('/dashboard')
    await expect(page.locator('.metric').first()).toContainText('16')
    for (const [width, height] of [[1366, 650], [1280, 620]]) {
      await page.setViewportSize({ width, height })
      expect(await page.locator('.main').evaluate(el => el.scrollHeight - el.clientHeight), `${width}×${height}`).toBeLessThanOrEqual(1)
      const recent = (await page.locator('.recent-panel').boundingBox())!
      const dock = (await page.locator('.demo-statusbar').boundingBox())!
      expect(recent.y + recent.height).toBeLessThan(dock.y)
      const trend = (await page.locator('.trend-panel').boundingBox())!
      expect(trend.y + trend.height).toBeLessThanOrEqual(recent.y)
    }
  })

  test(`${theme}: dashboard is one screen with local task and recent scrolling`, async ({ page }, testInfo) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await setup(page, theme)
    await page.goto('/dashboard')
    await expect(page.locator('.metric').first()).toContainText('16')
    for (const [width, height] of [[1920, 914], [1536, 730], [1366, 650], [1280, 620]]) {
      await page.setViewportSize({ width, height })
      expect(await page.locator('.main').evaluate(el => el.scrollHeight - el.clientHeight), `${width}×${height}`).toBeLessThanOrEqual(1)
      for (const selector of ['.dashboard-heading', '.dashboard-metrics', '.workflow-panel', '.trend-panel', '.tasks-panel', '.inventory-panel', '.recent-panel']) await expect(page.locator(selector)).toBeInViewport({ ratio: 1 })
      expect(await page.locator('.task-list').evaluate(el => el.scrollHeight - el.clientHeight)).toBeGreaterThan(0)
      expect(await page.locator('.recent-list').evaluate(el => el.scrollHeight - el.clientHeight)).toBeGreaterThan(0)
    }
    await page.setViewportSize({ width: 1536, height: 864 })
    await page.screenshot({ path: testInfo.outputPath(`dashboard-${theme}.png`) })
    await page.getByRole('button', { name: '查看数据 ↗', exact: true }).click()
    const table = page.getByRole('dialog', { name: '近 7 日新建单量', exact: true })
    await expect(table.locator('.el-table__body-wrapper tbody tr')).toHaveCount(7)
    await table.getByRole('button', { name: '关闭', exact: true }).click()
    await expect(table).not.toBeVisible()
    await page.setViewportSize({ width: 390, height: 700 })
    await expect.poll(() => page.locator('.main').evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
    expect(errors).toEqual([])
  })
}

test('dashboard drill-downs retain warehouse, status and creation date; failed refresh retains last success', async ({ page }) => {
  const state = await setup(page, 'light')
  await page.goto('/dashboard')
  await page.getByRole('combobox', { name: '仪表盘仓库' }).click()
  await page.getByRole('option', { name: 'WH-HZ · 杭州仓' }).click()
  await expect(page.locator('.metric').first()).toContainText('3')
  state.fail = true
  await page.getByRole('button', { name: '刷新仪表盘', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('更新失败')
  await expect(page.locator('.metric').first()).toContainText('3')
  const request = page.waitForRequest(req => new URL(req.url()).pathname.endsWith('/inbound/orders'))
  await page.locator('.metric').first().click()
  const url = new URL((await request).url())
  expect(url.searchParams.get('warehouse_id')).toBe('2')
  expect(url.searchParams.get('created_at_from')).toBe('2026-10-20 00:00:00')
  expect(url.searchParams.get('created_at_to')).toBe('2026-10-20 16:30:00')
  state.fail = false
  await page.goto('/dashboard')
  await page.getByRole('combobox', { name: '仪表盘仓库' }).click()
  await page.getByRole('option', { name: 'WH-HZ · 杭州仓' }).click()
  const task = page.waitForRequest(req => new URL(req.url()).pathname.endsWith('/tasks'))
  await page.locator('.task-type-strip button').filter({ hasText: '拣货' }).click()
  const taskURL = new URL((await task).url())
  expect(taskURL.searchParams.get('warehouse_id')).toBe('2')
  expect(taskURL.searchParams.get('task_type')).toBe('PICK')
  expect(taskURL.searchParams.get('status')).toBe('CREATED')
})

test('task-only users do not see warehouse, order or inventory information', async ({ page }) => {
  await setup(page, 'dark', true)
  await page.goto('/dashboard')
  await expect(page.getByRole('heading', { name: '作业待办', exact: true })).toBeVisible()
  await expect(page.locator('.metric')).toHaveCount(1)
  await expect(page.locator('.workflow-panel, .trend-panel, .inventory-panel')).toHaveCount(0)
  await expect(page.getByRole('combobox', { name: '仪表盘仓库' })).toHaveCount(0)
})

test('location browsing includes disabled warehouses but batch creation only offers enabled warehouses', async ({ page }) => {
  await setup(page, 'light')
  await page.route('**/api/v1/basic/warehouses*', route => route.fulfill({ json: { code: 0, msg: 'ok', data: {
    list: [{ id: '1', code: 'WH-ON', name: '启用仓', status: 1 }, { id: '2', code: 'WH-OFF', name: '停用仓', status: 0 }], total: 2,
  } } }))
  await page.goto('/basic/locations?warehouse_id=2&status=0')
  await expect(page.locator('.query-form .el-select').first()).toContainText('WH-OFF（停用仓）')
  await expect(page.locator('.query-form .el-select').last()).toContainText('禁用')
  await page.getByRole('button', { name: '批量生成库位', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '批量生成库位', exact: true })
  await expect(dialog.getByRole('combobox')).toHaveValue('')
  await dialog.locator('.el-select').click()
  await expect(page.getByRole('option', { name: 'WH-ON（启用仓）', exact: true })).toBeVisible()
  await expect(page.getByRole('option', { name: 'WH-OFF（停用仓）', exact: true })).toHaveCount(0)
})
