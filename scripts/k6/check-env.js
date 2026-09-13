// 环境自检脚本：验证服务可达、能登录，并列出仓库/SKU/可用库存，
// 帮助确定 outbound-e2e.js / outbound-stress.js 需要的 WAREHOUSE_ID、SKU_ID。
//
// 运行：
//   k6 run scripts/k6/check-env.js
//   k6 run -e BASE_URL=http://127.0.0.1:8080 scripts/k6/check-env.js
import http from 'k6/http';
import { check, group } from 'k6';
import { BASE_URL, USERNAME, PASSWORD, login, authHeaders, checkBizOK } from './lib.js';

export const options = {
  vus: 1,
  iterations: 1,
};

export default function () {
  group('0. 健康检查', () => {
    const res = http.get(`${BASE_URL}/healthz`);
    check(res, { 'healthz 200': (r) => r.status === 200 });
    console.log(`健康检查: ${res.status} ${res.body}`);
  });

  const token = login();
  console.log(`登录成功（${USERNAME}），token 前 20 位: ${token.substring(0, 20)}...`);
  const headers = authHeaders(token);

  group('1. 仓库列表', () => {
    const res = http.get(`${BASE_URL}/api/v1/basic/warehouses?page=1&page_size=5`, { headers });
    if (checkBizOK(res, '仓库列表')) {
      const list = res.json('data.list') || [];
      console.log(`仓库总数: ${res.json('data.total')}`);
      list.forEach((w) => {
        console.log(`  仓库 id=${w.id} code=${w.code} name=${w.name} status=${w.status}`);
      });
    }
  });

  group('2. SKU 列表', () => {
    const res = http.get(`${BASE_URL}/api/v1/basic/skus?page=1&page_size=10`, { headers });
    if (checkBizOK(res, 'SKU列表')) {
      const list = res.json('data.list') || [];
      console.log(`SKU 总数: ${res.json('data.total')}`);
      list.forEach((s) => {
        console.log(`  SKU id=${s.id} code=${s.code} name=${s.name}`);
      });
    }
  });

  group('3. 库存（可用量）', () => {
    // summary 按 SKU 汇总库存，available 即可用数量（stock = available + allocated）
    const res = http.get(`${BASE_URL}/api/v1/inventory/summary?page=1&page_size=10`, { headers });
    if (checkBizOK(res, '库存汇总')) {
      const list = res.json('data.list') || [];
      console.log(`有库存记录的 SKU 数: ${res.json('data.total')}`);
      list.forEach((i) => {
        console.log(
          `  库存 warehouse_id=${i.warehouse_id} sku_id=${i.sku_id} ` +
            `available=${i.available_qty || i.available} allocated=${i.allocated_qty || i.allocated}`
        );
      });
      if (list.length === 0) {
        console.log('  ⚠ 没有任何库存！审核出库单会因可用量不足失败，请先通过入库流程备货。');
      }
    }
  });

  console.log('\n自检完成。下一步示例：');
  console.log('  k6 run -e WAREHOUSE_ID=1 -e SKU_ID=1001 scripts/k6/outbound-e2e.js');
}
