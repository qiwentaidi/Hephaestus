// Command collect-data-migrate migrates only database collection payloads.
// It never invokes application startup migrations or fetches source databases.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"

	"hephaestus-backend/pkg/database/collect"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	apply := flag.Bool("apply", false, "实际写入（默认只预检；执行前备份并停止旧版服务）")
	driver := flag.String("driver", "postgres", "postgres 或 sqlite")
	dsnEnv := flag.String("dsn-env", "DATABASE_URL", "保存连接串的环境变量名称（避免密码出现在进程参数中）")
	flag.Parse()
	if err := run(*driver, os.Getenv(*dsnEnv), *apply); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(driver, dsn string, apply bool) error {
	if dsn == "" {
		return fmt.Errorf("连接串环境变量为空，请通过 -dsn-env 指定已设置的环境变量")
	}
	var dialector gorm.Dialector
	switch driver {
	case "postgres":
		dialector = postgres.Open(dsn)
	case "sqlite":
		// mode=rw prevents a mistyped file path creating an empty database.
		// Require an explicit URI so query characters in paths are unambiguous.
		uri, err := url.Parse(dsn)
		if err != nil || uri.Scheme != "file" || uri.Query().Get("mode") != "rw" {
			return fmt.Errorf("SQLite 请提供 file:/绝对路径?mode=rw 形式的 URI")
		}
		dialector = sqlite.Open(dsn)
	default:
		return fmt.Errorf("不支持的数据库类型: %s", driver)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("连接数据库失败，请核对连接配置")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	report, migrateErr := collect.MigrateCollectData(ctx, db, apply)
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return err
	}
	if migrateErr != nil {
		return migrateErr
	}
	if report.Failed > 0 || report.Conflicted > 0 {
		return fmt.Errorf("存在未迁移记录，原文已保留；请按报告处理后重新运行")
	}
	return nil
}
