import { check } from 'k6';
import { Counter } from 'k6/metrics';

import {
    WAREHOUSE_ID,
    SKU_ID,
    login,
    postJSON,
    getJSON,
    checkBizOK,
    uniqueBizNo,
} from './lib.js';


// ============================================================
// 1. 测试配置
// ============================================================

const VUS = parseInt(__ENV.VUS || '60', 10);

const ORDER_QTY =
    parseInt(__ENV.ORDER_QTY || '100', 10);

const TARGET_STOCK =
    parseInt(__ENV.TARGET_STOCK || '4000', 10);


// ============================================================
// 2. 自定义业务指标
// ============================================================

const approveSuccess =
    new Counter('approve_success_total');

const approveReject =
    new Counter('approve_reject_total');

const approveSystemFail =
    new Counter('approve_system_fail_total');


// ============================================================
// 3. k6 压力模型
// ============================================================

export const options = {
    scenarios: {
        allocation: {
            executor: 'per-vu-iterations',

            vus: VUS,

            // 每个 VU 只执行一次
            iterations: 1,

            maxDuration: '30s',

            exec: 'approveOne',
        },
    },

    thresholds: {
        // 网络错误 / 5xx 应该接近 0
        http_req_failed: ['rate<0.01'],

        // 这里是性能指标，不是防超卖正确性指标
        http_req_duration: ['p(95)<1000'],
    },
};


// ============================================================
// 4. 通用辅助函数
// ============================================================

function must(res, name) {
    if (!checkBizOK(res, name)) {
        throw new Error(
            `${name}失败: status=${res.status}, body=${res.body}`
        );
    }
}


// ------------------------------------------------------------
// 查询目标 SKU 的所有库存行，并汇总
// ------------------------------------------------------------

function getInventory(token) {
    const res = getJSON(
        `/api/v1/inventory` +
        `?warehouse_id=${WAREHOUSE_ID}` +
        `&sku_id=${SKU_ID}` +
        `&page=1&page_size=100`,
        token
    );

    must(res, '查询库存');

    const rows = res.json('data.list') || [];

    const stock =
        rows.reduce(
            (sum, row) =>
                sum + Number(row.stock_quantity || 0),
            0
        );

    const available =
        rows.reduce(
            (sum, row) =>
                sum + Number(row.available_quantity || 0),
            0
        );

    const allocated =
        rows.reduce(
            (sum, row) =>
                sum + Number(row.allocated_quantity || 0),
            0
        );

    return {
        rows,
        stock,
        available,
        allocated,
    };
}


// ------------------------------------------------------------
// 找一个可以用于上架的库位
// ------------------------------------------------------------

function getLocation(token) {
    const res = getJSON(
        `/api/v1/basic/locations` +
        `?warehouse_id=${WAREHOUSE_ID}` +
        `&page=1&page_size=100`,
        token
    );

    must(res, '查询库位');

    const list = res.json('data.list') || [];

    const location =
        list.find((item) => item.status === 1) ||
        list.find((item) => item.status !== 0);

    if (!location) {
        throw new Error('没有找到可用库位');
    }

    return location;
}


// ============================================================
// 5. 准备 4000 件库存
// ============================================================

function prepareInventory(token) {
    let inventory = getInventory(token);

    console.log(
        `准备前库存: ` +
        `stock=${inventory.stock}, ` +
        `available=${inventory.available}, ` +
        `allocated=${inventory.allocated}`
    );

    // 为了让实验干净，开始时不允许已经存在分配量
    if (inventory.allocated !== 0) {
        throw new Error(
            `目标 SKU 当前 allocated=${inventory.allocated}，` +
            `请先清理测试数据或更换测试 SKU`
        );
    }

    if (inventory.available > TARGET_STOCK) {
        throw new Error(
            `当前 available=${inventory.available} > ` +
            `目标 ${TARGET_STOCK}，请使用干净测试环境`
        );
    }

    // 已经正好 4000，就不需要补货
    if (inventory.available === TARGET_STOCK) {
        return inventory;
    }

    const need =
        TARGET_STOCK - inventory.available;

    console.log(
        `需要补货 ${need} 件，使 available 达到 ${TARGET_STOCK}`
    );

    const location = getLocation(token);


    // ==========================================================
    // 创建入库单
    // ==========================================================

    const createRes = postJSON(
        '/api/v1/inbound/orders',
        token,
        {
            warehouse_id: WAREHOUSE_ID,

            remark: 'k6 并发分配测试准备库存',

            details: [
                {
                    sku_id: SKU_ID,
                    expected_qty: need,
                },
            ],
        }
    );

    must(createRes, '创建入库单');

    const inboundOrderId =
        createRes.json('data.id');


    // ==========================================================
    // 提交 + 审核入库单
    // ==========================================================

    must(
        postJSON(
            `/api/v1/inbound/orders/${inboundOrderId}/submit`,
            token,
            {}
        ),
        '提交入库单'
    );

    must(
        postJSON(
            `/api/v1/inbound/orders/${inboundOrderId}/approve`,
            token,
            {}
        ),
        '审核入库单'
    );


    // ==========================================================
    // 查询入库明细
    // ==========================================================

    let detailRes = getJSON(
        `/api/v1/inbound/orders/${inboundOrderId}`,
        token
    );

    must(detailRes, '查询入库单');

    const detail =
        (detailRes.json('data.details') || [])[0];

    if (!detail) {
        throw new Error('没有找到入库明细');
    }


    // ==========================================================
    // 收货
    // ==========================================================

    must(
        postJSON(
            `/api/v1/inbound/orders/${inboundOrderId}/receive`,
            token,
            {
                detail_id: detail.id,

                qty: need,

                defective_qty: 0,

                batch_no: `K6-${Date.now()}`,
            }
        ),
        '收货'
    );


    // ==========================================================
    // 收货后查询上架任务
    // ==========================================================

    detailRes = getJSON(
        `/api/v1/inbound/orders/${inboundOrderId}`,
        token
    );

    must(detailRes, '查询上架任务');

    const putawayTask =
        (detailRes.json('data.tasks') || []).find(
            (task) =>
                task.task_type === 'PUTAWAY' &&
                task.done_qty < task.target_qty
        );

    if (!putawayTask) {
        throw new Error('没有生成上架任务');
    }


    // ==========================================================
    // 上架
    // ==========================================================

    must(
        postJSON(
            `/api/v1/inbound/tasks/${putawayTask.id}/putaway`,
            token,
            {
                task_id: putawayTask.id,

                location_id: location.id,

                qty:
                    putawayTask.target_qty -
                    putawayTask.done_qty,
            }
        ),
        '上架'
    );


    // ==========================================================
    // 再次确认库存
    // ==========================================================

    inventory = getInventory(token);

    console.log(
        `准备后库存: ` +
        `stock=${inventory.stock}, ` +
        `available=${inventory.available}, ` +
        `allocated=${inventory.allocated}`
    );

    const ok = check(inventory, {
        '初始 stock=4000':
            (x) => x.stock === TARGET_STOCK,

        '初始 available=4000':
            (x) => x.available === TARGET_STOCK,

        '初始 allocated=0':
            (x) => x.allocated === 0,
    });

    if (!ok) {
        throw new Error('测试库存准备失败');
    }

    return inventory;
}


// ============================================================
// 6. setup
//
// 所有真正并发请求开始之前，只执行一次
// ============================================================

export function setup() {
    if (!WAREHOUSE_ID || !SKU_ID) {
        throw new Error(
            '必须设置 WAREHOUSE_ID 和 SKU_ID'
        );
    }

    const token = login();


    // ----------------------------------------------------------
    // 第一步：准备精确的 4000 件库存
    // ----------------------------------------------------------

    const before =
        prepareInventory(token);


    // ----------------------------------------------------------
    // 第二步：提前创建 60 张订单
    // ----------------------------------------------------------

    const orderIds = [];

    for (let i = 0; i < VUS; i++) {

        const createRes = postJSON(
            '/api/v1/outbound/orders',
            token,
            {
                warehouse_id: WAREHOUSE_ID,

                biz_order_no:
                    uniqueBizNo(`K6PROOF${i}`),

                remark:
                    'k6 60并发库存分配证明',

                details: [
                    {
                        sku_id: SKU_ID,

                        expected_qty: ORDER_QTY,
                    },
                ],
            }
        );

        must(createRes, `创建订单 ${i + 1}`);

        const orderId =
            createRes.json('data.id');


        // --------------------------------------------------------
        // 提交到 SUBMITTED
        // --------------------------------------------------------

        const submitRes = postJSON(
            `/api/v1/outbound/orders/${orderId}/submit`,
            token,
            {}
        );

        must(
            submitRes,
            `提交订单 ${i + 1}`
        );

        orderIds.push(orderId);
    }


    console.log(
        `准备完成：${orderIds.length} 张订单`
    );

    console.log(
        `库存=${before.available}，` +
        `总需求=${VUS * ORDER_QTY}`
    );

    console.log(
        '========== 即将开始并发审核 =========='
    );


    // ----------------------------------------------------------
    // setup 返回的数据会传给所有 VU
    // ----------------------------------------------------------

    return {
        token,
        orderIds,
        before,
    };
}


// ============================================================
// 7. 真正的并发测试
//
// 60 个 VU，每个 VU 只执行一次
// ============================================================

export function approveOne(data) {
    // __VU 从 1 开始
    //
    // VU1  -> orderIds[0]
    // VU2  -> orderIds[1]
    // ...
    // VU60 -> orderIds[59]

    const orderId =
        data.orderIds[__VU - 1];


    const res = postJSON(
        `/api/v1/outbound/orders/${orderId}/approve`,
        data.token,
        {}
    );


    // ----------------------------------------------------------
    // 判断业务是否成功
    // ----------------------------------------------------------

    let code = null;

    try {
        code = res.json('code');
    } catch (_) {
        // 网络错误或非 JSON 响应
    }


    const success =
        res.status >= 200 &&
        res.status < 300 &&
        code === 0;


    if (success) {
        approveSuccess.add(1);

        return;
    }


    // ----------------------------------------------------------
    // 业务拒绝
    //
    // 库存不足导致审核失败是我们期待看到的结果。
    // ----------------------------------------------------------

    if (
        res.status >= 400 &&
        res.status < 500
    ) {
        approveReject.add(1);

        return;
    }


    // ----------------------------------------------------------
    // 5xx / 网络异常才属于真正系统错误
    // ----------------------------------------------------------

    approveSystemFail.add(1);
}


// ============================================================
// 8. teardown
//
// 60 个并发请求全部结束后，只执行一次
// ============================================================

export function teardown(data) {
    console.log(
        '========== 并发审核结束 =========='
    );


    // ----------------------------------------------------------
    // 查询最终库存
    // ----------------------------------------------------------

    const after =
        getInventory(data.token);


    console.log(
        `测试前: ` +
        `stock=${data.before.stock}, ` +
        `available=${data.before.available}, ` +
        `allocated=${data.before.allocated}`
    );

    console.log(
        `测试后: ` +
        `stock=${after.stock}, ` +
        `available=${after.available}, ` +
        `allocated=${after.allocated}`
    );


    // ==========================================================
    // 查询 60 张订单最终状态
    // ==========================================================

    let approved = 0;
    let rejected = 0;
    let unexpected = 0;


    for (
        let i = 0;
        i < data.orderIds.length;
        i++
    ) {
        const orderId =
            data.orderIds[i];

        const res = getJSON(
            `/api/v1/outbound/orders/${orderId}`,
            data.token
        );

        if (
            res.status < 200 ||
            res.status >= 300 ||
            res.json('code') !== 0
        ) {
            unexpected++;

            continue;
        }


        const status =
            res.json('data.order.status');


        // approve 成功以后，
        // 当前项目会进入 PICKING
        if (status === 'PICKING') {
            approved++;
        }

            // approve 因库存不足整体回滚，
        // 单据仍然停在 SUBMITTED
        else if (status === 'SUBMITTED') {
            rejected++;
        }

        else {
            unexpected++;
        }
    }


    // ==========================================================
    // 计算理论结果
    // ==========================================================

    const expectedApproved =
        Math.floor(
            data.before.available /
            ORDER_QTY
        );

    const expectedRejected =
        VUS - expectedApproved;


    const allocatedIncrease =
        after.allocated -
        data.before.allocated;

    const availableDecrease =
        data.before.available -
        after.available;


    console.log('');
    console.log(
        '========== 业务结果 =========='
    );

    console.log(
        `订单总数: ${VUS}`
    );

    console.log(
        `单订单需求: ${ORDER_QTY}`
    );

    console.log(
        `总需求: ${VUS * ORDER_QTY}`
    );

    console.log(
        `理论成功: ${expectedApproved}`
    );

    console.log(
        `实际成功: ${approved}`
    );

    console.log(
        `理论拒绝: ${expectedRejected}`
    );

    console.log(
        `实际拒绝: ${rejected}`
    );


    // ==========================================================
    // 最重要：业务不变量验证
    // ==========================================================

    check(after, {

        // --------------------------------------------------------
        // 1. 任何库存行都不能出现负数
        // --------------------------------------------------------

        '无负库存':
            (x) =>
                x.rows.every(
                    (row) =>
                        Number(row.stock_quantity) >= 0 &&
                        Number(row.available_quantity) >= 0 &&
                        Number(row.allocated_quantity) >= 0
                ),


        // --------------------------------------------------------
        // 2. 每一条库存记录都必须满足三数量模型
        // --------------------------------------------------------

        '每条库存满足 stock=available+allocated':
            (x) =>
                x.rows.every(
                    (row) =>
                        Number(row.stock_quantity) ===
                        Number(row.available_quantity) +
                        Number(row.allocated_quantity)
                ),


        // --------------------------------------------------------
        // 3. 汇总层面也必须成立
        // --------------------------------------------------------

        '汇总库存满足 stock=available+allocated':
            (x) =>
                x.stock ===
                x.available +
                x.allocated,


        // --------------------------------------------------------
        // 4. 分配只改变 available / allocated，
        //    stock 总量不应该变化
        // --------------------------------------------------------

        '库存总量前后不变':
            (x) =>
                x.stock ===
                data.before.stock,


        // --------------------------------------------------------
        // 5. available 减少量必须等于 allocated 增加量
        // --------------------------------------------------------

        '可用减少量=分配增加量':
            () =>
                availableDecrease ===
                allocatedIncrease,


        // --------------------------------------------------------
        // 6. 成功订单数量必须等于理论可分配数量
        // --------------------------------------------------------

        '成功订单数量正确':
            () =>
                approved ===
                expectedApproved,


        // --------------------------------------------------------
        // 7. 剩余订单必须被拒绝
        // --------------------------------------------------------

        '库存不足订单被正确拒绝':
            () =>
                rejected ===
                expectedRejected,


        // --------------------------------------------------------
        // 8. 不允许出现未知订单状态
        // --------------------------------------------------------

        '没有异常订单':
            () =>
                unexpected === 0,


        // --------------------------------------------------------
        // 9. 成功订单分配量和库存变化一致
        // --------------------------------------------------------

        '实际分配量=成功订单数×单订单数量':
            () =>
                allocatedIncrease ===
                approved * ORDER_QTY,
    });


    console.log('');
    console.log(
        '========== 最终结论 =========='
    );

    console.log(
        `stock: ${after.stock}`
    );

    console.log(
        `available: ${after.available}`
    );

    console.log(
        `allocated: ${after.allocated}`
    );

    console.log(
        `approved: ${approved}`
    );

    console.log(
        `rejected: ${rejected}`
    );
}