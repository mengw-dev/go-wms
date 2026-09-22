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

// 角色管理。

func (s *Service) CreateRole(ctx context.Context, req *dto.RoleCreateReq) error {
	if _, err := s.repo.GetRoleByName(ctx, req.Name); err == nil {
		return errcode.RoleExist
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.repo.CreateRole(ctx, &model.SysRole{Name: req.Name, Perms: req.Perms, Remark: req.Remark})
}

func (s *Service) UpdateRole(ctx context.Context, id int64, req *dto.RoleUpdateReq) error {
	builtin, err := s.isBuiltinRole(ctx, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if builtin {
		return errcode.ModifyBuiltinRoleForbidden
	}
	if err := s.repo.UpdateRole(ctx, id, req.Name, req.Perms, req.Remark); err != nil {
		return err
	}
	s.invalidatePermCache()
	return nil
}

func (s *Service) DeleteRole(ctx context.Context, id int64) error {
	builtin, err := s.isBuiltinRole(ctx, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if builtin {
		return errcode.ModifyBuiltinRoleForbidden
	}
	if err := s.repo.DeleteRole(ctx, id); err != nil {
		if errors.Is(err, repository.ErrRoleInUse) {
			return errcode.RoleInUse
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.RoleIDInvalid
		}
		return err
	}
	s.invalidatePermCache()
	return nil
}

func (s *Service) isBuiltinRole(ctx context.Context, id int64) (bool, error) {
	role, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		return false, err
	}
	return role.TenantID == 0 && role.Name == "admin", nil
}

func (s *Service) ListRoles(ctx context.Context, q *dto.RoleListQuery) ([]*model.SysRole, int64, error) {
	return s.repo.ListRoles(ctx, q.Keyword, q.Page, q.PageSize)
}

func (s *Service) ListAllRoles(ctx context.Context) ([]*model.SysRole, error) {
	return s.repo.ListAllRoles(ctx)
}
