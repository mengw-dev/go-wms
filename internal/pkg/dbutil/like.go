// Package dbutil 提供 GORM/SQL 相关的小工具。
package dbutil

import "strings"

// LikePattern 转义用户输入的 LIKE 模式通配符，避免 % 和 _ 被解释为 SQL 通配符。
//
// 用法：
//
//	q.Where("code LIKE ?", "%"+dbutil.LikePattern(keyword)+"%")
//
// 配合 GORM/MySQL 的 ESCAPE 行为：默认反斜杠是转义字符，因此 % 和 _ 前加 \。
// 同时反斜杠自身也要转义，避免用户输入 "\" 破坏模式。
func LikePattern(s string) string {
	repl := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return repl.Replace(s)
}
