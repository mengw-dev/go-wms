package service

import (
	basicapi "gowms/internal/modules/basic/api"
	invapi "gowms/internal/modules/inventory/api"
	"gowms/internal/modules/outbound/repository"
	taskapi "gowms/internal/modules/task/api"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/tx"
)

type Service struct {
	repo    *repository.Repository
	tm      *tx.Manager
	no      *orderno.Generator
	basic   basicapi.BasicAPI
	inv     invapi.InventoryAPI
	taskAPI taskapi.TaskAPI
}

func New(repo *repository.Repository, tm *tx.Manager, no *orderno.Generator,
	basic basicapi.BasicAPI, inv invapi.InventoryAPI, taskAPI taskapi.TaskAPI) *Service {
	return &Service{repo: repo, tm: tm, no: no, basic: basic, inv: inv, taskAPI: taskAPI}
}
