// k6 公共辅助：登录、鉴权头、统一业务响应处理。
// 后端统一响应结构：{ code: 0, msg: "success", data: ... }
// 注意：业务失败时 HTTP 状态码仍是 200，靠 body.code 区分（0 成功，非 0 业务错误）。
import http from 'k6/http';
import { check } from 'k6';

// 默认连本机 8080；可用环境变量 BASE_URL 覆盖，如 k6 run -e BASE_URL=http://192.168.1.10:8080
export const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:8080';
// 注意：不能用 USERNAME/PASSWORD 作为环境变量名——Windows 系统自带 USERNAME 变量
// （值为当前 Windows 登录名），k6 会注入它，导致 __ENV.USERNAME 永远不是 'admin'。
export const USERNAME = __ENV.K6_USERNAME || 'admin';
export const PASSWORD = __ENV.K6_PASSWORD || 'admin123';

// 测试目标仓库与 SKU（出库单必须依赖真实存在且启用的仓库/SKU）。
// 运行前用 check-env.js 探测，或通过环境变量传入：
//   k6 run -e WAREHOUSE_ID=1 -e SKU_ID=1001 outbound-e2e.js
export const WAREHOUSE_ID = parseInt(__ENV.WAREHOUSE_ID || '0', 10);
export const SKU_ID = parseInt(__ENV.SKU_ID || '0', 10);
// 每张出库单的需求数量（默认 1，压测时可调大）。
export const ORDER_QTY = parseInt(__ENV.ORDER_QTY || '1', 10);

// 登录并返回 JWT token。登录失败直接抛错（测试前提不满足，没有继续的意义）。
export function login() {
  const res = http.post(
    `${BASE_URL}/api/v1/login`,
    JSON.stringify({ username: USERNAME, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' }, tags: { api: 'login' } }
  );
  const ok = check(res, {
    'login http 200': (r) => r.status === 200,
    'login code=0': (r) => r.json('code') === 0,
    'login token exists': (r) => !!r.json('data.token'),
  });
  if (!ok) {
    throw new Error(`登录失败：status=${res.status} body=${res.body}`);
  }
  return res.json('data.token');
}

// 组装带 JWT 的请求头。
export function authHeaders(token) {
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  };
}

// 判断业务响应是否成功（HTTP 200 且 code=0）。
// check 的 name 用于在结果输出中区分是哪个接口失败。
export function checkBizOK(res, name) {
  return check(res, {
    [`${name} http 200`]: (r) => r.status === 200,
    [`${name} code=0`]: (r) => r.json('code') === 0,
  });
}

// 生成唯一业务单号（幂等键 biz_order_no）。
// k6 中 __VU 为虚拟用户编号、__ITER 为该 VU 的迭代序号，组合 Date.now 保证全局唯一。
export function uniqueBizNo(prefix) {
  return `${prefix}${Date.now()}${__VU}${__ITER}`;
}

// 快捷 POST JSON。
export function postJSON(path, token, body) {
  return http.post(`${BASE_URL}${path}`, JSON.stringify(body), {
    headers: authHeaders(token),
    tags: { api: path },
  });
}

// 快捷 GET。
export function getJSON(path, token) {
  return http.get(`${BASE_URL}${path}`, {
    headers: { Authorization: `Bearer ${token}` },
    tags: { api: path },
  });
}
