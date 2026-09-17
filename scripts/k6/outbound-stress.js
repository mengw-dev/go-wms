// 出库模块并发压测脚本，覆盖两类典型并发场景：
//
// 场景 A（mixed_flow，默认开启）：全流程并发
//   每个 VU 独立走 创建→提交→审核(分配)→拣货 完整链路。
//   各订单互不相同，但审核分配时争抢同一 SKU 的库存行锁（FIFO 分配按 SKU_ID 排序加 FOR UPDATE 锁），
//   正好压到：库存行锁排队、MySQL 死锁 1213 自动重试（TxRetry）、乐观锁 version 冲突。
//   库存耗尽后 approve 会业务失败（code!=0）——这是"防超卖"的正确表现，不算 HTTP 错误。
//
// 场景 B（pick_contention，需 -e ENABLE_PICK_CONTENTION=1 开启）：同单并发拣货
//   setup 阶段先创建一张大数量订单并审核通过，拿到拣货任务；
//   多个 VU 同时对同一批任务每次拣 1 件，压：task 行锁(GetForUpdate)、分配行 version 乐观锁、
//   picked_qty 原子递增、TxRetry 重试。
//   预期：恰好 target_qty 次成功，多出的请求因"任务已完成/数量超剩余"被业务拒绝（防超拣）。
//
// 前置条件：
//   - WAREHOUSE_ID、SKU_ID 真实存在
//   - 场景 A：该 SKU 可用库存 >= VU 峰值 × 迭代数 × ORDER_QTY（建议先备 500+）
//   - 场景 B：该 SKU 可用库存 >= CONTENTION_QTY（默认 100），且最好集中在单一批次（便于生成少量大任务）
//
// 运行示例：
//   k6 run -e WAREHOUSE_ID=1 -e SKU_ID=1001 scripts/k6/outbound-stress.js
//   k6 run -e WAREHOUSE_ID=1 -e SKU_ID=1001 -e ENABLE_PICK_CONTENTION=1 -e CONTENTION_QTY=100 scripts/k6/outbound-stress.js
import { check, group, sleep } from 'k6';
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

// 业务层成功/失败计数（HTTP 200 但 code!=0 算业务失败，如库存不足、状态冲突、乐观锁冲突）。
const bizSuccess = new Counter('biz_success_total');
const bizFail = new Counter('biz_fail_total');

// 场景 B 的大订单拣货总量（即并发争抢的"名额"）。
const CONTENTION_QTY = parseInt(__ENV.CONTENTION_QTY || '100', 10);
const STRESS_VUS = parseInt(__ENV.STRESS_VUS || '20', 10);
const STRESS_RAMP = __ENV.STRESS_RAMP || '20s';
const STRESS_PEAK = __ENV.STRESS_PEAK || '1m';
const STRESS_DOWN = __ENV.STRESS_DOWN || '20s';
const ENABLE_PICK_CONTENTION = __ENV.ENABLE_PICK_CONTENTION === '1';

// 记录一次业务调用的结果。
function recordBiz(res, api) {
  const ok = res.status === 200 && res.json('code') === 0;
  if (ok) {
    bizSuccess.add(1, { api });
  } else {
    bizFail.add(1, { api, code: String(res.json('code') || `http_${res.status}`) });
  }
  return ok;
}

const scenarios = {
  // 场景 A：20 VU 爬坡，全流程并发
  mixed_flow: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: STRESS_RAMP, target: Math.max(1, Math.floor(STRESS_VUS / 2)) }, // 爬坡
      { duration: STRESS_PEAK, target: STRESS_VUS },                              // 峰值保持
      { duration: STRESS_DOWN, target: 0 },                                       // 下降
    ],
    exec: 'mixedFlow',
    tags: { scenario: 'mixed_flow' },
  },
};

if (ENABLE_PICK_CONTENTION) {
  // 场景 B：30 VU 共享 400 次迭代，抢 100 个拣货名额
  scenarios.pick_contention = {
    executor: 'shared-iterations',
    vus: 30,
    iterations: CONTENTION_QTY * 4,
    startTime: '2m30s', // 等场景 A 跑完再开始，避免互相干扰
    exec: 'pickContention',
    tags: { scenario: 'pick_contention' },
  };
}

export const options = {
  scenarios,
  thresholds: {
    // HTTP 层错误（连接失败、5xx）应几乎为 0；业务失败看 biz_fail_total
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<1000', 'p(99)<2000'],
  },
};

// setup：登录；场景 B 额外准备大订单和拣货任务。
export function setup() {
  if (!WAREHOUSE_ID || !SKU_ID) {
    throw new Error('必须指定环境变量 WAREHOUSE_ID 和 SKU_ID，先运行 check-env.js 探测');
  }
  const token = login();
  const result = { token, contentionTasks: [] };

  if (ENABLE_PICK_CONTENTION) {
    // 创建大数量出库单 → 提交 → 审核分配 → 取拣货任务
    const createRes = postJSON('/api/v1/outbound/orders', token, {
      warehouse_id: WAREHOUSE_ID,
      biz_order_no: uniqueBizNo('K6LOCK'),
      remark: 'k6 同单并发拣货压测',
      details: [{ sku_id: SKU_ID, expected_qty: CONTENTION_QTY }],
    });
    const orderId = createRes.json('data.id');
    console.log(`场景B准备: order_id=${orderId} qty=${CONTENTION_QTY}`);

    postJSON(`/api/v1/outbound/orders/${orderId}/submit`, token, {});
    const approveRes = postJSON(`/api/v1/outbound/orders/${orderId}/approve`, token, {});
    if (approveRes.json('code') !== 0) {
      throw new Error(`场景B审核失败（库存不足?）: ${approveRes.body}`);
    }
    const detail = getJSON(`/api/v1/outbound/orders/${orderId}`, token);
    result.contentionTasks = (detail.json('data.tasks') || []).map((t) => t.id);
    console.log(`场景B就绪: 拣货任务数=${result.contentionTasks.length}，总名额=${CONTENTION_QTY}`);
  }

  return result;
}

// 场景 A：每个 VU 独立完成一张出库单的全流程。
export function mixedFlow(data) {
  const token = data.token;
  let orderId = 0;

  group('A-创建+提交+审核', () => {
    const createRes = postJSON('/api/v1/outbound/orders', token, {
      warehouse_id: WAREHOUSE_ID,
      biz_order_no: uniqueBizNo('K6STRESS'),
      remark: 'k6 并发压测',
      details: [{ sku_id: SKU_ID, expected_qty: ORDER_QTY }],
    });
    if (!recordBiz(createRes, 'create')) {
      return; // 创建失败（参数/单号冲突），本次迭代终止
    }
    orderId = createRes.json('data.id');

    recordBiz(postJSON(`/api/v1/outbound/orders/${orderId}/submit`, token, {}), 'submit');

    const approveRes = postJSON(`/api/v1/outbound/orders/${orderId}/approve`, token, {});
    if (!recordBiz(approveRes, 'approve')) {
      // 常见业务失败：可用库存不足（防超卖生效）。不继续拣货。
      return;
    }
  });

  if (!orderId) {
    return;
  }

  group('A-拣货至发货', () => {
    const detail = getJSON(`/api/v1/outbound/orders/${orderId}`, token);
    const tasks = detail.json('data.tasks') || [];
    tasks.forEach((t) => {
      const pickRes = postJSON(`/api/v1/outbound/tasks/${t.id}/pick`, token, {
        task_id: t.id,
        qty: t.target_qty,
      });
      recordBiz(pickRes, 'pick');
      sleep(0.05);
    });
  });

  sleep(0.5); // 模拟操作员操作间隔
}

// 场景 B：所有 VU 争抢同一批拣货任务，每次拣 1 件。
export function pickContention(data) {
  const tasks = data.contentionTasks;
  if (tasks.length === 0) {
    return;
  }
  const token = data.token;
  // 按迭代号轮询选择任务，让多个 VU 打到同一个 task 上形成竞争
  const taskId = tasks[__ITER % tasks.length];

  const res = postJSON(`/api/v1/outbound/tasks/${taskId}/pick`, token, {
    task_id: taskId,
    qty: 1,
  });
  recordBiz(res, 'pick_contention');
  // 不做 check 断言：业务失败（任务已满/状态冲突）是防超拣的预期行为，
  // 压测结束后对比 biz_success_total 中 pick_contention 成功数 ≈ CONTENTION_QTY 即可。
  sleep(0.02);
}
