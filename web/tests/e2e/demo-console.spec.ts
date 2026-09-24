import { expect, test, type Locator, type Page } from '@playwright/test'
import { confirmMessageBox, loginByUi, selectOption } from './support/api'

const demoUsername = process.env.E2E_DEMO_USERNAME || process.env.WMS_DEMO_USERNAME || 'demo1'
const demoPassword = process.env.E2E_DEMO_PASSWORD || process.env.WMS_DEMO_PASSWORD || 'demo123456'

function drawer(page: Page): Locator {
  return page.getByRole('dialog', { name: '演示快捷入口' })
}

async function skipTour(page: Page): Promise<void> {
  const skip = page.getByRole('button', { name: '跳过导览', exact: true })
  if (await skip.isVisible().catch(() => false)) await skip.click()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toHaveCount(0)
}

async function startScenarioFromHome(
  page: Page,
  scenario: 'inbound' | 'outbound' | 'stocktake' | 'full',
  cardTitle?: string,
): Promise<void> {
  const responsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === `/api/v1/demo/run/${scenario}`,
  )
  if (scenario === 'full') {
    await page.getByRole('button', { name: '自动演示完整业务闭环', exact: true }).click()
  } else {
    const card = page.locator('.scenario-card').filter({ hasText: cardTitle || '' })
    await card.getByRole('button', { name: '自动演示', exact: true }).click()
  }
  const response = await responsePromise
  expect(response.ok()).toBeTruthy()
  await expect(drawer(page)).toBeVisible()
}

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

test('full demo runs from the home CTA and exposes replayable business evidence', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await expect(page).toHaveURL(/demo$/)
  await skipTour(page)

  await startScenarioFromHome(page, 'full')
  const demoDrawer = drawer(page)
  await expect(demoDrawer.getByText('入库、出库、盘点三个核心流程已全部完成')).toBeVisible({ timeout: 60_000 })
  await expect(demoDrawer.getByLabel('真实执行结果回放')).toBeVisible()
  await expect(demoDrawer.getByText('当前回放步骤', { exact: true })).toBeVisible()
  await expect(demoDrawer.getByRole('list').getByText('创建入库单', { exact: true })).toBeVisible()
  await expect(demoDrawer.getByText('本次完整业务闭环产生', { exact: true })).toBeVisible({ timeout: 20_000 })
  await expect(demoDrawer.getByText('FIFO 批次分配', { exact: true })).toBeVisible()
  await expect(demoDrawer.getByText('数量关系', { exact: true })).toBeVisible()
  for (const label of ['查看入库单', '查看任务', '查看库存', '查看库存流水']) {
    await expect(demoDrawer.getByRole('button', { name: label, exact: true }).first()).toBeVisible()
  }

  const consoleButton = page.getByRole('button', { name: /演示快捷入口/ })
  await demoDrawer.locator('.el-drawer__close-btn').click()
  await expect(demoDrawer).not.toBeVisible()
  await consoleButton.click()
  await expect(demoDrawer).toBeVisible()
  await expect(demoDrawer.getByRole('button', { name: '返回演示中心', exact: true })).toBeVisible()

  await demoDrawer.getByRole('button', { name: '查看业务证据', exact: true }).last().click()
  await expect(page).toHaveURL(/\/demo\/activity/)
  await expect(page.getByRole('heading', { name: '本次业务执行证据' })).toBeVisible()
  await expect(page.getByText('本次演示', { exact: true })).toBeVisible()
  await expect(page.getByText('业务证据', { exact: true }).first()).toBeVisible()
})

test('demo tour supports next, skip and final direct execution', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await expect(page).toHaveURL(/\/demo$/)
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toBeVisible()

  await page.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(page.getByRole('heading', { name: '推荐业务闭环' })).toBeVisible()
  await page.getByRole('button', { name: '上一步', exact: true }).click()
  await page.getByRole('button', { name: '跳过导览', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).not.toBeVisible()

  await page.getByRole('button', { name: '快速导览', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎体验 WMS' })).toBeVisible()
  for (const title of ['推荐业务闭环', '结果与证据', '工程验证', '项目与源码']) {
    await page.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(page.getByRole('heading', { name: title })).toBeVisible()
  }

  const responsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/v1/demo/run/full',
  )
  await page.getByRole('button', { name: '开始体验完整业务闭环', exact: true }).click()
  await responsePromise
  await expect(drawer(page)).toBeVisible()
  await expect(drawer(page).getByLabel('真实执行结果回放')).toBeVisible()
})

test('home auto-demo CTAs call their own scenario APIs', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await skipTour(page)

  const scenarios = [
    { key: 'inbound', card: '批量入库', summary: /入库单 .* 已完成/ },
    { key: 'outbound', card: '上游出库', summary: /出库单 .* 已按 FIFO/ },
    { key: 'stocktake', card: '库存盘点', summary: /盘点单 .* 已完成/ },
  ] as const

  for (const item of scenarios) {
    await page.goto('/demo')
    await startScenarioFromHome(page, item.key, item.card)
    await expect(drawer(page).getByText(item.summary).first()).toBeVisible({ timeout: 60_000 })
    await drawer(page).locator('.el-drawer__close-btn').click()
    await expect(drawer(page)).not.toBeVisible()
  }
})

test('manual inbound guide completes through inventory evidence', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await skipTour(page)

  await page.getByRole('button', { name: '亲自体验入库', exact: true }).first().click()
  await expect(page).toHaveURL(/\/inbound\/orders$/)
  const guide = page.locator('section[aria-label="手动业务引导"]')
  await expect(guide.getByText('第 1 / 7 步', { exact: true })).toBeVisible()

  await page.locator('[data-tour="inbound-create"]').click()
  const createDialog = page.getByRole('dialog', { name: '新建入库单' })
  await selectOption(page, createDialog, '选择仓库', 'WH01（华东一号仓）')
  await selectOption(page, createDialog, '选择货品', 'SKU000001 农夫山泉饮用天然水')
  await createDialog.locator('.el-input-number input').first().fill('2')
  await createDialog.getByPlaceholder('备注（可选）').fill('Manual guided inbound E2E')
  await createDialog.getByRole('button', { name: '保存' }).click()
  await expect(createDialog).not.toBeVisible()
  await expect(guide.getByText(/已创建入库单/)).toBeVisible()
  await guide.getByRole('button', { name: '下一步' }).click()
  await expect(page).toHaveURL(/\/inbound\/orders\/\d+$/)

  await page.locator('[data-tour="inbound-submit"]').click()
  await confirmMessageBox(page)
  await expect(guide.getByText(/提交完成/)).toBeVisible()
  await guide.getByRole('button', { name: '下一步' }).click()

  await page.locator('[data-tour="inbound-approve"]').click()
  await confirmMessageBox(page)
  await expect(guide.getByText(/审核完成/)).toBeVisible()
  await guide.getByRole('button', { name: '下一步' }).click()

  await page.locator('[data-tour="inbound-receive"]').click()
  const receiveDialog = page.getByRole('dialog', { name: '收货' })
  await receiveDialog.getByPlaceholder('首次收货必填').fill('GUIDE-BATCH-E2E')
  await receiveDialog.getByRole('button', { name: '收货' }).click()
  await expect(page.getByText('收货成功')).toBeVisible()
  await receiveDialog.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(guide.getByText(/已生成上架任务/)).toBeVisible()
  await guide.getByRole('button', { name: '下一步' }).click()
  await expect(guide.getByText('查看上架任务', { exact: true })).toBeVisible()
  await guide.getByRole('button', { name: '下一步' }).click()

  await page.locator('[data-tour="inbound-putaway"]').first().click()
  const putawayDialog = page.getByRole('dialog', { name: '上架' })
  await selectOption(page, putawayDialog, '选择空闲库位', 'A01-02-02')
  await putawayDialog.getByRole('button', { name: '确定上架' }).click()
  await expect(putawayDialog).not.toBeVisible()
  await expect(guide.getByText(/库存已在真实业务事务中增加/)).toBeVisible()
  await guide.getByRole('button', { name: '下一步' }).click()

  await expect(page).toHaveURL(/\/inventory\?order_no=/)
  await expect(guide.getByText(/已查看入库单/)).toBeVisible()
  await guide.getByRole('button', { name: '下一步' }).click()
  const completed = page.locator('section[aria-label="手动业务引导完成"]')
  await expect(completed.getByText('你刚刚亲自完成', { exact: true })).toBeVisible()
  await expect(completed.getByRole('button', { name: '查看业务证据', exact: true })).toBeVisible()
  await expect(completed.getByRole('button', { name: '查看技术实现', exact: true })).toBeVisible()
  await expect(completed.getByRole('button', { name: '返回 Demo', exact: true })).toBeVisible()
})

test('engineering verification cards open their corresponding experiment sections', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await skipTour(page)

  for (const card of ['并发库存分配一致性', '供给不足并发验证', '模拟 PDA 并发拣货']) {
    await page.locator('.verify-card').filter({ hasText: card }).getByRole('button', { name: '打开实验' }).click()
    await expect(page).toHaveURL(/\/demo\/performance/)
    await expect(page.getByRole('heading', { name: card, exact: true })).toBeVisible()
    await page.goto('/demo')
  }
})

test('demo session stays valid after refreshing the demo home', async ({ page }) => {
  await loginByUi(page, demoUsername, demoPassword, /从真实业务流程理解这套 WMS/)
  await skipTour(page)
  const sessionBefore = await page.evaluate(() => sessionStorage.getItem('WMS_DEMO_SESSION'))
  expect(sessionBefore).toBeTruthy()

  const heartbeat = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/v1/demo/session/heartbeat',
  )
  await page.reload()
  await heartbeat
  await expect(page).toHaveURL(/\/demo$/)
  await expect(page.getByRole('heading', { name: /从真实业务流程理解这套 WMS/ })).toBeVisible()
  const sessionAfter = await page.evaluate(() => sessionStorage.getItem('WMS_DEMO_SESSION'))
  expect(sessionAfter).toBe(sessionBefore)
})
