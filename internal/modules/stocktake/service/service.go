package service

import (
	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/stocktake/repository"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/tx"
)

type Service struct {
	repo *repository.Repository
	tm   *tx.Manager
	no   *orderno.Generator
	inv  api.InventoryAPI
}

func New(repo *repository.Repository, tm *tx.Manager, no *orderno.Generator, inv api.InventoryAPI) *Service {
	return &Service{repo: repo, tm: tm, no: no, inv: inv}
}
