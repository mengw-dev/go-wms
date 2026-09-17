// 贴近现场的并发拣货压测（大仓早班波次模型）。
//
// 业务建模：
//   1. setup 阶段通过【真实入库流程】铺货：ROWS 个库位 × 每库位 QTY_PER_ROW 件，
//      每个库位独立批次，共 ROWS 条库存行（wms_inventory 按 仓库+库位+SKU+批次 唯一）。
//   2. 创建并审核一张 TOTAL 件的出库大单，FIFO 分配横跨 ROWS 个库存行
//      → 生成 ROWS 个拣货任务（一库存行一任务），每个任务 target ≈ QTY_PER_ROW。
//   3. wave 场景：WORKERS 个拣货员 VU，每人按 stride 分配到属于自己的任务（各拣各的），
//      逐件扫码 qty=1 直到拣满——真实 PDA 节奏（可用 SCAN_INTERVAL 调成 3 秒模拟人工）。
//   4. chaos 场景：额外 CHAOS 个 VU 反复随机抢拣同一批任务（模拟重复扫码/误派两人），
//      压防超拣。预期：绝大部分被业务拒绝（TaskQtyOver/TaskStatusWrong/Conflict 重试后失败）。
//
// 正确性判据（最重要，压完看 teardown 输出）：
//   - 出库单终态 SHIPPED，picked_qty 必须【恰好等于】TOTAL（多一件=超拣漏洞）
//   - 全部拣货任务 COMPLETED，done_qty == target_qty
//   - 所有库存行 stock_quantity >= 0（不能扣成负数）
//   - pick_ok_total 指标【恰好】等于 TOTAL；pick_reject_total 再多都属正常（防超拣生效）
//
// 运行（真实规模默认：40 库位 × 100 件 = 4000 件，40 拣货员 + 20 抢单 VU）：
//   k6 run -e PASSWORD=xxx -e WAREHOUSE_ID=1 -e SKU_ID=1001 scripts/k6/pick-stress.js
//
// 常用调节：
//   -e ROWS=60 -e QTY_PER_ROW=200     60 库位 × 200 件 = 12000 件
//   -e WORKERS=60                      60 个拣货员
//   -e CHAOS=0                         关闭抢单竞争，纯各拣各的
//   -e SCAN_INTERVAL=3                 模拟真人 3 秒扫一件（默认 0.05s 压满测）
import { check, sleep, group } from 'k6';
import { Counter } from 'k6/metrics';
import http from 'k6/http';
import {
  BASE_URL,
  WAREHOUSE_ID,
  SKU_ID,
  login,
  postJSON,
  getJSON,
  checkBizOK,
  uniqueBizNo,
  authHeaders,
} from './lib.js';

// ---------- 规模参数（环境变量可覆盖） ----------
const ROWS = parseInt(__ENV.ROWS || '40', 10);            // 库存行数 ≈ 拣货任务数
const QTY_PER_ROW = parseInt(__ENV.QTY_PER_ROW || '100', 10); // 每库位/任务件数
const TOTAL = ROWS * QTY_PER_ROW;                          // 大单总件数
const WORKERS = parseInt(__ENV.WORKERS || String(ROWS), 10); // 拣货员数（默认一人一任务）
const CHAOS = parseInt(__ENV.CHAOS || '20', 10);           // 额外抢单 VU 数（0 关闭）
const SCAN_INTERVAL = parseFloat(__ENV.SCAN_INTERVAL || '0.05'); // 扫码间隔秒
const ZONE = `K6${Date.now().toString(36).toUpperCase()}`;     // 本次压测专属库区，避免历史数据干扰

// ---------- 指标 ----------
const pickOk = new Counter('pick_ok_total');        // 拣货业务成功次数（期望 == TOTAL）
const pickReject = new Counter('pick_reject_total'); // 拣货业务拒绝次数（防超拣，预期较多）

const scenarios = {
  // 主场景：拣货员各负责任务，逐件扫码
  wave: {
    executor: 'per-vu-iterations',
    vus: WORKERS,
    iterations: 1,
    exec: 'wavePick',
    tags: { scenario: 'wave' },
  },
};
if (CHAOS > 0) {
  // 竞争场景：额外 VU 随机抢拣同一批任务
  scenarios.chaos = {
    executor: 'per-vu-iterations',
    vus: CHAOS,
    iterations: QTY_PER_ROW * 2, // 每人尝试 2 倍任务量，确保过量
    exec: 'chaosPick',
    startTime: '0s',
    tags: { scenario: 'chaos' },
  };
}

export const options = {
  setupTimeout: '10m',
  scenarios,
  thresholds: {
    http_req_failed: ['rate<0.02'],         // HTTP 层错误（连接/5xx）应 <2%
    http_req_duration: ['p(95)<1500', 'p(99)<3000'],
    checks: ['rate>0.95'],
  },
};

// ---------- 工具 ----------
function must(res, name) {
  if (!checkBizOK(res, name)) {
    throw new Error(`${name} 失败: ${res.status} ${res.body}`);
  }
}

// 通过完整入库流程在指定库位/批次铺一笔库存（创建→提交→审核→收货→上架）。
function seedStockAt(token, locationID, batchNo, qty, idx) {
  const create = postJSON('/api/v1/inbound/orders', token, {
    warehouse_id: WAREHOUSE_ID,
    remark: `k6 压测铺货 ${idx}`,
    details: [{ sku_id: SKU_ID, expected_qty: qty }],
  });
  must(create, `铺货-${idx}-创建入库单`);
  const inboundId = create.json('data.id');

  must(postJSON(`/api/v1/inbound/orders/${inboundId}/submit`, token, {}), `铺货-${idx}-提交`);
  must(postJSON(`/api/v1/inbound/orders/${inboundId}/approve`, token, {}), `铺货-${idx}-审核`);

  const detail = getJSON(`/api/v1/inbound/orders/${inboundId}`, token);
  must(detail, `铺货-${idx}-查明细`);
  const inboundDetails = detail.json('data.details') || [];
  const detailID = inboundDetails[0] ? inboundDetails[0].id : '';
  if (!detailID) {
    throw new Error(`铺货-${idx}: 入库明细为空 ${detail.body}`);
  }

  // 收货：指定批次，一次收齐 → 自动生成上架任务
  must(
    postJSON(`/api/v1/inbound/orders/${inboundId}/receive`, token, {
      detail_id: detailID,
      qty: qty,
      defective_qty: 0,
      batch_no: batchNo,
    }),
    `铺货-${idx}-收货`
  );

  const after = getJSON(`/api/v1/inbound/orders/${inboundId}`, token);
  must(after, `铺货-${idx}-查上架任务`);
  const putawayTask = (after.json('data.tasks') || []).find(
    (t) => t.task_type === 'PUTAWAY'
  );
  if (!putawayTask) {
    throw new Error(`铺货-${idx}: 未生成上架任务 ${after.body}`);
  }

  // 上架到指定库位 → 库存生效（仓库+库位+SKU+批次 唯一行）
  must(
    postJSON(`/api/v1/inbound/tasks/${putawayTask.id}/putaway`, token, {
      task_id: putawayTask.id,
      location_id: locationID,
      qty: qty,
    }),
    `铺货-${idx}-上架`
  );
}

// ---------- setup：铺货 + 建大单 + 审核出拣货任务 ----------
export function setup() {
  if (!WAREHOUSE_ID || !SKU_ID) {
    throw new Error('必须指定环境变量 WAREHOUSE_ID 和 SKU_ID，先运行 check-env.js 探测');
  }
  const token = login();
  console.log(`开始铺货：专属库区=${ZONE}，${ROWS} 库位 × ${QTY_PER_ROW} 件 = ${TOTAL} 件`);

  // 1. 批量建库位（接口幂等：已存在编码跳过）
  must(
    postJSON('/api/v1/basic/locations/batch', token, {
      warehouse_id: WAREHOUSE_ID,
      zone: ZONE,
      row_from: 1,
      row_to: ROWS,
      col_from: 1,
      col_to: 1,
    }),
    '批量建库位'
  );

  // 2. 拉库位列表，筛出本次库区，按编码排序
  const locRes = getJSON(
    `/api/v1/basic/locations?warehouse_id=${WAREHOUSE_ID}&page=1&page_size=100`,
    token
  );
  must(locRes, '查库位列表');
  const prefix = `${ZONE}-`;
  const locations = (locRes.json('data.list') || [])
    .filter((l) => l.code && l.code.indexOf(prefix) === 0)
    .sort((a, b) => (a.code < b.code ? -1 : 1));
  if (locations.length < ROWS) {
    throw new Error(`库位数量不足：期望 ${ROWS}，实际 ${locations.length}`);
  }

  // 3. 逐库位铺货（独立批次，保证每行库存独立 → FIFO 分配生成 ROWS 个拣货任务）
  const ts = Date.now().toString(36).toUpperCase();
  for (let i = 0; i < ROWS; i++) {
    seedStockAt(token, locations[i].id, `B${ts}${i}`, QTY_PER_ROW, i + 1);
    if ((i + 1) % 10 === 0) {
      console.log(`  铺货进度 ${i + 1}/${ROWS}`);
    }
  }
  console.log(`铺货完成：${ROWS} 条库存行`);

  // 4. 创建出库大单并审核（审核即分配 → 生成拣货任务）
  const create = postJSON('/api/v1/outbound/orders', token, {
    warehouse_id: WAREHOUSE_ID,
    biz_order_no: uniqueBizNo('K6WAVE'),
    remark: 'k6 拣货压测大单',
    details: [{ sku_id: SKU_ID, expected_qty: TOTAL }],
  });
  must(create, '创建出库大单');
  const orderId = create.json('data.id');

  must(postJSON(`/api/v1/outbound/orders/${orderId}/submit`, token, {}), '大单提交');
  const approve = postJSON(`/api/v1/outbound/orders/${orderId}/approve`, token, {});
  if (!checkBizOK(approve, '大单审核')) {
    throw new Error(`大单审核失败（库存不足?）: ${approve.body}`);
  }

  const orderDetail = getJSON(`/api/v1/outbound/orders/${orderId}`, token);
  must(orderDetail, '查大单详情');
  const tasks = (orderDetail.json('data.tasks') || []).map((t) => ({
    id: t.id,
    target: t.target_qty,
  }));
  console.log(
    `大单就绪 order_id=${orderId}，拣货任务=${tasks.length} 个，状态=${orderDetail.json('data.order.status')}`
  );

  return { token, orderId, tasks };
}

// 记录一次拣货结果。
function recordPick(res) {
  const ok = res.status === 200 && res.json('code') === 0;
  if (ok) {
    pickOk.add(1);
  } else {
    pickReject.add(1, { code: String(res.json('code') || `http_${res.status}`) });
  }
  return ok;
}

// 主场景：每个拣货员负责 stride 分配到的任务，逐件扫码到拣满。
export function wavePick(data) {
  const token = data.token;
  const tasks = data.tasks;
  // 确定性分工：VU i 负责所有 (idx % WORKERS == i-1) 的任务，
  // 无共享可变状态，每个任务恰好被一个拣货员认领（任务数与工人数不等时有人多拣/空闲）。
  const myTasks = tasks.filter((_, idx) => idx % WORKERS === __VU - 1);

  myTasks.forEach((task) => {
    let picked = 0;
    while (picked < task.target) {
      const res = postJSON(`/api/v1/outbound/tasks/${task.id}/pick`, token, {
        task_id: task.id,
        qty: 1, // 逐件扫码
      });
      if (recordPick(res)) {
        picked += 1;
      } else {
        // 业务失败：防超拣(TaskQtyOver)/状态错误 说明任务已完成；
        // 冲突类错误 TxRetry 内部已重试 3 次，仍失败则退出避免死循环。
        break;
      }
      sleep(SCAN_INTERVAL); // PDA 扫码节奏
    }
  });
}

// 竞争场景：额外 VU 反复随机抢拣，预期大部分被业务拒绝。
export function chaosPick(data) {
  const token = data.token;
  const tasks = data.tasks;
  const task = tasks[__ITER % tasks.length];
  const res = postJSON(`/api/v1/outbound/tasks/${task.id}/pick`, token, {
    task_id: task.id,
    qty: 1,
  });
  recordPick(res);
  sleep(0.02);
}

// ---------- teardown：数据正确性核对 ----------
export function teardown(data) {
  const token = data.token;
  const headers = authHeaders(token);

  console.log('\n========== 压测结果核对 ==========');

  // 1. 出库单终态与拣货总量（防超拣核心判据）
  const order = http.get(`${BASE_URL}/api/v1/outbound/orders/${data.orderId}`, { headers });
  const status = order.json('data.order.status');
  const picked = order.json('data.order.picked_qty');
  const expected = order.json('data.order.expected_qty');
  console.log(`出库单: status=${status} expected=${expected} picked=${picked} 期望 picked=${TOTAL}`);
  check(null, {
    '终态 SHIPPED': () => status === 'SHIPPED',
    '拣货总量恰好=需求量(无超拣)': () => picked === TOTAL,
  });

  // 2. 所有拣货任务 COMPLETED 且 done==target
  const tasks = order.json('data.tasks') || [];
  let doneSum = 0;
  let allDone = true;
  tasks.forEach((t) => {
    doneSum += t.done_qty;
    if (t.status !== 'COMPLETED' || t.done_qty !== t.target_qty) {
      allDone = false;
      console.log(`  异常任务 id=${t.id} status=${t.status} done=${t.done_qty}/${t.target_qty}`);
    }
  });
  console.log(`拣货任务: 共 ${tasks.length} 个，完成 ${allDone ? '全部 COMPLETED' : '存在异常'}，done 合计=${doneSum}`);
  check(null, {
    '全部任务 COMPLETED 且数量一致': () => allDone && doneSum === TOTAL,
  });

  // 3. 库存不能为负数
  const inv = http.get(
    `${BASE_URL}/api/v1/inventory?warehouse_id=${WAREHOUSE_ID}&sku_id=${SKU_ID}&page=1&page_size=100`,
    { headers }
  );
  const rows = inv.json('data.list') || [];
  const negative = rows.filter((r) => r.stock_quantity < 0 || r.available_quantity < 0);
  const stockSum = rows.reduce((s, r) => s + (r.stock_quantity || 0), 0);
  console.log(`库存: ${rows.length} 行，stock 合计=${stockSum}，负数行=${negative.length}`);
  check(null, {
    '无负库存': () => negative.length === 0,
  });

  console.log('==================================');
  console.log('还需对照 k6 汇总中的 pick_ok_total（应恰好=' + TOTAL + '）与 pick_reject_total');
}
