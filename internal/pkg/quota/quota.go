// Package quota 公开租户的数据量配额：演示/持久体验账号对外公开，
// 需要在业务写入入口拦住“无限塞数据”，避免单库被撑爆。
//
// 设计约束：
//   - 只对租户 ID > 0 的租户生效；平台租户（tenant_id=0，如 admin、种子数据）不受限；
//   - 配额值来自配置（limits 段），<=0 表示不限制；
//   - 计数查询依赖 GORM 全局租户回调自动注入 WHERE tenant_id=?（见 internal/pkg/tenant/gorm.go）。
package quota

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

// Guard 校验「当前租户已有数量 + 本次新增（add）」不超过上限。
//
// 入参：
//   - ctx：请求上下文，租户 ID 由鉴权中间件写入，决定计数范围；
//   - db：数据源（通常是 s.tm.DB()）；
//   - model：要计数的 GORM 模型（如 &model.ReceiptOrder{}）；
//   - limit：上限条数，<=0 表示不限制；
//   - add：本次将要新增的条数（单条创建传 1，批量创建传批大小）；
//   - label：业务名称，用于错误提示（如“入库单”）。
//
// 返回：达到上限时返回 errcode.QuotaExceeded 包装的动态消息；其余情况返回 nil 或查询错误。
//
// 注意：检查与写入不在同一事务内，并发下允许极小概率的少量超额（公开演示场景可接受）。
func Guard(ctx context.Context, db *gorm.DB, model any, limit, add int, label string) error {
	if limit <= 0 || add <= 0 || db == nil || tenant.FromContext(ctx) <= 0 {
		return nil
	}
	var count int64
	if err := db.WithContext(ctx).Model(model).Count(&count).Error; err != nil {
		return err
	}
	if count+int64(add) > int64(limit) {
		return errcode.New(errcode.QuotaExceeded.Code,
			fmt.Sprintf("%s数量已达上限（%d 条），请先清理部分数据后再操作", label, limit))
	}
	return nil
}

// GuardImportRows 校验单次批量导入的数据行数不超过上限（Excel 导入是一次能塞最多数据的入口）。
// 平台租户（tenant_id<=0）不受限。
func GuardImportRows(ctx context.Context, rows, limit int) error {
	if limit <= 0 || rows <= limit || tenant.FromContext(ctx) <= 0 {
		return nil
	}
	return errcode.New(errcode.QuotaExceeded.Code,
		fmt.Sprintf("单次导入行数（%d）超过上限（%d），请拆分文件后重试", rows, limit))
}
