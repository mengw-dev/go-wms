package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

// OrderDetail 入库单详情聚合。
type OrderDetail struct {
	Order   *model.ReceiptOrder         `json:"order"`
	Details []*model.ReceiptOrderDetail `json:"details"`
	Tasks   []*taskmodel.Task           `json:"tasks"`
}

func (s *Service) Get(ctx context.Context, id int64) (*OrderDetail, error) {
	o, err := s.repo.GetOrder(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.OrderNotFound
		}
		return nil, err
	}
	details, err := s.repo.ListDetails(s.tm.DB().WithContext(ctx), id)
	if err != nil {
		return nil, err
	}
	tasks, _, err := s.taskAPI.List(ctx, id, "", "", "", 1, taskapi.DetailTaskPageSize)
	if err != nil {
		return nil, err
	}
	return &OrderDetail{Order: o, Details: details, Tasks: tasks}, nil
}

func (s *Service) GetResponse(ctx context.Context, id int64) (*dto.OrderDetailResp, error) {
	detail, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &dto.OrderDetailResp{
		Order:   orderResponse(detail.Order),
		Details: orderDetailRowResponses(detail.Details),
		Tasks:   orderTaskResponses(detail.Tasks),
	}, nil
}

func orderDetailRowResponses(details []*model.ReceiptOrderDetail) []*dto.OrderDetailRowResp {
	if details == nil {
		return nil
	}
	resp := make([]*dto.OrderDetailRowResp, 0, len(details))
	for _, detail := range details {
		resp = append(resp, &dto.OrderDetailRowResp{
			ID:           detail.ID,
			TenantID:     detail.TenantID,
			OrderID:      detail.OrderID,
			SKUID:        detail.SKUID,
			SKUCode:      detail.SKUCode,
			SKUName:      detail.SKUName,
			ExpectedQty:  detail.ExpectedQty,
			ReceivedQty:  detail.ReceivedQty,
			DefectiveQty: detail.DefectiveQty,
			BatchNo:      detail.BatchNo,
			CreatedAt:    detail.CreatedAt,
			UpdatedAt:    detail.UpdatedAt,
		})
	}
	return resp
}

func orderTaskResponses(tasks []*taskmodel.Task) []*dto.OrderTaskResp {
	if tasks == nil {
		return nil
	}
	resp := make([]*dto.OrderTaskResp, 0, len(tasks))
	for _, task := range tasks {
		resp = append(resp, &dto.OrderTaskResp{
			ID:           task.ID,
			TenantID:     task.TenantID,
			TaskNo:       task.TaskNo,
			TaskType:     task.TaskType,
			Status:       task.Status,
			OrderID:      task.OrderID,
			OrderNo:      task.OrderNo,
			DetailID:     task.DetailID,
			AllocationID: task.AllocationID,
			SKUID:        task.SKUID,
			WarehouseID:  task.WarehouseID,
			LocationID:   task.LocationID,
			LocationCode: task.LocationCode,
			BatchNo:      task.BatchNo,
			TargetQty:    task.TargetQty,
			DoneQty:      task.DoneQty,
			Operator:     task.Operator,
			CreatedAt:    task.CreatedAt,
			UpdatedAt:    task.UpdatedAt,
		})
	}
	return resp
}

func (s *Service) List(ctx context.Context, q *dto.OrderQuery) ([]*model.ReceiptOrder, int64, error) {
	return s.repo.ListOrders(ctx, s.tm.DB(), q.WarehouseID, q.Status, q.Keyword, q.ImportTaskID, q.CreatedAtFrom, q.CreatedAtTo, q.Page, q.PageSize)
}

func (s *Service) ListResponses(ctx context.Context, q *dto.OrderQuery) ([]*dto.OrderResp, int64, error) {
	orders, total, err := s.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]*dto.OrderResp, 0, len(orders))
	for _, order := range orders {
		resp = append(resp, orderResponse(order))
	}
	return resp, total, nil
}

func orderResponse(order *model.ReceiptOrder) *dto.OrderResp {
	return &dto.OrderResp{
		ID:           order.ID,
		TenantID:     order.TenantID,
		OrderNo:      order.OrderNo,
		WarehouseID:  order.WarehouseID,
		Status:       order.Status,
		Source:       order.Source,
		Remark:       order.Remark,
		ExpectedQty:  order.ExpectedQty,
		ReceivedQty:  order.ReceivedQty,
		DefectiveQty: order.DefectiveQty,
		ImportTaskID: order.ImportTaskID,
		ImportRow:    order.ImportRow,
		CreatedBy:    order.CreatedBy,
		CreatedAt:    order.CreatedAt,
		UpdatedAt:    order.UpdatedAt,
	}
}
