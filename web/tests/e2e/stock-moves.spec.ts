import { expect, test, type Page } from '@playwright/test'

const date='2026-10-22T10:00:00+08:00'
const profile={user_id:'42',username:'worker',nickname:'仓库操作员',roles:[],perms:['wms:inventory','wms:inventory:move']}
const source={id:'10',warehouse_id:'1',warehouse_code:'WH01',location_code:'A01',sku_code:'SKU001',sku_name:'测试货品',unit:'箱',batch_no:'B001',available_qty:8,stock_in_time:date,enabled:true}
const receipt={id:'100',move_no:'YK20261022001',warehouse_id:'1',warehouse_code:'WH01',source_inventory_id:'10',target_inventory_id:'20',source_location_id:'1',target_location_id:'2',source_location_code:'A01',target_location_code:'A02',sku_id:'1',sku_code:'SKU001',sku_name:'测试货品',unit:'箱',batch_no:'B001',quantity:3,stock_in_time:date,operator:'worker',reason:'库位整理',created_at:date}
async function fixture(page:Page,theme:string,readOnly=false) {
  const user=readOnly?{...profile,perms:['wms:inventory']}:profile
  const writes:{key:string;body:unknown}[]=[],state={lost:false,done:false}
  await page.addInitScript(({user,theme})=>{localStorage.setItem('WMS_TOKEN','move-fixture');localStorage.setItem('WMS_USER',JSON.stringify(user));localStorage.setItem('gowms_theme',theme)},{user,theme})
  await page.route('**/api/v1/**',async route=>{
    const req=route.request(),url=new URL(req.url()),path=url.pathname.replace('/api/v1','')
    let data:unknown
    if(path==='/profile') data=user
    else if(path==='/version') data={version:'fixture',demo_enabled:false}
    else if(path==='/inventory/moves' && req.method()==='POST') {
      writes.push({key:req.headers()['idempotency-key']!,body:req.postDataJSON()});state.done=true
      if(state.lost){state.lost=false;await route.abort('connectionfailed');return}
      data=receipt
    }
    else if(path==='/inventory/moves') data={list:url.searchParams.has('operation_key')?(state.done?[receipt]:[]):Array.from({length:10},(_,i)=>({...receipt,id:String(100+i),move_no:receipt.move_no+i})),total:url.searchParams.has('operation_key')?(state.done?1:0):40}
    else if(path==='/inventory/move-source/10') data=source
    else if(path==='/inventory/move-locations') data={list:[{id:'2',code:'A02'}],total:1}
    else if(path==='/inventory') data={list:[{id:'10',warehouse_id:'1',location_id:'1',location_code:'A01',sku_id:'1',batch_no:'B001',stock_quantity:10,available_quantity:8,allocated_quantity:2,stock_in_time:date}],total:1}
    else if(path==='/inventory/trans') data={list:[],total:0}
    else throw Error('unexpected move API '+path)
    await route.fulfill({json:{code:0,msg:'ok',data}})
  })
  return {writes,state}
}
async function fill(page:Page) {
  const dialog=page.getByRole('dialog',{name:'仓内移库',exact:true})
  await expect(dialog).toBeVisible();await expect(dialog.getByText('测试货品 · SKU001')).toBeVisible()
  await dialog.getByRole('combobox',{name:'目标库位',exact:true}).click()
  await page.getByRole('option',{name:'A02',exact:true}).click()
  await dialog.getByRole('spinbutton').fill('3');await dialog.getByPlaceholder('例如：库位整理、调整存放位置').fill('库位整理')
  return dialog
}
for(const theme of ['light','dark']) {
  test(`${theme}: desktop create, movable table, ledger and no extra basic permission`,async({page})=>{
    await page.setViewportSize({width:1366,height:768});const {writes}=await fixture(page,theme)
    await page.goto('/inventory');await page.getByRole('button',{name:'移库',exact:true}).click()
    const dialog=await fill(page);await page.screenshot({path:test.info().outputPath(`move-dialog-${theme}.png`)})
    await dialog.getByRole('button',{name:'确认移库',exact:true}).click();await expect(dialog).toBeHidden()
    expect(writes).toHaveLength(1);expect(writes[0]!.key).toBeTruthy();expect(writes[0]!.body).toEqual({source_inventory_id:'10',target_location_id:'2',quantity:3,reason:'库位整理'})
    await expect(page.locator('.pagination')).toBeInViewport({ratio:1})
    await page.screenshot({path:test.info().outputPath(`moves-desktop-${theme}.png`)})
    await page.getByRole('button',{name:'库存流水',exact:true}).first().click();await expect(page).toHaveURL(/order_no=YK/)
  })
  test(`${theme}: mobile dialog and history stay within screen`,async({page})=>{
    await page.setViewportSize({width:390,height:844});await fixture(page,theme);await page.goto('/inventory/moves?source_inventory_id=10')
    const dialog=await fill(page)
    await expect(dialog.getByRole('button',{name:'确认移库',exact:true})).toBeInViewport({ratio:1})
    expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1)
    await page.screenshot({path:test.info().outputPath(`move-mobile-${theme}.png`)})
    await dialog.getByRole('button',{name:'返回',exact:true}).click()
    await expect(page.locator('.pagination')).toBeInViewport({ratio:1})
  })
}
test('lost response survives reload; result reconciliation sends no second mutation',async({page})=>{
  const {writes,state}=await fixture(page,'dark');state.lost=true
  await page.goto('/inventory/moves?source_inventory_id=10');const dialog=await fill(page)
  await dialog.getByRole('button',{name:'确认移库',exact:true}).click()
  await expect(dialog.getByText('Network Error',{exact:true})).toBeVisible()
  await page.goto('/inventory/moves');await expect(page.getByLabel('待确认移库')).toBeVisible()
  await page.getByRole('button',{name:'核对结果',exact:true}).click()
  await expect(page.getByLabel('待确认移库')).toHaveCount(0);expect(writes).toHaveLength(1)
})
test('read-only inventory cannot initiate or retry moves',async({page})=>{
  await fixture(page,'light',true);await page.goto('/inventory');await expect(page.getByRole('button',{name:'移库',exact:true})).toHaveCount(0)
  await page.goto('/inventory/moves?source_inventory_id=10');await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.locator('.el-table__body-wrapper tbody tr')).toHaveCount(10)
})
