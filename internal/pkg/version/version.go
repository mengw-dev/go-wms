// Package version 提供构建版本、提交号和构建时间信息。
package version

import "fmt"

// 由构建参数注入；本地开发默认值不会伪装成正式版本。
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// String 返回适合健康检查和日志展示的版本摘要。
func String() string {
	return fmt.Sprintf("%s (commit=%s, build_time=%s)", Version, Commit, BuildTime)
}
