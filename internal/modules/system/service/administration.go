package service

import (
	"context"
	"slices"
	"strings"

	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

// Administration uses current locked database rows, never the permission cache.
// In particular, platform scope alone is not proof of administrator authority.
type administration struct {
	actor  *model.SysUser
	target *model.SysUser
	roles  map[int64]*model.SysRole
	perms  []string
}

func (a *administration) platformAdmin() bool {
	return a.actor.TenantID == 0 && slices.Contains(a.perms, "*")
}

func (a *administration) allows(perm string) bool {
	return a.platformAdmin() || slices.Contains(a.perms, perm)
}

func (a *administration) covers(perms string) bool {
	if a.platformAdmin() {
		return true
	}
	for _, p := range expandPerms([]string{perms}) {
		if p == "*" || p == "wms:demo" || !slices.Contains(a.perms, p) {
			return false
		}
	}
	return true
}

func (a *administration) canGrant(role *model.SysRole, tenantID int64) bool {
	return role != nil && role.TenantID == tenantID &&
		(a.platformAdmin() || tenantID == a.actor.TenantID) && a.covers(role.Perms)
}

func (a *administration) validateAssignments(ids []int64, tenantID int64) error {
	for _, id := range ids {
		role := a.roles[id]
		if role == nil || role.TenantID != tenantID {
			return errcode.RoleIDInvalid
		}
		if !a.canGrant(role, tenantID) {
			return errcode.Forbidden
		}
	}
	return nil
}

// administer locks actor/target users in ascending order, then the union of
// their current roles and requested roles in ascending order. Checks and writes
// share one transaction, so concurrent revocation cannot pass a stale ceiling.
func (s *Service) administer(ctx context.Context, actorID, targetID int64, roleIDs []int64, permission string, fn func(*repository.Repository, *administration) error) error {
	if actorID <= 0 {
		return errcode.Unauthorized
	}
	return s.repo.Transaction(ctx, func(r *repository.Repository) error {
		ids := []int64{actorID}
		if targetID > 0 {
			ids = append(ids, targetID)
		}
		users, err := r.LockUsers(ctx, ids)
		if err != nil {
			return err
		}
		a := &administration{roles: make(map[int64]*model.SysRole)}
		for _, user := range users {
			if user.ID == actorID {
				a.actor = user
			}
			if user.ID == targetID {
				a.target = user
			}
		}
		if a.actor == nil || a.actor.Status != 1 || a.actor.TenantID != tenant.FromContext(ctx) {
			return errcode.Unauthorized
		}
		if err := r.UserRoleIDs(ctx, users); err != nil {
			return err
		}
		allRoleIDs := slices.Clone(roleIDs)
		for _, user := range users {
			allRoleIDs = append(allRoleIDs, user.RoleIDs...)
		}
		roles, err := r.LockRoles(ctx, allRoleIDs)
		if err != nil {
			return err
		}
		for _, role := range roles {
			a.roles[role.ID] = role
		}
		var raw []string
		for _, id := range a.actor.RoleIDs {
			if role := a.roles[id]; role != nil && role.TenantID == a.actor.TenantID {
				raw = append(raw, role.Perms)
			}
		}
		a.perms = expandPerms(raw)
		if !a.allows(permission) {
			return errcode.Forbidden
		}
		if targetID > 0 {
			if a.target == nil {
				return errcode.UserIDInvalid
			}
			if a.target.TenantID == 0 && a.target.Username == "admin" {
				return errcode.ModifyAdminForbidden
			}
			if !a.platformAdmin() && a.target.TenantID != a.actor.TenantID {
				return errcode.Forbidden
			}
			for _, id := range a.target.RoleIDs {
				if role := a.roles[id]; role != nil && !a.canGrant(role, a.target.TenantID) {
					return errcode.Forbidden
				}
			}
		}
		return fn(r, a)
	})
}

func (a *administration) editableRole(id int64) (*model.SysRole, error) {
	role := a.roles[id]
	if role == nil {
		return nil, errcode.RoleIDInvalid
	}
	if role.TenantID == 0 && role.Name == "admin" {
		return nil, errcode.ModifyBuiltinRoleForbidden
	}
	if !a.canGrant(role, role.TenantID) {
		return nil, errcode.Forbidden
	}
	return role, nil
}

func (a *administration) validateRolePerms(ctx context.Context, tenantID int64, perms string) error {
	// A platform administrator may manage another tenant, but cannot create a
	// wildcard role there. Retain the explicit exact-zero boundary as well.
	if tenantID != 0 {
		ctx = tenant.WithExactTenant(ctx, tenantID)
	}
	if err := ValidateRolePerms(ctx, perms); err != nil {
		return err
	}
	if !a.covers(strings.TrimSpace(perms)) {
		return errcode.Forbidden
	}
	return nil
}
