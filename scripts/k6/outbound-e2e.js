// 出库模块端到端冒烟测试：1 VU 跑通一张出库单的完整生命周期。
//
// 状态机路径（审核即分配，不停留 APPROVED）：
//   DRAFT --submit--> SUBMITTED --approve(分配库存+生成拣货任务)--> PICKING --pick(拣满)--> SHIPPED
//
// 前置条件：
//   1. 服务已启动（默认 127.0.0.1:8080）
//   2. WAREHOUSE_ID、SKU_ID 真实存在且启用（先用 check-env.js 探测）
//   3. 该仓库该 SKU 有 >= ORDER_QTY 的可用库存（available），否则 approve 会因可用量不足回滚
//
// 运行：
//   k6 run -e WAREHOUSE_ID=1 -e SKU_ID=1001 -e ORDER_QTY=2 scripts/k6/outbound-e2e.js
import { check, group, sleep } from 'k6';
import {
  BASE_URL,
  WAREHOUSE_ID,
  SKU_ID,
  ORDER_QTY,
  login,
  postJSON,
  getJSON,
  checkBizOK,
  uniqueBizNo,
} from './lib.js';

export const options = {
  vus: 1,
  iterations: 1,
};

// setup 在所有 VU 启动前执行一次，返回值会传入 default 的第一个参数。
export function setup() {
  if (!WAREHOUSE_ID || !SKU_ID) {
    throw new Error('必须指定环境变量 WAREHOUSE_ID 和 SKU_ID，先运行 check-env.js 探测');
  }
  return { token: login() };
}

export default function (data) {
  const token = data.token;
  let orderId = 0;

  group('1. 创建出库单 DRAFT', () => {
    const body = {
      warehouse_id: WAREHOUSE_ID,
      biz_order_no: uniqueBizNo('K6BIZ'), // 幂等键：重复提交同一 biz_order_no 会被拒绝
      remark: 'k6 e2e 冒烟测试',
      details: [{ sku_id: SKU_ID, expected_qty: ORDER_QTY }],
    };
    const res = postJSON('/api/v1/outbound/orders', token, body);
    if (checkBizOK(res, '创建出库单')) {
      orderId = res.json('data.id');
      console.log(`  创建成功 order_id=${orderId} order_no=${res.json('data.order_no')}`);
    }
  });

  group('2. 提交 SUBMITTED', () => {
    const res = postJSON(`/api/v1/outbound/orders/${orderId}/submit`, token, {});
    checkBizOK(res, '提交');
  });

  group('3. 审核+分配 PICKING', () => {
    // 审核事务内完成 FIFO 库存分配、生成分配行、生成拣货任务，任一步失败整体回滚。
    const res = postJSON(`/api/v1/outbound/orders/${orderId}/approve`, token, {});
    if (checkBizOK(res, '审核')) {
      console.log('  审核成功，库存已分配，拣货任务已生成');
    } else {
      console.log(`  审核失败（常见原因：可用库存不足）: ${res.body}`);
    }
  });

  let tasks = [];
  group('4. 查询详情，取拣货任务', () => {
    const res = getJSON(`/api/v1/outbound/orders/${orderId}`, token);
    if (checkBizOK(res, '查询详情')) {
      tasks = res.json('data.tasks') || [];
      const status = res.json('data.order.status');
      const allocated = res.json('data.order.allocated_qty');
      console.log(`  单据状态=${status} allocated_qty=${allocated} 任务数=${tasks.length}`);
      check(null, {
        '状态推进到 PICKING': () => status === 'PICKING',
        '已生成拣货任务': () => tasks.length > 0,
      });
    }
  });

  group('5. 逐任务拣货（一次拣满）', () => {
    // FIFO 分配可能跨批次/库位，一张单可能对应多个拣货任务；全部拣满后单据自动 SHIPPED。
    tasks.forEach((t) => {
      const res = postJSON(`/api/v1/outbound/tasks/${t.id}/pick`, token, {
        task_id: t.id,
        qty: t.target_qty, // 一次性拣满该任务的目标数量
      });
      if (checkBizOK(res, `拣货 task=${t.id}`)) {
        console.log(`  拣货完成 task_id=${t.id} qty=${t.target_qty}`);
      } else {
        console.log(`  拣货失败 task_id=${t.id}: ${res.body}`);
      }
      sleep(0.1);
    });
  });

  group('6. 校验终态 SHIPPED', () => {
    const res = getJSON(`/api/v1/outbound/orders/${orderId}`, token);
    if (checkBizOK(res, '终态查询')) {
      const status = res.json('data.order.status');
      const picked = res.json('data.order.picked_qty');
      console.log(`  终态 status=${status} picked_qty=${picked}`);
      check(null, {
        '单据终态 SHIPPED': () => status === 'SHIPPED',
        '拣货量=需求量': () => picked === ORDER_QTY,
      });
    }
  });
}
