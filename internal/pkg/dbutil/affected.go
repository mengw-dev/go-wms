package dbutil

import "gorm.io/gorm"

// RequireAffected 校验按主键修改/删除的执行结果：
// 执行报错原样返回；影响 0 行视为记录不存在，返回 gorm.ErrRecordNotFound。
// DryRun 模式（只生成 SQL、不真正执行）下没有行数概念，跳过断言。
//
// 只用于语义明确的普通 CRUD（修改/删除 ID=xxx 对象），由调用方把
// gorm.ErrRecordNotFound 转成对应业务错误；
// 条件 CAS（WHERE id=? AND version=?）与 worker 抢占（WHERE status=?）等
// “0 行有业务含义”的语句不要使用。
func RequireAffected(res *gorm.DB) error {
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 && !res.DryRun {
		return gorm.ErrRecordNotFound
	}
	return nil
}
