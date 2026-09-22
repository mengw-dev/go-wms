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

func (s *Service) ListRoles(ctx context.Context, q *dto.RoleListQuery) ([]*dto.RoleResp, int64, error) {
	roles, total, err := s.repo.ListRoles(ctx, q.Keyword, q.Page, q.PageSize)
	if err != nil {
		return nil, 0, err
	}
	return roleResponses(roles), total, nil
}

func (s *Service) ListAllRoles(ctx context.Context) ([]*dto.RoleResp, error) {
	roles, err := s.repo.ListAllRoles(ctx)
	if err != nil {
		return nil, err
	}
	return roleResponses(roles), nil
}

func roleResponses(roles []*model.SysRole) []*dto.RoleResp {
	resp := make([]*dto.RoleResp, 0, len(roles))
	for _, role := range roles {
		resp = append(resp, roleResponse(role))
	}
	return resp
}

func roleResponse(role *model.SysRole) *dto.RoleResp {
	return &dto.RoleResp{
		ID:        role.ID,
		TenantID:  role.TenantID,
		Name:      role.Name,
		Perms:     role.Perms,
		Remark:    role.Remark,
		CreatedAt: role.CreatedAt,
		UpdatedAt: role.UpdatedAt,
	}
}
