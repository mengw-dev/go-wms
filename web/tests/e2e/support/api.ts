import { expect, type APIRequestContext, type Locator, type Page } from '@playwright/test'

export const baseURL = process.env.E2E_BASE_URL || 'http://127.0.0.1:8081'

export const adminAccount = {
  username: 'admin',
  password: 'admin123',
}

type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE'

interface ApiEnvelope<T> {
  code: number
  msg: string
  data: T
}

interface PageData<T> {
  list: T[]
  total: number
}

interface WarehouseRecord {
  id: string
  code: string
  name: string
}

interface LocationRecord {
  id: string
  code: string
  warehouse_id: string
}

interface SkuRecord {
  id: string
  code: string
  name: string
  barcode: string
}

interface InboundOrderRecord {
  id: string
  order_no: string
}

interface InboundDetailRecord {
  id: string
  sku_id: string
  sku_code: string
  sku_name: string
  expected_qty: number
  received_qty: number
}

interface TaskRecord {
  id: string
  task_type: string
  target_qty: number
  done_qty: number
}

interface InboundDetailPayload {
  order: { status: string; warehouse_id: string }
  details: InboundDetailRecord[]
  tasks: TaskRecord[]
}

export interface SeedData {
  token: string
  suffix: string
  warehouse: WarehouseRecord
  location: LocationRecord
  sku: SkuRecord
}

export interface StockedSeedData extends SeedData {
  inboundOrder: InboundOrderRecord
  batchNo: string
}

export async function requestApi<T>(
  request: APIRequestContext,
  token: string | undefined,
  method: HttpMethod,
  path: string,
  data?: unknown,
): Promise<T> {
  const response = await request.fetch(`/api/v1${path}`, {
    method,
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    data,
  })
  const raw = await response.text()
  expect(response.ok(), `${method} ${path} -> HTTP ${response.status()}: ${raw}`).toBeTruthy()
  const body = JSON.parse(raw) as ApiEnvelope<T>
  expect(body.code, `${method} ${path} -> ${body.msg}`).toBe(0)
  return body.data
}

export async function loginByApi(request: APIRequestContext): Promise<string> {
  const data = await requestApi<{ token: string }>(
    request,
    undefined,
    'POST',
    '/login',
    adminAccount,
  )
  return data.token
}

export async function loginByUi(
  page: Page,
  username = adminAccount.username,
  password = adminAccount.password,
): Promise<void> {
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill(username)
  await page.getByPlaceholder('密码').fill(password)
  await page.getByRole('button', { name: /登\s*录/ }).click()
  await expect(page).not.toHaveURL(/\/login(?:\?|$)/)
  await expect(page.getByRole('heading', { name: /欢迎回来/ })).toBeVisible()
}

export function uniqueSuffix(): string {
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`.toUpperCase()
}

export async function seedBaseData(request: APIRequestContext): Promise<SeedData> {
  const token = await loginByApi(request)
  const suffix = uniqueSuffix()
  const warehouseCode = `E2E-WH-${suffix}`
  const warehouseName = `端到端测试仓 ${suffix}`

  await requestApi(request, token, 'POST', '/basic/warehouses', {
    code: warehouseCode,
    name: warehouseName,
    remark: `Playwright ${suffix}`,
  })
  const warehouses = await requestApi<PageData<WarehouseRecord>>(
    request,
    token,
    'GET',
    `/basic/warehouses?page=1&page_size=100&keyword=${encodeURIComponent(warehouseCode)}`,
  )
  const warehouse = warehouses.list.find((item) => item.code === warehouseCode)
  expect(warehouse, `warehouse ${warehouseCode} should exist`).toBeTruthy()

  const zone = `E2E${suffix.slice(-4)}`
  const locationCode = `${zone}-01-01`
  await requestApi(request, token, 'POST', '/basic/locations/batch', {
    warehouse_id: warehouse!.id,
    zone,
    row_from: 1,
    row_to: 1,
    col_from: 1,
    col_to: 1,
  })
  const locations = await requestApi<PageData<LocationRecord>>(
    request,
    token,
    'GET',
    `/basic/locations?page=1&page_size=100&warehouse_id=${warehouse!.id}&keyword=${encodeURIComponent(locationCode)}`,
  )
  const location = locations.list.find((item) => item.code === locationCode)
  expect(location, `location ${locationCode} should exist`).toBeTruthy()

  const skuCode = `E2E-SKU-${suffix}`
  const skuName = `端到端测试货品 ${suffix}`
  await requestApi(request, token, 'POST', '/basic/skus', {
    code: skuCode,
    barcode: `E2E-BAR-${suffix}`,
    name: skuName,
    spec: 'E2E',
    unit: '件',
  })
  const skus = await requestApi<PageData<SkuRecord>>(
    request,
    token,
    'GET',
    `/basic/skus?page=1&page_size=100&keyword=${encodeURIComponent(skuCode)}`,
  )
  const sku = skus.list.find((item) => item.code === skuCode)
  expect(sku, `sku ${skuCode} should exist`).toBeTruthy()

  return {
    token,
    suffix,
    warehouse: warehouse!,
    location: location!,
    sku: sku!,
  }
}

export async function seedStockedInventory(
  request: APIRequestContext,
  quantity = 1,
): Promise<StockedSeedData> {
  const seed = await seedBaseData(request)
  const batchNo = `E2E-BATCH-${seed.suffix}`
  const inboundOrder = await requestApi<InboundOrderRecord>(
    request,
    seed.token,
    'POST',
    '/inbound/orders',
    {
      warehouse_id: seed.warehouse.id,
      remark: `Playwright stock ${seed.suffix}`,
      details: [{ sku_id: seed.sku.id, expected_qty: quantity }],
    },
  )

  await requestApi(request, seed.token, 'POST', `/inbound/orders/${inboundOrder.id}/submit`)
  await requestApi(request, seed.token, 'POST', `/inbound/orders/${inboundOrder.id}/approve`)

  const beforeReceive = await requestApi<InboundDetailPayload>(
    request,
    seed.token,
    'GET',
    `/inbound/orders/${inboundOrder.id}`,
  )
  const detail = beforeReceive.details[0]
  expect(detail, 'inbound detail should exist').toBeTruthy()
  await requestApi(request, seed.token, 'POST', `/inbound/orders/${inboundOrder.id}/receive`, {
    detail_id: detail!.id,
    qty: quantity,
    defective_qty: 0,
    batch_no: batchNo,
  })

  const beforePutaway = await requestApi<InboundDetailPayload>(
    request,
    seed.token,
    'GET',
    `/inbound/orders/${inboundOrder.id}`,
  )
  const task = beforePutaway.tasks.find(
    (item) => item.task_type === 'PUTAWAY' && item.done_qty < item.target_qty,
  )
  expect(task, 'putaway task should exist').toBeTruthy()
  await requestApi(request, seed.token, 'POST', `/inbound/tasks/${task!.id}/putaway`, {
    task_id: task!.id,
    location_id: seed.location.id,
    qty: task!.target_qty - task!.done_qty,
  })

  return { ...seed, inboundOrder, batchNo }
}

export async function selectOption(
  page: Page,
  dialog: Locator,
  placeholder: string,
  optionName: string,
): Promise<void> {
  const select = dialog.locator('.el-select').filter({ hasText: placeholder }).first()
  await select.locator('.el-select__wrapper').click()
  await page.getByRole('option', { name: optionName, exact: true }).click()
  await page.keyboard.press('Escape')
  await expect(page.locator('.el-select-dropdown:visible')).toHaveCount(0)
}

export async function confirmMessageBox(page: Page, buttonName = '确定'): Promise<void> {
  const messageBox = page.locator('.el-message-box')
  await expect(messageBox).toBeVisible()
  await messageBox.getByRole('button', { name: buttonName }).click()
}
