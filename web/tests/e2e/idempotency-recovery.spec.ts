import { expect, test, type Page } from '@playwright/test'
import { confirmMessageBox, loginByUi, requestApi, seedBaseData, seedStockedInventory, selectOption } from './support/api'

// Forward the real command and wait for a successful backend commit, then drop
// only its response. No business response or database result is mocked.
async function loseSuccessfulResponse(page: Page, path: string) {
  let key = ''
  let payload = ''
  let committed!: () => void
  const done = new Promise<void>((resolve) => { committed = resolve })
  await page.route(`**/api/v1${path}`, async (route) => {
    key = route.request().headers()['idempotency-key'] ?? ''
    payload = route.request().postData() ?? ''
    const response = await route.fetch()
    expect(response.ok()).toBeTruthy()
    expect((await response.json()).code).toBe(0)
    await route.abort('failed')
    committed()
  }, { times: 1 })
  return { done, key: () => key, payload: () => payload }
}

async function expectRecovery(page: Page, path: string, key: string, payload: string, action: () => Promise<void>) {
  const response = page.waitForResponse((res) => res.url().endsWith(`/api/v1${path}`) && res.request().method() === 'POST')
  await action()
  const replay = await response
  expect(replay.request().headers()['idempotency-key']).toBe(key)
  // The claim token may be absent after refresh; it is intentionally excluded
  // from the server's business fingerprint and is unnecessary for replay.
  const original = payload ? JSON.parse(payload) : null
  const retried = replay.request().postDataJSON()
  if (original?.claim_token !== undefined) {
    delete original.claim_token
    delete retried.claim_token
  }
  expect(retried).toEqual(original)
  expect((await replay.json()).code).toBe(0)
}

test('idempotency: receive and putaway recover after commit, lost response and refresh', async ({ page, request }) => {
  const seed = await seedBaseData(request)
  const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/inbound/orders', {
    warehouse_id: seed.warehouse.id, details: [{ sku_id: seed.sku.id, expected_qty: 3 }],
  })
  await requestApi(request, seed.token, 'POST', `/inbound/orders/${order.id}/submit`)
  await requestApi(request, seed.token, 'POST', `/inbound/orders/${order.id}/approve`)
  await loginByUi(page)
  await page.goto(`/inbound/orders/${order.id}`)
  await page.getByRole('button', { name: '收货', exact: true }).click()
  let dialog = page.getByRole('dialog', { name: '收货', exact: true })
  await expect(dialog.locator('.el-alert--error')).toHaveCount(0)
  await dialog.getByPlaceholder('首次收货必填').fill(`B-${seed.suffix}`)
  const receivePath = `/inbound/orders/${order.id}/receive`
  const receiveLost = await loseSuccessfulResponse(page, receivePath)
  await dialog.getByRole('button', { name: '收货', exact: true }).click()
  await receiveLost.done
  await expect(dialog.getByRole('button', { name: '恢复收货' })).toBeVisible()
  await page.reload()
  await page.getByRole('button', { name: '恢复待确认收货' }).click()
  dialog = page.getByRole('dialog', { name: '收货', exact: true })
  await expectRecovery(page, receivePath, receiveLost.key(), receiveLost.payload(), () => dialog.getByRole('button', { name: '恢复收货' }).click())
  await expect(dialog.getByRole('button', { name: '恢复收货' })).toBeHidden()
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()

  const detail = await requestApi<{ details: { received_qty: number }[]; tasks: { id: string; task_type: string; done_qty: number }[] }>(request, seed.token, 'GET', `/inbound/orders/${order.id}`)
  expect(detail.details[0]?.received_qty).toBe(3)
  const tasks = detail.tasks.filter((task) => task.task_type === 'PUTAWAY')
  expect(tasks).toHaveLength(1)
  const putawayPath = `/inbound/tasks/${tasks[0]!.id}/putaway`
  await page.getByRole('button', { name: '上架', exact: true }).first().click()
  dialog = page.getByRole('dialog', { name: '上架', exact: true })
  await expect(dialog.locator('.el-alert--error')).toHaveCount(0)
  await selectOption(page, dialog, '选择空闲库位', seed.location.code)
  const putawayLost = await loseSuccessfulResponse(page, putawayPath)
  await dialog.getByRole('button', { name: '确定上架' }).click()
  await putawayLost.done
  await expect(dialog.getByRole('button', { name: '恢复上架' })).toBeVisible()
  await page.reload()
  await page.getByRole('button', { name: '恢复待确认上架' }).click()
  dialog = page.getByRole('dialog', { name: '上架', exact: true })
  await expectRecovery(page, putawayPath, putawayLost.key(), putawayLost.payload(), () => dialog.getByRole('button', { name: '恢复上架' }).click())
  await expect(dialog).toBeHidden()
  const inventory = await requestApi<{ list: { stock_quantity: number }[] }>(request, seed.token, 'GET', `/inventory?warehouse_id=${seed.warehouse.id}&sku_id=${seed.sku.id}`)
  expect(inventory.list).toHaveLength(1)
  expect(inventory.list[0]?.stock_quantity).toBe(3)
})

test('idempotency: claim and completed pick recover with original keys after refresh', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 2)
  const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/outbound/orders', {
    warehouse_id: seed.warehouse.id, biz_order_no: `IDEM-${seed.suffix}`, details: [{ sku_id: seed.sku.id, expected_qty: 2 }],
  })
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/submit`)
  await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/approve`)
  const data = await requestApi<{ tasks: { id: string }[] }>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
  const taskId = data.tasks[0]!.id
  await loginByUi(page)
  await page.goto(`/outbound/orders/${order.id}`)
  await page.getByRole('button', { name: '拣货', exact: true }).first().click()
  let dialog = page.getByRole('dialog', { name: 'PDA 拣货', exact: true })
  await expect(dialog.locator('.el-alert--error')).toHaveCount(0)
  const claimPath = `/pda/tasks/${taskId}/claim`
  const claimLost = await loseSuccessfulResponse(page, claimPath)
  await dialog.getByRole('button', { name: '领取任务' }).click()
  await claimLost.done
  await expect(dialog.getByRole('button', { name: '恢复领取' })).toBeVisible()
  await page.reload()
  await page.getByRole('button', { name: '恢复待确认拣货' }).click()
  dialog = page.getByRole('dialog', { name: 'PDA 拣货', exact: true })
  await expectRecovery(page, claimPath, claimLost.key(), claimLost.payload(), () => dialog.getByRole('button', { name: '恢复领取' }).click())
  await dialog.getByPlaceholder('扫描或输入批次号核对').fill(seed.batchNo)
  const pickPath = `/pda/tasks/${taskId}/pick`
  const pickLost = await loseSuccessfulResponse(page, pickPath)
  await dialog.getByRole('button', { name: '确定拣货' }).click()
  await pickLost.done
  await expect(dialog.getByRole('button', { name: '恢复拣货' })).toBeVisible()
  await page.reload()
  await page.getByRole('button', { name: '恢复待确认拣货' }).click()
  dialog = page.getByRole('dialog', { name: 'PDA 拣货', exact: true })
  await expectRecovery(page, pickPath, pickLost.key(), pickLost.payload(), () => dialog.getByRole('button', { name: '恢复拣货' }).click())
  await expect(dialog).toBeHidden()
  const after = await requestApi<{ order: { picked_qty: number; status: string }; tasks: { done_qty: number }[] }>(request, seed.token, 'GET', `/outbound/orders/${order.id}`)
  expect(after.order.picked_qty).toBe(2)
  expect(after.order.status).toBe('SHIPPED')
  expect(after.tasks[0]?.done_qty).toBe(2)
})

test('idempotency: completed stocktake approval can recover after lost response', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 2)
  const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/stocktake/orders', { warehouse_id: seed.warehouse.id })
  const data = await requestApi<{ details: { id: string }[] }>(request, seed.token, 'GET', `/stocktake/orders/${order.id}`)
  await requestApi(request, seed.token, 'POST', `/stocktake/orders/${order.id}/actual`, { detail_id: data.details[0]!.id, actual_qty: 1 })
  await loginByUi(page)
  await page.goto(`/stocktake/orders/${order.id}`)
  const path = `/stocktake/orders/${order.id}/approve`
  const lost = await loseSuccessfulResponse(page, path)
  await page.getByRole('button', { name: '审核', exact: true }).click()
  await confirmMessageBox(page, '确认审核')
  await lost.done
  await page.reload()
  await expectRecovery(page, path, lost.key(), lost.payload(), async () => {
    await page.getByRole('button', { name: '恢复待确认审核' }).click()
    await confirmMessageBox(page, '确认审核')
  })
  await expect(page.getByRole('button', { name: '恢复待确认审核' })).toBeHidden()
  const inventory = await requestApi<{ list: { stock_quantity: number }[] }>(request, seed.token, 'GET', `/inventory?warehouse_id=${seed.warehouse.id}&sku_id=${seed.sku.id}`)
  expect(inventory.list[0]?.stock_quantity).toBe(1)
})
