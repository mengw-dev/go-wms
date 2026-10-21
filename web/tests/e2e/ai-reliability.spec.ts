import { expect, test, type Page, type Route } from '@playwright/test'

async function setup(page: Page, theme = 'light') {
  const user = { user_id: '42', username: 'operator', nickname: '仓库管理员', roles: [], perms: ['*'] }
  await page.addInitScript(({ user, theme }) => {
    localStorage.setItem('WMS_TOKEN', 'ai-test')
    localStorage.setItem('WMS_USER', JSON.stringify(user))
    localStorage.setItem('gowms_theme', theme)
  }, { user, theme })
  const calls: Route[] = []
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/ai/chat')) { calls.push(route); return }
    const data = path.endsWith('/profile') ? user : path.endsWith('/dashboard/overview')
      ? { generated_at: '2026-10-21T09:00:00+08:00', today_from: '2026-10-21 00:00:00', access: {}, warehouses: [] }
      : { list: [], total: 0 }
    await route.fulfill({ json: { code: 0, msg: 'ok', data } })
  })
  return calls
}

for (const theme of ['light', 'dark']) {
  test(`${theme}: stopping AI cancels the request and a later answer cannot replace a new question`, async ({ page }, testInfo) => {
    const calls = await setup(page, theme)
    const failures: string[] = []
    page.on('requestfailed', req => { if (req.url().endsWith('/ai/chat')) failures.push(req.failure()?.errorText || '') })
    await page.setViewportSize({ width: 390, height: 740 })
    await page.goto('/ai')
    await page.getByRole('textbox').fill('第一次问题')
    await page.getByRole('button', { name: '发送', exact: true }).click()
    await expect.poll(() => calls.length).toBe(1)
    for (const width of [320, 390]) {
      await page.setViewportSize({ width, height: 740 })
      await expect(page.getByRole('button', { name: '停止等待', exact: true })).toBeInViewport()
      expect(await page.locator('.main').evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
    }
    await page.screenshot({ path: testInfo.outputPath(`ai-waiting-${theme}.png`) })
    await page.getByRole('button', { name: '停止等待', exact: true }).click()
    await expect.poll(() => failures.length).toBe(1)
    await page.getByRole('textbox').fill('第二次问题')
    await page.getByRole('button', { name: '发送', exact: true }).click()
    await expect.poll(() => calls.length).toBe(2)
    await calls[1]!.fulfill({ json: { code: 0, msg: 'ok', data: { answer: '新的回答' } } })
    await calls[0]!.fulfill({ json: { code: 0, msg: 'ok', data: { answer: '过时回答' } } }).catch(() => undefined)
    await expect(page.locator('.msg-list')).toContainText('新的回答')
    await expect(page.locator('.msg-list')).not.toContainText('过时回答')
    await expect(page.locator('.msg-list')).not.toContainText('回答失败')
    expect(await page.locator('.main').evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
  })
}

test('leaving the AI page aborts the in-flight request', async ({ page }) => {
  const calls = await setup(page)
  let aborted = false
  page.on('requestfailed', req => { if (req.url().endsWith('/ai/chat')) aborted = true })
  await page.goto('/ai')
  await page.getByRole('textbox').fill('库存情况')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect.poll(() => calls.length).toBe(1)
  await page.getByRole('menuitem', { name: '仪表盘', exact: true }).click()
  await expect.poll(() => aborted).toBe(true)
  await expect(page).toHaveURL(/\/dashboard$/)
})

test('AI quota errors retain the server explanation in the conversation', async ({ page }) => {
  const calls = await setup(page)
  await page.goto('/ai')
  await page.getByRole('textbox').fill('库存情况')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect.poll(() => calls.length).toBe(1)
  await calls[0]!.fulfill({ status: 429, json: { code: 80004, msg: '今日 AI 问答额度已用完，请明天再试' } })
  await expect(page.locator('.msg-list')).toContainText('今日 AI 问答额度已用完，请明天再试')
  await expect(page.getByRole('button', { name: '停止等待', exact: true })).toHaveCount(0)
})
