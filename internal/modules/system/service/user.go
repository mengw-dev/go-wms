package service

import (
	"context"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
)

// 用户管理。

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
