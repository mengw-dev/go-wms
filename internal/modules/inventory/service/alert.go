package service

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

func (s *Service) ListAlerts(ctx context.Context, q *dto.AlertQuery) (*dto.AlertListResp, error) {
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	var result *dto.AlertListResp
	// Count, summary and rows share a consistent read; no inventory row locks.
	err := s.tm.Tx(ctx, func(db *gorm.DB) error {
		var err error
		result, err = s.repo.ListAlerts(db, tenant.FromContext(ctx), q)
		return err
	})
	return result, err
}

func (s *Service) SaveAlertRule(ctx context.Context, req *dto.SaveAlertRuleReq, operator string) (*dto.AlertRuleSaved, error) {
	if req == nil || req.WarehouseID <= 0 || req.SKUID <= 0 || req.MinAvailableQty < 1 || req.MinAvailableQty > 2147483647 || req.Enabled == nil || operator == "" {
		return nil, errcode.ParamError
	}
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	var id int64
	err := s.tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(db *gorm.DB) error {
		if err := s.repo.AlertReferences(db, req.WarehouseID, req.SKUID); err != nil {
			return err
		}
		var err error
		id, err = s.repo.SaveAlertRule(db, &model.AlertRule{Base: modelbase.Base{ID: snowflake.Next()}, TenantID: tenant.FromContext(ctx), WarehouseID: req.WarehouseID, SKUID: req.SKUID, MinAvailableQty: req.MinAvailableQty, Enabled: *req.Enabled, UpdatedBy: operator})
		return err
	})
	if err != nil {
		return nil, err
	}
	return &dto.AlertRuleSaved{ID: id}, nil
}

func (s *Service) DeleteAlertRule(ctx context.Context, id int64) error {
	if id <= 0 {
		return errcode.ParamError
	}
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	return s.repo.DeleteAlertRule(s.tm.DB().WithContext(ctx), id)
}

func (s *Service) AlertOptions(ctx context.Context, q *dto.AlertOptionQuery) ([]*dto.AlertOption, int64, error) {
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	return s.repo.AlertOptions(s.tm.DB().WithContext(ctx), q)
}
