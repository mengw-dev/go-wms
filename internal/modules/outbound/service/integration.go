package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

// CreateExternal 按仓库/货品编码创建外部推送的出库单，并以业务单号幂等。
func (s *Service) CreateExternal(ctx context.Context, req *dto.ExternalCreateOrderReq, operator string) (*model.ShipmentOrder, bool, error) {
	// 非 HTTP 调用同样只操作一个租户，不能将平台 0 当作跨租户通行证。
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	existing, err := s.repo.GetOrderByBizNo(ctx, s.tm.DB(), req.BizOrderNo)
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	warehouse, err := s.basic.GetWarehouseByCode(ctx, req.WarehouseCode)
	if err != nil {
		return nil, false, err
	}
	details := make([]dto.OrderDetailItem, 0, len(req.Details))
	for _, item := range req.Details {
		sku, err := s.basic.GetSKUByCode(ctx, item.SKUCode)
		if err != nil {
			return nil, false, err
		}
		details = append(details, dto.OrderDetailItem{SKUID: sku.ID, ExpectedQty: item.ExpectedQty})
	}

	order, err := s.Create(ctx, &dto.CreateOrderReq{
		WarehouseID: warehouse.ID,
		BizOrderNo:  req.BizOrderNo,
		Remark:      req.Remark,
		Details:     details,
	}, operator)
	if errors.Is(err, errcode.BizOrderDuplicate) {
		existing, getErr := s.repo.GetOrderByBizNo(ctx, s.tm.DB(), req.BizOrderNo)
		if getErr == nil {
			return existing, true, nil
		}
		return nil, false, getErr
	}
	if err != nil {
		return nil, false, err
	}
	return order, false, nil
}
