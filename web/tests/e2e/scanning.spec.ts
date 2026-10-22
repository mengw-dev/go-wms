import { expect, test, type Page, type Locator } from '@playwright/test'

const date='2026-10-22T10:00:00+08:00'
const user={user_id:'42',username:'worker',nickname:'仓库作业员',roles:[],perms:['wms:inbound:view','wms:inbound:receive','wms:inbound:putaway','wms:outbound:view','wms:outbound:pick']}
const sku={id:'10',code:'SKU-A',name:'货品A',barcode:'00123'}
const details=[{id:'101',order_id:'1',sku_id:'10',sku_code:'SKU-A',sku_name:'货品A',expected_qty:5,received_qty:0,defective_qty:0,batch_no:'B001',allocated_qty:5,picked_qty:0},{id:'102',order_id:'1',sku_id:'20',sku_code:'SKU-B',sku_name:'货品B',expected_qty:5,received_qty:0,defective_qty:0,batch_no:'B002',allocated_qty:5,picked_qty:0}]
async function fixture(page:Page,theme:string,kind:'receive'|'putaway'|'pick') {
  const writes:{path:string;key:string;body:Record<string,unknown>}[]=[],lookups:string[]=[]
  await page.addInitScript(({user,theme})=>{localStorage.setItem('WMS_TOKEN','scan-fixture');localStorage.setItem('WMS_USER',JSON.stringify(user));localStorage.setItem('gowms_theme',theme)},{user,theme})
  await page.route('**/api/v1/**',async route=>{
    const req=route.request(),url=new URL(req.url()),path=url.pathname.replace('/api/v1','')
    let data:unknown
    if(req.method()!=='GET') {
      writes.push({path,key:req.headers()['idempotency-key']||'',body:req.postDataJSON()})
      if(path==='/pda/tasks/11/claim') data={claim_token:'test-claim',lease_expire_at:'2099-01-01T00:00:00+08:00',task_status:'IN_PROGRESS',done_qty:0,remaining_qty:2,order_status:'PICKING'}
      else if(path==='/pda/tasks/11/pick') data={task_status:'COMPLETED',done_qty:2,remaining_qty:0,order_status:'SHIPPED'}
      else if(path==='/inbound/orders/1/receive'||path==='/inbound/tasks/11/putaway')data=null
      else throw Error('unexpected scanner mutation '+path)
    }
    else if(path==='/profile')data=user
    else if(path==='/version')data={version:'scan-fixture',demo_enabled:false}
    else if(path.endsWith('/scan-sku')) {const code=url.searchParams.get('code')||'';lookups.push(code);data=code==='00123'||code==='SKU-A'?[sku]:code==='SKU-B'?[{id:'20',code:'SKU-B',name:'货品B',barcode:'00234'}]:[]}
    else if(path.endsWith('/putaway-locations')) {const code=url.searchParams.get('code'),rows=[{id:'2',code:'A02',status:2},{id:'3',code:'A03',status:1}].filter(row=>!code||row.code===code);data={list:rows,total:rows.length}}
    else if(path==='/inbound/orders/1'||path==='/outbound/orders/1')data={
      order:{id:'1',order_no:kind==='pick'?'CK1':'RK1',warehouse_id:'1',status:kind==='receive'?'APPROVED':kind==='putaway'?'PUTAWAY':'PICKING',source:'MANUAL',expected_qty:10,received_qty:kind==='receive'?0:10,defective_qty:0,allocated_qty:10,picked_qty:0,created_at:date,updated_at:date,created_by:'worker'},
      details,allocations:[],tasks:[{id:'11',task_no:'TASK11',task_type:kind==='pick'?'PICK':'PUTAWAY',status:'CREATED',order_id:'1',order_no:kind==='pick'?'CK1':'RK1',detail_id:'101',sku_id:'10',warehouse_id:'1',location_id:'1',location_code:'A01',batch_no:'B001',target_qty:2,done_qty:0,created_at:date}],
    }
    else throw Error('unexpected scanner API '+path)
    await route.fulfill({json:{code:0,msg:'ok',data}})
  })
  await page.goto(`/${kind==='pick'?'outbound':'inbound'}/orders/1`)
  await page.locator('.detail-actions').getByRole('button',{name:kind==='receive'?'收货':kind==='putaway'?'上架':'拣货',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:kind==='receive'?'收货':kind==='putaway'?'上架':'PDA 拣货',exact:true})
  await expect(dialog).toBeVisible()
  return{dialog,writes,lookups}
}
async function scan(field:Locator,value:string) {await field.fill(value);await field.press('Enter')}
for(const theme of ['light','dark'])for(const width of [1366,390]) {
  test(`${theme} ${width}: receive scan selects lines and repeated Enter never writes quantity`,async({page})=>{
    await page.setViewportSize({width,height:width===390?844:768});const {dialog,writes,lookups}=await fixture(page,theme,'receive')
    const field=dialog.getByRole('textbox',{name:'收货货品扫码',exact:true})
    await scan(field,'00123');await expect(dialog.getByText(/已定位 货品A/)).toBeVisible()
    await expect(dialog.locator('.el-table__body-wrapper tbody tr')).toHaveCount(1)
    expect(await field.evaluate(node => {
      const event = new KeyboardEvent('keydown', {key:'Enter',isComposing:true,bubbles:true,cancelable:true})
      node.dispatchEvent(event)
      return event.defaultPrevented
    })).toBe(false)
    expect(lookups).toHaveLength(1)
    await expect(field).toBeFocused()
    await page.keyboard.type('00123');await page.keyboard.press('Enter');await expect.poll(()=>lookups.length).toBe(2);expect(writes).toHaveLength(0)
    await expect(dialog.getByText(/已定位 货品A/)).toBeVisible()
    await expect(dialog.locator('.el-table__body-wrapper tbody tr')).toHaveCount(1)
    await expect(dialog.getByRole('button',{name:'收货',exact:true})).toBeEnabled()
    await page.screenshot({animations:'disabled',path:test.info().outputPath(`scan-receive-${theme}-${width}.png`)})
    await dialog.getByRole('button',{name:'收货',exact:true}).click();await expect.poll(()=>writes.length).toBe(1)
    expect(writes[0]!.key).toBeTruthy();expect(writes[0]!.body).toMatchObject({detail_id:'101',qty:5,batch_no:'B001'})
  })
  test(`${theme} ${width}: putaway scans SKU and enabled occupied location without basic permission`,async({page})=>{
    await page.setViewportSize({width,height:width===390?844:768});const {dialog,writes}=await fixture(page,theme,'putaway')
    await scan(dialog.getByRole('textbox',{name:'上架货品扫码',exact:true}),'00123');await expect(dialog.getByText('已核对 货品A · SKU-A')).toBeVisible()
    const location=dialog.getByRole('textbox',{name:'上架库位扫码',exact:true})
    await scan(location,'OTHER');await expect(dialog.getByText('当前单据仓库没有此编码的启用库位，请核对仓库和标签')).toBeVisible();await expect(dialog.getByRole('button',{name:'确定上架',exact:true})).toBeDisabled()
    await scan(location,'A02');await expect(dialog.getByRole('combobox',{name:'目标库位',exact:true})).toHaveAttribute('aria-expanded','false')
    await expect(dialog.getByText('A02（已占用）',{exact:true})).toBeVisible()
    await location.press('Enter');await expect(dialog.getByRole('button',{name:'确定上架',exact:true})).toBeEnabled();expect(writes).toHaveLength(0)
    await expect(dialog.locator('.el-dialog__footer')).toBeInViewport({ratio:1})
    await page.screenshot({animations:'disabled',path:test.info().outputPath(`scan-putaway-${theme}-${width}.png`)})
    await dialog.getByRole('button',{name:'确定上架',exact:true}).click();await expect.poll(()=>writes.length).toBe(1)
    expect(writes[0]!.key).toBeTruthy();expect(writes[0]!.body).toMatchObject({location_id:'2',qty:2})
  })
  test(`${theme} ${width}: pick checks wrong SKU, location and batch; only explicit confirm writes`,async({page})=>{
    await page.setViewportSize({width,height:width===390?844:768});const {dialog,writes}=await fixture(page,theme,'pick')
    await dialog.getByRole('button',{name:'领取任务',exact:true}).click();await expect(dialog.getByText('已领取',{exact:true})).toBeVisible()
    const field=dialog.getByRole('textbox',{name:'拣货货品扫码',exact:true})
    await scan(field,'SKU-B');await expect(dialog.getByText('货品不符，请核对当前任务对应的货品')).toBeVisible();await expect(dialog.getByRole('button',{name:'确定拣货',exact:true})).toBeDisabled()
    await scan(field,'00123');await expect(dialog.getByText('已核对 货品A · SKU-A')).toBeVisible()
    await scan(dialog.getByPlaceholder('扫描或输入作业库位'),'A01');await scan(dialog.getByPlaceholder('扫描或输入批次号核对'),'B001')
    await expect(dialog.getByRole('spinbutton',{name:'拣货数量'})).toBeEnabled()
    expect(writes).toHaveLength(1)
    await expect(dialog.locator('.el-dialog__footer')).toBeInViewport({ratio:1})
    await page.screenshot({animations:'disabled',path:test.info().outputPath(`scan-pick-${theme}-${width}.png`)})
    await dialog.getByRole('button',{name:'确定拣货',exact:true}).click();await expect(dialog).toBeHidden();expect(writes).toHaveLength(2)
    expect(writes[1]!.key).toBeTruthy();expect(writes[1]!.body).toMatchObject({qty:2,location_code:'A01',batch_no:'B001',claim_token:'test-claim'})
  })
}
