import { expect, test, type Page } from '@playwright/test'

const user = { user_id:'42',username:'reader',nickname:'库存查看员',roles:[],perms:['wms:inventory'] }
const date='2026-10-21T10:00:00+08:00'
async function fixture(page: Page,theme: string) {
  await page.addInitScript(({user,theme})=>{ localStorage.setItem('WMS_TOKEN','alert-ui-fixture');localStorage.setItem('WMS_USER',JSON.stringify(user));localStorage.setItem('gowms_theme',theme) },{user,theme})
  await page.route('**/api/v1/**',async route=>{
    expect(route.request().method()).toBe('GET')
    const url=new URL(route.request().url()),path=url.pathname.replace('/api/v1','')
    let data: unknown
    if(path==='/profile') data=user
    else if(path==='/version') data={version:'fixture',demo_enabled:false}
    else if(path==='/basic/warehouses') data={list:[{id:'1',code:'WH1',name:'测试仓',status:1}],total:1}
    else if(path==='/basic/skus') data={list:[{id:'2',code:'SKU001',name:'测试货品',barcode:'00123',unit:'件',status:1}],total:1}
    else if(path==='/inventory/alerts') data={warehouses:[{id:'1',code:'WH1',name:'测试仓',unit:''}],list:Array.from({length:10},(_,i)=>({id:String(i+1),warehouse_id:'1',warehouse_code:'WH1',warehouse_name:'测试仓',sku_id:'2',sku_code:'SKU001',sku_name:'测试货品'+i,unit:'件',min_available_qty:10,available_qty:2,stock_quantity:5,allocated_qty:3,gap_qty:8,state:'LOW',enabled:true,updated_at:date,updated_by:'tester'})),total:100,summary:{configured:100,low:100,out_of_stock:0},checked_at:date}
    else if(path==='/inventory') data={list:[],total:0}
    else throw Error('unexpected fixture API '+path)
    await route.fulfill({json:{code:0,msg:'ok',data}})
  })
}

for(const theme of ['light','dark']) {
  test(`${theme}: 预警只读入口、库存追溯与局部滚动`,async({page})=>{
    await page.setViewportSize({width:1366,height:768});await fixture(page,theme);await page.goto('/inventory/alerts')
    await expect(page.getByRole('heading',{name:'库存预警',exact:true})).toBeVisible()
    await expect(page.getByRole('button',{name:'配置预警规则',exact:true})).toHaveCount(0)
    await expect(page.locator('.el-table__body-wrapper tbody tr')).toHaveCount(10)
    await expect(page.locator('.pagination')).toBeInViewport({ratio:1})
    expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1)
    await page.screenshot({path:test.info().outputPath(`alerts-desktop-${theme}.png`)})
    await page.getByRole('button',{name:'库存明细',exact:true}).first().click()
    await expect(page).toHaveURL(/warehouse_id=1/);await expect(page).toHaveURL(/sku_keyword=SKU001/)
    await expect(page.getByPlaceholder('编码/名称/条码')).toHaveValue('SKU001')
  })
  test(`${theme}: 手机保留分页且没有整页横向溢出`,async({page})=>{
    await page.setViewportSize({width:390,height:844});await fixture(page,theme);await page.goto('/inventory/alerts')
    await expect(page.locator('.el-table__body-wrapper tbody tr')).toHaveCount(10)
    await expect(page.locator('.pagination')).toBeInViewport({ratio:1})
    expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1)
    await page.screenshot({path:test.info().outputPath(`alerts-mobile-${theme}.png`)})
  })
}
