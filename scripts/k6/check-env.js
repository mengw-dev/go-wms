// 环境自检脚本：验证服务可达、能登录，并自动定位演示仓库、SKU 和可用库存。
// 输出结果可直接用于 outbound-e2e.js / demo-flow.js。
//
// 运行：
//   k6 run scripts/k6/check-env.js
//   k6 run -e BASE_URL=http://127.0.0.1:8080 scripts/k6/check-env.js
import http from 'k6/http';
import { check, group } from 'k6';
import { BASE_URL, USERNAME, login, authHeaders, checkBizOK } from './lib.js';

const WAREHOUSE_CODE = __ENV.WAREHOUSE_CODE || 'WH01';
const SKU_CODE = __ENV.SKU_CODE || 'SKU000001';

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

  let warehouse = null;
  group('1. 演示仓库', () => {
    const res = http.get(
      `${BASE_URL}/api/v1/basic/warehouses?keyword=${encodeURIComponent(WAREHOUSE_CODE)}&page=1&page_size=10`,
      { headers }
    );
    if (checkBizOK(res, '仓库列表')) {
      const list = res.json('data.list') || [];
      warehouse = list.find((w) => w.code === WAREHOUSE_CODE) || null;
      if (warehouse) {
        console.log(`  推荐仓库: id=${warehouse.id} code=${warehouse.code} name=${warehouse.name}`);
      } else {
        console.log(`  ⚠ 未找到仓库 ${WAREHOUSE_CODE}`);
      }
    }
  });

  let sku = null;
  group('2. 演示货品', () => {
    const res = http.get(
      `${BASE_URL}/api/v1/basic/skus?keyword=${encodeURIComponent(SKU_CODE)}&page=1&page_size=10`,
      { headers }
    );
    if (checkBizOK(res, 'SKU列表')) {
      const list = res.json('data.list') || [];
      sku = list.find((s) => s.code === SKU_CODE) || null;
      if (sku) {
        console.log(`  推荐货品: id=${sku.id} code=${sku.code} name=${sku.name}`);
      } else {
        console.log(`  ⚠ 未找到货品 ${SKU_CODE}`);
      }
    }
  });

  let summary = null;
  group('3. 库存汇总', () => {
    const query = warehouse?.id
      ? `warehouse_id=${warehouse.id}&page=1&page_size=100`
      : 'page=1&page_size=100';
    const res = http.get(`${BASE_URL}/api/v1/inventory/summary?${query}`, { headers });
    if (checkBizOK(res, '库存汇总')) {
      const list = res.json('data.list') || [];
      summary = sku ? list.find((i) => i.sku_code === sku.code) : null;
      if (summary) {
        console.log(
          `  ${summary.sku_code}: stock=${summary.stock_quantity} available=${summary.available_quantity} allocated=${summary.allocated_quantity}`
        );
      } else {
        console.log('  ⚠ 目标货品暂无库存，审核出库会失败；可以先跑 demo-flow.js 或入库流程备货。');
      }
    }
  });

  console.log('\n========== 可直接使用的参数 ==========');
  console.log(`WAREHOUSE_ID=${warehouse?.id || ''}`);
  console.log(`SKU_ID=${sku?.id || ''}`);
  console.log(`可用库存=${summary?.available_quantity ?? 0}`);
  console.log('完整业务流程: k6 run -e WAREHOUSE_ID=... -e SKU_ID=... scripts/k6/demo-flow.js');
  console.log('出库冒烟测试: k6 run -e WAREHOUSE_ID=... -e SKU_ID=... -e ORDER_QTY=1 scripts/k6/outbound-e2e.js');
}