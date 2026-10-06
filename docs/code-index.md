# WMS 代码索引（code-index）

> 本文件基于**本地真实代码**生成（Go 后端 + Vue 前端 + 测试/脚本/部署）。
> 生成原则：不猜测。文件路径、`struct` / `interface` / 函数名逐字来自源码；职责优先采用源码文档注释，无注释处依据实现摘要；无法从代码确认的内容一律标注 **“待确认”**。
> 若本索引与代码不一致，**以代码为准**。

## 0. 使用说明

- 路径约定：第一、二、四部分的路径相对仓库根目录 `d:\wms-go\wms1\wms`；**第三部分（前端）的路径相对 `web/src/`**。
- 阅读顺序建议：先读本总览与“五、跨模块业务链路”，再按需下钻到对应模块小节。
- 每文件小节字段：核心符号（真实职责）、主要调用关系、测试文件、涉及表/模型。
- 标注为“待确认”的条目表示当前无法从代码中明确证实，请以实际代码为准。

## 0.1 技术栈与顶层目录

- 后端：Go（Gin + GORM + MySQL + Redis + Viper），入口 `cmd/wms/main.go`；迁移工具 `cmd/migrate/main.go`。
- 前端：Vue 3 `<script setup>` + TypeScript + Pinia + Vue Router 4 + Element Plus + Vite（axios）。
- 顶层目录：
  - `cmd/` 可执行入口（wms 服务、migrate 迁移）
  - `internal/` 后端代码：`app`（组装/路由）、`bootstrap`（DB/迁移/种子）、`modules/`（业务模块）、`pkg/`（通用能力）、`testutil/`（测试基础设施）
  - `migrations/` SQL 迁移（`embed.go` 内嵌 `versions/*.sql`）
  - `configs/` 配置（`config.yaml`，敏感项由 `WMS_*` 环境变量覆盖）
  - `deploy/` Docker Compose / Caddy / Prometheus / Grafana
  - `scripts/` 起栈、验证、E2E、k6 压测脚本
  - `web/` 前端工程（`src/`、`tests/e2e/`）
  - `docs/` 文档
  - `loadtest/`、`samples/`、`tmpgen/` 辅助目录（详见第四部分）

## 0.2 后端业务模块一览

| 模块 | 目录 | 职责概述 |
| --- | --- | --- |
| 入库 | `internal/modules/inbound` | 入库单生命周期、收货、上架、Excel 异步导入 |
| 出库 | `internal/modules/outbound` | 出库单生命周期、审核 + FIFO 分配、拣货、发货、集成下单 |
| 任务 | `internal/modules/task` | 收货/上架/拣货任务的状态与进度 |
| 库存 | `internal/modules/inventory` | 库存汇总/明细、并发分配与扣减、调整、流水 |
| 盘点 | `internal/modules/stocktake` | 盘点单、账面快照、实盘录入、差异、审核调整 |
| 基础数据 | `internal/modules/basic` | 仓库、库位、SKU |
| 系统 | `internal/modules/system` | 鉴权（登录/JWT）、权限、用户、角色、操作日志 |
| 演示 | `internal/modules/demo` | 演示账号会话、场景编排、实验、活动证据、性能数据 |
| AI | `internal/modules/ai` | 库存问答（智谱 BigModel） |

## 0.3 数据库表（来自 `migrations/versions/000001_init.up.sql`）

- 系统：`sys_user`、`sys_role`、`sys_user_role`、`sys_oper_log`
- 基础数据：`wms_warehouse`、`wms_location`、`wms_sku`
- 库存：`wms_inventory`、`wms_inventory_trans`
- 任务：`wms_task`
- 入库：`wms_receipt_order`、`wms_receipt_order_detail`、`wms_import_task`
- 出库：`wms_shipment_order`、`wms_shipment_order_detail`、`wms_allocation`
- 盘点：`wms_stocktake_order`、`wms_stocktake_detail`

后续迁移：`000002` 库存版本列、`000003` 任务拣货库位、`000004` 核心索引、`000005` 多租户加列、`000006` 导入 `run_token`、`000007` 租户前置索引、`000008` 库存 CHECK 不变量、`000009` 请求幂等表 `wms_idempotency`、`000010` 拣货任务租约列（详见第二部分 migrations 小节）。

---

## 一、后端核心业务模块

覆盖目录：`internal/modules/inbound`、`outbound`、`task`、`inventory`、`stocktake`（共 68 个 .go 文件）。

模块组装入口：`internal/app/app.go` 的 `App.New` 按 `basic → inventory`、`inbound/outbound → basic + inventory + task` 顺序注入；后台 worker 由 `cmd/wms/main.go` 启动（`InboundService.RunCompensator` / `RunImports`）。

### 入库 inbound — `internal/modules/inbound/service/service.go`
- **核心符号**
  - `type Service struct{...}` — 入库单/收货/上架/导入的业务服务，持有 repo、事务管理器、单号生成器和 basic/inventory/task 接口。
  - `func New(repo *repository.Repository, tm *tx.Manager, no *orderno.Generator, basic basicapi.BasicAPI, inv invapi.InventoryAPI, taskAPI taskapi.TaskAPI, uploadDir string, limits config.LimitsConfig) *Service` — 构造并注入依赖。
- **主要调用关系**：被 `internal/app/app.go` 创建并注入 `inbound/handler`、demo 服务；`main.go` 调用其 worker 方法。
- **测试文件**：无（同包有 `response_test.go`）。
- **涉及表/模型**：`model.ReceiptOrder`(`wms_receipt_order`)、`model.ReceiptOrderDetail`(`wms_receipt_order_detail`)、`model.ImportTask`(`wms_import_task`)。

### 入库 inbound — `internal/modules/inbound/service/order.go`
- **核心符号**
  - `func (s *Service) Create(ctx, req *dto.CreateOrderReq, operator string)` — 创建 DRAFT 入库单。
  - `func (s *Service) CreateResponse(...)` — 返回创建单的稳定 HTTP 响应。
  - `func (s *Service) createImportOrder(...)` — 导入建单，按导入行幂等复用。
  - `func (s *Service) createOrder(ctx, req, operator, prepare func(*gorm.DB, *model.ReceiptOrder) error)` — 建单统一入口（配额校验、仓库校验、明细构建、单号冲突重试）。
  - `func (s *Service) Update/Delete/Submit/Approve/Cancel` — 单据生命周期状态推进；`Approve` 生成收货任务。
  - `func (s *Service) batchOper/BatchDelete/BatchSubmit/BatchApprove/BatchCancel/DeleteByImportTask` — 批量操作，逐张执行部分成功不回滚。
  - `func (s *Service) transit(ctx, id, from, to)` — 通用状态流转（行锁 + `model.CanTransit` + CAS）。
  - `func (s *Service) buildDetails(...)` — 校验重复 SKU、取 SKU 快照、构建明细并累计 `expected`。
- **主要调用关系**：调用 `s.repo.*`、`s.taskAPI.Create`/`CancelByOrder`、`quota.Guard`、`s.basic.ValidateWarehouse/GetSKU`；被 `service/query.go`、`handler` 及 `import_parse.go`（`createImportOrder`）调用。
- **测试文件**：`order_test.go`、`response_test.go`。
- **涉及表/模型**：`wms_receipt_order`、`wms_receipt_order_detail`。

### 入库 inbound — `internal/modules/inbound/service/receiving.go`
- **核心符号**
  - `func (s *Service) Receive(ctx, orderID, detailID int64, req *dto.ReceiveReq, operator string) error` — 收货登记：校验状态/剩余量/批次号，原子累加明细与主单，收齐后生成上架任务并推进单据状态。
- **主要调用关系**：调用 `s.repo.GetOrderForUpdate/GetDetailForUpdate/IncrDetailReceive/IncrOrderReceive/ListDetails`、`s.taskAPI.AddProgressByDetail/Create`；被 `handler.receive` 调用。
- **测试文件**：`receiving_test.go`。
- **涉及表/模型**：`wms_receipt_order`、`wms_receipt_order_detail`；任务表 `wms_task`。

### 入库 inbound — `internal/modules/inbound/service/putaway.go`
- **核心符号**
  - `func (s *Service) Putaway(ctx, taskID, locationID int64, qty int, operator string) error` — 上架作业：库位/任务校验，调用库存 `Increase` 使库存生效，标记库位占用，推进任务，上架任务全部完成则单据 COMPLETED。
- **主要调用关系**：调用 `s.taskAPI.Get/GetForUpdate/AddProgress/CountUnfinished`、`s.inv.Increase`、`s.basic.ValidateLocationInWarehouse/UpdateLocationStatusInTx`；被 `handler.putaway` 调用。
- **测试文件**：无。
- **涉及表/模型**：`wms_receipt_order`、`wms_receipt_order_detail`；库存 `wms_inventory`（经 `invapi`）。

### 入库 inbound — `internal/modules/inbound/service/query.go`
- **核心符号**
  - `type OrderDetail struct{...}` — 入库单详情聚合（单据 + 明细 + 任务）。
  - `func (s *Service) Get / GetResponse` — 详情查询。
  - `func (s *Service) List / ListResponses` — 列表查询与响应转换。
  - `func orderDetailRowResponses / orderTaskResponses / orderResponse` — 隐藏内部字段的响应映射。
- **主要调用关系**：调用 `s.repo.GetOrder/ListDetails/ListOrders`、`s.taskAPI.List`；被 `handler.list/get` 调用。
- **测试文件**：`query_test.go`、`response_test.go`。
- **涉及表/模型**：`wms_receipt_order`、`wms_receipt_order_detail`、`wms_task`。

### 入库 inbound — `internal/modules/inbound/service/import.go`
- **核心符号**
  - `func (s *Service) Import(ctx, fileName string, data []byte) (*dto.ImportResp, error)` — 保存上传文件、落库 PENDING 导入任务，返回 `task_id`。
  - `func (s *Service) GetImport / ListImports / GetImportResponse / ListImportResponses` — 导入任务查询。
  - `func (s *Service) removeImportFile(ctx, task)` — 成功后删除源文件，失败任务保留文件。
  - `func importTaskResponse(task *model.ImportTask)` — 不暴露 token/文件路径的响应映射。
- **主要调用关系**：调用 `s.repo.CreateImportTask/GetImportTask/ListImportTasks`；被 `handler.importExcel/importStatus/listImports` 调用。
- **测试文件**：`import_test.go`。
- **涉及表/模型**：`wms_import_task`。

### 入库 inbound — `internal/modules/inbound/service/import_parse.go`
- **核心符号**
  - `type importResult struct{...}` — 导入结果统计（Total/Success/Failed/Message/Interrupted）。
  - `func (r importResult) status() model.ImportTaskStatus` — 由结果推导任务状态（中断或无成功行视为 FAILED）。
  - `func (s *Service) doImport(ctx, task) importResult` — 用 excelize 解析 xlsx，校验表头、配额、逐行建单，panic 转失败。
  - `func (s *Service) importRow(ctx, task, rowNo int, row []string) error` — 解析单行（仓库编码/货品编码/预期数量/备注）并建单。
- **主要调用关系**：被 `import_worker.go processImport` 调用；调用 `s.createImportOrder`、`s.basic.GetWarehouseByCode/GetSKUByCode`。
- **测试文件**：`import_test.go`。
- **涉及表/模型**：`wms_import_task`、`wms_receipt_order`。

### 入库 inbound — `internal/modules/inbound/service/import_worker.go`
- **核心符号**
  - `func (s *Service) RunImports(ctx)` — 单实例串行 worker 循环，领取 PENDING 任务并处理。
  - `func (s *Service) processImport(parent, task) error` — 领取（run token）、起心跳、执行导入、限时保存终态或归还任务。
  - `func (s *Service) heartbeatImport(ctx, cancel, task)` — 心跳续约，失去执行权则取消。
  - `func (s *Service) RunCompensator(ctx)` / `compensateOnce(ctx)` — 定期把超时 PROCESSING 任务归还 PENDING。
- **主要调用关系**：由 `cmd/wms/main.go` 启动；调用 `s.repo.NextPendingImport/ClaimImport/TouchImport/FinishImport/ReleaseImport/ListStaleImports/ResetStaleImport`、`tenant.WithTenant`。
- **测试文件**：`import_test.go`（含 worker 恢复/停止用例）。
- **涉及表/模型**：`wms_import_task`。

### 入库 inbound — `internal/modules/inbound/model/model.go`
- **核心符号**
  - `type OrderStatus string` + consts — 入库单状态；`type ImportTaskStatus string` + consts — 导入任务状态。
  - `var StatusTransitions` / `func CanTransit(from, to OrderStatus) bool` — 状态机唯一校验入口。
  - `type ReceiptOrder struct{...}` / `ReceiptOrderDetail` / `ImportTask` — 持久化模型（含 `ImportTaskID`+`ImportRow` 幂等键、`RunToken`）。
- **主要调用关系**：被本模块所有 service/repository 及各测试引用。
- **测试文件**：`model_test.go`。
- **涉及表/模型**：`wms_receipt_order`、`wms_receipt_order_detail`、`wms_import_task`。

### 入库 inbound — `internal/modules/inbound/repository/repository.go`
- **核心符号**
  - `type Repository struct{}` / `func New() *Repository`。
  - `func (r *Repository) CreateOrder / GetOrderForUpdate / GetOrder / GetByImportRow` — 单据读写（`GetOrderForUpdate` 用 `clause.Locking`）。
  - `func (r *Repository) UpdateStatus(tx, id, from, to) (int64, error)` — status CAS + version 递增。
  - `func (r *Repository) ReplaceDetails / DeleteOrder / ListDetails / GetDetailForUpdate` — 明细维护。
  - `func (r *Repository) IncrDetailReceive / IncrOrderReceive` — 原子累加收货量与状态推进（乐观锁）。
  - `func (r *Repository) ListOrders / ListIDsByImportTask` — 查询。
- **主要调用关系**：被 `inbound/service` 全模块调用。
- **测试文件**：无。
- **涉及表/模型**：`wms_receipt_order`、`wms_receipt_order_detail`。

### 入库 inbound — `internal/modules/inbound/repository/import.go`
- **核心符号**
  - `func (r *Repository) CreateImportTask / GetImportTask / NextPendingImport` — 导入任务读写/领取扫描。
  - `func (r *Repository) ClaimImport(db, task, token) (bool, error)` — PENDING→PROCESSING 条件更新抢锁。
  - `func ownedImport(db, task) *gorm.DB` — 以 id+tenant+status+run_token 限定执行权。
  - `func (r *Repository) LockImportExecution / FinishImport / TouchImport / ReleaseImport / ListStaleImports / ResetStaleImport / ListImportTasks` — 执行权与补偿相关操作。
- **主要调用关系**：被 `inbound/service/import*.go` 调用。
- **测试文件**：无。
- **涉及表/模型**：`wms_import_task`、`wms_receipt_order`（`ListImportTasks` 子查询）。

### 入库 inbound — `internal/modules/inbound/handler/handler.go`
- **核心符号**
  - `type Handler struct{...}` / `func New(svc *service.Service) *Handler`。
  - `func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker)` — 注册 `/inbound` 路由与 `wms:inbound:*` 权限。
  - 私有方法 `list/get/create/update/delete/submit/approve/cancel/batch*/receive/putaway/importExcel/importStatus/listImports/deleteByImportTask`。
- **主要调用关系**：调用 `service.Service` 各方法、`httpx`、`middleware`；路由由 router 挂载。
- **测试文件**：`handler_test.go`。
- **涉及表/模型**：无（HTTP 层）。

### 入库 inbound — `internal/modules/inbound/dto/dto.go`
- **核心符号**
  - `type CreateOrderReq / OrderDetailItem / OrderQuery / ReceiveReq / PutawayReq / ImportResp` — 请求结构。
  - `type OrderResp / OrderDetailResp / OrderDetailRowResp / OrderTaskResp / ImportTaskResp / BatchOperReq / BatchOperResp / BatchItemError` — 稳定响应契约。
- **主要调用关系**：被 handler、service、测试引用。
- **测试文件**：`dto_test.go`。
- **涉及表/模型**：无（传输对象）。

### 入库 inbound — `internal/modules/inbound/dto/dto_test.go`
- **核心符号**：`func TestCreateOrderReqAcceptsStringIDs(t *testing.T)` — 校验 `CreateOrderReq` 接受字符串型 ID。
- **主要调用关系**：测试 `dto.CreateOrderReq`。
- **测试文件**：`dto_test.go`（本文件）。
- **涉及表/模型**：无。

### 入库 inbound — `internal/modules/inbound/handler/handler_test.go`
- **核心符号**：`func TestListImportsRejectsInvalidLimitBeforeServiceCall(t *testing.T)` — 非法 limit 在调用 service 前即拒绝。
- **主要调用关系**：测试 `handler.listImports`。
- **测试文件**：`handler_test.go`（本文件）。
- **涉及表/模型**：无。

### 入库 inbound — `internal/modules/inbound/model/model_test.go`
- **核心符号**：`func TestCanTransit(t *testing.T)` — 校验入库单状态转换表；`func TestImportTaskJSONDoesNotExposeExecutionSecrets(t *testing.T)` — 确认 `ImportTask` JSON 不暴露敏感字段。
- **主要调用关系**：测试 `model.CanTransit`、`model.ImportTask`。
- **测试文件**：`model_test.go`（本文件）。
- **涉及表/模型**：`wms_receipt_order`、`wms_import_task`。

### 入库 inbound — `internal/modules/inbound/service/import_test.go`
- **核心符号**：`TestImportFileFailuresAndStrictQuantity` / `TestImportOwnershipAndIdempotency` / `TestInterruptedImportIsFailedEvenAfterPartialSuccess` / `TestImportWorkerResumesPendingAndStops` — 覆盖文件失败、执行权与幂等、中断置失败、worker 恢复与停止。
- **主要调用关系**：测试 `service.Import`/`doImport`/`importRow`/`RunImports`/`processImport` 与 `repository` 领取逻辑。
- **测试文件**：`import_test.go`（本文件）。
- **涉及表/模型**：`wms_import_task`、`wms_receipt_order`。

### 入库 inbound — `internal/modules/inbound/service/order_test.go`
- **核心符号**：`func TestBatchOperKeepsPartialSuccessAndHidesDatabaseErrors(t *testing.T)` — 批量操作保留部分成功并隐藏数据库错误。
- **主要调用关系**：测试 `service.batchOper`/`BatchDelete`。
- **测试文件**：`order_test.go`（本文件）。
- **涉及表/模型**：`wms_receipt_order`。

### 入库 inbound — `internal/modules/inbound/service/query_test.go`
- **核心符号**：`func TestGetOrderPreservesTenantAndCancellation(t *testing.T)` — 详情查询保留租户与取消状态。
- **主要调用关系**：测试 `service.Get`/`GetResponse`。
- **测试文件**：`query_test.go`（本文件）。
- **涉及表/模型**：`wms_receipt_order`、`wms_receipt_order_detail`。

### 入库 inbound — `internal/modules/inbound/service/receiving_test.go`
- **核心符号**：`TestReceiveCountsDefectiveGoodsOnce` / `TestReceiveRejectsInvalidQuantitiesBeforeTransaction` — 残品只计一次、非法数量在开事务前拒绝。
- **主要调用关系**：测试 `service.Receive`。
- **测试文件**：`receiving_test.go`（本文件）。
- **涉及表/模型**：`wms_receipt_order`、`wms_receipt_order_detail`、`wms_task`。

### 入库 inbound — `internal/modules/inbound/service/response_test.go`
- **核心符号**：`TestImportTaskResponseKeepsPublicFieldsAndHidesInternalPaths` / `TestOrderResponseKeepsPublicFieldsAndHidesInternalVersion` / `TestOrderDetailResponsesHideInternalVersions` — 响应契约不泄露内部字段。
- **主要调用关系**：测试 `orderResponse`/`orderDetailRowResponses`/`importTaskResponse`。
- **测试文件**：`response_test.go`（本文件）。
- **涉及表/模型**：无。

### 出库 outbound — `internal/modules/outbound/service/service.go`
- **核心符号**
  - `type Service struct{...}` — 出库单/分配/拣货/发货业务服务。
  - `func New(repo *repository.Repository, tm *tx.Manager, no *orderno.Generator, basic basicapi.BasicAPI, inv invapi.InventoryAPI, taskAPI taskapi.TaskAPI, limits config.LimitsConfig) *Service` — 构造。
- **主要调用关系**：被 `internal/app/app.go` 创建并注入 `outbound/handler` 与 demo 服务。
- **测试文件**：无。
- **涉及表/模型**：`wms_shipment_order`、`wms_shipment_order_detail`、`wms_allocation`。

### 出库 outbound — `internal/modules/outbound/service/order.go`
- **核心符号**
  - `func (s *Service) Create(ctx, req *dto.CreateOrderReq, operator string)` — 创建 DRAFT 出库单，`BizOrderNo` 幂等。
  - `func (s *Service) CreateResponse(...)` — 返回创建响应。
  - `func (s *Service) Delete / Submit` — 删除 DRAFT、DRAFT→SUBMITTED。
  - `func (s *Service) Approve(ctx, id, operator)` — 审核：按 SKU 排序明细，逐明细调用 `s.inv.Allocate` 做 FIFO 分配，落分配行，推进状态到 PICKING 并按分配行生成拣货任务。
  - `func (s *Service) Cancel(ctx, id, operator)` — 作废：按状态释放已分配库存并取消分配行/任务。
  - `func (s *Service) batchOper/BatchDelete/BatchSubmit/BatchApprove/BatchCancel` — 批量操作。
  - `func (s *Service) buildDetails(...)` — 构建明细并校验重复 SKU。
- **主要调用关系**：调用 `s.repo.*`、`s.inv.Allocate/Release`、`s.taskAPI.Create/CancelByOrder`、`quota.Guard`；被 handler、`integration.go CreateExternal` 调用。
- **测试文件**：`order_test.go`（batchOper）、`response_test.go`。
- **涉及表/模型**：`wms_shipment_order`、`wms_shipment_order_detail`、`wms_allocation`。

### 出库 outbound — `internal/modules/outbound/service/pick.go`
- **核心符号**
  - `type PickScan struct{ LocationCode, BatchNo string; Strict bool }` — 扫码核对输入；`Strict=true`（PDA 入口）时库位必填、任务有批次时批次必填。
  - `const pickLeaseTTL = 10 * time.Minute` — 拣货任务租约时长。
  - `const pickIdempotencyScope = "outbound.pick"` — 请求级幂等作用域。
  - `func (s *Service) Pick(ctx, taskID int64, qty int, operator string, scan *PickScan, claimToken, idempotencyKey string) (*dto.PickResult, error)` — 拣货：幂等快路径 → 锁任务行 → 领取凭证校验 → 锁分配行 → 聚合关系校验 → 扫码校验 → 推进任务 → 原子累加分配行/主单/明细 → 分配行拣满发货扣库存 → 主单拣满转 SHIPPED → 续租 → 生成提交时刻快照并写幂等记录；业务拒绝时返回非锁读当前快照 + 错误。
  - `func (s *Service) ClaimPickTask(ctx, taskID int64, operator string) (*dto.ClaimResult, error)` — 领取/续领任务租约（无租约/租约过期/本人持有可领），生成 `claim_token` 并返回租约与任务快照。
  - `func (s *Service) claimFailure(ctx, taskID) error` — 领取 0 行后定位原因（任务不存在/状态不允许 40007 / 被他人持有 40017）。
  - `func checkPickScan(scan *PickScan, t *taskmodel.Task) error` — 库位/批次核对；Strict 模式缺失返回 `PickLocationRequired`(50010)/`PickBatchRequired`(50011)。
  - `func checkPickClaim(claimToken string, t *taskmodel.Task) error` — 领取凭证与租约校验（凭证不符 40018、租约过期 40019；空凭证表示后台入口不校验）。
  - `func (s *Service) pickSnapshot(ctx, taskID) (*dto.PickResult, error)` / `replayPickResult(raw string) *dto.PickResult` — 非锁读快照 / 幂等记录回放。
- **主要调用关系**：调用 `s.repo.GetOrder/GetAllocationForUpdate/IncrAllocationPicked/IncrOrderPicked/IncrDetailPicked/ShipIfFullyPicked`、`s.taskAPI.Get/GetForUpdate/AddProgress/Claim/RenewClaim`、`s.inv.Ship`、`idempotency.Find/Insert/Fingerprint`；被 `handler.pick/pdaPick/pdaClaim` 与 demo 场景调用。
- **测试文件**：`internal/app/pick_race_test.go`（竞态 + PDA 集成用例）。
- **涉及表/模型**：`wms_shipment_order`、`wms_allocation`、`wms_task`、`wms_inventory`（经 `invapi`）、`wms_idempotency`。

### 出库 outbound — `internal/modules/outbound/service/integration.go`
- **核心符号**
  - `func (s *Service) CreateExternal(ctx, req *dto.ExternalCreateOrderReq, operator string) (*model.ShipmentOrder, bool, error)` — 外部系统按仓库/货品编码推送建单，按 `BizOrderNo` 幂等，返回是否命中已有单。
- **主要调用关系**：调用 `tenant.WithExactTenant`、`s.repo.GetOrderByBizNo`、`s.basic.GetWarehouseByCode/GetSKUByCode`、`s.Create`；被 `handler/integration.go createExternalOrder` 调用。
- **测试文件**：无。
- **涉及表/模型**：`wms_shipment_order`。

### 出库 outbound — `internal/modules/outbound/service/query.go`
- **核心符号**
  - `type OrderDetail struct{...}` — 出库单详情聚合（单据 + 明细 + 分配行 + 任务）。
  - `func (s *Service) Get / GetResponse / List / ListResponses` — 查询与响应。
  - `func orderDetailRowResponses / allocationResponses / orderTaskResponses / orderResponse` — 响应映射。
- **主要调用关系**：调用 `s.repo.GetOrder/ListDetails/ListAllocations/ListOrders`、`s.taskAPI.List`；被 handler 调用。
- **测试文件**：`response_test.go`。
- **涉及表/模型**：`wms_shipment_order`、`wms_shipment_order_detail`、`wms_allocation`、`wms_task`。

### 出库 outbound — `internal/modules/outbound/model/model.go`
- **核心符号**
  - `type OrderStatus string` + consts、`type AllocationStatus string` + consts。
  - `var StatusTransitions` / `func CanTransit(from, to OrderStatus) bool` — 状态机校验入口。
  - `type ShipmentOrder struct{...}`（含 `BizOrderNo` 幂等键）、`ShipmentOrderDetail`、`Allocation`（FIFO 分配行）。
- **主要调用关系**：被本模块 service/repository 与测试引用。
- **测试文件**：`model_test.go`。
- **涉及表/模型**：`wms_shipment_order`、`wms_shipment_order_detail`、`wms_allocation`。

### 出库 outbound — `internal/modules/outbound/repository/repository.go`
- **核心符号**
  - `type Repository struct{}` / `func New()`。
  - `func (r *Repository) CreateOrder / GetOrder / GetOrderForUpdate / GetOrderByBizNo / DeleteOrder` — 单据读写。
  - `func (r *Repository) UpdateStatus(tx, id, from, to)` — status CAS + version 递增。
  - `func (r *Repository) ListDetails / GetDetailForUpdate / UpdateDetailAllocated / IncrDetailPicked` — 明细维护。
  - `func (r *Repository) UpdateOrderProgress / IncrOrderPicked / ListOrders` — 主单进度（乐观锁）与查询。
  - `func (r *Repository) CreateAllocations / ListAllocations / GetAllocationForUpdate / IncrAllocationPicked / CancelAllocationsByOrder` — 分配行读写。
- **主要调用关系**：被 `outbound/service` 调用。
- **测试文件**：无。
- **涉及表/模型**：`wms_shipment_order`、`wms_shipment_order_detail`、`wms_allocation`。

### 出库 outbound — `internal/modules/outbound/handler/handler.go`
- **核心符号**
  - `type Handler struct{...}` / `func New(svc *service.Service) *Handler`。
  - `func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker)` — 注册 `/outbound` 路由与 `wms:outbound:*` 权限。
  - `func (h *Handler) RegisterPDARoutes(auth, checker)` — 注册 PDA 专用路由：`POST /pda/tasks/:id/claim`、`POST /pda/tasks/:id/pick`（权限 `wms:outbound:pick`）。
  - 私有方法 `list/get/create/delete/submit/approve/cancel/batch*/pick/pdaPick/pdaClaim/pickTask`；`pickTask` 为两个拣货入口共用（`strict` 区分严格/宽松），成功返回任务快照、业务拒绝以 `FailWithData` 带回快照。
- **主要调用关系**：调用 `service.Service`；被 `app.NewRouter` 挂载（含 `RegisterPDARoutes`）。
- **测试文件**：无。
- **涉及表/模型**：无（HTTP 层）。

### 出库 outbound — `internal/modules/outbound/handler/integration.go`
- **核心符号**
  - `func (h *Handler) RegisterIntegrationRoutes(pub *gin.RouterGroup, apiKey string, tenantID int64)` — 注册无 JWT、API Key 校验的 `/integration/outbound-orders`。
  - `func (h *Handler) createExternalOrder(c *gin.Context)` — 解析 `ExternalCreateOrderReq` 并调用 `svc.CreateExternal`。
- **主要调用关系**：调用 `service.CreateExternal`、`middleware.APIKey`；被 router 挂载。
- **测试文件**：无。
- **涉及表/模型**：无（HTTP 层）。

### 出库 outbound — `internal/modules/outbound/dto/dto.go`
- **核心符号**
  - `CreateOrderReq / OrderDetailItem / OrderQuery / PickReq / ExternalCreateOrderReq / ExternalOrderDetailItem / ExternalCreateOrderResp / BatchOperReq / BatchOperResp / BatchItemError` — 请求结构（`PickReq` 含可选 `claim_token`）。
  - `OrderResp / OrderDetailResp / OrderDetailRowResp / AllocationResp / OrderTaskResp` — 稳定响应契约。
  - `PickResult` — 拣货任务快照（`task_status`/`done_qty`/`remaining_qty`/`order_status`）；`ClaimResult` — 领取结果（内嵌 `PickResult` + `claim_token`/`lease_expire_at`）。
- **主要调用关系**：被 handler、service、测试引用。
- **测试文件**：无。
- **涉及表/模型**：无（传输对象）。

### 出库 outbound — `internal/modules/outbound/model/model_test.go`
- **核心符号**：`func TestCanTransit(t *testing.T)` — 校验出库单状态转换表。
- **主要调用关系**：测试 `model.CanTransit`。
- **测试文件**：`model_test.go`（本文件）。
- **涉及表/模型**：`wms_shipment_order`。

### 出库 outbound — `internal/modules/outbound/service/order_test.go`
- **核心符号**：`func TestBatchOperKeepsPartialSuccessAndHidesDatabaseErrors(t *testing.T)` — 批量操作保留部分成功并隐藏数据库错误。
- **主要调用关系**：测试 `service.batchOper`。
- **测试文件**：`order_test.go`（本文件）。
- **涉及表/模型**：`wms_shipment_order`。

### 出库 outbound — `internal/modules/outbound/service/response_test.go`
- **核心符号**：`TestOrderResponseKeepsPublicFieldsAndHidesInternalVersion` / `TestOrderDetailResponsesHideInternalVersions` — 响应契约不泄露内部 version。
- **主要调用关系**：测试 `orderResponse`/`orderDetailRowResponses`/`allocationResponses`。
- **测试文件**：`response_test.go`（本文件）。
- **涉及表/模型**：无。

### 任务 task — `internal/modules/task/service/service.go`
- **核心符号**
  - `type Service struct{...}` / `func New(repo *repository.Repository, db *gorm.DB) *Service`。
  - `func taskNoPrefix(t model.TaskType) string` — 任务号前缀 SH/SJ/PK。
  - `func (s *Service) Create(ctx, tx *gorm.DB, creates []*api.CreateTask) error` — 业务事务内批量建任务（任务号复用主键）。
  - `func (s *Service) AddProgress(...)` / `AddProgressByDetail(...)` — 锁读任务并推进完成量。
  - `func (s *Service) progress(tx, t, qty, operator) error` — 状态机推进核心（CREATED→IN_PROGRESS→COMPLETED，数量/状态校验）。
  - `func (s *Service) CancelByOrder / CountUnfinished / GetForUpdate` — 取消、未完成统计、锁读。
  - `func (s *Service) Claim / RenewClaim(ctx, tx, taskID, ...)` — 任务作业租约领取/续租（透传 repository 条件更新）。
  - `func (s *Service) List / Get / ListResponses / GetResponse` — 查询与响应。
- **主要调用关系**：被 inbound（收货/上架）、outbound（拣货）经 `task/api.TaskAPI` 接口调用；调用 `s.repo.*`。
- **测试文件**：`service_test.go`、`response_test.go`。
- **涉及表/模型**：`wms_task`。

### 任务 task — `internal/modules/task/api/api.go`
- **核心符号**
  - `type CreateTask struct{...}` — 建任务入参；`const DetailTaskPageSize = 200`。
  - `type TaskAPI interface{ Create / AddProgress / AddProgressByDetail / CountUnfinished / ListByOrderForUpdate / CancelByOrder / GetForUpdate / Claim / RenewClaim / List / Get }` — 对外契约，要求传入调用方事务。
- **主要调用关系**：由 `task.Service` 实现；被 inbound/outbound service 依赖。
- **测试文件**：无。
- **涉及表/模型**：无（接口定义）。

### 任务 task — `internal/modules/task/model/model.go`
- **核心符号**
  - `type TaskType string` + consts（RECEIVE/PUTAWAY/PICK）、`type TaskStatus string` + consts。
  - `var StatusTransitions` / `func CanTransit(from, to TaskStatus) bool` — 任务状态机。
  - `type Task struct{...}` — 统一任务表模型；含作业租约字段 `ClaimedBy`/`ClaimToken`/`LeaseExpireAt`（拣货 PDA 领取凭证，迁移 000010）。
- **主要调用关系**：被 service/repository 与 inbound/outbound 引用。
- **测试文件**：无。
- **涉及表/模型**：`wms_task`。

### 任务 task — `internal/modules/task/repository/repository.go`
- **核心符号**
  - `type Repository struct{}` / `func New()`。
  - `func (r *Repository) CreateBatch / GetForUpdate / GetByDetailForUpdate` — 创建与行锁读取。
  - `func (r *Repository) UpdateProgress(tx, t) (int64, error)` — version 乐观锁推进状态/完成量。
  - `func (r *Repository) CountUnfinished / CancelByOrder / Get / List`。
  - `func (r *Repository) Claim(tx, taskID, operator, token, expireAt) (int64, error)` — 条件领取/续领：`task_type=PICK` 且 `status IN (CREATED,IN_PROGRESS)` 且（无租约 / 租约过期 / 本人持有）。
  - `func (r *Repository) RenewClaim(tx, taskID, token, expireAt)` — 凭证一致时延长租约（调用方持行锁）。
- **主要调用关系**：被 `task/service` 调用。
- **测试文件**：无。
- **涉及表/模型**：`wms_task`。

### 任务 task — `internal/modules/task/handler/handler.go`
- **核心符号**
  - `type Handler struct{...}` / `func New(svc *service.Service) *Handler`。
  - `func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker)` — 注册 `/tasks`、`/tasks/:id` 只读路由（`wms:task`）。
  - 私有方法 `list/get`。
- **主要调用关系**：调用 `service.Service`；业务动作在各业务模块。
- **测试文件**：无。
- **涉及表/模型**：无（HTTP 层）。

### 任务 task — `internal/modules/task/dto/dto.go`
- **核心符号**
  - `type TaskResp struct{...}` — 任务查询稳定响应契约，不直接暴露 GORM Model。
- **主要调用关系**：被 `task/service` 与 handler 引用。
- **测试文件**：无。
- **涉及表/模型**：无（传输对象）。

### 任务 task — `internal/modules/task/service/service_test.go`
- **核心符号**：`TestTaskCreationSharesCallerTransaction` / `TestTaskConcurrentProgressDoesNotOverComplete` / `TestCreateRejectsInvalidTasks` / `TestCancelByOrderInvalidatesStaleProgress` — 建任务共用调用方事务、并发不超报、非法入参拒绝、取消后旧进度失效。
- **主要调用关系**：测试 `service.Create`/`AddProgress`/`progress`/`CancelByOrder`。
- **测试文件**：`service_test.go`（本文件）。
- **涉及表/模型**：`wms_task`。

### 任务 task — `internal/modules/task/service/response_test.go`
- **核心符号**：`func TestTaskResponseKeepsPublicFieldsAndHidesInternalVersion(t *testing.T)` — 任务响应不泄露内部 version。
- **主要调用关系**：测试 `taskResponse`/`taskResponses`。
- **测试文件**：`response_test.go`（本文件）。
- **涉及表/模型**：无。

### 库存 inventory — `internal/modules/inventory/service/service.go`
- **核心符号**
  - `type Service struct{...}` — 库存读写/分配/调整/释放/流水服务。
  - `func New(repo *repository.Repository, tm *pkgtx.Manager) *Service` — 构造。
- **主要调用关系**：被 `internal/app/app.go` 创建，作为 `invapi.InventoryAPI` 注入 inbound/outbound/stocktake/basic。
- **测试文件**：无。
- **涉及表/模型**：`wms_inventory`、`wms_inventory_trans`。

### 库存 inventory — `internal/modules/inventory/service/stock.go`
- **核心符号**
  - `func (s *Service) Increase(ctx, tx, req *api.IncreaseReq) error` — 上架入库：锁基础资料与四元组库存行，存在则累加，不存在则创建（唯一索引兜底并发）。
  - `func (s *Service) Allocate(ctx, tx, req *api.AllocateReq) (*api.AllocateResult, error)` — FIFO 分配：非锁定取候选（每批 `allocateBatchSize=20`，keyset 游标翻页，**不设批数上限**）→ 按剩余需求估最小前缀 → `LockInventoryByIDs` 锁读 → 锁内按 FIFO 重排并重算 → 逐行 `AllocateQty`（available↓ allocated↑）→ 写 ALLOCATE 流水；锁到的行比候选快照少（快照过期）只标记不失败，候选读完仍不足时：有快照变化返回 `Conflict` 交外层 `TxRetry` 换新快照，无变化才报 `AvailableNotEnough`。
  - `func (s *Service) Ship(ctx, tx, req *api.ShipReq) error` — 发货扣减：stock↓ allocated↓（`ShipQty` 双条件）。
  - `func (s *Service) Release(ctx, tx, req *api.ReleaseReq) error` — 取消分配：allocated↓ available↑。
  - `func (s *Service) Adjust(ctx, tx, req *api.AdjustReq) (int, error)` — 盘点调整：行锁内把账面数调整为 NewStock，调减不可吃掉已分配。
  - `func availableNotEnoughMsg(skuID int64, need, actual int) string` — 可用不足错误文案。
- **主要调用关系**：被 `invapi.InventoryAPI` 三处业务调用（inbound `putaway.Putaway`、outbound `Approve/Cancel/Pick`、stocktake `Approve`）；调用 `s.repo` 各方法。
- **测试文件**：`service_test.go`（含 `TestAllocateFIFO`、`TestAllocatePagesBeyondBatchWindow` 等）、`concurrency_test.go`。
- **涉及表/模型**：`wms_inventory`、`wms_inventory_trans`、`wms_location`（联查）。

### 库存 inventory — `internal/modules/inventory/service/query.go`
- **核心符号**
  - `func (s *Service) List / SummaryBySKU / ListTrans` — 库存、按 SKU 汇总、流水查询及响应转换。
  - `func (s *Service) HasStockByWarehouse / HasStockByLocation / HasStockBySKU` — 跨模块库存存在性检查（删除校验）。
  - `func inventoryResponses / inventoryTransResponses / inventorySummaryResponses` — 响应映射。
- **主要调用关系**：被 `inventory/handler` 和 basic 模块调用；调用 `s.repo.List/SummaryBySKU/ListTrans/HasStock*`。
- **测试文件**：`response_test.go`。
- **涉及表/模型**：`wms_inventory`、`wms_inventory_trans`、`wms_sku`（联查）。

### 库存 inventory — `internal/modules/inventory/repository/repository.go`
- **核心符号**
  - `type SummaryRow struct{...}`、`type QueryFilter struct{...}`、`type Repository struct{}` / `func New()`。
  - `func (r *Repository) LockBasicReferences(tx, warehouseID, locationID, skuID)` — 按固定顺序锁仓库/库位/SKU。
  - `func (r *Repository) ListFIFOCandidates(tx, warehouseID, skuID, limit, afterStockInTime, afterID)` — **非锁定**候选集（`ORDER BY stock_in_time ASC, id ASC`，用 `(stock_in_time, id)` 翻页）；`func (r *Repository) LockInventoryByIDs(tx, ids)` — 只锁指定主键（按主键升序加锁，不写 ORDER BY）；`ListLocationCodes` 单独补库位编码；`SumAvailableQty` 供分配失败时生成提示。
  - `func (r *Repository) GetForUpdate / GetByTupleForUpdate / Create / IncreaseQty` — 库存行读写。
  - `func (r *Repository) AllocateQty / ShipQty / ReleaseQty / AdjustNegative / AdjustPositive` — 条件更新（防超卖/防负）。
  - `func (r *Repository) InsertTrans` — 同事务写流水。
  - `func (r *Repository) List / SummaryBySKU / ListTrans / HasStockByWarehouse / HasStockByLocation / HasStockBySKU` — 查询。
- **主要调用关系**：被 `inventory/service` 调用。
- **测试文件**：`repository_test.go`。
- **涉及表/模型**：`wms_inventory`、`wms_inventory_trans`、`wms_sku`、`wms_location`。

### 库存 inventory — `internal/modules/inventory/model/model.go`
- **核心符号**
  - `type Inventory struct{...}` — 仓库+库位+SKU+批次四元组唯一，三数量模型（stock = available + allocated），`StockInTime` 为 FIFO 依据。
  - `type TransType string` + consts（RECEIVE/ALLOCATE/SHIP/RELEASE/ADJUST）。
  - `type InventoryTrans struct{...}` — 只增不改的库存流水。
- **主要调用关系**：被 service/repository/dto 引用。
- **测试文件**：无。
- **涉及表/模型**：`wms_inventory`、`wms_inventory_trans`。

### 库存 inventory — `internal/modules/inventory/api/api.go`
- **核心符号**
  - `IncreaseReq / AllocateReq / AllocateRow / AllocateResult / ShipReq / ReleaseReq / AdjustReq` — 变更入参/结果。
  - `type InventoryAPI interface{ Increase / Allocate / Ship / Release / Adjust }` — 对外契约，要求调用方传入事务。
- **主要调用关系**：由 `inventory/service.Service` 实现；被 inbound/outbound/stocktake/basic 依赖。
- **测试文件**：无。
- **涉及表/模型**：无（接口定义）。

### 库存 inventory — `internal/modules/inventory/handler/handler.go`
- **核心符号**
  - `type Handler struct{...}` / `func New(svc *service.Service) *Handler`。
  - `func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker)` — 注册 `/inventory`、`/inventory/summary`、`/inventory/trans` 只读路由（`wms:inventory`）。
  - 私有方法 `list/summary/listTrans`。
- **主要调用关系**：调用 `service.Service`；变更操作由其他模块经 API 完成。
- **测试文件**：无。
- **涉及表/模型**：无（HTTP 层）。

### 库存 inventory — `internal/modules/inventory/dto/dto.go`
- **核心符号**
  - `InventoryQuery / SummaryQuery / TransQuery` — 查询请求。
  - `InventoryResp / InventorySummaryResp / InventoryTransResp` — 稳定响应契约。
- **主要调用关系**：被 handler、service 引用。
- **测试文件**：无。
- **涉及表/模型**：无（传输对象）。

### 库存 inventory — `internal/modules/inventory/repository/repository_test.go`
- **核心符号**：`TestInventoryMutationsCannotCrossTenantOrTouchDeletedRows` / `TestSummaryCountsSKUsAndFIFOFiltersTenant` / `TestRawInventoryQueriesRespectExactTenantZero` / `TestListSKUKeywordFiltersTenant` — 租户隔离、汇总/FIFO 过滤、exact tenant=0 语义。
- **主要调用关系**：测试 `repository` 的 `SummaryBySKU`/`ListFIFOCandidates`/`List`/变更方法。
- **测试文件**：`repository_test.go`（本文件）。
- **涉及表/模型**：`wms_inventory`、`wms_inventory_trans`、`wms_sku`。

### 库存 inventory — `internal/modules/inventory/service/service_test.go`
- **核心符号**：`TestConcurrentAllocateAntiOversell` / `TestAllocateFIFO` / `TestShipReleaseInvariant` / `TestConcurrentIncreaseTransFlow` — 并发分配防超卖、FIFO 顺序、三数量不变量、并发上架流水。
- **主要调用关系**：测试 `service.Allocate`/`Increase`/`Ship`/`Release`。
- **测试文件**：`service_test.go`（本文件）。
- **涉及表/模型**：`wms_inventory`、`wms_inventory_trans`。

### 库存 inventory — `internal/modules/inventory/service/concurrency_test.go`
- **核心符号**：`TestConcurrentReleaseDoesNotDoubleRelease` / `TestConcurrentShipDoesNotDoubleDeduct` — 并发释放/发货不重复扣减。
- **主要调用关系**：测试 `service.Release`/`Ship`。
- **测试文件**：`concurrency_test.go`（本文件）。
- **涉及表/模型**：`wms_inventory`、`wms_inventory_trans`。

### 库存 inventory — `internal/modules/inventory/service/response_test.go`
- **核心符号**：`TestInventoryResponsesKeepQuantityContractAndHideVersion` / `TestInventoryTransResponseKeepsTraceFields` / `TestInventorySummaryResponseKeepsQuantityContract` — 数量契约与流水可追溯字段。
- **主要调用关系**：测试 `inventoryResponses`/`inventoryTransResponses`/`inventorySummaryResponses`。
- **测试文件**：`response_test.go`（本文件）。
- **涉及表/模型**：无。

### 盘点 stocktake — `internal/modules/stocktake/service/service.go`
- **核心符号**
  - `type Service struct{...}` — 盘点快照/实盘/审核服务。
  - `func New(repo *repository.Repository, tm *tx.Manager, no *orderno.Generator, inv api.InventoryAPI, limits config.LimitsConfig) *Service` — 构造。
- **主要调用关系**：被 `internal/app/app.go` 创建并注入 handler 与 demo 服务。
- **测试文件**：无。
- **涉及表/模型**：`wms_stocktake_order`、`wms_stocktake_detail`。

### 盘点 stocktake — `internal/modules/stocktake/service/order.go`
- **核心符号**
  - `func (s *Service) Create(ctx, req *dto.CreateOrderReq, operator string) (*model.StocktakeOrder, error)` — 配额校验 + 快照库存生成 DRAFT 盘点单。
  - `func (s *Service) CreateResponse(...)` — 返回创建响应。
  - `func (s *Service) Cancel(ctx, orderID int64) error` — 按状态机取消盘点单。
- **主要调用关系**：调用 `s.repo.SnapshotInventory/CreateOrder/GetOrderForUpdate/UpdateStatus`、`quota.Guard`；被 handler 调用。
- **测试文件**：`service_test.go`。
- **涉及表/模型**：`wms_stocktake_order`、`wms_stocktake_detail`、`wms_inventory`（快照源）。

### 盘点 stocktake — `internal/modules/stocktake/service/counting.go`
- **核心符号**
  - `func (s *Service) RecordActual(ctx, orderID, detailID int64, actualQty int) error` — 录入实盘数量（仅 DRAFT，校验明细归属）。
- **主要调用关系**：调用 `s.repo.GetOrderForUpdate/GetDetail/UpdateDetailActual`；被 `handler.recordActual` 调用。
- **测试文件**：无（被 `service_test.go` 间接触及）。
- **涉及表/模型**：`wms_stocktake_order`、`wms_stocktake_detail`。

### 盘点 stocktake — `internal/modules/stocktake/service/approve.go`
- **核心符号**
  - `func (s *Service) Approve(ctx, orderID int64, operator string) error` — 审核：按 `InventoryID` 排序明细，逐条调用 `s.inv.Adjust` 应用差异并 `MarkAdjusted`，无任何实盘则拒绝，最后置 COMPLETED。
- **主要调用关系**：调用 `s.repo.GetOrderForUpdate/ListDetails/MarkAdjusted/UpdateStatus`、`s.inv.Adjust`；被 `handler.approve` 调用。
- **测试文件**：`service_test.go`（含差异用锁后库存计算用例）。
- **涉及表/模型**：`wms_stocktake_order`、`wms_stocktake_detail`、`wms_inventory`（经 `invapi`）。

### 盘点 stocktake — `internal/modules/stocktake/service/query.go`
- **核心符号**
  - `type OrderDetail struct{...}` — 盘点单详情聚合。
  - `func (s *Service) Get / GetResponse / List / ListResponses` — 查询与响应。
  - `func orderResponse / detailResponses / detailResponse` — 响应映射。
- **主要调用关系**：调用 `s.repo.GetOrder/ListDetails/ListOrders`；被 handler 调用。
- **测试文件**：`response_test.go`。
- **涉及表/模型**：`wms_stocktake_order`、`wms_stocktake_detail`。

### 盘点 stocktake — `internal/modules/stocktake/model/model.go`
- **核心符号**
  - `type OrderStatus string` + consts（DRAFT/COMPLETED/CANCELLED）、`var StatusTransitions`、`func CanTransit(from, to OrderStatus) bool`。
  - `type StocktakeOrder struct{...}`、`type StocktakeDetail struct{...}`（`BookQty` 快照、`ActualQty *int`、`DiffQty`、`Adjusted`）。
- **主要调用关系**：被 service/repository 与测试引用。
- **测试文件**：`model_test.go`。
- **涉及表/模型**：`wms_stocktake_order`、`wms_stocktake_detail`。

### 盘点 stocktake — `internal/modules/stocktake/repository/repository.go`
- **核心符号**
  - `type Repository struct{}` / `func New()`。
  - `func (r *Repository) CreateOrder / GetOrderForUpdate / GetOrder / UpdateStatus / ListOrders` — 单据读写与 CAS 状态推进。
  - `func (r *Repository) ListDetails / GetDetail / UpdateDetailActual / MarkAdjusted` — 明细读写。
  - `func (r *Repository) SnapshotInventory(tx, warehouseID, locationID)` — 按仓库/库位范围取账面库存生成快照（显式租户过滤）。
- **主要调用关系**：被 `stocktake/service` 调用。
- **测试文件**：无。
- **涉及表/模型**：`wms_stocktake_order`、`wms_stocktake_detail`、`wms_inventory`、`wms_sku`、`wms_location`。

### 盘点 stocktake — `internal/modules/stocktake/handler/handler.go`
- **核心符号**
  - `type Handler struct{...}` / `func New(svc *service.Service) *Handler`。
  - `func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker)` — 注册 `/stocktake` 路由与 `wms:stocktake:*` 权限。
  - 私有方法 `list/get/create/recordActual/approve/cancel`。
- **主要调用关系**：调用 `service.Service`；被 router 挂载。
- **测试文件**：无。
- **涉及表/模型**：无（HTTP 层）。

### 盘点 stocktake — `internal/modules/stocktake/dto/dto.go`
- **核心符号**
  - `CreateOrderReq / OrderQuery / RecordActualReq` — 请求结构。
  - `OrderResp / DetailResp / OrderDetailResp` — 稳定响应契约。
- **主要调用关系**：被 handler、service 引用。
- **测试文件**：无。
- **涉及表/模型**：无（传输对象）。

### 盘点 stocktake — `internal/modules/stocktake/model/model_test.go`
- **核心符号**：`func TestStocktakeStatusTransitions(t *testing.T)` — 校验盘点单状态转换表。
- **主要调用关系**：测试 `model.CanTransit`。
- **测试文件**：`model_test.go`（本文件）。
- **涉及表/模型**：`wms_stocktake_order`。

### 盘点 stocktake — `internal/modules/stocktake/service/service_test.go`
- **核心符号**：`TestStocktakeSnapshotTenantIsolation` / `TestStocktakeRollbackAndMissingInventory` / `TestStocktakeDifferenceUsesLockedCurrentStock` — 快照租户隔离、回滚与缺失库存、差异基于锁后库存。
- **主要调用关系**：测试 `service.Create`/`RecordActual`/`Approve`。
- **测试文件**：`service_test.go`（本文件）。
- **涉及表/模型**：`wms_stocktake_order`、`wms_stocktake_detail`、`wms_inventory`。

### 盘点 stocktake — `internal/modules/stocktake/service/response_test.go`
- **核心符号**：`func TestStocktakeResponsesKeepPublicFieldsAndHideVersion(t *testing.T)` — 盘点响应不泄露内部 version。
- **主要调用关系**：测试 `orderResponse`/`detailResponse`。
- **测试文件**：`response_test.go`（本文件）。
- **涉及表/模型**：无。

---

### 本部分重点专题

### 1. 库存一致性与并发控制
- **行锁（FOR UPDATE）**：`inventory/repository/repository.go` `LockInventoryByIDs`、`GetForUpdate`、`GetByTupleForUpdate`、`LockBasicReferences`；`task/repository/repository.go` `GetForUpdate`、`GetByDetailForUpdate`；`inbound/repository/repository.go` `GetOrderForUpdate`、`GetDetailForUpdate`；`outbound/repository/repository.go` `GetOrderForUpdate`、`GetAllocationForUpdate`；`stocktake/repository/repository.go` `GetOrderForUpdate`。
- **数量条件更新（第二层防超卖）**：`inventory/repository/repository.go` `AllocateQty`（`WHERE available_quantity >= ?`）、`ShipQty`、`ReleaseQty`、`AdjustNegative`。
- **乐观锁 version**：字段定义 `pkg/modelbase/model.go` `Versioned`；写入点 `task/repository` `UpdateProgress`、`outbound/repository` `UpdateOrderProgress`/`IncrOrderPicked`/`IncrAllocationPicked`、`inbound/repository` `IncrOrderReceive`、以及三者 `UpdateStatus`（status CAS + version+1）。
- **事务边界与重试**：`pkg/tx` 的 `Manager.Tx`/`TxRetry` 与 `MaxTxRetry`；需要重试的入口为 `inbound/service/order.go` `transit`/`Update`/`createOrder`（`MaxOrderNoRetry`）、`inbound/service/receiving.go` `Receive`、`inbound/service/putaway.go` `Putaway`、`outbound/service/order.go` `Create`/`Approve`/`Cancel`、`outbound/service/pick.go` `Pick`、`stocktake/service/approve.go` `Approve`。`CreateExternal` 在建单前用 `tenant.WithExactTenant` 限定单租户。

### 2. 出库 FIFO 分配算法
- **选批次函数**：`inventory/repository/repository.go` `ListFIFOCandidates` — `WHERE warehouse_id=? AND sku_id=? AND available_quantity > 0`，**不加锁**，`ORDER BY i.stock_in_time ASC, i.id ASC`（`StockInTime` 为 FIFO 依据）；加锁另由 `LockInventoryByIDs` 完成，锁集合收敛到按需求估算出的候选前缀。
- **执行分配**：`inventory/service/stock.go` `Allocate` — 分批取 FIFO 候选（批大小 20，keyset 翻页到底，批数不设上限）→ 锁最小前缀 → 锁内按 FIFO 重排，逐行 `take = min(remaining, AvailableQty)` 并调用 `AllocateQty`（`available_quantity -= take` / `allocated_quantity += take`，stock 不变），写 `TransAllocate` 流水，返回 `AllocateResult.Rows`；锁到的行比候选快照少说明快照过期，继续翻后面的候选，候选读完仍不足时：观察到快照变化返回 `Conflict`（外层换新快照重试），无变化才返回 `AvailableNotEnough`。
- **调用方**：`outbound/service/order.go` `Approve` — 先按 `SKUID` 排序明细加锁，再逐明细调用 `s.inv.Allocate`，随后 `UpdateDetailAllocated` 并生成 `Allocation` 行（按 `LocationCode` 排序仅优化拣货路径）；分配量不足整体回滚。

### 3. 拣货（任务状态机 / 扫码校验 / 领取租约 / 扣减时机）
- **任务状态机**：`task/model/model.go` `TaskStatus`、`StatusTransitions`、`CanTransit`；推进逻辑 `task/service/service.go` `AddProgress`/`AddProgressByDetail` → `progress`（`CREATED→IN_PROGRESS→COMPLETED`，超量返回 `TaskQtyOver`，version 冲突返回 `Conflict`）。
- **扫码校验**：`outbound/service/pick.go` `checkPickScan` — 对 `PickScan.LocationCode`/`BatchNo` 与任务 `LocationCode`/`BatchNo` 做忽略大小写比对，不一致返回 `PickLocationMismatch`/`PickBatchMismatch`；PDA 入口 `Strict=true` 时库位必填（50010）、任务有批次时批次必填（50011）。
- **请求幂等**：`pkg/idempotency`（`Find`/`Insert`/`Fingerprint`，表 `wms_idempotency`，唯一键 `(tenant_id, scope, idempotency_key)`）；`outbound/service/pick.go` `Pick` 在事务首部走幂等快路径，命中回放 `ResultJSON` 首次快照并短路在凭证校验之前；记录与业务同事务提交。
- **领取凭证与租约**：`outbound/service/pick.go` `ClaimPickTask`/`checkPickClaim`（`pickLeaseTTL=10m`，成功拣货续租）；`task/repository` `Claim`/`RenewClaim`；字段 `wms_task.claimed_by`/`claim_token`/`lease_expire_at`（迁移 000010）。
- **拣货主流程**：`outbound/service/pick.go` `Pick` — 锁任务行→领取凭证校验→锁分配行→聚合关系校验（任务 ↔ 分配行 `order_id`/`sku_id`/`allocated_qty`，不一致 40016）→扫码校验→`AddProgress`→`IncrAllocationPicked`/`IncrOrderPicked`/`IncrDetailPicked`→返回任务快照。
- **扣减库存时机**：分配行**拣满**（`allocFullyPicked`，`a.PickedQty+qty == a.AllocatedQty`）时调用 `s.inv.Ship` 实扣库存；主单拣满由 `ShipIfFullyPicked` 的 SQL 条件（`picked_qty = allocated_qty`）推进 `SHIPPED`。任务生成在审核阶段（`outbound/service/order.go` `Approve` 按分配行建 `taskmodel.TaskPick`）。
- **并发验证**：`internal/app/pick_race_test.go` 覆盖"取消先到/拣货先到/同时到"三条竞态（双方都先锁任务行，无交叉加锁顺序）。

### 4. Excel 异步导入（状态机 / 领取 / run token）
- **状态机取值**：`inbound/model/model.go` `ImportTaskStatus` = `PENDING`/`PROCESSING`/`COMPLETED`/`FAILED`；结果→状态由 `inbound/service/import_parse.go` `importResult.status()` 决定。
- **落库与领取**：HTTP `inbound/service/import.go` `Import` 仅写文件 + 落库 PENDING；worker `inbound/service/import_worker.go` `RunImports` 循环 `processImport`，用 `inbound/repository/import.go` `NextPendingImport` 扫描、`ClaimImport`（`WHERE status=PENDING` 条件更新为 PROCESSING）竞争领取，多实例安全。
- **run token**：字段 `model.ImportTask.RunToken`；`import_worker.go processImport` 生成 `uuid.NewString()`；`inbound/repository/import.go` `ownedImport` 以 `id+tenant_id+status+run_token` 限定，`LockImportExecution`/`FinishImport`/`TouchImport`/`ReleaseImport`/`ResetStaleImport` 均带 token 校验，防止失去执行权的 worker 写入。
- **心跳与补偿**：`import_worker.go` `heartbeatImport`（`TouchImport` 续约，失去执行权即取消）；`RunCompensator`/`compensateOnce` 用 `ListStaleImports` + `ResetStaleImport` 把超时 PROCESSING 归还 PENDING。
- **行幂等**：唯一键 `uk_import_row`（`ImportTaskID`+`ImportRow`）+ `repository.GetByImportRow` + `createImportOrder` 兜底。

### 5. 多租户在本范围的体现
- **全局注入**：`pkg/tenant/gorm.go` `RegisterGORMCallbacks` — Create 前填充 `tenant_id`、Query/Row/Update/Delete 前注入 `WHERE tenant_id = ?`（`clause.CurrentTable` 限定主表）；ctx 传播 `pkg/tenant/tenant.go` `WithTenant`/`FromContext`/`Scope`/`WithExactTenant`。
- **需要手工过滤的点（Table 别名联查无 Schema）**：`inventory/repository/repository.go` `ListFIFOCandidates`、`ListLocationCodes`、`SumAvailableQty`、`SummaryBySKU`、`List`（SKU 关键字）用 `tenant.Scope`；`stocktake/repository/repository.go` `SnapshotInventory` 用 `tenant.Scope`。
- **执行上下文注入**：`inbound/service/import_worker.go` `processImport` 用 `tenant.WithTenant(parent, task.TenantID)` 为后台 worker 恢复租户上下文；`outbound/service/integration.go` `CreateExternal` 用 `tenant.WithExactTenant` 禁止 API Key 走平台旁路。
- **模型层**：各 `model.go` 的 `TenantID` 均前置进联合唯一索引（如 `uk_receipt_no`、`uk_shipment_no`、`uk_task_no`、`uk_inv`、`uk_stocktake_no`）。

### 6. 上架与收货如何联动生成任务、更新单据状态
- **收货侧**：`inbound/service/receiving.go` `Receive` — 明细/主单原子累加（`IncrDetailReceive`/`IncrOrderReceive`），按明细推进收货任务（`taskAPI.AddProgressByDetail`，收齐自动完成）；重读全部明细判断收齐后，为每条明细按 `上架量 = ReceivedQty - DefectiveQty` 生成 `taskmodel.TaskPutaway`（`taskAPI.Create`），有上架量则单据转 `OrderPutaway`，全为残品则直接 `OrderCompleted`。
- **上架侧**：`inbound/service/putaway.go` `Putaway` — 校验任务类型/库位属于单据仓库，行锁校验单据为 `OrderPutaway` 且任务未被取消；调用 `invapi.Increase` 使库存生效、`basic.UpdateLocationStatusInTx` 标记库位占用、`taskAPI.AddProgress` 推进上架任务；`taskAPI.CountUnfinished` 归零后经状态机把单据置 `OrderCompleted`。
- **审核侧入口**：`inbound/service/order.go` `Approve` 在 SUBMITTED→APPROVED 时为每条明细生成 `taskmodel.TaskReceive` 收货任务，作为上述联动链路的起点。

## 二、后端平台与基础设施

覆盖目录：`internal/modules/system`、`basic`、`ai`、`demo`、`internal/bootstrap`、`internal/app`、`cmd`、`internal/pkg`、`internal/testutil`、`migrations`（共 153 个 .go 文件 + 20 个 .sql 迁移文件）。

说明：职责来自源码文档注释；无注释处依据实现摘要，均不臆造；无法确认处标注"待确认"。符号名逐字来自源码。

---

### system — `internal/modules/system/service/service.go`
- **核心符号**
  - `type Service struct{...}` — 认证、用户角色权限和操作日志业务的根结构；持有 `repo`、`jwtSecret`/`jwtExpire`、进程内 `permCache`、`loginAttempts` 与异步日志通道 `logCh`。
  - `type loginAttempt struct` / `type permCacheItem struct` — 登录限流计数项、权限缓存项（含 `expire`）。
  - `const permCacheTTL = 60 * time.Second`、`maxLoginAttempts = 5`、`loginFailureWindow = 15 * time.Minute`、`maxTrackedLogins = 10000`、`loginSweepInterval = time.Minute` — 权限缓存与登录限流参数。
  - `func New(repo *repository.Repository, jwtSecret string, expireHours int) *Service` — 构造（`logCh` 缓冲 1024）。
- **主要调用关系**：被 `handler.New`、`app.New` 组装；实现 `api.SystemAPI`（供中间件注入）。
- **测试文件**：同包 `response_test.go`、`builtin_test.go`、`permission_test.go`、`login_limiter_test.go`、`auth_test.go`、`user_test.go`、`audit_test.go`。
- **涉及表/模型**：无直接表（经 repository）。

### system — `internal/modules/system/service/auth.go`
- **核心符号**
  - `Login(ctx, req *dto.LoginReq, clientIP string) (*dto.LoginResp, error)` — 登录：限流预占、慢查询与密码校验、签发 JWT。
  - `ValidateToken(ctx, userID int64, tokenVersion int) error` — 复核用户状态与 Token 版本。
  - `Profile(ctx, userID int64) (*dto.ProfileResp, error)` — 个人档案（含角色与权限）。
  - `ChangePassword(ctx, userID int64, req *dto.ChangePwdReq) error` — 改密并失效旧 Token。
- **主要调用关系**：调用 `s.repo.GetLoginUser/GetUserByID/GetPermsByUser`、`jwt.Generate`、`s.hashPassword/checkPassword`；被 `handler.login/profile/changePassword` 调用。
- **测试文件**：`auth_test.go`。
- **涉及表/模型**：`sys_user`、`sys_user_role`、`sys_role`。

### system — `internal/modules/system/service/permission.go`
- **核心符号**
  - `HasPerm(ctx, userID int64, perm string) bool` — 权限判断入口（带缓存）。
  - `cachedPerms/loadPerms/loadRolesAndPerms` — 缓存读取与回源加载（角色名 + 权限）。
  - `expandPerms(raw []string) []string` — 展开通配权限。
  - `invalidatePermCache()` — 角色/权限变更后失效缓存。
- **主要调用关系**：被 `middleware.Permission`（经 `SystemAPI`）调用；回源走 `repo.GetPermsByUser/ListRoleNamesByUser`。
- **测试文件**：`permission_test.go`。
- **涉及表/模型**：`sys_role`（`perms` 列）、`sys_user_role`。

### system — `internal/modules/system/service/user.go`
- **核心符号**
  - `CreateUser / UpdateUser / DeleteUser / ResetPassword` — 用户增删改与密码重置。
  - `isBuiltinAdmin(ctx, id int64) (bool, error)` — 内置管理员保护判断。
  - `ListUsers(ctx, q *dto.UserListQuery) ([]*dto.UserResp, int64, error)`、`userResponse(user *model.SysUser) *dto.UserResp` — 列表与响应映射。
- **主要调用关系**：调用 `s.repo.CreateUser/UpdateUser/DeleteUser/UpdatePassword/ListUsers`、`s.invalidatePermCache`；被 `handler.createUser/updateUser/deleteUser/resetPassword/listUsers` 调用。
- **测试文件**：`user_test.go`、`builtin_test.go`。
- **涉及表/模型**：`sys_user`、`sys_user_role`。

### system — `internal/modules/system/service/role.go`
- **核心符号**
  - `CreateRole / UpdateRole / DeleteRole` — 角色维护（含内置角色保护、引用校验）。
  - `isBuiltinRole(ctx, id int64) (bool, error)` — 内置角色判断。
  - `ListRoles / ListAllRoles`、`roleResponses / roleResponse` — 列表与响应映射。
- **主要调用关系**：调用 `s.repo.CreateRole/UpdateRole/DeleteRole/ListRoles/ListAllRoles`、`s.invalidatePermCache`；被 `handler.*Role` 调用。
- **测试文件**：`response_test.go`（同包）。
- **涉及表/模型**：`sys_role`、`sys_user_role`。

### system — `internal/modules/system/service/audit.go`
- **核心符号**
  - `Record(ctx, r middleware.OperLogRecord)` — 操作日志入队（非阻塞）。
  - `RunOperLogs(ctx)` — 单一消费者循环，`ctx` 取消时尽力写完队列。
  - `flushOperLogs(ctx, batch)` — 批量落库。
  - `ListOperLogs / ListOperLogResponses` — 查询与稳定响应。
- **主要调用关系**：`Record` 被 `middleware.OperLog` 调用；`RunOperLogs` 由 `cmd/wms/main.go` 启动；落库走 `repo.InsertOperLogs/ListOperLogs`。
- **测试文件**：`audit_test.go`。
- **涉及表/模型**：`sys_oper_log`。

### system — `internal/modules/system/service/login_limiter.go`
- **核心符号**
  - `beginLoginAttempt(key string) bool` — 在慢查询与密码校验前原子预占次数，避免并发同时通过。
  - `clearLoginAttempts(key string)` — 登录成功后清空计数（key = 用户名 + 客户端 IP）。
- **主要调用关系**：被 `auth.go` 的 `Login` 调用。
- **测试文件**：`login_limiter_test.go`。
- **涉及表/模型**：无（进程内 map）。

### system — `internal/modules/system/service/password.go`
- **核心符号**
  - `hashPassword(plain string) (string, error)` — bcrypt 生成哈希。
  - `checkPassword(hash, plain string) bool` — bcrypt 校验。
- **主要调用关系**：被 `auth.go`、`user.go` 调用。
- **测试文件**：无（由 `auth_test.go` 等间接覆盖）。
- **涉及表/模型**：无。

### system — `internal/modules/system/model/model.go`
- **核心符号**
  - `type Base = modelbase.Base`、`type Versioned = modelbase.Versioned` — 兼容别名；新模型直接用 `modelbase`。
  - `type SysUser struct`（`TableName() = "sys_user"`）— 用户（tenant_id 联合唯一用户名）。
  - `type SysRole struct`（`sys_role`）— 角色（tenant_id 联合唯一角色名）。
  - `type SysUserRole struct`（`sys_user_role`）— 用户角色关联。
  - `type SysOperLog struct`（`sys_oper_log`）— 脱敏后的写操作审计记录。
- **主要调用关系**：被 system 各 repository/service 使用。
- **测试文件**：`model_test.go`。
- **涉及表/模型**：`sys_user`、`sys_role`、`sys_user_role`、`sys_oper_log`。

### system — `internal/modules/system/repository/repository.go`
- **核心符号**
  - `var ( ErrInvalidRole, ErrRoleInUse, ErrAmbiguousUser ... )` — 角色/用户领域错误（`ErrAmbiguousUser`：同名账号不能任意挑选）。
  - `type Repository struct{...}`、`func New(db *gorm.DB) *Repository` — 持久化入口。
- **主要调用关系**：被 `service.New` 持有；由 `app.New` 构造。
- **测试文件**：无（各 repository 文件由 service 测试覆盖）。
- **涉及表/模型**：待确认（通用入口）。

### system — `internal/modules/system/repository/user.go`
- **核心符号**
  - `GetUserByUsername / GetLoginUser(ctx, username string, tenantID *int64) / GetUserByID` — 用户查询（`GetLoginUser` 兼容旧登录但不从同名账号任意挑选）。
  - `CreateUser / UpdateUser / DeleteUser` — 用户与其角色关联的原子维护（Repository 自行开短事务，聚合例外）。
  - `UpdatePassword(ctx, id int64, hash string)`、`ListUsers(...)`、`attachUserRoleIDs(...)`。
- **主要调用关系**：被 `service/auth.go`、`service/user.go` 调用。
- **测试文件**：无。
- **涉及表/模型**：`sys_user`、`sys_user_role`。

### system — `internal/modules/system/repository/role.go`
- **核心符号**
  - `GetRoleByName / GetRoleByID / CreateRole / UpdateRole / DeleteRole / ListRoles / ListAllRoles`。
  - `replaceRoles(tx *gorm.DB, userID, tenantID int64, roleIDs []int64) error` — 事务内替换用户角色关联。
- **主要调用关系**：被 `service/role.go`、`repository/user.go` 调用。
- **测试文件**：无。
- **涉及表/模型**：`sys_role`、`sys_user_role`。

### system — `internal/modules/system/repository/permission.go`
- **核心符号**
  - `ListRoleNamesByUser(ctx, userID int64) ([]string, error)` — 用户角色名。
  - `GetPermsByUser(ctx, userID int64) ([]string, error)` — 汇总用户所有角色的权限标识。
- **主要调用关系**：被 `service/permission.go`、`service/auth.go` 调用。
- **测试文件**：无。
- **涉及表/模型**：`sys_role`、`sys_user_role`。

### system — `internal/modules/system/repository/audit.go`
- **核心符号**
  - `InsertOperLogs(ctx, logs []*model.SysOperLog) error` — 批量插入操作日志。
  - `ListOperLogs(ctx, username, path string, page, size int) ([]*model.SysOperLog, int64, error)` — 分页查询。
- **主要调用关系**：被 `service/audit.go` 调用。
- **测试文件**：无。
- **涉及表/模型**：`sys_oper_log`。

### system — `internal/modules/system/dto/dto.go`
- **核心符号**
  - 请求：`LoginReq`、`UserCreateReq`、`UserUpdateReq`、`UserListQuery`、`ResetPwdReq`、`ChangePwdReq`、`StatusReq`、`RoleCreateReq`、`RoleUpdateReq`、`RoleListQuery`、`OperLogQuery`。
  - 响应：`LoginResp`、`ProfileResp`、`UserResp`、`RoleResp`、`OperLogResp`（操作日志查询的稳定响应契约）。
- **主要调用关系**：被 system handler 与 service 复用。
- **测试文件**：`dto_test.go`。
- **涉及表/模型**：无。

### system — `internal/modules/system/handler/handler.go`
- **核心符号**
  - `type Handler struct{...}`、`func New(svc *service.Service) *Handler`。
  - `RegisterRoutes(pub, auth *gin.RouterGroup, checker middleware.PermsChecker)` — pub 免登录、auth 已挂 Auth 中间件。
  - 各 HTTP handler：`login`、`profile`、`changePassword`、`listUsers`/`createUser`/`updateUser`/`updateUserStatus`/`deleteUser`/`resetPassword`、`listRoles`/`listAllRoles`/`createRole`/`updateRole`/`deleteRole`、`listOperLogs`。
- **主要调用关系**：被 `app/app.go` 注册路由；调用 `service.Service`。
- **测试文件**：无（同目录无 handler 测试）。
- **涉及表/模型**：无。

### system — `internal/modules/system/api/api.go`
- **核心符号**
  - `type SystemAPI interface` — 组合 `middleware.AuthValidator`（ValidateToken）、`middleware.PermsChecker`（HasPerm）、`middleware.OperLogRecorder`（Record）。
- **主要调用关系**：被中间件与 `app` 组装使用（依赖倒置）。
- **测试文件**：无。
- **涉及表/模型**：无。

### system — `internal/modules/system/dto/dto_test.go`
- **核心符号**：`func TestLoginTenantBinding(t *testing.T)` — 校验登录请求的租户绑定。
- **测试文件**：`dto_test.go`（本文件）。**涉及表/模型**：无。

### system — `internal/modules/system/service/auth_test.go`
- **核心符号**：`func TestLoginSelectsTenantWithoutGuessing(t *testing.T)`、`func TestCreateUserChecksUsernameWithinTargetTenant(t *testing.T)` — 登录不猜租户、创建用户检查目标租户内用户名。
- **测试文件**：`auth_test.go`（本文件）。**涉及表/模型**：`sys_user`。

### system — `internal/modules/system/service/permission_test.go`
- **核心符号**：`func TestTokenTenantAndExpiredPermissionCache(t *testing.T)`、`func TestHasPermDoesNotTrustUserIDOne(t *testing.T)`。
- **测试文件**：`permission_test.go`（本文件）。**涉及表/模型**：`sys_role`、`sys_user_role`。

### system — `internal/modules/system/service/login_limiter_test.go`
- **核心符号**：`func TestLoginLimiter(t *testing.T)`、`func TestLoginLimiterExpires(t *testing.T)`、`func TestLoginLimiterReservesConcurrentAttempts(t *testing.T)`、`func TestLoginLimiterBoundsMemoryAndSweepsOtherKeys(t *testing.T)`。
- **测试文件**：`login_limiter_test.go`（本文件）。**涉及表/模型**：无。

### system — `internal/modules/system/service/user_test.go`
- **核心符号**：`func TestUserPartialUpdateAndTenantRoles(t *testing.T)`、`func TestForeignRoleLinksCannotGrantPermissions(t *testing.T)`、`func TestRoleDeletionWaitsForConcurrentAssignment(t *testing.T)`。
- **测试文件**：`user_test.go`（本文件）。**涉及表/模型**：`sys_user`、`sys_user_role`、`sys_role`。

### system — `internal/modules/system/service/builtin_test.go`
- **核心符号**：`func TestBuiltinAdminProtection(t *testing.T)` — 内置管理员保护。
- **测试文件**：`builtin_test.go`（本文件）。**涉及表/模型**：`sys_user`、`sys_role`。

### system — `internal/modules/system/service/audit_test.go`
- **核心符号**：`func TestOperLogsDrainOnShutdownAndKeepTenant(t *testing.T)` — 关停时排空日志且保留租户。
- **测试文件**：`audit_test.go`（本文件）。**涉及表/模型**：`sys_oper_log`。

### system — `internal/modules/system/service/response_test.go`
- **核心符号**：`func TestSystemResponsesOnlyExposePublicFields(t *testing.T)`、`func TestOperLogResponseKeepsAuditFieldsAndStringIDs(t *testing.T)`。
- **测试文件**：`response_test.go`（本文件）。**涉及表/模型**：`sys_oper_log`。

### system — `internal/modules/system/model/model_test.go`
- **核心符号**：`func TestVersionedJSONDoesNotExposeInternalVersion(t *testing.T)`、`func TestSysUserJSONDoesNotExposeAuthenticationSecrets(t *testing.T)` — JSON 不暴露内部版本/认证机密。
- **测试文件**：`model_test.go`（本文件）。**涉及表/模型**：`sys_user`。

---

### basic — `internal/modules/basic/service/service.go`
- **核心符号**
  - `type Service struct{...}` — 仓库/库位/SKU 业务根结构（repo、`tm *tx.Manager`、`rdb redisClient`、`stock`、`limits`）。
  - `type redisClient interface` — 依赖倒置的 Redis 访问接口（Get/Set/Del）。
  - `func New(repo *repository.Repository, tm *tx.Manager, rdb redisClient, stock api.StockChecker, ...) *Service` — 构造注入。
- **主要调用关系**：被 `app.New` 组装，注入到 inbound/outbound 等模块。
- **测试文件**：`response_test.go`、`sku_test.go`、`delete_test.go`。
- **涉及表/模型**：无直接表。

### basic — `internal/modules/basic/service/warehouse.go`
- **核心符号**
  - `CreateWarehouse / UpdateWarehouse / UpdateWarehouseStatus / DeleteWarehouse` — 仓库增改删（删除含引用校验）。
  - `ListWarehouses / ListWarehouseResponses / warehouseResponse` — 列表与响应映射。
  - `ValidateWarehouse(ctx, id int64) error`、`GetWarehouseByCode(ctx, code string) (*model.Warehouse, error)`。
- **主要调用关系**：调用 `repo.*Warehouse*`、`repo.CountWarehouseReferences`、`s.stock`；被 `handler.*Warehouse` 与 inbound/outbound 等服务调用。
- **涉及表/模型**：`wms_warehouse`。

### basic — `internal/modules/basic/service/location.go`
- **核心符号**
  - `BatchCreateLocations(ctx, req *dto.LocationBatchReq) (created int, err error)` — 按"库区-排-列"批量生成，已存在编码跳过（幂等）。
  - `DeleteLocation / UpdateLocationStatus`。
  - `ListLocations / ListLocationResponses / locationResponse`。
  - `ValidateLocation(ctx, id int64)`、`ValidateLocationInWarehouse(ctx, warehouseID, id int64)`、`GetLocation(ctx, id int64)`、`UpdateLocationStatusInTx(ctx, tx *gorm.DB, id int64, status int)`。
- **主要调用关系**：被 `handler.*Location`、inbound/outbound/stocktake 服务调用。
- **涉及表/模型**：`wms_location`。

### basic — `internal/modules/basic/service/sku.go`
- **核心符号**
  - 包注释：提供仓库、库位和 SKU 的业务规则及缓存处理。
  - `CreateSKU / UpdateSKU / DeleteSKU`、`ListSKUs`、`GetByBarcode`、`barcodeKey(tenantID int64, barcode string) string`（条码缓存键含租户）。
  - `ValidateSKU`、`GetSKU`、`GetSKUByCode`、`ListSKUResponses`、`GetSKUByBarcodeResponse`、`skuResponse`。
- **主要调用关系**：被 `handler.*SKU` 与业务模块调用。
- **涉及表/模型**：`wms_sku`。

### basic — `internal/modules/basic/model/model.go`
- **核心符号**
  - 包注释：定义仓库、库位和 SKU 的持久化结构及编码唯一约束。
  - `type Warehouse struct`（`TableName() = "wms_warehouse"`）— 租户内范围边界。
  - `type Location struct`（`wms_location`）— 库存四元组中的实际存放位置。
  - `const ( LocationStatusDisabled / LocationStatusIdle / LocationStatusOccupied ... )` — 库位状态。
  - `type SKU struct`（`wms_sku`）— 租户内编码与条码分别唯一。
- **测试文件**：`model_json_test.go`。
- **涉及表/模型**：`wms_warehouse`、`wms_location`、`wms_sku`。

### basic — `internal/modules/basic/repository/repository.go`
- **核心符号**：`type Repository struct{}`、`func New() *Repository` — 无状态仓储（依赖调用方传入 `*gorm.DB`）。
- **涉及表/模型**：待确认（通用入口）。

### basic — `internal/modules/basic/repository/warehouse.go`
- **核心符号**：`GetWarehouseByCode`、`GetWarehouse`、`GetWarehouseForUpdate`（行锁，串行化删除与库位创建）、`CreateWarehouse`、`UpdateWarehouse`、`UpdateWarehouseStatus`（只改状态）、`DeleteWarehouse`、`CountLocationsByWarehouse`、`ListWarehouses`。
- **涉及表/模型**：`wms_warehouse`。

### basic — `internal/modules/basic/repository/location.go`
- **核心符号**：`GetLocationByCode`、`GetLocation`、`GetLocationForUpdate`（行锁）、`CreateLocation`、`CreateLocationBatch`、`UpdateLocation`、`UpdateLocationStatusInTx`、`DeleteLocation`、`ListLocations`、`ListLocationCodes`（批量生成幂等跳过用）。
- **涉及表/模型**：`wms_location`。

### basic — `internal/modules/basic/repository/sku.go`
- **核心符号**：`GetSKUByCode`、`GetSKUByBarcode`、`GetSKU`、`GetSKUForUpdate`（行锁）、`CreateSKU`、`UpdateSKU`、`DeleteSKU`、`ListSKUs`。
- **涉及表/模型**：`wms_sku`。

### basic — `internal/modules/basic/repository/references.go`
- **核心符号**
  - `CountWarehouseReferences` — 统计仓库下的库位/任务/单据引用（库存单独走 `StockChecker`）。
  - `CountLocationReferences` — 统计库位上的任务和单据引用。
  - `CountSKUReferences` — 统计货品上的任务、单据明细和分配引用。
  - `type referenceColumn struct`、`countReferences(...)` — 通用引用计数（用 `tenant.Scope`）。
- **涉及表/模型**：`wms_task`、`wms_receipt_order*`、`wms_shipment_order*`、`wms_allocation`、`wms_stocktake_detail` 等。

### basic — `internal/modules/basic/dto/dto.go`
- **核心符号**：`WarehouseReq`、`StatusReq`、`CommonQuery`、`WarehouseQuery`、`LocationQuery`、`LocationBatchReq`（批量初始化库位）、`SKUReq`、`WarehouseResp`、`LocationResp`、`SKUResp`、`LocationBatchResp`。
- **测试文件**：`dto_test.go`。

### basic — `internal/modules/basic/handler/handler.go`
- **核心符号**：`Handler`/`New`、`RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker)`；仓库 handler `listWarehouses/createWarehouse/updateWarehouse/warehouseStatus/deleteWarehouse`；库位 `listLocations/batchCreateLocations/locationStatus/deleteLocation`；SKU `listSKUs/getByBarcode/createSKU/updateSKU/deleteSKU`。
- **测试文件**：`handler_test.go`。

### basic — `internal/modules/basic/api/api.go`
- **核心符号**
  - 包注释：定义 basic 模块提供给其他业务模块的仓库、库位和 SKU 能力。
  - `type StockChecker interface` — 库存存在性校验，由 inventory 模块实现（app 组装注入）。
  - `type BasicAPI interface` — basic 模块对外接口。
- **涉及表/模型**：无。

### basic — `internal/modules/basic/dto/dto_test.go`
- **核心符号**：`func TestLocationBatchRespJSONContract(t *testing.T)`。
- **测试文件**：`dto_test.go`（本文件）。**涉及表/模型**：无。

### basic — `internal/modules/basic/handler/handler_test.go`
- **核心符号**：`func TestBasicListQueriesBindZeroStatusAndLocationFilters(t *testing.T)` — 列表查询正确绑定"零状态"和库位过滤。
- **测试文件**：`handler_test.go`（本文件）。**涉及表/模型**：`wms_warehouse`、`wms_location`、`wms_sku`。

### basic — `internal/modules/basic/repository/repository_test.go`
- **核心符号**：`func TestUpdateWarehouseStatusDoesNotTouchOtherFields(t *testing.T)`、`func TestListWarehousesAppliesStatusFilter(t *testing.T)`、`func TestListLocationsAppliesZoneStatusAndKeywordFilters(t *testing.T)`。
- **测试文件**：`repository_test.go`（本文件）。**涉及表/模型**：`wms_warehouse`、`wms_location`。

### basic — `internal/modules/basic/service/sku_test.go`
- **核心符号**：`func TestBarcodeCacheIsolatesTenants(t *testing.T)`、`func TestUpdateSKUInvalidatesUnchangedBarcode(t *testing.T)`、`func TestSKUQueriesPreserveDatabaseErrors(t *testing.T)`。
- **测试文件**：`sku_test.go`（本文件）。**涉及表/模型**：`wms_sku`。

### basic — `internal/modules/basic/service/delete_test.go`
- **核心符号**：`func TestDeleteBasicDataReturnsNotFound(t *testing.T)`、`func TestDeleteBasicDataBlocksInventoryAndReferences(t *testing.T)`、`func TestDeleteBasicDataRejectsZeroStockInventoryHistory(t *testing.T)`、`func TestConcurrentDeleteSKUAndIncreaseDoesNotCreateOrphanInventory(t *testing.T)`、`func TestConcurrentDeleteLocationAndIncreaseDoesNotCreateOrphanInventory(t *testing.T)`、`func TestConcurrentDeleteWarehouseAndCreateLocationDoesNotCreateOrphanLocation(t *testing.T)`。
- **测试文件**：`delete_test.go`（本文件）。**涉及表/模型**：`wms_sku`、`wms_location`、`wms_warehouse`、`wms_inventory`。

### basic — `internal/modules/basic/service/response_test.go`
- **核心符号**：`func TestBasicResponsesKeepPublicFieldsAndStringIDs(t *testing.T)`。
- **测试文件**：`response_test.go`（本文件）。**涉及表/模型**：无。

### basic — `internal/modules/basic/model/model_json_test.go`
- **核心符号**：`func TestExternalIDsAreJSONStrings(t *testing.T)` — 外部 ID 在 JSON 中以字符串输出。
- **测试文件**：`model_json_test.go`（本文件）。**涉及表/模型**：`wms_warehouse`、`wms_location`、`wms_sku`。

---

### ai — `internal/modules/ai/service/service.go`
- **核心符号**
  - 包注释：提供 AI 库存问答、快照和请求限流逻辑。
  - `const systemPrompt = ...` — 库存智能助手系统提示词（严禁生成 SQL）。
  - `type Service struct{...}` — 后端检索真实库存拼入提示词后调用 LLM；单轮无状态。
  - `func New(cfg config.AIConfig, db *gorm.DB, rdb *redis.Client) *Service`。
  - `Chat(ctx, userID int64, question string) (string, error)` — 限流 → 检索快照 → 拼 Prompt → 主模型（失败降级备用）。
  - `allowRate(...)` — 每用户每分钟限流（Redis INCR + EXPIRE，故障降级放行）。
  - `allowDailyQuota(...)` — 每租户每日上限（平台租户不限）。
  - `buildSnapshot(...)`、`matchedSKUDetails(...)` — 检索增强快照与 SKU 明细匹配。
- **主要调用关系**：调用 `repository.Repository`、`client.Client`；被 `handler.chat` 调用。
- **测试文件**：`service_test.go`。
- **涉及表/模型**：只读经 `repository`。

### ai — `internal/modules/ai/repository/repository.go`
- **核心符号**
  - 包注释：提供 AI 问答所需的只读库存快照查询。
  - `tenantScope(ctx, q *gorm.DB) *gorm.DB` — 裸表联查手动注入租户条件（AI 快照绝不跨租户泄漏）。
  - 类型：`Overview`、`WarehouseStock`、`SKUStock`、`InventoryDetail`、`SKUBrief`。
  - `Repository`/`New(db *gorm.DB)`。
  - `GetOverview`（SKU 总数 + 三数量汇总）、`ListWarehouseStock`、`TopSKUByStock`、`LowAvailableSKU`、`ListSKUBrief`、`ListInventoryDetailBySKU`。
- **主要调用关系**：被 `service.Service` 调用。
- **涉及表/模型**：`wms_inventory`、`wms_sku`、`wms_warehouse`（只读检索，联查表手动补 `deleted_at`）。

### ai — `internal/modules/ai/client/client.go`
- **核心符号**
  - 包注释：封装兼容 OpenAI 协议的智谱 Chat API 调用。
  - `type Message struct`、`type APIError struct`（HTTP 非 2xx 携带错误码）、`type Client struct`。
  - `func New(apiKey, baseURL string, timeoutSeconds int) *Client`。
  - `Chat(ctx, model string, messages []Message, temperature float64, maxTokens int) (string, error)` — 单次调用，主备降级由上层控制。
  - `type thinkingConfig struct` — 库存问答禁用思维链（否则 flash 模型可能返回空 content）。
- **涉及表/模型**：无。

### ai — `internal/modules/ai/dto/dto.go`
- **核心符号**：`ChatReq`（question 非空且不超过 200 字符）、`ChatResp`（纯文本，前端按纯文本渲染防 XSS）。
- **涉及表/模型**：无。

### ai — `internal/modules/ai/handler/handler.go`
- **核心符号**：`Handler`/`New`、`RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker)`（权限复用 `wms:inventory` 读权限）、`chat`。
- **涉及表/模型**：无。

### ai — `internal/modules/ai/service/service_test.go`
- **核心符号**：`func TestBuildSnapshot(t *testing.T)`、`func TestChatKeyMissing(t *testing.T)`、`func TestAllowRateRedisNil(t *testing.T)`。
- **测试文件**：`service_test.go`（本文件）。**涉及表/模型**：`wms_inventory`、`wms_sku`。

---

### demo — `internal/modules/demo/service/service.go`
- **核心符号**
  - 包注释：提供演示与个人体验账号的会话、配额和数据重置逻辑。
  - `activeSessionKeyOf(tenantID int64) string` — 会话锁键；锁按租户隔离（一租户一访客）。
  - `type Service struct{...}`、`func New(cfg *config.Config, db *gorm.DB, rdb *redis.Client, ...) *Service`。
  - `Enabled() bool`（`cfg.Demo.Enabled && rdb != nil`）、`IsDemoTenant(ctx) bool`、`Username(ctx) string`（兼容历史返回 `"demo"`）、`demoTenantID(ctx) (int64, error)`、`sessionTTL() time.Duration`、`ValidateSession(ctx, sessionID string) error`。
- **涉及表/模型**：跨业务模块（单据/任务/库存等），经各模块 service。

### demo — `internal/modules/demo/service/session.go`
- **核心符号**
  - `const demoExecutionTTL = 10 * time.Minute`、`demoResetLockKeyOf(tenantID int64) string`。
  - Lua 脚本：`renewSessionScript`、`deleteSessionScript`、`sessionTTLScript`。
  - 类型：`SessionInfo`、`DemoAccountInfo`。
  - `ClaimAccount(ctx)` — 返回第一个空闲演示账号（按会话锁判断，仅分配建议）。
  - `AcquireSession(ctx)` — 领取会话并**无条件重置该租户数据**（首访种初始数据，复访清上一手改动）。
  - `Heartbeat / ReleaseSession（释放并立即恢复数据）/ Reset（保留会话锁重置）/ SessionStatus`。
  - `renewSession / deleteSession / resetLocked / beginTenantRun`（为整请求持租户级 Redis 执行锁并延长会话租约）。
- **主要调用关系**：被 `handler.acquire/heartbeat/release/status/reset` 与中间件 `middleware.DemoSession`（经 `DemoSessionValidator`）调用。
- **测试文件**：`session_test.go`、`tenant_lock_test.go`。
- **涉及表/模型**：`sys_user`（演示账号）、业务单据表（重置）。

### demo — `internal/modules/demo/service/scenario.go`
- **核心符号**
  - 场景常量 `ScenarioInbound / ScenarioOutbound / ScenarioStocktake / ScenarioFull`（及步骤/整体状态常量）。
  - 类型：`ScenarioStep`、`ScenarioFact`、`ScenarioEvidence`、`ScenarioLink`、`ScenarioImplementation`、`ScenarioResult`、`ScenarioExecutionError`、`ScenarioOptions`、`scenarioRun`、`demoRefs`。
  - `newScenarioRun(name, summary string, steps ...ScenarioStep) *scenarioRun`、`execute(index int, statusChange, technical string, fn func() (string, error)) error`、`finish(summary string) *ScenarioResult`。
  - `RunScenario(ctx, sessionID, scenario string, options ...ScenarioOptions) (*ScenarioResult, error)` — 先恢复默认演示数据再执行；租户级 Redis 执行锁同租户串行。
  - `loadDemoBaseRefs / loadDemoRefs`、`demoBatchNo()`、`demoBizOrderNo(seq int) string`、`mergeScenarioResults`、`cloneScenarioImplementation`、`mergeScenarioImplementation`、`appendUniqueStrings`。
- **主要调用关系**：被 `handler.runScenario/runWithOptions` 与各 `scenario_*.go`、`drafts.go`、`experiment_*.go` 调用。
- **测试文件**：`scenario_test.go`。
- **涉及表/模型**：跨业务模块。

### demo — `internal/modules/demo/service/scenario_inbound.go`
- **核心符号**：`runInboundDemo(ctx, refs *demoRefs) (*ScenarioResult, error)`；辅助 `inventoryStockChangeSummary(trans *inventorymodel.InventoryTrans) string`、`stockChangeFacts(...) []ScenarioFact`。
- **涉及表/模型**：入库单/明细、`wms_inventory`、`wms_inventory_trans`。

### demo — `internal/modules/demo/service/scenario_outbound.go`
- **核心符号**：`runOutboundDemo(...)`；FIFO/流水辅助 `loadOutboundAllocationInventories`、`loadOutboundInventoryTrans`、`sortOutboundAllocationsByFIFO`、`outboundFIFOFacts`、`outboundPickTaskFacts`、`inventoryTransFacts`、`summarizeOutboundTrans`、`outboundFIFOEvidence`、`outboundTaskNumbers`、`outboundTransEvidence`、`outboundStockChangeValue`、`outboundStockChangeDetail`、`formatOutboundStockIn`、`outboundStockSummary`。
- **涉及表/模型**：`wms_shipment_order*`、`wms_allocation`、`wms_task`、`wms_inventory`、`wms_inventory_trans`。

### demo — `internal/modules/demo/service/scenario_stocktake.go`
- **核心符号**：`runStocktakeDemo(...)`；辅助 `summarizeStocktakeDetails`、`stocktakeSnapshotFacts`、`stocktakeActualFacts`、`stocktakeDifferenceFacts`、`stocktakeAdjustmentFacts`、`stocktakeBatchNo`、`stocktakeSummary`。
- **涉及表/模型**：`wms_stocktake_order`、`wms_stocktake_detail`、`wms_inventory_trans`。

### demo — `internal/modules/demo/service/drafts.go`
- **核心符号**：`createInboundDrafts`（模拟 Excel 导入：只建草稿）、`createOutboundDrafts`（模拟上游 OMS/ERP 推送：只建草稿）、`createStocktakeDrafts`、`normalizeDraftOptions(count, qty, defaultCount, defaultQty int) (int, int)`。
- **涉及表/模型**：入库单/出库单/盘点单草稿。

### demo — `internal/modules/demo/service/activity.go`
- **核心符号**：`Activity(ctx, limit int) (*ActivitySnapshot, error)` — 查询当前演示账号最近操作与业务记录。
- **测试文件**：`activity_test.go`。

### demo — `internal/modules/demo/service/activity_response.go`
- **核心符号**：`ActivitySnapshot`（操作记录接口稳定响应）；子响应 `activityOperationResp`、`activityInboundOrderResp`、`activityOutboundOrderResp`、`activityStocktakeOrderResp`、`activityTaskResp`、`activityInventoryTransResp`；映射 `activitySnapshot(...)`、`activityOperationResponses`、`activityInboundOrderResponses`、`activityOutboundOrderResponses`、`activityStocktakeOrderResponses`、`activityTaskResponses`、`activityInventoryTransResponses`。

### demo — `internal/modules/demo/service/performance.go`
- **核心符号**：`PerformanceSnapshot`（不依赖 Prometheus 的实时快照）、`ComponentHealth`、`DBPoolStats`、`RuntimeStats`、`BusinessStats`；`Performance(ctx) (*PerformanceSnapshot, error)` — 返回 DB/Redis/连接池/Go 运行时/业务数据快照。

### demo — `internal/modules/demo/service/personal.go`
- **核心符号**
  - 包注释：持久体验账号 user1..userN，一人一租户、数据长期保留（不重置、无会话锁），由 `bootstrap.SeedPersonalAccounts` 幂等创建。
  - `PersonalAccountInfo`、`PersonalEnabled() bool`、`PersonalAccounts() ([]PersonalAccountInfo, error)`（不下发密码）、`ClaimPersonalAccount(username string) (*PersonalAccountInfo, error)`（校验后返回登录凭据）。

### demo — `internal/modules/demo/service/experiment_allocation.go`
- **核心符号**：`ConcurrentResult`；`concurrentAttempt`、`concurrentAttemptSummary`、`concurrentInventoryStats`；`RunConcurrentAllocation(ctx, sessionID string, concurrency, qtyPerOrder int) (*ConcurrentResult, error)`；`prepareConcurrentOutbound`、`concurrentAttemptEvidence`、`summarizeConcurrentAttempts`、`concurrentInventoryInvariantOK`、`concurrentInventoryStats`、`concurrentRunTag()`、`concurrentDemoBizOrderNo(runTag string, sequence int) string`。
- **测试文件**：`experiment_allocation_test.go`。

### demo — `internal/modules/demo/service/experiment_picking.go`
- **核心符号**：`PickingInventoryEvidence`、`PickingResult`、`pickingExperimentPreparation`；`preparePickingExperiment(ctx)`、`RunConcurrentPicking(ctx, sessionID string, workers, contenders int) (*PickingResult, error)`、`pickRemaining(...)`。

### demo — `internal/modules/demo/service/experiment_shortage.go`
- **核心符号**：`ConcurrentShortageResult`（仅说明本次调用成功分配量是否受初始可用库存限制）、`concurrentShortageSummary`；`summarizeShortageAttempts(...)`、`RunConcurrentShortageValidation(ctx, sessionID string, concurrency, qtyPerOrder int) (*ConcurrentShortageResult, error)`（用真实 Outbound Service 并发建/提/审，不直接改库存表）。

### demo — `internal/modules/demo/service/experiment_restock.go`
- **核心符号**：`RestockDemo(ctx, sessionID string, qty int) (*ScenarioResult, error)`、`restockWithinRun(ctx, qty int) (*ScenarioResult, error)`、`loadRestockLocation(ctx, warehouseID, skuID int64) (*basicmodel.Location, error)`。

### demo — `internal/modules/demo/handler/handler.go`
- **核心符号**
  - `type Handler struct`/`New(svc *service.Service) *Handler`。
  - `RegisterPublicRoutes(pub *gin.RouterGroup)` — 免登录"在线体验"领取演示账号（演示未启用则不挂载 → 404）。
  - `RegisterPersonalPublicRoutes(pub *gin.RouterGroup)` — 免登录"个人空间"列出并领取持久账号。
  - `RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker)` — 会话 acquire/heartbeat/release/status、场景 `runInbound/runOutbound/runStocktake/runFull`、草稿 `runInboundDrafts/runOutboundDrafts/runStocktakeDrafts`、并发 `runConcurrent/runConcurrentShortage/runConcurrentPicking`、`runRestock`、`performance`、`activity`、`reset`。
  - 内部：`runWithOptions`、`runScenario`、`writeScenarioFailure`、`demoSessionID(c *gin.Context) string`。
- **测试文件**：`handler_test.go`。
- **涉及表/模型**：无。

### demo — `internal/modules/demo/handler/handler_test.go`
- **核心符号**：`func TestWriteScenarioFailureReturnsMergedExecutionResult(t *testing.T)` — 场景失败时返回合并后的执行结果。
- **测试文件**：`handler_test.go`（本文件）。**涉及表/模型**：无。

### demo — `internal/modules/demo/service/session_test.go`
- **核心符号**：`func TestSessionOperationsRejectPreviousOwner(t *testing.T)`、`func TestExecutionLockPreventsSessionReplacementDuringLongRun(t *testing.T)`。
- **测试文件**：`session_test.go`（本文件）。**涉及表/模型**：无（Redis 会话锁）。

### demo — `internal/modules/demo/service/tenant_lock_test.go`
- **核心符号**：`func TestScenarioExecutionLockIsScopedPerTenant(t *testing.T)`、`func TestDifferentDemoTenantsRunConcurrently(t *testing.T)`、`func TestOldSessionCannotResetNewSession(t *testing.T)`。
- **测试文件**：`tenant_lock_test.go`（本文件）。**涉及表/模型**：无（Redis 执行锁）。

### demo — `internal/modules/demo/service/scenario_test.go`
- **核心符号**：`func TestScenarioRunRecordsCompletedSteps(t *testing.T)`、`func TestScenarioExecutionErrorKeepsCompletedAndFailedSteps(t *testing.T)`、`func TestInventoryStockChangeSummaryUsesThreeQuantityInvariant(t *testing.T)`、`func TestOutboundFIFOFactsUseRealAllocationsInStockInOrder(t *testing.T)`、`func TestOutboundPickFactsUseCompletedRealTasks(t *testing.T)`、`func TestSummarizeOutboundTransUsesAllocationAndShipValues(t *testing.T)`、`func TestMergeScenarioResultsKeepsInboundEvidence(t *testing.T)`、`func TestMergeScenarioResultsCombinesTechnicalImplementation(t *testing.T)`、`func TestMergeScenarioResultsUsesGenericTitleForMultipleEvidenceSets(t *testing.T)`、`func TestStocktakePresentationUsesRecordedBookAndActualQuantities(t *testing.T)`、`func TestStocktakeAdjustmentFactsUsePersistedInventoryTrans(t *testing.T)`。
- **测试文件**：`scenario_test.go`（本文件）。**涉及表/模型**：`wms_inventory`、`wms_inventory_trans`、`wms_allocation`、`wms_task`。

### demo — `internal/modules/demo/service/activity_test.go`
- **核心符号**：`func TestActivitySnapshotKeepsBusinessRecordsAndHidesInternalVersion(t *testing.T)`。
- **测试文件**：`activity_test.go`（本文件）。**涉及表/模型**：跨业务模块单据表。

### demo — `internal/modules/demo/service/identity_test.go`
- **核心符号**：`func TestOnlyConfiguredDemoTenantsCanAcquireOrReset(t *testing.T)`、`func TestPersonalAccountCredentialsIncludeTenant(t *testing.T)`。
- **测试文件**：`identity_test.go`（本文件）。**涉及表/模型**：无。

### demo — `internal/modules/demo/service/experiment_allocation_test.go`
- **核心符号**：`func TestSummarizeConcurrentAttemptsByStage(t *testing.T)`、`func TestConcurrentInventoryInvariant(t *testing.T)`、`func TestSummarizeShortageAttemptsClassifiesRealFailureReasons(t *testing.T)`。
- **测试文件**：`experiment_allocation_test.go`（本文件）。**涉及表/模型**：`wms_inventory`。

---

### bootstrap — `internal/bootstrap/database.go`
- **核心符号**
  - `InitDB(cfg *config.Config) (*gorm.DB, error)` — 初始化 GORM MySQL 连接并注册多租户全局回调。
  - `InitRedis(cfg *config.Config) *redis.Client` — 初始化 Redis（缓存/单号可降级；会话校验等安全操作仍需拒绝故障请求）。
  - `AutoMigrate(db *gorm.DB) error` — 自动迁移表结构 + CHECK 约束（生产用 `cmd/migrate`）。
- **主要调用关系**：由 `cmd/wms/main.go` 的 `run` 调用。
- **涉及表/模型**：全部业务表。

### bootstrap — `internal/bootstrap/bootstrap.go`
- **核心符号**
  - 包注释：负责数据库与 Redis 初始化、开发环境迁移，以及演示数据的种子与重置。
  - `func Migrate(db *gorm.DB, cfg *config.Config) error` — 仅供开发和测试：AutoMigrate + 管理员初始化 + 演示数据与账号。
- **主要调用关系**：由 `cmd/wms/main.go` 在 debug 模式下调用。

### bootstrap — `internal/bootstrap/seed.go`
- **核心符号**：`func SeedDemo(db *gorm.DB, cfg *config.Config) error` — 写入演示基础数据与体验账号（先 `seedDemoData`，再 `SeedDemoAccounts` / `SeedPersonalAccounts`；各自幂等）。
- **涉及表/模型**：`sys_user` 等。

### bootstrap — `internal/bootstrap/seed_admin.go`
- **核心符号**：`func SeedAdmin(db *gorm.DB, cfg *config.Config) error` — 创建/修复内置管理员与角色（tenant_id=0）：密码取 `WMS_ADMIN_PASSWORD`，debug 默认 `admin123`，release 缺失即失败；管理员已存在时幂等补齐、不再要求密码。
- **涉及表/模型**：`sys_user`、`sys_role`、`sys_user_role`。

### bootstrap — `internal/bootstrap/seed_demo.go`
- **核心符号**：`type demoPlacement struct`（仓库序号-库位编码-SKU序号-批次-数量-入库时间）、`seedDemoData(db *gorm.DB) error`（仅当系统中不存在任何仓库时执行，避免覆盖用户数据）。
- **涉及表/模型**：`wms_warehouse`、`wms_location`、`wms_sku`、`wms_inventory`、`wms_inventory_trans`、演示单据。

### bootstrap — `internal/bootstrap/demo_accounts.go`
- **核心符号**：`const demoPerms = ...`（只读 + 业务操作 + 演示中心，无系统管理权限）、`SeedDemoAccounts(db *gorm.DB, cfg *config.Config) error`（幂等创建 demo1..demoN，每账号独占租户 10001+；同时禁用名单外历史演示账号；仅 `WMS_DEMO_ENABLED=true` 时创建）、`managedDemoTenantIDs() []int64`。
- **涉及表/模型**：`sys_user`、`sys_role`、`sys_user_role`。

### bootstrap — `internal/bootstrap/personal_accounts.go`
- **核心符号**：`const personalPerms = ...`（与演示账号一致但**不含 `wms:demo`**，保证数据不被演示重置）、`SeedPersonalAccounts(db *gorm.DB, cfg *config.Config) error`（幂等创建 user1..userN，独占租户 20001+，首次创建种初始数据后不再重置；仅 `WMS_PERSONAL_ENABLED=true`）、`managedPersonalTenantIDs() []int64`、`seedPersonalData(db *gorm.DB, tenantID int64) error`（必须在租户 ctx 内执行）。
- **测试文件**：`personal_test.go`。
- **涉及表/模型**：`sys_user`、`sys_role`、`sys_user_role`、业务数据表。

### bootstrap — `internal/bootstrap/reset.go`
- **核心符号**：`ResetDemoData(ctx context.Context, db *gorm.DB, tenantID int64) error` — 硬删除指定租户演示业务数据并重写默认演示数据；用户/角色/迁移记录/操作日志不删除；`tenantID <= 0` 时直接拒绝执行，防止误用导致全表物理删除。
- **涉及表/模型**：各业务表（不含 `sys_user`/`sys_role`/`sys_oper_log`）。

### bootstrap — `internal/bootstrap/seed_test.go`
- **核心符号**：`func TestSeedAdminCreatesAndRepairsPartialState(t *testing.T)`、`func TestSeedDemoAccountsDoesNotDisableSameNameInOtherTenant(t *testing.T)`。
- **测试文件**：`seed_test.go`（本文件）。**涉及表/模型**：`sys_user`、`sys_role`。

### bootstrap — `internal/bootstrap/personal_test.go`
- **核心符号**：`func TestSeedPersonalAccountsIsolatedTenants(t *testing.T)`、`func TestSeedPersonalAccountsShrinkAndDisable(t *testing.T)`、`func TestSeedPersonalAccountsDoesNotTouchDemoAccounts(t *testing.T)`、`func TestSeedPersonalAccountsDoesNotDisableSameNameInOtherTenant(t *testing.T)`。
- **测试文件**：`personal_test.go`（本文件）。**涉及表/模型**：`sys_user`、业务数据表。

---

### app — `internal/app/app.go`
- **核心符号**
  - 包注释：负责应用依赖组装、路由注册和模块连接。
  - `type App struct{...}` — 通过构造函数组装模块依赖，提供路由注册所需处理器。
  - `func New(cfg *config.Config, db *gorm.DB, rdb *redis.Client, metrics *observability.Metrics) *App` — 按依赖顺序组装（basic→inventory，inbound/outbound→basic+inventory+task）。
  - `type redisAdapter struct{ rdb *redis.Client }`、`newRedisAdapter`、`Get/Set/Del` — 将 go-redis 适配为 basic 的 `redisClient` 接口（依赖倒置）。
- **主要调用关系**：由 `cmd/wms/main.go` 构造。
- **测试文件**：`integration_test.go`、`router_test.go`。

### app — `internal/app/router.go`
- **核心符号**
  - `NewRouter() (*gin.Engine, error)` — 中间件链：`RequestID → CORS → Recovery → AccessLog → Auth(JWT) → OperLog(异步审计) → Permission(按路由)`（另含 BodyLimit）。
  - `healthz(ctx context.Context) error` — 健康检查：DB 必须可用，Redis 不可用不影响健康（已降级运行）。
- **主要调用关系**：被 `main.go` 调用；依赖 `systemapi.SystemAPI`、`demoservice` 等。
- **测试文件**：`router_test.go`、`integration_test.go`。

### app — `internal/app/router_test.go`
- **核心符号**：`func TestBusinessRoutesRequireDemoSession(t *testing.T)` — 业务路由要求演示会话。
- **测试文件**：`router_test.go`（本文件）。**涉及表/模型**：无。

### app — `internal/app/integration_test.go`
- **核心符号**：`func TestExternalAPIKeyIsolatesCodesAndBusinessNumbers(t *testing.T)`、`func TestExternalConcurrentDuplicateReturnsExistingOrder(t *testing.T)`。
- **测试文件**：`integration_test.go`（本文件）。**涉及表/模型**：业务单据表。

### app — `internal/app/pick_race_test.go`
- **核心符号**：`pickRaceStack`（真实 MySQL 组装拣货链路）、`newPickRaceStack`、`(*pickRaceStack).newPickOrder`、`txGate`/`newTaskUpdateGate`（gorm 回调暂停事务构造交错）、`signalTaskRowLock`；用例 `TestPickCancelRaceCancelFirst`、`TestPickCancelRacePickFirst`、`TestPickCancelRaceSimultaneous`、`TestPDAClaimStrictPickAndIdempotency`。
- **测试文件**：`pick_race_test.go`（本文件）。**涉及表/模型**：`wms_task`、`wms_allocation`、`wms_shipment_order*`、`wms_inventory`、`wms_idempotency`。

---

### cmd/wms — `cmd/wms/main.go`
- **核心符号**
  - 包注释：启动 HTTP 服务并管理数据库、Redis 和后台任务的资源生命周期。
  - `func main()`、`func run() error` — `run` 管理资源生命周期（loadDotEnv → config.Load → log.Init → snowflake.Init → InitDB →（debug）`bootstrap.Migrate` /（release）仅同步演示与持久体验账号 → InitRedis → app.New → serve；启动 `RunOperLogs`、`RunCompensator`、`RunImports`、`RunImportFileCleanup` 等 worker），返回后 main 才退出，确保 defer 执行。
- **测试文件**：`server_test.go`、`dotenv_test.go`。

### cmd/wms — `cmd/wms/server.go`
- **核心符号**：`func serve(ctx context.Context, shutdownTimeout time.Duration, servers ...*http.Server) error` — 收到退出信号或任一监听失败时优雅关闭全部 HTTP 服务。

### cmd/wms — `cmd/wms/dotenv.go`
- **核心符号**：`func loadDotEnv(path string) error` — 支持本地开发 KEY=VALUE 配置；已有环境变量优先；缺少文件正常，读取失败/非法保留错误且不输出密钥值。

### cmd/migrate — `cmd/migrate/main.go`
- **核心符号**
  - 包注释：应用版本化数据库迁移，并把管理员初始化、演示数据、应用账户补齐拆成独立子命令。
  - `var errUsage`、`func main()`、`func run() error`（子命令 `up` / `down` / `version` / `force` / `bootstrap-admin` / `seed-demo` / `ensure-app-user`）、`func seedBootstrapAdmin(cfg *config.Config) error`、`func seedDemo(cfg *config.Config) error`、`func withDB(cfg *config.Config, fn func(*gorm.DB) error) error`、`func usage()`、`func waitForMySQL(db *sql.DB, timeout time.Duration) error`、`func migrationDSN(dsn string) string`。
- **主要调用关系**：使用 `migrations.FS`（iofs 源）+ golang-migrate MySQL driver。
- **涉及表/模型**：`schema_migrations`（迁移记录表）。

### cmd/migrate — `cmd/migrate/app_user.go`
- **核心符号**：`func ensureAppUser(db *sql.DB, dsn string) error` — 用 root 连接幂等创建/修复应用账户（`MYSQL_USER` / `MYSQL_PASSWORD`）：`CREATE USER IF NOT EXISTS` + `ALTER USER`（密码对齐当前配置）+ `GRANT` 目标库全部权限；不触碰任何 schema，供已有数据卷升级补齐账户。
- **涉及表/模型**：无（只操作 `mysql.user` 与授权）。

### cmd/wms — `cmd/wms/dotenv_test.go`
- **核心符号**：`func TestLoadDotEnv(t *testing.T)`、`func TestLoadDotEnvReportsReadError(t *testing.T)`。
- **测试文件**：`dotenv_test.go`（本文件）。**涉及表/模型**：无。

### cmd/wms — `cmd/wms/server_test.go`
- **核心符号**：`func TestServeStopsAllServersOnListenerFailure(t *testing.T)`、`func TestServeStopsOnCancellation(t *testing.T)`、`func TestServeClosesActiveConnectionsOnShutdownTimeout(t *testing.T)`。
- **测试文件**：`server_test.go`（本文件）。**涉及表/模型**：无。

---

### pkg/config — `internal/pkg/config/config.go`
- **核心符号**
  - 包注释：加载、覆盖并校验应用配置。
  - `Load(path string) (*Config, error)` — 读取 `configs/config.yaml`；环境变量覆盖（`WMS_` 前缀，`.` 分隔，如 `WMS_MYSQL_DSN`）。
  - `SplitCSV(value string) []string`、`(*Config).CORSOrigins()`、`(*Config).TrustedProxyCIDRs()`。
- **测试文件**：`config_test.go`。

### pkg/config — `internal/pkg/config/types.go`
- **核心符号**
  - `Config` 及子配置：`ServerConfig`、`MySQLConfig`、`RedisConfig`、`JWTConfig`、`LogConfig`、`UploadConfig`、`MetricsConfig`、`IntegrationConfig`、`DemoConfig`、`PersonalConfig`、`LimitsConfig`、`AIConfig`。
  - `DemoConfig`：`AccountUsername(i)`（demo1..）、`AccountTenantID(i)`（10001+）、`AccountIndex(tenantID)`；`const demoTenantIDBase int64 = 10000`。
  - `PersonalConfig`：`AccountUsername`（user1..）、`AccountTenantID`（20001+）、`AccountNickname`、`AccountIndexByUsername`；`const personalTenantIDBase int64 = 20000`。
  - `LimitsConfig` — 公开租户数据量配额，仅 `tenant_id > 0` 生效。
  - `AIConfig` — `APIKey` 只读环境变量 `ZHIPU_API_KEY`，绝不写入配置文件。

### pkg/tenant — `internal/pkg/tenant/tenant.go`
- **核心符号**
  - 包注释：多租户最小版（共享库加列）；租户 ID 经 JWT→ctx→GORM 全局回调注入 `WHERE tenant_id = ?`；ctx 无租户或为 0 视为平台旁路。
  - `const tenantCtxKey / exactScopeKey`。
  - `WithExactTenant(ctx, tenantID int64) context.Context` — 限定单一租户（含 0），用于 API Key 等不应具平台旁路权限的入口。
  - `hasScope(ctx) bool`、`WithTenant(ctx, tenantID int64) context.Context`、`FromContext(ctx) int64`、`Scope(ctx) (tenantID int64, scoped bool)`。
- **测试文件**：`gorm_test.go`。

### pkg/tenant — `internal/pkg/tenant/gorm.go`
- **核心符号**
  - `var ErrMismatch` — 写入数据租户与操作上下文不一致。
  - `RegisterGORMCallbacks(db *gorm.DB) error` — 注册全局回调：Create Before 自动填充/拒绝跨租户；Query/Row/Update/Delete Before 注入 `WHERE tenant_id = ?`（Scan 走 Row 链）。
  - `tenantField(stmt) *schema.Field`、`tenantWhereScope(db)`（`clause.CurrentTable` 限定主表）、`fillTenantOnCreate(db)`、`setTenantIfZero(...)`。

### pkg/lock — `internal/pkg/lock/lock.go`
- **核心符号**
  - 包注释：提供带持有者校验的 Redis 分布式锁。
  - `type Locker struct`、`New(rdb *redis.Client) *Locker`、`unlockScript`（Lua 校验持有者后释放，防误删他人锁）。
  - `Lock(ctx, key string, ttl time.Duration) (release func(), ok bool, err error)` — `SET NX EX` 加锁。

### pkg/tx — `internal/pkg/tx/tx.go`
- **核心符号**
  - 包注释：提供事务重试、冲突分类和事务错误处理。
  - `const ( MaxOrderNoRetry / MaxTxRetry / retryBackoff ... )`。
  - `type Manager struct`、`New(db *gorm.DB) *Manager`、`DB() *gorm.DB`（非事务连接）。
  - `Tx(ctx, fn func(tx *gorm.DB) error) error` — 事务内执行，panic/error 回滚。
  - `TxRetry(ctx, maxAttempts int, fn func(tx *gorm.DB) error) error` — 并发冲突/死锁自动用新事务重试。
  - `IsRetryable(err error) bool`（errcode.IsConflict 或 MySQL 1213；1205 不重试）、`IsDuplicateErr(err error) bool`（唯一索引冲突兜底）。
- **测试文件**：`tx_test.go`。

### pkg/jwt — `internal/pkg/jwt/jwt.go`
- **核心符号**
  - 包注释：签发和严格校验用户身份 Token。
  - `type Claims struct{...}` — 已认证身份（`uid` / `username` / `ver` / `tid`）。
  - `var ErrInvalidToken`。
  - `Generate(secret string, expire time.Duration, userID int64, username string, tokenVersion int, tenantID int64) (string, error)`。
  - `Parse(secret, tokenStr string) (*Claims, error)` — 校验签名与必需 claim。
- **测试文件**：`jwt_test.go`。

### pkg/middleware — `internal/pkg/middleware/auth.go`
- **核心符号**
  - `type AuthValidator interface` — 签名校验后复核用户状态和 Token 版本（禁用/改密后旧 Token 立即失效）。
  - `Auth(secret string, validator AuthValidator) gin.HandlerFunc` — 解析 Bearer Token、复核状态、注入 userID/username（与租户）。
  - `type PermsChecker interface`（由 system 实现）、`Permission(checker PermsChecker, perm string) gin.HandlerFunc`。
- **测试文件**：`api_key_test.go`（同目录）。

### pkg/middleware — `internal/pkg/middleware/api_key.go`
- **核心符号**：`APIKey(expected string, tenantID int64) gin.HandlerFunc` — 校验外部系统集成的 `X-API-Key`（以 `WithExactTenant` 限定租户）。
- **测试文件**：`api_key_test.go`。

### pkg/middleware — `internal/pkg/middleware/demo.go`
- **核心符号**：`type DemoSessionValidator interface`（由 demo 模块实现）、`DemoSession(validator DemoSessionValidator) gin.HandlerFunc` — 限制每个演示账号同时只有一个有效会话（按租户隔离）。
- **测试文件**：`demo_test.go`。

### pkg/middleware — `internal/pkg/middleware/request_context.go`
- **核心符号**：`type ctxKey string`；`const ( ctxRequestID / ctxUserID / ctxUsername / ctxTenantID )`；`RequestID() gin.HandlerFunc`；`UserID/Username/RequestIDOf/TenantIDOf(c *gin.Context)`；`normalizeRequestID(id string) string`、`newRequestID() string`。

### pkg/middleware — `internal/pkg/middleware/requestid.go`
- **核心符号**：`randHex() string` — 随机十六进制请求 ID 片段。

### pkg/middleware — `internal/pkg/middleware/operlog.go`
- **核心符号**：`bodyWriter`（捕获响应体）；`OperLogRecord`、`OperLogRecorder`（由 system 实现，异步写库）；`BodyLimit(limitMB int64) gin.HandlerFunc`；`OperLog(rec OperLogRecorder) gin.HandlerFunc`。

### pkg/middleware — `internal/pkg/middleware/audit.go`
- **核心符号**：`const auditParamLimit = 2048`、`var sensitiveKeys`；`sanitizeOperLogParams(contentType, body string) string`、`redactSensitive(value any) any`、`redactRawParams(raw string) string`、`isSensitiveKey(key string) bool`、`truncateUTF8(value string, maxBytes int) string`。
- **测试文件**：`audit_test.go`。

### pkg/middleware — `internal/pkg/middleware/access_log.go`
- **核心符号**：`AccessLog() gin.HandlerFunc` — 方法/路径/状态/耗时/操作人，>500ms 打 warn。

### pkg/middleware — `internal/pkg/middleware/recovery.go`
- **核心符号**：`Recovery() gin.HandlerFunc` — panic 兜底，防止进程退出。

### pkg/middleware — `internal/pkg/middleware/cors.go`
- **核心符号**：`CORS(allowedOrigins []string) gin.HandlerFunc` — 只允许配置中的精确 Origin，避免通配符。

### pkg/middleware — `internal/pkg/middleware/middleware.go`
- **核心符号**：包注释 — 提供请求身份、审计、恢复和跨域等 HTTP 中间件。

### pkg/response — `internal/pkg/response/response.go`
- **核心符号**：`Body`（`{code,msg,data}`）、`pageData`；`OK`、`OKPage`、`Fail`（归一化错误，HTTP 状态优先用错误模板声明的 `HTTPStatus`）、`FailWithData`（保留业务码并回传结构化数据，用于 Demo 场景失败回传已执行步骤）、`fail`、`httpStatus(bizErr *errcode.Error) int`。
- **测试文件**：`response_test.go`。

### pkg/errcode — `internal/pkg/errcode/errcode.go`
- **核心符号**：`type Error struct`（Code/Msg + 底层 cause）；`Error() / Unwrap()`；`New`、`NewHTTP`、`Wrap`、`From`；`var conflictCodes`、`IsConflict(err)`、`IsConflictCode(code)`；分区错误码变量（通用、系统/认证 10000+、基础资料 20000+、库存 30000+、入库 40000+、出库 50000+、盘点 60000+、演示 70000+、AI 80000+、数据量配额 90000+）。
- **测试文件**：`errcode_test.go`。

### pkg/httpx — `internal/pkg/httpx/params.go`
- **核心符号**：`PathID`、`QueryID`、`ID`、`OptionalQueryID`、`QueryInt`、`IsBodyTooLarge`、`BindJSON`、`FailParam` — 统一请求参数解析/校验，失败时已写响应。
- **测试文件**：`params_test.go`。

### pkg/quota — `internal/pkg/quota/quota.go`
- **核心符号**
  - 包注释：公开租户数据量配额（仅 `tenant_id > 0` 生效，`limits <= 0` 不限）。
  - `Guard(ctx, db *gorm.DB, model any, limit, add int, label string) error` — 校验"已有 + 新增"不超上限（软配额，非严格额度）。
  - `GuardImportRows(ctx, rows, limit int) error` — 单次批量导入行数上限。
- **测试文件**：`quota_test.go`。

### pkg/snowflake — `internal/pkg/snowflake/snowflake.go`
- **核心符号**：`const`（41 位时间戳 + 10 位节点 + 12 位序列）；`type generator struct`、`var defaultGenerator`；`Init(node int64) error`（多实例必须不同节点号）；`Next() int64`；`(*generator).nextAt(now int64) int64`。
- **测试文件**：`snowflake_test.go`。

### pkg/orderno — `internal/pkg/orderno/orderno.go`
- **核心符号**：`type Generator struct`（主方案 Redis Lua INCR + TTL；降级日期+F+UUID）；`New(rdb redis.UniversalClient) *Generator`（rdb 可为 nil）；`const ttlSeconds = 48*3600`；`var luaScript`；`Next(ctx, prefix string) string`。
- **测试文件**：`orderno_test.go`。

### pkg/modelbase — `internal/pkg/modelbase/model.go`
- **核心符号**：`type Base struct`（业务表通用字段）；`type Versioned struct`（增加内部修订号，是否乐观锁取决于具体 SQL）。

### pkg/dbutil — `internal/pkg/dbutil/like.go`
- **核心符号**：`LikePattern(s string) string` — 转义用户输入的 LIKE 通配符（`%`、`_`、反斜杠），配合 MySQL 默认 `\` 转义。

### pkg/observability — `internal/pkg/observability/metrics.go`
- **核心符号**：`type Metrics struct`（registry + requestsTotal/requestSeconds/inFlight）；`New(db *gorm.DB, serviceName string) *Metrics`（注册 Go/Process collector、DB collector、build_info）；`Registry()`、`Handler()`、`Middleware()`；`type dbCollector struct`、`newDBCollector`、`Describe`、`Collect`。
- **测试文件**：`metrics_test.go`。

### pkg/log — `internal/pkg/log/log.go`
- **核心符号**：`type ctxKey string`、`const (...)`；`var logger`；`Init(level string)`、`L() *slog.Logger`、`WithContext(ctx) *slog.Logger`（提取 request_id/user_id）、`WithRequestID`、`WithUserID`。

### pkg/concurrent — `internal/pkg/concurrent/safego.go`
- **核心符号**：`SafeGo(ctx context.Context, fn func())`、`SafeGoNoCtx(fn func())` — 包装 `defer recover()` + 日志，避免后台任务 panic 拖垮服务。

### pkg/typex — `internal/pkg/typex/idlist.go`
- **核心符号**：`type Int64List []int64` — JSON 用字符串数组表示 int64 ID 防 JS 精度丢失；`MarshalJSON`、`UnmarshalJSON`（兼容字符串/数字数组）。
- **测试文件**：`idlist_test.go`。

### pkg/version — `internal/pkg/version/version.go`
- **核心符号**：`var ( Version = "dev"; Commit = "unknown"; BuildTime = "unknown" )`；`String() string` — 摘要，适合健康检查和日志。

### pkg/config — `internal/pkg/config/config_test.go`
- **核心符号**：`func TestLoadRejectsUnsafeReleaseConfig`、`TestLoadRejectsInvalidNode`、`TestLoadNodeZeroAndNegative`、`TestLoadReadsEnvironmentOverrides`、`TestLoadValidConfig`、`TestLoadRejectsDevDSNInRelease`、`TestLoadRejectsDevAPIKeyInRelease`。
- **测试文件**：`config_test.go`（本文件）。**涉及表/模型**：无。

### pkg/errcode — `internal/pkg/errcode/errcode_test.go`
- **核心符号**：`func TestFromRecognizesWrappedBusinessError`、`TestIsConflictRecognizesWrappedError`、`TestWrapPreservesCause`、`TestWrapPreservesHTTPStatus`。
- **测试文件**：`errcode_test.go`（本文件）。**涉及表/模型**：无。

### pkg/httpx — `internal/pkg/httpx/params_test.go`
- **核心符号**：`func TestID`、`TestQueryInt`。
- **测试文件**：`params_test.go`（本文件）。**涉及表/模型**：无。

### pkg/jwt — `internal/pkg/jwt/jwt_test.go`
- **核心符号**：`func TestGenerateAndParseTokenVersion`、`TestParseRejectsInvalidClaims`、`TestParseRejectsWrongSecretAndExpiredToken`。
- **测试文件**：`jwt_test.go`（本文件）。**涉及表/模型**：无。

### pkg/middleware — `internal/pkg/middleware/api_key_test.go`
- **核心符号**：`func TestAPIKey(t *testing.T)`。
- **测试文件**：`api_key_test.go`（本文件）。**涉及表/模型**：无。

### pkg/middleware — `internal/pkg/middleware/audit_test.go`
- **核心符号**：`func TestSanitizeOperLogParamsRedactsSensitiveJSON`、`TestOperLogPreservesTenantAndRedactsQuery`、`TestOperLogRejectsTruncatedRequestBody`、`TestMalformedAuditInputDoesNotLeakSecrets`、`TestOperLogLargeResponseDoesNotChangeHTTPBody`、`TestSanitizeOperLogParamsRedactsRawQuery`、`TestTruncateUTF8DoesNotBreakCharacters`、`TestCORSAllowsOnlyConfiguredOrigin`、`TestNormalizeRequestID`。
- **测试文件**：`audit_test.go`（本文件）。**涉及表/模型**：无。

### pkg/middleware — `internal/pkg/middleware/demo_test.go`
- **核心符号**：`func TestDemoSessionRequiresSessionForDemoUser`、`TestDemoSessionAllowsAcquireWithoutSessionHeader`、`TestDemoSessionPassesValidSession`、`TestDemoSessionLeavesNonDemoUsersUntouched`、`TestDemoSessionLeavesDisabledDemoUntouched`、`TestDemoSessionMapsInvalidSessionError`。
- **测试文件**：`demo_test.go`（本文件）。**涉及表/模型**：无。

### pkg/observability — `internal/pkg/observability/metrics_test.go`
- **核心符号**：`func TestMetricsMiddlewareRecordsRoute(t *testing.T)`。
- **测试文件**：`metrics_test.go`（本文件）。**涉及表/模型**：无。

### pkg/orderno — `internal/pkg/orderno/orderno_test.go`
- **核心符号**：`func TestFallbackAcrossGenerators`、`TestRedisSequenceAndTTLRepair`。
- **测试文件**：`orderno_test.go`（本文件）。**涉及表/模型**：无。

### pkg/quota — `internal/pkg/quota/quota_test.go`
- **核心符号**：`func TestGuardBlocksWhenLimitReached`、`TestGuardCountsWithinTenantOnly`、`TestGuardSkipsPlatformAndUnlimited`、`TestGuardImportRows`。
- **测试文件**：`quota_test.go`（本文件）。**涉及表/模型**：待确认（测试内定义模型）。

### pkg/response — `internal/pkg/response/response_test.go`
- **核心符号**：`func TestHTTPStatus`、`TestFailWithDataKeepsBusinessErrorAndPayload`。
- **测试文件**：`response_test.go`（本文件）。**涉及表/模型**：无。

### pkg/snowflake — `internal/pkg/snowflake/snowflake_test.go`
- **核心符号**：`func TestClockRollbackDoesNotReuseIDs`、`TestConcurrentGenerationAtSameMillisecond`、`TestDifferentNodesAndInitialization`。
- **测试文件**：`snowflake_test.go`（本文件）。**涉及表/模型**：无。

### pkg/tenant — `internal/pkg/tenant/gorm_test.go`
- **核心符号**：`func TestTenantIsolation`、`TestTenantScanIsolation`、`TestWithExactTenantZeroDoesNotBypass`、`TestScope`、`TestTenantCreatePointerSliceAndBatches`、`TestFromContext`。
- **测试文件**：`gorm_test.go`（本文件）。**涉及表/模型**：待确认（测试内定义带 tenant_id 的模型）。

### pkg/tx — `internal/pkg/tx/tx_test.go`
- **核心符号**：`func TestErrorClassification`、`TestTxRetryRejectsInvalidAttemptsAndCanceledContext`。
- **测试文件**：`tx_test.go`（本文件）。**涉及表/模型**：无。

### pkg/typex — `internal/pkg/typex/idlist_test.go`
- **核心符号**：`func TestInt64ListJSON`、`TestInt64ListAcceptsLegacyNumbers`。
- **测试文件**：`idlist_test.go`（本文件）。**涉及表/模型**：无。

---

### testutil — `internal/testutil/mysql.go`
- **核心符号**
  - `OpenIsolatedMySQL(t *testing.T, baseDSN string, models ...any) *gorm.DB` — 建临时 schema、只迁移指定模型、结束时 drop，绝不写入开发者工作库；`WMS_TEST_REQUIRED=1` 时缺失/不可用则 fail。
  - `parseTestDSN(t *testing.T, baseDSN string) *mysqlDriver.Config` — 强制 `ParseTime=true`，统一时间语义。
  - `IsolatedDatabaseName(dsn string) string`。
- **测试文件**：`mysql_test.go`。

### testutil — `internal/testutil/mysql_test.go`
- **核心符号**：`func TestParseTestDSNForcesTimeParsing(t *testing.T)` — 确认测试 DSN 强制开启时间解析。
- **测试文件**：`mysql_test.go`（本文件）。**涉及表/模型**：无。

### testutil — `internal/testutil/redis.go`
- **核心符号**：`OpenTestRedis(t *testing.T) *redis.Client` — 返回隔离 Redis 客户端；`WMS_TEST_REQUIRED=1` 时不可用则 fail，否则 skip。

---

### migrations — `migrations/embed.go`
- **核心符号**：`var FS embed.FS`（`//go:embed versions/*.sql`）— 版本化 SQL 迁移嵌入应用二进制。

### migrations — `migrations/migrations_test.go`
- **核心符号**：`func TestMigrationsAndImportTokenRollback(t *testing.T)` — 校验 up/回滚/重放、索引列顺序（如 `idx_inv_tenant_fifo` = `tenant_id,warehouse_id,sku_id,available_quantity,stock_in_time`）与 CHECK 约束（`chk_inv_allocated_non_negative`、`chk_inv_quantity_balance`）；辅助 `indexColumns`、`hasCheckConstraint`。
- **主要调用关系**：使用 `migrations.FS` + `testutil.OpenIsolatedMySQL`。
- **测试文件**：`migrations_test.go`（本文件）。**涉及表/模型**：`wms_import_task`、`wms_inventory`、`wms_location`。

### migrations 000001 — `migrations/versions/000001_init.up.sql`
- **核心符号**：初始化迁移脚本（参考；实际以 AutoMigrate 为准，用于人工审阅与生产 DBA 评审）。建系统表 `sys_user`、`sys_role`、`sys_user_role`、`sys_oper_log`；基础资料 `wms_warehouse`、`wms_location`、`wms_sku`；库存 `wms_inventory`、`wms_inventory_trans`；统一任务 `wms_task`；入库 `wms_receipt_order`、`wms_receipt_order_detail`、`wms_import_task`；出库 `wms_shipment_order`、`wms_shipment_order_detail`、`wms_allocation`；盘点 `wms_stocktake_order`、`wms_stocktake_detail`。
- **涉及表/模型**：上述全部表。

### migrations 000001 — `migrations/versions/000001_init.down.sql`
- **核心符号**：`SET FOREIGN_KEY_CHECKS = 0` 后按依赖倒序 `DROP TABLE IF EXISTS` 全部业务表，最后恢复 `FOREIGN_KEY_CHECKS = 1`。

### migrations 000002 — `migrations/versions/000002_add_inventory_version.up.sql`
- **核心符号**：为 `wms_inventory` 补充乐观锁版本列 `version INT NOT NULL DEFAULT 1`（000001 建表脚本遗漏）。
- **涉及表/模型**：`wms_inventory`。

### migrations 000002 — `migrations/versions/000002_add_inventory_version.down.sql`
- **核心符号**：`ALTER TABLE wms_inventory DROP COLUMN version;`。

### migrations 000003 — `migrations/versions/000003_add_task_pick_location.up.sql`
- **核心符号**：拣货任务补作业位置 `location_id`/`location_code`/`batch_no` 及索引 `idx_task_location`（拣货员直达库位并按批次核对）。
- **涉及表/模型**：`wms_task`。

### migrations 000003 — `migrations/versions/000003_add_task_pick_location.down.sql`
- **核心符号**：删除 `idx_task_location`、`batch_no`、`location_code`、`location_id`。

### migrations 000004 — `migrations/versions/000004_add_core_indexes.up.sql`
- **核心符号**：纯加法补建核心查询索引（消除高频路径全表扫描）：`idx_inv_fifo`、`idx_inv_location`、`idx_task_order_type_status`、`idx_ro_wh_status`、`idx_so_wh_status`、`idx_sto_wh_status`、`idx_alloc_order_status`、`uk_loc_wh_code`、`idx_alloc_detail`、`idx_task_detail`、`idx_task_allocation`、`idx_task_sku`、`idx_import_stale`、`idx_oper_log_username`、`idx_user_role_role`、`idx_trans_inv_type`（并删除低区分度 `idx_trans_type`、`idx_task_type`、`idx_task_status`）。
- **涉及表/模型**：`wms_inventory`、`wms_task`、`wms_receipt_order`、`wms_shipment_order`、`wms_stocktake_order`、`wms_allocation`、`wms_location`、`wms_import_task`、`sys_oper_log`、`sys_user_role`、`wms_inventory_trans`。

### migrations 000004 — `migrations/versions/000004_add_core_indexes.down.sql`
- **核心符号**：恢复原索引结构（重建被删冗余单列索引，删除本次新增索引）。

### migrations 000005 — `migrations/versions/000005_multitenant.up.sql`
- **核心符号**：多租户最小版（共享库加列）：18 张表新增 `tenant_id BIGINT NOT NULL DEFAULT 0`；唯一键改为 `tenant_id` 前置的联合唯一；高频查询表补 `tenant_id` 普通索引。
- **涉及表/模型**：全部业务表（`sys_user`、`sys_role`、`sys_user_role`、`sys_oper_log`、`wms_*` 共 18 张）。

### migrations 000005 — `migrations/versions/000005_multitenant.down.sql`
- **核心符号**：回滚多租户列与联合唯一键（需先保证各业务编码全局唯一，否则会失败）。

### migrations 000006 — `migrations/versions/000006_import_run_token.up.sql`
- **核心符号**：为 `wms_import_task` 增加 `run_token`（每次领取导入任务使用独立执行标识，防旧 worker 覆盖重跑结果）。
- **涉及表/模型**：`wms_import_task`。

### migrations 000006 — `migrations/versions/000006_import_run_token.down.sql`
- **核心符号**：`ALTER TABLE wms_import_task DROP COLUMN run_token;`。

### migrations 000007 — `migrations/versions/000007_tenant_index_alignment.up.sql`
- **核心符号**：统一多租户查询索引，把 `tenant_id` 前置到高频复合/唯一索引（仅调索引，不动数据）：`uk_loc_wh_code`、`uk_user_role`、`idx_inv_tenant_fifo`、`idx_inv_tenant_location`、`idx_inv_tenant_sku`、`idx_task_tenant_*`、`idx_ro/so/sto_tenant_wh_status`、`idx_rod/sod/std_tenant_*`、`idx_alloc_tenant_*`、`idx_trans_tenant_inv_type`、`idx_oper_log_tenant_username`、`idx_user_role_tenant_role`。
- **涉及表/模型**：`wms_location`、`sys_user_role`、`wms_inventory`、`wms_task`、`wms_receipt_order*`、`wms_shipment_order*`、`wms_allocation`、`wms_stocktake_order`/`wms_stocktake_detail`、`wms_inventory_trans`、`sys_oper_log`。
- **测试文件**：由 `migrations_test.go` 校验（含索引列顺序 `["tenant_id","warehouse_id","sku_id","available_quantity","stock_in_time"]`）。

### migrations 000007 — `migrations/versions/000007_tenant_index_alignment.down.sql`
- **核心符号**：恢复多租户改造前的索引结构（只回滚索引，不删 `tenant_id` 列或数据）。

### migrations 000008 — `migrations/versions/000008_inventory_invariants.up.sql`
- **核心符号**：新增库存不变量 CHECK 约束 `chk_inv_allocated_non_negative`（`allocated_quantity >= 0`）、`chk_inv_quantity_balance`（`stock_quantity = available_quantity + allocated_quantity`）；已有违规数据会使迁移 fail-fast。
- **涉及表/模型**：`wms_inventory`。

### migrations 000008 — `migrations/versions/000008_inventory_invariants.down.sql`
- **核心符号**：`ALTER TABLE wms_inventory DROP CHECK chk_inv_quantity_balance, DROP CHECK chk_inv_allocated_non_negative;`。

### migrations 000009 — `migrations/versions/000009_idempotency.up.sql`
- **核心符号**：新建请求级幂等表 `wms_idempotency`（`tenant_id`/`scope`/`idempotency_key`/`request_hash`/`object_id`/`result_json`/`created_at`，唯一键 `uk_idem_tenant_scope_key`）。
- **涉及表/模型**：`wms_idempotency`。
- **测试文件**：由 `migrations_test.go` 校验建表与唯一键列顺序。

### migrations 000009 — `migrations/versions/000009_idempotency.down.sql`
- **核心符号**：`DROP TABLE IF EXISTS wms_idempotency;`。

### migrations 000010 — `migrations/versions/000010_task_pick_lease.up.sql`
- **核心符号**：`wms_task` 增加拣货作业租约列 `claimed_by`（持有人）、`claim_token`（领取凭证）、`lease_expire_at`（DATETIME(3) NULL，惰性过期）。
- **涉及表/模型**：`wms_task`。
- **测试文件**：由 `migrations_test.go` 校验列存在与 down/up 往返。

### migrations 000010 — `migrations/versions/000010_task_pick_lease.down.sql`
- **核心符号**：`ALTER TABLE wms_task DROP COLUMN lease_expire_at, DROP COLUMN claim_token, DROP COLUMN claimed_by;`。

---

### 本部分重点专题

### 专题 1：鉴权
- **JWT Claims 字段**：`internal/pkg/jwt/jwt.go` 的 `type Claims`（`uid` / `username` / `ver` / `tid`）。
- **生成/解析**：`jwt.Generate(secret, expire, userID, username, tokenVersion, tenantID)`、`jwt.Parse(secret, tokenStr)`；`ErrInvalidToken`。
- **auth 中间件**：`internal/pkg/middleware/auth.go` 的 `Auth(secret string, validator AuthValidator)`、`Permission(checker PermsChecker, perm string)`；`AuthValidator.ValidateToken`、`PermsChecker.HasPerm` 由 `internal/modules/system/service` 实现（`auth.go:ValidateToken`、`permission.go:HasPerm`）。
- **API Key 中间件**：`internal/pkg/middleware/api_key.go` 的 `APIKey(expected string, tenantID int64)`（校验 `X-API-Key`，经 `tenant.WithExactTenant` 限定租户）。
- **登录限流**：`internal/modules/system/service/login_limiter.go` 的 `beginLoginAttempt` / `clearLoginAttempts`，参数 `maxLoginAttempts=5`、`loginFailureWindow=15*time.Minute`（进程内 `loginAttempts` map）。
- **密码哈希**：`internal/modules/system/service/password.go` 的 `hashPassword` / `checkPassword`（bcrypt）。
- **登录主流程**：`internal/modules/system/service/auth.go` 的 `Login`（限流预占 → 查用户 → bcrypt 校验 → `jwt.Generate`）。

### 专题 2：多租户
- **上下文注入与过滤**：`internal/pkg/tenant/tenant.go` 的 `WithTenant` / `WithExactTenant` / `FromContext` / `Scope`；`internal/pkg/tenant/gorm.go` 的 `RegisterGORMCallbacks`、`tenantWhereScope`、`fillTenantOnCreate`、`tenantField`、`setTenantIfZero`、`ErrMismatch`（在 `InitDB` 中注册）。
- **请求上下文中间件**：`internal/pkg/middleware/request_context.go` 的 `ctxTenantID` 与 `TenantIDOf(c *gin.Context)`；租户 ID 来自 JWT claims（`tid`），由 `Auth` 中间件写入 ctx。
- **平台旁路 tenant_id=0**：`tenant.FromContext` 未设置或为 0 时 `scoped=false`（平台旁路，不注入隔离条件）；`WithExactTenant(ctx, 0)` 则 `scoped=true` 且只能访问平台租户数据。
- **配套**：`internal/pkg/quota/quota.go` 的 `Guard`（仅 `tenant_id > 0` 生效）；`migrations 000005/000007` 完成 `tenant_id` 列与租户前置索引。

### 专题 3：数据库基础设施
- **InitDB**：`internal/bootstrap/database.go` 的 `InitDB`（GORM MySQL + 注册多租户全局回调）；`InitRedis`、`AutoMigrate`（含 CHECK 约束）。
- **Seed 系列**：`internal/bootstrap/seed.go:SeedDemo`（演示基础数据 + 体验账号，幂等）、`seed_admin.go:SeedAdmin`（内置管理员，密码取 `WMS_ADMIN_PASSWORD`，debug 默认 admin123）、`seed_demo.go:seedDemoData`（仓库/库位/SKU/库存/流水/演示单据，仅当无仓库时执行）、`demo_accounts.go:SeedDemoAccounts`（demo1..demoN）、`personal_accounts.go:SeedPersonalAccounts`（user1..userN，含 `seedPersonalData`）、`reset.go:ResetDemoData`（按租户硬删并重种，`tenantID <= 0` 拒绝执行）。
- **迁移机制**：`migrations/embed.go` 的 `FS embed.FS`（`//go:embed versions/*.sql`）；`migrations/versions` 下 000001~000010 各版本 up/down（初始化建表 → 库存版本列 → 任务拣货位置 → 核心索引 → 多租户加列 → 导入 run_token → 租户索引对齐 → 库存 CHECK 不变量 → 请求幂等表 → 拣货任务租约列）。
- **应用迁移入口**：`cmd/migrate/main.go`（golang-migrate + iofs 源 + MySQL driver，`run` / `seedBootstrapAdmin` / `seedDemo` / `withDB` / `waitForMySQL` / `migrationDSN`）+ `app_user.go:ensureAppUser`（幂等创建/修复应用账户，已有数据卷升级补齐）。
- **测试**：`migrations/migrations_test.go` 的 `TestMigrationsAndImportTokenRollback`。

### 专题 4：Demo 演示模块
- **demo1..N 账号**：`internal/bootstrap/demo_accounts.go` 的 `SeedDemoAccounts` / `demoPerms` / `managedDemoTenantIDs`（租户 10001+）；账号命名与租户换算见 `internal/pkg/config/types.go` 的 `DemoConfig.AccountUsername` / `AccountTenantID` / `AccountIndex`、`demoTenantIDBase`。
- **会话锁与租约 TTL**：`internal/modules/demo/service/session.go` 的 `activeSessionKeyOf`（service.go）、`AcquireSession` / `Heartbeat` / `ReleaseSession` / `Reset` / `SessionStatus`、`renewSession` / `deleteSession` / `resetLocked` / `beginTenantRun`、Lua 脚本 `renewSessionScript` / `deleteSessionScript` / `sessionTTLScript`、`demoExecutionTTL = 10 * time.Minute`；中间件 `internal/pkg/middleware/demo.go:DemoSession`（`DemoSessionValidator`）。
- **/demo 路由开关**：`internal/modules/demo/service/service.go:Enabled()`（`cfg.Demo.Enabled && rdb != nil`）；`handler.go:RegisterPublicRoutes`（未启用则不挂载）。
- **scenario 场景**：`internal/modules/demo/service/scenario.go`（`ScenarioInbound`/`ScenarioOutbound`/`ScenarioStocktake`/`ScenarioFull`、`RunScenario`、`scenarioRun`）+ `scenario_inbound.go`/`scenario_outbound.go`/`scenario_stocktake.go`/`drafts.go`。
- **experiments 实验**：`experiment_allocation.go`（`RunConcurrentAllocation`）、`experiment_picking.go`（`RunConcurrentPicking`）、`experiment_shortage.go`（`RunConcurrentShortageValidation`）、`experiment_restock.go`（`RestockDemo`）。

### 专题 5：分布式锁与事务
- **分布式锁**：`internal/pkg/lock/lock.go` 的 `Locker` / `New` / `Lock`（`SET NX EX` + Lua `unlockScript` 校验持有者释放）。
- **事务**：`internal/pkg/tx/tx.go` 的 `Manager` / `New` / `DB` / `Tx` / `TxRetry`（死锁 1213 重试、`retryBackoff` 退避）、`IsRetryable`、`IsDuplicateErr`；配合 `internal/pkg/errcode/errcode.go` 的 `IsConflict` / `conflictCodes`。
- **请求幂等**：`internal/pkg/idempotency/idempotency.go` 的 `Record`（表 `wms_idempotency`）、`Find`（显式租户查询）、`Insert`（唯一键冲突映射 `errcode.Conflict` 交 `TxRetry` 重试）、`Fingerprint`；使用约定：Find → 业务写入 → 同事务 Insert，重试命中回放 `ResultJSON`。
- **并发安全 goroutine**：`internal/pkg/concurrent/safego.go` 的 `SafeGo` / `SafeGoNoCtx`（panic 恢复）。

### 专题 6：配置与启动
- **config.Load**：`internal/pkg/config/config.go` 的 `Load(path string)`（viper，`WMS_` 前缀环境变量覆盖）；结构体见 `internal/pkg/config/types.go` 的 `Config` 及各子配置。
- **.env 加载**：`cmd/wms/dotenv.go` 的 `loadDotEnv(path string)`（已有环境变量优先，不输出密钥值）。
- **启动与优雅关停**：`cmd/wms/main.go` 的 `main` / `run`（loadDotEnv → `config.Load` → `log.Init` → `snowflake.Init` → `InitDB` → `Migrate`/`Seed` → `InitRedis` → `app.New` → `serve`）；`cmd/wms/server.go` 的 `serve(ctx, shutdownTimeout, servers...)` 在收到退出信号或任一监听失败时优雅关闭全部 HTTP 服务；`internal/app/router.go` 的 `NewRouter` 组装中间件链、`healthz` 健康检查。

## 三、前端

> 范围：`web/src` 全量源码（API、状态、路由、组合式函数、Guide/Demo 外挂层、事件、工具、视图、组件）。
> 技术栈：Vue 3 `<script setup>` + TypeScript + Pinia + Vue Router 4 + Element Plus + Vite（axios）。
> 说明：本文档只读索引，不臆造；无法从代码确认处标注“待确认”。

---

### api — api/request.ts

- **核心导出/声明**：
  - `type ApiErrorKind = 'auth' | 'demo' | 'business' | 'network'` — 统一错误分类。
  - `class ApiError extends Error { code?; status?; data?; kind }` — 携带业务码/HTTP 状态/分类的自定义错误。
  - `interface RequestOptions { silentError?: boolean }` — 静默错误开关。
  - `get<T>(url, params?, options?)` / `post<T>` / `put<T>` / `del<T>` / `upload<T>` — 各方法的业务层封装（响应拦截器已剥离 `data`）。
- **内部逻辑**：`classifyError(status, code)`（undefined→network；401→auth；70002/70003/70005/70006→demo；其余→business）；请求拦截器注入 `Authorization: Bearer` 与 `X-Demo-Session`；响应拦截器 code!=0 时按 `silentError` 决定是否弹错；`handleDemoSessionExpired()` 用 `demoSessionRedirecting` 防重复跳转，1.5s 后复位。
- **依赖/调用关系**：被所有 `api/*.ts` 依赖；被 stores/views/composables 间接使用（经各 api 函数）。
- **测试文件**：无独立 spec（逻辑由 api 调用方与 E2E 覆盖）。

### api — api/auth.ts

- **核心导出/声明**：`login(data)`、`getProfile()`、`changePassword(data)`。
- **依赖/调用关系**：依赖 `request`；被 `stores/auth`、`views/login`、`layouts/Layout` 调用。
- **测试文件**：无。

### api — api/basic.ts

- **核心导出/声明**：仓库 CRUD（`listWarehouses`/`createWarehouse`/`updateWarehouse`/`deleteWarehouse`/`updateWarehouseStatus`）、库位（`listLocations`/`batchCreateLocations`/`deleteLocation`/`updateLocationStatus`）、货品（`listSkus`/`createSku`/`updateSku`/`deleteSku`/`getSkuByBarcode`）。
- **依赖/调用关系**：被 `views/basic/*`、`utils/options.ts` 调用。
- **测试文件**：无。

### api — api/inbound.ts

- **核心声明**：入库单列表/详情/创建/更新/提交/审核/取消/删除、收货 `receiveInbound`、上架 `putawayInboundTask`、Excel 导入（`importInboundOrders`/`getImportStatus` 等）、批量操作、按批次删除。
- **依赖/调用关系**：被 `composables/inbound/*`、`views/inbound/*`、`components/ReceiveDialog|PutawayDialog`、`views/dashboard` 调用。
- **测试文件**：无（其消费方 `useInboundImport.spec.ts` 覆盖导入轮询）。

### api — api/outbound.ts

- **核心声明**：出库单列表/详情/创建/提交/审核（分配）/取消/删除、拣货 `pickOutboundTask`、批量操作。
- **依赖/调用关系**：被 `views/outbound/*`、`components/PickDialog`、`views/dashboard` 调用。
- **测试文件**：无。

### api — api/inventory.ts

- **核心声明**：`listInventory`、`listInventorySummary`、`listInventoryTrans`。
- **依赖/调用关系**：被 `views/inventory/Inventory.vue` 调用。
- **测试文件**：无。

### api — api/stocktake.ts

- **核心声明**：`listStocktakeOrders`、`getStocktakeOrder`、`createStocktakeOrder`、`submitStocktakeActual`、`approveStocktakeOrder`、`cancelStocktakeOrder`。
- **依赖/调用关系**：被 `views/stocktake/*` 调用。
- **测试文件**：无。

### api — api/task.ts

- **核心声明**：`listTasks`、`getTask`。
- **依赖/调用关系**：被 `views/task/Tasks.vue`、`views/dashboard`、`components/PickDialog|PutawayDialog` 间接调用。
- **测试文件**：无。

### api — api/system.ts

- **核心声明**：用户 CRUD（`listUsers`/`createUser`/`updateUser`/`deleteUser`/`updateUserStatus`/`resetUserPassword`）、角色 CRUD（`listRoles`/`createRole`/`updateRole`/`deleteRole`/`listAllRoles`）、`listOperLogs`。
- **依赖/调用关系**：被 `views/system/*` 调用。
- **测试文件**：无。

### api — api/demo.ts

- **核心声明**：`acquireDemoSession`/`claimDemoAccount`/`heartbeatDemoSession`/`releaseDemoSession`/`getDemoSessionStatus`、`runDemoScenario`/`runConcurrentDemo`/`runConcurrentShortageDemo`/`runConcurrentPicking`/`restockDemo`/`getDemoPerformance`/`getDemoActivity`/`resetDemoData`；`interface DemoAccountInfo`。
- **依赖/调用关系**：被 `composables/demo/useDemoSession|useDemoRunner`、`components/DemoConsole`、`views/login`、`views/demo/*` 调用。
- **测试文件**：无。

### api — api/personal.ts

- **核心声明**：`interface PersonalAccountInfo`、`listPersonalAccounts`、`claimPersonalAccount`。
- **依赖/调用关系**：被 `views/login/index.vue` 调用。
- **测试文件**：无。

### api — api/ai.ts

- **核心声明**：`interface AIChatResp`、`aiChat(question)`（超时 45000ms）。
- **依赖/调用关系**：被 `views/ai/index.vue` 调用。
- **测试文件**：无。

### api — api/version.ts

- **核心声明**：`getVersion()`。
- **依赖/调用关系**：被 `views/login/index.vue` 调用（`demo_enabled`/`personal_enabled` 特性开关）。
- **测试文件**：无。

### api — api/types/index.ts

- **核心声明**：聚合再导出 `common/auth/system/basic/inventory/task/inbound/outbound/stocktake/demo` 全部类型。
- **依赖/调用关系**：被约 36 个文件以 `@/api/types` 引入。
- **测试文件**：无。

### api — api/types/common.ts

- **核心声明**：`EntityID = string`（避免 JS Number 精度丢失）、`ApiResponse<T>`、`PageData<T>`、`PageQuery`、`BatchOperResult`、`OrderDetailLine`、`VersionResult`。
- **测试文件**：无。

### api — api/types/auth.ts

- **核心声明**：登录/用户资料/改密相关类型。
- **测试文件**：无。

### api — api/types/system.ts

- **核心声明**：`UserItem`/`RoleItem`/`OperLogItem` 等系统管理类型。
- **测试文件**：无。

### api — api/types/basic.ts

- **核心声明**：`WarehouseItem`/`LocationItem`/`SkuItem` 等基础资料类型。
- **测试文件**：无。

### api — api/types/inventory.ts

- **核心声明**：`InventoryItem`/`InventorySummaryItem`/`InventoryTransItem`。
- **测试文件**：无。

### api — api/types/task.ts

- **核心声明**：`TaskItem`。
- **测试文件**：无。

### api — api/types/inbound.ts

- **核心声明**：`InboundOrderItem`/`InboundOrderDetail`/`InboundOrderDetailRow` 等。
- **测试文件**：无。

### api — api/types/outbound.ts

- **核心声明**：`OutboundOrderItem`/`OutboundOrderDetail` 等。
- **测试文件**：无。

### api — api/types/stocktake.ts

- **核心声明**：`StocktakeOrderItem`/`StocktakeOrderDetail`/`StocktakeDetailItem`。
- **测试文件**：无。

### api — api/types/demo.ts

- **核心声明**：`DemoActivitySnapshot`/`DemoPerformanceSnapshot`/`DemoScenarioResult`/`DemoScenarioEvidence`/`DemoScenarioStep`/`DemoConcurrentResult`/`DemoConcurrentShortageResult`/`DemoPickingResult` 等。
- **测试文件**：无。

---

### stores — stores/auth.ts

- **核心导出**：`useAuthStore()`（Pinia setup store）。
  - 常量 `TOKEN_KEY='WMS_TOKEN'`、`USER_KEY='WMS_USER'`、`DEMO_SESSION_KEY='WMS_DEMO_SESSION'`；`interface AuthUser { user_id; username; nickname; roles: string[]; perms: string[] }`。
  - 内部函数：`parseUser`、`readAuthPair`（token+user 必须成对，否则视为损坏并清理）、`writeAuth`（先清两处再写一处）、`pickStorage(isDemo)`（演示账号→sessionStorage，普通→localStorage）。
  - state：`token`/`user`/`demoSessionId`/`demoSessionExpiresIn`。
  - getters：`isLoggedIn`(!!token)、`isDemo`(perms 含 `wms:demo`)、`displayName`、`perms`、`hasPerm`（支持 `*` 通配）。
  - actions：`setAuth`/`setProfile`/`setDemoSession`/`clearDemoSession`/`clear`。
- **依赖/调用关系**：被 `main.ts` 之外的路由守卫、Layout、各视图消费；被 `views/login` 写入。
- **测试文件**：`stores/auth.spec.ts`。

### stores — stores/guide.ts

- **核心导出**：`useGuideStore()` 及从 `@/guide/*` 重导出的类型/函数（兼容层）。
  - state：`active`/`scenario`/`currentStep`/`orderId`/`orderNo`/`taskId`/`taskNo`/`startedAt`/`completed`/`verifiedStepIds`/`inferredStepIds`/`facts`/`lastOutcome`/`mismatch`（由 `loadGuideState()` 恢复）。
  - getters：`steps`/`currentStepDefinition`/`currentStepRoute`/`currentStepNumber`/`totalSteps`/`canGoPrevious`/`canAdvance`。
  - actions：`persist`/`start`/`recordBusinessResult`/`next`/`previous`/`reposition`/`setMismatch`/`restart`/`cancel`。
- **依赖/调用关系**：`guide/businessBridge.ts` 写入；`components/DemoConsole|ManualGuide`、`views/demo/index` 消费。
- **测试文件**：`stores/guide.spec.ts`。

### stores — stores/theme.ts

- **核心导出**：`useThemeStore()`；`ThemeMode`；`STORAGE_KEY='gowms_theme'`；`init()`/`toggle()` 驱动 `html.dark`。
- **依赖/调用关系**：`main.ts` 调 `init()`；`layouts/Layout` 调 `toggle()`。
- **测试文件**：无。

---

### router — router/index.ts

- **核心导出**：`router`（Vue Router 实例）；`routes`（Layout 下约 20 条业务子路由，含 `meta.title`、部分 `meta.perm`，详情页含 `meta.activeMenu`）。
- **守卫逻辑**（`router.beforeEach`）：未登录且非 `/login` → 跳 `/login?redirect=...`；已登录访问 `/login` → 跳 `/`；`to.meta.perm` 存在且 `hasPerm` 不通过 → 跳 `/dashboard`。`router.afterEach` 设置 `document.title`。
- **依赖/调用关系**：依赖 `stores/auth`；被 `main.ts` 挂载。
- **测试文件**：无。

### 应用装配 — main.ts

- **核心逻辑**：`createApp(App)` → `use(createPinia())` → `useThemeStore().init()` → `installGuideBusinessBridge()` → `use(router)` → 注册 `permission` 指令 → 批量注册 Element Plus 图标 → `mount('#app')`。
- **测试文件**：无。

### 应用装配 — App.vue

- **核心职责**：用 `ElConfigProvider`（`zh-cn`）包裹 `<router-view />`。
- **测试文件**：无。

### 布局 — layouts/Layout.vue

- **核心职责**：主框架（侧边菜单按 `auth.hasPerm` 显隐、演示中心子菜单仅 `auth.isDemo` 可见；顶栏含主题切换、用户下拉修改密码/退出登录；`onMounted` 拉 `getProfile` 刷新 profile；退出时 `isDemo` 先 `releaseDemoSession`）。
- **依赖/调用关系**：`stores/auth`、`stores/theme`、`api/auth`、`api/demo`；挂载 `DemoConsole`、`ManualGuide`。
- **测试文件**：无。

### 指令 — directives/permission.ts

- **核心导出**：`permission: Directive<HTMLElement, string>`，`mounted`/`updated` 时按 `auth.hasPerm(permission)` 设置 `el.hidden`。
- **依赖/调用关系**：`main.ts` 注册为 `v-permission`；各视图按钮使用。
- **测试文件**：无。

### 常量 — constants.ts

- **核心导出**：`TagType`、`STATUS_TAG_MAP`/`statusTag`、`STATUS_TEXT_MAP`/`statusText`、`TASK_TYPE_TEXT`/`taskTypeText`、`COMMON_STATUS`、`LOCATION_STATUS`/`locationStatusText`/`locationStatusTag`，以及 `INBOUND_STATUS_OPTIONS`/`OUTBOUND_STATUS_OPTIONS`/`STOCKTAKE_STATUS_OPTIONS`/`TASK_STATUS_OPTIONS`/`TASK_TYPE_OPTIONS`/`TRANS_TYPE_OPTIONS`。
- **依赖/调用关系**：被几乎所有业务视图消费。
- **测试文件**：无。

---

### composables — composables/autoRefresh.ts

- **核心导出**：`useAutoRefresh(refresh, intervalMs=5000, shouldRefresh=()=>true): AutoRefreshController`；`AutoRefreshController { running; lastUpdatedAt; error; start; stop; refreshNow }`。
- **防重叠机制**：内部 `let inFlight = false`；`trigger()` 首行 `if (inFlight) return`，`shouldRefresh()` 不通过即返回，进入后 `inFlight=true`，`finally` 复位；`intervalMs=0`（`TIMER_DISABLED`）不启用定时轮询，仅响应 `onDataChanged` 数据变更事件；`onMounted(start)`/`onBeforeUnmount(stop)`。
- **依赖/调用关系**：依赖 `utils/events`；被 dashboard、Tasks、Inventory、订单页、demo Activity/Performance 消费。
- **测试文件**：`composables/autoRefresh.spec.ts`。

### composables/inbound — composables/inbound/useInboundOrders.ts

- **核心导出**：`useInboundOrders()` — 入库单列表查询编排：仓库映射、批次选项、日期区间、批量操作（`availableBatchOps`/`singleImportBatch`/`onBatchCommand`）、行操作 `onSubmit`/`onApprove`/`onCancel`/`onDelete`；提交/审核时 `emitBusinessEvent(INBOUND_ORDER_SUBMITTED/APPROVED)`。
- **依赖/调用关系**：`api/inbound`、`utils/options`、`events/businessEvents`；被 `views/inbound/InboundOrders.vue` 消费。
- **测试文件**：无。

### composables/inbound — composables/inbound/useInboundOrderForm.ts

- **核心导出**：`useInboundOrderForm(onSaved)` — 入库单创建/编辑对话框编排：`editDialog`/`editForm`/`openCreate`/`openEdit`/`addDetail`/`removeDetail`/`submitEdit`；创建成功 `emitBusinessEvent(INBOUND_ORDER_CREATED)`。
- **依赖/调用关系**：`api/inbound`、`events/businessEvents`；被 `InboundOrders.vue` 消费。
- **测试文件**：无。

### composables/inbound — composables/inbound/useInboundImport.ts

- **核心导出**：`useInboundImport(onCompleted)` — Excel 导入与状态轮询：`importDialog`/`importFile`/`importInfo`/`openImport`/`closeImport`/`startImport`/`onFileChange`/`onFileRemove`/`handleExceed`；常量 `POLL_INTERVAL_MS = 2000`。
- **状态机与 token 作废**：`let pollToken = 0`；`stopPolling()` 递增 token 并清 timer；`startPolling(taskId)` 记录本次 token，`tick()` 首行校验 `token !== pollToken` 即丢弃旧响应；串行 `setTimeout` 轮询；COMPLETED/FAILED 终止（COMPLETED 弹成功）；`onUnmounted(stopPolling)`。
- **依赖/调用关系**：`api/inbound`；被 `InboundOrders.vue` 消费。
- **测试文件**：`composables/inbound/useInboundImport.spec.ts`。

### composables/demo — composables/demo/useDemoSession.ts

- **核心导出**：`useDemoSession(options)` — 演示会话领取/心跳续期/空闲倒计时/释放；返回 `acquiring`/`exiting`/`remaining`/`idleTimeoutMinutes`/`initialize`/`release`。常量 `ACTIVITY_THROTTLE_MS=1000`、`RENEW_CHECK_INTERVAL_MS=10000`、`RENEW_MIN_INTERVAL_MS=20000`、`RENEW_URGENT_SECONDS=30`、`DEFAULT_SESSION_TTL_SECONDS=300`、`COUNTDOWN_WARNING_SECONDS=60`。
- **依赖/调用关系**：`api/demo`、`stores/auth`；被 `components/DemoConsole` 消费。
- **测试文件**：无。

### composables/demo — composables/demo/useDemoRunner.ts

- **核心导出**：`useDemoRunner(options)` — 场景执行：`scenarioRunning`/`runningScenario`/`result`/`resultDialogVisible`/`runScenario`/`clearResult`；`runFullScenario()` 拼接 inbound+outbound 并用 `mergeDemoScenarioResults` 合并；`isDemoScenarioResult` 类型守卫。
- **依赖/调用关系**：`api/demo`、`utils/demoScenario`、`utils/events`；被 `DemoConsole` 消费。
- **测试文件**：无。

### composables/demo — composables/demo/useStagedDemoRunner.ts

- **核心导出**：`useStagedDemoRunner(options)` — 分步执行状态机；导出常量 `STAGED_GROUPS`/`STAGED_EXECUTION_STEPS`（10 步）、类型 `StepKey`/`ExecutionStep`/`StepResult`；返回 `mode`/`currentIndex`/`results`/`running`/`error`/`currentStep`/`completed`/`progress`/`lastResult`/`groupStates`/`start`/`executeNext`/`validateCurrentPage`/`runAll`；`STEP_INTERVAL_MS=900`。
- **依赖/调用关系**：`api/demo`；被 `components/demo/StagedDemoRunner.vue` 消费。
- **测试文件**：无。

### composables/demo — composables/demo/useGuideTarget.ts

- **核心导出**：`useGuideTarget(options)` — DOM 高亮定位：`targetRect`/`targetElement`/`viewport`/`locateTarget`/`refreshTarget`/`clearTarget`/`resetLocatedStep`；`MAX_LOCATE_ATTEMPTS=30`。
- **依赖/调用关系**：被 `useGuideRunner` 使用。
- **测试文件**：无。

### composables/demo — composables/demo/useGuideRunner.ts

- **核心导出**：`useGuideRunner()` — Guide Overlay 生命周期：`visible`/`contentVisible`/`step`/`scenarioLabel`/`targetRect`/`restartGuide`/`skipGuide`/`exitGuide`/`repositionGuide`/`completeNavigate`/`completedOrderPath`/`manualEvidencePath`；用 `MutationObserver` 监测业务弹层。
- **依赖/调用关系**：`stores/guide`、`useGuideTarget`、`guide/routeMatcher`；被 `components/demo/ManualGuide.vue` 消费。
- **测试文件**：无。

---

### guide — guide/types.ts

- **核心导出**：`GuideScenario = 'inbound' | 'outbound' | 'stocktake'`、`GuideStep`、`GuideFact`、`GuideBusinessResult`。
- **依赖/调用关系**：被 `definitions`/`persistence`/`stores/guide` 引用。
- **测试文件**：无。

### guide — guide/definitions.ts

- **核心导出**：`GUIDE_STEPS: Record<GuideScenario, readonly GuideStep[]>`（inbound 7 步、outbound 7 步、stocktake 6 步，每步含 `id`/`route`/`target`/`title`/`description`/`event`）；`getGuideSteps(scenario)`、`getGuideStep(scenario, index)`、`GUIDE_SCENARIO_LABELS`。
- **依赖/调用关系**：`guide/types`、`guide/events`；被 `stores/guide`、`views/demo/index` 消费。
- **测试文件**：无。

### guide — guide/events.ts

- **核心导出**：`GUIDE_EVENTS`（Guide 层内部事件常量，由 `businessBridge` 从业务事件转换而来）。
- **依赖/调用关系**：被 `businessBridge`、`definitions` 使用。
- **测试文件**：无。

### guide — guide/persistence.ts

- **核心导出**：`GUIDE_STORAGE_KEY = 'wms-manual-guide-v1'`；`interface PersistedGuideState`；`loadGuideState`/`saveGuideState`/`clearGuideState`（基于 sessionStorage）。旧数据缺 `inferredStepIds` 时自动兼容为 `[]`。
- **依赖/调用关系**：被 `stores/guide` 使用。
- **测试文件**：无。

### guide — guide/routeMatcher.ts

- **核心导出**：`resolveGuideRoute(route, orderId, orderNo)`（替换 `:orderId`/`:orderNo`）、`matchGuideStepByRoute(...)`、`isGuideFlowRoute(...)`。
- **依赖/调用关系**：被 `useGuideRunner`、`stores/guide` 使用。
- **测试文件**：`guide/routeMatcher.spec.ts`。

### guide — guide/businessBridge.ts

- **核心导出**：`installGuideBusinessBridge(): () => void`（应用装配阶段调用一次，返回卸载函数供测试）。
- **职责**：订阅 `events/businessEvents`，把中性业务事件翻译为 Guide 步骤完成；内部快照 `inboundSnapshot`/`outboundSnapshot`/`stocktakeSnapshot`/`inventorySnapshot`；`recordIfCurrentStep(event, result, orderId?)` 仅当前步骤 event 匹配才记录；`reconcileInbound`/`reconcileOutbound`/`reconcileStocktake`/`reconcileInventory` 按业务状态核对步骤；用 `watch`（guide.active/scenario/currentStep/completed/orderId/orderNo）触发重核对。
- **依赖/调用关系**：`events/businessEvents`、`stores/guide`、`guide/*`；由 `main.ts` 装配。
- **测试文件**：`guide/businessBridge.spec.ts`。

---

### events — events/businessEvents.ts

- **核心导出**（唯一业务事件契约）：
  - `BUSINESS_EVENTS`（含 `INBOUND_ORDER_CREATED/SUBMITTED/APPROVED/LOADED`、`OUTBOUND_ORDER_CREATED/SUBMITTED/ALLOCATED/LOADED`、`STOCKTAKE_ORDER_CREATED/LOADED`、`INVENTORY_TRANS_LOADED`）。
  - `interface BusinessTaskRef { taskId; taskNo; targetQty; doneQty }`、`interface BusinessStocktakeDetail { bookQty; actualQty; diffQty; adjusted }`、`interface BusinessEventPayload {...}`。
  - `onBusinessEvent(event, listener)`（返回取消订阅）、`emitBusinessEvent(event, payload={})`（无订阅者无副作用）。
- **依赖/调用关系**：业务页面/组合式函数 emit，`guide/businessBridge` 订阅；依赖方向：业务页面 → businessEvents ← Guide/Demo。
- **测试文件**：`events/businessEvents.spec.ts`。

---

### utils — utils/events.ts

- **核心导出**：`DATA_CHANGED_EVENT='wms:data-changed'`、`emitDataChanged`、`onDataChanged`；`OPEN_DEMO_CONSOLE_EVENT`/`RUN_DEMO_SCENARIO_EVENT`/`RUN_DEMO_STAGED_EVENT`；`DemoConsoleScenario`/`DemoRunMode`/`DemoRunScope`；`openDemoConsole`/`runDemoScenarioInConsole`/`runDemoStepByStepInConsole`/`runDemoAutomaticallyInConsole`。
- **依赖/调用关系**：被 `autoRefresh`、`DemoConsole`、`views/demo/index` 使用。
- **测试文件**：无。

### utils — utils/demoEvidence.ts

- **核心导出**：`STORAGE_KEY='wms-demo-evidence-context-v1'`；`DemoRunContext`/`DemoEvidenceContext`/`DemoEvidenceFocus`；`rememberDemoEvidence`/`rememberDemoExecutionWindow`/`clearDemoEvidence`/`readDemoEvidence`/`resolveDemoEvidenceFocus`/`filterDemoActivity`。
- **依赖/调用关系**：被 `views/demo/Activity`、`views/demo/index`、`useDemoRunner` 使用。
- **测试文件**：`utils/demoEvidence.spec.ts`。

### utils — utils/demoOperations.ts

- **核心导出**：`DemoOperationDetail`/`DemoOperationRow`；`operationMeta`（解析 HTTP 业务操作）、`isBusinessOperation`/`isExperimentOperation`、`buildDemoOperationRows`/`buildBusinessOperationRows`/`buildExperimentOperationRows`。
- **依赖/调用关系**：被 `views/demo/Activity`、`views/demo/Performance`、`ManualGuide` 使用。
- **测试文件**：`utils/demoOperations.spec.ts`。

### utils — utils/demoScenario.ts

- **核心导出**：`mergeDemoScenarioResults(results)`（合并 evidence/links/steps/implementation）。
- **依赖/调用关系**：被 `useDemoRunner` 使用。
- **测试文件**：`utils/demoScenario.spec.ts`。

### utils — utils/index.ts

- **核心导出**：`cleanParams`（过滤空值但保留 0/false）、`formatTime`（RFC3339→本地秒级）。
- **依赖/调用关系**：被几乎所有视图使用。
- **测试文件**：`utils/index.spec.ts`。

### utils — utils/options.ts

- **核心导出**：`interface IdOption`；`loadWarehouseOptions`/`toOptionMap`/`loadSkuMap`/`loadIdleLocationOptions`/`loadLocationOptions`。
- **依赖/调用关系**：`api/basic`；被各订单/详情/表单页使用。
- **测试文件**：无。

---

### views — views/login/index.vue

- **核心职责**：登录页，含三种入口——账号密码登录（`login`）、在线体验（`claimDemoAccount`+`login`+`acquireDemoSession`）、个人空间（`listPersonalAccounts`/`claimPersonalAccount`）；`getVersion` 的 `demo_enabled`/`personal_enabled` 作为特性开关决定入口显隐。
- **测试文件**：无。

### views — views/dashboard/index.vue

- **核心职责**：仪表盘——权限过滤的统计卡（今日入库/出库、异常、任务）、待办任务分组、最近单据、快捷入口；`useAutoRefresh(() => refreshDashboard(true))`（默认 5000ms）静默刷新。
- **测试文件**：无。

### views/basic — views/basic/Warehouses.vue

- **核心职责**：仓库管理 CRUD 列表 + 启停状态开关（停用前二次确认）。
- **测试文件**：无。

### views/basic — views/basic/Locations.vue

- **核心职责**：库位管理（先选仓库再查询）、批量新建库位、启停状态。
- **测试文件**：无。

### views/basic — views/basic/Skus.vue

- **核心职责**：货品（SKU）管理 CRUD（编码/条码/名称/规格/单位）。
- **测试文件**：无。

### views/system — views/system/Users.vue

- **核心职责**：用户管理 CRUD、角色分配、重置密码、启停状态。
- **测试文件**：无。

### views/system — views/system/Roles.vue

- **核心职责**：角色管理 CRUD；内置 `PERMISSION_GROUPS`（基础数据/库存与任务/入库/出库/盘点/系统等权限分组）供勾选。
- **测试文件**：无。

### views/system — views/system/OperLogs.vue

- **核心职责**：操作日志查询（按用户名筛选，展示方法/路径/IP/耗时/状态）。
- **测试文件**：无。

### views/task — views/task/Tasks.vue

- **核心职责**：任务中心列表（按类型/状态/`order_id` 筛选）；`useAutoRefresh(() => load(true), 0)`（仅事件驱动刷新）。
- **测试文件**：无。

### views/ai — views/ai/index.vue

- **核心职责**：AI 库存问答（内存态会话历史、建议问题、回车发送、`aiChat`）；无后端状态持久化。
- **测试文件**：无。

### views/inventory — views/inventory/Inventory.vue

- **核心职责**：库存三个标签页（明细/汇总/流水）；流水查询后 `emitBusinessEvent(INVENTORY_TRANS_LOADED, { orderNo, transTypes, transTotal })`；支持从 `route.query` 打开流水抽屉。
- **测试文件**：无。

### views/inbound — views/inbound/InboundOrders.vue

- **核心职责**：入库单列表页；组合 `useInboundOrders` + `useInboundOrderForm` + `useInboundImport` 三个 composable；含收货/上架弹窗与 Excel 导入弹窗；`useAutoRefresh(() => load(true), 0, () => selectedRows.value.length === 0)`。
- **测试文件**：无。

### views/inbound — views/inbound/InboundOrderDetail.vue

- **核心职责**：入库单详情（单据操作分发按状态显隐、明细、关联任务、收货/上架）；加载后 `emitOrderLoaded()` 发 `INBOUND_ORDER_LOADED`（含 pendingPutawayTask）。
- **测试文件**：无。

### views/outbound — views/outbound/OutboundOrders.vue

- **核心职责**：出库单列表页；内联实现列表/批量操作/新建对话框；行操作 emit `OUTBOUND_ORDER_SUBMITTED`/`OUTBOUND_ORDER_ALLOCATED`，创建 emit `OUTBOUND_ORDER_CREATED`；`useAutoRefresh(() => load(true), 0, ...)`。
- **测试文件**：无。

### views/outbound — views/outbound/OutboundOrderDetail.vue

- **核心职责**：出库单详情（单据操作、明细、分配明细、关联任务、拣货）；加载后 `emitOrderLoaded()` 发 `OUTBOUND_ORDER_LOADED`（含 allocationCount/allocatedQty/pickTaskCount/pickTask）。
- **测试文件**：无。

### views/stocktake — views/stocktake/StocktakeOrders.vue

- **核心职责**：盘点单列表 + 新建（仓库/库位范围选择，留空为整仓）；创建成功 emit `STOCKTAKE_ORDER_CREATED`；`useAutoRefresh(() => load(true), 0)`。
- **测试文件**：无。

### views/stocktake — views/stocktake/StocktakeOrderDetail.vue

- **核心职责**：盘点单详情（DRAFT 状态可逐行录入实盘并保存、差异汇总卡、明细表）；加载后 `emitOrderLoaded()` 发 `STOCKTAKE_ORDER_LOADED`（含每行 bookQty/actualQty/diffQty/adjusted）。
- **测试文件**：无。

### views/demo — views/demo/index.vue

- **核心职责**：演示中心首页（业务流程 5 步展示、自动/分步/引导入口、工程验证入口、架构图入口）；`readDemoEvidence` 展示最近执行证据，订阅 `onDataChanged` 刷新；启动引导前 `router.push(firstStep.route)` + `guide.start(scenario)`。
- **测试文件**：无。

### views/demo — views/demo/Activity.vue

- **核心职责**：业务证据页（业务对象/库存流水/业务操作记录三个标签、汇总卡、详情抽屉）；支持按 `route.query` 聚焦某次执行（`resolveDemoEvidenceFocus`/`filterDemoActivity`）；`useAutoRefresh(() => load(true), 5000)`（静默）。
- **测试文件**：无。

### views/demo — views/demo/Performance.vue

- **核心职责**：工程验证页（并发库存分配、拣货作业验证、异步导入机制三个实验卡 + 运行状态抽屉 + 实验记录抽屉）；`useAutoRefresh(..., 5000, () => 所有弹层关闭)`。
- **测试文件**：无。

---

### components — components/DemoConsole.vue

- **核心职责**：演示控制抽屉/状态条；消费 `useDemoSession`（会话/心跳/空闲释放）与 `useDemoRunner`（场景执行），监听 `OPEN_DEMO_CONSOLE_EVENT`/`RUN_DEMO_SCENARIO_EVENT`/`RUN_DEMO_STAGED_EVENT`；承载 `StagedDemoRunner` 与 `DemoRunViewer` 弹层；提供一键重置/退出演示。
- **测试文件**：无。

### components/demo — components/demo/ManualGuide.vue

- **核心职责**：手动引导 Overlay（高亮框 + 气泡 + 完成面板 + 操作记录抽屉）；消费 `useGuideRunner`/`useGuideStore`，仅做展示。
- **测试文件**：无。

### components/demo — components/demo/DemoRunViewer.vue

- **核心职责**：自动演示结果查看器；按 `STAGE_DEFINITIONS`（5 阶段）把 `DemoScenarioResult.steps` 映射为阶段视图，展示阶段/事实/证据；`watch` 结果变化重置到末阶段。
- **测试文件**：无。

### components/demo — components/demo/DemoTour.vue

- **核心职责**：演示导览（Element Plus `el-tour`）；`TOUR_SEEN_KEY_PREFIX='WMS_DEMO_TOUR_SEEN:'` 按 demoSessionId 记录已读；`defineExpose({ open })`。
- **测试文件**：无。

### components/demo — components/demo/StagedDemoRunner.vue

- **核心职责**：分步执行视图；消费 `useStagedDemoRunner`，展示分组/当前步骤/最近结果；`defineExpose({ start })`。
- **测试文件**：无。

### components — components/PickDialog.vue

- **核心职责**：拣货对话框；待执行 PICK 任务选择、批次核对（任务已指定批次时强制扫码一致）、数量提交（`pickOutboundTask`）；`defineExpose({ open })`。
- **测试文件**：无。

### components — components/PutawayDialog.vue

- **核心职责**：上架对话框；待执行 PUTAWAY 任务选择、空闲库位选择、数量提交（`putawayInboundTask`）；`defineExpose({ open })`。
- **测试文件**：无。

### components — components/ReceiveDialog.vue

- **核心职责**：收货对话框；按明细逐行收货（数量/不良品/批次，首次收货必填批次）、行级提交（`receiveInbound`）；`defineExpose({ open })`。
- **测试文件**：无。

### components/common — components/common/PageHeader.vue

- **核心职责**：通用页头；props `title`/`description?`/`eyebrow?`，具名插槽 `actions`。
- **测试文件**：无。

---

### 本部分重点专题

### 专题 1：前端状态管理（Pinia）

**auth 登录态（`stores/auth.ts`）**：token 与 user 原子成对读写（`readAuthPair` 校验成对、`writeAuth` 先清后写）；存储选择由 `pickStorage(isDemo)` 决定——演示账号（perms 含 `wms:demo`）用 sessionStorage，普通用 localStorage；键：`WMS_TOKEN`/`WMS_USER`/`WMS_DEMO_SESSION`；getter `isLoggedIn`/`isDemo`/`displayName`/`hasPerm`（`*` 通配）；action `setAuth`/`setProfile`/`setDemoSession`/`clearDemoSession`/`clear`。

**guide 运行态（`stores/guide.ts`）**：
- 运行态字段：`active`、`scenario`、`currentStep`、`orderId`、`orderNo`、`taskId`、`taskNo`、`startedAt`、`completed`、`verifiedStepIds`（真实事件）、`inferredStepIds`（语义跳跃推理）、`facts`、`lastOutcome`、`mismatch`。
- getter：`steps`/`currentStepDefinition`/`currentStepRoute`/`currentStepNumber`/`totalSteps`/`canGoPrevious`/`canAdvance`。
- action：`persist`/`start`/`recordBusinessResult`/`next`/`previous`/`reposition`/`setMismatch`/`restart`/`cancel`。

### 专题 2：路由与权限

- **`router/index.ts`**：`beforeEach` 三态守卫——未登录非 `/login` → `/login?redirect`；已登录访问 `/login` → `/`；`to.meta.perm` 存在且 `auth.hasPerm` 不通过 → `/dashboard`。`afterEach` 写 `document.title`。`meta.activeMenu` 用于详情页高亮父菜单，`meta.title` 用于面包屑/标题。
- **`directives/permission.ts`**：`permission` 指令，`mounted`/`updated` 按 `auth.hasPerm` 设置 `el.hidden`。
- **`stores/auth.ts::hasPerm`**：`perms` 含 `*` 时全通过。

### 专题 3：业务页面与 Demo/Guide 解耦

- **唯一契约**：`events/businessEvents.ts`（`BUSINESS_EVENTS` + `BusinessEventPayload` + `onBusinessEvent`/`emitBusinessEvent`）。
- **订阅并翻译**：`guide/businessBridge.ts::installGuideBusinessBridge`，订阅业务事件 → `recordIfCurrentStep`/`reconcile*` → 写入 `stores/guide`。
- **依赖方向**：业务页面（`views/inbound|outbound|stocktake|inventory`、`composables/inbound/*`）只 `emitBusinessEvent` 中性事件；Guide/Demo（`guide/*`、`components/demo/*`、`views/demo/*`）只订阅；业务页面不直接依赖 Guide Store。

### 专题 4：自动刷新防重叠（`composables/autoRefresh.ts`）

- `useAutoRefresh(refresh, intervalMs=5000, shouldRefresh=()=>true)`；内部 `inFlight` 布尔锁：`trigger()` 首行 `if (inFlight) return`，`shouldRefresh()` 不通过返回，进入后置 `true`，`finally` 复位，保证同一时刻最多一个 refresh。
- `intervalMs=0`（`TIMER_DISABLED`）不启用定时轮询，仅由 `onDataChanged`（`DATA_CHANGED_EVENT`）事件驱动。
- 生命周期自动 `onMounted(start)`/`onBeforeUnmount(stop)`。

### 专题 5：Excel 导入轮询（`composables/inbound/useInboundImport.ts`）

- `useInboundImport(onCompleted)`；`POLL_INTERVAL_MS = 2000`。
- 状态机：提交导入拿到 `taskId` → `startPolling(taskId)` 记录本次 `pollToken` → 串行 `setTimeout(tick)` → `getImportStatus` → COMPLETED/FAILED 终止（COMPLETED 弹成功）。
- token 作废：`stopPolling()` 递增 `pollToken` 并清 timer；`tick()` 首行 `token !== pollToken` 即丢弃旧响应，避免过期轮询写回。
- `onUnmounted(stopPolling)` 自动停止。

### 专题 6：API 错误分类与 silentError（`api/request.ts`）

- `classifyError(status, code)`：无状态码 → `network`；401 → `auth`；`70002/70003/70005/70006` → `demo`；其余 → `business`。
- `ApiError` 携带 `code`/`status`/`data`/`kind`；`RequestOptions.silentError` 控制是否跳过自动 `ElMessage` 弹错。
- 请求拦截器注入 `Authorization: Bearer <token>` 与 `X-Demo-Session`；响应拦截器剥离并返回 `data`，blob 直返；`handleDemoSessionExpired()` 用 `demoSessionRedirecting` 防重复跳转。

## 四、测试、脚本与部署

> 说明：本文件仅描述测试、脚本、部署、CI 与文档文件，不涉及 Go 业务代码与前端业务代码。所有结论依据文件内容，不确定处标注“待确认”。

---

### E2E 测试 — `web/tests/e2e/support/api.ts`

- **职责**：Playwright E2E 的共享工具层。导出 `baseURL`（默认 `http://127.0.0.1`，可用 `E2E_BASE_URL` 覆盖）、管理员账号 `admin/admin123`、集成 API Key（`E2E_INTEGRATION_API_KEY` / `WMS_INTEGRATION_API_KEY` / 读取仓库根 `.env`）。
- **关键入口/命令**：`requestApi`（带 `Authorization` 调 `/api/v1`，校验 HTTP ok 与 `code=0`）、`loginByApi`、`loginByUi`、`seedBaseData`（登录→建仓库→批量建库位→建 SKU 并回查）、`seedStockedInventory`（在 `seedBaseData` 基础上走完入库提交/审核/收货/上架，铺出真实库存）、`selectOption`/`confirmMessageBox`（Element Plus 组件交互）、`uniqueSuffix`。
- **关系**：被 `inventory-lifecycle.spec.ts`、`auth-and-permissions.spec.ts`、`external-integration.spec.ts` 引用；调用真实后端 API（`/api/v1`），依赖后端与前端已启动。

### E2E 测试 — `web/tests/e2e/inventory-lifecycle.spec.ts`

- **职责**：真实后端库存全链路场景：Excel 导入部分成功、入库 UI 全流程（建单→提交→审核→收货→上架→库存流水）、出库 UI 全流程（FIFO 分配→批次校验→拣货发货→库存归零）、盘点 UI 流程（录入实盘→审核→库存按差异调整）。
- **关键入口/命令**：`npx playwright test inventory-lifecycle.spec.ts`（或 `npm run test:e2e -- inventory-lifecycle`）。账号：`admin/admin123`（`loginByUi`）。
- **关系**：导入用例读取 `samples/inbound/入库单批量导入示例_含错误行.xlsx`（期望 30 行、22/8 成功失败）；其余用例经 `support/api.ts` 的 `seedBaseData`/`seedStockedInventory` 准备数据。

### E2E 测试 — `web/tests/e2e/demo-console.spec.ts`

- **职责**：演示中心（`/demo`）场景，覆盖一屏启动页、抽屉/弹窗空白处关闭、引导演示的退出与续做、无空闲库位时自动建库位、分步与一键自动演示、工程验证三卡（并发分配/拣货/导入）、实验记录与业务证据分流、演示会话跨刷新保持、PDA 并发实验的任务隔离与并发防护、外部待拣任务不被误改。
- **关键入口/命令**：`npx playwright test demo-console.spec.ts`。账号：`demo1/demo123456`（可由 `E2E_DEMO_USERNAME`/`E2E_DEMO_PASSWORD` 覆盖）。用例向 `/api/v1/demo/session/*`、`/api/v1/demo/run/*` 发真实请求。
- **关系**：依赖演示租户中的固定基础资料（`WH01`、`SKU000001`、演示库位 `A01-02-02`、`D...` 库位）；`test.afterEach` 调用 `/demo/session/release` 释放演示席位。

### E2E 测试 — `web/tests/e2e/login-boundaries.spec.ts`

- **职责**：登录边界场景，**通过 `page.route` 模拟 API，不写业务数据库**。覆盖：手工登录省略/平台租户 `0`/大整数租户编号的传参保持、演示登录席位已满时清除登录态、个人空间登录传入选定账号租户、演示登录进入专用演示首页。
- **关键入口/命令**：`npx playwright test login-boundaries.spec.ts`。无真实账号，靠 mock 的 `/api/v1/version`、`/api/v1/login`、`/api/v1/demo/account`、`/api/v1/personal/*`、`/api/v1/demo/session/*`。
- **关系**：只验证前端行为，不依赖后端与数据库；对应 `web/src/views/login/index.vue`。

### E2E 测试 — `web/tests/e2e/auth-and-permissions.spec.ts`

- **职责**：认证与权限场景：管理员登录及受保护账号（内置管理员不可禁用/编辑/删除，内置角色不可改），退出确认弹窗居中，个人空间持久账号登录，受限角色用户看不到系统管理入口。
- **关键入口/命令**：`npx playwright test auth-and-permissions.spec.ts`。经 `loginByApi` 拿管理员 token，通过 `/system/roles`、`/system/users` 真实建角色/用户并在 `finally` 中删除。
- **关系**：使用 `support/api.ts` 的 `baseURL`、`loginByApi`、`loginByUi`、`requestApi`、`uniqueSuffix`；写真实业务库（受限用户用例）。

### E2E 测试 — `web/tests/e2e/external-integration.spec.ts`

- **职责**：外部 OMS 集成接口场景：`POST /api/v1/integration/outbound-orders` 错误 Key 返回 401、首次创建返回 `idempotent:false` 与 `status:DRAFT`、重复同一 `biz_order_no` 返回 `idempotent:true` 且 `order_id` 一致，并在出库页面查到该单。
- **关键入口/命令**：`npx playwright test external-integration.spec.ts`。需要 `WMS_INTEGRATION_API_KEY`（来自环境变量或根 `.env`）。
- **关系**：用 `seedBaseData` 建仓库/SKU；依赖 `scripts/e2e.ps1` 中设置的集成 Key 环境变量。

---

### 前端工程配置 — `web/playwright.config.ts`

- **职责**：Playwright 配置。`testDir=./tests/e2e`，串行执行（`fullyParallel:false`、`workers:1`），仅 chromium 项目，超时 90s、断言 15s，CI 下 2 次重试并禁用 `test.only`。
- **关键入口/命令**：`npm run test:e2e`（`playwright test`）、`test:e2e:headed`、`test:e2e:ui`、`test:e2e:report`。baseURL 取 `E2E_BASE_URL`，默认 `http://127.0.0.1`。
- **关系**：输出 `playwright-report`、`test-results`；被 CI 的 e2e job 与 `scripts/e2e.ps1` 调用。

### 前端工程配置 — `web/vite.config.ts`

- **职责**：Vite 开发/构建配置。启用 Vue 插件与 `unplugin-vue-components`（Element Plus 按需、`importStyle: css`）；别名 `@`→`./src`；dev server 端口 5173，`/api` 代理到 `VITE_PROXY_TARGET`（默认 `http://127.0.0.1:8080`）。
- **关键入口/命令**：由 `npm run dev` / `npm run build` 使用。
- **关系**：CI e2e 通过 `npm run dev -- --host 127.0.0.1` 起前端，代理指向同机后端。

### 前端工程配置 — `web/package.json`

- **职责**：前端依赖与脚本清单。Node 版本要求 `^20.19.0 || >=22.12.0`；脚本：`dev`、`build`（先 `sync-overview --check`，再 `vue-tsc -b` 与 `vite build`）、`lint`（eslint）、`test`（vitest run）、`test:watch`、`test:e2e`、`test:e2e:headed`、`test:e2e:ui`、`test:e2e:report`、`preview`、`sync:overview`、`check:overview`。
- **关键入口/命令**：`npm ci` / `npm run lint` / `npm test` / `npm run build` / `npm run test:e2e`。
- **关系**：被 `scripts/windows/verify.ps1`、CI frontend job、`scripts/e2e.ps1` 调用；`build` 依赖 `web/scripts/sync-overview.mjs`。

### 前端工程配置 — `web/eslint.config.js`

- **职责**：ESLint 扁平配置。组合 `@eslint/js`、`typescript-eslint`、`eslint-plugin-vue`（flat/essential）；忽略 `dist`、`node_modules`、`coverage`、`playwright-report`、`test-results`；为 `.vue` 指定 TS 解析器，为 `tests/**` 与 `playwright.config.ts` 声明 `process` 全局；关闭 `vue/multi-word-component-names` 与 `no-explicit-any`。
- **关键入口/命令**：`npm run lint`。
- **关系**：CI frontend job 与 `verify.ps1` 执行。

### 前端工程配置 — `web/tsconfig.json` / `web/tsconfig.app.json` / `web/tsconfig.node.json`

- **职责**：`tsconfig.json` 仅做 project references（引用 `tsconfig.app.json` 与 `tsconfig.node.json`）。`tsconfig.app.json` 继承 `@vue/tsconfig/tsconfig.dom.json`，限定 `src/**`，类型 `vite/client`，配置 `@/*` 路径与未使用变量检查。`tsconfig.node.json` 面向 Node 环境（`module: nodenext`），仅含 `vite.config.ts`。
- **关键入口/命令**：`vue-tsc -b`（`npm run build` 内）。
- **关系**：`vite.config.ts` 与 `src` 的类型检查入口。

### 前端工程配置 — `web/vitest.config.ts`

- **职责**：单元测试（Vitest）配置。环境 `node`、开启 globals、仅收集 `src/**/*.{test,spec}.ts`，别名 `@`→`./src`。
- **关键入口/命令**：`npm test`（`vitest run`）、`npm run test:watch`。
- **关系**：只覆盖 `src` 下的单元测试，不涉及 `tests/e2e`（E2E 归 Playwright）。

---

### 脚本 — `scripts/wms-common.ps1`

- **职责**：所有 Windows 脚本共享的公共函数库（被 `scripts/windows/*.ps1` 与 `scripts/e2e.ps1` 点源加载）。提供：`Write-Step/Write-Ok` 输出、`Assert-Docker`（检测并自动拉起 Docker Desktop + Compose v2）、`Wait-DockerEngine`、`Assert-Node`（校验 Node ≥20.19 或 ≥22.12）、`Get-ComposePath`/`Invoke-WmsCompose`（固定 `deploy/docker-compose.yaml` 与根 `.env`）、`New-RandomHex`、`Read-DotEnv`/`Save-DotEnv`、`Test-PlaceholderSecret`（识别占位密钥）。
- **关键入口/命令**：非独立执行，供其他脚本 `. (Join-Path ... wms-common.ps1)` 加载。
- **关系**：`$WmsRoot` 派生自脚本位置，被所有脚本作为根路径来源。

### 脚本 — `scripts/e2e.ps1`

- **职责**：用**独立 Compose 项目与独立数据卷**启动真实后端栈并跑 Playwright E2E，结束后 `down -v` 清理。设独立端口（API 28080、Web 28081）、独立库 `gowms_e2e`、演示/个人账号开关与固定密钥，并设 `E2E_BASE_URL=http://127.0.0.1:28081`。
- **关键入口/命令**：`.\scripts\e2e.ps1 [-NoBuild] [-Grep <表达式>]`。内部：校验 Docker 与 Node → `docker compose -p gowms-e2e up -d [--build]` → 轮询 `/healthz` → `npm --prefix web run test:e2e -- [--grep ...]` → finally 清理。
- **关系**：被 `scripts/windows/verify.ps1 -WithE2E` 调用；复用 `wms-common.ps1` 的 `Assert-Node`。

### 脚本 — `scripts/windows/start.ps1`

- **职责**：生产式 Docker 一键启动。读取/修复根 `.env`（缺失或占位则生成随机 MySQL 密码、JWT、集成 Key、Grafana 密码等默认值）→ `docker compose up -d [--build]` → 轮询 API `/healthz` 与 Web 首页 → 打印访问地址与账号 → 默认打开浏览器。
- **关键入口/命令**：`.\scripts\windows\start.ps1 [-NoBrowser] [-NoBuild]`，根目录 `start.cmd` 是其包装。
- **关系**：依赖 `wms-common.ps1`；被 `reset.ps1` 调用（传 `-NoBrowser`）。

### 脚本 — `scripts/windows/start-dev.ps1`

- **职责**：本机开发一键启动，**不使用 Docker**：先检查本机 `127.0.0.1:3306`（MySQL）与 `6379`（Redis）在监听，再各开一个 PowerShell 窗口分别 `go run ./cmd/wms`（:8080）与 `npm run dev`（Vite :5173，日志写 `tmpgen/dev-frontend.log`）。
- **关键入口/命令**：`.\scripts\windows\start-dev.ps1 [-Demo $true|$false]`（默认开演示与个人空间）；通过环境变量 `WMS_DEMO_ENABLED`/`WMS_PERSONAL_ENABLED` 传递开关。
- **关系**：依赖 `configs/config.yaml` 的数据库连接；停止方式为关闭两个窗口。

### 脚本 — `scripts/windows/verify.ps1`

- **职责**：统一验证入口。顺序执行：`gofmt -l .`（有输出即失败）→ `go build ./...` → `go test ./... -count=1` → `go vet ./...` → `golangci-lint@v2.13.2` → `check-tenant-raw.ps1` → （可选）race；随后 `Assert-Node` 并在 `web/` 下执行 `npm ci`（缺 node_modules 时）、`npm run lint`、`npm test`、`npm run build`，（可选）`scripts/e2e.ps1`；最后 `docker compose config --quiet` 与 `git diff --check`。
- **关键入口/命令**：`.\scripts\windows\verify.ps1 [-WithRace] [-WithE2E]`，`verify.cmd` 是其包装。`-WithRace` 需 `.env` 与运行中的 Compose MySQL/Redis，在 `golang:1.26-alpine` 容器内 `CGO_ENABLED=1 go test -race ./...`。
- **关系**：调用 `wms-common.ps1`、`check-tenant-raw.ps1`、`scripts/e2e.ps1`；对齐 CI 的检查项。

### 脚本 — `scripts/windows/check-tenant-raw.ps1`

- **职责**：静态审计 `internal/**/*.go`（排除 `_test.go`）中出现 `.Table(` 或 `.Raw(` 的语句，若其后 8 行内不含 `tenant.Scope`/`tenantScope`/`tenant_id` 则判为违规并非零退出。
- **关键入口/命令**：`.\scripts\windows\check-tenant-raw.ps1`（由 `verify.ps1` 调用）。
- **关系**：保障裸表/原生 SQL 的租户隔离。

### 脚本 — `scripts/windows/package.ps1`

- **职责**：交付打包。要求工作区干净（`git status --porcelain` 为空），用 `git archive --format=zip` 从 HEAD 导出 zip（默认 `dist/wms-<短commit>.zip`），不包含 `.env`、`node_modules`、测试产物等未跟踪文件。
- **关键入口/命令**：`.\scripts\windows\package.ps1 [-OutputPath <路径>]`。
- **关系**：依赖 git；产出交付包。

### 脚本 — `scripts/windows/run-k6.ps1`

- **职责**：k6 压测启动器。按 `-Mode` 选择脚本：`check`→`check-env.js`、`flow`→`demo-flow.js`、`smoke`→`outbound-e2e.js`、`stress`→`outbound-stress.js`、`wave`→`pick-stress.js`；把参数写入 `BASE_URL`/`K6_USERNAME`/`K6_PASSWORD`/`WAREHOUSE_ID`/`SKU_ID`/`ORDER_QTY` 环境变量；`-RemoteWrite` 时加 `--out experimental-prometheus-rw` 并指向本机 Prometheus 9090。
- **关键入口/命令**：`.\scripts\windows\run-k6.ps1 -Mode <check|flow|smoke|stress|wave> [-BaseUrl ...] [-Username/-Password ...] [-WarehouseId/-SkuId ...] [-OrderQty n] [-RemoteWrite]`。`smoke/stress/wave` 强制要求仓库与 SKU ID。
- **关系**：依赖本机安装 `k6`；`wave`/`stress` 会向真实后端写入压测数据。

### 脚本 — `scripts/windows/start-monitoring.ps1` / `stop-monitoring.ps1`

- **职责**：`start-monitoring.ps1` 补齐 `.env` 中 Grafana 端口/管理员/随机密码，然后 `docker compose --profile monitoring up -d prometheus grafana` 并等待 Grafana `/api/health`，打印 Prometheus/Grafana 地址。`stop-monitoring.ps1` 仅 `--profile monitoring stop prometheus grafana`，不动 WMS/MySQL/Redis。
- **关键入口/命令**：`.\scripts\windows\start-monitoring.ps1`、`.\scripts\windows\stop-monitoring.ps1`（对应 `.cmd` 包装）。
- **关系**：依赖 `wms-common.ps1`；与 `deploy/prometheus.yml`、`deploy/grafana/**` 联动。

### 脚本 — `scripts/windows/stop.ps1`

- **职责**：`docker compose down --remove-orphans` 停止 WMS 栈，保留数据卷。
- **关键入口/命令**：`.\scripts\windows\stop.ps1`（`stop.cmd` 包装）。
- **关系**：依赖 `wms-common.ps1`。

### 脚本 — `scripts/windows/reset.ps1`

- **职责**：清空数据重启。需交互输入 `RESET`（或 `-Force`），执行 `down -v` 删除 MySQL/Redis/上传卷，再调用 `start.ps1` 起干净实例。
- **关键入口/命令**：`.\scripts\windows\reset.ps1 [-Force] [-NoBrowser]`（`reset.cmd` 包装）。
- **关系**：调用 `wms-common.ps1` 与 `start.ps1`；会永久删除数据卷。

### 脚本 — `scripts/windows/*.cmd`（`verify.cmd`、`start-monitoring.cmd`、`stop-monitoring.cmd`、`reset.cmd`、`stop.cmd`）

- **职责**：PowerShell 脚本的 CMD 双击/命令行包装器，统一 `powershell -NoProfile -ExecutionPolicy Bypass -File` 调用同名 `.ps1`；部分在失败/结束时 `pause`。
- **关键入口/命令**：直接双击或 `xxx.cmd`。
- **关系**：一一对应同名 `.ps1`。根目录 `start.cmd` 包装 `scripts/windows/start.ps1`。

---

### k6 压测 — `scripts/k6/lib.js`

- **职责**：k6 公共辅助库。导出 `BASE_URL`（默认 `http://127.0.0.1:8080`）、`USERNAME`/`PASSWORD`（默认 `admin/admin123`，用 `K6_*` 变量避免与 Windows 的 `USERNAME` 冲突）、`WAREHOUSE_ID`/`SKU_ID`/`ORDER_QTY`；函数 `login()`、`authHeaders(token, demoSessionId)`、`checkBizOK`、`uniqueBizNo`、`postJSON`/`getJSON`（统一 `{code,msg,data}` 判读与响应状态白名单）。
- **关键入口/命令**：被所有 k6 脚本 `import`。
- **关系**：所有 k6 脚本与 `run-k6.ps1` 的公共依赖。

### k6 压测 — `scripts/k6/check-env.js`

- **职责**：环境自检。健康检查 → 登录 → 查仓库（默认 `WH01`）→ 查 SKU（默认 `SKU000001`）→ 查库存汇总，输出可直接复用的 `WAREHOUSE_ID`、`SKU_ID`、可用库存。
- **关键入口/命令**：`k6 run scripts/k6/check-env.js`，或 `run-k6.ps1 -Mode check`。
- **关系**：为其它压测脚本提供前置参数。

### k6 压测 — `scripts/k6/demo-flow.js`

- **职责**：1 VU 正常业务流程演示：入库建单→提交→审核→收货→上架→出库审核 FIFO 分配→拣货发货→盘点→核对库存不变量（非负、`stock=available+allocated`）。
- **关键入口/命令**：`k6 run scripts/k6/demo-flow.js`，或 `run-k6.ps1 -Mode flow`。可用 `WAREHOUSE_ID`/`SKU_ID`/`INBOUND_QTY`/`OUTBOUND_QTY`。
- **关系**：业务链路与页面“一键完整流程”一致。

### k6 压测 — `scripts/k6/outbound-e2e.js`

- **职责**：出库单端到端冒烟（1 VU 1 迭代），验证 `DRAFT→submit→approve(分配)→PICKING→pick→SHIPPED` 状态机终态与 `picked_qty`。
- **关键入口/命令**：`k6 run -e WAREHOUSE_ID=.. -e SKU_ID=.. scripts/k6/outbound-e2e.js`，或 `run-k6.ps1 -Mode smoke`。强制要求仓库/SKU ID，需可用库存。
- **关系**：依赖 `lib.js`。

### k6 压测 — `scripts/k6/outbound-stress.js`

- **职责**：出库并发压测。场景 A `mixed_flow`（ramping-vus，多 VU 各自跑完整链路，压库存行锁/FIFO/死锁重试/乐观锁，库存耗尽即业务拒绝）；场景 B `pick_contention`（`ENABLE_PICK_CONTENTION=1` 时，多 VU 抢同一批拣货任务各拣 1 件，验证恰好 `CONTENTION_QTY` 次成功、无超拣）。含自定义指标 `biz_success_total`/`biz_fail_total`。
- **关键入口/命令**：`k6 run -e WAREHOUSE_ID=.. -e SKU_ID=.. scripts/k6/outbound-stress.js`，或 `run-k6.ps1 -Mode stress`。规模由 `STRESS_VUS`/`STRESS_RAMP`/`STRESS_PEAK`/`STRESS_DOWN`/`ORDER_QTY` 控制。
- **关系**：依赖 `lib.js`；建议用独立压测环境。

### k6 压测 — `scripts/k6/pick-stress.js`

- **职责**：贴近现场的并发拣货压测。setup 用**真实入库流程**在 `ROWS` 个库位各铺 `QTY_PER_ROW` 件独立批次 → 建并审核大单触发 FIFO 生成 `ROWS` 个拣货任务 → `wave` 场景（`WORKERS` 个 VU 各逐件扫码）+ `chaos` 场景（`CHAOS` 个 VU 抢拣，压防超拣）；teardown 核对单据 `SHIPPED`、`picked_qty` 恰好等于 `TOTAL`、任务全 `COMPLETED`、无负库存、`pick_ok_total` 恰好等于 `TOTAL`。
- **关键入口/命令**：`k6 run -e WAREHOUSE_ID=.. -e SKU_ID=.. scripts/k6/pick-stress.js`，或 `run-k6.ps1 -Mode wave`。规模由 `ROWS`/`QTY_PER_ROW`/`WORKERS`/`CHAOS`/`SCAN_INTERVAL` 控制。
- **关系**：依赖 `lib.js`；默认规模大（40×100=4000 件、60 VU），适合专用环境。

### k6 压测 — `scripts/k6/read-stress.js`

- **职责**：读路径压测（ramping-vus 到 20 VU 保持 1 分钟）。覆盖登录（bcrypt 成本）、出库列表首页与深分页第 100 页、出库详情（最重聚合）、入库列表、库存汇总、库存流水、任务列表、SKU 列表；用 `api` tag 分接口打印各自 p95。
- **关键入口/命令**：`k6 run scripts/k6/read-stress.js`（或 `-e BASE_URL=...`）。`run-k6.ps1` 未提供对应 Mode，需直接运行。
- **关系**：依赖 `lib.js`。

### k6 压测 — `scripts/k6/allocation-proof.js`

- **职责**：并发库存分配“证明”脚本。setup 先把目标 SKU 备到精确 `TARGET_STOCK`（默认 4000 件、要求初始 `allocated=0`）并预建 `VUS`（默认 60）张 `SUBMITTED` 出库单；随后 `VUS` 个 VU 同时对各自订单 `approve`，teardown 校验：无负库存、每行与汇总 `stock=available+allocated`、库存总量前后不变、`available` 减少量= `allocated` 增加量、成功订单数=理论可分配数、剩余被拒绝、无异常状态。
- **关键入口/命令**：`k6 run -e WAREHOUSE_ID=.. -e SKU_ID=.. scripts/k6/allocation-proof.js`（可调 `VUS`/`ORDER_QTY`/`TARGET_STOCK`）。
- **关系**：需干净测试环境（初始无分配量）。

### k6 压测 — `scripts/k6/approve-concurrency.js`

- **职责**：上一脚本的入门版，只覆盖“并发审核”这一步。setup 不备货，只读一次库存并预建 `VUS`（默认 10）张 `SUBMITTED` 出库单；随后 `VUS` 个 VU 同时对各自订单 `approve`；teardown 校验 4 条不变量：无负库存、每行 `stock=available+allocated`、库存总量前后不变、`available` 减少量=`allocated` 增加量、分配总量不超过原有可用库存。用 `Counter` 区分成功/业务拒绝/系统失败三类结果。
- **关键入口/命令**：`k6 run -e WAREHOUSE_ID=.. -e SKU_ID=.. -e K6_USERNAME=user1 scripts/k6/approve-concurrency.js`（可调 `VUS`/`ORDER_QTY`；`ORDER_QTY` 调大到总需求超过库存即可看到部分拒绝）。
- **关系**：依赖 `lib.js`；不自动备货，要求目标 SKU 已有可用库存。

### loadtest — `loadtest/01-hello.js`

- **职责**：k6 入门示例脚本。1 个虚拟用户连续 7 秒访问 `http://127.0.0.1:8080/healthz`，断言 HTTP 200 与 `code=0`。
- **关键入口/命令**：`k6 run loadtest/01-hello.js`（硬编码地址，未走环境变量）。
- **关系**：与 `scripts/k6/**` 无 import 关系，属独立教学文件；功能上被 `scripts/k6/check-env.js` 的健康检查覆盖。

---

### 部署 — `deploy/docker-compose.yaml`

- **职责**：主 Compose 定义，服务：`mysql:8.0`、`redis:7-alpine`、`migrate`（复用应用镜像，`entrypoint sh -c "./migrate up && ./migrate ensure-app-user"`：结构迁移 + 幂等补齐应用账户，关闭健康检查）、`bootstrap-admin`（一次性创建/修复平台管理员，依赖 `migrate` 成功）、`wms`（应用镜像，含 MySQL/Redis/JWT/演示/个人/ZHIPU/Metrics 等环境变量，`stop_grace_period` 默认 45s，健康检查 `/healthz`）、`web`（前端 nginx 镜像）、`caddy`（`tls` profile，自动 HTTPS）、`prometheus`/`grafana`（`monitoring` profile）。自有镜像同时声明 `image`（GHCR）与 `build`：服务器 `pull`，本地/E2E 带 `--build`。
- **关键入口/命令**：`docker compose --env-file .env -f deploy/docker-compose.yaml up -d [--build]`；profile：`--profile tls`、`--profile monitoring`。
- **关系**：被 `wms-common.ps1`/`start.ps1`/`e2e.ps1`、Makefile、CI、deploy.yml 引用；依赖 `deploy/Caddyfile`、`deploy/prometheus.yml`、`deploy/grafana/**`。

### 部署 — `deploy/docker-compose.dev.yaml`

- **职责**：本地开发 overlay，只把 `mysql` 暴露到 `127.0.0.1:3306`、`redis` 暴露到 `127.0.0.1:6379`（仅回环）。
- **关键入口/命令**：`docker compose -f deploy/docker-compose.yaml -f deploy/docker-compose.dev.yaml up -d mysql redis`（Makefile `compose-infra`）。
- **关系**：与主 Compose 叠加使用；供本机 `start-dev.ps1`/本地 Go 连接。

### 部署 — `deploy/Caddyfile`

- **职责**：Caddy 反向代理 + 自动 HTTPS。`{$WMS_SITE_DOMAIN}` 站点压缩并 `reverse_proxy web:8080`；`www.` 子域 301 跳主域；`email {$WMS_ACME_EMAIL}` 用于证书通知。
- **关键入口/命令**：由 `caddy` 容器（`--profile tls`）加载。
- **关系**：依赖服务器 `.env` 的 `WMS_SITE_DOMAIN`/`WMS_ACME_EMAIL`；仅服务器启用，本地/E2E 不带该 profile。

### 部署 — `deploy/prometheus.yml`

- **职责**：Prometheus 采集配置。`scrape_interval`/`evaluation_interval` 15s；job `gowms` 抓取 `wms:9090` 的 `/metrics`。
- **关键入口/命令**：由 `prometheus` 容器挂载加载（带 `--web.enable-remote-write-receiver` 以接收 k6 远程写）。
- **关系**：与 `deploy/grafana` 数据源、`run-k6.ps1 -RemoteWrite` 联动。

### 部署 — `deploy/grafana/provisioning/datasources/prometheus.yml`

- **职责**：Grafana 数据源声明：Prometheus，`uid=prometheus`，URL `http://prometheus:9090`，默认且不可编辑。
- **关键入口/命令**：Grafana 启动时自动加载。
- **关系**：被 `deploy/grafana/dashboards/wms-overview.json` 引用。

### 部署 — `deploy/grafana/provisioning/dashboards/dashboards.yml`

- **职责**：Grafana 面板供给配置：从 `/var/lib/grafana/dashboards` 读取面板文件到 `WMS` 文件夹，禁止删除与 UI 修改，30s 刷新。
- **关键入口/命令**：Grafana 启动时自动加载。
- **关系**：加载 `deploy/grafana/dashboards/wms-overview.json`。

### 部署 — `deploy/grafana/dashboards/wms-overview.json`

- **职责**：预置 Grafana 面板定义 “WMS Overview”，含服务状态、请求速率、5xx 错误率、P95 响应时间、当前并发请求等面板（另有接口/状态码 QPS、MySQL 连接池、Goroutine、内存等，详见 `docs/monitoring.md`）。
- **关键入口/命令**：随 Grafana 自动供给，无需手动导入。
- **关系**：由 `dashboards.yml` 加载，数据来自 Prometheus。

---

### CI — `.github/workflows/ci.yml`

- **职责**：CI 流水线，含四个 job：`backend`（ubuntu + MySQL 8.0/Redis 7 service：gofmt 检查、golangci-lint v2.13.2、PowerShell 脚本语法校验、`docker compose config --quiet`、`go mod verify`、`cmd/migrate up → bootstrap-admin → seed-demo → version`、`go build ./...`、`govulncheck` 漏洞扫描（仅拦截可调用且已有修复版本的漏洞）、`go test -race ./... -v -count=1`）；`frontend`（node 22.14.0：`npm ci`、`npm audit --omit=dev`、lint、单测、类型检查+构建）；`containers`（构建后端与前端镜像，needs backend+frontend）；`e2e`（needs backend+frontend，起 MySQL/Redis service，执行迁移/管理员初始化/演示数据，后台起 Go 后端与 Vite，等待健康后 `npm run test:e2e`，始终上传 Playwright 报告，失败时收集服务日志）。
- **关键入口/命令**：`push`（任意分支）与 `pull_request`（main）触发。
- **关系**：设置 `WMS_TEST_REQUIRED=1`、`E2E_BASE_URL` 等；被 `deploy.yml` 以 `workflow_run` 依赖。

### CI — `.github/workflows/deploy.yml`

- **职责**：CD 部署。`workflow_run`（CI 在 main 成功）或 `workflow_dispatch` 触发。`build` job：checkout CI 那次提交（`workflow_run.head_sha`）→ 构建并推送 `gowms-api`/`gowms-web` 到 GHCR（`latest` 与 `sha-xxxxxxx` 双标签）。`deploy` job：未配置 `DEPLOY_HOST` 时跳过；否则 SSH 到服务器 `git fetch`+`git reset --hard origin/main`，`docker compose ... --profile tls pull` 与 `up -d --no-build`，`image prune`，最后健康检查 `DEPLOY_HEALTH_URL`。
- **关键入口/命令**：需 Secrets `DEPLOY_HOST`/`DEPLOY_USER`/`DEPLOY_SSH_KEY`/`DEPLOY_PORT`、Variables `DEPLOY_PATH`/`DEPLOY_HEALTH_URL`。
- **关系**：依赖 CI；使用 `deploy/docker-compose.yaml`（`--profile tls` 会自动带上 Caddy）。

---

### 根 — `Makefile`

- **职责**：常用开发者命令别名。目标：`run`、`migrate-up`、`migrate-down`、`build`、`test`、`test-required`、`test-race`、`lint`、`tidy`、`compose-up`、`compose-infra`、`compose-monitoring`、`compose-monitoring-stop`、`compose-down`。
- **关键入口/命令**：`make <target>`（GNU Make；Windows 需自行安装）。
- **关系**：调用 `cmd/wms`、`cmd/migrate` 与 `deploy/docker-compose.yaml`；`compose-infra` 叠加 `docker-compose.dev.yaml`。

### 根 — `Dockerfile`

- **职责**：后端多阶段镜像。构建阶段 `golang:1.26-alpine` 编译 `cmd/wms` 与 `cmd/migrate`（静态、`-ldflags="-s -w"`）；运行阶段 `alpine:3.21`（装 ca-certificates/tzdata/wget），复制二进制、`configs`、`migrations`，建 `/app/data/uploads`，以非 root 用户 `app` 运行，`EXPOSE 8080`，`HEALTHCHECK` 打 `/healthz`，`ENTRYPOINT ./wms`。
- **关键入口/命令**：`docker build -t gowms-api .`。
- **关系**：被 Compose（`migrate`/`wms`）与 CI `containers` job 构建。

### 根 — `web/Dockerfile`

- **职责**：前端多阶段镜像。构建阶段 `node:22-alpine`：`npm ci` + `npm run build`；运行阶段 `nginxinc/nginx-unprivileged:1.27-alpine`，复制 `nginx.conf` 与 `dist` 到 `/usr/share/nginx/html`，非 root（uid 101），`EXPOSE 8080`，`HEALTHCHECK` 打 `/healthz`。
- **关键入口/命令**：`docker build -t gowms-web ./web`。
- **关系**：被 Compose `web` 服务与 CI `containers` job 构建；配合 `web/nginx.conf`（含 `/api` 反代与 `/healthz`，待确认细节）。

### 根 — `start.cmd`

- **职责**：根目录一键启动包装器，调用 `scripts/windows/start.ps1`，失败时 `pause`。
- **关键入口/命令**：双击或 `start.cmd`。
- **关系**：`scripts/windows/start.ps1` 的入口。

---

### 文档 — `docs/context-handoff.md`

- **职责**：上下文压缩交接摘要。记录项目目标、最高优先级约束（禁止破坏性 git 操作等）、已完成任务组、验证基线、工作区状态、剩余任务与最终验收清单。
- **关系**：`AGENTS.md`/`.trae/rules` 要求每次开工先读；本地被 `.gitignore` 忽略。

### 文档 — `docs/go-style.md`

- **职责**：Go 代码约定与阅读路线。覆盖目录/依赖、命名格式、DTO 与转换、可选字段与共享状态、错误与事务、Context 与后台任务、多租户与缓存、验证学习顺序及当前维护范围。
- **关系**：配合 `docs/review-progress.md`；已纳入 git 跟踪。

### 文档 — `docs/review-progress.md`

- **职责**：持续审查与优化记录（阶段记录，非验收报告）。含当前判断表、已修复关键问题（P0/P1/P2 列表）、必须保留的边界、下一阶段顺序、学习要点、本地验证与发布注意、迭代清单（护栏、业务正确性、测试与可观测性、结构整理、文件职责约定），以及多轮夜间 heartbeat 记录。
- **关系**：与 `docs/context-handoff.md` 相互印证；已纳入 git 跟踪。

### 文档 — `docs/requirements.md`

- **职责**：需求规格说明书。含项目概述、用户角色与权限、功能需求、业务流程需求（状态机）、非功能需求、范围外与已知边界。
- **关系**：需求层面基线，被 `README`/架构文档呼应。

### 文档 — `docs/architecture.md`

- **职责**：架构与代码阅读说明。保留模块化单体、库存规则和事务、单据流程、导入任务与生命周期、多租户/认证/Redis、日志与错误、启动和部署。
- **关系**：与 `docs/database.md`、`docs/api.md` 共同描述系统。

### 文档 — `docs/database.md`

- **职责**：数据库设计。18 张表总览、ER 关系、核心表结构、全局设计约定、与 AutoMigrate 的关系、迁移 000006（导入执行标识）与 000007（多租户索引对齐）。
- **关系**：对应 `migrations/versions/**`。

### 文档 — `docs/api.md`

- **职责**：API 接口文档。通用约定、系统管理、基础数据、库存查询、任务中心、入库/出库/盘点管理、演示模式、健康检查、调试建议。
- **关系**：与后端 handler 路由对应；E2E 与 k6 脚本的接口调用以此为准。

### 文档 — `docs/demo.md`

- **职责**：演示模式（多演示账号）说明。功能、配置（`WMS_DEMO_*` 环境变量、Redis 会话键）、本地使用、体验者/面试官使用说明、压测与 Grafana 展示、安全边界。
- **关系**：对应 `web/tests/e2e/demo-console.spec.ts` 与 `/demo` 前端页面。

### 文档 — `docs/load-testing.md`

- **职责**：业务流程模拟与 k6 压测说明。三层验证（正常流程/出库冒烟/真实并发）、前置条件、`run-k6.ps1` 各模式用法、Prometheus+Grafana 接入、演示话术与注意。
- **关系**：描述 `scripts/k6/**` 与 `scripts/windows/run-k6.ps1` 的使用。

### 文档 — `docs/monitoring.md`

- **职责**：监控说明。链路（WMS `/metrics` → Prometheus 15s → Grafana）、启动方式、面板清单、常用 PromQL、远程服务器 SSH 隧道访问与安全提醒。
- **关系**：对应 `deploy/prometheus.yml`、`deploy/grafana/**`、`start-monitoring.ps1`。

### 文档 — `docs/index.html`（含 `docs/images/**`、`docs/.nojekyll`）

- **职责**：项目概览静态页（纯 HTML+CSS，中文），用于 GitHub Pages/公网在线展示业务流程与技术架构；`docs/images/demo-console*.png`、`overview.png` 为配图；`.nojekyll` 关闭 Jekyll 处理。
- **关系**：与 `web/dist/overview.html`、`web/scripts/sync-overview.mjs` 关联（架构图在演示中心以弹窗打开 /overview.html）。

---

### 本部分重点专题

### 1. 本地起栈与验证流程

- **`scripts/windows/start.ps1`**：Docker 生产式一键起栈。生成/修复根 `.env` 的随机密钥 → `docker compose up -d [--build]` → 等 API `/healthz` 与 Web 首页 → 打开浏览器。依赖 Docker Desktop + Compose v2、根 `.env`、`deploy/docker-compose.yaml`。
- **`scripts/windows/start-dev.ps1`**：本机原生开发起栈（不用 Docker）。要求本机 `3306` MySQL 与 `6379` Redis 已运行；分别新开窗口 `go run ./cmd/wms`（:8080）与 `npm run dev`（Vite :5173）。依赖 `configs/config.yaml`。
- **`scripts/windows/verify.ps1`**：统一验证。默认跑 gofmt→build→`go test -count=1`→vet→golangci-lint→裸表租户审计→前端 lint/单测/build→Compose config 校验→`git diff --check`。`-WithRace` 需 `.env` 与运行中的 `deploy-mysql-1`/`deploy-redis-1`（Linux 容器内 race）；`-WithE2E` 委托 `scripts/e2e.ps1`。
- **`scripts/e2e.ps1`**：独立 Compose 项目（`gowms-e2e`）+ 独立卷起真实栈（API 28080、Web 28081、库 `gowms_e2e`），等健康后跑 Playwright，结束 `down -v` 清理。依赖 Docker + Node，`[-NoBuild] [-Grep]`。

### 2. E2E 运行方式与依赖

- **前置服务**：真实后端（默认 `http://127.0.0.1` 前端入口，`E2E_BASE_URL` 覆盖）+ MySQL + Redis；本地推荐用 `scripts/e2e.ps1` 起独立栈（此时默认 baseURL 为 `http://127.0.0.1:28081`，`E2E_BASE_URL` 由脚本设置）。
- **运行命令**：`npm --prefix web run test:e2e [-- --grep <表达式>]`、`test:e2e:headed`、`test:e2e:ui`、`test:e2e:report`；CI 中 `working-directory: web`、`E2E_BASE_URL=http://127.0.0.1:5173`（Vite dev server）。
- **账号**：管理员 `admin/admin123`（`login-boundaries`/`auth-and-permissions`/`inventory-lifecycle`/`external-integration`）；演示 `demo1/demo123456`（可由 `E2E_DEMO_USERNAME`/`E2E_DEMO_PASSWORD` 覆盖，`demo-console`）；个人空间 `user1/user123456`（CI `.env` 中 `WMS_PERSONAL_*`）。
- **`support/api.ts` 提供的 seed 能力**：`seedBaseData`（登录 → 建仓库 → 批量建库位 → 建 SKU，并回查确认存在）；`seedStockedInventory`（在其上走入库 提交/审核/收货/上架，铺出指定数量真实库存并返回入库单号与批次号）；另有 `loginByApi`/`loginByUi`、`requestApi`、`selectOption`/`confirmMessageBox`、`uniqueSuffix`、`integrationApiKey`。
- **注意**：`login-boundaries.spec.ts` 用 mock API、不写库；其余多数用真实后端（`auth-and-permissions` 的受限用户、`external-integration` 会写业务库）。

### 3. k6 压测脚本与运行入口

- **`check-env.js`**：自检并探测 `WAREHOUSE_ID`/`SKU_ID`/可用库存；`run-k6.ps1 -Mode check`。
- **`demo-flow.js`**：1 VU 完整业务闭环（入库→出库→盘点）并核对库存不变量；`-Mode flow`。
- **`outbound-e2e.js`**：出库单 DRAFT→SHIPPED 冒烟；`-Mode smoke`（需仓库/SKU ID）。
- **`outbound-stress.js`**：混流并发压测，可选同单并发拣货；`-Mode stress`（规模由 `STRESS_*` 控制）。
- **`pick-stress.js`**：真实波次拣货压测（铺货+拣货+抢单）；`-Mode wave`。
- **`read-stress.js`**：读路径压测（列表/详情/深分页/登录）；无对应 Mode，直接 `k6 run`。
- **`allocation-proof.js`**：并发分配正确性证明（60 VU 抢 4000 件库存）；直接 `k6 run`。
- **`approve-concurrency.js`**：并发审核入门版（10 VU 抢同一 SKU，自动备货流程省略）；直接 `k6 run`。
- **通用入口**：`scripts/windows/run-k6.ps1 -Mode <check|flow|smoke|stress|wave>`（透传 `BASE_URL`/账号/仓库/SKU/`ORDER_QTY`）；`-RemoteWrite` 把指标写入本机 Prometheus 9090，配合 `start-monitoring.ps1` 的 Grafana 查看。
- **`loadtest/01-hello.js`**：独立 k6 入门示例（健康检查 7s），不经 `run-k6.ps1`。

### 4. CI（.github/workflows）执行步骤

- **`ci.yml`**（push 任意分支 / PR to main）：
  1. `backend`：gofmt 检查 → golangci-lint v2.13.2 → PowerShell 全脚本语法校验 → `docker compose config --quiet` → `go mod verify` → `go run ./cmd/migrate up` → `bootstrap-admin` → `seed-demo` → `version` → `go build ./...` → `govulncheck`（有可调用且已修复的漏洞即失败）→ `go test -race ./... -v -count=1`（MySQL/Redis service，`WMS_TEST_REQUIRED=1`）。
  2. `frontend`：`npm ci` → `npm audit --omit=dev` → `npm run lint` → `npm test` → `npm run build`。
  3. `containers`（needs backend+frontend）：`docker build` 后端与前端镜像。
  4. `e2e`（needs backend+frontend）：安装依赖与 Playwright Chromium → 执行迁移/管理员初始化/演示数据 → 后台起 Go 后端与 Vite → 等 8080/5173 健康 → `npm run test:e2e` → 始终上传报告、失败收集日志。
- **`deploy.yml`**（CI 在 main 成功后 `workflow_run` 或手动）：`build` job 构建并推送 GHCR 镜像（`latest` + `sha-xxxxxxx`）→ `deploy` job（未配 `DEPLOY_HOST` 则跳过）SSH 到服务器 `git reset --hard origin/main` → `docker compose --profile tls pull` → `up -d --no-build` → `image prune` → 健康检查。

## 五、跨模块业务链路（重点专题导航）

> 各部分的“重点专题”小节给出模块内的实现细节；本节给出端到端链路与关键落点（符号名均为真实代码）。

### 5.1 多租户

- 注入与过滤：`internal/pkg/tenant/tenant.go`（`WithTenant` / `WithExactTenant`）与 `internal/pkg/tenant/gorm.go`（GORM 全局回调注入 `tenant_id`）。
- 请求上下文：`internal/pkg/middleware/request_context.go` 的 `TenantIDOf`。
- 平台旁路：`tenant_id = 0` 的平台账号不注入租户过滤条件；集成入口使用 `WithExactTenant` 固定租户。
- 需手工处理租户一致性的位置：库存 `ListFIFOCandidates` / `ListLocationCodes` / 汇总查询、导入 worker 的 `tenant.WithTenant`（细节见第一、二部分专题）。
- 详细清单见：`## 一、…` 与 `## 二、…` 末尾“重点专题·多租户”。

### 5.2 库存一致性与并发控制

- 数据：`wms_inventory`（含 `version` 列，见迁移 `000002`；CHECK 不变量见迁移 `000008`）。
- 悲观锁：`internal/modules/inventory/repository/repository.go` 的 `LockInventoryByIDs`（`FOR UPDATE`，按主键升序逐行加锁）。
- 条件更新（CAS）：同文件 `AllocateQty` / `ShipQty` / `ReleaseQty` / `AdjustNegative`。
- 事务边界：`internal/pkg/tx`（`tx.Manager`）。
- 细节见 `## 一、…` 末尾“重点专题·库存一致性与并发控制”。

### 5.3 出库 FIFO 分配

链路：`internal/modules/outbound/service/order.go` 的 `Approve` → 库存 `Allocate`（`internal/modules/inventory/service/stock.go`）→ `ListFIFOCandidates` 取候选（按 `stock_in_time ASC, id ASC`）→ `LockInventoryByIDs` 只锁最小前缀 → 写 `wms_allocation` → 生成拣货任务 → 出库单进入 `PICKING`。

### 5.4 拣货

`internal/modules/outbound/service/pick.go` 的 `Pick`（扫码校验 `checkPickScan`）→ 分配行拣满时调用库存 `Ship` 扣减 → 出库单更新为 `SHIPPED`。

### 5.5 Excel 异步导入

`internal/modules/inbound/service/import.go` 的 `Import`（落库 `PENDING`）→ `internal/modules/inbound/service/import_worker.go` 的 `RunImports`（`repository.ClaimImport` 领取 + `run_token` 防重复）→ `PROCESSING` → `COMPLETED` / `FAILED`；`ResetStaleImport` 与 `RunCompensator` 处理卡死任务。

### 5.6 入库收货与上架联动

`internal/modules/inbound/service/receiving.go` 的 `Receive`（收齐后生成上架任务）→ `internal/modules/inbound/service/putaway.go` 的 `Putaway`（调用库存 `Increase`）→ 任务全部完成后入库单置 `COMPLETED`。

### 5.7 Demo 演示模块

- 会话与租约：`internal/modules/demo/service/session.go`；场景编排：`scenario*.go`；实验：`experiments*.go`；活动证据：`activity*.go`。
- 账号与租户：演示账号 `demo1..demoN`、持久体验账号 `user1..userN`（种子见 `internal/bootstrap/demo_accounts.go` / `personal_accounts.go`）。
- 路由开关：`demo.enabled`（`configs/config.yaml`，可被 `WMS_DEMO_ENABLED` 覆盖），关闭时不挂载任何 `/demo` 路由。
- 细节见 `## 二、…` 末尾“重点专题·Demo 演示模块”。

### 5.8 鉴权

`internal/pkg/jwt/jwt.go`（`Claims` 字段 `uid/username/ver/tid`）→ `internal/pkg/middleware/auth.go` 解析并注入上下文 → `internal/modules/system/service/auth.go` 登录、`permission.go` 的 `HasPerm` 权限判断、`login_limiter.go` 登录限流、`password.go` 密码哈希；服务间/集成调用走 `internal/pkg/middleware/api_key.go`。

### 5.9 前端状态管理与路由

- 状态：`stores/auth.ts`（登录态 token/user 原子读取、单一介质写入）、`stores/guide.ts`（引导运行态）。
- 路由与权限：`router/index.ts`（登录态守卫 + `meta.perm` 权限校验）、`directives/permission.ts`。
- 解耦契约：`events/businessEvents.ts`（`onBusinessEvent` / `emitBusinessEvent`）是业务页面与 Demo/Guide 之间**唯一**契约；`guide/businessBridge.ts` 的 `installGuideBusinessBridge` 订阅业务事件并翻译为引导进度。依赖方向：**业务页面 → businessEvents ← Guide/Demo**。
- 自动刷新：`composables/autoRefresh.ts`（`useAutoRefresh`，inFlight 串行防重叠）。
- 导入轮询：`composables/inbound/useInboundImport.ts`（`useInboundImport`，pollToken 作废迟到响应）。
- 错误分类：`api/request.ts` 的 `classifyError`（内部函数）与 `RequestOptions.silentError`、`ApiError`。

## 六、自检与维护

- 自检：本索引的文件路径、`struct` / `interface` / 函数名均取自源码；关键符号（`ListFIFOCandidates`、`LockInventoryByIDs`、`ClaimImport`、`RunImports`、`TenantIDOf`、`useAutoRefresh`、`emitBusinessEvent`、`installGuideBusinessBridge`、`classifyError` 等）已抽样核对真实存在；表名取自 `migrations/versions/000001_init.up.sql`。
- 维护：修改代码后请同步更新对应小节；标为“待确认”的条目在确认结论后应更新为确定内容。
- 导航说明：面向 AI / Codex 的项目导航另见根目录 `AGENTS.md`（该文件为本地文件，已被 `.gitignore` 忽略，不提交到远端）。

