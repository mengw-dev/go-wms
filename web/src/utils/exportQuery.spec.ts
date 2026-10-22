import { describe, expect, it } from 'vitest'
import { exportQuery } from './exportQuery'
describe('Excel export query',()=>{
  it('drops pagination and empty fields, preserves false, IDs, literal keywords and scope',()=>{
    const query={page:8,page_size:10,warehouse_id:'9223372036854775800',in_stock_only:false,order_no:'%_001',status:'',sku_id:undefined}
    expect(exportQuery(query)).toEqual({warehouse_id:'9223372036854775800',in_stock_only:false,order_no:'%_001'})
    expect(query.page).toBe(8)
  })
})
