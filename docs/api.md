# WMS API 接口文档

## 2026-10-22 仪表盘运营信息

原 GET `/dashboard/overview` 补充 `activity`（wms:inventory 权限下的今日上架/发货流水数及最近8笔）和 `import_issues`（wms:inbound:view 权限下近七日创建的异常导入数及最近5次，全部仓库）。前者是流水数，后者是历史异常，均不推导未记录的任务完成时间或“未处理”状态。预警摘要复用已有 GET `/inventory/alerts`，使用独立 checked_at 和精确租户范围；口径、权限、范围与跳转详见[仪表盘运营信息](dashboard-operations.md)。

## 2026-10-22 Excel 导出

新增GET `/inventory/export`、`/inventory/trans/export`、`/stocktake/orders/export`，成功返回完整XLSX附件，错误保持HTTP/JSON；按筛选导出全部匹配明细，最多10000行。权限、参数、精确租户0和盘点差异口径见[Excel导出](excel-export.md)。

## 2026-10-22 单据打印与标签

没有新增后端API。单据打印页面复用原入库/出库/盘点详情GET接口及查看权限；标签取基础资料已加载列表的原条码/库位编码。浏览器预览路径、数据快照、权限与规格见[单据打印与条码标签](document-printing.md)。

## 2026-10-21 库存预警

新增 `/inventory/alerts` 当前预警及规则查询、`/inventory/alert-options` 管理选项、`PUT /inventory/alert-rules` 与 `DELETE /inventory/alert-rules/:id`。规则按仓库+货品配置可用量下限，不改变库存；权限、参数、零库存与恢复口径见[库存预警](inventory-alerts.md)。
## 2026-10-21 基础波次与播种拣货

新增 `/outbound/waves` 列表、同仓候选预览、建波、领取、汇总拣货、槽位分货和显式发货接口。所有写操作要求 `Idempotency-Key`，返回字符串 wave_id；原出库详情任务补充字符串 wave_id，非零任务由波次工作台处理。完整参数、状态、权限和库存口径见[波次与播种拣货](wave-picking.md)。普通逐单拣货继续使用原接口。

> Base URL：`/api/v1` ｜ 数据格式：JSON ｜ 前后端同构：本仓库 `web/src/api/` 即可调用 SDK 参考

## 2026-10-21 出库单据规模

手工及外部新建出库单默认最多200条明细；审核所有明细累计最多1000条库存分配记录及拣货任务，配置项对所有租户生效。超明细返回HTTP400/业务码50020，超分配行返回HTTP400/业务码50021；提示拆单或整理库存。审核失败整单回滚，不产生部分库存、明细、分配、任务或流水。没有新增请求体字段或前端类型。

降低上限后，历史超限单仍可查询/取消，外部同号同内容仍可回放，但不继续审核；回放不新建、不修改历史单。配置及本地验证范围见[规模验证](scale-validation.md)。

## 2026-10-20 仪表盘只读汇总

`GET /dashboard/overview?warehouse_id=<可选正整数>` 需要登录，并经过与其他业务路由相同的 Demo 会话校验。服务端逐块检查原有 `wms:inbound:view`、`wms:outbound:view`、`wms:task`、`wms:inventory`、`wms:basic`、`wms:stocktake:view` 权限；无权限的业务表不查询，相应字段不返回。无任何业务权限时只返回范围与权限元数据。

- `generated_at`、`today_from/to`、`trend_from`：本次统计时点、今日创建时间范围、近七日起点。发布环境继续使用 `TZ=Asia/Shanghai`、MySQL `loc=Local` 与业务 DATETIME 的既有约定。
- `inbound/outbound`：全部当前状态分组计数及其中今日创建数量、近七日按创建日期分组的单量、最近五条新建单据。包含取消单；不是实际收发货数量、完成量或准时率。每日空缺由前端补零。
- `task_counts`：当前 CREATED/IN_PROGRESS 按类型与状态分组；`older_24h` 只表示创建超过 24 小时且未完成，不表示违反 SLA。`oldest_tasks` 最多八条，按创建时间和 ID 升序；仅返回展示和跳转字段，不返回领取凭证等内部字段。
- `inventory`：现存量大于零的库存记录数、去重 SKU 数、其中含分配量的记录数。不把不同单位的商品数量混加。
- `locations`：启用库位数、其中状态为占用的库位数，仅 basic 权限可见；不是按体积计算的仓容利用率。`warehouses` 是同一租户下的仓库选择项。
- `draft_stocktakes`：当前 DRAFT 盘点单数，不推导未经定义的库存准确率。

查询使用业务模型的租户/软删回调，在短只读 repeatable-read 事务内获取一致视图。沿用普通平台上下文的跨租户旁路语义；精确租户 0 仍只能查看租户 0。不存在的仓库返回空范围，非法 ID 返回 400，不进行业务写入或数据库迁移。

`GET /tasks` 新增可选正整数 `warehouse_id`，可与原有 `order_id/task_type/status/keyword` 组合，列表和 total 使用同一过滤条件；不改变任务写入、领取、租约或幂等语义。

## 2026-10-18 参数与会话补充

- 单次数量、单据总数量、单条库存现存量采用 0–2147483647 的 INT 范围；创建明细/收货/上架/拣货要求正整数，残品及实盘允许 0。明细合计和同一库存行累计超限也返回参数错误（400），事务不产生部分写入。跨库存行报表可使用 int64 汇总。
- 创建用户、重置密码、修改密码继续使用 bcrypt：设置密码保持 6–32 字符，同时 UTF-8 长度不超过 72 字节；超出字节上限返回 10017（HTTP 400），不截断密码。前端三个入口同步校验。
- 客户端只有在请求所属认证版本、token、演示会话仍与当前一致时，才因 401/70003 清除会话；旧请求晚返回不会退出新登录。
- 单据详情按每批 200 条聚合全部任务；业务仓库/SKU/库位选项按每页 100 条继续加载到全部结果，任一页失败不返回不完整选项。聚合不承诺跨请求的严格快照一致性。

## 1. 通用约定

### 1.1 认证

`POST /login`、`GET /version` 公开；启用相应体验功能时，`POST /demo/account`、`GET /personal/accounts`、`POST /personal/login` 也公开。根路径 `/healthz` 和 `/version` 用于探针与版本查询。`/integration/*` 使用 API Key，其余业务接口需要请求头：

```text
Authorization: Bearer <token>     # 登录接口返回，HS256 JWT
```

外部系统集成接口使用：

```text
X-API-Key: <WMS_INTEGRATION_API_KEY>
```

API Key 在服务端配置中绑定 `WMS_INTEGRATION_TENANT_ID`，请求体、查询参数和自定义请求头中的租户字段都会被忽略。绑定租户为 `0` 时只访问平台数据，不代表可以访问所有租户；绑定为正数时只访问该租户。

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

### 幂等操作结果未知

收货、上架、PDA 领取/拣货与盘点审核的业务事务失败后，会核对同 key 是否已有成功记录。核对数据库查询失败或请求取消时，不能据原来的状态/数量拒绝断言「未执行」，返回 HTTP **503**、业务码 **50301**（结果暂时无法确认）。客户端必须保留原 `Idempotency-Key`、原参数与待确认记录，恢复后重试同一操作；禁止自动换 key。响应不暴露数据库错误细节。

同 key 内容冲突仍为 HTTP 409 / 40901；已成功且快照合法时回放首次结果。超过 7 天保留窗口先人工核对，不能自动重发。前端详情页的恢复入口独立于单据当前状态，有 pending 的已完成任务仍可选择恢复。

### 数据库并发繁忙（50302）

事务管理器遇到 MySQL 锁等待超时 1205，或死锁 1213 重试耗尽/超出重试窗口时，返回 HTTP **503**、业务码 **50302**，不暴露驱动错误。1205 不自动重试；1213 和既有业务版本冲突可重开整个事务，默认最多 3 次，在首轮开始后的 2 秒窗口内才发起后续事务。等待采用抖动，默认三次尝试的等待合计不足 300ms；窗口不强行中断正在执行的事务，单轮时限仍由请求 context 和数据库超时决定。

业务版本冲突耗尽仍保留原 HTTP 409/业务码。客户端遇到 50302 须保留原操作 key、原参数和待确认记录，稍后恢复同一操作；不能自动换 key 或当成确定未执行。幂等核对本身失败时仍优先返回 50301。

### 1.3 分页与通用查询参数

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `page` | 1 | 页码，≥1，最大 size 100 |
| `page_size` | 10 | 每页条数 |
| `status` / `keyword` 等 | — | 各列表接口支持的业务过滤，见各节 |

## 2. 系统管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/login` | 公开 | 登录，返回 token、user_id、username、nickname、roles、perms |
| GET | `/profile` | 登录 | 当前用户信息（含角色权限串） |
| PUT | `/password` | 登录 | 修改本人密码 `{old_password, new_password}`，成功后需重新登录 |
| GET | `/system/users` | `wms:system:user` | 用户列表（keyword 过滤） |
| POST | `/system/users` | `wms:system:user` | 创建用户 |
| PUT | `/system/users/:id` | `wms:system:user` | 更新用户 |
| DELETE | `/system/users/:id` | `wms:system:user` | 删除用户（软删除） |
| PUT | `/system/users/:id/status` | `wms:system:user` | 启用/停用 `{status}` |
| PUT | `/system/users/:id/password` | `wms:system:user` | 管理员重置密码 |
| GET | `/system/roles/all` | `wms:system:role` | 全部角色（下拉用，不分页） |
| GET / POST | `/system/roles` | `wms:system:role` | 角色列表 / 创建 |
| PUT / DELETE | `/system/roles/:id` | `wms:system:role` | 更新 / 删除角色 |
| GET | `/system/oper-logs` | `wms:system:log` | 操作日志（username、path 过滤） |

**登录示例**

```json
// POST /api/v1/login
{ "username": "admin", "password": "admin123", "tenant_id": "0" }

// 200
{
  "code": 0, "msg": "ok",
  "data": {
    "token": "eyJhbGciOi...",
    "user_id": "1", "username": "admin", "nickname": "管理员", "roles": ["admin"], "perms": ["*"]
  }
}
```

`tenant_id` 是可选的非负整数**字符串**。省略或传 null 时，仅当用户名在未软删账号中唯一才允许登录；不同租户存在同名账号时需填写所属租户编号。显式 `"0"` 只匹配平台账号，不使用跨租户旁路。账号不存在、租户不匹配、用户名有歧义或密码错误统一返回 10002，不返回其他租户的账号列表。演示与个人空间领取接口返回 `tenant_id`，前端自动携带它登录。上述 admin 密码仅是本地种子示例，部署配置应使用自己的凭据。

登录限流在当前进程内分两层原子预占：查询前按客户端 IP 限制 15 分钟内最多 100 次登录请求（包括成功和不存在账号，不因单次登录成功重置）；解析账号后按「租户 ID + 用户 ID + IP」限制最多 5 次尝试，账号登录成功后清空该账号额度。大小写、重音等经数据库匹配到同一账号的写法，以及省略租户/显式租户的同一账号，共享账号额度；不同租户的账号独立。失败或取消保留已预占次数。两层共用最多 10000 个 key 的有界记录，每分钟在请求到来时清理过期项，容量满时拒绝新 key。这不是跨实例统一限流，不能替代入口访问频率限制。

用户更新中的 `nickname`、`status`、`role_ids` 都可省略。省略或 null 保留原值；`nickname: ""` 清空昵称，`status: 0` 禁用，`role_ids: []` 清空角色关联。内置管理员禁止通过用户管理修改、删除或重置密码，可通过 `/password` 验证旧密码后修改自己的密码。

## 3. 基础数据（写入需 `wms:basic`）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET / POST | `/basic/warehouses` | 仓库列表 / 创建 |
| PUT / DELETE | `/basic/warehouses/:id` | 更新 / 删除仓库 |
| PUT | `/basic/warehouses/:id/status` | 启停仓库 |
| GET | `/basic/locations` | 库位列表（warehouse_id 过滤） |
| POST | `/basic/locations/batch` | **批量生成库位**（已存在编码跳过；排/列端点 1–2147483647，每批最多 1000 个） |
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

## 4. 库存查询与仓内移库

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/inventory` | 库存明细（warehouse_id/sku_id/location_id/keyword 过滤），含现存/可用/分配/冻结量与批次 |
| GET | `/inventory/summary` | 按 SKU 汇总（同仓聚合批次） |
| GET | `/inventory/trans` | 库存流水（inventory_id/trans_type/order_no/时间范围过滤） |

## 5. 任务中心

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/tasks` | 任务列表（task_type: RECEIVE/PUTAWAY/PICK、status、order_id、warehouse_id过滤；keyword按任务号或来源单号包含匹配）；已领取任务返回 `claimed_by` 与 `lease_expire_at` |
| GET | `/tasks/:id` | 任务详情（含租约字段，未领取时省略） |

## 6. 入库管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/inbound/orders` | 登录 | 入库单列表 |
| GET | `/inbound/orders/:id` | 登录 | 单据详情（含明细、可含任务） |
| POST / PUT / DELETE | `/inbound/orders[/:id]` | `wms:inbound:create` | 创建 / 编辑草稿 / 删除草稿 |
| POST | `/inbound/orders/:id/submit` | `wms:inbound:submit` | 提交 DRAFT→SUBMITTED |
| POST | `/inbound/orders/:id/approve` | `wms:inbound:approve` | 审核 SUBMITTED→APPROVED，生成收货任务 |
| POST | `/inbound/orders/:id/cancel` | `wms:inbound:cancel` | 取消（APPROVED 前） |
| POST | `/inbound/orders/:id/receive` | `wms:inbound:receive` | 收货 `{detail_id, qty, defective_qty?, batch_no}`；`qty` 为含残品的总量，`0 ≤ defective_qty ≤ qty`，支持按明细部分收货 |

**收货幂等**：请求头可选携带 `Idempotency-Key`（项目自带客户端全部携带）。同一 key + 相同内容（指纹含单据、明细、数量、残品量与批次号，批次按精确比较）重试回放空成功（`data` 仍为 `null`），不重复累计、收齐后重放不重复生成上架任务；同 key 不同内容返回 409 / 40901；幂等记录与业务写入同事务提交，业务失败不留记录；未携带 key 的调用方保持旧契约但**不提供请求级去重**。幂等窗口与 PDA 一致（7 天保留期）。
| POST | `/inbound/tasks/:id/putaway` | `wms:inbound:putaway` | 上架 `{location_id, qty}`，路径 `:id` 为 task_id：库存 Increase + RECEIVE 流水 |

**上架幂等**：请求头可选携带 `Idempotency-Key`（项目自带客户端全部携带）。同一 key + 相同内容（指纹含任务、库位与数量）重试回放空成功（`data` 仍为 `null`），不重复增加库存、流水或任务进度；同 key 不同内容返回 409 / 40901；幂等记录与库存生效、库位占用、任务推进、单据状态同事务提交，业务失败整体回滚不留记录；未携带 key 的调用方保持旧契约但**不提供请求级去重**。幂等窗口与 PDA 一致（7 天保留期）。
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
| POST | `/outbound/tasks/:id/pick` | `wms:outbound:pick` | 拣货 `{qty, batch_no?, location_code?, claim_token?}`，路径 `:id` 为 task_id，可分次；`batch_no/location_code` 为扫码核对值（可选，填了才校验，不一致返回 50008/50009）；请求头可带 `Idempotency-Key` 做请求级幂等（指纹含任务、数量、规范化后的库位/批次与入口类型，不同内容复用返回 40901）；成功与业务拒绝均返回任务快照；全部分配行拣完自动发货扣库存 |

**库存不足导致审核失败的示例**

```json
// POST /api/v1/outbound/orders/1861.../approve
// 400（HTTP 状态按业务错误码映射；30201 属普通业务错误 → 400，body.code 才是业务错误码）
{ "code": 30201, "msg": "可用库存不足", "data": null }
```

### 7.1 外部 OMS 推送出库单

| 方法 | 路径 | 认证 | 说明 |
| --- | --- | --- | --- |
| POST | `/integration/outbound-orders` | `X-API-Key` | 按固定租户的仓库/货品编码创建草稿出库单，`biz_order_no` 幂等 |

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

重复推送同一 `biz_order_no` 时按创建内容核对：

- 内容一致（仓库编码解析结果、备注、明细的「SKU ID + 数量」集合一致（请求编码先按数据库规则解析为商品身份）；**明细顺序不影响相等判断**）→ 返回原订单并设置 `idempotent: true`，不重复建单；
- 内容不一致（数量、SKU 集合、备注或仓库任一不同）→ HTTP 409，业务码 `50012`；
- 历史订单因缺少 SKU 编码快照无法可靠核对内容 → HTTP 409，业务码 `50013`，需人工核对，不会被当作重复成功放行；
- 同一编码的 SKU 不允许重复出现（`50007`）。

内容核对只比较创建期不可变字段（仓库、备注、明细 SKU ID 与数量快照）；创建时编码保留作历史展示，主数据仅变更大小写后仍能回放。请求编码无法解析或商品已不存在时仍返回主数据错误，订单后续状态、已拣数量、更新时间不参与。两条路径口径一致：首次查询命中业务单号、并发撞唯一键后回查。业务单号去重跟随订单生命周期保留，不依赖 7 天清理的请求级幂等记录；同一 `biz_order_no` 在不同租户下相互独立，并发推送同一业务单号时数据库唯一约束保证只创建一张单。

```json
// POST /api/v1/integration/outbound-orders
// 同号不同内容，HTTP 409
{ "code": 50012, "msg": "业务订单号已存在，且创建内容不一致，请核对", "data": null }
```

### 7.2 PDA 拣货（领取租约 + 强制扫码）

现场作业使用 PDA 专用入口：领取与拣货都必须携带 `Idempotency-Key`（缺失返回 400），库位必填（任务有批次时批次必填），服务端强校验；
领取任务后携带领取凭证提交，任务被他人接手后旧凭证自动失效；同一 `Idempotency-Key` 重试领取会回放首次凭证，不轮换 token。

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/pda/tasks/:id/claim` | `wms:outbound:pick` | 领取（或本人续领）任务租约：缺 `Idempotency-Key` 400；返回 `claim_token`、租约到期时间与任务快照；被他人持有且租约未过期返回 40017（HTTP 409） |
| POST | `/pda/tasks/:id/pick` | `wms:outbound:pick` | 强制扫码拣货：缺 `Idempotency-Key` 400；缺库位 50010；任务有批次但未扫批次 50011；与任务不一致 50009/50008；凭证不符 40018（HTTP 409）；租约过期 40019（HTTP 409） |

**领取响应示例**

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "claim_token": "365589593526898688",
    "lease_expire_at": "2026-10-05T20:11:37+08:00",
    "task_status": "IN_PROGRESS",
    "done_qty": 1,
    "remaining_qty": 1,
    "order_status": "PICKING"
  }
}
```

**拣货请求示例**

```json
// POST /api/v1/pda/tasks/3655.../pick
// Header: Idempotency-Key: 3f2c...（PDA 入口必填，缺失 400；重试必须复用同一 key）
{
  "qty": 1,
  "location_code": "A01-02-03",
  "batch_no": "B20261005",
  "claim_token": "365589593526898688"
}
// data: { "task_status": "IN_PROGRESS", "done_qty": 1, "remaining_qty": 1, "order_status": "PICKING" }
```

**语义说明**

- 幂等重放：同一 `Idempotency-Key` + 相同内容（比较前对库位/批次去空格并折叠大小写；指纹含任务、数量、入口类型）回放首次成功的响应快照（不重复扣减，也不受凭证过期影响）；同 key 不同内容返回 409 / 40901；快照缺失或损坏返回 500，不用当前进度兜底。
- 幂等窗口：幂等保证仅覆盖记录保留窗口——`wms_idempotency` 记录保留 7 天，超期由后台清理；窗口外重放同一 key 会按新请求执行。
- 业务拒绝（数量超限、任务被取消、被他人持有等）仍返回 400/409，`data` 带当前任务快照，PDA 可就地刷新进度，不必退出重进。
- 租约 10 分钟，每次成功拣货自动续租；无租约 / 租约过期 / 本人持有时可领取或续领，被接手后旧设备凭证失效（40018，HTTP 409）。
- 后台 `/outbound/tasks/:id/pick` 保持宽松语义（扫码、凭证与 `Idempotency-Key` 均可选），供人工处理。

## 8. 盘点管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/stocktake/orders` | `wms:stocktake:view` | 盘点单列表，可按单号keyword、warehouse_id、status组合筛选 |
| GET | `/stocktake/orders/:id` | `wms:stocktake:view` | 详情（含明细快照/实盘/差异） |
| POST | `/stocktake/orders` | `wms:stocktake:create` | 创建即快照 `{warehouse_id, location_id?}` |
| POST | `/stocktake/orders/:id/actual` | `wms:stocktake:stocktake` | 录入实盘 `{detail_id, actual_qty}`（逐行）；数量必填且 ≥0，0 合法，缺失/null 拒绝且不改明细 |
| POST | `/stocktake/orders/:id/approve` | `wms:stocktake:approve` | 审核：锁内重算差异，调整库存 + ADJUST 流水 |
| POST | `/stocktake/orders/:id/cancel` | `wms:stocktake:cancel` | 取消 |

`keyword` 按单号包含匹配，`%`、`_`、反斜杠按字面值转义，与入出库列表一致；空关键字不加此条件。`page`默认1，`page_size`默认10、上限100；结果按ID倒序，total与列表使用相同过滤及租户/软删规则。

详情保持 `{order, details}`，`order.id`及明细关联ID为字符串；内部version/deleted_at不返回。明细 `actual_qty: null` 表示未盘，`0` 表示明确盘零，不能互相替代。

**盘点审核幂等**：请求头可选携带 `Idempotency-Key`（项目自带客户端全部携带）。同一 key 重试回放空成功（`data` 仍为 `null`）——审核成功但响应丢失后的重试不会因「已终态」被误报为业务失败；同 key 用于其他盘点单返回 409 / 40901；幂等记录与差异调整、ADJUST 流水、明细标记、状态推进同事务提交，业务失败整体回滚不留记录；未携带 key 的调用方保持旧契约但**不提供请求级去重**。终态防重复调整仍由状态机与明细 `adjusted` 标记保证。幂等窗口与 PDA 一致（7 天保留期）。

## 9. 演示模式

演示接口都需要 `wms:demo` 权限；除 `session/acquire` 外，还必须携带请求头 `X-Demo-Session`。

`/demo/run/*` 的可选请求体只接受空输入（含空白）或单个 JSON 对象。非法、截断、数组、标量、`null`、拼接/尾随 JSON 及字段类型/绑定校验失败均返回参数错误（HTTP 400），不执行默认场景；不依赖 `Content-Length`。空体与 `{}` 仍沿用服务默认值与数量上限。完整场景只通过一次 `/demo/run/full` 编排，避免分段场景各自重置数据。

首页分步运行器调用真实业务接口，收货、上架、后台拣货均携带 `Idempotency-Key`；检查点、响应丢失核对与暂停边界见 [演示说明](demo.md#初始数据与分步恢复)。初始化流水新增 `OPENING` 类型（期初库存，`order_no=SEED-*`），不是收货或盘点单据。

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
| POST | `/demo/run/concurrent` | 并发出库分配实验 `{ "concurrency": 20, "qty_per_order": 1 }`，展示库存锁、FIFO 分配及成功/拒绝结果 |
| GET | `/demo/performance` | 运行状态与指标快照：DB/Redis、连接池、Go 运行时和业务数量 |
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

## 12. ID 与接口契约

数据库内部主键仍使用 `BIGINT`，Go 代码内部仍使用 `int64`。所有对外的 `id`、`*_id` 字段在 JSON 中使用字符串传输，避免 JavaScript 超过 `Number.MAX_SAFE_INTEGER` 后发生精度丢失。

示例：

```json
{
  "id": "357813313721077761",
  "warehouse_id": "2",
  "sku_id": "30"
}
```

客户端请求也应发送字符串 ID。角色 ID 数组兼容接收字符串和数字，但服务端始终输出字符串。

## 13. 权限说明

管理员角色使用 `*` 拥有全部权限，其他角色可按模块配置：

| 权限 | 用途 |
| --- | --- |
| `wms:system:user` | 用户管理 |
| `wms:system:role` | 角色管理 |
| `wms:system:log` | 操作日志 |
| `wms:basic` | 仓库、库位、货品读取与维护 |
| `wms:inventory` | 库存、汇总、流水 |
| `wms:task` | 任务中心 |
| `wms:inbound:view/create/submit/approve/cancel/receive/putaway` | 入库各阶段 |
| `wms:outbound:view/create/submit/approve/cancel/pick` | 出库各阶段 |
| `wms:stocktake:view/create/stocktake/approve/cancel` | 盘点各阶段 |

多个权限使用英文逗号分隔，例如：

```text
wms:basic,wms:inventory,wms:task,wms:inbound:view,wms:inbound:receive
```

## 14. AI 库存问答

`POST /api/v1/ai/chat`，需要登录和 `wms:inventory` 权限。请求 `{"question":"库存情况"}`；成功返回统一响应中的 `data.answer`，后端检索真实库存快照，不允许模型生成并执行 SQL。

- 分钟限流默认每用户10次；日额度默认每正租户100次，平台租户不限。两者用 Redis 单键 Lua 原子计数/设置过期，按提问尝试计数，失败或取消不自动返还。Redis 故障仍按现有策略降级放行。
- `429 / 80002`：分钟频率超限；`429 / 80004`：日额度耗尽；`503 / 80003`：未配置模型密钥；`503 / 80001`：模型不可用或问答预算耗尽。
- 限流、快照和主备请求共用默认25秒预算；主模型有独立备用时最多使用剩余时间一半。取消或总预算到期不继续调用备用，主模型401/403也不切换备用。
- 默认外层服务端/代理30秒、前端35秒。前端提供停止等待，并在离页/身份变化时取消；取消不撤销已提交的额度计数，也不承诺第三方模型退费。会话为单轮，前端对话历史只保存在当前页面内存。

## 仓内移库

新增GET `/inventory/moves`、GET `/inventory/move-source/:id`、GET `/inventory/move-locations` 与POST `/inventory/moves`。写操作必须同时拥有库存查看和 `wms:inventory:move` 权限，并传入 `Idempotency-Key`。同仓、批次、数量、持久回放和记录查询契约见[仓内移库](stock-movement.md)。

## 扫码辅助作业

新增只读GET `/inbound/orders/:id/scan-sku`、`/outbound/orders/:id/scan-sku` 与 `/inbound/orders/:id/putaway-locations`。按当前单据及精确租户核对货品或同仓启用库位，不自动提交业务；参数、权限、编码歧义及设备边界见[扫码说明](scan-assistance.md)。

## 异常、冻结与客户退货

新增命令的权限、请求、响应及原请求恢复规则见[接口清单](warehouse-closure.md#接口)。所有 ID 使用 JSON 字符串；写入必须携带 `Idempotency-Key`，未知结果保留原键与参数。
