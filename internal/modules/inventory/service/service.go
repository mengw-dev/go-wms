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
