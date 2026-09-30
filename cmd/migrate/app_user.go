package main

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

// appUserPattern 限制应用账户名的字符集，避免把环境变量直接拼进 SQL 标识符/字面量。
var appUserPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)

// ensureAppUser 幂等创建/修复 MySQL 应用账户并授予目标库权限。
//
// MySQL 官方镜像只在首次初始化数据目录时创建 MYSQL_USER：已有 mysql_data 数据卷
// 升级到“应用改用 wms_app”后，该账户可能不存在或权限/密码与当前 .env 不一致。
// 这里用 root（管理）连接补齐账户、修正密码并重申授权；只操作账户与授权，
// 不触碰业务数据，也不重建任何 schema，因此可以安全地重复执行。
func ensureAppUser(db *sql.DB, dsn string) error {
	user := strings.TrimSpace(os.Getenv("MYSQL_USER"))
	if user == "" {
		user = "wms_app"
	}
	if !appUserPattern.MatchString(user) {
		return fmt.Errorf("invalid MYSQL_USER %q: only letters, digits, '_', '.', '-' are allowed", user)
	}
	password := os.Getenv("MYSQL_PASSWORD")
	if password == "" {
		return fmt.Errorf("MYSQL_PASSWORD is required: it is the password for the application account %q", user)
	}

	cfg, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse admin dsn: %w", err)
	}
	database := strings.TrimSpace(cfg.DBName)
	if database == "" {
		return fmt.Errorf("database name is missing from the admin dsn")
	}
	if strings.ContainsRune(database, '`') {
		return fmt.Errorf("database name %q contains an unsupported character", database)
	}

	// 三条语句都幂等：账户不存在则创建；存在则把密码修正为当前配置；
	// 授权重复执行无副作用，只保证权限范围与官方镜像首次初始化时一致。
	statements := []string{
		fmt.Sprintf("CREATE USER IF NOT EXISTS %s@'%%' IDENTIFIED BY %s", quoteLiteral(user), quoteLiteral(password)),
		fmt.Sprintf("ALTER USER %s@'%%' IDENTIFIED BY %s", quoteLiteral(user), quoteLiteral(password)),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO %s@'%%'", database, quoteLiteral(user)),
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("ensure app user: %w", err)
		}
	}
	return nil
}

// quoteLiteral 按 MySQL 默认 sql_mode 的字符串字面量规则转义（反斜杠与单引号）。
func quoteLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `''`)
	return "'" + value + "'"
}
