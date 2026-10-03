// 并发审核测试（入门版）：N 个 VU 同时审核 N 张出库单，抢同一个 SKU 的库存。
//
// 只回答一个问题：并发审核会不会超卖？
// 想看完整版（含自动备货入库全流程、9 条不变量断言）请读同目录的 allocation-proof.js。
//
// 前置条件：
//   - 目标 SKU 已有可用库存（本脚本不自动备货；库存为 0 会直接报错提示）
//   - 用持久体验账号运行：admin 会产生租户 0 孤儿行，demo 账号有会话锁和数据重置
//
// 运行示例：
//   # 温和场景：每单 1 件，库存充足时全部审核成功
//   k6 run -e WAREHOUSE_ID=1 -e SKU_ID=1001 -e K6_USERNAME=user1 scripts/k6/approve-concurrency.js
//   # 抢库存场景：每单 1000 件，总需求超过库存，能看到部分被拒绝
//   k6 run -e WAREHOUSE_ID=1 -e SKU_ID=1001 -e K6_USERNAME=user1 -e VUS=10 -e ORDER_QTY=1000 scripts/k6/approve-concurrency.js
import { check } from 'k6';
import { Counter } from 'k6/metrics';
import {
  WAREHOUSE_ID,
  SKU_ID,
  ORDER_QTY,
  login,
  postJSON,
  getJSON,
  uniqueBizNo,
} from './lib.js';

// 并发人数 = 要审核的单据数。调大竞争更激烈。
const VUS = parseInt(__ENV.VUS || '10', 10);

// 审核结果分类计数，k6 结束时会在汇总里打印这三个指标。
const approveSuccess = new Counter('approve_success_total'); // 抢到库存，审核通过
const approveReject = new Counter('approve_reject_total'); // 库存被抢完，后端业务拒绝
const approveSystemFail = new Counter('approve_system_fail_total'); // 5xx / 网络异常，才是真出错

export const options = {
  scenarios: {
    // per-vu-iterations：每个 VU 只跑 1 次，N 个请求几乎同时发出。
    // 对比 ramping-vus 会错开发请求，抢不到行锁，压不出并发问题。
    approve: {
      executor: 'per-vu-iterations',
      vus: VUS,
      iterations: 1,
      maxDuration: '30s',
      exec: 'approveOne', // 并发阶段执行哪个函数
    },
  },
  thresholds: {
    // 网络错误/5xx 占比必须 < 1%。库存不足的 400 属业务拒绝，不计入 http_req_failed。
    http_req_failed: ['rate<0.01'],
  },
};

// 准备阶段的一次性调用：失败直接抛错终止。
// 不用 check()，否则每建一张单都会在汇总里多出两条记录，把 teardown 的断言淹掉。
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

// 读一次库存，返回明细行 + 三个汇总量（三者满足 stock = available + allocated）。
function getInventory(token) {
  const res = getJSON(
    `/api/v1/inventory?warehouse_id=${WAREHOUSE_ID}&sku_id=${SKU_ID}&page=1&page_size=100`,
    token
  );
  must(res, '查询库存');

  const rows = res.json('data.list') || [];
  return {
    rows,
    stock: rows.reduce((sum, row) => sum + Number(row.stock_quantity || 0), 0),
    available: rows.reduce((sum, row) => sum + Number(row.available_quantity || 0), 0),
    allocated: rows.reduce((sum, row) => sum + Number(row.allocated_quantity || 0), 0),
  };
}

// setup 在所有并发请求之前执行一次：登录、看库存、把单据准备好。
export function setup() {
  if (!WAREHOUSE_ID || !SKU_ID) {
    throw new Error('必须设置 WAREHOUSE_ID 和 SKU_ID');
  }

  const token = login();
  const before = getInventory(token);
  if (before.available <= 0) {
    throw new Error(`SKU ${SKU_ID} 当前可用库存为 0，请先在页面上给它备货`);
  }

  // 提前创建并提交单据到 SUBMITTED，并发阶段就只剩 approve 一个动作。
  const orderIds = [];
  for (let i = 0; i < VUS; i++) {
    const createRes = postJSON('/api/v1/outbound/orders', token, {
      warehouse_id: WAREHOUSE_ID,
      biz_order_no: uniqueBizNo(`K6APPROVE${i}`), // 业务单号必须唯一
      remark: 'k6 并发审核测试',
      details: [{ sku_id: SKU_ID, expected_qty: ORDER_QTY }],
    });
    must(createRes, '创建订单');
    const orderId = createRes.json('data.id');

    const submitRes = postJSON(`/api/v1/outbound/orders/${orderId}/submit`, token, {});
    must(submitRes, '提交订单');
    orderIds.push(orderId);
  }

  console.log(
    `准备完成: ${VUS} 张待审核单据, 可用库存 ${before.available}, 总需求 ${VUS * ORDER_QTY}`
  );

  // setup 的返回值会作为参数传给每个 VU 和 teardown。
  return { token, orderIds, before };
}

// 并发阶段：每个 VU 审核一张单据。
export function approveOne(data) {
  // __VU 从 1 开始，数组下标从 0 开始，所以要减 1：VU1 审第 1 张，VU2 审第 2 张……
  const orderId = data.orderIds[__VU - 1];

  const res = postJSON(`/api/v1/outbound/orders/${orderId}/approve`, data.token, {});

  let code = null;
  try {
    code = res.json('code');
  } catch (_) {
    // 响应不是 JSON（网络层出错），交给下面的兜底分支。
  }

  if (res.status >= 200 && res.status < 300 && code === 0) {
    approveSuccess.add(1); // 抢到库存
  } else if (res.status >= 400 && res.status < 500) {
    approveReject.add(1); // 库存已抢完而被拒——这正是防超卖的预期表现
  } else {
    approveSystemFail.add(1); // 5xx / 网络异常
  }
}

// teardown 在所有 VU 跑完后执行一次：核对库存，确认没有超卖。
export function teardown(data) {
  const after = getInventory(data.token);

  // 理论结果：现有库存最多成全几张整单，其余必然被拒。
  const expectedSuccess = Math.min(VUS, Math.floor(data.before.available / ORDER_QTY));
  const expectedReject = VUS - expectedSuccess;
  const availableDecrease = data.before.available - after.available;
  const allocatedIncrease = after.allocated - data.before.allocated;

  console.log(
    `审核前: stock=${data.before.stock} available=${data.before.available} allocated=${data.before.allocated}`
  );
  console.log(`审核后: stock=${after.stock} available=${after.available} allocated=${after.allocated}`);
  console.log(
    `理论上: 成功 ${expectedSuccess} 张 / 拒绝 ${expectedReject} 张 ` +
      `(可用库存 ${data.before.available}, 每张要 ${ORDER_QTY})`
  );
  console.log('实际成功/拒绝数量请看上方 approve_success_total / approve_reject_total');

  // 是否超卖，看下面几条不变量是否同时成立。
  check(after, {
    // 1. 任何库存行都不能为负（超卖的典型表现就是 available 变成负数）。
    '无负库存': (x) =>
      x.rows.every(
        (row) =>
          Number(row.stock_quantity) >= 0 &&
          Number(row.available_quantity) >= 0 &&
          Number(row.allocated_quantity) >= 0
      ),

    // 2. 库存守恒：审核只是把 available 挪进 allocated，总量不变。
    '每条库存满足 stock=available+allocated': (x) =>
      x.rows.every(
        (row) =>
          Number(row.stock_quantity) === Number(row.available_quantity) + Number(row.allocated_quantity)
      ),
    '库存总量前后不变': (x) => x.stock === data.before.stock,

    // 3. 可用量减少的部分必须全部变成已分配量（不能凭空多出分配）。
    '可用减少量=分配增加量': () => availableDecrease === allocatedIncrease,

    // 4. 分配出去的总量不能超过原有可用库存（核心防超卖断言）。
    '分配总量不超过库存总量': () => allocatedIncrease <= data.before.available,
  });
}