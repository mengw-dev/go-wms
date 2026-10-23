import { expect, test, type Page } from '@playwright/test'

const date = '2026-10-23T12:35:40+08:00'
const profile = { user_id: '42', username: 'operator', nickname: '仓库操作员', roles: [], perms: ['*'] }
const order = { id: '17', order_no: 'CK20261023000004', biz_order_no: '312', warehouse_id: '1', status: 'PICKING', expected_qty: 3, allocated_qty: 3, picked_qty: 0, created_at: date }
const task = { id: '11', task_no: 'PK202610230001', task_type: 'PICK', status: 'CREATED', order_id: '17', location_code: 'A01-01-01', batch_no: 'B20260910', target_qty: 3, done_qty: 0, wave_id: '0' }

async function fixture(page: Page, theme = 'dark') {
  const state = { detailFailure: false, writes: 0 }
  await page.addInitScript(({ profile, theme }) => {
    localStorage.setItem('WMS_TOKEN', 'navigation-test')
    localStorage.setItem('WMS_USER', JSON.stringify(profile))
    localStorage.setItem('gowms_theme', theme)
  }, { profile, theme })
  await page.route('**/api/v1/**', async route => {
    const req = route.request(), path = new URL(req.url()).pathname.replace('/api/v1', '')
    if (req.method() !== 'GET') state.writes++
    expect(req.method()).toBe('GET')
    if (path === '/outbound/orders/17' && state.detailFailure) {
      await route.fulfill({ status: 503, json: { code: 50000, msg: 'test temporary unavailable' } }); return
    }
    let data: unknown
    if (path === '/profile') data = profile
    else if (path === '/basic/warehouses') data = { list: [{ id: '1', code: 'WH01', name: '华东一号仓', status: 1 }], total: 1 }
    else if (path === '/basic/skus') data = { list: [{ id: '1', code: 'SKU001', name: '测试货品' }], total: 1 }
    else if (path === '/outbound/orders') data = { list: [order], total: 1 }
    else if (path === '/outbound/orders/17') data = { order, details: [], allocations: [{ id: '21', location_code: task.location_code, batch_no: task.batch_no, allocated_qty: 3, picked_qty: 0, status: 'ALLOCATED' }], tasks: [task] }
    else if (path === '/outbound/exceptions' || path === '/outbound/returns') data = { list: [], total: 0 }
    else throw Error(`Unexpected navigation test API: ${path}`)
    await route.fulfill({ json: { code: 0, msg: 'ok', data } })
  })
  return state
}

for (const theme of ['light', 'dark']) {
  for (const width of [1366, 390]) {
    test(`${theme} ${width}: list details opens exception actions and both history entries`, async ({ page }) => {
      await page.setViewportSize({ width, height: 844 })
      const state = await fixture(page, theme)
      await page.goto('/outbound/orders')
      await page.getByRole('button', { name: '详情', exact: true }).click()
      await expect(page).toHaveURL(/\/outbound\/orders\/17$/)
      await expect(page.getByText('异常与售后', { exact: true })).toBeVisible()
      await page.screenshot({ path: test.info().outputPath(`outbound-detail-${theme}-${width}.png`), animations: 'disabled' })
      await page.getByRole('button', { name: '登记短缺 / 货损', exact: true }).click()
      const dialog = page.getByRole('dialog', { name: '登记出库异常', exact: true })
      await expect(dialog).toBeVisible()
      await dialog.getByRole('button', { name: '返回', exact: true }).click()
      await expect(page.locator('.batch-hint')).toContainText('沿用原入库批次')
      await expect(page.locator('.detail-scroll')).toContainText('B20260910')
      await page.getByRole('button', { name: '退货记录', exact: true }).click()
      await expect(page.getByRole('heading', { name: '客户退货记录', exact: true })).toBeVisible()
      await page.getByRole('button', { name: '前往出库单处理', exact: true }).click()
      await page.getByRole('button', { name: '详情', exact: true }).click()
      await page.getByRole('button', { name: /^异常记录/ }).click()
      await expect(page.getByRole('heading', { name: '出库异常记录', exact: true })).toBeVisible()
      expect(state.writes).toBe(0)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    })
  }
}

for (const asset of ['js', 'css']) {
  test(`missing page ${asset}: prompts, keeps current page, refreshes the original target`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    const state = await fixture(page)
    let blocked = true, attempts = 0
    await page.route('**/*OutboundOrderDetail*', async route => {
      const url = new URL(route.request().url())
      const matching = url.pathname.endsWith(`.${asset}`) ||
        (url.pathname.endsWith('.vue') && (asset === 'css' ? url.searchParams.get('type') === 'style' : !url.searchParams.has('type')))
      if (!matching) { await route.continue(); return }
      attempts++
      if (blocked) await route.fulfill({ status: 404, body: 'missing old asset' })
      else await route.continue()
    })
    await page.goto('/outbound/orders')
    await page.getByRole('button', { name: '详情', exact: true }).click()
    const notice = page.getByRole('dialog', { name: '页面暂时无法打开', exact: true })
    await expect(notice).toBeVisible()
    await expect(page).toHaveURL(/\/outbound\/orders$/)
    await page.screenshot({ path: test.info().outputPath(`page-recovery-${asset}.png`), animations: 'disabled' })
    await notice.getByRole('button', { name: '留在当前页', exact: true }).click()
    await expect(notice).toBeHidden()
    await page.getByRole('button', { name: '详情', exact: true }).click()
    await expect(notice).toBeVisible()
    await expect(notice.getByRole('button', { name: '刷新并打开', exact: true })).toBeInViewport({ ratio: 1 })
    blocked = false
    await notice.getByRole('button', { name: '刷新并打开', exact: true }).click()
    await expect(page).toHaveURL(/\/outbound\/orders\/17$/)
    await expect(page.getByText('异常与售后', { exact: true })).toBeVisible()
    expect(attempts).toBeGreaterThanOrEqual(2)
    expect(state.writes).toBe(0)
  })
}

test('detail API fails: explicit retry restores the exception panel', async ({ page }) => {
  const state = await fixture(page)
  state.detailFailure = true
  await page.goto('/outbound/orders/17')
  await expect(page.getByText('出库单加载失败，请检查网络后重试', { exact: true })).toBeVisible()
  await expect(page.getByText('异常与售后', { exact: true })).toHaveCount(0)
  state.detailFailure = false
  await page.getByRole('button', { name: '重新加载出库单', exact: true }).click()
  await expect(page.getByText('异常与售后', { exact: true })).toBeVisible()
  expect(state.writes).toBe(0)
})

test('initial deep link asset failure: refresh keeps destination and session storage', async ({ page }) => {
  const state = await fixture(page)
  let blocked = true
  await page.route('**/*OutboundOrderDetail*', async route => {
    const url = new URL(route.request().url())
    const script = url.pathname.endsWith('.js') || (url.pathname.endsWith('.vue') && !url.searchParams.has('type'))
    if (blocked && script) await route.fulfill({ status: 404, body: 'missing old module' })
    else await route.continue()
  })
  await page.goto('/outbound/orders/17?from=exceptions#receipt')
  const notice = page.getByRole('dialog', { name: '页面暂时无法打开', exact: true })
  await expect(notice).toBeVisible()
  await page.evaluate(() => sessionStorage.setItem('navigation-preserve', 'original-operation'))
  blocked = false
  await notice.getByRole('button', { name: '刷新并打开', exact: true }).click()
  await expect(page.getByText('异常与售后', { exact: true })).toBeVisible()
  await expect(page).toHaveURL(/\/outbound\/orders\/17\?from=exceptions#receipt$/)
  expect(await page.evaluate(() => sessionStorage.getItem('navigation-preserve'))).toBe('original-operation')
  expect(state.writes).toBe(0)
})
