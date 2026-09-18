// Package tenant 多租户最小版：共享库加列方案。
// 租户 ID 经 JWT → 请求 ctx 传播 → GORM 全局回调自动注入 WHERE tenant_id = ?。
// ctx 中无租户（或为 0）视为平台旁路：迁移、种子、演示重置等场景不注入隔离条件。
package tenant

import "context"

type ctxKey string

const tenantCtxKey ctxKey = "tenant_id"

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
