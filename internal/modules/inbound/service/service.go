package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	basicapi "gowms/internal/modules/basic/api"
	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	"gowms/internal/modules/inbound/repository"
	invapi "gowms/internal/modules/inventory/api"
	sysmodel "gowms/internal/modules/system/model"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/lock"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tx"
)

type Service struct {
	repo      *repository.Repository
	tm        *tx.Manager
	no        *orderno.Generator
	basic     basicapi.BasicAPI
	inv       invapi.InventoryAPI
	taskAPI   taskapi.TaskAPI
	uploadDir string
	locker    *lock.Locker // 补偿扫描分布式锁，多实例部署防重复补偿；nil 时跳过（单实例语义）
}

func New(repo *repository.Repository, tm *tx.Manager, no *orderno.Generator,
	basic basicapi.BasicAPI, inv invapi.InventoryAPI, taskAPI taskapi.TaskAPI, uploadDir string,
	locker *lock.Locker) *Service {
	return &Service{repo: repo, tm: tm, no: no, basic: basic, inv: inv, taskAPI: taskAPI, uploadDir: uploadDir, locker: locker}
}

func excelizeOpenFile(path string) (*excelize.File, error) {
	return excelize.OpenFile(path)
}

// Excel 异步导入 / 悬挂补偿相关调参。
const (
	importHeartbeatInterval  = 30 * time.Second // 处理中任务心跳间隔
	compensateScanInterval   = 2 * time.Minute  // 悬挂任务扫描间隔
	compensateLockTTL        = 5 * time.Minute  // 补偿分布式锁持有时长
	pendingStaleThreshold    = 2 * time.Minute  // PENDING 超时阈值（超过未被抢占视为悬挂）
	processingStaleThreshold = 5 * time.Minute  // PROCESSING 心跳超时阈值
	staleScanLimit           = 10               // 单次扫描悬挂任务上限
	maxImportErrMsgLen       = 1000             // 导入失败信息落库最大长度
)

// 分布式锁 key。
const compensateLockKey = "wms:import:compensate"

// ---------- 单据生命周期 ----------

// Create 创建入库单（RK 单号；唯一索引兜底 + 重新生成重试 3 次）。
func (s *Service) Create(ctx context.Context, req *dto.CreateOrderReq, operator string) (*model.ReceiptOrder, error) {
	return s.createOrder(ctx, req, operator, nil)
}

// CreateImportOrder 导入建单：以（导入任务 ID + Excel 行号）为幂等键，Source = IMPORT。
// 补偿扫描重跑已处理过的行时直接返回已有单据，不再重复建单。
func (s *Service) CreateImportOrder(ctx context.Context, taskID string, rowNo int, req *dto.CreateOrderReq, operator string) (*model.ReceiptOrder, error) {
	// 幂等检查：该行已建单则直接复用（补偿重跑）
	if o, err := s.repo.GetByImportRow(ctx, s.tm.DB(), taskID, rowNo); err == nil {
		return o, nil
	}
	o, err := s.createOrder(ctx, req, operator, func(order *model.ReceiptOrder) {
		order.Source = model.SourceImport
		order.ImportTaskID = &taskID
		order.ImportRow = rowNo
	})
	if err != nil {
		// 幂等兜底：重跑/并发撞 uk_import_row 唯一键，回读已有单视为成功
		if exist, gerr := s.repo.GetByImportRow(ctx, s.tm.DB(), taskID, rowNo); gerr == nil {
			return exist, nil
		}
		return nil, err
	}
	return o, nil
}

// createOrder 建单核心：decorate 在落库前注入来源/幂等键等扩展字段（RK 单号冲突重试 3 次）。
func (s *Service) createOrder(ctx context.Context, req *dto.CreateOrderReq, operator string, decorate func(*model.ReceiptOrder)) (*model.ReceiptOrder, error) {
	if err := s.basic.ValidateWarehouse(ctx, req.WarehouseID); err != nil {
		return nil, err
	}
	details, expected, err := s.buildDetails(ctx, req.Details)
	if err != nil {
		return nil, err
	}
	var order *model.ReceiptOrder
	for i := 0; i < tx.MaxOrderNoRetry; i++ { // 单号冲突重试
		order = &model.ReceiptOrder{
			Base:        sysmodel.Base{ID: snowflake.Next()},
			OrderNo:     s.no.Next(ctx, model.OrderNoPrefix),
			WarehouseID: req.WarehouseID, Status: model.OrderDraft,
			Source: model.SourceManual, Remark: req.Remark, ExpectedQty: expected, CreatedBy: operator,
		}
		if decorate != nil {
			decorate(order)
		}
		err = s.tm.Tx(ctx, func(tx *gorm.DB) error {
			return s.repo.CreateOrder(tx, order, details)
		})
		if err == nil {
			return order, nil
		}
		if !tx.IsDuplicateErr(err) {
			return nil, err
		}
		log.WithContext(ctx).Warn("order_no duplicated, retry", "order_no", order.OrderNo)
	}
	return nil, errcode.OrderNoDuplicate
}

// Update 仅 DRAFT 可编辑。
func (s *Service) Update(ctx context.Context, id int64, req *dto.CreateOrderReq) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			return errcode.OrderNotFound
		}
		if o.Status != model.OrderDraft {
			return errcode.OrderStatusWrong
		}
		if err := s.basic.ValidateWarehouse(ctx, req.WarehouseID); err != nil {
			return err
		}
		details, expected, err := s.buildDetails(ctx, req.Details)
		if err != nil {
			return err
		}
		o.WarehouseID = req.WarehouseID
		o.Remark = req.Remark
		o.ExpectedQty = expected
		res := tx.Model(&model.ReceiptOrder{}).Where("id = ? AND version = ?", id, o.Version).Updates(map[string]any{
			"warehouse_id": o.WarehouseID, "remark": o.Remark, "expected_qty": o.ExpectedQty,
			"version": gorm.Expr("version + 1"),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.OrderVersionBad
		}
		return s.repo.ReplaceDetails(tx, id, details)
	})
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			return errcode.OrderNotFound
		}
		if o.Status != model.OrderDraft {
			return errcode.OrderStatusWrong
		}
		return s.repo.DeleteOrder(tx, id)
	})
}

// Submit 提交：DRAFT → SUBMITTED。
func (s *Service) Submit(ctx context.Context, id int64) error {
	return s.transit(ctx, id, model.OrderDraft, model.OrderSubmitted)
}

// Approve 审核：SUBMITTED → APPROVED，并生成收货任务。
func (s *Service) Approve(ctx context.Context, id int64, operator string) error {
	_ = operator // 预留：任务创建暂不记录操作人，保持与其他生命周期方法签名一致
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			return errcode.OrderNotFound
		}
		if o.Status != model.OrderSubmitted {
			return errcode.OrderStatusWrong
		}
		// 状态机校验：SUBMITTED → APPROVED
		if !model.CanTransit(o.Status, model.OrderApproved) {
			return errcode.OrderStatusWrong
		}
		details, err := s.repo.ListDetails(tx, id)
		if err != nil {
			return err
		}
		if n, err := s.repo.UpdateStatus(tx, id, model.OrderSubmitted, model.OrderApproved); err != nil || n == 0 {
			if err != nil {
				return err
			}
			return errcode.OrderVersionBad
		}
		tasks := make([]*taskapi.CreateTask, 0, len(details))
		for _, d := range details {
			tasks = append(tasks, &taskapi.CreateTask{
				TaskType: taskmodel.TaskReceive, OrderID: o.ID, OrderNo: o.OrderNo,
				DetailID: d.ID, SKUID: d.SKUID, WarehouseID: o.WarehouseID, TargetQty: d.ExpectedQty,
			})
		}
		return s.taskAPI.Create(ctx, tx, tasks)
	})
}

// Cancel 取消：DRAFT/SUBMITTED/APPROVED 可取消，同时取消任务。
func (s *Service) Cancel(ctx context.Context, id int64) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			return errcode.OrderNotFound
		}
		// 状态机校验：DRAFT/SUBMITTED/APPROVED 可取消；RECEIVING 之后不可取消
		if !model.CanTransit(o.Status, model.OrderCancelled) {
			return errcode.OrderStatusWrong
		}
		if n, err := s.repo.UpdateStatus(tx, id, o.Status, model.OrderCancelled); err != nil || n == 0 {
			if err != nil {
				return err
			}
			return errcode.OrderVersionBad
		}
		return s.taskAPI.CancelByOrder(ctx, tx, id)
	})
}

func (s *Service) transit(ctx context.Context, id int64, from, to model.OrderStatus) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			return errcode.OrderNotFound
		}
		// 状态机校验：查转换表，非法流转（跨状态、终态再转、重复提交）一律拒绝
		if !model.CanTransit(o.Status, to) || o.Status != from {
			return errcode.OrderStatusWrong
		}
		if n, err := s.repo.UpdateStatus(tx, id, o.Status, to); err != nil {
			return err
		} else if n == 0 {
			// 行锁内仍 CAS 失败：并发状态变更，语义为版本冲突而非状态非法
			return errcode.OrderVersionBad
		}
		return nil
	})
}

// ---------- 收货 ----------

// Receive 分次收货：累计不超过预期数量，记录残品；首次收货录入批次号。
// 全部收完后同事务生成上架任务，单据流转 RECEIVING → PUTAWAY。
// 并发收货/死锁由 TxRetry 自动整事务重试；数量累加全部使用数据库原子递增。
func (s *Service) Receive(ctx context.Context, orderID, detailID int64, req *dto.ReceiveReq, operator string) error {
	return s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, orderID)
		if err != nil {
			return errcode.OrderNotFound
		}
		if o.Status != model.OrderApproved && o.Status != model.OrderReceiving {
			return errcode.OrderStatusWrong
		}
		d, err := s.repo.GetDetailForUpdate(tx, detailID)
		if err != nil {
			return errcode.OrderNotFound
		}
		if d.OrderID != orderID {
			return errcode.ParamError
		}
		remaining := d.ExpectedQty - d.ReceivedQty - d.DefectiveQty
		if req.Qty+req.DefectiveQty > remaining {
			return errcode.ReceiveQtyOver
		}
		// 批次号：首次收货必填并落库；后续保持一致
		if d.BatchNo == "" {
			if req.BatchNo == "" {
				return errcode.BatchNoRequired
			}
			d.BatchNo = req.BatchNo
		} else if req.BatchNo != "" && req.BatchNo != d.BatchNo {
			return errcode.BatchNoInconsistent
		}
		// 明细原子累加（批次号仅首次落库）
		if err := s.repo.IncrDetailReceive(tx, d, req.Qty, req.DefectiveQty); err != nil {
			return err
		}
		// 收货任务推进：完成量 = 良品 + 残品（残品同样经过收货作业）。
		// 收齐时任务完成量恰好等于目标量，任务自动 CREATED → IN_PROGRESS → COMPLETED。
		if err := s.taskAPI.AddProgressByDetail(ctx, tx, orderID, detailID, taskmodel.TaskReceive, req.Qty+req.DefectiveQty, operator); err != nil {
			return err
		}

		// 重读全部明细（同事务可见原子累加后的新值）判断是否全部收齐
		all, err := s.repo.ListDetails(tx, orderID)
		if err != nil {
			return err
		}
		fullyReceived := true
		for _, item := range all {
			if item.ReceivedQty+item.DefectiveQty < item.ExpectedQty {
				fullyReceived = false
				break
			}
		}
		var toStatus model.OrderStatus
		var putawayTasks []*taskapi.CreateTask
		if fullyReceived {
			// 上架任务（残品不入库，上架量 = 已收 - 残品）
			for _, item := range all {
				putawayQty := item.ReceivedQty - item.DefectiveQty
				if putawayQty <= 0 {
					continue
				}
				putawayTasks = append(putawayTasks, &taskapi.CreateTask{
					TaskType: taskmodel.TaskPutaway, OrderID: o.ID, OrderNo: o.OrderNo,
					DetailID: item.ID, SKUID: item.SKUID, WarehouseID: o.WarehouseID, TargetQty: putawayQty,
				})
			}
			if len(putawayTasks) > 0 {
				toStatus = model.OrderPutaway
			} else {
				// 全部残品：无上架作业，收齐即完成（否则单据永远停在 PUTAWAY）
				toStatus = model.OrderCompleted
			}
			// 状态机校验：APPROVED（首次收货即收齐）或 RECEIVING → PUTAWAY/COMPLETED
			if !model.CanTransit(o.Status, toStatus) {
				return errcode.OrderStatusWrong
			}
		} else if o.Status == model.OrderApproved {
			// 状态机校验：首次部分收货 APPROVED → RECEIVING
			if !model.CanTransit(o.Status, model.OrderReceiving) {
				return errcode.OrderStatusWrong
			}
			toStatus = model.OrderReceiving
		} else {
			toStatus = o.Status // RECEIVING 中继续收货，状态不变
		}
		// 主单原子累加 + 状态推进（version 乐观锁，冲突由 TxRetry 重试）
		if n, err := s.repo.IncrOrderReceive(tx, o.ID, o.Version, req.Qty, req.DefectiveQty, toStatus); err != nil {
			return err
		} else if n == 0 {
			return errcode.OrderVersionBad
		}
		// 收齐 → 生成上架任务
		if len(putawayTasks) > 0 {
			if err := s.taskAPI.Create(ctx, tx, putawayTasks); err != nil {
				return err
			}
		}
		return nil
	})
}

// ---------- 上架 ----------

// Putaway 上架执行：指定库位 + 数量，调用 inventory.Increase 增加库存，库位标记占用。
// 全部上架任务完成后单据流转 COMPLETED。支持分多次上架。
// 并发上架/死锁由 TxRetry 自动整事务重试。
func (s *Service) Putaway(ctx context.Context, taskID, locationID int64, qty int, operator string) error {
	// 事务外只读不可变路由信息（OrderID/DetailID/SKUID/TaskType/TaskNo/仓库 建后不变）
	routing, err := s.taskAPI.Get(ctx, taskID)
	if err != nil {
		return errcode.TaskNotFound
	}
	if routing.TaskType != taskmodel.TaskPutaway {
		return errcode.TaskStatusWrong
	}
	// 库位校验：存在、非禁用，且必须属于单据仓库，防止跨仓库上架产生不一致库存
	if err := s.basic.ValidateLocationInWarehouse(ctx, routing.WarehouseID, locationID); err != nil {
		return err
	}
	return s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, routing.OrderID)
		if err != nil {
			return errcode.OrderNotFound
		}
		if o.Status != model.OrderPutaway {
			return errcode.OrderStatusWrong
		}
		// 事务内行锁读取任务：权威校验任务未被并发取消
		t, err := s.taskAPI.GetForUpdate(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if t.Status != taskmodel.TaskCreated && t.Status != taskmodel.TaskInProgress {
			return errcode.TaskStatusWrong
		}
		var detail *model.ReceiptOrderDetail
		details, err := s.repo.ListDetails(tx, o.ID)
		if err != nil {
			return err
		}
		for _, d := range details {
			if d.ID == routing.DetailID {
				detail = d
				break
			}
		}
		if detail == nil || detail.BatchNo == "" {
			return errcode.BatchNoRequired
		}
		// 库存生效：上架时才增加库存
		if err := s.inv.Increase(ctx, tx, &invapi.IncreaseReq{
			WarehouseID: o.WarehouseID, LocationID: locationID, SKUID: routing.SKUID,
			BatchNo: detail.BatchNo, Quantity: qty,
			OrderNo: o.OrderNo, TaskNo: routing.TaskNo, Operator: operator,
		}); err != nil {
			return err
		}
		// 库位标记占用
		if err := s.basic.UpdateLocationStatusInTx(ctx, tx, locationID, basicmodel.LocationStatusOccupied); err != nil {
			return err
		}
		// 推进任务（含状态机与数量校验）
		if err := s.taskAPI.AddProgress(ctx, tx, taskID, qty, operator); err != nil {
			return err
		}
		// 全部上架任务完成 → 单据 COMPLETED
		unfinished, err := s.taskAPI.CountUnfinished(ctx, tx, o.ID, taskmodel.TaskPutaway)
		if err != nil {
			return err
		}
		if unfinished == 0 {
			// 状态机校验：PUTAWAY → COMPLETED
			if !model.CanTransit(o.Status, model.OrderCompleted) {
				return errcode.OrderStatusWrong
			}
			if n, err := s.repo.UpdateStatus(tx, o.ID, model.OrderPutaway, model.OrderCompleted); err != nil {
				return err
			} else if n == 0 {
				return errcode.OrderVersionBad
			}
		}
		return nil
	})
}

// ---------- 查询 ----------

type OrderDetail struct {
	Order   *model.ReceiptOrder         `json:"order"`
	Details []*model.ReceiptOrderDetail `json:"details"`
	Tasks   []*taskmodel.Task           `json:"tasks"`
}

func (s *Service) Get(ctx context.Context, id int64) (*OrderDetail, error) {
	o, err := s.repo.GetOrder(ctx, s.tm.DB(), id)
	if err != nil {
		return nil, errcode.OrderNotFound
	}
	details, err := s.repo.ListDetails(s.tm.DB(), id)
	if err != nil {
		return nil, err
	}
	tasks, _, err := s.taskAPI.List(ctx, id, "", 1, taskapi.DetailTaskPageSize)
	if err != nil {
		return nil, err
	}
	return &OrderDetail{Order: o, Details: details, Tasks: tasks}, nil
}

func (s *Service) List(ctx context.Context, q *dto.OrderQuery) ([]*model.ReceiptOrder, int64, error) {
	return s.repo.ListOrders(ctx, s.tm.DB(), q.WarehouseID, q.Status, q.Keyword, q.Page, q.PageSize)
}

// ---------- 内部 ----------

// buildDetails 校验 SKU 并填充编码名称快照。
func (s *Service) buildDetails(ctx context.Context, items []dto.OrderDetailItem) ([]*model.ReceiptOrderDetail, int, error) {
	details := make([]*model.ReceiptOrderDetail, 0, len(items))
	expected := 0
	seen := map[int64]struct{}{}
	for _, it := range items {
		if _, dup := seen[it.SKUID]; dup {
			return nil, 0, errcode.DetailDuplicateSKU
		}
		seen[it.SKUID] = struct{}{}
		sku, err := s.basic.GetSKU(ctx, it.SKUID)
		if err != nil {
			return nil, 0, err
		}
		details = append(details, &model.ReceiptOrderDetail{
			Base:  sysmodel.Base{ID: snowflake.Next()},
			SKUID: sku.ID, SKUCode: sku.Code, SKUName: sku.Name,
			ExpectedQty: it.ExpectedQty,
		})
		expected += it.ExpectedQty
	}
	return details, expected, nil
}

// ---------- Excel 异步导入 ----------

// Import 上传 Excel：秒返回 taskId，后台异步解析。
// 任务状态机 PENDING → PROCESSING → COMPLETED/FAILED，CAS 更新防重复执行。
func (s *Service) Import(ctx context.Context, fileName string, data []byte) (*dto.ImportResp, error) {
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return nil, err
	}
	taskID := fmt.Sprintf("IMP%d", snowflake.Next())
	path := filepath.Join(s.uploadDir, taskID+filepath.Ext(fileName))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	t := &model.ImportTask{
		Base:   sysmodel.Base{ID: snowflake.Next()},
		TaskID: taskID, Status: model.ImportPending,
		FileName: fileName, FilePath: path,
	}
	if err := s.repo.CreateImportTask(ctx, s.tm.DB(), t); err != nil {
		return nil, err
	}
	go s.processImport(taskID) // 异步执行
	return &dto.ImportResp{TaskID: taskID}, nil
}

// GetImport 查询导入进度（前端轮询）。
func (s *Service) GetImport(ctx context.Context, taskID string) (*model.ImportTask, error) {
	t, err := s.repo.GetImportTask(ctx, s.tm.DB(), taskID)
	if err != nil {
		return nil, errcode.ImportTaskNotFound
	}
	return t, nil
}

// processImport 执行导入：CAS 抢占 → 解析 Excel 逐行建单 → 写结果。
func (s *Service) processImport(taskID string) {
	ctx := context.Background()
	n, err := s.repo.CASImportStatus(s.tm.DB(), taskID, model.ImportPending, model.ImportProcessing)
	if err != nil || n == 0 { // 已被其他 goroutine/节点抢占
		return
	}
	// 心跳：长任务定期刷新 updated_at，防止被悬挂补偿误判
	stopHeartbeat := make(chan struct{})
	go func() {
		t := time.NewTicker(importHeartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = s.repo.TouchImport(s.tm.DB(), taskID)
			case <-stopHeartbeat:
				return
			}
		}
	}()
	defer close(stopHeartbeat)

	t, err := s.repo.GetImportTask(ctx, s.tm.DB(), taskID)
	if err != nil {
		return
	}
	total, success, fail, errMsg := s.doImport(ctx, t)
	status := model.ImportCompleted
	if success == 0 && total > 0 {
		status = model.ImportFailed
	}
	if err := s.repo.FinishImport(s.tm.DB(), taskID, status, total, success, fail, errMsg); err != nil {
		log.L().Error("finish import failed", "task_id", taskID, "err", err)
	}
}

// doImport 逐行解析：列 = 仓库编码 | 货品编码 | 预期数量 | 备注，每行一张入库单。
func (s *Service) doImport(ctx context.Context, t *model.ImportTask) (total, success, fail int, errMsg string) {
	f, err := excelizeOpenFile(t.FilePath)
	if err != nil {
		return 0, 0, 1, "打开文件失败: " + err.Error()
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
		return 0, 0, 1, errcode.ImportTemplateHeader.Msg
	}
	header := rows[0]
	if len(header) < 3 || header[0] != "仓库编码" || header[1] != "货品编码" || header[2] != "预期数量" {
		return 0, 0, 1, errcode.ImportTemplateHeader.Msg
	}
	var failMsgs []string
	for i, row := range rows[1:] {
		total++
		if len(row) < 3 {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: 列数不足", i+2))
			continue
		}
		var expectedQty int
		if _, err := fmt.Sscanf(strings.TrimSpace(row[2]), "%d", &expectedQty); err != nil || expectedQty <= 0 {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: 预期数量非法", i+2))
			continue
		}
		wh, err := s.basic.GetWarehouseByCode(ctx, strings.TrimSpace(row[0]))
		if err != nil {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: %v", i+2, err))
			continue
		}
		sku, err := s.basic.GetSKUByCode(ctx, strings.TrimSpace(row[1]))
		if err != nil {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: %v", i+2, err))
			continue
		}
		remark := ""
		if len(row) > 3 {
			remark = row[3]
		}
		// 幂等建单：以（导入任务 ID + Excel 行号）为键，补偿重跑不重复建单
		if _, err = s.CreateImportOrder(ctx, t.TaskID, i+2, &dto.CreateOrderReq{
			WarehouseID: wh.ID, Remark: remark,
			Details: []dto.OrderDetailItem{{SKUID: sku.ID, ExpectedQty: expectedQty}},
		}, "import"); err != nil {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: %v", i+2, err))
			continue
		}
		success++
	}
	if len(failMsgs) > 0 {
		errMsg = strings.Join(failMsgs, "; ")
		if len(errMsg) > maxImportErrMsgLen {
			errMsg = errMsg[:maxImportErrMsgLen]
		}
	}
	return total, success, fail, errMsg
}

// StartCompensator 悬挂任务补偿：按固定间隔扫描超时任务，CAS 抢占后重新执行。
func (s *Service) StartCompensator(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(compensateScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.compensateOnce()
			}
		}
	}()
}

func (s *Service) compensateOnce() {
	ctx := context.Background()
	// 多实例部署时用分布式锁防止重复补偿；Redis 故障降级为直接执行（单实例语义）。
	if s.locker != nil {
		release, ok, err := s.locker.Lock(ctx, compensateLockKey, compensateLockTTL)
		if err != nil {
			log.L().Warn("compensate lock unavailable, run in standalone mode", "err", err)
		} else if !ok {
			return // 其他实例正在补偿
		} else {
			defer release()
		}
	}
	now := time.Now()
	stale, err := s.repo.ListStaleImports(ctx, s.tm.DB(),
		now.Add(-pendingStaleThreshold), now.Add(-processingStaleThreshold), staleScanLimit)
	if err != nil {
		log.L().Error("scan stale imports failed", "err", err)
		return
	}
	for _, t := range stale {
		if t.Status == model.ImportProcessing { // 心跳超时的 PROCESSING：复位后重跑
			n, err := s.repo.ResetProcessingToPending(s.tm.DB(), t.TaskID)
			if err != nil || n == 0 {
				continue
			}
			log.L().Warn("stale processing import reset", "task_id", t.TaskID)
		}
		// PENDING：CAS 抢占后重跑
		go s.processImport(t.TaskID)
	}
}
