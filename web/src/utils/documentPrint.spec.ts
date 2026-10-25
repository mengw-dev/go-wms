import { describe, expect, it } from 'vitest'
import type { OutboundOrderDetail } from '@/api/types'
import { inboundPrint, outboundPrint, stocktakePrint } from './documentPrint'

const order={id:'9007199254740993',order_no:'TEST00001',biz_order_no:'ERP1',warehouse_id:'9223372036854775800',location_id:'2',location_code:'A-01',status:'DRAFT',source:'MANUAL',import_task_id:'',remark:'<script>alert(1)</script>',expected_qty:4,allocated_qty:4,picked_qty:0,received_qty:0,defective_qty:0,created_by:'operator',created_at:'2026-10-21T08:00:00Z',updated_at:'2026-10-21T08:00:00Z'}
const line={id:'3',order_id:order.id,inventory_id:'5',sku_id:'6',sku_code:'000123',sku_name:'货品',location_id:'2',location_code:'A-01',expected_qty:4,received_qty:0,defective_qty:0,allocated_qty:4,picked_qty:0,batch_no:'B01',book_qty:4,actual_qty:null as number|null,diff_qty:0,adjusted:false}

describe('只读打印快照',()=>{
  it('保留字符串大ID、条码前导零与实际零数量，不把用户文字解析为HTML',()=>{
    const data={order:{...order},details:[{...line}],tasks:[]},print=inboundPrint(data)
    expect(print.warehouseID).toBe('9223372036854775800')
    expect(print.sections[0]!.rows[0]).toEqual(['000123','货品','B01',4,0,0])
    expect(print.remark).toBe('<script>alert(1)</script>')
    data.details[0]!.received_qty=4
    expect(print.sections[0]!.rows[0]![4]).toBe(0)
  })
  it('保留出库原库位、批次、已拣数量，不重新合并或分配',()=>{
    const print=outboundPrint({order,details:[line],allocations:[{id:'7',order_id:order.id,detail_id:'3',inventory_id:'5',sku_id:'6',location_id:'2',location_code:'A-01',batch_no:'B01',allocated_qty:4,picked_qty:2,status:'ALLOCATED'}],tasks:[]})
    expect(print.sections[1]!.rows[0]).toEqual(['货品','A-01','B01',4,2])
  })
  it('未分配单据不伪装为可执行拣货指令，兼容旧空数组响应',()=>{
    const print=outboundPrint({order,details:[line],allocations:null as unknown as OutboundOrderDetail['allocations'],tasks:[]})
    expect(print.sections).toHaveLength(1)
    expect(print.notes[0]).toContain('不能作为实际拣货指令')
  })
  it('异常重分配只打印当前作业，取消与已发分配独立标为历史', () => {
    const allocation = { id: '7', order_id: order.id, detail_id: '3', inventory_id: '5', sku_id: '6', location_id: '2', location_code: 'A-01', batch_no: 'B01', allocated_qty: 4, picked_qty: 0, status: 'ALLOCATED' }
    const snapshot = { order, details: [line], allocations: [allocation, { ...allocation, id: '8', picked_qty: 2, status: 'CANCELLED' }, { ...allocation, id: '9', picked_qty: 4, status: 'PICKED' }], tasks: [] }
    const print = outboundPrint(snapshot)
    expect(print.sections[1]!.rows).toEqual([['货品', 'A-01', 'B01', 4, 0]])
    expect(print.sections[2]!.title).toContain('不作为拣货指令')
    expect(print.sections[2]!.rows.map(row => row[3])).toEqual(['已取消', '已拣货'])
    expect(snapshot.allocations).toHaveLength(3)
    expect(outboundPrint({ ...snapshot, allocations: snapshot.allocations.slice(1) }).notes[0]).toContain('不能作为实际拣货指令')
  })
  it('盘点未录入留空，已录入零保留0与负差异',()=>{
    const print=stocktakePrint({order,details:[line,{...line,id:'4',actual_qty:0}]})
    expect(print.sections[0]!.rows[0]!.slice(-2)).toEqual([null,null])
    expect(print.sections[0]!.rows[1]!.slice(-2)).toEqual([0,-4])
  })
})
