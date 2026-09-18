// 读接口（查询/列表）压测脚本——覆盖前端高频读取路径，补齐写路径之外的盲区。
//
// 覆盖接口：
//   登录（bcrypt 校验的 CPU 成本）、出库/入库列表（含深分页第 100 页）、
//   出库订单详情（聚合主单+明细+任务，最重的读接口）、库存汇总、库存流水、任务列表、货品列表。
//
// 技巧：每个接口单独打 api tag，thresholds 用宽松阈值(2000ms)，
// k6 会在 THRESHOLDS 区打印每个接口各自的 p95，便于横向对比找慢接口。
//
// 运行：
//   k6 run -e BASE_URL=http://127.0.0.1:8080 scripts/k6/read-stress.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { BASE_URL, USERNAME, PASSWORD, authHeaders, checkBizOK, login } from './lib.js';

export const options = {
  scenarios: {
    reads: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '5s', target: 20 },  // 爬坡到 20 VU
        { duration: '1m', target: 20 },  // 峰值保持 1 分钟
        { duration: '5s', target: 0 },   // 收尾
      ],
      exec: 'readFlow',
      tags: { scenario: 'reads' },
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    // 宽松阈值仅为让 k6 打印各接口 p95 数值，不做严格卡线
    'http_req_duration{api:login}': ['p(95)<2000'],
    'http_req_duration{api:outbound_list}': ['p(95)<2000'],
    'http_req_duration{api:outbound_list_deep}': ['p(95)<2000'],
    'http_req_duration{api:outbound_detail}': ['p(95)<2000'],
    'http_req_duration{api:inbound_list}': ['p(95)<2000'],
    'http_req_duration{api:inventory_summary}': ['p(95)<2000'],
    'http_req_duration{api:inventory_trans}': ['p(95)<2000'],
    'http_req_duration{api:task_list}': ['p(95)<2000'],
    'http_req_duration{api:sku_list}': ['p(95)<2000'],
  },
};

// setup：登录一次，并取一张真实出库单 id 用于详情压测（通常是最近创建的单，明细/任务最多）。
export function setup() {
  const token = login();
  const res = http.get(`${BASE_URL}/api/v1/outbound/orders?page=1&page_size=1`, {
    headers: authHeaders(token),
    tags: { api: 'outbound_list' },
  });
  const list = res.json('data.list') || [];
  const orderId = list[0] && list[0].id ? String(list[0].id) : '';
  console.log(`详情压测用出库单 id=${orderId || '（无数据，将跳过详情）'}`);
  return { token, orderId };
}

// 每个 VU 的一轮读取组合：模拟运营人员连续打开各页面的典型读路径。
export function readFlow(data) {
  const headers = authHeaders(data.token);

  // 登录：每轮重新登录，测量 bcrypt 密码校验的真实成本。
  const loginRes = http.post(
    `${BASE_URL}/api/v1/login`,
    JSON.stringify({ username: USERNAME, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' }, tags: { api: 'login' } }
  );
  check(loginRes, { 'login ok': (r) => r.status === 200 && r.json('code') === 0 });

  // 出库列表第一页 + 深分页（第 100 页，压 LIMIT/OFFSET 性能）
  get('/api/v1/outbound/orders?page=1&page_size=20', headers, 'outbound_list');
  get('/api/v1/outbound/orders?page=100&page_size=20', headers, 'outbound_list_deep');

  // 出库详情：主单+明细+拣货任务聚合，最重的读接口
  if (data.orderId) {
    get(`/api/v1/outbound/orders/${data.orderId}`, headers, 'outbound_detail');
  }

  get('/api/v1/inbound/orders?page=1&page_size=20', headers, 'inbound_list');
  get('/api/v1/inventory/summary?page=1&page_size=50', headers, 'inventory_summary');
  get('/api/v1/inventory/trans?page=1&page_size=20', headers, 'inventory_trans');
  get('/api/v1/tasks?status=CREATED&page=1&page_size=20', headers, 'task_list');
  get('/api/v1/basic/skus?page=1&page_size=20', headers, 'sku_list');

  sleep(0.3); // 模拟页面停留
}

function get(path, headers, tag) {
  const res = http.get(`${BASE_URL}${path}`, { headers, tags: { api: tag } });
  checkBizOK(res, tag);
}
