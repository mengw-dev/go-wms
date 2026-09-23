import { expect, test } from '@playwright/test'
import { confirmMessageBox, loginByUi } from './support/api'

const demoUsername = process.env.E2E_DEMO_USERNAME || process.env.WMS_DEMO_USERNAME || 'demo1'
const demoPassword = process.env.E2E_DEMO_PASSWORD || process.env.WMS_DEMO_PASSWORD || 'demo123456'

test.afterEach(async ({ page }) => {
  try {
    const session = await page.evaluate(() => ({
      token: sessionStorage.getItem('WMS_TOKEN'),
      sessionId: sessionStorage.getItem('WMS_DEMO_SESSION'),
    }))
    if (!session.token || !session.sessionId) return
    await page.request.post('/api/v1/demo/session/release', {
      headers: {
        Authorization: `Bearer ${session.token}`,
        'X-Demo-Session': session.sessionId,
      },
    })
  } catch {
    // The page may already be closed; session expiry still releases the lock.
  }
})

test('demo quick controller runs the full flow, navigates from business pages and releases', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await expect(page).toHaveURL(/\/demo$/)

  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toBeVisible()
  await page.getByRole('button', { name: '跳过导览', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).not.toBeVisible()

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
  await expect(drawer.getByLabel('真实执行结果回放')).toBeVisible()
  await expect(drawer.getByText('创建入库单', { exact: true })).toBeVisible()
  await expect(drawer.getByText('状态变化').first()).toBeVisible()
  await expect(drawer.getByText('完成上架', { exact: true })).toBeVisible()
  await expect(drawer.getByText('库存入账', { exact: true })).toBeVisible()
  await expect(drawer.getByText('本次完整业务闭环产生', { exact: true })).toBeVisible()
  for (const label of ['查看入库单', '查看任务', '查看库存', '查看库存流水']) {
    await expect(drawer.getByRole('button', { name: label, exact: true }).first()).toBeVisible()
  }

  await expect(drawer.getByText('审核并 FIFO 分配库存', { exact: true })).toBeVisible()
  await expect(drawer.getByText('FIFO 1', { exact: true })).toBeVisible()
  await expect(drawer.getByText('FIFO 2', { exact: true })).toBeVisible()
  await expect(drawer.getByText(/B20260901.*本次分配 30/).first()).toBeVisible()
  await expect(drawer.getByText(/B20260905.*本次分配 20/).first()).toBeVisible()
  await expect(drawer.getByText('PICK 任务数量', { exact: true }).first()).toBeVisible()
  await expect(drawer.getByText('FIFO 分配', { exact: true })).toBeVisible()
  await expect(drawer.getByText('2 个批次 / 50 件', { exact: true })).toBeVisible()
  await expect(drawer.getByText('库存变化', { exact: true }).first()).toBeVisible()
  await expect(drawer.getByRole('button', { name: '查看出库单', exact: true })).toBeVisible()
  await expect(drawer.getByRole('button', { name: '查看拣货任务', exact: true })).toBeVisible()
  await expect(drawer.getByText('创建盘点', { exact: true })).toBeVisible()
  await expect(drawer.getByText('库存快照', { exact: true }).first()).toBeVisible()
  await expect(drawer.getByText('录入实盘', { exact: true })).toBeVisible()
  await expect(drawer.getByText('核对差异', { exact: true })).toBeVisible()
  await expect(drawer.getByText('审核盘点', { exact: true })).toBeVisible()
  await expect(drawer.getByText('库存调整入账', { exact: true })).toBeVisible()
  await expect(drawer.getByText('账面数量', { exact: true }).first()).toBeVisible()
  await expect(drawer.getByText('实盘数量', { exact: true }).first()).toBeVisible()
  await expect(drawer.getByText('盘点差异', { exact: true }).first()).toBeVisible()
  await expect(drawer.getByText('-3', { exact: true }).first()).toBeVisible()
  await expect(drawer.getByText('ADJUST -3', { exact: true }).first()).toBeVisible()

  await drawer.getByRole('button', { name: '查看出库单', exact: true }).click()
  await expect(page).toHaveURL(/\/outbound\/orders\/\d+/)
  await expect(page.locator('.detail-header .header-title')).toHaveText('出库单详情')

  await consoleButton.click()
  await expect(drawer).toBeVisible()

  await drawer.getByRole('button', { name: '查看入库单', exact: true }).click()
  await expect(page).toHaveURL(/\/inbound\/orders\/\d+/)
  await expect(page.locator('.detail-header .header-title')).toHaveText('入库单详情')
  await consoleButton.click()
  await expect(drawer).toBeVisible()

  await drawer.getByRole('button', { name: '查看盘点单', exact: true }).click()
  await expect(page).toHaveURL(/\/stocktake\/orders\/\d+/)
  await expect(page.locator('.detail-header .header-title')).toHaveText('盘点单详情')

  await consoleButton.click()
  await expect(drawer).toBeVisible()

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

test('demo tour is scoped to one demo session and can be reopened manually', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await expect(page).toHaveURL(/\/demo$/)

  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toBeVisible()
  await expect(page.getByRole('button', { name: '跳过导览', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(page.getByRole('heading', { name: '推荐业务闭环' })).toBeVisible()
  await page.getByRole('button', { name: '上一步', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toBeVisible()
  await page.getByRole('button', { name: '跳过导览', exact: true }).click()

  const sessionStorageKeys = await page.evaluate(() => Object.keys(sessionStorage))
  expect(sessionStorageKeys.some((key) => key.startsWith('WMS_DEMO_TOUR_SEEN:'))).toBe(true)

  await page.getByRole('button', { name: '亲自体验', exact: true }).click()
  await expect(page).toHaveURL(/\/inbound\/orders/)
  await page.getByRole('menuitem', { name: '体验中心', exact: true }).click()
  await expect(page).toHaveURL(/\/demo$/)
  await expect(page.getByRole('heading', { name: /从真实业务流程理解这套 WMS/ })).toBeVisible()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toHaveCount(0)

  await page.getByRole('button', { name: '快速导览', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toBeVisible()
  for (const title of ['推荐业务闭环', '结果与证据', '工程验证', '项目与源码']) {
    await page.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(page.getByRole('heading', { name: title })).toBeVisible()
  }
  await page.getByRole('button', { name: '开始体验完整业务闭环', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Demo 快捷控制器' })).toBeVisible()
})

test('demo account is logged out after a page reload', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await expect(page.getByRole('button', { name: /Demo 快捷控制器/ })).toBeVisible()

  await page.reload()

  await expect(page).toHaveURL(/\/login(?:\?|$)/)
})
