// 库存并发分配正确性证明：N 个 VU 同时审核 N 张出库单，争抢同一 SKU 的有限库存。
//
// 目的：用 teardown 的业务不变量断言，证明并发分配既不超卖、也不丢分配。
//
// 前置条件：
//   - 目标 SKU 的 allocated 必须为 0，且 available <= TARGET_STOCK（脚本会先补货到 TARGET_STOCK）
//   - 使用持久体验账号（-e K6_USERNAME=user1）：admin 会产生租户 0 孤儿行，demo 账号有会话锁与数据重置
//
// 运行示例：
//   k6 run -e WAREHOUSE_ID=1 -e SKU_ID=1001 -e K6_USERNAME=user1 scripts/k6/allocation-proof.js
import { check } from 'k6';
import { Counter } from 'k6/metrics';
import {
  WAREHOUSE_ID,
  SKU_ID,
  login,
  postJSON,
  getJSON,
  uniqueBizNo,
} from './lib.js';

const VUS = parseInt(__ENV.VUS || '60', 10);
const ORDER_QTY = parseInt(__ENV.ORDER_QTY || '100', 10);
const TARGET_STOCK = parseInt(__ENV.TARGET_STOCK || '4000', 10);

// 业务结果分类：2xx+code=0 为成功；4xx 为业务拒绝（库存不足属预期）；5xx/网络异常才算系统失败。
const approveSuccess = new Counter('approve_success_total');
const approveReject = new Counter('approve_reject_total');
const approveSystemFail = new Counter('approve_system_fail_total');

export const options = {
  scenarios: {
    // 每个 VU 只审核一张单据，让 N 个请求尽可能同时落到同一 SKU 的库存行上。
    allocation: {
      executor: 'per-vu-iterations',
      vus: VUS,
      iterations: 1,
      maxDuration: '30s',
      exec: 'approveOne',
    },
  },
  thresholds: {
    // 网络错误/5xx 应接近 0；业务拒绝不计入 HTTP 失败。
    http_req_failed: ['rate<0.01'],
    // 性能指标，不是防超卖正确性指标。
    http_req_duration: ['p(95)<1000'],
    // teardown 的 9 条业务不变量必须全部通过，k6 才算成功（退出码 0）。
    // check 默认不影响退出码，必须显式加这个阈值，否则不变量红了 CI 仍会绿。
    checks: ['rate==1.0'],
  },
};

// 准备阶段的一次性调用：失败直接抛错终止。
// 这里刻意不用 check()：setup 要建 60 张单，每次 check 都会在汇总里留下一条记录，
// 等于用上百条噪声淹没真正重要的不变量断言。
function must(res, name) {
  let code = null;
  try {
    code = res.json('code');
  } catch (_) {
    // 响应不是 JSON（网络层出错），交给下面统一抛出。
  }
  if (res.status < 200 || res.status >= 300 || code !== 0) {
    throw new Error(`${name}失败: status=${res.status}, body=${res.body}`);
  }
}

// 汇总目标 SKU 在目标仓库下的所有库存行。
function getInventory(token) {
  const res = getJSON(
    `/api/v1/inventory?warehouse_id=${WAREHOUSE_ID}&sku_id=${SKU_ID}&page=1&page_size=100`,
    token
  );
  must(res, '查询库存');

  const rows = res.json('data.list') || [];
  const stock = rows.reduce((sum, row) => sum + Number(row.stock_quantity || 0), 0);
  const available = rows.reduce((sum, row) => sum + Number(row.available_quantity || 0), 0);
  const allocated = rows.reduce((sum, row) => sum + Number(row.allocated_quantity || 0), 0);

  return { rows, stock, available, allocated };
}

// 找一个可用于上架的启用库位。
function getLocation(token) {
  const res = getJSON(
    `/api/v1/basic/locations?warehouse_id=${WAREHOUSE_ID}&page=1&page_size=100`,
    token
  );
  must(res, '查询库位');

  const list = res.json('data.list') || [];
  const location = list.find((item) => item.status === 1) || list.find((item) => item.status !== 0);
  if (!location) {
    throw new Error('没有找到可用库位');
  }
  return location;
}

// 把目标 SKU 的可用库存补到 TARGET_STOCK，返回补货后的库存快照。
function prepareInventory(token) {
  let inventory = getInventory(token);
  console.log(`准备前库存: stock=${inventory.stock}, available=${inventory.available}, allocated=${inventory.allocated}`);

  // 实验要求起点干净：已有分配量说明该 SKU 正被其他流程占用。
  if (inventory.allocated !== 0) {
    throw new Error(`目标 SKU 当前 allocated=${inventory.allocated}，请先清理测试数据或更换测试 SKU`);
  }
  // 只补不裁：多于目标说明环境不干净，直接失败而不是改动存量。
  if (inventory.available > TARGET_STOCK) {
    throw new Error(`当前 available=${inventory.available} > 目标 ${TARGET_STOCK}，请使用干净测试环境`);
  }
  if (inventory.available === TARGET_STOCK) {
    return inventory;
  }

  const need = TARGET_STOCK - inventory.available;
  console.log(`需要补货 ${need} 件，使 available 达到 ${TARGET_STOCK}`);

  const location = getLocation(token);

  const createRes = postJSON('/api/v1/inbound/orders', token, {
    warehouse_id: WAREHOUSE_ID,
    remark: 'k6 并发分配测试准备库存',
    details: [{ sku_id: SKU_ID, expected_qty: need }],
  });
  must(createRes, '创建入库单');
  const inboundOrderId = createRes.json('data.id');

  must(postJSON(`/api/v1/inbound/orders/${inboundOrderId}/submit`, token, {}), '提交入库单');
  must(postJSON(`/api/v1/inbound/orders/${inboundOrderId}/approve`, token, {}), '审核入库单');

  let detailRes = getJSON(`/api/v1/inbound/orders/${inboundOrderId}`, token);
  must(detailRes, '查询入库单');
  const detail = (detailRes.json('data.details') || [])[0];
  if (!detail) {
    throw new Error('没有找到入库明细');
  }

  must(
    postJSON(`/api/v1/inbound/orders/${inboundOrderId}/receive`, token, {
      detail_id: detail.id,
      qty: need,
      defective_qty: 0,
      batch_no: `K6-${Date.now()}`,
    }),
    '收货'
  );

  detailRes = getJSON(`/api/v1/inbound/orders/${inboundOrderId}`, token);
  must(detailRes, '查询上架任务');
  const putawayTask = (detailRes.json('data.tasks') || []).find(
    (task) => task.task_type === 'PUTAWAY' && task.done_qty < task.target_qty
  );
  if (!putawayTask) {
    throw new Error('没有生成上架任务');
  }

  must(
    postJSON(`/api/v1/inbound/tasks/${putawayTask.id}/putaway`, token, {
      task_id: putawayTask.id,
      location_id: location.id,
      qty: putawayTask.target_qty - putawayTask.done_qty,
    }),
    '上架'
  );

  inventory = getInventory(token);
  console.log(`准备后库存: stock=${inventory.stock}, available=${inventory.available}, allocated=${inventory.allocated}`);

  const ok = check(inventory, {
    '初始 stock=4000': (x) => x.stock === TARGET_STOCK,
    '初始 available=4000': (x) => x.available === TARGET_STOCK,
    '初始 allocated=0': (x) => x.allocated === 0,
  });
  if (!ok) {
    throw new Error('测试库存准备失败');
  }

  return inventory;
}

// setup 只在所有并发请求开始前执行一次。
export function setup() {
  if (!WAREHOUSE_ID || !SKU_ID) {
    throw new Error('必须设置 WAREHOUSE_ID 和 SKU_ID');
  }
  const token = login();

  const before = prepareInventory(token);

  // 提前把 VUS 张出库单提交到 SUBMITTED，让并发阶段只剩 approve 一个动作，
  // 保证竞争点是库存分配本身，而不是建单与提交的耗时。
  const orderIds = [];
  for (let i = 0; i < VUS; i++) {
    const createRes = postJSON('/api/v1/outbound/orders', token, {
      warehouse_id: WAREHOUSE_ID,
      biz_order_no: uniqueBizNo(`K6PROOF${i}`),
      remark: 'k6 60并发库存分配证明',
      details: [{ sku_id: SKU_ID, expected_qty: ORDER_QTY }],
    });
    must(createRes, `创建订单 ${i + 1}`);
    const orderId = createRes.json('data.id');

    const submitRes = postJSON(`/api/v1/outbound/orders/${orderId}/submit`, token, {});
    must(submitRes, `提交订单 ${i + 1}`);
    orderIds.push(orderId);
  }

  console.log(`准备完成：${orderIds.length} 张订单`);
  console.log(`库存=${before.available}，总需求=${VUS * ORDER_QTY}`);
  console.log('========== 即将开始并发审核 ==========');

  // setup 的返回值会作为 data 传给每个 VU。
  return { token, orderIds, before };
}

// 并发阶段：每个 VU 只审核一张单据。
export function approveOne(data) {
  // __VU 从 1 开始：VU1 审核 orderIds[0]，VU2 审核 orderIds[1]，依此类推。
  const orderId = data.orderIds[__VU - 1];

  const res = postJSON(`/api/v1/outbound/orders/${orderId}/approve`, data.token, {});

  let code = null;
  try {
    code = res.json('code');
  } catch (_) {
    // 网络错误或非 JSON 响应，交给下面的分类处理。
  }

  const success = res.status >= 200 && res.status < 300 && code === 0;
  if (success) {
    approveSuccess.add(1);
    return;
  }

  // 4xx 是业务拒绝：库存被别的 VU 抢完后，剩余订单审核失败正是预期结果。
  if (res.status >= 400 && res.status < 500) {
    approveReject.add(1);
    return;
  }

  // 只有 5xx / 网络异常才算真正的系统错误。
  approveSystemFail.add(1);
}

// teardown 在所有并发请求结束后执行一次，负责业务不变量验证。
export function teardown(data) {
  console.log('========== 并发审核结束 ==========');

  const after = getInventory(data.token);
  console.log(`测试前: stock=${data.before.stock}, available=${data.before.available}, allocated=${data.before.allocated}`);
  console.log(`测试后: stock=${after.stock}, available=${after.available}, allocated=${after.allocated}`);

  // 逐单核对最终状态：审核成功推进到 PICKING；因库存不足整体回滚则停在 SUBMITTED。
  let approved = 0;
  let rejected = 0;
  let unexpected = 0;

  for (let i = 0; i < data.orderIds.length; i++) {
    const orderId = data.orderIds[i];
    const res = getJSON(`/api/v1/outbound/orders/${orderId}`, data.token);
    if (res.status < 200 || res.status >= 300 || res.json('code') !== 0) {
      unexpected++;
      continue;
    }

    const status = res.json('data.order.status');
    if (status === 'PICKING') {
      approved++;
    } else if (status === 'SUBMITTED') {
      rejected++;
    } else {
      unexpected++;
    }
  }

  // 理论结果：现有库存最多成全几张整单，再与订单总数 VUS 取小
  // （库存远多于需求时要封顶，否则会算出「理论成功 4000 / 理论拒绝 -3940」这种荒谬值）。
  const expectedApproved = Math.min(VUS, Math.floor(data.before.available / ORDER_QTY));
  const expectedRejected = VUS - expectedApproved;
  const allocatedIncrease = after.allocated - data.before.allocated;
  const availableDecrease = data.before.available - after.available;

  console.log('');
  console.log('========== 业务结果 ==========');
  console.log(`订单总数: ${VUS}`);
  console.log(`单订单需求: ${ORDER_QTY}`);
  console.log(`总需求: ${VUS * ORDER_QTY}`);
  console.log(`理论成功: ${expectedApproved}`);
  console.log(`实际成功: ${approved}`);
  console.log(`理论拒绝: ${expectedRejected}`);
  console.log(`实际拒绝: ${rejected}`);

  // 核心：9 条业务不变量，任何一条失败都说明并发分配有正确性问题。
  check(after, {
    // 1. 任何库存行都不能出现负数。
    '无负库存': (x) =>
      x.rows.every(
        (row) =>
          Number(row.stock_quantity) >= 0 &&
          Number(row.available_quantity) >= 0 &&
          Number(row.allocated_quantity) >= 0
      ),

    // 2. 每条库存行都必须满足三数量模型。
    '每条库存满足 stock=available+allocated': (x) =>
      x.rows.every(
        (row) =>
          Number(row.stock_quantity) ===
          Number(row.available_quantity) + Number(row.allocated_quantity)
      ),

    // 3. 汇总层面同样成立。
    '汇总库存满足 stock=available+allocated': (x) => x.stock === x.available + x.allocated,

    // 4. 分配只改变 available/allocated，stock 总量不变。
    '库存总量前后不变': (x) => x.stock === data.before.stock,

    // 5. available 减少量必须等于 allocated 增加量。
    '可用减少量=分配增加量': () => availableDecrease === allocatedIncrease,

    // 6. 成功订单数必须等于理论可分配数。
    '成功订单数量正确': () => approved === expectedApproved,

    // 7. 剩余订单必须被拒绝。
    '库存不足订单被正确拒绝': () => rejected === expectedRejected,

    // 8. 不允许出现未知订单状态。
    '没有异常订单': () => unexpected === 0,

    // 9. 分配增量必须等于成功订单数 × 单订单数量。
    '实际分配量=成功订单数×单订单数量': () => allocatedIncrease === approved * ORDER_QTY,
  });

  console.log('');
  console.log('========== 最终结论 ==========');
  console.log(`stock: ${after.stock}`);
  console.log(`available: ${after.available}`);
  console.log(`allocated: ${after.allocated}`);
  console.log(`approved: ${approved}`);
  console.log(`rejected: ${rejected}`);
}
