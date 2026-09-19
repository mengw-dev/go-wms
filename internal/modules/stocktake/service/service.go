package service

import (
	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/stocktake/repository"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/tx"
)

type Service struct {
	repo   *repository.Repository
	tm     *tx.Manager
	no     *orderno.Generator
	inv    api.InventoryAPI
	limits config.LimitsConfig // 公开租户数据量配额（防止访客无限建单）
}

func New(repo *repository.Repository, tm *tx.Manager, no *orderno.Generator, inv api.InventoryAPI,
	limits config.LimitsConfig) *Service {
	return &Service{repo: repo, tm: tm, no: no, inv: inv, limits: limits}
}
