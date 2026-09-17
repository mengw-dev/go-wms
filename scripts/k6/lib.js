// k6 公共辅助：登录、鉴权头、统一业务响应处理。
// 后端统一响应结构：{ code: 0, msg: "success", data: ... }
// 业务失败时 HTTP 状态码通常为 4xx/5xx，业务码仍可通过 body.code 判断。
import http from 'k6/http';
import { check } from 'k6';

// 默认连本机 8080；可用环境变量 BASE_URL 覆盖，如 k6 run -e BASE_URL=http://192.168.1.10:8080
export const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:8080';
// 不能用 USERNAME/PASSWORD 作为环境变量名：Windows 系统自带 USERNAME 变量。
export const USERNAME = __ENV.K6_USERNAME || 'admin';
export const PASSWORD = __ENV.K6_PASSWORD || 'admin123';

// 对外 ID 使用字符串，和后端 JSON `id,string` 契约一致，避免 JS 大整数精度问题。
export const WAREHOUSE_ID = (__ENV.WAREHOUSE_ID || '').trim();
export const SKU_ID = (__ENV.SKU_ID || '').trim();
export const ORDER_QTY = parseInt(__ENV.ORDER_QTY || '1', 10);

// 业务错误可能是 400/409/423；这些由 checkBizOK 或业务指标判断，
// 不应被 k6 统计为 HTTP 失败。只有 5xx/网络错误才算基础设施失败。
const expectedStatus = typeof http.expectedStatuses === 'function'
  ? http.expectedStatuses(200, 201, 204, 400, 409, 423)
  : null;

// 登录并返回 JWT token。登录失败直接抛错，让环境问题尽早暴露。
export function login() {
  const res = http.post(
    `${BASE_URL}/api/v1/login`,
    JSON.stringify({ username: USERNAME, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' }, tags: { api: 'login' } }
  );
  const ok = check(res, {
    'login http 2xx': (r) => r.status >= 200 && r.status < 300,
    'login code=0': (r) => r.json('code') === 0,
    'login token exists': (r) => !!r.json('data.token'),
  });
  if (!ok) {
    throw new Error(`登录失败：status=${res.status} body=${res.body}`);
  }
  return res.json('data.token');
}

// 组装请求头。demoSessionId 非空时用于 WMS 演示模式。
export function authHeaders(token, demoSessionId = '') {
  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  };
  if (demoSessionId) {
    headers['X-Demo-Session'] = demoSessionId;
  }
  return headers;
}

// 判断业务响应是否成功：HTTP 2xx 且 body.code=0。
export function checkBizOK(res, name) {
  return check(res, {
    [`${name} http 2xx`]: (r) => r.status >= 200 && r.status < 300,
    [`${name} code=0`]: (r) => r.json('code') === 0,
  });
}

// 生成唯一业务单号（幂等键 biz_order_no）。
export function uniqueBizNo(prefix) {
  const vu = typeof __VU !== 'undefined' ? __VU : 0;
  const iter = typeof __ITER !== 'undefined' ? __ITER : 0;
  const random = Math.floor(Math.random() * 1000000);
  return `${prefix}${Date.now()}${vu}${iter}${random}`;
}

// 快捷 POST JSON。
function requestOptions(headers, apiPath) {
  const options = {
    headers,
    tags: { api: apiPath },
  };
  if (expectedStatus) {
    options.responseCallback = expectedStatus;
  }
  return options;
}

// 快捷 POST JSON。
export function postJSON(path, token, body, demoSessionId = '') {
  return http.post(`${BASE_URL}${path}`, JSON.stringify(body), requestOptions(authHeaders(token, demoSessionId), path));
}

// 快捷 GET。
export function getJSON(path, token, demoSessionId = '') {
  return http.get(`${BASE_URL}${path}`, requestOptions(authHeaders(token, demoSessionId), path));
}
