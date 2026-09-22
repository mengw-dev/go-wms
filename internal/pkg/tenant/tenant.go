// Package tenant 多租户最小版：共享库加列方案。
// 租户 ID 经 JWT → 请求 ctx 传播 → GORM 全局回调自动注入 WHERE tenant_id = ?。
// ctx 中无租户（或为 0）视为平台旁路：迁移、种子、演示重置等场景不注入隔离条件。
package tenant

import "context"

type ctxKey string

const tenantCtxKey ctxKey = "tenant_id"
const exactScopeKey ctxKey = "exact_tenant_scope"

// WithExactTenant 限定单一租户，包括 tenantID=0。用于 API Key 等不应具有平台旁路权限的入口。
// 租户 ID 必须非负；调用入口负责校验。后续 WithTenant 不会取消此隔离要求。
func WithExactTenant(ctx context.Context, tenantID int64) context.Context {
	return context.WithValue(WithTenant(ctx, tenantID), exactScopeKey, true)
}

func hasScope(ctx context.Context) bool {
	exact, _ := ctx.Value(exactScopeKey).(bool)
	return exact || FromContext(ctx) > 0
}

// WithTenant 将租户 ID 放入 ctx（覆盖已有值；tenantID <= 0 时清除租户信息）。
func WithTenant(ctx context.Context, tenantID int64) context.Context {
	if tenantID <= 0 {
		return context.WithValue(ctx, tenantCtxKey, int64(0))
	}
	return context.WithValue(ctx, tenantCtxKey, tenantID)
}

// FromContext 取出当前租户 ID；未设置或为 0 时返回 0（平台旁路语义）。
func FromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	v, _ := ctx.Value(tenantCtxKey).(int64)
	return v
}

// Scope 返回当前租户 ID 以及是否要求租户隔离。
// WithTenant(ctx, 0) 或未设置租户时 scoped=false，表示平台旁路；
// WithExactTenant(ctx, 0) 时 scoped=true 且 tenantID=0，表示只能访问平台租户数据。
func Scope(ctx context.Context) (tenantID int64, scoped bool) {
	return FromContext(ctx), hasScope(ctx)
}
