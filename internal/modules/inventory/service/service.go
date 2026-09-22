// Package service 提供库存读写、分配、调整、释放和流水业务规则。
package service

import (
	"gowms/internal/modules/inventory/repository"
	pkgtx "gowms/internal/pkg/tx"
)

type Service struct {
	repo *repository.Repository
	tm   *pkgtx.Manager
}

func New(repo *repository.Repository, tm *pkgtx.Manager) *Service {
	return &Service{repo: repo, tm: tm}
}
