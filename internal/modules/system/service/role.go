package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

// 角色管理。

func (s *Service) CreateRole(ctx context.Context, actorID int64, req *dto.RoleCreateReq) error {
	createCtx := tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	if err := ValidateRolePerms(ctx, req.Perms); err != nil {
		return err
	}
	if _, err := s.repo.GetRoleByName(createCtx, req.Name); err == nil {
		return errcode.RoleExist
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := s.administer(ctx, actorID, 0, nil, PermSystemRole, func(r *repository.Repository, a *administration) error {
		if err := a.validateRolePerms(ctx, a.actor.TenantID, req.Perms); err != nil {
			return err
		}
		if a.actor.TenantID == 0 && req.Name == "admin" && !a.platformAdmin() {
			return errcode.Forbidden
		}
		return r.CreateRole(createCtx, &model.SysRole{Name: req.Name, Perms: req.Perms, Remark: req.Remark})
	}); err != nil {
		// 并发窗口内前置检查可能同时通过：唯一索引兜底，回查后返回业务错误而不是内部错误。
		if pkgtx.IsDuplicateErr(err) {
			if _, lookupErr := s.repo.GetRoleByName(createCtx, req.Name); lookupErr == nil {
				return errcode.RoleExist
			} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return lookupErr
			}
		}
		return err
	}
	return nil
}

func (s *Service) UpdateRole(ctx context.Context, actorID, id int64, req *dto.RoleUpdateReq) error {
	if err := ValidateRolePerms(ctx, req.Perms); err != nil {
		return err
	}
	targetTenant := tenant.FromContext(ctx)
	err := s.administer(ctx, actorID, 0, []int64{id}, PermSystemRole, func(r *repository.Repository, a *administration) error {
		role, err := a.editableRole(id)
		if err != nil {
			return err
		}
		targetTenant = role.TenantID
		if err := a.validateRolePerms(ctx, targetTenant, req.Perms); err != nil {
			return err
		}
		if targetTenant == 0 && req.Name == "admin" {
			return errcode.ModifyBuiltinRoleForbidden
		}
		return r.UpdateRole(ctx, id, req.Name, req.Perms, req.Remark)
	})
	if err != nil {
		if pkgtx.IsDuplicateErr(err) {
			found, lookupErr := s.repo.GetRoleByName(tenant.WithExactTenant(ctx, targetTenant), req.Name)
			if lookupErr == nil && found.ID != id {
				return errcode.RoleExist
			}
			if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return lookupErr
			}
		}
		return err
	}
	s.invalidatePermCache()
	return nil
}

func (s *Service) DeleteRole(ctx context.Context, actorID, id int64) error {
	err := s.administer(ctx, actorID, 0, []int64{id}, PermSystemRole, func(r *repository.Repository, a *administration) error {
		if _, err := a.editableRole(id); err != nil {
			return err
		}
		return r.DeleteRoleInTx(ctx, id)
	})
	if errors.Is(err, repository.ErrRoleInUse) {
		return errcode.RoleInUse
	}
	if err != nil {
		return err
	}
	s.invalidatePermCache()
	return nil
}

// ListAssignableRoles is advisory: every assignment rechecks the same ceiling
// while holding the user/role locks. A target ID selects an existing user's tenant.
func (s *Service) ListAssignableRoles(ctx context.Context, actorID, targetID int64) ([]*dto.RoleResp, error) {
	result := make([]*dto.RoleResp, 0)
	err := s.administer(ctx, actorID, targetID, nil, PermSystemUser, func(r *repository.Repository, a *administration) error {
		targetTenant := a.actor.TenantID
		if a.target != nil {
			targetTenant = a.target.TenantID
		}
		roles, err := r.ListAllRoles(tenant.WithExactTenant(ctx, targetTenant))
		if err != nil {
			return err
		}
		for _, role := range roles {
			if a.canGrant(role, targetTenant) {
				result = append(result, roleResponse(role))
			}
		}
		return nil
	})
	return result, err
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
