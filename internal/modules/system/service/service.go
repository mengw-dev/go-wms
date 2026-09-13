package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/snowflake"
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

// ---------- 登录/档案 ----------

func (s *Service) Login(ctx context.Context, req *dto.LoginReq, clientIP string) (*dto.LoginResp, error) {
	attemptKey := strings.ToLower(strings.TrimSpace(req.Username)) + "|" + clientIP
	if !s.allowLogin(attemptKey) {
		return nil, errcode.TooManyLoginAttempts
	}
	u, err := s.repo.GetUserByUsername(ctx, req.Username)
	if err != nil {
		s.recordLoginFailure(attemptKey)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.UserOrPwdWrong
		}
		return nil, err
	}
	if u.Status != 1 {
		s.recordLoginFailure(attemptKey)
		return nil, errcode.UserDisabled
	}
	if !checkPassword(u.PasswordHash, req.Password) {
		s.recordLoginFailure(attemptKey)
		return nil, errcode.UserOrPwdWrong
	}
	s.clearLoginFailures(attemptKey)
	token, err := jwt.Generate(s.jwtSecret, s.jwtExpire, u.ID, u.Username, u.TokenVersion)
	if err != nil {
		return nil, err
	}
	roles, perms, err := s.loadRolesAndPerms(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &dto.LoginResp{
		Token: token, UserID: u.ID, Username: u.Username,
		Nickname: u.Nickname, Roles: roles, Perms: perms,
	}, nil
}

// ValidateToken 复核用户仍存在、启用且 Token 版本未被密码修改/禁用操作作废。
func (s *Service) ValidateToken(ctx context.Context, userID int64, tokenVersion int) error {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.Unauthorized
		}
		return errcode.Internal
	}
	if u.Status != 1 || u.TokenVersion != tokenVersion {
		return errcode.Unauthorized
	}
	return nil
}

func (s *Service) Profile(ctx context.Context, userID int64) (*dto.ProfileResp, error) {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, errcode.UserIDInvalid
	}
	roles, perms, err := s.loadRolesAndPerms(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &dto.ProfileResp{
		UserID: u.ID, Username: u.Username, Nickname: u.Nickname, Roles: roles, Perms: perms,
	}, nil
}

func (s *Service) ChangePassword(ctx context.Context, userID int64, req *dto.ChangePwdReq) error {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return errcode.UserIDInvalid
	}
	if !checkPassword(u.PasswordHash, req.OldPassword) {
		return errcode.OldPwdWrong
	}
	hash, err := hashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}
	s.invalidatePermCache()
	return nil
}

// ---------- 用户管理 ----------

func (s *Service) CreateUser(ctx context.Context, req *dto.UserCreateReq) error {
	if _, err := s.repo.GetUserByUsername(ctx, req.Username); err == nil {
		return errcode.UserExist
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	return s.repo.CreateUser(ctx, &model.SysUser{
		Username: req.Username, PasswordHash: hash, Nickname: req.Nickname, Status: 1,
	}, req.RoleIDs)
}

func (s *Service) UpdateUser(ctx context.Context, id int64, req *dto.UserUpdateReq) error {
	if id == 1 { // 内置管理员不允许修改角色/状态
		return errcode.ModifyAdminForbidden
	}
	if _, err := s.repo.GetUserByID(ctx, id); err != nil {
		return errcode.UserIDInvalid
	}
	if err := s.repo.UpdateUser(ctx, id, req.Nickname, req.Status, req.RoleIDs); err != nil {
		return err
	}
	s.invalidatePermCache()
	return nil
}

func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	if id == 1 {
		return errcode.ModifyAdminForbidden
	}
	return s.repo.DeleteUser(ctx, id)
}

func (s *Service) ResetPassword(ctx context.Context, id int64, req *dto.ResetPwdReq) error {
	if _, err := s.repo.GetUserByID(ctx, id); err != nil {
		return errcode.UserIDInvalid
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(ctx, id, hash)
}

func (s *Service) ListUsers(ctx context.Context, q *dto.UserListQuery) ([]*model.SysUser, int64, error) {
	return s.repo.ListUsers(ctx, q.Keyword, q.Page, q.PageSize)
}

// ---------- 角色管理 ----------

func (s *Service) CreateRole(ctx context.Context, req *dto.RoleCreateReq) error {
	if _, err := s.repo.GetRoleByName(ctx, req.Name); err == nil {
		return errcode.RoleExist
	}
	return s.repo.CreateRole(ctx, &model.SysRole{Name: req.Name, Perms: req.Perms, Remark: req.Remark})
}

func (s *Service) UpdateRole(ctx context.Context, id int64, req *dto.RoleUpdateReq) error {
	if err := s.repo.UpdateRole(ctx, id, req.Name, req.Perms, req.Remark); err != nil {
		return err
	}
	s.invalidatePermCache()
	return nil
}

func (s *Service) DeleteRole(ctx context.Context, id int64) error {
	n, err := s.repo.CountUserRole(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errcode.RoleInUse
	}
	if err := s.repo.DeleteRole(ctx, id); err != nil {
		return err
	}
	s.invalidatePermCache()
	return nil
}

func (s *Service) ListRoles(ctx context.Context, q *dto.RoleListQuery) ([]*model.SysRole, int64, error) {
	return s.repo.ListRoles(ctx, q.Keyword, q.Page, q.PageSize)
}

func (s *Service) ListAllRoles(ctx context.Context) ([]*model.SysRole, error) {
	return s.repo.ListAllRoles(ctx)
}

// ---------- 权限校验（实现 middleware.PermsChecker） ----------

// HasPerm 校验用户是否拥有 perm 权限；通配符 * 表示全部权限。
func (s *Service) HasPerm(ctx context.Context, userID int64, perm string) bool {
	if userID == 1 {
		return true // 内置管理员
	}
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
		if ok { // 拉取失败时用旧缓存兜底
			return item.perms
		}
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

// expandPerms 角色表存的是逗号分隔串，展开为单个权限。
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

func (s *Service) allowLogin(key string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	item, ok := s.loginAttempts[key]
	if !ok {
		return true
	}
	if time.Now().After(item.ResetAt) {
		delete(s.loginAttempts, key)
		return true
	}
	return item.Failures < maxLoginFailures
}

func (s *Service) recordLoginFailure(key string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	item := s.loginAttempts[key]
	if now.After(item.ResetAt) {
		item = loginAttempt{ResetAt: now.Add(loginFailureWindow)}
	}
	item.Failures++
	s.loginAttempts[key] = item
}

func (s *Service) clearLoginFailures(key string) {
	s.loginMu.Lock()
	delete(s.loginAttempts, key)
	s.loginMu.Unlock()
}

func (s *Service) invalidatePermCache() {
	s.permMu.Lock()
	s.permCache = make(map[int64]permCacheItem)
	s.permMu.Unlock()
}

// ---------- 操作日志（实现 middleware.OperLogRecorder） ----------

// Record 非阻塞投递到异步通道，后台批量落库。
func (s *Service) Record(_ context.Context, r middleware.OperLogRecord) {
	item := &model.SysOperLog{
		ID: snowflake.Next(), UserID: r.UserID, Username: r.Username,
		Path: r.Path, Method: r.Method, Params: r.Params, IP: r.IP,
		CostMs: r.CostMs, Status: r.Status, Result: r.Result,
		CreatedAt: time.Now(),
	}
	select {
	case s.logCh <- item:
	default:
		slog.Warn("oper log channel full, dropped", "path", r.Path)
	}
}

// consumeOperLogs 后台批量消费操作日志。
func (s *Service) consumeOperLogs() {
	batch := make([]*model.SysOperLog, 0, 100)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case item := <-s.logCh:
			batch = append(batch, item)
			if len(batch) >= 100 {
				s.flushOperLogs(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.flushOperLogs(batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *Service) flushOperLogs(batch []*model.SysOperLog) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.repo.InsertOperLogs(ctx, batch); err != nil {
		slog.Error("insert oper logs failed", "err", err, "count", len(batch))
	}
}

// ---------- 操作日志查询 ----------

func (s *Service) ListOperLogs(ctx context.Context, q *dto.OperLogQuery) ([]*model.SysOperLog, int64, error) {
	return s.repo.ListOperLogs(ctx, q.Username, q.Path, q.Page, q.PageSize)
}
