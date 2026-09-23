import { expect, test } from '@playwright/test'
import { confirmMessageBox, loginByUi } from './support/api'

const demoUsername = process.env.E2E_DEMO_USERNAME || process.env.WMS_DEMO_USERNAME || 'demo1'
const demoPassword = process.env.E2E_DEMO_PASSWORD || process.env.WMS_DEMO_PASSWORD || 'demo123456'

test('demo quick controller runs the full flow, navigates from business pages and releases', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await expect(page).toHaveURL(/\/demo$/)

  const consoleButton = page.getByRole('button', { name: /Demo 快捷控制器/ })
  const drawer = page.getByRole('dialog', { name: 'Demo 快捷控制器' })
  await expect(consoleButton).toBeVisible()
  await expect(drawer).not.toBeVisible()

  await page.getByRole('button', { name: '开始完整演示', exact: true }).click()
  await expect(drawer).toBeVisible()
  await expect(drawer.getByText('独立演示租户')).toBeVisible()
  await expect(drawer.getByText('当前运行')).toBeVisible()
  await expect(drawer.getByRole('button', { name: '完整业务闭环', exact: true })).toBeVisible()

  await drawer.getByRole('button', { name: '完整业务闭环', exact: true }).click()
  await expect(drawer.getByText('入库、出库、盘点三个核心流程已全部完成')).toBeVisible({ timeout: 60_000 })

  await drawer.getByRole('button', { name: '查看业务证据', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/activity/)
  await expect(page.getByRole('heading', { name: '业务操作记录' })).toBeVisible()

  await consoleButton.click()
  await expect(drawer).toBeVisible()
  await drawer.getByRole('button', { name: '返回演示中心', exact: true }).click()
  await expect(page).toHaveURL(/\/demo$/)

  await page.getByRole('button', { name: '亲自体验', exact: true }).click()
  await expect(page).toHaveURL(/\/inbound\/orders/)
  await consoleButton.click()
  await expect(drawer).toBeVisible()

  await drawer.getByRole('button', { name: '工程验证', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/performance/)
  await expect(page.getByRole('heading', { name: '运行状态与指标快照' })).toBeVisible()
  await expect(page.getByText('MySQL 状态')).toBeVisible()

  await consoleButton.click()
  await expect(drawer).toBeVisible()
  await drawer.getByRole('button', { name: '重置数据', exact: true }).click()
  await confirmMessageBox(page, '确定重置')
  await expect(page.getByText('演示数据已恢复为初始状态')).toBeVisible()

  await drawer.getByRole('button', { name: '退出 Demo', exact: true }).click()
  await confirmMessageBox(page, '退出并重置')
  await expect(page).toHaveURL(/\/login(?:\?|$)/)
})

test('demo account is logged out after a page reload', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await expect(page.getByRole('button', { name: /Demo 快捷控制器/ })).toBeVisible()

  await page.reload()

  await expect(page).toHaveURL(/\/login(?:\?|$)/)
})
