export function dashboardFixture(warehouseId = '0') {
  const total = warehouseId === '2' ? 3 : 16
  return {
    generated_at: '2026-10-20T16:30:00+08:00', today_from: '2026-10-20 00:00:00', today_to: '2026-10-20 16:30:00', trend_from: '2026-10-14 00:00:00', warehouse_id: warehouseId,
    access: { inbound: true, outbound: true, tasks: true, inventory: true, basic: true, stocktake: true },
    warehouses: [{ id: '1', code: 'WH-SH', name: '上海中心仓' }, { id: '2', code: 'WH-HZ', name: '杭州仓' }],
    inbound: {
      statuses: [{ status: 'DRAFT', total: 12, today: total }, { status: 'SUBMITTED', total: 8, today: 0 }, { status: 'APPROVED', total: 5, today: 0 }, { status: 'RECEIVING', total: 3, today: 0 }, { status: 'PUTAWAY', total: 7, today: 0 }, { status: 'COMPLETED', total: 28, today: 0 }],
      days: [4, 8, 6, 12, 9, 14, total].map((n, i) => ({ date: `2026-10-${14 + i}`, total: n })),
      recent: Array.from({ length: 5 }, (_, i) => ({ id: String(i + 1), order_no: `RK202610200000000${i + 1}`, status: 'PUTAWAY', warehouse_id: warehouseId === '0' ? '1' : warehouseId, created_at: `2026-10-20T16:${25 - i}:00+08:00` })),
    },
    outbound: {
      statuses: [{ status: 'DRAFT', total: 6, today: 12 }, { status: 'SUBMITTED', total: 4, today: 0 }, { status: 'APPROVED', total: 7, today: 0 }, { status: 'PICKING', total: 9, today: 0 }, { status: 'SHIPPED', total: 32, today: 0 }],
      days: [3, 5, 9, 8, 11, 10, 12].map((n, i) => ({ date: `2026-10-${14 + i}`, total: n })),
      recent: Array.from({ length: 5 }, (_, i) => ({ id: String(i + 10), order_no: `CK202610200000000${i + 1}`, status: 'PICKING', warehouse_id: warehouseId === '0' ? '1' : warehouseId, created_at: `2026-10-20T16:${24 - i}:00+08:00` })),
    },
    task_counts: [{ task_type: 'RECEIVE', status: 'CREATED', total: 5, older_24h: 1 }, { task_type: 'PUTAWAY', status: 'CREATED', total: 8, older_24h: 2 }, { task_type: 'PICK', status: 'CREATED', total: 6, older_24h: 0 }, { task_type: 'PICK', status: 'IN_PROGRESS', total: 4, older_24h: 0 }],
    oldest_tasks: Array.from({ length: 8 }, (_, i) => ({ id: String(i + 1), order_id: String(i + 1), order_no: `RK202610200000000${i + 1}`, task_no: 'TASK0001', task_type: 'PUTAWAY', status: 'CREATED', warehouse_id: '1', created_at: '2026-10-19T14:00:00+08:00' })),
    inventory: { skus: 128, rows: 346, allocated_rows: 48 }, locations: { enabled: 200, occupied: 124 }, draft_stocktakes: 2,
    activity: { putaway_records: 18, shipment_records: 24, recent: Array.from({ length: 8 }, (_, i) => ({ id: String(i + 50), inventory_id: '9', trans_type: i % 2 ? 'RECEIVE' : 'SHIP', order_no: `CK202610200000000${i}`, task_no: 'TASK0001', created_at: `2026-10-20T16:${20-i}:00+08:00` })) },
    import_issues: { total: 2, recent: [{ task_id: 'import-failed', status: 'FAILED', file_name: '入库校验.xlsx', success_rows: 0, fail_rows: 1, error_msg: '第 3 行 SKU 不存在，请核对货品编码。', created_at: '2026-10-20T10:00:00+08:00' }, { task_id: 'import-partial', status: 'COMPLETED', file_name: '批量入库.xlsx', success_rows: 19, fail_rows: 1, error_msg: '<script>unsafe</script> 只是单元格文本', created_at: '2026-10-19T10:00:00+08:00' }] },
  }
}
