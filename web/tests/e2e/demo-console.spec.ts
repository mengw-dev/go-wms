import { expect, test } from '@playwright/test'
import { confirmMessageBox, loginByUi } from './support/api'

const demoUsername = process.env.E2E_DEMO_USERNAME || process.env.WMS_DEMO_USERNAME || 'demo1'
const demoPassword = process.env.E2E_DEMO_PASSWORD || process.env.WMS_DEMO_PASSWORD || 'demo123456'

test('business center creates drafts, runs flows, shows metrics and records, then releases', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await expect(page).toHaveURL(/\/demo$/)

  const consoleButton = page.getByRole('button', { name: /业务流程中心/ })
  const dialog = page.getByRole('dialog', { name: '业务流程中心' })
  await expect(consoleButton).toBeVisible()
  await expect(dialog).not.toBeVisible()
  await page.getByRole('button', { name: '开始完整演示', exact: true }).click()
  await expect(dialog).toBeVisible()

  await dialog.getByRole('button', { name: '模拟 Excel 批量入库' }).click()
  await expect(dialog.getByText(/已创建 3 张入库草稿/)).toBeVisible()
  await dialog.getByRole('button', { name: '前往入库单处理' }).click()
  await expect(page).toHaveURL(/\/inbound\/orders/)

  await consoleButton.click()
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: '一键完整流程' }).click()
  await expect(dialog.getByText('入库、出库、盘点三个核心流程已全部完成')).toBeVisible({ timeout: 60_000 })

  await dialog.getByRole('button', { name: '一键补货入库' }).click()
  await expect(dialog.getByText(/一键补货完成：已通过完整入库流程补充 500 件库存/)).toBeVisible({ timeout: 30_000 })

  await dialog.getByRole('button', { name: '并发出库审核分配' }).click()
  await expect(dialog.getByText(/并发出库审核分配完成/)).toBeVisible({ timeout: 30_000 })

  await dialog.getByRole('button', { name: 'PDA 并发拣货' }).click()
  await expect(dialog.getByText(/PDA 并发拣货完成/)).toBeVisible({ timeout: 60_000 })

  await dialog.getByRole('button', { name: '运行状态与指标', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/performance/)
  await expect(page.getByRole('heading', { name: '运行状态与指标快照' })).toBeVisible()
  await expect(page.getByText('MySQL 状态')).toBeVisible()

  await page.getByRole('main').getByRole('button', { name: '操作记录', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/activity/)
  await expect(page.getByRole('heading', { name: '业务操作记录' })).toBeVisible()

  await page.getByRole('button', { name: /业务流程中心/ }).click()
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: '重置数据', exact: true }).click()
  await confirmMessageBox(page, '确定重置')
  await expect(page.getByText('演示数据已恢复为初始状态')).toBeVisible()

  await dialog.getByRole('button', { name: '退出并重置', exact: true }).click()
  await confirmMessageBox(page, '退出并重置')
  await expect(page).toHaveURL(/\/login(?:\?|$)/)
})

test('demo account is logged out after a page reload', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await expect(page.getByRole('button', { name: /业务流程中心/ })).toBeVisible()

  await page.reload()

  await expect(page).toHaveURL(/\/login(?:\?|$)/)
})
