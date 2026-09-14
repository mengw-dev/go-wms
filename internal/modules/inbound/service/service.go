package service

import (
	basicapi "gowms/internal/modules/basic/api"
	"gowms/internal/modules/inbound/repository"
	invapi "gowms/internal/modules/inventory/api"
	taskapi "gowms/internal/modules/task/api"
	"gowms/internal/pkg/lock"
	"gowms/internal/pkg/orderno"
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
