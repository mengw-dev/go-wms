import { expect, test, type Page } from '@playwright/test'

// UI fixtures only. Business writes are rejected; real flows remain in lifecycle E2E.
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

async function mockBusiness(page: Page, theme: 'light' | 'dark', guided = false, demo = false) {
  const state = { empty: false }
  const profile = guided || demo ? { ...user, perms: ['*', 'wms:demo'] } : user
  await page.addInitScript(({ user, theme, guided, demo }) => {
    localStorage.setItem('WMS_TOKEN', 'business-layout-token')
    localStorage.setItem('WMS_USER', JSON.stringify(user))
    localStorage.setItem('gowms_theme', theme)
    if (guided || demo) {
      sessionStorage.setItem('WMS_TOKEN', 'business-layout-token')
      sessionStorage.setItem('WMS_USER', JSON.stringify(user))
      sessionStorage.setItem('WMS_DEMO_SESSION', 'business-guide-session')
      if (guided) sessionStorage.setItem('wms-manual-guide-v1', JSON.stringify({ active: true, scenario: 'inbound', currentStep: 0, completed: false, startedAt: Date.now() }))
    }
  }, { user: profile, theme, guided, demo })
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '')
    if (!path.startsWith('/demo/session/')) expect(route.request().method()).toBe('GET')
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
    if (path === '/profile') data = profile
    else if (path.startsWith('/demo/session/')) data = { session_id: 'business-guide-session', expires_in: 300 }
    else if (path === '/demo/activity') data = { operations: [], inbound_orders: [], outbound_orders: [], stocktake_orders: [], tasks: [], inventory_trans: [] }
    else if (path === '/system/roles/all') data = [role]
    else if (path === '/inbound/imports') data = [{ task_id: 'batch-001', status: 'COMPLETED', success_rows: 10, total_rows: 10, created_at: date }]
    else if (path === '/inventory/alerts') data = { list: [{ id: '1', warehouse_id: '1', warehouse_code: warehouse.code, warehouse_name: warehouse.name, sku_id: '1', sku_code: sku.code, sku_name: sku.name, unit: sku.unit, available_qty: 9, stock_quantity: 12, allocated_qty: 3, min_available_qty: 10, gap_qty: 1, enabled: true, state: 'LOW', updated_at: date }], total: 1, warehouses: [warehouse], summary: { configured: 1, low: 1, out_of_stock: 0 }, checked_at: date }
    else if (path === '/inbound/orders/1/putaway-locations') data = { list: [location], total: 1 }
    else if (path === '/inventory/alert-options') data = { list: [], total: 0 }
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

for (const theme of ['light', 'dark'] as const) {
  test(`${theme}: demo evidence tables scroll locally with pagination outside results`, async ({ page }) => {
    await page.setViewportSize({ width: 1366, height: 650 })
    await mockBusiness(page, theme, false, true)
    await page.route('**/api/v1/demo/activity*', route => route.fulfill({ json: { code: 0, msg: 'ok', data: {
      inbound_orders: Array.from({ length: 25 }, (_, i) => ({ ...order, id: String(i + 1), order_no: `RK20261020${i}` })),
      outbound_orders: [], stocktake_orders: [], tasks: [], inventory_trans: [], operations: [],
    } } }))
    await page.goto('/demo/activity')
    await expect(page.locator('.evidence-tabs .el-table__body-wrapper tbody tr').first()).toBeVisible()
    expect(await page.locator('.main').evaluate(el => el.scrollHeight - el.clientHeight)).toBeLessThanOrEqual(1)
    await expect(page.locator('.evidence-tabs .el-pagination:visible')).toBeInViewport({ ratio: 1 })
    const scroll = page.locator('.evidence-tabs .el-tab-pane:visible .el-scrollbar__wrap')
    expect(await scroll.evaluate(el => el.scrollHeight - el.clientHeight)).toBeGreaterThan(10)
  })

  test(`${theme}: dock dragging, pagination clearance and compact inventory filters`, async ({ page }) => {
    await page.setViewportSize({ width: 1366, height: 650 })
    await mockBusiness(page, theme, false, true)
    await page.goto('/inventory')
    const dock = page.locator('.demo-statusbar')
    const pagination = page.locator('.pagination:visible')
    await expect(dock).toBeVisible()
    await expect(pagination).toBeInViewport({ ratio: 1 })
    expect((await pagination.boundingBox())!.y + (await pagination.boundingBox())!.height).toBeLessThan((await dock.boundingBox())!.y)
    const toggle = page.locator('.stock-toggle-filter .el-switch')
    const search = page.getByRole('button', { name: '查询', exact: true }).first()
    expect((await search.boundingBox())!.x - ((await toggle.boundingBox())!.x + (await toggle.boundingBox())!.width)).toBeLessThan(40)
    const handle = page.getByRole('button', { name: '拖动演示控制条（方向键可移动）', exact: true })
    const before = (await dock.boundingBox())!
    await handle.hover()
    await page.mouse.down()
    await page.mouse.move(before.x - 180, before.y - 180, { steps: 8 })
    await page.mouse.up()
    const moved = (await dock.boundingBox())!
    expect(moved.y).toBeLessThan(before.y - 100)
    await page.reload()
    await expect(dock).toBeVisible()
    expect(Math.abs((await dock.boundingBox())!.y - moved.y)).toBeLessThan(2)
    await handle.focus()
    await page.keyboard.press('ArrowUp')
    expect((await dock.boundingBox())!.y).toBeLessThan(moved.y - 10)
    await page.setViewportSize({ width: 700, height: 550 })
    await expect(dock).toBeInViewport({ ratio: 1 })
    await dock.getByRole('button', { name: '演示控制', exact: true }).click()
    const drawer = page.getByRole('dialog', { name: '演示控制', exact: true })
    await drawer.getByRole('button', { name: '恢复控制条位置', exact: true }).click()
    expect(await page.evaluate(() => localStorage.getItem('WMS_DEMO_DOCK_POSITION_V1'))).toBeNull()
  })

  test(`${theme}: list rows scroll locally while filters and pagination stay visible`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 620 })
    await mockBusiness(page, theme)
    for (const path of ['/inbound/orders', '/outbound/orders']) {
      await page.goto(path)
      await expect(page.locator('.el-table__body-wrapper tbody tr').first()).toBeVisible()
      expect(await page.locator('.main').evaluate(el => el.scrollHeight - el.clientHeight)).toBeLessThanOrEqual(1)
      await expect(page.locator('.pagination')).toBeInViewport({ ratio: 1 })
      const scroll = page.locator('.el-table__body-wrapper .el-scrollbar__wrap')
      expect(await scroll.evaluate(el => el.scrollHeight - el.clientHeight)).toBeGreaterThan(30)
      await scroll.evaluate(el => { el.scrollTop = el.scrollHeight })
      await expect(page.getByRole('button', { name: '查询', exact: true }).first()).toBeInViewport()
    }
  })
}

const lists = [
  ['/inbound/orders', '入库管理'], ['/outbound/orders', '出库管理'], ['/stocktake/orders', '盘点管理'],
  ['/tasks', '任务中心'], ['/inventory', '库存查询'], ['/basic/warehouses', '仓库管理'],
  ['/basic/locations', '库位管理'], ['/basic/skus', '货品管理'], ['/system/users', '用户管理'],
  ['/system/roles', '角色管理'], ['/system/logs', '操作日志'],
]

for (const theme of ['light', 'dark'] as const) {
  for (const width of [1366, 390]) {
    test(`${theme}: ${width}px all result tables support taller areas, scrolling, persistence and reset`, async ({ page }) => {
      await page.setViewportSize({ width, height: 800 })
      await mockBusiness(page, theme)
      for (const [path] of [...lists, ['/inventory/alerts']]) {
        await page.goto(path!)
        const area = page.locator('.business-table-area:visible')
        await expect(area).toHaveCount(1)
        await expect(area.locator('.el-table__body-wrapper tbody tr').first()).toBeVisible()
        const before = (await area.boundingBox())!.height
        const bodyBefore = await area.locator('.el-table__body-wrapper').evaluate(el => el.clientHeight)
        await area.getByRole('button', { name: '一屏高度', exact: true }).click()
        await expect(area).toHaveClass(/is-resized/)
        await expect.poll(async () => (await area.boundingBox())!.height).toBe(700)
        expect(await area.locator('.el-table__body-wrapper').evaluate(el => el.clientHeight), path).toBeGreaterThan(bodyBefore + 25)
        expect(await page.locator('.main').evaluate(el => el.scrollHeight - el.clientHeight), path).toBeGreaterThan(20)
        await page.reload()
        await expect(page.locator('.business-table-area:visible')).toHaveClass(/is-resized/)
        await expect.poll(async () => (await area.boundingBox())!.height).toBe(700)
        await page.locator('.pagination:visible').scrollIntoViewIfNeeded()
        await expect(page.locator('.pagination:visible')).toBeInViewport({ ratio: 1 })
        await area.getByRole('button', { name: '恢复默认', exact: true }).click()
        await expect(area).not.toHaveClass(/is-resized/)
        await expect.poll(() => page.locator('.main').evaluate(el => el.scrollHeight-el.clientHeight), path).toBeLessThanOrEqual(1)
        expect(Math.abs((await area.boundingBox())!.height-before), path).toBeLessThanOrEqual(2)
        await contained(page)
      }
    })
  }

  test(`${theme}: drag, keyboard and cancellation preserve layout and do not issue business writes`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1366, height: 800 })
    await mockBusiness(page, theme)
    await page.route('**/api/v1/inbound/orders?*', route => route.fulfill({ json: { code: 0, msg: 'ok', data: { list: Array.from({ length: 30 }, (_, i) => ({ ...order, id: String(i+1) })), total: 30 } } }))
    await page.goto('/inbound/orders')
    const area = page.locator('.business-table-area'), handle = area.getByRole('separator', { name: '拖动调整表格高度' })
    await expect(handle).toBeVisible()
    const before = (await area.boundingBox())!.height, box = (await handle.boundingBox())!
    await page.mouse.move(box.x+30, box.y+10)
    await page.mouse.down()
    await page.mouse.move(box.x+30, box.y+100, { steps: 8 })
    await page.mouse.up()
    await expect(area).toHaveClass(/is-resized/)
    expect((await area.boundingBox())!.height).toBeGreaterThan(before+60)
    await expect(page.locator('body')).not.toHaveClass(/table-height-dragging/)
    await handle.focus()
    await handle.press('Home')
    await expect(area).not.toHaveClass(/is-resized/)
    await handle.press('ArrowDown')
    expect((await area.boundingBox())!.height).toBeGreaterThan(before+25)
    await handle.press('End')
    await expect.poll(async () => (await area.boundingBox())!.height).toBe(700)
    expect(await area.locator('.el-table__body-wrapper .el-scrollbar__wrap').evaluate(el => el.clientHeight)).toBeGreaterThan(550)
    await page.screenshot({ path: testInfo.outputPath(`resized-${theme}.png`), animations: 'disabled' })
    await handle.scrollIntoViewIfNeeded()
    const next = (await handle.boundingBox())!
    await page.mouse.move(next.x+20,next.y+10); await page.mouse.down()
    await page.mouse.move(next.x+20,next.y-60,{steps:4})
    await handle.press('Escape')
    await page.mouse.up()
    await expect.poll(async () => (await area.boundingBox())!.height).toBe(700)
    await expect(page.locator('body')).not.toHaveClass(/table-height-dragging/)
    await area.getByRole('button', { name: '恢复默认', exact: true }).click()
    await expect(area).not.toHaveClass(/is-resized/)
  })
}

test('inventory tab heights and phone/desktop preferences remain independent; navigation cleans active drag', async ({ page }) => {
  await mockBusiness(page, 'dark')
  await page.setViewportSize({ width: 1366, height: 800 })
  await page.goto('/inventory')
  const detail = page.locator('[data-height-key="inventory-detail"]')
  await detail.getByRole('button', { name: '一屏高度', exact: true }).click()
  await page.getByRole('tab', { name: '按SKU汇总', exact: true }).click()
  const summary = page.locator('[data-height-key="inventory-summary"]')
  await expect(summary).not.toHaveClass(/is-resized/)
  await expect.poll(() => page.locator('.main').evaluate(el => el.scrollHeight-el.clientHeight)).toBeLessThanOrEqual(1)
  await summary.getByRole('button', { name: '加高', exact: true }).click()
  const summaryHeight = (await summary.boundingBox())!.height
  await page.getByRole('tab', { name: '库存明细', exact: true }).click()
  await expect.poll(async () => (await detail.boundingBox())!.height).toBe(700)
  await page.setViewportSize({ width: 390, height: 800 })
  await expect(detail).not.toHaveClass(/is-resized/)
  await detail.getByRole('button', { name: '加高', exact: true }).click()
  await page.setViewportSize({ width: 1366, height: 800 })
  await expect.poll(async () => (await detail.boundingBox())!.height).toBe(700)
  await page.setViewportSize({ width: 1366, height: 300 })
  await expect.poll(async () => (await detail.boundingBox())!.height).toBe(600)
  await page.setViewportSize({ width: 1366, height: 800 })
  await expect.poll(async () => (await detail.boundingBox())!.height).toBe(700)
  await page.getByRole('tab', { name: '按SKU汇总', exact: true }).click()
  await expect.poll(async () => (await summary.boundingBox())!.height).toBe(summaryHeight)
  const handle = summary.getByRole('separator')
  await handle.scrollIntoViewIfNeeded(); const box = (await handle.boundingBox())!
  await page.mouse.move(box.x+10,box.y+10); await page.mouse.down()
  await expect(page.locator('body')).toHaveClass(/table-height-dragging/)
  await page.goto('/tasks'); await page.mouse.up()
  await expect(page.locator('body')).not.toHaveClass(/table-height-dragging/)
  await expect(page.locator('.business-table-area')).not.toHaveClass(/is-resized/)
})

for (const theme of ['light', 'dark'] as const) {
  for (const width of [320, 390, 430, 768]) {
    test(`${theme}: ${width}px business records show fields, actions and aligned pagination without horizontal scrolling`, async ({ page }, testInfo) => {
      await page.setViewportSize({ width, height: 844 })
      await mockBusiness(page, theme, false, true)
      for (const [path, title] of lists) {
        await page.goto(path!)
        await expect(page.getByRole('heading', { name: title!, exact: true })).toBeVisible()
        const table = page.locator('.mobile-record-table:visible').first()
        const row = table.locator('.el-table__body-wrapper tbody tr').first()
        await expect(row).toBeVisible()
        await contained(page)
        expect(await table.locator('.el-scrollbar__wrap').evaluate(el => el.scrollWidth - el.clientWidth), path).toBeLessThanOrEqual(1)
        expect(await page.locator('.main').evaluate(el => el.scrollHeight - el.clientHeight), path).toBeLessThanOrEqual(1)
        const cells = await row.locator('td:not(.el-table-column--selection)').evaluateAll(els => els.map(el => ({
          width: el.getBoundingClientRect().width, label: getComputedStyle(el, '::before').content,
        })))
        expect(cells.length).toBeGreaterThan(2)
        for (const cell of cells) {
          expect(cell.width).toBeLessThan(width - 16)
          expect(cell.label).not.toBe('none')
          expect(cell.label).not.toBe('""')
        }
        const pagination = page.locator('.business-pagination:visible').first()
        await expect(pagination).toBeInViewport({ ratio: 1 })
        const prev = (await pagination.locator('.btn-prev').boundingBox())!
        const next = (await pagination.locator('.btn-next').boundingBox())!
        expect(Math.abs(prev.y - next.y)).toBeLessThan(1)
        const dock = (await page.locator('.demo-statusbar').boundingBox())!
        const pager = (await pagination.boundingBox())!
        expect(pager.y + pager.height).toBeLessThan(dock.y)
        expect(dock.height).toBeLessThan(width <= 520 ? 60 : 100)
        await page.getByRole('button', { name: '搜索与筛选', exact: false }).first().click()
        await expect(page.getByRole('button', { name: '查询', exact: true }).first()).toBeVisible()
        await page.getByRole('button', { name: '搜索与筛选', exact: false }).first().click()
        if (width === 390 && ['/inbound/orders', '/inventory', '/system/users'].includes(path!)) {
          await page.screenshot({ path: testInfo.outputPath(`${path!.replaceAll('/', '-')}-${theme}.png`) })
        }
      }
    })
  }

  test(`${theme}: phone selection, paging, detail and receiving reuse the existing controls`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockBusiness(page, theme, false, true)
    await page.route('**/api/v1/inbound/orders?*', route => route.fulfill({ json: { code: 0, msg: 'ok', data: {
      list: Array.from({ length: 10 }, (_, i) => ({ ...order, id: String(i + 1) })), total: 35,
    } } }))
    await page.goto('/inbound/orders')
    await page.locator('.el-table__header-wrapper .el-checkbox').click()
    await expect(page.getByRole('button', { name: '批量操作', exact: true })).toBeEnabled()
    await expect(page.locator('.selected-hint')).toContainText('10')
    await page.getByRole('button', { name: '清除选择', exact: true }).click()
    await expect(page.locator('.selected-hint')).toHaveClass(/is-hidden/)
    const request = page.waitForRequest(req => new URL(req.url()).pathname.endsWith('/inbound/orders') && new URL(req.url()).searchParams.get('page') === '2')
    await page.locator('.business-pagination .btn-next').click()
    await request
    const details = page.locator('.el-table__body-wrapper tbody tr').first().getByRole('button', { name: '详情', exact: true })
    await details.scrollIntoViewIfNeeded()
    await details.click()
    await expect(page.locator('.document-id')).toHaveText(longNo)
    await page.locator('.detail-actions').getByRole('button', { name: '收货', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '收货', exact: true })
    await expect(dialog.locator('.el-dialog__footer')).toBeInViewport({ ratio: 1 })
    const receive = dialog.getByRole('button', { name: '收货', exact: true })
    await receive.scrollIntoViewIfNeeded()
    // Browser scrolling can leave a fractional CSS pixel at the scroll boundary.
    await expect(receive).toBeInViewport({ ratio: 0.98 })
    await receive.click({ trial: true })
    expect(await dialog.locator('.el-table .el-scrollbar__wrap').evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
    await dialog.getByRole('button', { name: '关闭', exact: true }).click()
    await expect(dialog).not.toBeVisible()
  })

  test(`${theme}: phone create forms and date filters stay within the viewport`, async ({ page }) => {
    await mockBusiness(page, theme)
    for (const width of [320, 390]) {
      await page.setViewportSize({ width, height: 740 })
      for (const [path, title] of [
        ['/basic/warehouses', '新增仓库'], ['/basic/skus', '新增货品'], ['/basic/locations', '批量生成库位'],
        ['/system/users', '新增用户'], ['/system/roles', '新增角色'], ['/stocktake/orders', '新建盘点单'],
        ['/inbound/orders', '新建入库单'], ['/outbound/orders', '新建出库单'],
      ]) {
        await page.goto(path!)
        await page.getByRole('button', { name: title!, exact: true }).click()
        const dialog = page.getByRole('dialog', { name: title!, exact: true })
        await expect(dialog.locator('.el-dialog__footer')).toBeInViewport({ ratio: 1 })
        expect(await dialog.evaluate(el => el.scrollWidth - el.clientWidth), title).toBeLessThanOrEqual(1)
        expect(await dialog.locator('.el-dialog__body').evaluate(el => el.scrollWidth - el.clientWidth), title).toBeLessThanOrEqual(1)
        await dialog.getByRole('button', { name: '取消', exact: true }).click()
      }
    }
    await page.goto('/inbound/orders')
    await page.getByRole('button', { name: '搜索与筛选', exact: false }).click()
    await page.getByPlaceholder('开始日期', { exact: true }).click()
    const picker = page.locator('.order-date-picker:visible')
    await expect(picker).toBeVisible()
    expect(await picker.evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
    const box = (await picker.boundingBox())!
    expect(box.x).toBeGreaterThanOrEqual(0)
    expect(box.x + box.width).toBeLessThanOrEqual(391)
  })

  test(`${theme}: guide status strip leaves business actions clickable`, async ({ page }) => {
    await page.setViewportSize({ width: 1366, height: 650 })
    await mockBusiness(page, theme, true)
    await page.goto('/inbound/orders')
    const strip = page.getByRole('region', { name: '手动演示状态' })
    await expect(strip).toBeVisible()
    await page.getByRole('button', { name: '业务操作记录', exact: true }).click()
    const records = page.getByRole('dialog', { name: '本次业务操作记录' })
    await records.getByRole('button', { name: '关闭此对话框' }).click()
    await expect(page.locator('.guide-bubble')).toBeVisible()
    const create = page.locator('[data-tour="inbound-create"]')
    const stripBox = (await strip.boundingBox())!
    expect((await create.boundingBox())!.y).toBeGreaterThan(stripBox.y + stripBox.height)
    await create.click()
    await expect(page.getByRole('dialog', { name: '新建入库单', exact: true })).toBeVisible()
  })

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
