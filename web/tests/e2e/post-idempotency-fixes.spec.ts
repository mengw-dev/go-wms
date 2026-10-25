import { expect, test, type APIRequestContext } from '@playwright/test'
import { seedStockedInventory, requestApi, loginByUi } from './support/api'
import type { InventoryItem, OutboundOrderDetail } from '../../src/api/types'
import type { StockMove } from '../../src/api/stockMoves'
import type { WaveDetail } from '../../src/api/wave'

interface Envelope<T> { code: number; data: T; msg: string }
interface Rows<T> { list: T[] }

async function command<T>(request: APIRequestContext, token: string, path: string, data: unknown): Promise<T> {
  const response = await request.post(`/api/v1${path}`, {
    data, headers: { Authorization: `Bearer ${token}`, 'Idempotency-Key': crypto.randomUUID() },
  })
  const body = await response.json() as Envelope<T>
  expect(response.ok()).toBe(true)
  expect(body.code, body.msg).toBe(0)
  return body.data
}

test.afterEach(async ({ page, request }) => {
  const auth = await page.evaluate(() => ({
    token: sessionStorage.getItem('WMS_TOKEN'), session: sessionStorage.getItem('WMS_DEMO_SESSION'),
  })).catch(() => null)
  if (auth?.token && auth.session) await request.post('/api/v1/demo/session/release', {
    headers: { Authorization: `Bearer ${auth.token}`, 'X-Demo-Session': auth.session },
  })
})

test('异常重分配后的打印分开当前指令与取消历史', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 20)
  const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/outbound/orders', {
    warehouse_id: seed.warehouse.id, biz_order_no: `PRINT-${seed.suffix}`,
    details: [{ sku_id: seed.sku.id, expected_qty: 5 }],
  })
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/submit`)
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/approve`)
  const before = await requestApi<OutboundOrderDetail>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
  const incident = await command<{ object_id: string }>(request, seed.token, '/outbound/exceptions', {
    task_id: before.tasks[0]!.id, kind: 'DAMAGE', quantity: 1, confirm_returned: true, reason: '未发实物退回',
  })
  await command(request, seed.token, `/outbound/exceptions/${incident.object_id}/resolve`, { close: false, reason: '重新分配' })
  const after = await requestApi<OutboundOrderDetail>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
  expect(after.allocations.map(row => row.status).sort()).toEqual(['ALLOCATED', 'CANCELLED'])
  await loginByUi(page)
  await page.goto(`/outbound/orders/${order.id}`)
  await page.getByRole('button', { name: '打印预览', exact: true }).click()
  const active = page.locator('.print-paper section').filter({ hasText: '按原分配行核对拣货' })
  await expect(active.locator('tbody tr')).toHaveCount(1)
  const history = page.locator('.print-paper section').filter({ hasText: '分配历史（仅追溯，不作为拣货指令）' })
  await expect(history.locator('tbody tr')).toHaveCount(1)
  await expect(history).toContainText('已取消')
})

const ledgerTypes = { FREEZE: '冻结', UNFREEZE: '解冻', SHORTAGE: '短缺扣减' } as const
for (const [type, label] of Object.entries(ledgerTypes)) test(`库存流水可按${label}筛选并导出`, async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 10)
  const stocks = await requestApi<Rows<InventoryItem>>(request, seed.token, 'GET', `/inventory?warehouse_id=${seed.warehouse.id}`)
  const inventory = stocks.list[0]!
  await command(request, seed.token, '/inventory/freeze', {
    inventory_id: inventory.id, quantity: 1, unfreeze: false, reason: '导出回归',
  })
  if (type === 'UNFREEZE') await command(request, seed.token, '/inventory/freeze', {
    inventory_id: inventory.id, quantity: 1, unfreeze: true, reason: '复验合格',
  })
  await loginByUi(page)
  await page.goto(`/inventory?inventory_id=${inventory.id}`)
  const drawer = page.getByRole('dialog').filter({ hasText: '库存流水' })
  await drawer.locator('.el-form-item').filter({ hasText: '类型' }).locator('.el-select').click()
  await page.getByRole('option', { name: label, exact: true }).click()
  const response = page.waitForResponse(r => r.url().includes('/inventory/trans/export'))
  const download = page.waitForEvent('download')
  await drawer.getByRole('button', { name: '导出Excel', exact: true }).click()
  const exported = await response
  expect(exported.status()).toBe(200)
  expect(new URL(exported.url()).searchParams.get('trans_type')).toBe(type)
  expect(exported.headers()['content-type']).toContain('spreadsheetml')
  expect(await (await download).failure()).toBeNull()
})

test('移库已提交但回执ID损坏时保留原请求，只读核对后恢复', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 6)
  const stocks = await requestApi<Rows<InventoryItem>>(request, seed.token, 'GET', `/inventory?warehouse_id=${seed.warehouse.id}`)
  await requestApi(request, seed.token, 'POST', '/basic/locations/batch', {
    warehouse_id: seed.warehouse.id, zone: 'TARGET', row_from: 1, row_to: 1, col_from: 1, col_to: 1,
  })
  const bins = await requestApi<Rows<{ id: string; code: string }>>(request, seed.token, 'GET', `/basic/locations?warehouse_id=${seed.warehouse.id}&page_size=100`)
  const target = bins.list.find(bin => bin.id !== seed.location.id)!
  await loginByUi(page)
  await page.goto(`/inventory/moves?source_inventory_id=${stocks.list[0]!.id}`)
  const dialog = page.getByRole('dialog', { name: '仓内移库', exact: true })
  await expect(dialog.getByRole('spinbutton')).toBeEnabled()
  await dialog.locator('.el-select').click()
  await page.getByRole('option', { name: target.code, exact: true }).click()
  await dialog.locator('textarea').fill('回执故障注入')
  let writes = 0
  await page.route('**/api/v1/inventory/moves', async route => {
    if (route.request().method() !== 'POST') { await route.continue(); return }
    writes++
    const response = await route.fetch()
    const body = await response.json() as Envelope<StockMove>
    expect(body.code).toBe(0)
    await route.fulfill({ response, json: { ...body, data: { ...body.data, id: '0' } } })
  })
  await dialog.getByRole('button', { name: '确认移库', exact: true }).click()
  const pending = () => page.evaluate(() => JSON.parse(localStorage.getItem('WMS_PENDING_OPS') || '{"items":[]}').items as { operationKey: string }[])
  await expect.poll(async () => (await pending()).length).toBe(1)
  await expect(dialog).toContainText('移库响应无法核对')
  const key = (await pending())[0]!.operationKey
  await dialog.getByRole('button', { name: '返回', exact: true }).click()
  await page.route('**/api/v1/inventory/moves?*', async route => {
    if (!new URL(route.request().url()).searchParams.has('operation_key')) { await route.continue(); return }
    const response = await route.fetch()
    const body = await response.json() as Envelope<Rows<StockMove>>
    await route.fulfill({ response, json: { ...body, data: { ...body.data, list: body.data.list.map(row => ({ ...row, id: '0' })) } } })
  })
  await page.getByRole('button', { name: '核对结果', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: '原请求与移库记录不一致，已保留本地记录，请人工核对' })).toBeVisible()
  expect((await pending())[0]!.operationKey).toBe(key)
  await page.unroute('**/api/v1/inventory/moves?*')
  await page.getByRole('button', { name: '核对结果', exact: true }).click()
  await expect.poll(async () => (await pending()).length).toBe(0)
  expect(writes).toBe(1)
})

test('演示会话同身份续期后条码预览保持打开', async ({ page, request }) => {
  const offered = await request.post('/api/v1/demo/account')
  const account = await offered.json() as Envelope<{ username: string; password: string }>
  expect(account.code).toBe(0)
  await loginByUi(page, account.data.username, account.data.password, /WMS 业务闭环/)
  await page.goto('/basic/skus')
  await page.getByRole('row').filter({ hasText: 'SKU000001' }).getByRole('button', { name: '标签', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '条码标签预览', exact: true })
  await expect(dialog).toBeVisible()
  const session = await page.evaluate(() => sessionStorage.getItem('WMS_DEMO_SESSION'))
  const heartbeat = page.waitForResponse(r => r.url().endsWith('/demo/session/heartbeat') && r.request().method() === 'POST', { timeout: 35000 })
  await page.mouse.move(500, 300)
  const response = await heartbeat
  const body = await response.json() as Envelope<{ session_id: string }>
  expect(body.code).toBe(0)
  expect(body.data.session_id).toBe(session)
  await response.finished()
  // Closing dialogs remain visible during Element Plus's 300 ms transition.
  await page.waitForTimeout(600)
  await expect(dialog).toBeVisible()
})

test('冻结库存后工程并发演示与性能统计使用四数量口径', async ({ page, request }) => {
  const account = await (await request.post('/api/v1/demo/account')).json() as Envelope<{ username: string; password: string }>
  expect(account.code).toBe(0)
  await loginByUi(page, account.data.username, account.data.password, /WMS 业务闭环/)
  const auth = await page.evaluate(() => ({ token: sessionStorage.getItem('WMS_TOKEN'), session: sessionStorage.getItem('WMS_DEMO_SESSION') }))
  const headers = { Authorization: `Bearer ${auth.token}`, 'X-Demo-Session': auth.session! }
  const get = async <T>(path: string): Promise<T> => {
    const response = await request.get(`/api/v1${path}`, { headers })
    const body = await response.json() as Envelope<T>
    expect(body.code, body.msg).toBe(0)
    return body.data
  }
  const warehouses = await get<Rows<{ id: string }>>('/basic/warehouses?page_size=100')
  const skus = await get<Rows<{ id: string }>>('/basic/skus?page_size=100&keyword=SKU000001')
  const stocks = await get<Rows<InventoryItem>>(`/inventory?warehouse_id=${warehouses.list[0]!.id}&sku_id=${skus.list[0]!.id}`)
  const stock = stocks.list.find(row => row.available_quantity > 1)!
  const frozen = await (await request.post('/api/v1/inventory/freeze', {
    headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
    data: { inventory_id: stock.id, quantity: 1, unfreeze: false, reason: '正常质量隔离' },
  })).json() as Envelope<unknown>
  expect(frozen.code).toBe(0)
  const result = await (await request.post('/api/v1/demo/run/concurrent', {
    headers, data: { concurrency: 2, qty_per_order: 1 },
  })).json() as Envelope<{ invariant_ok: boolean; stock_total: number; available_total: number; allocated_total: number; frozen_total: number }>
  expect(result.code).toBe(0)
  expect(result.data.invariant_ok).toBe(true)
  expect(result.data.frozen_total).toBe(1)
  expect(result.data.stock_total).toBe(result.data.available_total + result.data.allocated_total + result.data.frozen_total)
  const performance = await get<{ business: { frozen_total: number } }>('/demo/performance')
  expect(performance.business.frozen_total).toBe(1)
  await page.goto('/demo/performance')
  await page.getByRole('button', { name: '运行状态', exact: true }).click()
  await expect(page.getByText(/库存四数量/)).toBeVisible()
  await expect(page.getByText('库存四数量', { exact: true }).locator('..')).toContainText('冻结 1')
})

test('三轮异常退回重分配后组波发货、混合退货与移库保持库存一致', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 40)
  const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/outbound/orders', {
    warehouse_id: seed.warehouse.id, biz_order_no: `CYCLES-${seed.suffix}`,
    details: [{ sku_id: seed.sku.id, expected_qty: 5 }],
  })
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/submit`)
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/approve`)
  for (let n = 0; n < 3; n++) {
    const detail = await requestApi<OutboundOrderDetail>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
    const task = detail.tasks.find(row => row.status === 'CREATED')!
    await requestApi(request, seed.token, 'POST', `/outbound/tasks/${task.id}/pick`, { qty: 2 })
    const incident = await command<{ object_id: string }>(request, seed.token, '/outbound/exceptions', {
      task_id: task.id, kind: 'DAMAGE', quantity: 1, confirm_returned: true, reason: `第${n + 1}轮实物退回`,
    })
    await command(request, seed.token, `/outbound/exceptions/${incident.object_id}/resolve`, { close: false, reason: '重分配未发数量' })
  }
  const wave = await command<{ wave_id: string }>(request, seed.token, '/outbound/waves', { order_ids: [order.id] })
  await command(request, seed.token, `/outbound/waves/${wave.wave_id}/claim`, {})
  const detail = await requestApi<WaveDetail>(request, seed.token, 'GET', `/outbound/waves/${wave.wave_id}`)
  expect(detail.items).toHaveLength(1)
  const line = detail.lines[0]!, item = detail.items[0]!, member = detail.orders[0]!
  await command(request, seed.token, `/outbound/waves/${wave.wave_id}/lines/${line.id}/collect`, {
    qty: 5, sku_code: line.sku_code, location_code: line.location_code, batch_no: line.batch_no,
  })
  await command(request, seed.token, `/outbound/waves/${wave.wave_id}/items/${item.id}/sort`, {
    qty: 5, sku_code: line.sku_code, slot_code: member.slot_code, batch_no: line.batch_no,
  })
  await command(request, seed.token, `/outbound/waves/${wave.wave_id}/orders/${member.id}/ship`, {})
  const shipped = await requestApi<OutboundOrderDetail>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
  expect(shipped.order.status).toBe('SHIPPED')
  expect(shipped.tasks.filter(row => row.status === 'CANCELLED')).toHaveLength(3)
  const allocation = shipped.allocations.find(row => row.status === 'PICKED')!
  await command(request, seed.token, '/outbound/returns', {
    allocation_id: allocation.id, location_id: seed.location.id, good_qty: 3, bad_qty: 2, reason: '合格与不良分别处理',
  })
  await requestApi(request, seed.token, 'POST', '/basic/locations/batch', {
    warehouse_id: seed.warehouse.id, zone: 'CYCLE', row_from: 1, row_to: 1, col_from: 1, col_to: 1,
  })
  const bins = await requestApi<Rows<{ id: string }>>(request, seed.token, 'GET', `/basic/locations?warehouse_id=${seed.warehouse.id}&page_size=100`)
  const stocks = await requestApi<Rows<InventoryItem>>(request, seed.token, 'GET', `/inventory?warehouse_id=${seed.warehouse.id}`)
  await command(request, seed.token, '/inventory/moves', {
    source_inventory_id: stocks.list[0]!.id, target_location_id: bins.list.find(row => row.id !== seed.location.id)!.id,
    quantity: 2, reason: '只移动可用量',
  })
  const end = await requestApi<Rows<InventoryItem>>(request, seed.token, 'GET', `/inventory?warehouse_id=${seed.warehouse.id}`)
  expect(end.list.reduce((sum, row) => sum + row.stock_quantity, 0)).toBe(40)
  expect(end.list.reduce((sum, row) => sum + (row.frozen_quantity ?? 0), 0)).toBe(5)
  expect(end.list.every(row => row.allocated_quantity === 0 && row.stock_quantity === row.available_quantity + (row.frozen_quantity ?? 0))).toBe(true)
  await loginByUi(page)
  await page.goto(`/outbound/orders/${order.id}`)
  await page.getByRole('button', { name: '退货记录', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: '合格 3 / 不良 2' })).toBeVisible()
})
