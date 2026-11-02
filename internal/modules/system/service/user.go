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

// 用户管理。

func (s *Service) CreateUser(ctx context.Context, actorID int64, req *dto.UserCreateReq) error {
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	if _, err := s.repo.GetUserByUsername(ctx, req.Username); err == nil {
		return errcode.UserExist
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	err = s.administer(ctx, actorID, 0, req.RoleIDs, PermSystemUser, func(r *repository.Repository, a *administration) error {
		if err := a.validateAssignments(req.RoleIDs, a.actor.TenantID); err != nil {
			return err
		}
		return r.CreateUserInTx(ctx, &model.SysUser{Username: req.Username, PasswordHash: hash, Nickname: req.Nickname, Status: 1}, req.RoleIDs)
	})
	if errors.Is(err, repository.ErrInvalidRole) {
		return errcode.RoleIDInvalid
	}
	if err != nil {
		// 并发窗口内前置检查可能同时通过：唯一索引兜底，回查后返回业务错误而不是内部错误。
		if pkgtx.IsDuplicateErr(err) {
			if _, lookupErr := s.repo.GetUserByUsername(ctx, req.Username); lookupErr == nil {
				return errcode.UserExist
			} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return lookupErr
			}
		}
		return err
	}
	return nil
}

func (s *Service) UpdateUser(ctx context.Context, actorID, id int64, req *dto.UserUpdateReq) error {
	if req.Status != nil && *req.Status != 0 && *req.Status != 1 {
		return errcode.ParamError
	}
	err := s.administer(ctx, actorID, id, req.RoleIDs, PermSystemUser, func(r *repository.Repository, a *administration) error {
		if err := a.validateAssignments(req.RoleIDs, a.target.TenantID); err != nil {
			return err
		}
		return r.UpdateUserInTx(ctx, id, req.Nickname, req.Status, req.RoleIDs)
	})
	if errors.Is(err, repository.ErrInvalidRole) {
		return errcode.RoleIDInvalid
	}
	if err != nil {
		return err
	}
	s.invalidatePermCache()
	return nil
}

func (s *Service) DeleteUser(ctx context.Context, actorID, id int64) error {
	err := s.administer(ctx, actorID, id, nil, PermSystemUser, func(r *repository.Repository, _ *administration) error {
		return r.DeleteUserInTx(ctx, id)
	})
	if err == nil {
		s.invalidatePermCache()
	}
	return err
}

func (s *Service) ResetPassword(ctx context.Context, actorID, id int64, req *dto.ResetPwdReq) error {
	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	return s.administer(ctx, actorID, id, nil, PermSystemUser, func(r *repository.Repository, _ *administration) error {
		return r.UpdatePassword(ctx, id, hash)
	})
}

func (s *Service) ListUsers(ctx context.Context, q *dto.UserListQuery) ([]*dto.UserResp, int64, error) {
	users, total, err := s.repo.ListUsers(ctx, q.Keyword, q.Page, q.PageSize)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]*dto.UserResp, 0, len(users))
	for _, user := range users {
		resp = append(resp, userResponse(user))
	}
	return resp, total, nil
}

func userResponse(user *model.SysUser) *dto.UserResp {
	return &dto.UserResp{
		ID:        user.ID,
		TenantID:  user.TenantID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		Status:    user.Status,
		RoleIDs:   user.RoleIDs,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
