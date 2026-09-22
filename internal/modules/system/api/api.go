// Package api 定义 system 模块提供给其他模块的身份、权限和审计能力。
package api

import (
	"gowms/internal/pkg/middleware"
)

// SystemAPI system 模块对外暴露的接口（供中间件与 app 组装使用）。
type SystemAPI interface {
	middleware.AuthValidator   // ValidateToken：用户状态与 Token 版本校验
	middleware.PermsChecker    // HasPerm：权限校验
	middleware.OperLogRecorder // Record：操作日志异步落库
}
