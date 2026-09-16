import { expect, test, type Locator, type Page } from '@playwright/test'
import {
  confirmMessageBox,
  loginByUi,
  seedBaseData,
  seedStockedInventory,
  selectOption,
} from './support/api'

function tableRow(page: Page, text: string): Locator {
  return page.getByRole('row').filter({ hasText: text }).first()
}

function dialog(page: Page, name: string): Locator {
  return page.getByRole('dialog', { name })
}

test('inbound UI flow creates stock and inventory transaction', async ({ page, request }) => {
  const seed = await seedBaseData(request)
  const remark = `E2E inbound ${seed.suffix}`
  const warehouseLabel = `${seed.warehouse.code}（${seed.warehouse.name}）`
  const skuLabel = `${seed.sku.code} ${seed.sku.name}`

  await loginByUi(page)
  await page.goto('/inbound/orders')
  await page.getByRole('button', { name: '新建入库单' }).click()

  const formDialog = dialog(page, '新建入库单')
  await selectOption(page, formDialog, '选择仓库', warehouseLabel)
  await selectOption(page, formDialog, '选择货品', skuLabel)
  await formDialog.getByPlaceholder('备注（可选）').fill(remark)
  await formDialog.locator('.el-input-number input').first().fill('3')
  await formDialog.getByRole('button', { name: '保存' }).click()
  await expect(formDialog).not.toBeVisible()

  let row = tableRow(page, remark)
  await expect(row).toContainText('草稿')
  await row.getByRole('button', { name: '提交' }).click()
  await confirmMessageBox(page)
  await expect(row).toContainText('已提交')

  await row.getByRole('button', { name: '审核' }).click()
  await confirmMessageBox(page)
  await expect(row.getByRole('button', { name: '收货' })).toBeVisible()

  await row.getByRole('button', { name: '收货' }).click()
  const receiveDialog = dialog(page, '收货')
  await receiveDialog.getByPlaceholder('首次收货必填').fill(`E2E-BATCH-${seed.suffix}`)
  await receiveDialog.getByRole('button', { name: '收货' }).click()
  await expect(page.getByText('收货成功')).toBeVisible()
  await receiveDialog.getByRole('button', { name: '关闭', exact: true }).click()

  row = tableRow(page, remark)
  await expect(row.getByRole('button', { name: '上架' })).toBeVisible()
  await row.getByRole('button', { name: '上架' }).click()
  const putawayDialog = dialog(page, '上架')
  await selectOption(page, putawayDialog, '选择空闲库位', seed.location.code)
  await putawayDialog.getByRole('button', { name: '确定上架' }).click()
  await expect(putawayDialog).not.toBeVisible()
  await expect(row).toContainText('已完成')

  await page.goto('/inventory')
  await page.getByPlaceholder('编码/名称/条码').fill(seed.sku.code)
  await page.getByRole('button', { name: '查询' }).first().click()
  const inventoryRow = tableRow(page, seed.sku.name)
  await expect(inventoryRow).toBeVisible()
  await expect(inventoryRow.locator('td').nth(4)).toHaveText('3')
  await expect(inventoryRow.locator('td').nth(5)).toHaveText('3')

  await inventoryRow.getByRole('button', { name: '流水' }).click()
  const drawer = dialog(page, '库存流水')
  await expect(drawer.getByText('入库', { exact: true })).toBeVisible()
})

test('outbound UI flow allocates FIFO stock and ships it', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 2)
  const bizOrderNo = `E2E-OUT-${seed.suffix}`
  const warehouseLabel = `${seed.warehouse.code}（${seed.warehouse.name}）`
  const skuLabel = `${seed.sku.code} ${seed.sku.name}`

  await loginByUi(page)
  await page.goto('/outbound/orders')
  await page.getByRole('button', { name: '新建出库单' }).click()

  const formDialog = dialog(page, '新建出库单')
  await selectOption(page, formDialog, '选择仓库', warehouseLabel)
  await formDialog.getByPlaceholder('业务订单号（幂等键，重复将创建失败）').fill(bizOrderNo)
  await selectOption(page, formDialog, '选择货品', skuLabel)
  await formDialog.locator('.el-input-number input').first().fill('2')
  await formDialog.getByRole('button', { name: '保存' }).click()
  await expect(formDialog).not.toBeVisible()

  const row = tableRow(page, bizOrderNo)
  await expect(row).toContainText('草稿')
  await row.getByRole('button', { name: '提交' }).click()
  await confirmMessageBox(page)
  await expect(row).toContainText('已提交')

  await row.getByRole('button', { name: '审核' }).click()
  await confirmMessageBox(page)
  await expect(row.getByRole('button', { name: '拣货' })).toBeVisible()

  await row.getByRole('button', { name: '拣货' }).click()
  const pickDialog = dialog(page, '拣货')
  await pickDialog.getByRole('button', { name: '确定拣货' }).click()
  await expect(page.getByText('拣货成功')).toBeVisible()
  await expect(row).toContainText('已发货')

  await page.goto('/inventory')
  await page.getByPlaceholder('编码/名称/条码').fill(seed.sku.code)
  await page.getByRole('button', { name: '查询' }).first().click()
  const inventoryRow = tableRow(page, seed.sku.name)
  await expect(inventoryRow).toBeVisible()
  await expect(inventoryRow.locator('td').nth(4)).toHaveText('0')
  await expect(inventoryRow.locator('td').nth(5)).toHaveText('0')
})

test('stocktake UI flow adjusts inventory by the counted difference', async ({ page, request }) => {
  const seed = await seedStockedInventory(request, 2)
  const remark = `E2E stocktake ${seed.suffix}`
  const warehouseLabel = `${seed.warehouse.code}（${seed.warehouse.name}）`

  await loginByUi(page)
  await page.goto('/stocktake/orders')
  await page.getByRole('button', { name: '新建盘点单' }).click()

  const formDialog = dialog(page, '新建盘点单')
  await selectOption(page, formDialog, '选择仓库', warehouseLabel)
  await selectOption(page, formDialog, '不选则为整仓盘点', seed.location.code)
  await formDialog.getByPlaceholder('备注（可选）').fill(remark)
  await formDialog.getByRole('button', { name: '创建' }).click()
  await expect(formDialog).not.toBeVisible()

  const row = tableRow(page, remark)
  await row.getByRole('button', { name: '录入实盘' }).click()
  await expect(page).toHaveURL(/\/stocktake\/orders\/.+/)
  await page.locator('.el-input-number input').first().fill('1')
  await page.getByRole('button', { name: '保存' }).first().click()
  await expect(page.getByText('实盘数已保存')).toBeVisible()

  await page.getByRole('button', { name: '审核' }).first().click()
  await confirmMessageBox(page, '确认审核')
  await expect(page.getByText('审核完成')).toBeVisible()

  await page.goto('/inventory')
  await page.getByPlaceholder('编码/名称/条码').fill(seed.sku.code)
  await page.getByRole('button', { name: '查询' }).first().click()
  const inventoryRow = tableRow(page, seed.sku.name)
  await expect(inventoryRow).toBeVisible()
  await expect(inventoryRow.locator('td').nth(5)).toHaveText('1')
})
