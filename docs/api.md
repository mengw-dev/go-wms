# GoWMS API 接口文档

> Base URL：`/api/v1` ｜ 数据格式：JSON ｜ 前后端同构：本仓库 `web/src/api/` 即可调用 SDK 参考

## 1. 通用约定

### 1.1 认证

除 `POST /login`、`GET /healthz` 和 `/integration/*` API Key 接口外，所有接口需要请求头：

```text
Authorization: Bearer <token>     # 登录接口返回，HS256 JWT
```

外部系统集成接口使用：

```text
X-API-Key: <WMS_INTEGRATION_API_KEY>
```

### 1.2 统一响应

```json
{ "code": 0, "msg": "ok", "data": { } }
```

| code | 含义 |
| --- | --- |
| 0 | 成功 |
| 非 0 | 业务错误（错误码定义见 `internal/pkg/errcode`，如库存不足、状态不允许等，`msg` 为可直接展示的中文提示） |

分页接口的 `data`：

```json
{ "list": [ ... ], "total": 123 }
```

### 1.3 分页与通用查询参数

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `page` | 1 | 页码，≥1，最大 size 100 |
| `page_size` | 10 | 每页条数 |
| `status` / `keyword` 等 | — | 各列表接口支持的业务过滤，见各节 |

## 2. 系统管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/login` | 公开 | 登录，返回 `{token, user}` |
| GET | `/profile` | 登录 | 当前用户信息（含角色权限串） |
| PUT | `/password` | 登录 | 修改本人密码 `{old_password, new_password}`，成功后需重新登录 |
| GET | `/system/users` | 登录 | 用户列表（keyword 过滤） |
| POST | `/system/users` | `wms:system:user` | 创建用户 |
| PUT | `/system/users/:id` | `wms:system:user` | 更新用户 |
| DELETE | `/system/users/:id` | `wms:system:user` | 删除用户（软删除） |
| PUT | `/system/users/:id/status` | `wms:system:user` | 启用/停用 `{status}` |
| PUT | `/system/users/:id/password` | `wms:system:user` | 管理员重置密码 |
| GET | `/system/roles/all` | 登录 | 全部角色（下拉用，不分页） |
| GET / POST | `/system/roles` | 登录 / `wms:system:role` | 角色列表 / 创建 |
| PUT / DELETE | `/system/roles/:id` | `wms:system:role` | 更新 / 删除角色 |
| GET | `/system/oper-logs` | 登录 | 操作日志（username、时间范围过滤） |

**登录示例**

```json
// POST /api/v1/login
{ "username": "admin", "password": "admin123" }

// 200
{
  "code": 0, "msg": "ok",
  "data": {
    "token": "eyJhbGciOi...",
    "user": { "id": 1, "username": "admin", "nickname": "管理员", "roles": ["admin"], "perms": ["*"] }
  }
}
```

## 3. 基础数据（写入需 `wms:basic`）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET / POST | `/basic/warehouses` | 仓库列表 / 创建 |
| PUT / DELETE | `/basic/warehouses/:id` | 更新 / 删除仓库 |
| PUT | `/basic/warehouses/:id/status` | 启停仓库 |
| GET | `/basic/locations` | 库位列表（warehouse_id 过滤） |
| POST | `/basic/locations/batch` | **批量生成库位**（幂等：已存在编码自动跳过） |
| PUT | `/basic/locations/:id/status` | 库位启停 |
| DELETE | `/basic/locations/:id` | 删除库位 |
| GET | `/basic/skus` | 货品列表 |
| GET | `/basic/skus/barcode/:barcode` | 按条码查货品（Redis 缓存） |
| POST / PUT / DELETE | `/basic/skus[/:id]` | 货品 CRUD |

**批量生成库位示例**（按 `库区-排-列` 规则笛卡尔展开）

```json
// POST /api/v1/basic/locations/batch
{
  "warehouse_id": 1,
  "zone": "A01",
  "row_from": 1, "row_to": 2,
  "col_from": 1, "col_to": 3
}
// 生成 A01-01-01 … A01-02-03，返回 {created: 6, skipped: 0}（已存在的编码自动跳过）
```

## 4. 库存查询（只读，内部写接口不对外）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/inventory` | 库存明细（warehouse_id/sku_id/location_id/keyword 过滤），含三数量与批次 |
| GET | `/inventory/summary` | 按 SKU 汇总（同仓聚合批次） |
| GET | `/inventory/trans` | 库存流水（inventory_id/trans_type/order_no/时间范围过滤） |

## 5. 任务中心

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/tasks` | 任务列表（task_type: RECEIVE/PUTAWAY/PICK、status、order_id 过滤） |
| GET | `/tasks/:id` | 任务详情 |

## 6. 入库管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/inbound/orders` | 登录 | 入库单列表 |
| GET | `/inbound/orders/:id` | 登录 | 单据详情（含明细、可含任务） |
| POST / PUT / DELETE | `/inbound/orders[/:id]` | `wms:inbound:create` | 创建 / 编辑草稿 / 删除草稿 |
| POST | `/inbound/orders/:id/submit` | `wms:inbound:submit` | 提交 DRAFT→SUBMITTED |
| POST | `/inbound/orders/:id/approve` | `wms:inbound:approve` | 审核 SUBMITTED→APPROVED，生成收货任务 |
| POST | `/inbound/orders/:id/cancel` | `wms:inbound:cancel` | 取消（APPROVED 前） |
| POST | `/inbound/orders/:id/receive` | `wms:inbound:receive` | 收货 `{detail_id, qty, defective_qty?, batch_no}`，按明细多次部分收 |
| POST | `/inbound/tasks/:id/putaway` | `wms:inbound:putaway` | 上架 `{task_id, location_id, qty}`：库存 Increase + RECEIVE 流水 |
| POST | `/inbound/import` | `wms:inbound:create` | multipart 上传 Excel，异步建单，返回 `{task_id}` |
| GET | `/inbound/import/:taskId` | 登录 | 导入进度 `{status, total_rows, success_rows, fail_rows, error_msg}` |

**创建入库单示例**

```json
// POST /api/v1/inbound/orders
{
  "warehouse_id": 1,
  "remark": "8月采购到货",
  "details": [
    { "sku_id": 101, "expected_qty": 50 },
    { "sku_id": 102, "expected_qty": 30 }
  ]
}
// data: { "id": 186..., "order_no": "RK20260830-00001", "status": "DRAFT" }
// 批次号在收货时提供（batch_no 必填，FIFO 依据）
```

## 7. 出库管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/outbound/orders` | 登录 | 出库单列表 |
| GET | `/outbound/orders/:id` | 登录 | 详情（含明细与分配行） |
| POST | `/outbound/orders` | `wms:outbound:create` | 创建，`biz_order_no` 唯一幂等 |
| DELETE | `/outbound/orders/:id` | `wms:outbound:create` | 删除草稿 |
| POST | `/outbound/orders/:id/submit` | `wms:outbound:submit` | 提交 |
| POST | `/outbound/orders/:id/approve` | `wms:outbound:approve` | **审核即分配**：FIFO 锁库 + 分配明细 + 拣货任务；库存不足整体失败 |
| POST | `/outbound/orders/:id/cancel` | `wms:outbound:cancel` | 取消并释放锁库（已拣货不可取消） |
| POST | `/outbound/tasks/:id/pick` | `wms:outbound:pick` | 拣货 `{task_id, qty, batch_no?, location_code?}`，可分次；`batch_no/location_code` 为扫码核对值，与任务不一致返回 50008/50009；全部分配行拣完自动发货扣库存 |

**审核失败（防超卖生效）示例**

```json
// POST /api/v1/outbound/orders/1861.../approve
// 200（HTTP 层正常，业务层失败）
{ "code": 40012, "msg": "库存不足，无法完成分配", "data": null }
```

### 7.1 外部 OMS 推送出库单

| 方法 | 路径 | 认证 | 说明 |
| --- | --- | --- | --- |
| POST | `/integration/outbound-orders` | `X-API-Key` | 按仓库/货品编码创建草稿出库单，`biz_order_no` 幂等 |

请求示例：

```json
{
  "warehouse_code": "WH01",
  "biz_order_no": "OMS-20260916-1001",
  "remark": "OMS 推送：门店补货",
  "details": [
    { "sku_code": "SKU000001", "expected_qty": 2 },
    { "sku_code": "SKU000003", "expected_qty": 1 }
  ]
}
```

响应示例：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "order_id": "358740812147724289",
    "order_no": "CK20260916000001",
    "biz_order_no": "OMS-20260916-1001",
    "status": "DRAFT",
    "idempotent": false
  }
}
```

重复推送同一个 `biz_order_no` 时返回原订单，并设置 `idempotent: true`，不会重复建单。

## 8. 盘点管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/stocktake/orders` | 登录 | 盘点单列表 |
| GET | `/stocktake/orders/:id` | 登录 | 详情（含明细快照/实盘/差异） |
| POST | `/stocktake/orders` | `wms:stocktake:create` | 创建即快照 `{warehouse_id, location_id?}` |
| POST | `/stocktake/orders/:id/actual` | `wms:stocktake:stocktake` | 录入实盘 `{detail_id, actual_qty}`（逐行） |
| POST | `/stocktake/orders/:id/approve` | `wms:stocktake:approve` | 审核：锁内重算差异，调整库存 + ADJUST 流水 |
| POST | `/stocktake/orders/:id/cancel` | `wms:stocktake:cancel` | 取消 |

## 9. 演示模式

演示接口都需要 `wms:demo` 权限；除 `session/acquire` 外，还必须携带请求头 `X-Demo-Session`。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/demo/session/acquire` | 获取单实例演示会话；已有会话时返回 HTTP 423 |
| POST | `/demo/session/heartbeat` | 续期当前演示会话 |
| POST | `/demo/session/release` | 释放会话并恢复初始数据 |
| GET | `/demo/session/status` | 查询会话剩余时间 |
| POST | `/demo/run/inbound` | 执行入库、收货、上架场景 |
| POST | `/demo/run/outbound` | 执行出库 FIFO 分配、拣货场景 |
| POST | `/demo/run/stocktake` | 执行盘点、差异调整场景 |
| POST | `/demo/run/full` | 一键走完入库、出库、盘点完整流程 |
| POST | `/demo/run/inbound_drafts` | 模拟 Excel 批量入库草稿 `{ "count": 3, "qty": 20 }`，后续由体验者手工处理 |
| POST | `/demo/run/outbound_drafts` | 模拟上游系统批量出库草稿 `{ "count": 3, "qty": 5 }` |
| POST | `/demo/run/stocktake_drafts` | 批量创建盘点草稿 `{ "count": 2 }` |
| POST | `/demo/run/concurrent` | 并发出库测试 `{ "concurrency": 20, "qty_per_order": 1 }`，展示库存锁、FIFO 和防超卖 |
| GET | `/demo/performance` | 实时性能快照：DB/Redis、连接池、Go 运行时和业务数量 |
| GET | `/demo/activity?limit=20` | 当前演示账号的接口操作、单据、任务和库存流水 |
| POST | `/demo/reset` | 手动恢复初始数据，保留当前会话 |

示例：

```http
POST /api/v1/demo/session/acquire
Authorization: Bearer <demo-token>
```

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "session_id": "uuid",
    "expires_in": 300
  }
}
```

```http
POST /api/v1/demo/run/full
Authorization: Bearer <demo-token>
X-Demo-Session: uuid
```

详细部署和使用方式见 [demo.md](demo.md)。

## 10. 健康检查

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/healthz` | DB 必须可用；Redis 不可用不影响（已降级） |

## 11. 调试建议

```bash
# 1. 登录取 token
curl -s -X POST localhost:8080/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}'

# 2. 携带 token 调业务接口
curl -s localhost:8080/api/v1/inventory?page=1&page_size=10 \
  -H 'Authorization: Bearer <token>'
```
