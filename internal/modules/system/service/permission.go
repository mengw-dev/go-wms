package service

import (
	"context"
	"strings"
	"time"

	"gowms/internal/pkg/log"
)

// 权限缓存和权限判断。

func (s *Service) HasPerm(ctx context.Context, userID int64, perm string) bool {
	perms := s.cachedPerms(ctx, userID)
	for _, p := range perms {
		if p == "*" || p == perm {
			return true
		}
	}
	return false
}

func (s *Service) cachedPerms(ctx context.Context, userID int64) []string {
	s.permMu.Lock()
	item, ok := s.permCache[userID]
	s.permMu.Unlock()
	if ok && time.Now().Before(item.expire) {
		return item.perms
	}
	perms, err := s.loadPerms(ctx, userID)
	if err != nil {
		log.WithContext(ctx).Error("load perms failed", "user_id", userID, "err", err)
		// 权限缓存过期后不能用旧权限放行，否则数据库故障可能无限延长已撤销的授权。
		return nil
	}
	s.permMu.Lock()
	s.permCache[userID] = permCacheItem{perms: perms, expire: time.Now().Add(permCacheTTL)}
	s.permMu.Unlock()
	return perms
}

func (s *Service) loadPerms(ctx context.Context, userID int64) ([]string, error) {
	raw, err := s.repo.GetPermsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return expandPerms(raw), nil
}

func (s *Service) loadRolesAndPerms(ctx context.Context, userID int64) ([]string, []string, error) {
	roles, err := s.repo.ListRoleNamesByUser(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	perms, err := s.loadPerms(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	return roles, perms, nil
}

func expandPerms(raw []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, r := range raw {
		for _, p := range strings.Split(r, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, dup := seen[p]; !dup {
				seen[p] = struct{}{}
				out = append(out, p)
			}
		}
	}
	return out
}

func (s *Service) invalidatePermCache() {
	s.permMu.Lock()
	s.permCache = make(map[int64]permCacheItem)
	s.permMu.Unlock()
}
