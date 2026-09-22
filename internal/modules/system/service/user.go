package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/errcode"
)

// 用户管理。

func (s *Service) CreateUser(ctx context.Context, req *dto.UserCreateReq) error {
	if _, err := s.repo.GetUserByUsername(ctx, req.Username); err == nil {
		return errcode.UserExist
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	err = s.repo.CreateUser(ctx, &model.SysUser{
		Username: req.Username, PasswordHash: hash, Nickname: req.Nickname, Status: 1,
	}, req.RoleIDs)
	if errors.Is(err, repository.ErrInvalidRole) {
		return errcode.RoleIDInvalid
	}
	return err
}

func (s *Service) UpdateUser(ctx context.Context, id int64, req *dto.UserUpdateReq) error {
	builtin, err := s.isBuiltinAdmin(ctx, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if builtin {
		return errcode.ModifyAdminForbidden
	}
	if req.Status != nil && *req.Status != 0 && *req.Status != 1 {
		return errcode.ParamError
	}
	if err := s.repo.UpdateUser(ctx, id, req.Nickname, req.Status, req.RoleIDs); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.UserIDInvalid
		}
		if errors.Is(err, repository.ErrInvalidRole) {
			return errcode.RoleIDInvalid
		}
		return err
	}
	s.invalidatePermCache()
	return nil
}

func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	builtin, err := s.isBuiltinAdmin(ctx, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if builtin {
		return errcode.ModifyAdminForbidden
	}
	return s.repo.DeleteUser(ctx, id)
}

func (s *Service) ResetPassword(ctx context.Context, id int64, req *dto.ResetPwdReq) error {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.UserIDInvalid
		}
		return err
	}
	if user.TenantID == 0 && user.Username == "admin" {
		return errcode.ModifyAdminForbidden
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(ctx, id, hash)
}

func (s *Service) isBuiltinAdmin(ctx context.Context, id int64) (bool, error) {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return false, err
	}
	return user.TenantID == 0 && user.Username == "admin", nil
}

func (s *Service) ListUsers(ctx context.Context, q *dto.UserListQuery) ([]*model.SysUser, int64, error) {
	return s.repo.ListUsers(ctx, q.Keyword, q.Page, q.PageSize)
}
