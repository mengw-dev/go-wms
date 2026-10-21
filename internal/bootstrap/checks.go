package bootstrap

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type quantityCheck struct {
	table, name, expression string
	columns                 []string
}

// 表达式采用 MySQL 保存的括号结构；只忽略空白、反引号和整式外围括号，不抹去运算优先级。
var quantityChecks = []quantityCheck{
	{"wms_inventory", "chk_inv_non_negative", "(available_quantity >= 0) AND (stock_quantity >= 0)", []string{"available_quantity", "stock_quantity"}},
	{"wms_inventory", "chk_inv_allocated_non_negative", "allocated_quantity >= 0", []string{"allocated_quantity"}},
	{"wms_inventory", "chk_inv_quantity_balance", "stock_quantity = (available_quantity + allocated_quantity)", []string{"stock_quantity", "available_quantity", "allocated_quantity"}},
	{"wms_task", "chk_task_quantity_progress", "(target_qty >= 0) AND (done_qty >= 0) AND (done_qty <= target_qty)", []string{"target_qty", "done_qty"}},
	{"wms_allocation", "chk_allocation_quantity_progress", "(allocated_qty >= 0) AND (picked_qty >= 0) AND (picked_qty <= allocated_qty)", []string{"allocated_qty", "picked_qty"}},
	{"wms_shipment_order", "chk_shipment_quantity_progress", "(expected_qty >= 0) AND (allocated_qty >= 0) AND (picked_qty >= 0) AND (picked_qty <= allocated_qty) AND (allocated_qty <= expected_qty)", []string{"expected_qty", "allocated_qty", "picked_qty"}},
	{"wms_inventory_alert_rule", "chk_alert_min_qty", "min_available_qty > 0", []string{"min_available_qty"}},
}

// AuditQuantityChecks 只读审计全部租户及软删行，并拒绝同名异定义或未启用的 CHECK。
// 用于停写后的开发建表/正式迁移前检查；旧版本尚未建立的表或列留给后续迁移。
// 不自动修账；独立 DDL 仍会验证实际数据，不能把预检当成跨表迁移事务。
func AuditQuantityChecks(db *gorm.DB) error {
	return auditQuantityChecks(db, true)
}

// VerifyQuantityChecks 仅核对正式启动所需的约束，不执行 DDL 或扫描业务数据。
// 必须在迁移完成后运行，缺失、定义不符或未启用均停止启动。
func VerifyQuantityChecks(db *gorm.DB) error {
	for _, check := range quantityChecks {
		exists, err := checkDefinition(db, check)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("missing check %s.%s; run cmd/migrate up before startup", check.table, check.name)
		}
	}
	return nil
}

func auditQuantityChecks(db *gorm.DB, allowMissing bool) error {
	for _, check := range quantityChecks {
		var columns []string
		if err := db.Raw(`SELECT COLUMN_NAME FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name IN ?`, check.table, check.columns).
			Scan(&columns).Error; err != nil { // schema 元数据检查，不存在 tenant_id 行过滤。
			return fmt.Errorf("inspect columns for %s: %w", check.name, err)
		}
		if len(columns) != len(check.columns) {
			if allowMissing {
				continue
			}
			return fmt.Errorf("missing table or columns for %s.%s", check.table, check.name)
		}
		if _, err := checkDefinition(db, check); err != nil {
			return err
		}
		var invalid bool
		query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM `%s` WHERE NOT (%s))", check.table, check.expression)
		if err := db.Raw(query).Scan(&invalid).Error; err != nil { // 启动审计全部 tenant_id 和软删行；标识符来自固定清单。
			return fmt.Errorf("audit %s.%s: %w", check.table, check.name, err)
		}
		if invalid {
			return fmt.Errorf("quantity check audit failed for %s.%s; reconcile existing data before migration (no data repaired)", check.table, check.name)
		}
	}
	return nil
}

func ensureQuantityChecks(db *gorm.DB) error {
	// 所有数据、已有定义都先审计，避免后面才发现脏数据而先加了一部分约束。
	if err := auditQuantityChecks(db, false); err != nil {
		return err
	}
	for _, check := range quantityChecks {
		exists, err := checkDefinition(db, check)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		query := fmt.Sprintf("ALTER TABLE `%s` ADD CONSTRAINT `%s` CHECK (%s)", check.table, check.name, check.expression)
		if err := db.Exec(query).Error; err != nil {
			var driverErr *mysqldriver.MySQLError
			// MySQL 3822 仅可能是并发建同名 CHECK；仍须回查正确表、表达式及 ENFORCED。
			if !errors.As(err, &driverErr) || driverErr.Number != 3822 {
				return fmt.Errorf("add check %s.%s: %w", check.table, check.name, err)
			}
			if valid, verifyErr := checkDefinition(db, check); verifyErr != nil || !valid {
				return fmt.Errorf("add check %s.%s (duplicate not verified): %w", check.table, check.name, errors.Join(err, verifyErr))
			}
		}
		if exists, err := checkDefinition(db, check); err != nil || !exists {
			return fmt.Errorf("check %s.%s not enforced after DDL: %w", check.table, check.name, errors.Join(errors.New("missing or invalid CHECK"), err))
		}
	}
	return nil
}

func checkDefinition(db *gorm.DB, check quantityCheck) (bool, error) {
	var rows []struct{ CheckClause, Enforced string }
	err := db.Raw(`SELECT cc.CHECK_CLAUSE, tc.ENFORCED
		FROM information_schema.table_constraints tc
		JOIN information_schema.check_constraints cc ON cc.constraint_schema = tc.constraint_schema AND cc.constraint_name = tc.constraint_name
		WHERE tc.table_schema = DATABASE() AND tc.table_name = ? AND tc.constraint_name = ? AND tc.constraint_type = 'CHECK'`, check.table, check.name).
		Scan(&rows).Error // schema 定义检查，与业务 tenant_id 无关。
	if err != nil {
		return false, fmt.Errorf("inspect check %s.%s: %w", check.table, check.name, err)
	}
	if len(rows) == 0 {
		return false, nil
	}
	if len(rows) != 1 || rows[0].Enforced != "YES" || normalizeCheck(rows[0].CheckClause) != normalizeCheck(check.expression) {
		return false, fmt.Errorf("check definition mismatch or NOT ENFORCED: %s.%s", check.table, check.name)
	}
	return true, nil
}

func normalizeCheck(expression string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '`' {
			return -1
		}
		return unicode.ToLower(r)
	}, expression)
	for len(s) > 1 && s[0] == '(' && s[len(s)-1] == ')' {
		depth, outer := 0, true
		for i, r := range s {
			switch r {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == 0 && i != len(s)-1 {
				outer = false
				break
			}
		}
		if !outer || depth != 0 {
			break
		}
		s = s[1 : len(s)-1]
	}
	return s
}
