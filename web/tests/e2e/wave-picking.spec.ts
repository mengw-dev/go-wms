import { expect, test, type Locator } from '@playwright/test'
import { loginByUi, requestApi, seedStockedInventory } from './support/api'
import type { WaveDetail } from '../../src/api/wave'

// The two branches meet here: real migrated MySQL, normal order allocation,
// browser wave commands, ordinary-pick isolation and final stock conservation.
for (const display of [
  { theme: 'light', width: 1366, height: 768, rule: false },
  { theme: 'dark', width: 390, height: 844, rule: true },
]) {
  test(`wave merge: ${display.theme} ${display.width} group, collect, sow and ship`, async ({ page, request }, testInfo) => {
    const seed = await seedStockedInventory(request, 20)
    const ids: string[] = []
    for (const qty of [3, 5]) {
      const order = await requestApi<{ id: string }>(request, seed.token, 'POST', '/outbound/orders', {
        warehouse_id: seed.warehouse.id, biz_order_no: `WAVE-${seed.suffix}-${qty}`,
        details: [{ sku_id: seed.sku.id, expected_qty: qty }],
      })
      ids.push(order.id)
      await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/submit`)
      await requestApi(request, seed.token, 'POST', `/outbound/orders/${order.id}/approve`)
    }
    await page.setViewportSize(display)
    await page.addInitScript(theme => localStorage.setItem('gowms_theme', theme), display.theme)
    await loginByUi(page)
    await page.goto('/outbound/waves')
    await page.getByRole('button', { name: '新建波次', exact: true }).click()
    const create = page.getByRole('dialog', { name: '新建波次', exact: true })
    await create.locator('.el-select').click()
    await page.getByRole('option', { name: `${seed.warehouse.code}（${seed.warehouse.name}）`, exact: true }).click()
    await create.getByRole('button', { name: display.rule ? '按创建时间选单' : '手动选择订单', exact: true }).click()
    await expect(create.locator('.el-table__body tbody tr')).toHaveCount(2)
    if (!display.rule) await create.locator('.el-table__header-wrapper .el-checkbox').click()
    await expect(create.getByText('已选择 2 单', { exact: true })).toBeVisible()
    await create.getByRole('button', { name: '确认创建波次', exact: true }).click()
    await expect(page).toHaveURL(/\/outbound\/waves\/\d+$/)
    const id = page.url().split('/').pop()!
    const detail = () => requestApi<WaveDetail>(request, seed.token, 'GET', `/outbound/waves/${id}`)
    const initial = await detail()
    expect(initial.orders.map(o => o.order_id).sort()).toEqual(ids.sort())
    expect(initial.lines).toHaveLength(1)
    expect(initial.lines[0]!.target_qty).toBe(8)
    const stock = () => requestApi<{ list: { id: string; stock_quantity: number; available_quantity: number; allocated_quantity: number }[] }>(
      request, seed.token, 'GET', `/inventory?warehouse_id=${seed.warehouse.id}&sku_id=${seed.sku.id}`)

    // Merged detail keeps the workbench entry and prevents the old pick UI.
    await page.goto(`/outbound/orders/${ids[0]}`)
    await expect(page.getByRole('button', { name: '进入波次工作台', exact: true })).toBeVisible()
    await expect(page.locator('.detail-actions').getByRole('button', { name: '拣货', exact: true })).toHaveCount(0)
    const rejected = await request.post(`/api/v1/outbound/tasks/${initial.items[0]!.task_id}/pick`, {
      headers: { Authorization: `Bearer ${seed.token}`, 'Idempotency-Key': `ordinary-${seed.suffix}` }, data: { qty: 1 },
    })
    expect((await rejected.json()).code).toBe(50408)
    await page.getByRole('button', { name: '进入波次工作台', exact: true }).click()
    await page.getByRole('button', { name: '领取波次', exact: true }).click()
    await expect(page.getByRole('button', { name: '续领波次', exact: true })).toBeEnabled()
    await expect(page.locator('.wave-grid').getByRole('button', { name: '拣货', exact: true })).toBeEnabled()
    await page.locator('.wave-grid').getByRole('button', { name: '拣货', exact: true }).click()
    const fill = async (dialog: Locator, label: string, value: string) => {
      await dialog.locator('.el-form-item').filter({ hasText: label }).locator('input').fill(value)
    }
    const collect = page.getByRole('dialog', { name: '登记汇总拣货', exact: true })
    await fill(collect, '货品编码或条码', seed.sku.barcode)
    await fill(collect, '来源库位', seed.location.code)
    await fill(collect, '批次（无批次留空）', seed.batchNo)
    const keys: string[] = []
    if (display.rule) {
      let lose = true
      await page.route(`**/api/v1/outbound/waves/${id}/lines/*/collect`, async route => {
        keys.push(route.request().headers()['idempotency-key']!)
        if (!lose) { await route.continue(); return }
        lose = false
        const response = await route.fetch()
        expect((await response.json()).code).toBe(0)
        await route.abort('failed')
      })
    }
    await collect.getByRole('button', { name: '确认登记', exact: true }).click()
    if (display.rule) {
      await expect(page.getByRole('button', { name: '恢复原操作', exact: true })).toBeVisible()
      await page.reload()
      await page.getByRole('button', { name: '恢复原操作', exact: true }).click()
      await expect(page.getByRole('button', { name: '恢复原操作', exact: true })).toHaveCount(0)
      expect(keys).toHaveLength(2)
      expect(keys[0]).toBeTruthy()
      expect(keys[1]).toBe(keys[0])
    }
    await expect(collect).not.toBeVisible()
    expect((await detail()).lines[0]!.picked_qty).toBe(8)
    expect((await stock()).list[0]).toMatchObject({ stock_quantity: 20, available_quantity: 12, allocated_quantity: 8 })
    await page.getByRole('tab', { name: '02 播种分货', exact: true }).click()
    for (const member of initial.orders) {
      const row = page.locator('.wave-grid tbody tr').filter({ hasText: member.slot_code })
      await row.getByRole('button', { name: '分货', exact: true }).click()
      const sort = page.getByRole('dialog', { name: '登记播种分货', exact: true })
      await fill(sort, '货品编码或条码', seed.sku.code)
      await fill(sort, '批次（无批次留空）', seed.batchNo)
      await fill(sort, '目标槽位', member.slot_code)
      await sort.getByRole('button', { name: '确认登记', exact: true }).click()
      await expect(sort).not.toBeVisible()
    }
    expect((await detail()).wave.status).toBe('READY')
    await page.getByRole('tab', { name: '03 确认发货', exact: true }).click()
    for (const member of initial.orders) {
      const slot = page.locator('.wave-slots article').filter({ hasText: member.slot_code })
      await slot.getByRole('button', { name: '确认发货', exact: true }).click()
      await page.locator('.el-message-box').getByRole('button', { name: '确定', exact: true }).click()
      await expect(slot).toContainText('已发货')
      const order = await requestApi<{ order: { status: string }; tasks: { done_qty: number; target_qty: number }[] }>(
        request, seed.token, 'GET', `/outbound/orders/${member.order_id}`)
      expect(order.order.status).toBe('SHIPPED')
      expect(order.tasks.every(t => t.done_qty === t.target_qty)).toBe(true)
    }
    expect((await detail()).wave.status).toBe('COMPLETED')
    expect((await stock()).list[0]).toMatchObject({ stock_quantity: 12, available_quantity: 12, allocated_quantity: 0 })
    const ledger = await requestApi<{ list: { quantity_change: number }[]; total: number }>(request, seed.token, 'GET',
      `/inventory/trans?inventory_id=${(await stock()).list[0]!.id}&trans_type=SHIP&page_size=100`)
    expect(ledger.total).toBe(2)
    expect(ledger.list.reduce((sum, r) => sum + r.quantity_change, 0)).toBe(-8)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath('completed.png'), fullPage: true })
    await page.emulateMedia({ media: 'print' })
    await expect(page.getByRole('heading', { name: '汇总拣货单（来源库位）', exact: true })).toBeVisible()
    await expect(page.locator('.wave-grid').first()).toContainText(seed.sku.name)
    await page.screenshot({ path: testInfo.outputPath('print.png'), fullPage: true })
  })
}
