package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/system/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/jwt"
)

// 登录、Token 校验和个人档案。

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
