package tenant

import (
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// RegisterGORMCallbacks 在 gorm.DB 上注册全局租户回调：
//   - Create Before：ctx 有租户且字段为零值时自动填充 tenant_id
//   - Query / Row / Update / Delete Before：ctx 有租户时注入 WHERE tenant_id = ?
//
// 注意 Scan()/Rows() 走的是 Row 回调链（gorm:row）而非 Query 链，
// Model(&T{}).Scan(&dest) 的聚合/投影查询必须同时注册 Row 链才会被过滤。
// 判定"表是否有租户列"用 stmt.Schema.LookUpField 反射（按 Go 字段名/列名匹配），
// 不维护表名清单；Table("xxx 别名") 的裸表查询无 Schema，天然绕过（由调用方自行保证隔离）。
// ctx 租户 = 0 一律跳过（平台旁路：AutoMigrate、种子、演示重置、后台补偿等）。
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
	tenantID := FromContext(db.Statement.Context)
	if tenantID <= 0 {
		return
	}
	db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: clause.Column{Table: clause.CurrentTable, Name: field.DBName}, Value: tenantID},
	}})
}

// fillTenantOnCreate Create 前自动填充：仅当 ctx 有租户且记录的 TenantID 为零值时写入，
// 不覆盖显式指定的租户（如平台代操作指定租户建数据）。
func fillTenantOnCreate(db *gorm.DB) {
	if db.Statement == nil || db.Statement.Schema == nil {
		return
	}
	field := tenantField(db.Statement)
	if field == nil {
		return
	}
	tenantID := FromContext(db.Statement.Context)
	if tenantID <= 0 {
		return
	}
	switch db.Statement.ReflectValue.Kind() {
	case reflect.Struct:
		setTenantIfZero(db.Statement.ReflectValue, field, tenantID)
	case reflect.Slice, reflect.Array:
		// 指针切片（[]*T）的元素在 setTenantIfZero 内解引用处理
		for i := range db.Statement.ReflectValue.Len() {
			setTenantIfZero(db.Statement.ReflectValue.Index(i), field, tenantID)
		}
	}
}

func setTenantIfZero(row reflect.Value, field *schema.Field, tenantID int64) {
	if !row.IsValid() {
		return
	}
	// 切片元素可能是指针（[]*T 是 GORM 常见批量写法），解引用后再判断
	if row.Kind() == reflect.Pointer {
		if row.IsNil() {
			return
		}
		row = row.Elem()
	}
	if row.Kind() != reflect.Struct {
		return
	}
	fv := row.FieldByIndex(field.StructField.Index)
	if fv.CanSet() && fv.Kind() == reflect.Int64 && fv.Int() == 0 {
		fv.SetInt(tenantID)
	}
}
