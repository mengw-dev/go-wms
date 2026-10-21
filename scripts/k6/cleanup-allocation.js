// 仅清理专用测试租户内、目标仓库/单一 SKU 的 K6PROOF 订单。
// 默认只读预览；CLEANUP_EXECUTE=1 才经业务取消接口释放分配，不删除单据或改库存。
// 运行前停止该测试范围的建单、编辑、拣货和压测，避免分页及取消间的并发变化。
import { check } from 'k6';
import execution from 'k6/execution';
import encoding from 'k6/encoding';
import { WAREHOUSE_ID, SKU_ID, USERNAME, login, postJSON, getJSON } from './lib.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: { checks: ['rate==1'] },
};

const PAGE_SIZE = 100;
const MAX_ROWS = 100000; // 超预算明确失败，不能把截断结果当作完整清理。

function demand(condition, message) {
  if (!condition) throw new Error(message);
}

function id(value, name) {
  demand(typeof value === 'string' && /^[1-9][0-9]{0,18}$/.test(value)
    && (value.length < 19 || value <= '9223372036854775807'), `${name}必须是正 int64 字符串`);
  return value;
}

function quantity(value, name) {
  demand(Number.isSafeInteger(value) && value >= 0, `${name}数量无效`);
  return value;
}

function plus(left, right) {
  return quantity(left + right, '汇总');
}

function data(res, name) {
  let body;
  try { body = res.json(); } catch (_) { throw new Error(`${name}返回非 JSON`); }
  // 不输出登录凭证或整个服务端响应体。
  demand(res.status >= 200 && res.status < 300 && body && body.code === 0 && body.data != null,
    `${name}失败：HTTP ${res.status}`);
  return body.data;
}

function configuration() {
  id(WAREHOUSE_ID, 'WAREHOUSE_ID');
  id(SKU_ID, 'SKU_ID');
  demand(typeof __ENV.K6_USERNAME === 'string' && __ENV.K6_USERNAME.trim() !== '', '必须显式设置 K6_USERNAME');
  const mode = __ENV.CLEANUP_EXECUTE || '0';
  demand(mode === '0' || mode === '1', 'CLEANUP_EXECUTE 只接受 0 或 1');
  const prefix = __ENV.CLEANUP_PREFIX || 'K6PROOF';
  demand(/^K6PROOF[A-Za-z0-9_-]{0,57}$/.test(prefix), 'CLEANUP_PREFIX 必须是 K6PROOF 开头的测试前缀');
  return { warehouse: WAREHOUSE_ID, sku: SKU_ID, prefix, execute: mode === '1' };
}

function tenantOf(token) {
  // 仅是客户端防误用，不代替 API 服务端的 JWT 签名/权限/租户校验。
  const raw = encoding.b64decode(token.split('.')[1], 'rawurl', 's');
  const claims = JSON.parse(raw);
  const tid = raw.match(/"tid"\s*:\s*(0|[1-9][0-9]*)\s*[,}]/);
  demand(tid && claims.username === USERNAME, '登录身份或租户声明无效');
  // tid 在当前 JWT 中是 JSON 数字；使用原始十进制文本，避免雪花 ID 精度损失。
  return id(tid[1], '登录租户（平台租户 0 禁止清理）');
}

function pages(token, path, label) {
  const rows = [];
  const seen = new Set();
  let total;
  for (let page = 1; ; page++) {
    const result = data(getJSON(`${path}&page=${page}&page_size=${PAGE_SIZE}`, token), label);
    demand(Array.isArray(result.list), `${label}缺少列表`);
    quantity(result.total, `${label}总数`);
    demand(result.total <= MAX_ROWS, `${label}超过 ${MAX_ROWS} 行预算，停止并缩小范围`);
    if (total === undefined) total = result.total;
    demand(total === result.total, `${label}总数变化，请停止并发写入后重新核对`);
    demand(result.list.length === Math.min(PAGE_SIZE, total - rows.length), `${label}分页不完整`);
    for (const row of result.list) {
      const key = id(row.id, `${label}ID`);
      demand(!seen.has(key), `${label}分页重复，停止清理`);
      seen.add(key);
      rows.push(row);
    }
    if (rows.length === total) return rows;
  }
}

function inventory(token, scope) {
  const rows = pages(token, `/api/v1/inventory?warehouse_id=${scope.warehouse}&sku_id=${scope.sku}`, '查询库存');
  const result = { stock: 0, available: 0, allocated: 0 };
  for (const row of rows) {
    demand(row.warehouse_id === scope.warehouse && row.sku_id === scope.sku && row.tenant_id === scope.tenant,
      '库存行超出目标租户/仓库/货品范围');
    const stock = quantity(row.stock_quantity, '现存');
    const available = quantity(row.available_quantity, '可用');
    const allocated = quantity(row.allocated_quantity, '分配');
    demand(stock === plus(available, allocated), '库存行不满足现存=可用+分配');
    result.stock = plus(result.stock, stock);
    result.available = plus(result.available, available);
    result.allocated = plus(result.allocated, allocated);
  }
  return result;
}

function inScope(order, scope) {
  return order.warehouse_id === scope.warehouse && order.tenant_id === scope.tenant
    && typeof order.biz_order_no === 'string' && order.biz_order_no.startsWith(scope.prefix);
}

function inspect(payload, orderID, scope) {
  const order = payload.order;
  demand(order && order.id === orderID && inScope(order, scope), '订单详情身份或范围变化');
  demand(Array.isArray(payload.details) && payload.details.length > 0, '订单明细缺失');
  // 混合 SKU 整单取消会影响其他货品，因此整单排除，绝不按某一行匹配就取消。
  if (payload.details.some(row => row.sku_id !== scope.sku)) return null;
  if (order.status === 'CANCELLED') return null;
  demand(['DRAFT', 'SUBMITTED', 'APPROVED', 'PICKING'].includes(order.status), '目标测试单已发货或状态不允许取消');
  demand(quantity(order.picked_qty, '订单已拣') === 0, '目标测试单已有拣货，请人工核对');
  demand(Array.isArray(payload.tasks) && Array.isArray(payload.allocations), '订单任务或分配明细缺失');
  for (const row of payload.details) {
    demand(row.order_id === orderID && row.tenant_id === scope.tenant, '明细归属不一致');
    demand(quantity(row.picked_qty, '明细已拣') === 0, '明细已有拣货');
  }
  for (const row of payload.tasks) {
    demand(row.sku_id === scope.sku && row.warehouse_id === scope.warehouse
      && row.order_id === orderID && row.tenant_id === scope.tenant, '任务超出目标范围');
    demand(quantity(row.done_qty, '任务进度') === 0, '任务已有作业进度');
  }
  let release = 0;
  for (const row of payload.allocations) {
    demand(row.sku_id === scope.sku && row.order_id === orderID && row.tenant_id === scope.tenant, '分配超出目标范围');
    demand(quantity(row.picked_qty, '分配已拣') === 0, '分配已有拣货');
    if (row.status === 'ALLOCATED') release = plus(release, quantity(row.allocated_qty, '分配'));
  }
  demand(quantity(order.allocated_qty, '订单分配') === release, '主单与分配明细数量不一致');
  // 取消前再读，任何业务内容变化都先停止；服务端取消仍负责锁内的未拣货校验。
  // k6 的 JSON 对象属性枚举顺序不能作为内容版本；只按固定字段顺序比较业务语义。
  const signature = JSON.stringify([
    order.id, order.tenant_id, order.warehouse_id, order.biz_order_no, order.status,
    order.expected_qty, order.allocated_qty, order.picked_qty,
    payload.details.map(row => [row.id, row.order_id, row.tenant_id, row.sku_id, row.expected_qty, row.allocated_qty, row.picked_qty]),
    payload.allocations.map(row => [row.id, row.inventory_id, row.sku_id, row.status, row.allocated_qty, row.picked_qty]),
    payload.tasks.map(row => [row.id, row.sku_id, row.warehouse_id, row.status, row.target_qty, row.done_qty]),
  ]);
  return { id: orderID, release, signature };
}

function detail(token, orderID) {
  return data(getJSON(`/api/v1/outbound/orders/${orderID}`, token), '查询订单详情');
}

function cleanup() {
  const scope = configuration(); // 必填项检查在登录和一切业务调用之前。
  let token;
  try { token = login(); } catch (_) { throw new Error('登录失败，请检查测试账号和接口'); }
  scope.tenant = tenantOf(token);
  const before = inventory(token, scope);
  const orders = pages(token, `/api/v1/outbound/orders?warehouse_id=${scope.warehouse}&keyword=${encodeURIComponent(scope.prefix)}`, '查询测试订单');
  const plan = [];
  let release = 0;
  for (const order of orders) {
    if (!inScope(order, scope)) continue; // 服务端 keyword 是模糊搜索，客户端仍需精确验证前缀。
    const candidate = inspect(detail(token, order.id), order.id, scope);
    if (candidate) { plan.push(candidate); release = plus(release, candidate.release); }
  }
  demand(before.allocated === release, '分配量包含其他订单或不完整数据，停止清理，不扩大取消范围');
  console.log(`${scope.execute ? '执行' : '只读预览'}：租户=${scope.tenant} 仓库=${scope.warehouse} SKU=${scope.sku} 前缀=${scope.prefix}，可取消 ${plan.length} 单、释放 ${release} 件`);
  console.log(`候选 ID（最多20个）：${plan.slice(0, 20).map(item => item.id).join(',')}`);
  if (!scope.execute) return;

  for (let start = 0; start < plan.length; start += 200) {
    const chunk = plan.slice(start, start + 200);
    for (const item of chunk) {
      const current = inspect(detail(token, item.id), item.id, scope);
      demand(current && current.signature === item.signature, '取消前订单已变化，停止后续批次');
    }
    const result = data(postJSON('/api/v1/outbound/orders/batch-cancel', token, { ids: chunk.map(item => item.id) }), '批量取消');
    demand(quantity(result.success, '取消成功') === chunk.length && quantity(result.fail, '取消失败') === 0
      && (result.errors === undefined || (Array.isArray(result.errors) && result.errors.length === 0)),
    '批量取消部分失败或计数异常；已成功的操作不回滚，核对后再预览');
  }
  for (const item of plan) {
    const current = detail(token, item.id).order;
    demand(current && current.id === item.id && inScope(current, scope) && current.status === 'CANCELLED', '取消后的订单状态未确认');
  }
  const after = inventory(token, scope);
  const ok = check(after, {
    '分配量已清零': value => value.allocated === 0,
    '现存量不变': value => value.stock === before.stock,
    '可用量按取消量回补': value => value.available === plus(before.available, release),
  });
  demand(ok, '清理后库存核对失败，请人工核对，不自动重发取消');
  console.log(`清理完成：取消 ${plan.length} 单，库存=${after.stock}，可用=${after.available}，分配=${after.allocated}`);
}

export default function () {
  try {
    cleanup();
    check(true, { '清理流程完成': value => value === true });
  } catch (error) {
    check(false, { '清理流程完成': value => value === true });
    execution.test.abort(`清理失败：${error.message}`); // VU 抛错本身不保证 k6 非零退出。
  }
}
