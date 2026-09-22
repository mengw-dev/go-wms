// Package repository 提供仓库、库位和 SKU 的持久化操作及删除引用检查。
package repository

type Repository struct{}

func New() *Repository { return &Repository{} }
