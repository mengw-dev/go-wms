import { expect, test, type Locator, type Page } from '@playwright/test'
import { confirmMessageBox, loginByUi, selectOption } from './support/api'

const demoUsername = process.env.E2E_DEMO_USERNAME || process.env.WMS_DEMO_USERNAME || 'demo1'
const demoPassword = process.env.E2E_DEMO_PASSWORD || process.env.WMS_DEMO_PASSWORD || 'demo123456'

function drawer(page: Page): Locator {
  return page.locator('.demo-console-drawer')
}

function resultViewer(page: Page): Locator {
  return page.getByLabel('真实业务执行结果')
}

async function loginDemo(page: Page): Promise<void> {
  await loginByUi(page, demoUsername, demoPassword, /WMS 业务闭环/)
  await expect(page).toHaveURL(/demo$/)
}

async function startScenarioFromHome(
  page: Page,
  scenario: 'inbound' | 'outbound' | 'stocktake' | 'full',
  cardTitle?: string,
): Promise<Locator> {
  const expectedPaths = scenario === 'full'
    ? ['/api/v1/demo/run/inbound', '/api/v1/demo/run/outbound']
    : [`/api/v1/demo/run/${scenario}`]
  const responsePromises = expectedPaths.map((path) =>
    page.waitForResponse(
      (response) => response.request().method() === 'POST' && new URL(response.url()).pathname === path,
    ),
  )

  if (scenario === 'full') {
    await page.getByRole('button', { name: '开始自动演示', exact: true }).click()
  } else {
    const step = page.locator('.flow-step').filter({ hasText: cardTitle || '' }).first()
    await step.click()
    await page.locator('.detail-panel').getByRole('button', { name: '自动演示', exact: true }).click()
  }

  for (const response of await Promise.all(responsePromises)) {
    expect(response.ok()).toBeTruthy()
  }
  const viewer = resultViewer(page)
  await expect(viewer).toBeVisible({ timeout: 60_000 })
  return viewer
}

async function closeResultDialog(page: Page): Promise<void> {
  const dialog = page.locator('.demo-result-dialog')
  await expect(dialog).toBeVisible()
  await dialog.locator('.el-dialog__headerbtn').click()
  await expect(resultViewer(page)).toHaveCount(0)
}

type ApiEnvelope<T> = { code: number; msg: string; data: T }
type PageData<T> = { list: T[]; total: number }
interface PickingExperimentResult {
  experiment_order_count: number
  experiment_order_nos: string[]
  task_count: number
  completed_tasks: number
  shipped_orders: number
}
interface WarehouseRow { id: string; code: string }
interface SkuRow { id: string; code: string }
interface OutboundOrderRow { id: string; order_no: string }
interface TaskRow { id: string; task_type: string; status: string; done_qty: number }

async function demoHeaders(page: Page) {
  const session = await page.evaluate(() => ({
    token: sessionStorage.getItem('WMS_TOKEN'),
    sessionId: sessionStorage.getItem('WMS_DEMO_SESSION'),
  }))
  return {
    Authorization: `Bearer ${session.token}`,
    'X-Demo-Session': session.sessionId || '',
  }
}

async function requestDemoJson<T>(page: Page, method: 'get' | 'post', path: string, data?: unknown): Promise<T> {
  const response = await page.request[method](`/api/v1${path}`, {
    headers: await demoHeaders(page),
    data,
  })
  expect(response.ok()).toBeTruthy()
  const body = (await response.json()) as ApiEnvelope<T>
  expect(body.code).toBe(0)
  return body.data
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

test('demo home is a concise one-screen launcher', async ({ page }) => {
  await loginDemo(page)

  await expect(page.getByRole('heading', { name: 'WMS 业务闭环', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '开始自动演示', exact: true })).toHaveCount(1)
  await expect(page.getByRole('button', { name: '手动体验', exact: true })).toHaveCount(1)
  await expect(page.locator('.flow-step')).toHaveCount(5)
  await expect(page.locator('.engineering-card')).toHaveCount(3)
  await expect(page.getByText('库存盘点', { exact: true })).toHaveCount(0)
  await expect(page.locator('.deep-links .el-button')).toHaveCount(2)
  await expect(page.getByRole('button', { name: '打开演示控制' })).toHaveCount(0)

  const metrics = await page.locator('.main').evaluate((element) => ({
    scrollHeight: element.scrollHeight,
    clientHeight: element.clientHeight,
  }))
  expect(metrics.scrollHeight).toBeLessThanOrEqual(metrics.clientHeight)
})

test('full demo opens a three-column result panel without stocktake', async ({ page }) => {
  await loginDemo(page)
  const viewer = await startScenarioFromHome(page, 'full')

  await expect(viewer.getByRole('heading', { name: '完整业务闭环已完成', exact: true })).toBeVisible()
  await expect(viewer.locator('.stage-item')).toHaveCount(5)
  await expect(viewer.locator('.summary-strip > div')).toHaveCount(6)
  await expect(viewer.getByText('盘点', { exact: true })).toHaveCount(0)
  await expect(viewer.getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await expect(viewer.getByRole('button', { name: '下一步', exact: true })).toBeVisible()
  await expect(viewer.getByRole('button', { name: '打开真实页面', exact: true })).toBeVisible()

  await closeResultDialog(page)
  const statusbar = page.locator('.demo-statusbar')
  await expect(statusbar).toBeVisible()
  await statusbar.getByRole('button', { name: '演示控制', exact: true }).click()
  const demoDrawer = drawer(page)
  await expect(demoDrawer).toBeVisible()
  await demoDrawer.getByRole('button', { name: '查看结果', exact: true }).click()
  await expect(resultViewer(page)).toBeVisible()

  await resultViewer(page).getByRole('button', { name: '查看操作记录', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/activity/)
  await expect(page.getByRole('heading', { name: '本次业务执行证据', exact: true })).toBeVisible()
  await expect(page.locator('.summary-card > div')).toHaveCount(6)
  await expect(page.getByRole('tab', { name: '业务对象', exact: true })).toBeVisible()
  await expect(page.getByRole('tab', { name: '库存流水', exact: true })).toBeVisible()
  await expect(page.getByRole('tab', { name: '操作记录', exact: true })).toBeVisible()
  await expect(page.getByText('盘点', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '详情', exact: true }).first().click()
  const detailDrawer = page.getByRole('dialog', { name: /详情$/ }).last()
  await expect(detailDrawer).toBeVisible()
  await expect(detailDrawer.locator('.el-descriptions')).toBeVisible()
})

test('home auto-demo CTAs call their own scenario APIs', async ({ page }) => {
  await loginDemo(page)
  const scenarios = [
    { key: 'inbound', card: '入库单', summary: /入库单 .* 已完成/ },
    { key: 'outbound', card: '出库单', summary: /出库单 .* 已按 FIFO/ },
  ] as const

  for (const item of scenarios) {
    await page.goto('/demo')
    const viewer = await startScenarioFromHome(page, item.key, item.card)
    await expect(viewer.getByText(item.summary).first()).toBeVisible({ timeout: 60_000 })
    await closeResultDialog(page)
  }
})

test('manual inbound guide completes through inventory evidence', async ({ page }) => {
  await loginDemo(page)

  await page.locator('.flow-step').filter({ hasText: '入库单' }).click()
  await page.locator('.detail-panel').getByRole('button', { name: '进入入库页面', exact: true }).click()
  await expect(page).toHaveURL(/\/inbound\/orders$/)

  const guide = page.locator('section[aria-label="手动业务引导"]')
  await expect(guide.getByText('第 1 / 7 步', { exact: true })).toBeVisible()
  await expect(page.locator('.guide-highlight')).toHaveCount(1)
  await expect(page.locator('.demo-statusbar')).toBeVisible()

  await page.getByRole('button', { name: '查看操作记录', exact: true }).click()
  const guideRecords = page.getByRole('dialog', { name: '本次操作记录' })
  await expect(guideRecords).toBeVisible()
  await expect(page.locator('.guide-bubble')).toHaveCount(0)
  await guideRecords.getByRole('button', { name: '关闭此对话框' }).click()
  await expect(page.locator('.guide-bubble')).toBeVisible()

  await page.locator('[data-tour="inbound-create"]').click()
  const createDialog = page.getByRole('dialog', { name: '新建入库单' })
  await selectOption(page, createDialog, '选择仓库', 'WH01（华东一号仓）')
  await selectOption(page, createDialog, '选择货品', 'SKU000001 农夫山泉饮用天然水')
  await createDialog.locator('.el-input-number input').first().fill('2')
  await createDialog.getByPlaceholder('备注（可选）').fill('Manual guided inbound E2E')
  await createDialog.getByRole('button', { name: '保存' }).click()
  await expect(createDialog).not.toBeVisible()
  await expect(page).toHaveURL(/\/inbound\/orders\/\d+$/)
  await expect(guide.getByText('第 2 / 7 步', { exact: true })).toBeVisible()
  await expect(guide.getByText(/已创建入库单/)).toBeVisible()

  await page.locator('[data-tour="inbound-submit"]').click()
  await confirmMessageBox(page)
  await expect(guide.getByText('第 3 / 7 步', { exact: true })).toBeVisible()
  await expect(guide.getByText(/提交完成/)).toBeVisible()

  await page.locator('[data-tour="inbound-approve"]').click()
  await confirmMessageBox(page)
  await expect(guide.getByText('第 4 / 7 步', { exact: true })).toBeVisible()
  await expect(guide.getByText(/审核完成/)).toBeVisible()

  await page.locator('[data-tour="inbound-receive"]').click()
  const receiveDialog = page.getByRole('dialog', { name: '收货' })
  await receiveDialog.getByPlaceholder('首次收货必填').fill('GUIDE-BATCH-E2E')
  await receiveDialog.getByRole('button', { name: '收货' }).click()
  await expect(page.getByText('收货成功')).toBeVisible()
  await receiveDialog.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(guide.getByText('第 6 / 7 步', { exact: true })).toBeVisible()
  await expect(guide.getByText('完成上架', { exact: true })).toBeVisible()

  await page.locator('[data-tour="inbound-putaway"]').first().click()
  const putawayDialog = page.getByRole('dialog', { name: '上架' })
  await selectOption(page, putawayDialog, '选择空闲库位', 'A01-02-02')
  await putawayDialog.getByRole('button', { name: '确定上架' }).click()
  await expect(putawayDialog).not.toBeVisible()

  await expect(page).toHaveURL(/\/inventory\?order_no=/)
  const inventoryDrawer = page.getByRole('dialog', { name: '库存流水' })
  await expect(inventoryDrawer).toBeVisible()
  await inventoryDrawer.getByRole('button', { name: '关闭此对话框' }).click()
  await expect(inventoryDrawer).not.toBeVisible()

  const completed = page.locator('section[aria-label="手动业务引导完成"]')
  await expect(completed.getByText('手动流程已完成', { exact: true })).toBeVisible()
  for (const label of ['查看业务证据', '查看业务对象', '返回演示中心', '重新开始', '退出引导']) {
    await expect(completed.getByRole('button', { name: label, exact: true })).toBeVisible()
  }
  await expect(completed.getByRole('button', { name: '查看技术实现', exact: true })).toHaveCount(0)

  const orderFact = await completed.locator('.guide-complete-facts span').filter({ hasText: '入库单：' }).textContent()
  const orderNo = orderFact?.split('：').at(-1)?.trim() || ''
  expect(orderNo).toBeTruthy()
  await completed.getByRole('button', { name: '查看业务证据', exact: true }).click()
  await expect(page).toHaveURL(/\/demo\/activity\?/)
  await expect(page).toHaveURL(/source=manual/)
  await expect(page).toHaveURL(/order_id=/)
  await expect(page).toHaveURL(new RegExp(`order_no=${encodeURIComponent(orderNo)}`))
  await expect(page.getByRole('heading', { name: '本次业务执行证据', exact: true })).toBeVisible()
  await expect(page.getByText('入库手动体验', { exact: true })).toBeVisible()
  await expect(page.getByText(orderNo, { exact: true }).first()).toBeVisible()
})

test('engineering verification exposes three compact experiments without separate shortage section', async ({ page }) => {
  await loginDemo(page)
  await page.goto('/demo/performance')

  await expect(page.getByRole('heading', { name: '工程验证', exact: true })).toBeVisible()
  await expect(page.locator('.engineering-card')).toHaveCount(3)
  for (const section of ['allocation', 'picking', 'import']) {
    await expect(page.locator(`[data-section="${section}"]`)).toBeVisible()
  }
  await expect(page.locator('[data-section="shortage"]')).toHaveCount(0)
  const order = await page.locator('.engineering-card').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-section')))
  expect(order).toEqual(['allocation', 'picking', 'import'])

  const metrics = await page.locator('.main').evaluate((element) => ({
    scrollHeight: element.scrollHeight,
    clientHeight: element.clientHeight,
  }))
  expect(metrics.scrollHeight).toBeLessThanOrEqual(metrics.clientHeight)

  const allocationCard = page.locator('[data-section="allocation"]')
  await allocationCard.getByRole('button', { name: '库存充足模式', exact: true }).click()
  const allocationDialog = page.getByRole('dialog', { name: '配置并发库存分配' })
  await expect(allocationDialog.getByText('库存充足', { exact: true })).toBeVisible()
  await expect(allocationDialog.getByText('供给不足', { exact: true })).toBeVisible()
  await expect(allocationDialog.getByText('初始库存目标', { exact: true })).toBeVisible()
  await allocationDialog.getByRole('button', { name: '关闭', exact: true }).click()

  const pickingCard = page.locator('[data-section="picking"]')
  await pickingCard.getByRole('button', { name: '配置并运行', exact: true }).click()
  const pickingDialog = page.getByRole('dialog', { name: '配置拣货作业验证' })
  await expect(pickingDialog.getByText('模拟重复扫码', { exact: true })).toBeVisible()
  await pickingDialog.getByRole('button', { name: '关闭', exact: true }).click()

  const importCard = page.locator('[data-section="import"]')
  await importCard.getByRole('button', { name: '查看机制', exact: true }).click()
  const mechanismDialog = page.getByRole('dialog', { name: '异步导入可靠性机制' })
  await expect(mechanismDialog.getByText('当前页面只展示实现机制，不使用静态成功数据冒充真实实验。')).toBeVisible()
  await mechanismDialog.getByRole('button', { name: '关闭', exact: true }).click()

  await page.getByRole('button', { name: '运行状态', exact: true }).click()
  const runtimeDrawer = page.getByRole('dialog', { name: '运行状态与指标快照' })
  await expect(runtimeDrawer).toBeVisible()
  await runtimeDrawer.getByRole('button', { name: '关闭此对话框' }).click()

  await page.getByRole('button', { name: '返回演示中心', exact: true }).click()
  await expect(page).toHaveURL(/\/demo$/)
})

test('demo session stays valid across refresh and page navigation', async ({ page }) => {
  await loginDemo(page)
  const sessionBefore = await page.evaluate(() => sessionStorage.getItem('WMS_DEMO_SESSION'))
  expect(sessionBefore).toBeTruthy()
  await expect(page.locator('.demo-statusbar')).toBeVisible()

  await page.goto('/inbound/orders')
  await expect(page.locator('.demo-statusbar')).toBeVisible()
  expect(await page.evaluate(() => sessionStorage.getItem('WMS_DEMO_SESSION'))).toBe(sessionBefore)

  await page.goto('/demo/activity')
  await expect(page.locator('.demo-statusbar')).toBeVisible()
  expect(await page.evaluate(() => sessionStorage.getItem('WMS_DEMO_SESSION'))).toBe(sessionBefore)

  await page.goto('/demo')
  const heartbeat = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/v1/demo/session/heartbeat',
  )
  await page.reload()
  await heartbeat
  await expect(page).toHaveURL(/\/demo$/)
  await expect(page.getByRole('heading', { name: 'WMS 业务闭环', exact: true })).toBeVisible()
  const sessionAfter = await page.evaluate(() => sessionStorage.getItem('WMS_DEMO_SESSION'))
  expect(sessionAfter).toBe(sessionBefore)
})

test('PDA experiment prepares its own tasks on a clean demo and isolates consecutive runs', async ({ page }) => {
  await loginDemo(page)
  await page.goto('/demo/performance')
  await page.locator('[data-section="picking"]').getByRole('button', { name: '配置并运行', exact: true }).click()
  const pickingDialog = page.getByRole('dialog', { name: '配置拣货作业验证' })

  const runExperiment = async (): Promise<PickingExperimentResult> => {
    const responsePromise = page.waitForResponse(
      (response) =>
        response.request().method() === 'POST' &&
        new URL(response.url()).pathname === '/api/v1/demo/run/picking',
    )
    await pickingDialog.getByRole('button', { name: '运行模拟 PDA 实验' }).click()
    const response = await responsePromise
    expect(response.ok()).toBeTruthy()
    const body = (await response.json()) as ApiEnvelope<PickingExperimentResult>
    expect(body.code).toBe(0)
    return body.data
  }

  const first = await runExperiment()
  expect(first.experiment_order_count).toBe(3)
  expect(first.task_count).toBeGreaterThan(0)
  expect(first.completed_tasks).toBe(first.task_count)
  expect(first.shipped_orders).toBe(3)

  const second = await runExperiment()
  expect(second.experiment_order_count).toBe(3)
  expect(second.task_count).toBeGreaterThan(0)
  expect(second.experiment_order_nos.some((orderNo) => first.experiment_order_nos.includes(orderNo))).toBe(false)
})

test('PDA experiment does not modify an unrelated pending pick task', async ({ page }) => {
  await loginDemo(page)

  const warehouses = await requestDemoJson<PageData<WarehouseRow>>(page, 'get', '/basic/warehouses?page=1&page_size=100&keyword=WH01')
  const skus = await requestDemoJson<PageData<SkuRow>>(page, 'get', '/basic/skus?page=1&page_size=100&keyword=SKU000001')
  const warehouse = warehouses.list.find((item) => item.code === 'WH01')
  const sku = skus.list.find((item) => item.code === 'SKU000001')
  expect(warehouse).toBeTruthy()
  expect(sku).toBeTruthy()

  const order = await requestDemoJson<OutboundOrderRow>(page, 'post', '/outbound/orders', {
    warehouse_id: warehouse!.id,
    biz_order_no: `PDA-GUARD-${Date.now()}`,
    remark: 'PDA unrelated pending task guard',
    details: [{ sku_id: sku!.id, expected_qty: 1 }],
  })
  await requestDemoJson<void>(page, 'post', `/outbound/orders/${order.id}/submit`)
  await requestDemoJson<void>(page, 'post', `/outbound/orders/${order.id}/approve`)

  const beforePage = await requestDemoJson<PageData<TaskRow>>(page, 'get', `/tasks?page=1&page_size=20&order_id=${order.id}&task_type=PICK`)
  const unrelated = beforePage.list.find((item) => item.task_type === 'PICK')
  expect(unrelated).toBeTruthy()

  await page.goto('/demo/performance')
  await page.locator('[data-section="picking"]').getByRole('button', { name: '配置并运行', exact: true }).click()
  const pickingDialog = page.getByRole('dialog', { name: '配置拣货作业验证' })
  const responsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/v1/demo/run/picking',
  )
  await pickingDialog.getByRole('button', { name: '运行模拟 PDA 实验' }).click()
  await responsePromise

  const afterPage = await requestDemoJson<PageData<TaskRow>>(page, 'get', `/tasks?page=1&page_size=20&order_id=${order.id}&task_type=PICK`)
  const unchanged = afterPage.list.find((item) => item.id === unrelated!.id)
  expect(unchanged).toBeTruthy()
  expect(unchanged!.done_qty).toBe(unrelated!.done_qty)
  expect(unchanged!.status).toBe(unrelated!.status)
})