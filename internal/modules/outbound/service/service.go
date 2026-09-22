// Package service 提供出库单、库存分配、拣货和发货业务规则。
package service

import (
	basicapi "gowms/internal/modules/basic/api"
	invapi "gowms/internal/modules/inventory/api"
	"gowms/internal/modules/outbound/repository"
	taskapi "gowms/internal/modules/task/api"
	"gowms/internal/pkg/config"
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
	limits  config.LimitsConfig // 公开租户数据量配额（防止访客无限建单）
}

func New(repo *repository.Repository, tm *tx.Manager, no *orderno.Generator,
	basic basicapi.BasicAPI, inv invapi.InventoryAPI, taskAPI taskapi.TaskAPI,
	limits config.LimitsConfig) *Service {
	return &Service{repo: repo, tm: tm, no: no, basic: basic, inv: inv, taskAPI: taskAPI, limits: limits}
}
