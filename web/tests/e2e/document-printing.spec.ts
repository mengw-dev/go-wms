import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const date='2026-10-21T08:00:00Z',qaDir=process.env.WMS_PRINT_QA_DIR
async function fixture(page: Page,theme='light',basic=true,fail=false) {
  const user={user_id:'42',username:'printer',nickname:'单据查看员',roles:[],perms:basic ? ['*'] : ['wms:inbound:view','wms:outbound:view','wms:stocktake:view']}
  await page.addInitScript(({user,theme})=>{
    localStorage.setItem('WMS_TOKEN','print-fixture');localStorage.setItem('WMS_USER',JSON.stringify(user));localStorage.setItem('gowms_theme',theme)
    Reflect.set(window,'__printCalls',0);window.print=()=>{Reflect.set(window,'__printCalls',Number(Reflect.get(window,'__printCalls'))+1)}
  },{user,theme})
  await page.route('**/api/v1/**',async route=>{
    expect(route.request().method()).toBe('GET')
    const path=new URL(route.request().url()).pathname.replace('/api/v1','')
    let data: unknown
    if(path==='/profile') data=user
    else if(path==='/version') data={version:'fixture',demo_enabled:false}
    else if(path==='/basic/warehouses') {expect(basic).toBeTruthy();data={list:[{id:'1',code:'WH01',name:'样张测试仓',status:1,created_at:date}],total:1}}
    else if(path==='/basic/skus') data={list:[{id:'2',code:'SKU-001',name:'精密零件 <img src=x>',barcode:'00012345',unit:'件',status:1,created_at:date},{id:'3',code:'SKU-002',name:'不支持的条码',barcode:'中文条码',unit:'件',status:1,created_at:date}],total:2}
    else if(path==='/basic/locations') data={list:[{id:'4',warehouse_id:'1',code:'A-01-01',zone:'A',status:1,created_at:date}],total:1}
    else if(/^\/(inbound|outbound|stocktake)\/orders\/1$/.test(path)) {
      if(fail) {await route.fulfill({status:404,json:{code:40001,msg:'not found',data:null}});return}
      const kind=path.split('/')[1]!,count=kind==='outbound' ? 45 : 2
      const order={id:'1',warehouse_id:'1',order_no:`${kind.toUpperCase()}-PRINT-0001`,status:kind==='outbound' ? 'PICKING' : 'DRAFT',remark:'样张数据，仅用于打印验证',created_at:date,updated_at:date,created_by:'printer'}
      const details=Array.from({length:count},(_,i)=>({id:String(i+10),order_id:'1',sku_id:'2',sku_code:`PRINTROW${String(i+1).padStart(3,'0')}`,sku_name:`精密零件${i+1}`,batch_no:`B${String(i+1).padStart(3,'0')}`,location_code:'A-01-01',expected_qty:4,received_qty:0,defective_qty:0,allocated_qty:4,picked_qty:0,book_qty:4,actual_qty:i===0 ? null : 0,diff_qty:0}))
      const allocations=details.map((d,i)=>({id:String(i+100),order_id:'1',detail_id:d.id,sku_id:'2',location_code:'A-01-01',batch_no:d.batch_no,allocated_qty:4,picked_qty:0,status:'ALLOCATED'}))
      data={order,details,allocations,tasks:[]}
    } else if(path==='/outbound/exceptions'||path==='/outbound/returns')data={list:[],total:0}
    else throw Error('unexpected printing API '+path)
    await route.fulfill({json:{code:0,msg:'ok',data}})
  })
}

for(const theme of ['light','dark']) {
  for(const kind of ['inbound','outbound','stocktake']) {
    test(`${theme}: ${kind} 单据只读打印、完整行与纸面样式`,async({page})=>{
      await page.setViewportSize({width:1366,height:900});await fixture(page,theme);await page.goto(`/print/${kind}/1`)
      const button=page.getByRole('button',{name:'打印 / 保存 PDF',exact:true})
      await expect(button).toBeEnabled();await expect(page.locator('.print-paper')).toContainText(`${kind.toUpperCase()}-PRINT-0001`)
      const expected=kind==='outbound' ? 90 : 2
      await expect(page.locator('.print-paper tbody tr')).toHaveCount(expected)
      await button.click();await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'__printCalls'))).toBe(1)
      await page.emulateMedia({media:'print'})
      await expect(page.locator('.print-controls')).not.toBeVisible()
      expect(await page.locator('.print-scroll').evaluate(el=>getComputedStyle(el).overflow)).toBe('visible')
      if(qaDir && theme==='light' && kind!=='inbound') {await mkdir(qaDir,{recursive:true});await page.pdf({path:join(qaDir,kind==='outbound' ? 'outbound-long.pdf' : 'stocktake.pdf'),preferCSSPageSize:true,printBackground:true})}
      if(kind==='inbound') await page.screenshot({path:test.info().outputPath(`inbound-${theme}.png`),fullPage:true})
    })
  }
  test(`${theme}: 单张条码、前导零、固定尺寸及打印后关闭`,async({page})=>{
    await page.setViewportSize({width:1366,height:900});await fixture(page,theme);await page.goto('/basic/skus')
    await page.getByRole('button',{name:'标签',exact:true}).first().click()
    const dialog=page.getByRole('dialog',{name:'条码标签预览',exact:true})
    await expect(dialog.locator('svg[role="img"]')).toHaveAttribute('aria-label','条码 00012345')
    await expect(dialog.locator('.barcode-label img')).toHaveCount(0)
    await dialog.locator('.el-input-number input').fill('2');await dialog.locator('.el-input-number input').blur()
    await expect(dialog.locator('.barcode-label')).toHaveCount(2)
    await dialog.getByRole('button',{name:'打印 / 保存 PDF',exact:true}).click()
    await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'__printCalls'))).toBe(1)
    await page.emulateMedia({media:'print'});await expect(page.locator('#app')).not.toBeVisible()
    await expect(dialog.locator('.label-sheet')).toBeVisible()
    const width=await dialog.locator('.barcode-label').first().evaluate(el=>el.getBoundingClientRect().width)
    expect(width).toBeCloseTo(90*96/25.4,0)
    if(qaDir && theme==='light') {
      await page.emulateMedia({media:'screen'})
      await dialog.locator('.el-input-number input').fill('20');await dialog.locator('.el-input-number input').blur()
      await expect(dialog.locator('.barcode-label')).toHaveCount(20)
      await page.emulateMedia({media:'print'})
      await mkdir(qaDir,{recursive:true});await page.pdf({path:join(qaDir,'labels.pdf'),preferCSSPageSize:true,printBackground:true})
    }
    await page.screenshot({path:test.info().outputPath(`labels-${theme}.png`),fullPage:true})
    await page.emulateMedia({media:'screen'});await dialog.getByRole('button',{name:'返回',exact:true}).click()
    await expect(dialog).not.toBeVisible();await page.emulateMedia({media:'print'});await expect(page.locator('#app')).toBeVisible()
  })
}

test('不支持的条码阻止整批打印，不静默丢失标签',async({page})=>{
  await fixture(page);await page.goto('/basic/skus');await page.getByRole('button',{name:'本页标签预览',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:'条码标签预览',exact:true})
  await expect(dialog).toContainText('仅支持1–64位')
  await expect(dialog.getByRole('button',{name:'打印 / 保存 PDF',exact:true})).toBeDisabled()
})
test('库位标签包含仓库上下文，手机预览局部横向滚动',async({page})=>{
  await page.setViewportSize({width:390,height:844});await fixture(page);await page.goto('/basic/locations')
  await page.getByRole('button',{name:'本页标签预览',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:'条码标签预览',exact:true})
  await expect(dialog.locator('.barcode-label')).toContainText('样张测试仓')
  await expect(dialog.locator('svg[role="img"]')).toHaveAttribute('aria-label','条码 A-01-01')
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1)
  await expect(dialog.getByRole('button',{name:'打印 / 保存 PDF',exact:true})).toBeInViewport()
})
test('单据只读权限无需基础资料权限，加载失败禁止打印旧数据',async({page})=>{
  await fixture(page,'light',false);await page.goto('/print/inbound/1')
  await expect(page.getByRole('button',{name:'打印 / 保存 PDF',exact:true})).toBeEnabled()
  await expect(page.locator('.print-paper')).toContainText('仓库 ID')
  await page.route('**/api/v1/inbound/orders/1',route=>route.fulfill({status:404,json:{code:40001,msg:'not found',data:null}}))
  await page.getByRole('button',{name:'刷新数据',exact:true}).click()
  await expect(page.getByRole('button',{name:'打印 / 保存 PDF',exact:true})).toBeDisabled()
  await expect(page.locator('.print-paper')).toHaveCount(0)
})

for(const kind of ['inbound','outbound','stocktake']) {
  test(`${kind}: 只读详情入口可以打开打印预览`,async({page})=>{
    await fixture(page,'light',false);await page.goto(`/${kind}/orders/1`)
    await page.getByRole('button',{name:'打印预览',exact:true}).click()
    await expect(page).toHaveURL(new RegExp(`/print/${kind}/1$`))
    await expect(page.getByRole('button',{name:'打印 / 保存 PDF',exact:true})).toBeEnabled()
  })
}

test('手机单据预览局部滚动，打印按钮可见',async({page})=>{
  await page.setViewportSize({width:390,height:844});await fixture(page,'dark');await page.goto('/print/inbound/1')
  await expect(page.getByRole('button',{name:'打印 / 保存 PDF',exact:true})).toBeInViewport()
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1)
  expect(await page.locator('.print-scroll').evaluate(el=>el.scrollWidth>el.clientWidth)).toBeTruthy()
})

test('超出200张时拒绝整批，不渲染过量标签',async({page})=>{
  await fixture(page)
  await page.route('**/api/v1/basic/skus?**',route=>{
    expect(route.request().method()).toBe('GET')
    return route.fulfill({json:{code:0,data:{list:Array.from({length:11},(_,i)=>({
      id:String(i+1),code:`SKU-${i}`,name:`货品${i}`,barcode:`0001234${i}`,unit:'件',status:1,created_at:date,
    })),total:11}}})
  })
  await page.goto('/basic/skus');await page.getByRole('button',{name:'本页标签预览',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:'条码标签预览',exact:true})
  await dialog.locator('.el-input-number input').fill('20');await dialog.locator('.el-input-number input').blur()
  await expect(dialog).toContainText('单次最多200张')
  await expect(dialog.locator('.barcode-label')).toHaveCount(0)
  await expect(dialog.getByRole('button',{name:'打印 / 保存 PDF',exact:true})).toBeDisabled()
})

test('等待字体时关闭并重开标签，不会误打印下一批',async({page})=>{
  await fixture(page);await page.goto('/basic/skus')
  await page.getByRole('button',{name:'标签',exact:true}).first().click()
  const dialog=page.getByRole('dialog',{name:'条码标签预览',exact:true})
  await page.evaluate(()=>{
    const ready=new Promise<void>(resolve=>Reflect.set(window,'__releaseFonts',resolve))
    Object.defineProperty(document.fonts,'ready',{configurable:true,get:()=>ready})
  })
  await dialog.getByRole('button',{name:'打印 / 保存 PDF',exact:true}).click()
  await dialog.getByRole('button',{name:'返回',exact:true}).click()
  await expect(dialog).not.toBeVisible()
  await page.getByRole('button',{name:'标签',exact:true}).first().click()
  await expect(dialog).toBeVisible()
  await page.evaluate(async()=>{
    Reflect.get(window,'__releaseFonts')()
    await document.fonts.ready
    await new Promise<void>(resolve=>requestAnimationFrame(()=>resolve()))
  })
  expect(await page.evaluate(()=>Reflect.get(window,'__printCalls'))).toBe(0)
  await expect(dialog.getByRole('button',{name:'打印 / 保存 PDF',exact:true})).toBeEnabled()
})
