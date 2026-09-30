package service

import (
	"context"
	"strings"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

// 租户角色权限注册表：普通租户只能授予这里登记过的权限，防止通过角色管理自我提权。
// 权限串必须与各模块 handler 的 middleware.Permission 参数、前端角色配置页保持一致；
// 新增业务权限时先在这里登记，否则租户角色无法授予该权限。
const (
	PermBasic     = "wms:basic"
	PermInventory = "wms:inventory"
	PermTask      = "wms:task"

	PermInboundView    = "wms:inbound:view"
	PermInboundCreate  = "wms:inbound:create"
	PermInboundSubmit  = "wms:inbound:submit"
	PermInboundApprove = "wms:inbound:approve"
	PermInboundCancel  = "wms:inbound:cancel"
	PermInboundReceive = "wms:inbound:receive"
	PermInboundPutaway = "wms:inbound:putaway"

	PermOutboundView    = "wms:outbound:view"
	PermOutboundCreate  = "wms:outbound:create"
	PermOutboundSubmit  = "wms:outbound:submit"
	PermOutboundApprove = "wms:outbound:approve"
	PermOutboundCancel  = "wms:outbound:cancel"
	PermOutboundPick    = "wms:outbound:pick"

	PermStocktakeView    = "wms:stocktake:view"
	PermStocktakeCreate  = "wms:stocktake:create"
	PermStocktakeCount   = "wms:stocktake:stocktake"
	PermStocktakeApprove = "wms:stocktake:approve"
	PermStocktakeCancel  = "wms:stocktake:cancel"

	PermSystemUser = "wms:system:user"
	PermSystemRole = "wms:system:role"
	PermSystemLog  = "wms:system:log"
)

// allowedTenantPerms 是租户角色可授予权限的判定集合。
// wms:demo 不在其中：它是演示租户种子专用的权限，普通租户持有会被前端误判为演示账号。
var allowedTenantPerms = map[string]struct{}{
	PermBasic:     {},
	PermInventory: {},
	PermTask:      {},

	PermInboundView:    {},
	PermInboundCreate:  {},
	PermInboundSubmit:  {},
	PermInboundApprove: {},
	PermInboundCancel:  {},
	PermInboundReceive: {},
	PermInboundPutaway: {},

	PermOutboundView:    {},
	PermOutboundCreate:  {},
	PermOutboundSubmit:  {},
	PermOutboundApprove: {},
	PermOutboundCancel:  {},
	PermOutboundPick:    {},

	PermStocktakeView:    {},
	PermStocktakeCreate:  {},
	PermStocktakeCount:   {},
	PermStocktakeApprove: {},
	PermStocktakeCancel:  {},

	PermSystemUser: {},
	PermSystemRole: {},
	PermSystemLog:  {},
}

// IsAllowedTenantPermission 判断权限串是否在租户可授予的白名单内。
func IsAllowedTenantPermission(perm string) bool {
	_, ok := allowedTenantPerms[perm]
	return ok
}

// ValidateRolePerms 校验角色权限串是否符合当前请求的授权边界：
// 平台旁路（tenant_id=0，平台管理员维护平台角色）保留 * 语义；
// 普通租户（含精确租户 0）只允许注册表内的权限，拒绝 *、未注册权限与空权限项。
// 全空白权限串表示“无权限角色”，允许保存。
func ValidateRolePerms(ctx context.Context, perms string) error {
	if _, scoped := tenant.Scope(ctx); !scoped || strings.TrimSpace(perms) == "" {
		return nil
	}
	for _, item := range strings.Split(perms, ",") {
		if p := strings.TrimSpace(item); p == "" || !IsAllowedTenantPermission(p) {
			return errcode.RolePermNotAllowed
		}
	}
	return nil
}
