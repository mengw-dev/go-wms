package service

import (
	"context"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
)

// 角色管理。

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
