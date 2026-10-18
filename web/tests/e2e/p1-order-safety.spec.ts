import { expect, test, type Page } from '@playwright/test'
import { confirmMessageBox, loginByUi, requestApi, seedBaseData, seedStockedInventory } from './support/api'

// Navigate through the real SPA router, retaining the mounted Layout. A full
// page.goto would hide the component-reuse bug this regression protects.
async function navigateWithinApp(page: Page, path: string) {
  await page.evaluate(async (target) => {
    const element = document.querySelector('#app') as HTMLElement & {
      __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<unknown> } } } }
    }
    await element.__vue_app__.config.globalProperties.$router.push(target)
  }, path)
}

test('P1: inbound edit cannot save stale data while the next order is loading', async ({ page, request }) => {
  const seed = await seedBaseData(request)
  const create = (remark: string, qty: number) => requestApi<{ id: string; order_no: string }>(request, seed.token, 'POST', '/inbound/orders', {
    warehouse_id: seed.warehouse.id, remark, details: [{ sku_id: seed.sku.id, expected_qty: qty }],
  })
  const a = await create(`A-${seed.suffix}`, 2)
  const b = await create(`B-${seed.suffix}`, 7)
  await loginByUi(page)
  await page.goto('/inbound/orders')
  const row = (number: string) => page.getByRole('row').filter({ hasText: number }).first()
  await row(a.order_no).getByRole('button', { name: '编辑', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '编辑入库单', exact: true })
  await expect(dialog.getByPlaceholder('备注（可选）')).toHaveValue(`A-${seed.suffix}`)
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  let unblock!: () => void
  const gate = new Promise<void>((resolve) => { unblock = resolve })
  await page.route(`**/api/v1/inbound/orders/${b.id}`, async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await gate
    await route.continue()
  }, { times: 1 })
  const writes: string[] = []
  page.on('request', (req) => { if (req.method() === 'PUT' && req.url().includes('/inbound/orders/')) writes.push(req.url()) })
  try {
    await row(b.order_no).getByRole('button', { name: '编辑', exact: true }).click()
    await expect(dialog.getByRole('button', { name: '保存', exact: true })).toBeDisabled()
    await expect(dialog.getByPlaceholder('备注（可选）')).toHaveValue('')
    expect(writes).toHaveLength(0)
  } finally { unblock() }
  await expect(dialog.getByPlaceholder('备注（可选）')).toHaveValue(`B-${seed.suffix}`)
  await dialog.getByPlaceholder('备注（可选）').fill('B updated safely')
  const saved = page.waitForResponse((res) => res.url().endsWith(`/inbound/orders/${b.id}`) && res.request().method() === 'PUT')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  expect((await (await saved).json()).code).toBe(0)
  const original = await requestApi<{ order: { remark: string; expected_qty: number } }>(request, seed.token, 'GET', `/inbound/orders/${a.id}`)
  const updated = await requestApi<{ order: { remark: string; expected_qty: number } }>(request, seed.token, 'GET', `/inbound/orders/${b.id}`)
  expect(original.order.remark).toBe(`A-${seed.suffix}`)
  expect(original.order.expected_qty).toBe(2)
  expect(updated.order.remark).toBe('B updated safely')
  expect(updated.order.expected_qty).toBe(7)
})

for (const kind of ['inbound', 'outbound', 'stocktake'] as const) {
  test(`P1: ${kind} SPA detail navigation operates on the new order ID`, async ({ page, request }) => {
    const seed = await seedStockedInventory(request, 2)
    const create = async (suffix: string) => requestApi<{ id: string; order_no: string }>(request, seed.token, 'POST', `/${kind}/orders`, {
      warehouse_id: seed.warehouse.id, remark: `${kind}-${suffix}-${seed.suffix}`,
      ...(kind === 'outbound' ? { biz_order_no: `${suffix}-${seed.suffix}` } : {}),
      ...(kind !== 'stocktake' ? { details: [{ sku_id: seed.sku.id, expected_qty: 1 }] } : {}),
    })
    const a = await create('A')
    const b = await create('B')
    await loginByUi(page)
    await page.goto(`/${kind}/orders/${a.id}`)
    await expect(page.getByText(a.order_no, { exact: true }).first()).toBeVisible()
    const bLoaded = page.waitForResponse((res) => res.url().endsWith(`/${kind}/orders/${b.id}`) && res.request().method() === 'GET')
    await navigateWithinApp(page, `/${kind}/orders/${b.id}`)
    await bLoaded
    await expect(page.getByText(b.order_no, { exact: true }).first()).toBeVisible()
    if (kind === 'stocktake') {
      await page.locator('.actual-editor .el-input-number input').first().fill('1')
      const saved = page.waitForResponse((res) => res.url().endsWith(`/stocktake/orders/${b.id}/actual`) && res.request().method() === 'POST')
      await page.getByRole('button', { name: '保存', exact: true }).first().click()
      expect((await (await saved).json()).code).toBe(0)
      const approved = page.waitForResponse((res) => res.url().endsWith(`/stocktake/orders/${b.id}/approve`) && res.request().method() === 'POST')
      await page.getByRole('button', { name: '审核', exact: true }).click()
      await confirmMessageBox(page, '确认审核')
      expect((await (await approved).json()).code).toBe(0)
    } else {
      const submitted = page.waitForResponse((res) => res.url().endsWith(`/${kind}/orders/${b.id}/submit`) && res.request().method() === 'POST')
      await page.getByRole('button', { name: '提交', exact: true }).click()
      await confirmMessageBox(page)
      expect((await (await submitted).json()).code).toBe(0)
    }
    const original = await requestApi<{ order: { status: string }; details: { actual_qty: number | null }[] }>(request, seed.token, 'GET', `/${kind}/orders/${a.id}`)
    const current = await requestApi<{ order: { status: string } }>(request, seed.token, 'GET', `/${kind}/orders/${b.id}`)
    expect(original.order.status).toBe('DRAFT')
    expect(current.order.status).toBe(kind === 'stocktake' ? 'COMPLETED' : 'SUBMITTED')
    if (kind === 'stocktake') expect(original.details[0]?.actual_qty).toBeNull()
  })
}
