import { expect, test, type Locator, type Page } from '@playwright/test'

// UI-only fixtures: every API request is intercepted; no business data is written.
async function mockWorkspace(page: Page, theme: 'light' | 'dark', perms = ['*']) {
  const user = { user_id: '42', username: 'operator', nickname: '仓库管理员', roles: ['operator'], perms }
  await page.addInitScript(({ user, theme }) => {
    localStorage.setItem('WMS_TOKEN', 'ui-test-token')
    localStorage.setItem('WMS_USER', JSON.stringify(user))
    if (!localStorage.getItem('gowms_theme')) localStorage.setItem('gowms_theme', theme)
  }, { user, theme })
  await page.route('**/api/v1/**', async route => {
    expect(route.request().method()).toBe('GET')
    const url = new URL(route.request().url())
    const path = url.pathname.replace('/api/v1', '')
    let data: unknown
    if (path === '/profile') data = user
    else if (path === '/basic/warehouses') data = { list: [{ id: '1', code: 'WH-SH', name: '上海中心仓', status: 1 }], total: 1 }
    else if (path === '/basic/skus') data = { list: [{ id: '1', code: 'SKU-001', name: '工业扫码终端', unit: '台' }], total: 1 }
    else if (path === '/inbound/imports') data = []
    else if (path === '/inbound/orders' || path === '/outbound/orders') {
      const prefix = path.includes('inbound') ? 'RK' : 'CK'
      const statuses = ['DRAFT', 'SUBMITTED', 'APPROVED', 'COMPLETED']
      const list = statuses.map((status, i) => ({
        id: String(i + 1), order_no: `${prefix}20261008000${i + 1}`, biz_order_no: `ERP-20261008-${i + 1}`,
        warehouse_id: '1', status, expected_qty: 120, received_qty: i * 30,
        defective_qty: 0, allocated_qty: i * 30, picked_qty: i * 20, created_at: '2026-10-08T09:30:00+08:00',
      })).filter(row => !url.searchParams.get('status') || row.status === url.searchParams.get('status'))
        .filter(row => !url.searchParams.get('keyword') || row.order_no.includes(url.searchParams.get('keyword')!))
      data = { list, total: list.length }
    } else if (path === '/tasks') data = { list: [{
      id: '1', task_no: 'TASK202610080001', task_type: 'PICK', status: 'CREATED',
      order_no: 'CK202610080001', target_qty: 120, done_qty: 30, location_code: 'A-01-02',
    }], total: 1 }
    else if (path === '/inventory') data = { list: [{
      id: '1', warehouse_id: '1', location_id: '1', location_code: 'A-01-02', sku_id: '1',
      batch_no: 'B20261008', stock_quantity: 120, available_quantity: 90, allocated_quantity: 30,
      stock_in_time: '2026-10-08T09:30:00+08:00',
    }], total: 1 }
    else if (path === '/inventory/summary') data = { list: [{
      sku_id: '1', sku_code: 'SKU-001', sku_name: '工业扫码终端', unit: '台',
      stock_quantity: 120, available_quantity: 90, allocated_quantity: 30,
    }], total: 1 }
    else if (path === '/inventory/trans') data = { list: [], total: 0 }
    else { await route.abort(); throw new Error(`Unexpected UI API: ${path}`) }
    await route.fulfill({ json: { code: 0, msg: 'ok', data } })
  })
}

async function expectContained(page: Page) {
  await expect.poll(() => page.locator('.main').evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1)
}

async function expectReadable(locator: Locator) {
  await expect.poll(() => locator.evaluate(el => {
    const style = getComputedStyle(el)
    const luminance = (color: string) => {
      const rgb = color.match(/[\d.]+/g)!.slice(0, 3).map(Number).map(value => {
        const channel = value / 255
        return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
      })
      return rgb[0]! * 0.2126 + rgb[1]! * 0.7152 + rgb[2]! * 0.0722
    }
    const foreground = luminance(style.color)
    const background = luminance(style.backgroundColor)
    return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05)
  })).toBeGreaterThanOrEqual(4.5)
}

for (const theme of ['light', 'dark'] as const) {
  test(`${theme}: desktop workspace, filters, selection and theme persistence`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1440, height: 1000 })
    await mockWorkspace(page, theme)
    await page.goto('/dashboard')
    await expect(page.locator('.stat')).toHaveCount(6)
    await expectContained(page)
    await page.screenshot({ path: testInfo.outputPath('dashboard.png'), fullPage: true, animations: 'disabled' })
    await page.getByRole('button', { name: '收起导航' }).click()
    await expect(page.locator('.aside')).toHaveCount(0)
    await page.getByRole('button', { name: '展开导航' }).click()
    await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()

    for (const [path, title] of [['/inbound/orders', '入库管理'], ['/outbound/orders', '出库管理']]) {
      await page.goto(path!)
      await expect(page.getByRole('heading', { name: title!, exact: true })).toBeVisible()
      await expect(page.locator('.table-summary')).toContainText('4')
      const createButton = page.getByRole('button', { name: /新建.*库单/ })
      await expectReadable(createButton)
      await createButton.hover()
      await expectReadable(createButton)
      await page.mouse.move(0, 0)
      await expectReadable(page.locator('.el-table .el-tag--warning').first())
      await expect(page.getByRole('button', { name: '批量操作' })).toBeDisabled()
      await page.locator('.el-table__body-wrapper .el-checkbox').first().click()
      await expect(page.getByRole('button', { name: '批量操作' })).toBeEnabled()
      await page.getByRole('button', { name: '清除选择' }).click()
      await expect(page.getByRole('button', { name: '批量操作' })).toBeDisabled()
      await page.getByPlaceholder('单号模糊搜索').fill('0001')
      await page.getByRole('button', { name: '查询', exact: true }).click()
      await expect(page.locator('.el-table__body-wrapper tbody tr')).toHaveCount(1)
      await page.getByRole('button', { name: '重置', exact: true }).click()
      await expect(page.locator('.el-table__body-wrapper tbody tr')).toHaveCount(4)
      await expectContained(page)
      await page.screenshot({ path: testInfo.outputPath(`${path!.split('/')[1]}.png`), fullPage: true, animations: 'disabled' })
    }
    await page.goto('/inventory')
    await expect(page.getByText('工业扫码终端', { exact: true })).toBeVisible()
    await expectContained(page)
    await page.screenshot({ path: testInfo.outputPath('inventory.png'), fullPage: true, animations: 'disabled' })
    await page.getByRole('button', { name: theme === 'dark' ? '切换为浅色模式' : '切换为深色模式' }).click()
    await page.reload()
    await expect(page.locator('html')).toHaveClass(theme === 'dark' ? '' : 'dark')
  })

  test(`${theme}: mobile navigation, horizontally scrollable tables and dialogs`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockWorkspace(page, theme)
    await page.goto('/dashboard')
    await expect(page.locator('.stat')).toHaveCount(6)
    await expectContained(page)
    await page.screenshot({ path: testInfo.outputPath('dashboard-mobile.png'), fullPage: true, animations: 'disabled' })
    await page.getByRole('button', { name: '展开导航' }).click()
    const navigation = page.getByRole('dialog', { name: '导航菜单' })
    await expect(navigation).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(navigation).not.toBeVisible()
    await expect(page.getByRole('button', { name: '展开导航' })).toBeFocused()
    await page.getByRole('button', { name: '展开导航' }).click()
    await navigation.getByRole('menuitem', { name: '入库管理' }).click()
    await navigation.getByRole('menuitem', { name: '入库单', exact: true }).click()
    await expect(navigation).not.toBeVisible()
    await expect(page.locator('.table-summary')).toContainText('4')
    await expectContained(page)
    await page.getByPlaceholder('开始日期').click()
    const calendar = page.locator('.el-date-range-picker:visible')
    await expect(calendar).toBeVisible()
    const calendarBox = (await calendar.boundingBox())!
    expect(calendarBox.x).toBeGreaterThanOrEqual(0)
    expect(calendarBox.x + calendarBox.width).toBeLessThanOrEqual(390)
    await page.keyboard.press('Escape')
    await page.screenshot({ path: testInfo.outputPath('inbound-mobile.png'), fullPage: true, animations: 'disabled' })
    const scroll = page.locator('.el-table__body-wrapper .el-scrollbar__wrap')
    expect(await scroll.evaluate(el => el.scrollWidth > el.clientWidth)).toBe(true)
    await scroll.evaluate(el => { el.scrollLeft = el.scrollWidth })
    await expect(page.getByRole('button', { name: '详情', exact: true }).first()).toBeInViewport()
    await page.getByRole('button', { name: '新建入库单' }).click()
    const dialog = page.getByRole('dialog', { name: '新建入库单' })
    await expect(dialog).toBeVisible()
    expect(await dialog.evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
    await page.screenshot({ path: testInfo.outputPath('create-mobile.png'), fullPage: true, animations: 'disabled' })
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
    await page.goto('/inventory')
    await expect(page.getByRole('button', { name: '流水', exact: true })).toBeVisible()
    await expectContained(page)
    await page.getByRole('button', { name: '流水', exact: true }).click()
    const drawer = page.getByRole('dialog', { name: '库存流水' })
    await expect(drawer).toBeVisible()
    expect((await drawer.boundingBox())!.width).toBeLessThanOrEqual(390)
    await page.screenshot({ path: testInfo.outputPath('inventory-drawer-mobile.png'), fullPage: true, animations: 'disabled' })
  })
}

test('mobile navigation preserves permission boundaries', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await mockWorkspace(page, 'light', ['wms:inventory'])
  await page.goto('/inventory')
  await page.getByRole('button', { name: '展开导航' }).click()
  const navigation = page.getByRole('navigation', { name: '主导航' })
  await expect(navigation.getByText('库存管理', { exact: true })).toBeVisible()
  await expect(navigation.getByText('系统管理', { exact: true })).toHaveCount(0)
  await expect(navigation.getByText('入库管理', { exact: true })).toHaveCount(0)
})
