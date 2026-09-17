// Package concurrent 提供 goroutine 安全运行工具。
//
// Go 的语义是 goroutine 内 panic 会终止整个进程，而不是只杀 goroutine。
// SafeGo 包装 defer recover() + 日志，避免后台任务因 panic 拖垮服务。
package concurrent

import (
	"context"
	"runtime/debug"

	"gowms/internal/pkg/log"
)

// SafeGo 启动一个带 panic 恢复的 goroutine。
// recover 后仅记录错误日志，不会影响其他 goroutine 或主进程。
// 可选 ctx 用于在日志中关联 request_id / user_id（不取消 goroutine）。
func SafeGo(ctx context.Context, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.WithContext(ctx).Error("goroutine panic recovered",
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}

// SafeGoNoCtx 启动无 ctx 关联的 panic 恢复 goroutine（用于无需请求上下文的后台任务）。
func SafeGoNoCtx(fn func()) {
	SafeGo(context.Background(), fn)
}
