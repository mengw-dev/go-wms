import { expect, test, type Page } from '@playwright/test'

// UI fixtures only. All writes are rejected; real business flows remain in lifecycle E2E.
const date = '2026-10-20T09:30:00+08:00'
const longNo = 'RK202610201234567890123456789012345678901234567890'
const user = { user_id: '42', username: 'operator', nickname: '仓库管理员', roles: ['operator'], perms: ['*'] }
const order = { id: '1', order_no: longNo, biz_order_no: 'ERP-20261020-0001', warehouse_id: '1', status: 'APPROVED', source: 'MANUAL', expected_qty: 120, received_qty: 60, defective_qty: 0, picked_qty: 0, allocated_qty: 120, created_at: date, created_by: 'operator', remark: '核对库存与作业进度' }
const task = { id: '11', task_no: 'TASK202610200001', task_type: 'PUTAWAY', status: 'CREATED', target_qty: 60, done_qty: 0, order_no: longNo, batch_no: 'B20261020', location_code: 'A-01-01', created_at: date }
const line = { id: '21', sku_id: '1', sku_code: 'SKU-001', sku_name: '工业扫码终端', expected_qty: 120, received_qty: 60, defective_qty: 0, allocated_qty: 120, picked_qty: 0, batch_no: 'B20261020', location_code: 'A-01-01', book_qty: 120, actual_qty: 118, diff_qty: -2, adjusted: false }
const warehouse = { id: '1', code: 'WH-SH', name: '上海中心仓', status: 1, created_at: date }
const sku = { id: '1', code: 'SKU-001', name: '工业扫码终端', barcode: '690000000001', spec: '标准版', unit: '台', created_at: date }
const location = { id: '1', warehouse_id: '1', code: 'A-01-01', zone: 'A', status: 1, created_at: date }
const role = { id: '2', name: '仓库操作员', perms: 'wms:inventory,wms:task', remark: '日常仓储作业', created_at: date }
const inventory = { id: '1', warehouse_id: '1', location_id: '1', location_code: 'A-01-01', sku_id: '1', batch_no: 'B20261020', stock_quantity: 120, available_quantity: 90, allocated_quantity: 30, stock_in_time: date }

async function mockBusiness(page: Page, theme: 'light' | 'dark') {
  const state = { empty: false }
  await page.addInitScript(({ user, theme }) => {
    localStorage.setItem('WMS_TOKEN', 'business-layout-token')
    localStorage.setItem('WMS_USER', JSON.stringify(user))
    localStorage.setItem('gowms_theme', theme)
  }, { user, theme })
  await page.route('**/api/v1/**', async route => {
    expect(route.request().method()).toBe('GET')
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '')
    const lists: Record<string, unknown[]> = {
      '/basic/warehouses': [warehouse], '/basic/skus': [sku], '/basic/locations': [location],
      '/system/users': [{ id: '2', username: 'warehouse01', nickname: '仓库操作员', status: 1, role_ids: ['2'], created_at: date }],
      '/system/roles': [role], '/system/oper-logs': [{ id: '1', username: 'operator', method: 'POST', path: '/api/v1/inbound/orders', ip: '127.0.0.1', cost_ms: 23, status: 200, created_at: date }],
      '/tasks': [task], '/inventory': [inventory], '/inventory/trans': [],
      '/inventory/summary': [{ ...inventory, sku_code: sku.code, sku_name: sku.name, unit: sku.unit }],
      '/stocktake/orders': [{ ...order, status: 'DRAFT', location_id: '0' }],
      '/inbound/orders': Array.from({ length: 10 }, (_, i) => ({ ...order, id: String(i + 1), order_no: `RK20261020000${i + 1}` })),
      '/outbound/orders': Array.from({ length: 10 }, (_, i) => ({ ...order, id: String(i + 1), order_no: `CK20261020000${i + 1}` })),
    }
    let data: unknown
    if (path === '/profile') data = user
    else if (path === '/system/roles/all') data = [role]
    else if (path === '/inbound/imports') data = [{ task_id: 'batch-001', status: 'COMPLETED', success_rows: 10, total_rows: 10, created_at: date }]
    else if (/\/(inbound|outbound|stocktake)\/orders\/1$/.test(path)) data = {
      order: { ...order, status: path.includes('stocktake') ? 'DRAFT' : 'APPROVED' }, details: [line], allocations: [line],
      tasks: [{ ...task, task_type: path.includes('outbound') ? 'PICK' : 'PUTAWAY' }],
    }
    else if (path in lists) {
      const list = state.empty ? [] : lists[path]!
      data = { list, total: list.length }
    } else throw new Error(`Unexpected business layout API: ${path}`)
    await route.fulfill({ json: { code: 0, msg: 'ok', data } })
  })
  return state
}

async function contained(page: Page) {
  await expect.poll(() => page.locator('.main').evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1)
}

const lists = [
  ['/inbound/orders', '入库管理'], ['/outbound/orders', '出库管理'], ['/stocktake/orders', '盘点管理'],
  ['/tasks', '任务中心'], ['/inventory', '库存查询'], ['/basic/warehouses', '仓库管理'],
  ['/basic/locations', '库位管理'], ['/basic/skus', '货品管理'], ['/system/users', '用户管理'],
  ['/system/roles', '角色管理'], ['/system/logs', '操作日志'],
]

for (const theme of ['light', 'dark'] as const) {
  test(`${theme}: all business lists preserve filters, data, empty states and table scrolling`, async ({ page }, testInfo) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.setViewportSize({ width: 1536, height: 864 })
    const state = await mockBusiness(page, theme)
    for (const [path, title] of lists) {
      await page.goto(path!)
      await expect(page.getByRole('heading', { name: title!, exact: true })).toBeVisible()
      await expect(page.locator('.el-table__body-wrapper tbody tr').first()).toBeVisible()
      await contained(page)
      await expect(page.getByRole('button', { name: '查询', exact: true }).first()).toBeVisible()
      // Table headers begin high enough to leave the majority of the page for results.
      expect((await page.locator('.el-table').first().boundingBox())!.y).toBeLessThan(360)
      await expect(page.locator('.pagination').first()).toBeInViewport({ ratio: 1 })
      await page.screenshot({ path: testInfo.outputPath(`${path!.replaceAll('/', '-')}.png`), animations: 'disabled' })
    }
    await page.goto('/inbound/orders')
    await page.getByRole('button', { name: '更多筛选', exact: true }).click()
    await expect(page.locator('#inbound-extra-filters')).toBeVisible()
    await page.locator('#inbound-extra-filters .el-select').click()
    await page.getByRole('option', { name: /batch-001/ }).click()
    await page.getByRole('button', { name: /收起筛选/ }).click()
    // Collapsed advanced filters keep an active count; reset clears their value.
    await expect(page.locator('#inbound-extra-filters')).not.toBeVisible()
    await expect(page.getByRole('button', { name: '更多筛选 · 1', exact: true })).toHaveAttribute('aria-expanded', 'false')
    await page.getByRole('button', { name: '重置', exact: true }).click()
    await expect(page.locator('#inbound-extra-filters')).not.toBeVisible()
    state.empty = true
    for (const [path] of lists) {
      await page.goto(path!)
      await expect(page.locator('.el-table__empty-text').first()).toBeVisible()
      await contained(page)
    }
    expect(errors).toEqual([])
  })

  test(`${theme}: business forms and operating dialogs retain controls with fixed footers`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1366, height: 650 })
    await mockBusiness(page, theme)
    for (const [path, button, title] of [
      ['/basic/warehouses', '新增仓库', '新增仓库'], ['/basic/skus', '新增货品', '新增货品'],
      ['/basic/locations', '批量生成库位', '批量生成库位'], ['/system/users', '新增用户', '新增用户'],
      ['/system/roles', '新增角色', '新增角色'], ['/stocktake/orders', '新建盘点单', '新建盘点单'],
      ['/inbound/orders', '新建入库单', '新建入库单'], ['/outbound/orders', '新建出库单', '新建出库单'],
    ]) {
      await page.goto(path!)
      await page.getByRole('button', { name: button!, exact: true }).click()
      const dialog = page.getByRole('dialog', { name: title!, exact: true })
      await expect(dialog).toBeVisible()
      await expect(dialog.locator('.el-dialog__footer')).toBeInViewport()
      expect(await dialog.evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
      await dialog.getByRole('button', { name: '取消', exact: true }).click()
      await expect(dialog).not.toBeVisible()
    }
    for (const [path, action, title] of [
      ['/inbound/orders/1', '收货', '收货'], ['/inbound/orders/1', '上架', '上架'], ['/outbound/orders/1', '拣货', 'PDA 拣货'],
    ]) {
      await page.goto(path!)
      await expect(page.locator('.document-id')).toHaveText(longNo)
      await page.locator('.detail-actions').getByRole('button', { name: action!, exact: true }).click()
      const dialog = page.getByRole('dialog', { name: title!, exact: true })
      await expect(dialog.locator('.operation-context')).toContainText(longNo)
      await expect(dialog.locator('.el-dialog__footer')).toBeInViewport()
      await page.screenshot({ path: testInfo.outputPath(`${action}.png`), animations: 'disabled' })
      await dialog.getByRole('button', { name: action === '收货' ? '关闭' : '取消', exact: true }).click()
    }
  })

  test(`${theme}: narrow business pages, long identifiers and AI composer stay usable`, async ({ page }, testInfo) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.setViewportSize({ width: 390, height: 844 })
    await mockBusiness(page, theme)
    for (const [path, title] of lists) {
      await page.goto(path!)
      await expect(page.getByRole('heading', { name: title!, exact: true })).toBeVisible()
      await contained(page)
    }
    for (const kind of ['inbound', 'outbound', 'stocktake']) {
      await page.goto(`/${kind}/orders/1`)
      await expect(page.locator('.document-id')).toHaveText(longNo)
      await contained(page)
      expect(await page.locator('.business-facts').evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
      await page.screenshot({ path: testInfo.outputPath(`${kind}-detail-mobile.png`), fullPage: true, animations: 'disabled' })
      if (kind !== 'stocktake') {
        await page.locator('.detail-actions').getByRole('button', { name: kind === 'inbound' ? '上架' : '拣货', exact: true }).click()
        const dialog = page.getByRole('dialog', { name: kind === 'inbound' ? '上架' : 'PDA 拣货', exact: true })
        await expect(dialog.locator('.operation-context')).toContainText(longNo)
        await expect(dialog.locator('.el-dialog__footer')).toBeInViewport({ ratio: 1 })
        expect(await dialog.evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
        await page.screenshot({ path: testInfo.outputPath(`${kind}-operation-mobile.png`), animations: 'disabled' })
        await dialog.getByRole('button', { name: '取消', exact: true }).click()
      }
    }
    await page.goto('/ai')
    await expect(page.getByRole('heading', { name: 'AI 库存问答', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '发送', exact: true })).toBeInViewport()
    await contained(page)
    await page.screenshot({ path: testInfo.outputPath('ai-mobile.png'), animations: 'disabled' })
    expect(errors).toEqual([])
  })
}
