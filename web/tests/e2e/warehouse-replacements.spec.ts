import { expect, test, type APIRequestContext } from '@playwright/test'
import { confirmMessageBox, loginByUi, requestApi, seedStockedInventory, type StockedSeedData } from './support/api'
import type { OutboundOrderDetail } from '../../src/api/types'
import type { InventoryAlert } from '../../src/api/inventoryAlerts'
import type { WaveDetail } from '../../src/api/wave'

async function command(request: APIRequestContext, token: string, path: string, data: unknown) {
  const response = await request.post(`/api/v1${path}`, {
    data, headers: { Authorization: `Bearer ${token}`, 'Idempotency-Key': crypto.randomUUID() },
  })
  const body = await response.json()
  expect(body.code, `${path}: ${body.msg}`).toBe(0)
  return body.data as { object_id: string }
}

async function replacement(request: APIRequestContext, seed: StockedSeedData, partial = 0) {
  const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/outbound/orders', {
    warehouse_id: seed.warehouse.id, biz_order_no: `REPLACE-${seed.suffix}`,
    details: [{ sku_id: seed.sku.id, expected_qty: 5 }],
  })
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/submit`)
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/approve`)
  const original = await requestApi<OutboundOrderDetail>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
  if (partial) await requestApi(request, seed.token, 'POST', `/outbound/tasks/${original.tasks[0]!.id}/pick`, { qty: partial })
  const reported = await command(request, seed.token, '/outbound/exceptions', {
    task_id: original.tasks[0]!.id, kind: 'DAMAGE', quantity: 1,
    reason: '未发实物已经全部退回，货损隔离', confirm_returned: true,
  })
  await command(request, seed.token, `/outbound/exceptions/${reported.object_id}/resolve`, {
    close: false, reason: '重新分配未发需求',
  })
  return { ...order, originalTaskID: original.tasks[0]!.id }
}

test('real replacement history: preview and create a wave from untouched replacement tasks', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 20)
  const order = await replacement(request, seed)
  const candidates = await requestApi<{ id: string }[]>(request, seed.token, 'GET',
    `/outbound/waves/candidates?warehouse_id=${seed.warehouse.id}&limit=20`)
  expect(candidates.map(row => row.id)).toContain(order.id)
  await loginByUi(page)
  await page.goto('/outbound/waves')
  await page.getByRole('button', { name: '新建波次', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '新建波次', exact: true })
  await dialog.locator('.el-select').click()
  await page.getByRole('option', { name: `${seed.warehouse.code}（${seed.warehouse.name}）`, exact: true }).click()
  await dialog.getByRole('button', { name: '手动选择订单', exact: true }).click()
  const row = dialog.getByRole('row').filter({ hasText: `REPLACE-${seed.suffix}` })
  await expect(row).toBeVisible()
  await row.locator('.el-checkbox').click()
  const created = page.waitForResponse(res => res.url().endsWith('/api/v1/outbound/waves') && res.request().method() === 'POST')
  await dialog.getByRole('button', { name: '确认创建波次', exact: true }).click()
  const body = await (await created).json()
  expect(body.code).toBe(0)
  await expect(page).toHaveURL(new RegExp(`/outbound/waves/${body.data.wave_id}$`))
  const wave = await requestApi<WaveDetail>(request, seed.token, 'GET', `/outbound/waves/${body.data.wave_id}`)
  expect(wave.items).toHaveLength(1)
  expect(wave.orders).toHaveLength(1)
  const detail = await requestApi<OutboundOrderDetail>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
  expect(detail.tasks.find(task => task.id === order.originalTaskID)?.status).toBe('CANCELLED')
})

test('real alert rule: create, edit threshold, pause, resume, trace stock and delete', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 6)
  await loginByUi(page)
  await page.goto(`/inventory/alerts?warehouse_id=${seed.warehouse.id}`)
  await page.getByRole('button', { name: '配置预警规则', exact: true }).click()
  const create = page.getByRole('dialog', { name: '配置预警规则', exact: true })
  await create.locator('.el-select').nth(1).click()
  await page.getByRole('option').filter({ hasText: seed.sku.code }).click()
  await create.getByRole('spinbutton').fill('10')
  await create.getByRole('button', { name: '保存规则', exact: true }).click()
  await expect(create).toBeHidden()
  const listed = await requestApi<{ list: InventoryAlert[] }>(request, seed.token, 'GET', `/inventory/alerts?warehouse_id=${seed.warehouse.id}&state=ALL`)
  expect(listed.list[0]!.sku_id).toBe(seed.sku.id)
  const row = page.getByRole('row').filter({ hasText: seed.sku.code })
  await expect(row).toContainText('低库存')
  await row.getByRole('button', { name: '规则', exact: true }).click()
  const edit = page.getByRole('dialog', { name: '修改预警规则', exact: true })
  await expect(edit).toContainText(seed.sku.code)
  await edit.getByRole('spinbutton').fill('12')
  await edit.locator('.el-switch').click()
  await edit.getByRole('button', { name: '保存规则', exact: true }).click()
  await expect(edit).toBeHidden()
  await page.locator('.query-form .el-select').nth(1).click()
  await page.getByRole('option', { name: '全部规则', exact: true }).click()
  await page.getByRole('button', { name: '查询', exact: true }).click()
  await expect(row).toContainText('已暂停')
  await row.getByRole('button', { name: '规则', exact: true }).click()
  await expect(edit.getByRole('spinbutton')).toHaveValue('12')
  await edit.locator('.el-switch').click()
  await edit.getByRole('button', { name: '保存规则', exact: true }).click()
  await expect(edit).toBeHidden()
  await row.getByRole('button', { name: '库存明细', exact: true }).click()
  await expect(page).toHaveURL(/\/inventory\?/)
  await expect(page.getByRole('row').filter({ hasText: seed.batchNo })).toBeVisible()
  await page.goto(`/inventory/alerts?warehouse_id=${seed.warehouse.id}`)
  await row.getByRole('button', { name: '删除', exact: true }).click()
  await confirmMessageBox(page)
  await expect(row).toHaveCount(0)
})

test('real cancellation: returned historical pick progress does not block an untouched replacement', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 20)
  const order = await replacement(request, seed, 2)
  await loginByUi(page)
  await page.goto(`/outbound/orders/${order.id}`)
  await page.locator('.detail-actions').getByRole('button', { name: '取消', exact: true }).click()
  await confirmMessageBox(page, '确认取消')
  await expect(page.getByText('已取消', { exact: true }).first()).toBeVisible()
  const detail = await requestApi<OutboundOrderDetail>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
  expect(detail.order.status).toBe('CANCELLED')
  expect(detail.tasks.find(task => task.id === order.originalTaskID)).toMatchObject({ status: 'CANCELLED', done_qty: 2 })
  const inventory = await requestApi<{ list: { stock_quantity: number; available_quantity: number; allocated_quantity: number; frozen_quantity: number }[] }>(request, seed.token, 'GET', `/inventory?warehouse_id=${seed.warehouse.id}`)
  expect(inventory.list[0]).toMatchObject({ stock_quantity: 20, available_quantity: 19, allocated_quantity: 0, frozen_quantity: 1 })
})
