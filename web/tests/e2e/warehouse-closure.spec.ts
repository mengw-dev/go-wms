import { expect, test, type Locator } from '@playwright/test'
import { loginByUi, requestApi, seedStockedInventory } from './support/api'
import type { OutboundOrderDetail } from '../../src/api/types'
import type { WaveDetail } from '../../src/api/wave'

async function reason(dialog: Locator, value: string) { await dialog.locator('textarea').fill(value) }

for (const display of [
  { theme: 'light', width: 1366, height: 768 },
  { theme: 'dark', width: 1366, height: 768 },
  { theme: 'dark', width: 390, height: 844 },
]) {
  test(`warehouse closure: ${display.theme} ${display.width} freeze, recover and receive return`, async ({ page, request }, testInfo) => {
    const seed = await seedStockedInventory(request, 20)
    const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/outbound/orders', {
      warehouse_id: seed.warehouse.id, biz_order_no: `CLOSURE-${seed.suffix}`, details: [{ sku_id: seed.sku.id, expected_qty: 5 }],
    })
    await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/submit`)
    await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/approve`)
    const detail = () => requestApi<OutboundOrderDetail>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
    const stock = async () => (await requestApi<{ list: { id: string; stock_quantity: number; available_quantity: number; allocated_quantity: number; frozen_quantity: number }[] }>(request, seed.token, 'GET', `/inventory?warehouse_id=${seed.warehouse.id}&sku_id=${seed.sku.id}`)).list[0]!
    await page.setViewportSize(display)
    await page.addInitScript(theme => localStorage.setItem('gowms_theme', theme), display.theme)
    await loginByUi(page)
    await page.goto(`/inventory?warehouse_id=${seed.warehouse.id}`)
    await page.getByRole('button', { name: '冻结/解冻', exact: true }).click()
    const freeze = page.getByRole('dialog', { name: '库存冻结 / 解冻', exact: true })
    await freeze.getByRole('spinbutton').fill('2')
    await reason(freeze, '质量待检')
    await freeze.getByRole('button', { name: '确认冻结', exact: true }).click()
    await expect(freeze).not.toBeVisible()
    expect(await stock()).toMatchObject({ stock_quantity: 20, available_quantity: 13, allocated_quantity: 5, frozen_quantity: 2 })
    await page.getByRole('button', { name: '冻结/解冻', exact: true }).click()
    await freeze.getByText('解冻', { exact: true }).click()
    await reason(freeze, '复检合格')
    await freeze.getByRole('button', { name: '确认解冻', exact: true }).click()
    await expect(freeze).not.toBeVisible()
    expect((await stock()).frozen_quantity).toBe(1)

    const original = (await detail()).tasks[0]!
    await requestApi(request, seed.token, 'POST', `/outbound/tasks/${original.id}/pick`, { qty: 2 })
    await page.goto(`/outbound/orders/${order.id}`)
    await page.getByRole('button', { name: '登记短缺 / 货损', exact: true }).click()
    const exception = page.getByRole('dialog', { name: '登记出库异常', exact: true })
    await exception.getByText('货损冻结', { exact: true }).click()
    await reason(exception, '未发货包装破损，实物已退回')
    await exception.locator('.el-checkbox').click()
    await exception.getByRole('button', { name: '确认处理', exact: true }).click()
    await expect(exception).not.toBeVisible()
    await expect(page.getByRole('button', { name: '处理异常', exact: true })).toBeEnabled()
    expect((await detail()).order).toMatchObject({ status: 'EXCEPTION', picked_qty: 0, allocated_qty: 0 })
    expect(await stock()).toMatchObject({ stock_quantity: 20, available_quantity: 18, frozen_quantity: 2, allocated_quantity: 0 })
    await page.getByRole('button', { name: '处理异常', exact: true }).click()
    const resolve = page.getByRole('dialog', { name: '处理出库异常', exact: true })
    await reason(resolve, '重新分配未发需求')
    await resolve.getByRole('button', { name: '确认处理', exact: true }).click()
    await expect(resolve).not.toBeVisible()
    await expect(page.getByRole('button', { name: '登记短缺 / 货损', exact: true })).toBeEnabled()
    const replacement = (await detail()).tasks.find(t => t.status === 'CREATED')!
    expect(replacement.target_qty).toBe(5)
    await requestApi(request, seed.token, 'POST', `/outbound/tasks/${replacement.id}/pick`, { qty: 5 })
    await page.reload()
    await page.getByRole('button', { name: '验收客户退货', exact: true }).click()
    const returns = page.getByRole('dialog', { name: '客户退货验收', exact: true })
    await returns.locator('.el-form-item').filter({ hasText: '验收入库库位' }).locator('.el-select').click()
    await page.getByRole('option', { name: seed.location.code, exact: true }).click()
    await returns.locator('.return-quantities').getByRole('spinbutton').nth(0).fill('2')
    await returns.locator('.return-quantities').getByRole('spinbutton').nth(1).fill('1')
    await reason(returns, '客户退回，合格2件不良1件')
    const keys: string[] = []
    let lose = display.width === 390
    await page.route('**/api/v1/outbound/returns', async route => {
      if (route.request().method() !== 'POST') { await route.continue(); return }
      keys.push(route.request().headers()['idempotency-key']!)
      if (!lose) { await route.continue(); return }
      lose = false
      const response = await route.fetch()
      expect((await response.json()).code).toBe(0)
      await route.abort('failed')
    })
    await returns.getByRole('button', { name: '确认处理', exact: true }).click()
    if (display.width === 390) {
      await expect(returns).toContainText('网络')
      await page.reload()
      await page.getByRole('button', { name: '恢复原操作', exact: true }).click()
      await expect(page.getByRole('button', { name: '恢复原操作', exact: true })).toHaveCount(0)
      expect(keys).toHaveLength(2); expect(keys[0]).toBe(keys[1])
    } else await expect(returns).not.toBeVisible()
    expect(await stock()).toMatchObject({ stock_quantity: 18, available_quantity: 15, allocated_quantity: 0, frozen_quantity: 3 })
    const receipts = await requestApi<{ total: number }>(request, seed.token, 'GET', `/outbound/returns?order_id=${order.id}`)
    expect(receipts.total).toBe(1)
    await page.getByRole('button', { name: '退货记录', exact: true }).click()
    await expect(page.getByRole('heading', { name: '客户退货记录', exact: true })).toBeVisible()
    await expect(page.locator('.mobile-record-table')).toContainText('合格 2 / 不良 1')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    if (display.width === 390) {
      await expect(page.locator('.business-pagination .el-pagination__jump')).not.toBeVisible()
      const bounds = await page.locator('.business-pagination').boundingBox()
      expect(bounds && bounds.x + bounds.width <= display.width).toBeTruthy()
    }
    await page.screenshot({ path: testInfo.outputPath('customer-return.png'), fullPage: true, animations: 'disabled' })
  })
}

test('warehouse closure: abort a started wave and restore ordinary picking', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 10)
  const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/outbound/orders', { warehouse_id: seed.warehouse.id, biz_order_no: `ABORT-${seed.suffix}`, details: [{ sku_id: seed.sku.id, expected_qty: 4 }] })
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/submit`)
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/approve`)
  const command = async (path: string, data: unknown) => {
    const response = await request.post(`/api/v1${path}`, { data, headers: { Authorization: `Bearer ${seed.token}`, 'Idempotency-Key': crypto.randomUUID() } })
    const body = await response.json(); expect(body.code, body.msg).toBe(0); return body.data
  }
  const ack = await command('/outbound/waves', { order_ids: [order.id], remark: '' })
  await command(`/outbound/waves/${ack.wave_id}/claim`, undefined)
  const before = await requestApi<WaveDetail>(request, seed.token, 'GET', `/outbound/waves/${ack.wave_id}`)
  await command(`/outbound/waves/${ack.wave_id}/lines/${before.lines[0]!.id}/collect`, { qty: 2, sku_code: seed.sku.code, location_code: seed.location.code, batch_no: seed.batchNo })
  await loginByUi(page)
  await page.goto(`/outbound/waves/${ack.wave_id}`)
  await page.getByRole('button', { name: '异常结束波次', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '异常结束波次', exact: true })
  await reason(dialog, '设备故障，集货已退回原位')
  await expect(dialog.getByRole('button', { name: '确认异常结束', exact: true })).toBeDisabled()
  await dialog.locator('.el-checkbox').click()
  await dialog.getByRole('button', { name: '确认异常结束', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page.getByText(/波次已异常结束：设备故障/)).toBeVisible()
  const after = await requestApi<WaveDetail>(request, seed.token, 'GET', `/outbound/waves/${ack.wave_id}`)
  expect(after.wave.status).toBe('ABORTED'); expect(after.lines[0]!.picked_qty).toBe(2)
  await page.goto(`/outbound/orders/${order.id}`)
  await expect(page.locator('.detail-actions').getByRole('button', { name: '拣货', exact: true })).toBeEnabled()
})
