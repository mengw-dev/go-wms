import { expect, test } from '@playwright/test'
import { STAGED_EXECUTION_STEPS, validStagedSnapshot } from '../../src/utils/stagedDemoState'

const user = { user_id: '9001', username: 'demo1', nickname: '演示访客1', roles: ['demo'], perms: ['*', 'wms:demo'] }

function completedRun() {
  const now = new Date().toISOString()
  const results = STAGED_EXECUTION_STEPS.map(step => ({ ...step, objectNo: 'RK202610200000001', status: 'COMPLETED',
    before: '待执行', after: '已完成', quantity: '20 件', createdAt: now, route: '/inventory',
    facts: Array.from({ length: 10 }, (_, i) => ({ label: `核对项 ${i + 1}`, value: '完整业务记录' })) }))
  const kind: Record<string, string> = { 'inbound-receive': 'receive', 'inbound-putaway': 'putaway', 'outbound-allocate': 'outbound-approve', 'outbound-pick': 'pick' }
  const writes = STAGED_EXECUTION_STEPS.filter(step => step.key !== 'inventory-check').map((step, i) => {
    const k = kind[step.key] || step.key
    const payload = k.endsWith('create') ? { warehouse_id: '1', remark: '演示记录', biz_order_no: 'ERP1', details: [{ sku_id: '1', expected_qty: 20 }] }
      : k === 'receive' ? { detail_id: '1', qty: 20, defective_qty: 0, batch_no: 'B1' }
      : k === 'putaway' ? { location_id: '1', qty: 20 }
      : k === 'pick' ? { qty: 20, location_code: 'A01', batch_no: 'B1' } : {}
    return { step: step.key, name: k === 'pick' ? 'pick:1' : k === 'outbound-approve' ? 'outbound-allocate' : k,
      kind: k, objectId: k.endsWith('create') ? '' : '1', payload, status: 'done',
      response: { id: '1', order_no: 'RK202610200000001' }, operationKey: `00000000-0000-4000-8000-${String(i).padStart(12, '0')}` }
  })
  const snapshot = { version: 1, runId: '11111111-1111-4111-8111-111111111111', mode: 'auto', scope: 'full', startedAt: now,
    completedAt: now, index: results.length, results, inboundID: '1', inboundNo: 'RK1', outboundID: '2', outboundNo: 'CK2',
    warehouse: { id: '1', code: 'WH01' }, sku: { id: '1', code: 'SKU1' }, writes }
  expect(validStagedSnapshot(snapshot)).toBe(true)
  return snapshot
}

for (const theme of ['light', 'dark']) {
  test(`${theme}: automatic result details stay local and result actions remain visible`, async ({ page }) => {
    await page.setViewportSize({ width: 1366, height: 650 })
    await page.addInitScript(({ user, theme }) => {
      sessionStorage.setItem('WMS_TOKEN', 'auto-result-token')
      sessionStorage.setItem('WMS_USER', JSON.stringify(user))
      sessionStorage.setItem('WMS_DEMO_SESSION', 'auto-result-session')
      localStorage.setItem('gowms_theme', theme)
    }, { user, theme })
    await page.route('**/api/v1/**', async route => {
      const path = new URL(route.request().url()).pathname
      const data = path.endsWith('/profile') ? user : path.includes('/demo/session/') ? { session_id: 'auto-result-session', expires_in: 300 }
        : path.endsWith('/demo/run/full') ? { name: 'full', status: 'completed', summary: '真实业务执行结果',
          evidence: ['入库单', '出库单', '作业任务', 'FIFO 分配', '库存流水', '库存变化'].map(label => ({ label, value: '已记录' })),
          steps: Array.from({ length: 10 }, (_, i) => ({ title: `业务步骤 ${i + 1}`, status: 'completed', detail: '可追溯的业务处理说明。'.repeat(150) })) }
        : path.endsWith('/demo/activity') ? { operations: [], inbound_orders: [], outbound_orders: [], stocktake_orders: [], tasks: [], inventory_trans: [] }
        : { list: [], total: 0 }
      await route.fulfill({ json: { code: 0, msg: 'ok', data } })
    })
    await page.goto('/demo')
    await expect(page.locator('.home-dock')).toBeVisible()
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('wms:run-demo-scenario', { detail: { scenario: 'full' } })))
    const dialog = page.getByRole('dialog', { name: '自动演示结果', exact: true })
    await expect(dialog).toBeInViewport({ ratio: 1 })
    const scroll = dialog.locator('.stage-step-list')
    expect(await scroll.evaluate(el => el.scrollHeight - el.clientHeight)).toBeGreaterThan(50)
    await scroll.evaluate(el => { el.scrollTop = el.scrollHeight })
    await expect(dialog.getByRole('button', { name: '查看业务操作记录', exact: true })).toBeInViewport()
    await expect(dialog.locator('.run-footer')).toBeInViewport()
    expect(await dialog.locator('..').evaluate(el => el.scrollHeight - el.clientHeight)).toBeLessThanOrEqual(1)
  })

  test(`${theme}: completed demo records scroll inside a viewport-sized dialog`, async ({ page }, testInfo) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.setViewportSize({ width: 1366, height: 650 })
    await page.addInitScript(({ user, theme, snapshot }) => {
      sessionStorage.setItem('WMS_TOKEN', 'demo-scroll-token')
      sessionStorage.setItem('WMS_USER', JSON.stringify(user))
      sessionStorage.setItem('WMS_DEMO_SESSION', 'scroll-session')
      sessionStorage.setItem('WMS_STAGED_DEMO_V1:9001:scroll-session', JSON.stringify(snapshot))
      localStorage.setItem('gowms_theme', theme)
    }, { user, theme, snapshot: completedRun() })
    await page.route('**/api/v1/**', async route => {
      const path = new URL(route.request().url()).pathname
      const data = path.endsWith('/profile') ? user : path.includes('/demo/session/') ? { session_id: 'scroll-session', expires_in: 300 }
        : path.endsWith('/demo/activity') ? { operations: [], inbound_orders: [], outbound_orders: [], stocktake_orders: [], tasks: [], inventory_trans: [] }
        : { list: [], total: 0 }
      await route.fulfill({ json: { code: 0, msg: 'ok', data } })
    })
    await page.goto('/demo')
    await page.locator('.home-dock').getByRole('button', { name: '演示控制', exact: true }).click()
    await page.getByRole('button', { name: '查看分步结果', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '分步执行演示', exact: true })
    await expect(dialog.getByText('分步流程已完成', { exact: true })).toBeVisible()
    for (const size of [{ width: 1366, height: 650 }, { width: 1920, height: 914 }]) {
      await page.setViewportSize(size)
      await expect(dialog).toBeInViewport({ ratio: 1 })
      const log = dialog.locator('.executed-list')
      expect(await log.evaluate(el => el.scrollHeight - el.clientHeight)).toBeGreaterThan(20)
      await log.evaluate(el => { el.scrollTop = el.scrollHeight })
      await expect(log.getByText('完成拣货与发货', { exact: true })).toBeInViewport()
      await expect(dialog.getByRole('button', { name: '打开真实页面', exact: true })).toBeInViewport()
      await expect(dialog.getByRole('button', { name: '业务操作记录', exact: true })).toBeInViewport()
      expect(await dialog.locator('..').evaluate(el => el.scrollHeight - el.clientHeight)).toBeLessThanOrEqual(1)
    }
    await page.screenshot({ path: testInfo.outputPath(`demo-scroll-${theme}.png`) })
    await page.setViewportSize({ width: 390, height: 700 })
    await expect(dialog.getByRole('button', { name: '关闭此对话框' })).toBeInViewport()
    expect(await dialog.evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
    expect(errors).toEqual([])
  })
}
