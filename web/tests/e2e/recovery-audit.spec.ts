import { expect, test, type Page } from '@playwright/test'

type Kind = 'freeze' | 'return' | 'abort' | 'collect' | 'create'
const at = '2026-10-24T10:00:00+08:00'
async function fixture(page: Page, mode: 'network' | 'receipt' = 'network', readonly = false) {
  const user = {user_id:'42',username:'auditor',nickname:'恢复验收',roles:[],perms:readonly?['wms:outbound:view']:['*']}
  const state = {writes:[] as {key:string; body:unknown}[],commits:new Set<string>(),basicReads:0,optionsFail:false}
  await page.addInitScript(user=>{localStorage.setItem('WMS_TOKEN','recovery-audit');localStorage.setItem('WMS_USER',JSON.stringify(user));localStorage.setItem('gowms_theme','dark')},user)
  await page.route('**/api/v1/**',async route=>{
    const req=route.request(),url=new URL(req.url()),path=url.pathname.replace('/api/v1','')
    if(req.method()!=='GET') {
      const key=req.headers()['idempotency-key']!
      state.writes.push({key,body:req.postData()?req.postDataJSON():undefined});state.commits.add(key)
      if(state.writes.length===1) {
        if(mode==='network') await route.abort('failed')
        else await route.fulfill({json:{code:0,data:{}}})
        return
      }
      await route.fulfill({json:{code:0,data:path==='/inventory/freeze'?{inventory_id:'1'}:path==='/outbound/returns'?{object_id:'9'}:{wave_id:'1'}}});return
    }
    let data:unknown
    if(path==='/profile')data=user
    else if(path==='/basic/warehouses') {
      state.basicReads++
      if(readonly||state.optionsFail){await route.fulfill({status:readonly?403:503,json:{code:10003,msg:'options unavailable'}});return}
      data={list:[{id:'1',code:'WH01',name:'恢复测试仓',status:1}],total:1}
    }
    else if(path==='/basic/skus')data={list:[{id:'1',code:'SKU',name:'恢复货品',status:1}],total:1}
    else if(path==='/inventory')data={list:[{id:'1',warehouse_id:'1',location_id:'21',location_code:'BIN',sku_id:'1',batch_no:'B',stock_quantity:3,available_quantity:3,allocated_quantity:0,frozen_quantity:0,stock_in_time:at}],total:1}
    else if(path==='/outbound/orders/1')data={order:{id:'1',order_no:'CK-RETURN',warehouse_id:'1',status:'SHIPPED',picked_qty:3,expected_qty:3,allocated_qty:3,created_at:at},details:[],allocations:[],tasks:[]}
    else if(path==='/outbound/exceptions'||path==='/outbound/returns')data={list:[],total:0}
    else if(path==='/outbound/orders/1/returnable')data=[{allocation_id:'11',sku_id:'1',sku_code:'SKU',sku_name:'恢复货品',batch_no:'B',shipped_qty:3,returned_qty:0,returnable_qty:3}]
    else if(path==='/outbound/orders/1/return-locations')data={list:[{id:'21',code:'BIN-RETURN'}],total:1}
    else if(path==='/outbound/waves')data={list:[{id:'1',wave_no:'WAVE-RECOVERY',warehouse_id:'1',status:'PICKING',created_at:at}],total:1}
    else if(path==='/outbound/waves/candidates')data=[{id:'3',order_no:'CK-CANDIDATE',warehouse_id:'1',status:'APPROVED',created_at:at}]
    else if(path==='/outbound/waves/1')data={wave:{id:'1',wave_no:'WAVE-RECOVERY',warehouse_id:'1',status:'PICKING',claimed_by:'auditor',lease_expire_at:'2099-01-01T00:00:00Z',created_at:at},orders:[],items:[],lines:[{id:'11',sku_id:'1',sku_code:'SKU',sku_name:'恢复货品',location_code:'BIN',batch_no:'B',target_qty:3,picked_qty:0}]}
    else throw Error(`Unexpected recovery API ${path}`)
    await route.fulfill({json:{code:0,data}})
  })
  return state
}
async function open(page:Page,kind:Kind) {
  if(kind==='freeze') {
    await page.goto('/inventory');await page.getByRole('button',{name:'冻结/解冻',exact:true}).click()
  } else if(kind==='return') {
    await page.goto('/outbound/orders/1');await page.getByRole('button',{name:'验收客户退货',exact:true}).click()
  } else if(kind==='create') {
    await page.goto('/outbound/waves');await page.getByRole('button',{name:'新建波次',exact:true}).click()
  } else {
    await page.goto('/outbound/waves/1');await page.getByRole('button',{name:kind==='abort'?'异常结束波次':'拣货',exact:true}).click()
  }
  const title={freeze:'库存冻结 / 解冻',return:'客户退货验收',abort:'异常结束波次',collect:'登记汇总拣货',create:'新建波次'}[kind]
  const dialog=page.getByRole('dialog',{name:title,exact:true})
  if(kind==='create') {
    await dialog.locator('.el-select').click()
    await page.getByRole('option',{name:'WH01（恢复测试仓）',exact:true}).click()
    await dialog.getByRole('button',{name:'按创建时间选单',exact:true}).click()
    await expect(dialog.getByText('已选择 1 单',{exact:true})).toBeVisible()
  } else if(kind==='collect') {
    for(const [label,value] of [['货品编码或条码','SKU'],['来源库位','BIN'],['批次（无批次留空）','B']])await dialog.getByRole('textbox',{name:label,exact:true}).fill(value!)
  } else {
    if(kind==='return') {
      await dialog.locator('.el-form-item').filter({hasText:'验收入库库位'}).locator('.el-select').click()
      await page.getByRole('option',{name:'BIN-RETURN',exact:true}).click()
    }
    if(kind==='abort')await dialog.locator('.el-checkbox').click()
    await dialog.locator('textarea').fill('恢复测试，保持原参数')
  }
  return {dialog,submit:{freeze:'确认冻结',return:'确认处理',abort:'确认异常结束',collect:'确认登记',create:'确认创建波次'}[kind]}
}
for(const mode of ['receipt','network'] as const) {
  for(const kind of ['freeze','return','abort','collect','create'] as const) {
    test(`${kind} ${mode}: recover inside the open dialog using the original key`,async({page})=>{
      await page.setViewportSize({width:390,height:844})
      const state=await fixture(page,mode)
      const {dialog,submit}=await open(page,kind)
      await dialog.getByRole('button',{name:submit,exact:true}).click()
      await expect.poll(()=>page.evaluate(()=>JSON.parse(localStorage.getItem('WMS_PENDING_OPS')||'{"items":[]}').items.length),{timeout:2000}).toBe(1)
      const recover=dialog.getByRole('button',{name:'恢复原操作',exact:true})
      await expect(recover).toBeVisible({timeout:2000})
      await recover.click()
      await expect(dialog).toBeHidden()
      expect(state.writes).toHaveLength(2)
      expect(state.writes[1]).toEqual(state.writes[0])
      expect(state.commits.size).toBe(1)
      expect(await page.evaluate(()=>JSON.parse(localStorage.getItem('WMS_PENDING_OPS')!).items)).toHaveLength(0)
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
    })
  }
}
test('wave readonly does not request forbidden master data',async({page})=>{
  const state=await fixture(page,'network',true)
  await page.goto('/outbound/waves')
  await expect(page.getByText('WAVE-RECOVERY',{exact:true})).toBeVisible()
  expect(state.basicReads).toBe(0)
})
test('wave warehouse options can be retried without losing the list',async({page})=>{
  const state=await fixture(page);state.optionsFail=true
  await page.goto('/outbound/waves')
  await expect(page.getByText('WAVE-RECOVERY',{exact:true})).toBeVisible()
  const retry=page.getByRole('button',{name:'重新加载仓库',exact:true})
  await expect(retry).toBeVisible({timeout:2000})
  state.optionsFail=false;await retry.click()
  await page.getByRole('button',{name:'新建波次',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:'新建波次',exact:true})
  await dialog.locator('.el-select').click()
  await expect(page.getByRole('option',{name:'WH01（恢复测试仓）',exact:true})).toBeVisible()
  expect(state.writes).toHaveLength(0)
})
