// Package service 提供入库单、收货、上架和导入任务的业务规则。
package service

import (
	"time"

	basicapi "gowms/internal/modules/basic/api"
	"gowms/internal/modules/inbound/repository"
	invapi "gowms/internal/modules/inventory/api"
	taskapi "gowms/internal/modules/task/api"
	"gowms/internal/pkg/config"
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

	failedFileRetention time.Duration       // 失败导入文件保留期，到期由后台清理删除
	limits              config.LimitsConfig // 公开租户数据量配额（防止访客无限建单）
}

func New(repo *repository.Repository, tm *tx.Manager, no *orderno.Generator,
	basic basicapi.BasicAPI, inv invapi.InventoryAPI, taskAPI taskapi.TaskAPI, upload config.UploadConfig,
	limits config.LimitsConfig) *Service {
	retention := time.Duration(upload.FailedFileRetentionHours) * time.Hour
	if retention <= 0 {
		retention = defaultFailedFileRetention
	}
	return &Service{repo: repo, tm: tm, no: no, basic: basic, inv: inv, taskAPI: taskAPI,
		uploadDir: upload.Dir, failedFileRetention: retention, limits: limits}
}
