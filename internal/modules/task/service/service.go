package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/modules/task/api"
	"gowms/internal/modules/task/model"
	"gowms/internal/modules/task/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/snowflake"
)

type Service struct {
	repo *repository.Repository
	no   *orderno.Generator
	db   *gorm.DB
}

func New(repo *repository.Repository, no *orderno.Generator, db *gorm.DB) *Service {
	return &Service{repo: repo, no: no, db: db}
}

// taskNoPrefix 任务单号前缀：收货 SH / 上架 SJ / 拣货 PK。
func taskNoPrefix(t model.TaskType) string {
	switch t {
	case model.TaskReceive:
		return "SH"
	case model.TaskPutaway:
		return "SJ"
	case model.TaskPick:
		return "PK"
	default:
		return "TK"
	}
}

// Create 在业务事务内批量创建任务。
func (s *Service) Create(ctx context.Context, tx *gorm.DB, creates []*api.CreateTask) error {
	now := make([]*model.Task, 0, len(creates))
	for _, ct := range creates {
		if ct.TargetQty <= 0 {
			continue
		}
		now = append(now, &model.Task{
			Base:     sysmodel.Base{ID: snowflake.Next()},
			TaskNo:   s.no.Next(ctx, taskNoPrefix(ct.TaskType)),
			TaskType: ct.TaskType, Status: model.TaskCreated,
			OrderID: ct.OrderID, OrderNo: ct.OrderNo,
			DetailID: ct.DetailID, AllocationID: ct.AllocationID,
			SKUID: ct.SKUID, WarehouseID: ct.WarehouseID, TargetQty: ct.TargetQty,
		})
	}
	if len(now) == 0 {
		return nil
	}
	return s.repo.CreateBatch(tx, now)
}

// AddProgress 累加任务完成量并推进状态机（须在业务事务内调用）。
func (s *Service) AddProgress(ctx context.Context, tx *gorm.DB, taskID int64, qty int, operator string) error {
	t, err := s.repo.GetForUpdate(tx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.TaskNotFound
		}
		return err
	}
	return s.progress(tx, t, qty, operator)
}

// AddProgressByDetail 按单据明细定位任务并累加完成量（须在业务事务内调用）。
func (s *Service) AddProgressByDetail(ctx context.Context, tx *gorm.DB, orderID, detailID int64, taskType model.TaskType, qty int, operator string) error {
	t, err := s.repo.GetByDetailForUpdate(tx, orderID, detailID, taskType)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.TaskNotFound
		}
		return err
	}
	return s.progress(tx, t, qty, operator)
}

// CancelByOrder 取消单据下所有未完成任务。
func (s *Service) CancelByOrder(ctx context.Context, tx *gorm.DB, orderID int64) error {
	return s.repo.CancelByOrder(tx, orderID)
}

// CountUnfinished 统计单据下未完成任务数。
func (s *Service) CountUnfinished(ctx context.Context, tx *gorm.DB, orderID int64, taskType model.TaskType) (int64, error) {
	return s.repo.CountUnfinished(tx, orderID, taskType)
}

// progress 锁定行内推进状态机的核心逻辑。
func (s *Service) progress(tx *gorm.DB, t *model.Task, qty int, operator string) error {
	if qty <= 0 {
		return errcode.TaskQtyOver
	}
	switch t.Status {
	case model.TaskCreated, model.TaskInProgress:
		// 允许推进
	default:
		return errcode.TaskStatusWrong
	}
	if t.DoneQty+qty > t.TargetQty {
		return errcode.TaskQtyOver
	}
	t.DoneQty += qty
	t.Operator = operator
	// 计算目标状态：首次报进度 → IN_PROGRESS；报满目标量 → COMPLETED（可一次直达）
	next := t.Status
	if t.Status == model.TaskCreated {
		next = model.TaskInProgress
	}
	if t.DoneQty == t.TargetQty {
		next = model.TaskCompleted
	}
	// 状态机校验：查转换表，非法流转（如已完成/已取消再报进度）拒绝
	if next != t.Status && !model.CanTransit(t.Status, next) {
		return errcode.TaskStatusWrong
	}
	t.Status = next
	if n, err := s.repo.UpdateProgress(tx, t); err != nil {
		return err
	} else if n == 0 {
		// version 冲突：任务已被并发事务推进，提示重试而非"任务不存在"
		return errcode.Conflict
	}
	return nil
}

// GetForUpdate 事务内行锁读取任务（供业务事务在执行前锁定并校验任务状态）。
func (s *Service) GetForUpdate(ctx context.Context, tx *gorm.DB, taskID int64) (*model.Task, error) {
	t, err := s.repo.GetForUpdate(tx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.TaskNotFound
		}
		return nil, err
	}
	return t, nil
}

func (s *Service) List(ctx context.Context, orderID int64, taskType, status, keyword string, page, size int) ([]*model.Task, int64, error) {
	return s.repo.List(ctx, s.db, orderID, taskType, status, keyword, page, size)
}

func (s *Service) Get(ctx context.Context, taskID int64) (*model.Task, error) {
	t, err := s.repo.Get(ctx, s.db, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.TaskNotFound
		}
		return nil, err
	}
	return t, nil
}
