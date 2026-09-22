package tenant

import (
	"errors"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// ErrMismatch 表示写入数据的租户与当前操作上下文不一致。
var ErrMismatch = errors.New("tenant does not match operation context")

// RegisterGORMCallbacks 在 gorm.DB 上注册全局租户回调：
//   - Create Before：ctx 有租户时自动填充 tenant_id，并拒绝显式写入其他租户
//   - Query / Row / Update / Delete Before：ctx 有租户时注入 WHERE tenant_id = ?
//
// 注意 Scan()/Rows() 走的是 Row 回调链（gorm:row）而非 Query 链，
// Model(&T{}).Scan(&dest) 的聚合/投影查询必须同时注册 Row 链才会被过滤。
// 判定"表是否有租户列"用 stmt.Schema.LookUpField 反射（按 Go 字段名/列名匹配），
// 不维护表名清单；Table("xxx 别名") 的裸表查询无 Schema，天然绕过（由调用方自行保证隔离）。
// 通常 ctx 租户 = 0 跳过；WithExactTenant 明确要求隔离时，0 也注入查询条件。
func RegisterGORMCallbacks(db *gorm.DB) error {
	if err := db.Callback().Create().Before("gorm:create").Register("tenant:fill_create", fillTenantOnCreate); err != nil {
		return err
	}
	if err := db.Callback().Query().Before("gorm:query").Register("tenant:scope_query", tenantWhereScope); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").Register("tenant:scope_row", tenantWhereScope); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("tenant:scope_update", tenantWhereScope); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("tenant:scope_delete", tenantWhereScope); err != nil {
		return err
	}
	return nil
}

// tenantField 返回该表模型上的 TenantID 字段描述；无该列返回 nil。
func tenantField(stmt *gorm.Statement) *schema.Field {
	if stmt == nil || stmt.Schema == nil {
		return nil
	}
	return stmt.Schema.LookUpField("TenantID")
}

// tenantWhereScope Query/Update/Delete 统一注入：WHERE tenant_id = ?。
// 条件用 clause.CurrentTable 限定主表名，避免 JOIN 场景（关联表也有 tenant_id）列名歧义。
func tenantWhereScope(db *gorm.DB) {
	if db.Statement == nil {
		return
	}
	field := tenantField(db.Statement)
	if field == nil {
		return
	}
	tenantID, scoped := Scope(db.Statement.Context)
	if !scoped {
		return
	}
	db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: clause.Column{Table: clause.CurrentTable, Name: field.DBName}, Value: tenantID},
	}})
}

// fillTenantOnCreate 在有租户的上下文中只能创建该租户的数据。
// 平台代操作必须使用平台上下文，并在记录上显式指定租户。
func fillTenantOnCreate(db *gorm.DB) {
	if db.Statement == nil || db.Statement.Schema == nil {
		return
	}
	field := tenantField(db.Statement)
	if field == nil {
		return
	}
	tenantID, scoped := Scope(db.Statement.Context)
	if !scoped {
		return
	}
	switch db.Statement.ReflectValue.Kind() {
	case reflect.Struct:
		if err := setTenantIfZero(db.Statement.ReflectValue, field, tenantID); err != nil {
			_ = db.AddError(err)
		}
	case reflect.Slice, reflect.Array:
		// 指针切片（[]*T）的元素在 setTenantIfZero 内解引用处理
		for i := range db.Statement.ReflectValue.Len() {
			if err := setTenantIfZero(db.Statement.ReflectValue.Index(i), field, tenantID); err != nil {
				_ = db.AddError(err)
				return
			}
		}
	}
}

func setTenantIfZero(row reflect.Value, field *schema.Field, tenantID int64) error {
	if !row.IsValid() {
		return nil
	}
	// 切片元素可能是指针（[]*T 是 GORM 常见批量写法），解引用后再判断
	if row.Kind() == reflect.Pointer {
		if row.IsNil() {
			return nil
		}
		row = row.Elem()
	}
	if row.Kind() != reflect.Struct {
		return nil
	}
	fv := row.FieldByIndex(field.StructField.Index)
	if fv.Kind() == reflect.Int64 && fv.Int() != 0 && fv.Int() != tenantID {
		return ErrMismatch
	}
	if fv.CanSet() && fv.Kind() == reflect.Int64 && fv.Int() == 0 {
		fv.SetInt(tenantID)
	}
	return nil
}
