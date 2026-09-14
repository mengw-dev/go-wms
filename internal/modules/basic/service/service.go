package service

import (
	"context"
	"time"

	"gowms/internal/modules/basic/api"
	"gowms/internal/modules/basic/repository"
	"gowms/internal/pkg/tx"
)

type Service struct {
	repo  *repository.Repository
	tm    *tx.Manager
	rdb   redisClient
	stock api.StockChecker
}

type redisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

func New(repo *repository.Repository, tm *tx.Manager, rdb redisClient, stock api.StockChecker) *Service {
	return &Service{repo: repo, tm: tm, rdb: rdb, stock: stock}
}
