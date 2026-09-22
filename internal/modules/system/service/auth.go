package service

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/tenant"
)

// 登录、Token 校验和个人档案。

func (s *Service) Login(ctx context.Context, req *dto.LoginReq, clientIP string) (*dto.LoginResp, error) {
	if req == nil || req.Username == "" || req.Password == "" || (req.TenantID != nil && *req.TenantID < 0) {
		return nil, errcode.ParamError
	}
	attemptKey := strings.ToLower(strings.TrimSpace(req.Username)) + "|" + clientIP
	if !s.beginLoginAttempt(attemptKey) {
		return nil, errcode.TooManyLoginAttempts
	}
	u, err := s.repo.GetLoginUser(ctx, req.Username, req.TenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, repository.ErrAmbiguousUser) {
			return nil, errcode.UserOrPwdWrong
		}
		return nil, err
	}
	if !checkPassword(u.PasswordHash, req.Password) {
		return nil, errcode.UserOrPwdWrong
	}
	if u.Status != 1 {
		return nil, errcode.UserDisabled
	}
	token, err := jwt.Generate(s.jwtSecret, s.jwtExpire, u.ID, u.Username, u.TokenVersion, u.TenantID)
	if err != nil {
		return nil, err
	}
	roles, perms, err := s.loadRolesAndPerms(tenant.WithTenant(ctx, u.TenantID), u.ID)
	if err != nil {
		return nil, err
	}
	s.clearLoginAttempts(attemptKey)
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
		return err
	}
	if u.Status != 1 || u.TokenVersion != tokenVersion || u.TenantID != tenant.FromContext(ctx) {
		return errcode.Unauthorized
	}
	return nil
}

func (s *Service) Profile(ctx context.Context, userID int64) (*dto.ProfileResp, error) {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.UserIDInvalid
		}
		return nil, err
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.UserIDInvalid
		}
		return err
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
