import { expect, test } from '@playwright/test'
import { loginByApi, loginByUi, requestApi, uniqueSuffix } from './support/api'

test('user manager assigns permitted roles without role-management permission', async ({ page, request }) => {
  const suffix = uniqueSuffix().toLowerCase()
  const admin = await loginByApi(request)
  const managerName = `mgr_${suffix}`
  const childName = `child_${suffix}`
  const roleName = `basic_${suffix}`
  const managerRole = `manager_${suffix}`
  const password = 'test-password-123'
  const users: string[] = []
  const roles: string[] = []
  try {
    for (const [name, perms] of [[roleName, 'wms:basic'], [managerRole, 'wms:system:user,wms:basic']]) {
      await requestApi(request, admin, 'POST', '/system/roles', { name, perms })
      const result = await requestApi<{ list: { id: string; name: string }[] }>(request, admin, 'GET', `/system/roles?keyword=${name}&page=1&page_size=100`)
      const role = result.list.find(item => item.name === name)
      expect(role).toBeTruthy()
      roles.push(role!.id)
    }
    await requestApi(request, admin, 'POST', '/system/users', { username: managerName, password, role_ids: [roles[1]] })
    const managers = await requestApi<{ list: { id: string; username: string }[] }>(request, admin, 'GET', `/system/users?keyword=${managerName}&page=1&page_size=100`)
    users.push(managers.list.find(item => item.username === managerName)!.id)
    await loginByUi(page, managerName, password)
    const options = page.waitForResponse(response => response.url().includes('/system/users/role-options'))
    await page.goto('/system/users')
    expect((await options).status()).toBe(200)
    await page.getByRole('button', { name: '新增用户', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByPlaceholder('登录账号').fill(childName)
    await dialog.getByPlaceholder('6-32 位', { exact: true }).fill(password)
    await dialog.locator('.el-select').click()
    await expect(page.getByRole('option', { name: 'admin', exact: true })).toHaveCount(0)
    await page.getByRole('option', { name: roleName, exact: true }).click()
    await dialog.getByPlaceholder('显示名称').click()
    const created = page.waitForResponse(response => response.url().endsWith('/system/users') && response.request().method() === 'POST')
    await dialog.getByRole('button', { name: '确定', exact: true }).click()
    expect((await (await created).json()).code).toBe(0)
    await expect(dialog).not.toBeVisible()
    const result = await requestApi<{ list: { id: string; username: string; role_ids: string[] }[] }>(request, admin, 'GET', `/system/users?keyword=${childName}&page=1&page_size=100`)
    const child = result.list.find(item => item.username === childName)
    expect(child?.role_ids).toEqual([roles[0]])
    users.push(child!.id)
    const managerToken = await loginByApi(request, managerName, password)
    const forged = await request.put(`/api/v1/system/users/${child!.id}`, {
      headers: { Authorization: `Bearer ${managerToken}` }, data: { expected_version: 1, role_ids: ['1'] },
    })
    expect(forged.status()).toBe(403)
    expect((await forged.json()).code).toBe(40300)
  } finally {
    // Only entities created by this test, in the explicitly isolated E2E stack.
    for (const id of users.reverse()) await requestApi(request, admin, 'DELETE', `/system/users/${id}`)
    for (const id of roles.reverse()) await requestApi(request, admin, 'DELETE', `/system/roles/${id}`)
  }
})
