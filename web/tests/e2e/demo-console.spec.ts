import { expect, test } from '@playwright/test'
import { confirmMessageBox, loginByUi } from './support/api'

const demoUsername = process.env.E2E_DEMO_USERNAME || process.env.WMS_DEMO_USERNAME || 'demo'
const demoPassword = process.env.E2E_DEMO_PASSWORD || process.env.WMS_DEMO_PASSWORD || 'demo123456'

test('demo console runs the complete business flow, resets and releases the session', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword)

  await expect(page.getByRole('button', { name: /演示控制台/ })).toBeVisible()
  await expect(page.getByRole('dialog', { name: 'WMS 业务流程演示' })).toBeVisible()

  await page.getByRole('button', { name: '一键完整流程演示' }).click()
  await expect(page.getByRole('dialog', { name: 'WMS 业务流程演示' }).getByText('入库、出库、盘点三个核心流程已全部完成')).toBeVisible({ timeout: 60_000 })

  await page.getByRole('button', { name: '并发业务演示' }).click()
  await expect(page.getByRole('dialog', { name: 'WMS 业务流程演示' }).getByText(/并发演示完成/)).toBeVisible({ timeout: 30_000 })

  await page.getByRole('button', { name: '性能指标', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/performance/)
  await expect(page.getByRole('heading', { name: '系统性能指标' })).toBeVisible()
  await expect(page.getByText('MySQL 状态')).toBeVisible()

  await page.getByRole('main').getByRole('button', { name: '操作记录', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/activity/)
  await expect(page.getByRole('heading', { name: '演示操作记录' })).toBeVisible()

  await page.getByRole('button', { name: /演示控制台/ }).click()
  await expect(page.getByRole('dialog', { name: 'WMS 业务流程演示' })).toBeVisible()
  await page.getByRole('button', { name: '重置数据', exact: true }).click()
  await confirmMessageBox(page, '确定重置')
  await expect(page.getByText('演示数据已恢复为初始状态')).toBeVisible()

  await page.getByRole('button', { name: '退出并重置', exact: true }).click()
  await confirmMessageBox(page, '退出并重置')
  await expect(page).toHaveURL(/\/login(?:\?|$)/)
})