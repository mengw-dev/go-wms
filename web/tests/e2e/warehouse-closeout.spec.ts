import { expect, test, type APIRequestContext, type Locator, type Page } from '@playwright/test'
import { loginByUi, requestApi, seedBaseData, seedStockedInventory, type SeedData } from './support/api'

// Real JWT -> HTTP -> migrated MySQL. Only the response-loss test intercepts a
// request: it forwards the command and discards its already committed response.
interface PageData<T> { list: T[]; total: number }
interface Stock {
  id: string; location_id: string; batch_no: string; stock_quantity: number
  available_quantity: number; allocated_quantity: number; stock_in_time: string
}
interface Movement {
  id: string; move_no: string; source_inventory_id: string; target_inventory_id: string
  quantity: number; batch_no: string
}
interface Ledger { trans_type: string; quantity_change: number }
interface PickTask {
  id: string; task_type: string; task_no: string; location_code: string
  batch_no: string; target_qty: number; done_qty: number
}

async function stocks(request: APIRequestContext, seed: SeedData) {
  return requestApi<PageData<Stock>>(request, seed.token, 'GET',
    `/inventory?warehouse_id=${seed.warehouse.id}&sku_id=${seed.sku.id}&page_size=100`)
}

async function targetLocation(request: APIRequestContext, seed: SeedData) {
  const zone = `MOVE${seed.suffix.slice(-6)}`
  await requestApi(request, seed.token, 'POST', '/basic/locations/batch', {
    warehouse_id: seed.warehouse.id, zone, row_from: 1, row_to: 1, col_from: 1, col_to: 1,
  })
  const code = `${zone}-01-01`
  const result = await requestApi<PageData<{ id: string; code: string }>>(
    request, seed.token, 'GET',
    `/basic/locations?warehouse_id=${seed.warehouse.id}&keyword=${code}&page_size=100`,
  )
  expect(result.list).toHaveLength(1)
  return result.list[0]!
}

async function scan(field: Locator, code: string) {
  await expect(field).toBeEditable()
  await field.fill(code)
  await field.press('Enter')
}

async function fillMove(page: Page, source: Stock, targetCode: string) {
  await page.goto(`/inventory/moves?source_inventory_id=${source.id}`)
  const dialog = page.getByRole('dialog', { name: '仓内移库', exact: true })
  await expect(dialog.getByRole('spinbutton')).toBeEnabled()
  await dialog.getByRole('combobox', { name: '目标库位', exact: true }).click()
  await page.getByRole('option', { name: targetCode, exact: true }).click()
  await dialog.getByRole('spinbutton').fill('2')
  await dialog.getByPlaceholder('例如：库位整理、调整存放位置').fill('收尾验收：同仓移库')
  await expect(dialog.getByRole('button', { name: '确认移库', exact: true })).toBeEnabled()
  return dialog
}

async function checkMove(request: APIRequestContext, seed: SeedData, movement: Movement) {
  const inventory = await stocks(request, seed)
  expect(inventory.list).toHaveLength(2)
  expect(inventory.list.map(row => row.stock_quantity).sort()).toEqual([2, 4])
  expect(inventory.list.reduce((sum, row) => sum + row.available_quantity, 0)).toBe(6)
  expect(inventory.list.every(row => row.allocated_quantity === 0 && row.batch_no === movement.batch_no)).toBe(true)
  expect(new Set(inventory.list.map(row => row.stock_in_time)).size).toBe(1)
  const ledger = await requestApi<PageData<Ledger>>(request, seed.token, 'GET',
    `/inventory/trans?order_no=${movement.move_no}&page_size=100`)
  expect(ledger.total).toBe(2)
  expect(ledger.list.map(row => row.trans_type).sort()).toEqual(['MOVE_IN', 'MOVE_OUT'])
  expect(ledger.list.map(row => row.quantity_change).sort((a, b) => a - b)).toEqual([-2, 2])
}

for (const display of [
  { theme: 'light', width: 1366, height: 768 },
  { theme: 'dark', width: 390, height: 844 },
]) {
  test(`real warehouse closeout: ${display.theme} ${display.width} scan, move and ship`, async ({ page, request }) => {
    test.setTimeout(120_000)
    await page.setViewportSize(display)
    await page.addInitScript(theme => localStorage.setItem('gowms_theme', theme), display.theme)
    const seed = await seedBaseData(request)
    const batch = `CLOSE-${seed.suffix}`
    const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/inbound/orders', {
      warehouse_id: seed.warehouse.id, details: [{ sku_id: seed.sku.id, expected_qty: 6 }],
    })
    await requestApi(request, seed.token, 'POST', `/inbound/orders/${order.id}/submit`)
    await requestApi(request, seed.token, 'POST', `/inbound/orders/${order.id}/approve`)
    await loginByUi(page)
    await page.goto(`/inbound/orders/${order.id}`)
    await page.locator('.detail-actions').getByRole('button', { name: '收货', exact: true }).click()
    const receive = page.getByRole('dialog', { name: '收货', exact: true })
    await scan(receive.getByRole('textbox', { name: '收货货品扫码', exact: true }), seed.sku.barcode)
    await expect(receive.getByText(`已定位 ${seed.sku.name}`, { exact: false })).toBeVisible()
    // Repeated scan must leave the original quantity and persisted order untouched.
    await scan(receive.getByRole('textbox', { name: '收货货品扫码', exact: true }), seed.sku.code)
    const untouched = await requestApi<{ details: { received_qty: number }[] }>(request, seed.token, 'GET', `/inbound/orders/${order.id}`)
    expect(untouched.details[0]!.received_qty).toBe(0)
    await receive.getByPlaceholder('首次收货必填').fill(batch)
    await receive.getByRole('button', { name: '收货', exact: true }).click()
    await expect(page.getByText('收货成功', { exact: true })).toBeVisible()
    await receive.getByRole('button', { name: '关闭', exact: true }).click()

    await page.locator('.detail-actions').getByRole('button', { name: '上架', exact: true }).click()
    const putaway = page.getByRole('dialog', { name: '上架', exact: true })
    await scan(putaway.getByRole('textbox', { name: '上架货品扫码', exact: true }), seed.sku.barcode)
    await expect(putaway.getByText(`已核对 ${seed.sku.name} · ${seed.sku.code}`, { exact: true })).toBeVisible()
    const locationScan = putaway.getByRole('textbox', { name: '上架库位扫码', exact: true })
    await scan(locationScan, 'NO-SUCH-CLOSEOUT-LOCATION')
    await expect(putaway.getByText('当前单据仓库没有此编码的启用库位，请核对仓库和标签')).toBeVisible()
    await expect(putaway.getByRole('button', { name: '确定上架', exact: true })).toBeDisabled()
    await scan(locationScan, seed.location.code)
    await expect(putaway.getByRole('button', { name: '确定上架', exact: true })).toBeEnabled()
    expect((await stocks(request, seed)).total).toBe(0)
    await putaway.getByRole('button', { name: '确定上架', exact: true }).click()
    await expect(putaway).toBeHidden()
    const beforeMove = await stocks(request, seed)
    expect(beforeMove.list).toHaveLength(1)
    expect(beforeMove.list[0]!.stock_quantity).toBe(6)

    const target = await targetLocation(request, seed)
    const move = await fillMove(page, beforeMove.list[0]!, target.code)
    const moved = page.waitForResponse(res => res.url().endsWith('/api/v1/inventory/moves') && res.request().method() === 'POST')
    await move.getByRole('button', { name: '确认移库', exact: true }).click()
    const moveResponse = await moved
    expect(moveResponse.ok()).toBe(true)
    const moveBody = await moveResponse.json()
    expect(moveBody.code).toBe(0)
    const movement = moveBody.data as Movement
    await expect(move).toBeHidden()
    await checkMove(request, seed, movement)

    const outbound = await requestApi<{ id: string }>(request, seed.token, 'POST', '/outbound/orders', {
      warehouse_id: seed.warehouse.id, biz_order_no: `CLOSE-${seed.suffix}`,
      details: [{ sku_id: seed.sku.id, expected_qty: 6 }],
    })
    await requestApi(request, seed.token, 'POST', `/outbound/orders/${outbound.id}/submit`)
    await requestApi(request, seed.token, 'POST', `/outbound/orders/${outbound.id}/approve`)
    const allocated = await stocks(request, seed)
    expect(allocated.list.reduce((sum, row) => sum + row.allocated_quantity, 0)).toBe(6)
    await page.goto(`/outbound/orders/${outbound.id}`)
    for (let remaining = 2; remaining > 0; remaining--) {
      const state = await requestApi<{ tasks: PickTask[] }>(request, seed.token, 'GET', `/outbound/orders/${outbound.id}`)
      const tasks = state.tasks.filter(task => task.task_type === 'PICK' && task.done_qty < task.target_qty)
      expect(tasks).toHaveLength(remaining)
      const task = tasks[0]!
      await page.locator('.detail-actions').getByRole('button', { name: '拣货', exact: true }).click()
      const pick = page.getByRole('dialog', { name: 'PDA 拣货', exact: true })
      await expect(pick.getByText(`${task.task_no}（目标 ${task.target_qty} 已完成 0）`, { exact: true })).toBeVisible()
      await pick.getByRole('button', { name: '领取任务', exact: true }).click()
      await expect(pick.getByText('已领取', { exact: true })).toBeVisible()
      await scan(pick.getByRole('textbox', { name: '拣货货品扫码', exact: true }), seed.sku.barcode)
      await expect(pick.getByText(`已核对 ${seed.sku.name} · ${seed.sku.code}`, { exact: true })).toBeVisible()
      await scan(pick.getByPlaceholder('扫描或输入作业库位'), task.location_code)
      await scan(pick.getByPlaceholder('扫描或输入批次号核对'), batch)
      await expect(pick.getByRole('spinbutton', { name: '拣货数量' })).toHaveValue(String(task.target_qty))
      const unchanged = await requestApi<{ tasks: PickTask[] }>(request, seed.token, 'GET', `/outbound/orders/${outbound.id}`)
      expect(unchanged.tasks.find(item => item.id === task.id)!.done_qty).toBe(0)
      await expect(pick.getByRole('button', { name: '确定拣货', exact: true })).toBeInViewport({ ratio: 1 })
      await pick.getByRole('button', { name: '确定拣货', exact: true }).click()
      await expect(pick).toBeHidden()
    }
    const shipped = await requestApi<{ order: { status: string } }>(request, seed.token, 'GET', `/outbound/orders/${outbound.id}`)
    expect(shipped.order.status).toBe('SHIPPED')
    const final = await stocks(request, seed)
    expect(final.list).toHaveLength(2)
    expect(final.list.every(row => row.stock_quantity === 0 && row.available_quantity === 0 && row.allocated_quantity === 0)).toBe(true)
    await page.screenshot({ animations: 'disabled', path: test.info().outputPath(`closeout-${display.theme}.png`) })
  })
}

test('real warehouse closeout: committed move survives lost response and reload without duplicate stock', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 6)
  const source = (await stocks(request, seed)).list[0]!
  const target = await targetLocation(request, seed)
  await loginByUi(page)
  const dialog = await fillMove(page, source, target.code)
  let writes = 0
  page.on('request', req => { if (req.method() === 'POST' && req.url().endsWith('/api/v1/inventory/moves')) writes++ })
  let result: Movement | undefined
  let key = ''
  await page.route('**/api/v1/inventory/moves', async route => {
    if (route.request().method() !== 'POST') { await route.continue(); return }
    key = route.request().headers()['idempotency-key'] || ''
    const response = await route.fetch()
    expect(response.ok()).toBe(true)
    const body = await response.json()
    expect(body.code).toBe(0)
    result = body.data
    await route.abort('failed')
  }, { times: 1 })
  await dialog.getByRole('button', { name: '确认移库', exact: true }).click()
  await expect(dialog.getByText('Network Error', { exact: true })).toBeVisible()
  expect(result).toBeDefined()
  expect(key).not.toBe('')
  await page.reload()
  await expect(page.getByLabel('待确认移库')).toBeVisible()
  await expect(page.getByRole('dialog', { name: '仓内移库', exact: true })).toBeHidden()
  await page.getByRole('button', { name: '核对结果', exact: true }).click()
  await expect(page.getByLabel('待确认移库')).toHaveCount(0)
  expect(writes).toBe(1)
  const history = await requestApi<PageData<Movement>>(request, seed.token, 'GET', `/inventory/moves?operation_key=${key}`)
  expect(history.total).toBe(1)
  expect(history.list[0]!.id).toBe(result!.id)
  await checkMove(request, seed, result!)
})
