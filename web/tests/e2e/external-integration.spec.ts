import { expect, test } from '@playwright/test'
import { integrationApiKey, loginByUi, seedBaseData } from './support/api'

test('external OMS API creates outbound orders idempotently', async ({ page, request }) => {
  const seed = await seedBaseData(request)
  const bizOrderNo = `OMS-E2E-${seed.suffix}`
  const payload = {
    warehouse_code: seed.warehouse.code,
    biz_order_no: bizOrderNo,
    remark: 'Playwright OMS integration',
    details: [{ sku_code: seed.sku.code, expected_qty: 2 }],
  }

  const unauthorized = await request.post('/api/v1/integration/outbound-orders', {
    headers: { 'X-API-Key': 'wrong-key' },
    data: payload,
  })
  expect(unauthorized.status()).toBe(401)

  const first = await request.post('/api/v1/integration/outbound-orders', {
    headers: { 'X-API-Key': integrationApiKey },
    data: payload,
  })
  expect(first.ok()).toBeTruthy()
  const firstBody = await first.json()
  expect(firstBody.code).toBe(0)
  expect(firstBody.data.idempotent).toBe(false)
  expect(firstBody.data.status).toBe('DRAFT')

  const second = await request.post('/api/v1/integration/outbound-orders', {
    headers: { 'X-API-Key': integrationApiKey },
    data: payload,
  })
  expect(second.ok()).toBeTruthy()
  const secondBody = await second.json()
  expect(secondBody.data.idempotent).toBe(true)
  expect(secondBody.data.order_id).toBe(firstBody.data.order_id)

  await loginByUi(page)
  await page.goto('/outbound/orders')
  await page.getByPlaceholder('单号模糊搜索').fill(bizOrderNo)
  await page.getByRole('button', { name: '查询' }).click()
  await expect(page.getByRole('row').filter({ hasText: bizOrderNo })).toBeVisible()
})
