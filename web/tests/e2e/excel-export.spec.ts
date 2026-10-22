import { readFile } from 'node:fs/promises'
import { expect, test, type Page } from '@playwright/test'

const mime='application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
// Transport fixture only; actual XLSX structure and cell types are verified by Go round-trip tests.
const payload=Buffer.from('PK\u0003\u0004transport-fixture')
const date='2026-10-22T02:00:00Z'
const user={user_id:'42',username:'reader',nickname:'查看员',roles:[],perms:['wms:inventory','wms:stocktake:view']}
async function fixture(page: Page,theme='light') {
  await page.addInitScript(({user,theme})=>{
    localStorage.setItem('WMS_TOKEN','export-fixture');localStorage.setItem('WMS_USER',JSON.stringify(user));localStorage.setItem('gowms_theme',theme)
  },{user,theme})
  await page.route('**/api/v1/**',async route=>{
    expect(route.request().method()).toBe('GET')
    const path=new URL(route.request().url()).pathname.replace('/api/v1','')
    if(path.endsWith('/export')) {await route.fulfill({contentType:mime,body:payload});return}
    let data: unknown
    if(path==='/profile') data=user
    else if(path==='/version') data={version:'fixture',demo_enabled:false}
    else if(path==='/inventory') data={list:[{id:'9',warehouse_id:'1',location_id:'2',sku_id:'3',batch_no:'B001',stock_quantity:10,available_quantity:8,allocated_quantity:2,stock_in_time:date,location_code:'A-01'}],total:103}
    else if(path==='/inventory/trans') data={list:[],total:0}
    else if(path==='/stocktake/orders') data={list:[{id:'1',warehouse_id:'1',order_no:'PD001',status:'DRAFT',created_at:date}],total:1}
    else if(path==='/stocktake/orders/1') data={order:{id:'1',warehouse_id:'1',order_no:'PD001',status:'DRAFT',created_at:date},details:[]}
    else throw Error('unexpected export fixture API '+path)
    await route.fulfill({json:{code:0,msg:'ok',data}})
  })
}
async function download(page: Page,button: ReturnType<Page['getByRole']>,path: string) {
  const request=page.waitForRequest(req=>new URL(req.url()).pathname.endsWith(path))
  const received=page.waitForEvent('download')
  await button.click()
  const [req,file]=await Promise.all([request,received])
  expect(req.headers().authorization).toBe('Bearer export-fixture')
  expect(new URL(req.url()).searchParams.has('page')).toBeFalsy()
  expect(new URL(req.url()).searchParams.has('page_size')).toBeFalsy()
  expect(file.suggestedFilename()).toMatch(/\.xlsx$/)
  const saved=await file.path();expect(saved).toBeTruthy()
  expect(await readFile(saved!)).toEqual(payload)
  return new URL(req.url()).searchParams
}

for(const theme of ['light','dark']) {
  test(`${theme}: 库存按筛选下载全部结果，保留false`,async({page})=>{
    await page.setViewportSize({width:1366,height:900});await fixture(page,theme)
    await page.goto('/inventory?warehouse_id=1&sku_keyword=000123&in_stock_only=false')
    const params=await download(page,page.getByRole('button',{name:'导出Excel',exact:true}).first(),'/inventory/export')
    expect(params.get('warehouse_id')).toBe('1');expect(params.get('sku_keyword')).toBe('000123');expect(params.get('in_stock_only')).toBe('false')
    await expect(page.getByText('文件已生成，请查看浏览器下载记录')).toBeVisible()
    await page.screenshot({path:test.info().outputPath(`export-${theme}.png`)})
  })
  test(`${theme}: 流水下载携带当前库存行`,async({page})=>{
    await fixture(page,theme);await page.goto('/inventory')
    await page.getByRole('button',{name:'流水',exact:true}).first().click()
    const dialog=page.getByRole('dialog',{name:'库存流水',exact:true})
    const params=await download(page,dialog.getByRole('button',{name:'导出Excel',exact:true}),'/inventory/trans/export')
    expect(params.get('inventory_id')).toBe('9')
  })
  test(`${theme}: 只读盘点差异与单据全明细入口`,async({page})=>{
    await fixture(page,theme);await page.goto('/stocktake/orders?warehouse_id=1&status=DRAFT')
    await expect(page.getByRole('button',{name:'新建盘点单',exact:true})).toHaveCount(0)
    await expect(page.getByRole('button',{name:'审核',exact:true})).toHaveCount(0)
    await expect(page.getByRole('button',{name:'取消',exact:true})).toHaveCount(0)
    const params=await download(page,page.getByRole('button',{name:'导出盘点差异',exact:true}),'/stocktake/orders/export')
    expect(params.get('warehouse_id')).toBe('1');expect(params.get('status')).toBe('DRAFT');expect(params.get('differences_only')).toBe('true')
    await page.getByRole('button',{name:'详情',exact:true}).first().click()
    await expect(page.getByRole('button',{name:'审核',exact:true})).toHaveCount(0)
    const detail=await download(page,page.getByRole('button',{name:'导出盘点明细',exact:true}),'/stocktake/orders/export')
    expect(detail.get('order_id')).toBe('1');expect(detail.has('differences_only')).toBeFalsy()
  })
  test(`${theme}: 手机导出入口不造成整页横向溢出`,async({page})=>{
    await page.setViewportSize({width:390,height:844});await fixture(page,theme);await page.goto('/stocktake/orders')
    await expect(page.getByRole('button',{name:'导出盘点差异',exact:true})).toBeInViewport()
    expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1)
    await page.screenshot({path:test.info().outputPath(`export-mobile-${theme}.png`)})
  })
}

for(const status of [200,400]) {
  test(`HTTP${status} JSON错误不能另存为Excel`,async({page})=>{
    await fixture(page);let downloads=0;page.on('download',()=>downloads++)
    await page.route('**/api/v1/inventory/export?**',route=>route.fulfill({status,json:{code:400,msg:'导出超过10000行，请缩小筛选范围后重试'}}))
    await page.goto('/inventory');await page.getByRole('button',{name:'导出Excel',exact:true}).first().click()
    await expect(page.getByText('导出超过10000行，请缩小筛选范围后重试')).toBeVisible()
    expect(downloads).toBe(0)
  })
}

test('文件请求401沿用登录失效处理',async({page})=>{
  await fixture(page)
  await page.route('**/api/v1/inventory/export?**',route=>route.fulfill({status:401,json:{code:40100,msg:'登录已过期'}}))
  await page.goto('/inventory');await page.getByRole('button',{name:'导出Excel',exact:true}).first().click()
  await expect(page).toHaveURL(/\/login/)
  expect(await page.evaluate(()=>localStorage.getItem('WMS_TOKEN'))).toBeNull()
})

test('离开详情取消在途下载，不保存旧文件或弹出旧错误',async({page})=>{
  await fixture(page);let downloads=0;page.on('download',()=>downloads++)
  let release!:()=>void
  const gate=new Promise<void>(resolve=>{release=resolve})
  await page.route('**/api/v1/stocktake/orders/export?**',async route=>{await gate;await route.fulfill({contentType:mime,body:payload})})
  await page.goto('/stocktake/orders');await page.getByRole('button',{name:'详情',exact:true}).first().click()
  const started=page.waitForRequest(req=>req.url().includes('/orders/export?'))
  await page.getByRole('button',{name:'导出盘点明细',exact:true}).click();await started
  const cancelled=page.waitForEvent('requestfailed',req=>req.url().includes('/orders/export?'))
  await page.goBack();await cancelled;release()
  await expect(page).toHaveURL(/\/stocktake\/orders$/)
  await expect(page.getByRole('button',{name:'导出盘点差异',exact:true})).toBeEnabled()
  expect(downloads).toBe(0)
  await expect(page.getByText('导出失败，请稍后重试')).toHaveCount(0)
})
