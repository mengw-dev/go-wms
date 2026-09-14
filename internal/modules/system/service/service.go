package service

import (
	"sync"
	"time"

	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
)

type Service struct {
	repo      *repository.Repository
	jwtSecret string
	jwtExpire time.Duration

	permMu    sync.Mutex
	permCache map[int64]permCacheItem // 进程内权限缓存，TTL 60s；角色变更后自动过期

	loginMu       sync.Mutex
	loginAttempts map[string]loginAttempt

	logCh chan *model.SysOperLog // 操作日志异步写入通道
}

type loginAttempt struct {
	Failures int
	ResetAt  time.Time
}

type permCacheItem struct {
	perms  []string
	expire time.Time
}

const (
	permCacheTTL       = 60 * time.Second
	maxLoginFailures   = 5
	loginFailureWindow = 15 * time.Minute
)

func New(repo *repository.Repository, jwtSecret string, expireHours int) *Service {
	s := &Service{
		repo:          repo,
		jwtSecret:     jwtSecret,
		jwtExpire:     time.Duration(expireHours) * time.Hour,
		permCache:     make(map[int64]permCacheItem),
		loginAttempts: make(map[string]loginAttempt),
		logCh:         make(chan *model.SysOperLog, 1024),
	}
	go s.consumeOperLogs()
	return s
}
