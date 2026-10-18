import { expect, test } from '@playwright/test'
import { loginByUi, requestApi, seedBaseData } from './support/api'

test('P2: order form can select warehouse and SKU beyond the first 100 records', async ({ page, request }) => {
  const seed = await seedBaseData(request)
  for (let i = 0; i < 101; i++) {
    await requestApi(request, seed.token, 'POST', '/basic/warehouses', { code: `P2W-${seed.suffix}-${i}`, name: `P2 warehouse ${i}` })
    await requestApi(request, seed.token, 'POST', '/basic/skus', { code: `P2S-${seed.suffix}-${i}`, barcode: `P2B-${seed.suffix}-${i}`, name: `P2 SKU ${i}`, unit: '件' })
  }
  await loginByUi(page)
  const secondWarehouses = page.waitForResponse((r) => r.url().includes('/basic/warehouses?') && new URL(r.url()).searchParams.get('page') === '2')
  const secondSkus = page.waitForResponse((r) => r.url().includes('/basic/skus?') && new URL(r.url()).searchParams.get('page') === '2')
  await page.goto('/inbound/orders')
  await Promise.all([secondWarehouses, secondSkus])
  await page.getByRole('button', { name: '新建入库单', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '新建入库单', exact: true })
  await dialog.locator('.el-select').first().click()
  await page.getByRole('option', { name: `${seed.warehouse.code}（${seed.warehouse.name}）`, exact: true }).click()
  await dialog.locator('.el-select').nth(1).click()
  const option = page.getByRole('option').filter({ hasText: seed.sku.code })
  await expect(option).toBeVisible()
  await option.click()
  const saved = page.waitForResponse((r) => r.url().endsWith('/inbound/orders') && r.request().method() === 'POST')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  const body = await (await saved).json()
  expect(body.code).toBe(0)
  const detail = await requestApi<{ order: { warehouse_id: string }; details: { sku_id: string }[] }>(request, seed.token, 'GET', `/inbound/orders/${body.data.id}`)
  expect(detail.order.warehouse_id).toBe(seed.warehouse.id)
  expect(detail.details[0]?.sku_id).toBe(seed.sku.id)
})
