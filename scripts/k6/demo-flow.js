// 面向体验者/面试官演示的完整业务流程脚本：
// 查询基础资料 -> 入库建单 -> 审核 -> 收货 -> 上架 -> 出库审核分配 -> 拣货发货 -> 盘点 -> 结果核对。
//
// 这是 1 VU 正常操作流程，适合演示“系统怎么工作”；并发能力请接着运行
// outbound-stress.js 或 pick-stress.js。
//
// 运行：
//   k6 run scripts/k6/demo-flow.js
//   k6 run -e BASE_URL=http://127.0.0.1:8080 -e WAREHOUSE_ID=21 -e SKU_ID=69 scripts/k6/demo-flow.js
import { check, group, sleep } from 'k6';
import {
  WAREHOUSE_ID,
  SKU_ID,
  login,
  postJSON,
  getJSON,
  checkBizOK,
  uniqueBizNo,
} from './lib.js';

const WAREHOUSE_CODE = __ENV.WAREHOUSE_CODE || 'WH01';
const SKU_CODE = __ENV.SKU_CODE || 'SKU000001';
const INBOUND_QTY = parseInt(__ENV.INBOUND_QTY || '20', 10);
const OUTBOUND_QTY = parseInt(__ENV.OUTBOUND_QTY || '10', 10);

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1'],
  },
};

function must(res, name) {
  if (!checkBizOK(res, name)) {
    throw new Error(`${name} 失败: status=${res.status} body=${res.body}`);
  }
}

function first(list) {
  return list && list.length > 0 ? list[0] : null;
}

export function setup() {
  const token = login();
  let warehouseId = WAREHOUSE_ID;
  let skuId = SKU_ID;

  if (!warehouseId) {
    const res = getJSON(
      `/api/v1/basic/warehouses?keyword=${encodeURIComponent(WAREHOUSE_CODE)}&page=1&page_size=10`,
      token
    );
    must(res, `查找仓库 ${WAREHOUSE_CODE}`);
    const warehouse = (res.json('data.list') || []).find((w) => w.code === WAREHOUSE_CODE);
    if (!warehouse) {
      throw new Error(`未找到仓库 ${WAREHOUSE_CODE}`);
    }
    warehouseId = warehouse.id;
  }

  if (!skuId) {
    const res = getJSON(
      `/api/v1/basic/skus?keyword=${encodeURIComponent(SKU_CODE)}&page=1&page_size=10`,
      token
    );
    must(res, `查找货品 ${SKU_CODE}`);
    const sku = (res.json('data.list') || []).find((s) => s.code === SKU_CODE);
    if (!sku) {
      throw new Error(`未找到货品 ${SKU_CODE}`);
    }
    skuId = sku.id;
  }

  const locRes = getJSON(
    `/api/v1/basic/locations?warehouse_id=${warehouseId}&page=1&page_size=100`,
    token
  );
  must(locRes, '查询库位');
  const locations = locRes.json('data.list') || [];
  const location = locations.find((item) => item.status === 1) || locations.find((item) => item.status !== 0);
  if (!location) {
    throw new Error(`仓库 ${warehouseId} 没有可用库位，请先执行演示数据初始化`);
  }

  console.log(
    `演示参数: warehouse_id=${warehouseId}, sku_id=${skuId}, location=${location.code}, inbound=${INBOUND_QTY}, outbound=${OUTBOUND_QTY}`
  );
  return { token, warehouseId, skuId, location };
}

export default function (data) {
  const token = data.token;
  const warehouseId = data.warehouseId;
  const skuId = data.skuId;
  const location = data.location;
  let inboundOrderId = '';
  let outboundOrderId = '';
  let stocktakeOrderId = '';

  group('1. 入库：建单、审核、收货、上架', () => {
    const create = postJSON('/api/v1/inbound/orders', token, {
      warehouse_id: warehouseId,
      remark: '演示：正常入库作业',
      details: [{ sku_id: skuId, expected_qty: INBOUND_QTY }],
    });
    must(create, '创建入库单');
    inboundOrderId = create.json('data.id');
    console.log(`  入库单创建: order_id=${inboundOrderId} order_no=${create.json('data.order_no')}`);

    must(postJSON(`/api/v1/inbound/orders/${inboundOrderId}/submit`, token, {}), '提交入库单');
    must(postJSON(`/api/v1/inbound/orders/${inboundOrderId}/approve`, token, {}), '审核入库单');

    const detail = getJSON(`/api/v1/inbound/orders/${inboundOrderId}`, token);
    must(detail, '查询入库明细');
    const inboundDetail = first(detail.json('data.details'));
    if (!inboundDetail) {
      throw new Error('入库明细为空');
    }

    const batchNo = `B${Date.now()}`;
    must(
      postJSON(`/api/v1/inbound/orders/${inboundOrderId}/receive`, token, {
        detail_id: inboundDetail.id,
        qty: INBOUND_QTY,
        defective_qty: 0,
        batch_no: batchNo,
      }),
      '收货'
    );

    const afterReceive = getJSON(`/api/v1/inbound/orders/${inboundOrderId}`, token);
    must(afterReceive, '查询上架任务');
    const putawayTask = (afterReceive.json('data.tasks') || []).find(
      (task) => task.task_type === 'PUTAWAY' && task.done_qty < task.target_qty
    );
    if (!putawayTask) {
      throw new Error('未生成上架任务');
    }
    must(
      postJSON(`/api/v1/inbound/tasks/${putawayTask.id}/putaway`, token, {
        task_id: putawayTask.id,
        location_id: location.id,
        qty: putawayTask.target_qty - putawayTask.done_qty,
      }),
      '上架'
    );
    console.log(`  收货并上架完成: batch=${batchNo}, location=${location.code}`);
  });

  sleep(0.2);

  group('2. 出库：建单、审核 FIFO 分配、拣货发货', () => {
    const create = postJSON('/api/v1/outbound/orders', token, {
      warehouse_id: warehouseId,
      biz_order_no: uniqueBizNo('CUST'),
      remark: '演示：客户订单出库',
      details: [{ sku_id: skuId, expected_qty: OUTBOUND_QTY }],
    });
    must(create, '创建出库单');
    outboundOrderId = create.json('data.id');
    console.log(`  出库单创建: order_id=${outboundOrderId} order_no=${create.json('data.order_no')}`);

    must(postJSON(`/api/v1/outbound/orders/${outboundOrderId}/submit`, token, {}), '提交出库单');
    must(postJSON(`/api/v1/outbound/orders/${outboundOrderId}/approve`, token, {}), '审核并分配库存');

    const detail = getJSON(`/api/v1/outbound/orders/${outboundOrderId}`, token);
    must(detail, '查询拣货任务');
    const tasks = detail.json('data.tasks') || [];
    if (tasks.length === 0) {
      throw new Error('未生成拣货任务');
    }
    tasks.forEach((task) => {
      if (task.done_qty >= task.target_qty) {
        return;
      }
      must(
        postJSON(`/api/v1/outbound/tasks/${task.id}/pick`, token, {
          task_id: task.id,
          qty: task.target_qty - task.done_qty,
        }),
        `拣货任务 ${task.id}`
      );
    });
    const shipped = getJSON(`/api/v1/outbound/orders/${outboundOrderId}`, token);
    must(shipped, '查询出库终态');
    check(null, {
      '出库单最终为 SHIPPED': () => shipped.json('data.order.status') === 'SHIPPED',
      '拣货数量等于需求量': () => shipped.json('data.order.picked_qty') === OUTBOUND_QTY,
    });
    console.log(`  出库完成: status=${shipped.json('data.order.status')} picked=${shipped.json('data.order.picked_qty')}`);
  });

  sleep(0.2);

  group('3. 盘点：快照、实盘、审核调整', () => {
    const create = postJSON('/api/v1/stocktake/orders', token, {
      warehouse_id: warehouseId,
      remark: '演示：库存盘点与差异调整',
    });
    must(create, '创建盘点单');
    stocktakeOrderId = create.json('data.id');
    console.log(`  盘点单创建: order_id=${stocktakeOrderId} order_no=${create.json('data.order_no')}`);

    const detail = getJSON(`/api/v1/stocktake/orders/${stocktakeOrderId}`, token);
    must(detail, '查询盘点快照');
    const item = first(detail.json('data.details'));
    if (!item) {
      throw new Error('盘点明细为空');
    }
    const actualQty = Math.max(0, item.book_qty - 1);
    must(
      postJSON(`/api/v1/stocktake/orders/${stocktakeOrderId}/actual`, token, {
        detail_id: item.id,
        actual_qty: actualQty,
      }),
      '录入实盘数量'
    );
    must(postJSON(`/api/v1/stocktake/orders/${stocktakeOrderId}/approve`, token, {}), '审核盘点单');
    console.log(`  盘点完成: book=${item.book_qty}, actual=${actualQty}, diff=${actualQty - item.book_qty}`);
  });

  group('4. 结果核对', () => {
    const inventory = getJSON(
      `/api/v1/inventory?warehouse_id=${warehouseId}&sku_id=${skuId}&page=1&page_size=100`,
      token
    );
    must(inventory, '查询最终库存');
    const rows = inventory.json('data.list') || [];
    const negative = rows.filter((row) => row.stock_quantity < 0 || row.available_quantity < 0 || row.allocated_quantity < 0);
    const stockSum = rows.reduce((sum, row) => sum + (row.stock_quantity || 0), 0);
    const availableSum = rows.reduce((sum, row) => sum + (row.available_quantity || 0), 0);
    const allocatedSum = rows.reduce((sum, row) => sum + (row.allocated_quantity || 0), 0);

    check(null, {
      '库存不为负数': () => negative.length === 0,
      '库存总量 = 可用量 + 分配量': () => stockSum === availableSum + allocatedSum,
      '目标货品存在库存行': () => rows.length > 0,
    });
    console.log(
      `  最终库存: rows=${rows.length} stock=${stockSum} available=${availableSum} allocated=${allocatedSum}`
    );
  });
}