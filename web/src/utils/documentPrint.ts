import type { InboundOrderDetail, OutboundOrderDetail, StocktakeOrderDetail } from '@/api/types'

export interface PrintSection {
  title: string
  columns: string[]
  rows: (string | number | null)[][]
}

export interface PrintDocument {
  title: string
  orderNo: string
  warehouseID: string
  status: string
  createdAt: string
  remark: string
  sections: PrintSection[]
  notes: string[]
}

export function inboundPrint(data: InboundOrderDetail): PrintDocument {
  const order = data.order
  return {
    title: '入库单',
    orderNo: order.order_no,
    warehouseID: order.warehouse_id,
    status: order.status,
    createdAt: order.created_at,
    remark: order.remark,
    sections: [{
      title: '入库明细',
      columns: ['货品编码', '货品名称', '批次', '应收数量', '已收数量', '不良数量'],
      rows: (data.details ?? []).map(detail => [
        detail.sku_code, detail.sku_name, detail.batch_no || '-',
        detail.expected_qty, detail.received_qty, detail.defective_qty,
      ]),
    }],
    notes: [
      '数量按各货品库存计量单位核对。',
      '本单为已保存业务数据的查询快照，打印不代表已收货或已上架。',
    ],
  }
}

export function outboundPrint(data: OutboundOrderDetail): PrintDocument {
  const order = data.order
  const lines = data.details ?? []
  const allocations = data.allocations ?? []
  const details = new Map(lines.map(detail => [detail.id, detail]))
  const sections: PrintSection[] = [{
    title: '出库明细',
    columns: ['货品编码', '货品名称', '应出数量', '已分配数量', '已拣数量'],
    rows: lines.map(detail => [
      detail.sku_code, detail.sku_name,
      detail.expected_qty, detail.allocated_qty, detail.picked_qty,
    ]),
  }]
  if (allocations.length) {
    sections.push({
      title: '按原分配行核对拣货',
      columns: ['货品', '来源库位', '批次', '分配数量', '已拣数量'],
      rows: allocations.map(allocation => [
        details.get(allocation.detail_id)?.sku_name || allocation.sku_id,
        allocation.location_code, allocation.batch_no || '-',
        allocation.allocated_qty, allocation.picked_qty,
      ]),
    })
  }
  return {
    title: '出库单 / 拣货单',
    orderNo: order.order_no,
    warehouseID: order.warehouse_id,
    status: order.status,
    createdAt: order.created_at,
    remark: order.remark,
    sections,
    notes: [
      allocations.length
        ? '请按原分配库位、批次及数量核对，不以本单另行分配或重复扣减库存。'
        : '尚未分配库存，当前单据不能作为实际拣货指令。',
      '本单为已保存数据的查询快照，数量按货品库存单位核对，打印不代表已发货。',
    ],
  }
}

export function stocktakePrint(data: StocktakeOrderDetail): PrintDocument {
  const order = data.order
  return {
    title: '盘点单',
    orderNo: order.order_no,
    warehouseID: order.warehouse_id,
    status: order.status,
    createdAt: order.created_at,
    remark: order.remark,
    sections: [{
      title: '盘点明细',
      columns: ['货品', '库位', '批次', '账面数量', '实盘数量', '差异'],
      rows: (data.details ?? []).map(detail => [
        detail.sku_name || detail.sku_code, detail.location_code, detail.batch_no || '-',
        detail.book_qty, detail.actual_qty,
        detail.actual_qty === null ? null
          : order.status === 'DRAFT' ? detail.actual_qty - detail.book_qty : detail.diff_qty,
      ]),
    }],
    notes: [
      '空白实盘栏表示尚未录入；0 表示已盘点数量为零。',
      '仅包含服务端已保存的盘点数据，页面未保存输入不参与打印。数量按货品库存计量单位核对。',
      '纸面填写后仍需在系统中录入并审核；打印本身不调整库存。',
    ],
  }
}
