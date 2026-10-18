// Package quantity 定义与正式 MySQL INT 数量字段一致的业务上限。
package quantity

// Max 同时适用于单次数量、单据累计数量及单条库存现存量。
// 聚合报表可以使用 int64 汇总多条库存，不受单行上限限制。
const Max = 2147483647
