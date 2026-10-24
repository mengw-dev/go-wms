import { expect, test, type Page } from '@playwright/test'

const time = '2026-10-24T08:00:00+08:00'
const warehouses = [{ id: '1', code: 'WH01', name: '审查一号仓', status: 1 }, { id: '2', code: 'WH02', name: '审查二号仓', status: 1 }]
async function navigate(page: Page, path: string) {
  await page.evaluate(async target => {
    const app = (document.querySelector('#app') as HTMLElement & { __vue_app__: { config: { globalProperties: { $router: { push(path: string): Promise<unknown> } } } } }).__vue_app__
    await app.config.globalProperties.$router.push(target)
  }, path)
}
async function fixture(page: Page, settings: { readonly?: boolean; basicFailure?: boolean } = {}) {
  const user = { user_id: '42', username: 'auditor', nickname: '验收用户', roles: [], perms: settings.readonly ? ['wms:inbound:view', 'wms:outbound:view'] : ['*'] }
  const state = { writes: [] as string[], bodies: [] as unknown[], basicReads: 0 }
  await page.addInitScript(user => { localStorage.setItem('WMS_TOKEN', 'hidden-audit'); localStorage.setItem('WMS_USER', JSON.stringify(user)) }, user)
  await page.route('**/api/v1/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname.replace('/api/v1', '')
    if (req.method() !== 'GET') { state.writes.push(`${req.method()} ${path}`); state.bodies.push(req.postData() ? req.postDataJSON() : undefined); await route.fulfill({ json: { code: 0, data: { id: '1' } } }); return }
    if (path.startsWith('/basic/')) {
      state.basicReads++
      if (settings.readonly || settings.basicFailure) { await route.fulfill({ status: settings.readonly ? 403 : 503, json: { code: 10003, msg: 'fixture options unavailable' } }); return }
    }
    let data: unknown
    if (path === '/profile') data = user
    else if (path === '/basic/warehouses') data = { list: warehouses, total: 2 }
    else if (path === '/basic/skus') data = { list: [{ id: '1', code: 'SKU-AUDIT', name: '审查货品', unit: '件', status: 1, created_at: time }], total: 1 }
    else if (path === '/basic/locations') { const id = url.searchParams.get('warehouse_id') || '1'; data = { list: [{ id: `${id}1`, warehouse_id: id, code: `BIN-${id}`, status: 1 }], total: 1 } }
    else if (path === '/inbound/imports') data = []
    else if (path === '/outbound/exceptions' || path === '/outbound/returns') data = { list: [], total: 0 }
    else if (/^\/(inbound|outbound|stocktake)\/orders(?:\/1)?$/.test(path)) {
      const kind = path.split('/')[1]!
      const order = { id: '1', order_no: `AUDIT-${kind}`, biz_order_no: 'audit', warehouse_id: '1', status: 'DRAFT', expected_qty: 3, received_qty: 0, defective_qty: 0, allocated_qty: 0, picked_qty: 0, created_at: time, updated_at: time }
      data = path.endsWith('/1') ? { order, details: [], allocations: [], tasks: [] } : { list: [order], total: 1 }
    } else if (path === '/system/roles') data = { list: [{ id: '2', name: '审核角色', perms: 'wms:inventory', remark: '', created_at: time }], total: 1 }
    else throw new Error(`Unexpected hidden audit read ${path}`)
    await route.fulfill({ json: { code: 0, data } })
  })
  return state
}

for (const kind of ['inbound', 'outbound']) {
  for (const mode of ['readonly', 'options-error'] as const) {
    test(`${kind} list: ${mode} does not hide accessible orders`, async ({ page }) => {
      const state = await fixture(page, { readonly: mode === 'readonly', basicFailure: mode === 'options-error' })
      await page.goto(`/${kind}/orders`)
      await expect(page.getByRole('button', { name: '详情', exact: true })).toBeVisible({ timeout: 2500 })
      await expect(page.locator('.mobile-record-table')).toContainText(`AUDIT-${kind}`)
      if (mode === 'readonly') {
        expect(state.basicReads).toBe(0)
        await page.locator('.el-table__body-wrapper .el-checkbox').first().click()
        await page.getByRole('button', { name: '批量操作', exact: true }).click()
        for (const name of ['批量删除', '批量提交', '批量审核', '批量作废']) await expect(page.getByRole('menuitem', { name, exact: true })).toHaveAttribute('aria-disabled', 'true', { timeout: 1500 })
      }
      expect(state.writes).toHaveLength(0)
    })
  }
}

for (const kind of ['inbound', 'outbound', 'stocktake', 'sku', 'role']) {
  test(`${kind}: a confirmation cannot write after leaving its page`, async ({ page }) => {
    const state = await fixture(page)
    const path = kind === 'sku' ? '/basic/skus' : kind === 'role' ? '/system/roles' : `/${kind}/orders/1`
    await page.goto(path)
    const button = kind === 'sku' || kind === 'role' ? '删除' : kind === 'stocktake' ? '取消' : '提交'
    await page.getByRole('button', { name: button, exact: true }).first().click()
    const box = page.locator('.el-message-box')
    await expect(box).toBeVisible()
    await navigate(page, '/outbound/returns')
    await expect(page.getByRole('heading', { name: '客户退货记录', exact: true })).toBeVisible()
    // The old implementation leaves its global confirmation above the new page.
    // Confirming that stale prompt must never mutate the abandoned object.
    const stale = await box.waitFor({ state: 'hidden', timeout: 1200 }).then(() => false, () => true)
    if (stale) await box.locator('.el-message-box__btns button').last().click()
    await expect.poll(() => state.writes.length, { timeout: 1000 }).toBe(0)
    await expect(box).not.toBeVisible()
  })

  test(`${kind}: the original confirmation still executes exactly once on its own page`, async ({ page }) => {
    const state = await fixture(page)
    await page.goto(kind === 'sku' ? '/basic/skus' : kind === 'role' ? '/system/roles' : `/${kind}/orders/1`)
    await page.getByRole('button', { name: kind === 'sku' || kind === 'role' ? '删除' : kind === 'stocktake' ? '取消' : '提交', exact: true }).first().click()
    await page.locator('.el-message-box__btns button').last().click()
    const target = kind === 'sku' ? 'DELETE /basic/skus/1' : kind === 'role' ? 'DELETE /system/roles/2' : `POST /${kind}/orders/1/${kind === 'stocktake' ? 'cancel' : 'submit'}`
    await expect.poll(() => state.writes).toEqual([target])
  })
}

async function selectWarehouse(page: Page, id: string) {
  const dialog = page.getByRole('dialog', { name: '新建盘点单', exact: true })
  await dialog.locator('.el-select').first().click()
  await page.getByRole('option', { name: `WH0${id}（审查${id === '1' ? '一' : '二'}号仓）`, exact: true }).click()
  return dialog
}

test('stocktake: delayed old warehouse locations cannot replace the current warehouse', async ({ page }) => {
  await fixture(page)
  let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/basic/locations?**', async route => {
    const id = new URL(route.request().url()).searchParams.get('warehouse_id')!
    if (id === '1') await gate
    await route.fulfill({ json: { code: 0, data: { list: [{ id: `${id}1`, warehouse_id: id, code: `BIN-${id}` }], total: 1 } } })
  })
  try {
    await page.goto('/stocktake/orders')
    await page.getByRole('button', { name: '新建盘点单', exact: true }).click()
    await selectWarehouse(page, '1')
    const second = page.waitForResponse(r => r.url().includes('/basic/locations?') && new URL(r.url()).searchParams.get('warehouse_id') === '2')
    const dialog = await selectWarehouse(page, '2')
    await second
    const first = page.waitForResponse(r => r.url().includes('/basic/locations?') && new URL(r.url()).searchParams.get('warehouse_id') === '1')
    release(); await first
    await dialog.locator('.el-select').nth(1).click()
    await expect(page.getByRole('option', { name: 'BIN-2', exact: true })).toBeVisible({ timeout: 2500 })
    await expect(page.getByRole('option', { name: 'BIN-1', exact: true })).toHaveCount(0)
  } finally { release() }
})

test('stocktake: unresolved location scope cannot silently submit as the whole warehouse', async ({ page }) => {
  const state = await fixture(page)
  let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/basic/locations?**', async route => { await gate; await route.fulfill({ json: { code: 0, data: { list: [], total: 0 } } }) })
  try {
    await page.goto('/stocktake/orders')
    await page.getByRole('button', { name: '新建盘点单', exact: true }).click()
    const dialog = await selectWarehouse(page, '1')
    await expect(dialog.getByRole('button', { name: '创建', exact: true })).toBeDisabled({ timeout: 2000 })
    expect(state.writes).toHaveLength(0)
  } finally { release() }
})

test('inventory: a failed second ledger never shows the first inventory history', async ({ page }) => {
  await fixture(page)
  await page.route('**/api/v1/inventory?**', route => route.fulfill({ json: { code: 0, data: { list: [1, 2].map(id => ({ id: String(id), warehouse_id: '1', location_id: String(id), location_code: `BIN-${id}`, sku_id: '1', batch_no: `B-${id}`, stock_quantity: 3, available_quantity: 3, allocated_quantity: 0, frozen_quantity: 0, stock_in_time: time })), total: 2 } } }))
  await page.route('**/api/v1/inventory/trans?**', async route => {
    const id = new URL(route.request().url()).searchParams.get('inventory_id')
    if (id === '2') { await route.fulfill({ status: 503, json: { code: 50000, msg: 'ledger temporarily unavailable' } }); return }
    await route.fulfill({ json: { code: 0, data: { list: [{ id: '11', trans_type: 'OPENING', quantity_change: 3, before_quantity: 0, after_quantity: 3, available_before: 0, available_after: 3, order_no: 'LEDGER-FIRST-ONLY', created_at: time }], total: 1 } } })
  })
  await page.goto('/inventory')
  await page.getByRole('button', { name: '流水', exact: true }).nth(0).click()
  const drawer = page.getByRole('dialog', { name: '库存流水', exact: true })
  await expect(drawer).toContainText('LEDGER-FIRST-ONLY')
  await drawer.locator('.el-drawer__close-btn').click()
  await expect(drawer).toBeHidden()
  await page.getByRole('button', { name: '流水', exact: true }).nth(1).click()
  await expect(drawer).not.toContainText('LEDGER-FIRST-ONLY', { timeout: 2000 })
  await expect(drawer).toContainText('库存流水加载失败')
})

test('wave: an expired lease disables work without an extra business request', async ({ page }) => {
  await page.clock.install({ time: new Date(time) })
  const state = await fixture(page)
  await page.route('**/api/v1/outbound/waves/1', route => route.fulfill({ json: { code: 0, data: {
    wave: { id: '1', wave_no: 'WAVE-AUDIT', status: 'PICKING', claimed_by: 'auditor', lease_expire_at: '2026-10-24T08:01:00+08:00', created_at: time },
    lines: [{ id: '1', sku_code: 'SKU-AUDIT', sku_name: '审查货品', location_code: 'BIN-1', batch_no: 'B-1', target_qty: 3, picked_qty: 0 }], orders: [], items: [],
  } } }))
  await page.goto('/outbound/waves/1')
  const pick = page.getByRole('button', { name: '拣货', exact: true })
  await expect(pick).toBeEnabled()
  await page.clock.fastForward(61_000)
  await expect(pick).toBeDisabled({ timeout: 2000 })
  await expect(page.getByRole('button', { name: '领取波次', exact: true })).toBeVisible()
  expect(state.writes).toHaveLength(0)
})

test('PDA: expiry returns to claim controls and never automatically claims again', async ({ page }) => {
  await page.clock.install({ time: new Date(time) })
  await fixture(page)
  await page.route('**/api/v1/outbound/orders/1', route => route.fulfill({ json: { code: 0, data: {
    order: { id: '1', order_no: 'PDA-AUDIT', warehouse_id: '1', status: 'PICKING', expected_qty: 3, allocated_qty: 3, picked_qty: 0, created_at: time }, details: [], allocations: [],
    tasks: [{ id: '11', task_no: 'TASK-AUDIT', task_type: 'PICK', status: 'CREATED', sku_id: '1', warehouse_id: '1', location_code: 'BIN-1', batch_no: 'B-1', target_qty: 3, done_qty: 0 }],
  } } }))
  let claims = 0
  await page.route('**/api/v1/pda/tasks/11/claim', async route => {
    claims++
    await route.fulfill({ json: { code: 0, data: { claim_token: 'audit-lease', lease_expire_at: '2026-10-24T08:01:00+08:00', task_status: 'IN_PROGRESS', done_qty: 0, remaining_qty: 3, order_status: 'PICKING' } } })
  })
  await page.goto('/outbound/orders/1')
  await page.locator('.detail-actions').getByRole('button', { name: '拣货', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'PDA 拣货', exact: true })
  await dialog.getByRole('button', { name: '领取任务', exact: true }).click()
  await expect(dialog.getByText('已领取', { exact: true })).toBeVisible()
  await page.clock.fastForward(61_000)
  await expect(dialog.getByRole('button', { name: '领取任务', exact: true })).toBeVisible()
  await expect(dialog.getByRole('button', { name: '确定拣货', exact: true })).toBeDisabled()
  expect(claims).toBe(1)
})

test('mobile dark stocktake: failed scope requires retry, then submits the selected bin', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const state = await fixture(page)
  await page.addInitScript(() => localStorage.setItem('gowms_theme', 'dark'))
  let fail = true
  await page.route('**/api/v1/basic/locations?**', async route => {
    if (fail) { await route.fulfill({ status: 503, json: { code: 50000, msg: 'temporarily unavailable' } }); return }
    await route.fulfill({ json: { code: 0, data: { list: [{ id: '11', warehouse_id: '1', code: 'BIN-1' }], total: 1 } } })
  })
  await page.goto('/stocktake/orders')
  await page.getByRole('button', { name: '新建盘点单', exact: true }).click()
  const dialog = await selectWarehouse(page, '1')
  await expect(dialog).toContainText('库位选项加载失败')
  await expect(dialog.getByRole('button', { name: '创建', exact: true })).toBeDisabled()
  fail = false
  await dialog.getByRole('button', { name: '重新加载库位', exact: true }).click()
  await dialog.locator('.el-select').nth(1).click()
  await page.getByRole('option', { name: 'BIN-1', exact: true }).click()
  await dialog.getByRole('button', { name: '创建', exact: true }).click()
  await expect.poll(() => state.writes).toEqual(['POST /stocktake/orders'])
  expect(state.bodies[0]).toMatchObject({ warehouse_id: '1', location_id: '11', location_code: 'BIN-1' })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
