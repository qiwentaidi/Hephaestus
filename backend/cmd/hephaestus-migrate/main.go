// Command hephaestus-migrate runs the Hephaestus startup database migration
// offline. It exists so an already-installed desktop client that aborts on
// legacy data can be repaired without shipping a new client build.
//
// The tool only performs the same idempotent AutoMigrate the server/desktop
// performs on startup. It never deletes subdomain results; the migration itself
// retires the legacy subdomain_scan_task table.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	_ "hephaestus-backend/pkg/scantask" // registers the pre-AutoMigrate schedule upgrade

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const version = "1.0.0"

type options struct {
	dbPath    string
	dryRun    bool
	assumeYes bool
	noBackup  bool
}

func main() {
	setupConsole()

	opts := options{}
	flag.StringVar(&opts.dbPath, "db", "", "SQLite 数据库文件路径（默认读取桌面端配置）")
	flag.BoolVar(&opts.dryRun, "dry-run", false, "只检查，不修改数据库")
	flag.BoolVar(&opts.assumeYes, "yes", false, "不再询问，直接执行迁移")
	flag.BoolVar(&opts.noBackup, "no-backup", false, "跳过迁移前自动备份")
	flag.Usage = usage
	flag.Parse()

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "\n[失败] %v\n", err)
		pauseIfInteractive(opts.assumeYes)
		os.Exit(1)
	}
	pauseIfInteractive(opts.assumeYes)
}

// pauseIfInteractive keeps the console window open when the tool is started by
// double-clicking, so the result stays readable.
func pauseIfInteractive(assumeYes bool) {
	if assumeYes {
		return
	}
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Print("\n按回车退出... ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, `Hephaestus 数据库迁移工具 v%s

用于修复桌面客户端启动时报“自动迁移表结构失败”的本地数据库。
请在运行前完全退出 Hephaestus 客户端（包括托盘图标和后台进程）。

用法:
  hephaestus-migrate.exe [-db <数据库路径>] [-dry-run] [-yes] [-no-backup]

参数:
`, version)
	flag.PrintDefaults()
	fmt.Fprint(out, `
示例:
  hephaestus-migrate.exe                 # 自动读取桌面端配置并迁移
  hephaestus-migrate.exe -dry-run        # 只检查冲突数量
  hephaestus-migrate.exe -db D:\data\hephaestus.db
`)
}

func run(opts options) error {
	config.SetRuntimeMode(config.RuntimeModeDesktop)

	dbPath, err := resolveDatabasePath(opts.dbPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("数据库文件不存在: %s\n请用 -db 指定实际路径，或先启动一次客户端让程序生成数据库", dbPath)
		}
		return fmt.Errorf("读取数据库文件失败: %w", err)
	}

	fmt.Printf("Hephaestus 数据库迁移工具 v%s\n", version)
	fmt.Printf("数据库: %s\n", dbPath)
	fmt.Printf("大小:   %.2f MB\n\n", float64(info.Size())/(1024*1024))

	db, err := openSQLite(dbPath)
	if err != nil {
		return err
	}
	if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}

	if !db.Migrator().HasTable("vul_scan_task") {
		return fmt.Errorf("该文件不是 Hephaestus 数据库（缺少 vul_scan_task 表），已中止，未做任何修改")
	}

	legacyCount, conflicts, err := inspectLegacyTasks(db)
	if err != nil {
		return fmt.Errorf("检查旧子域名任务失败: %w", err)
	}
	hadLegacyTable := db.Migrator().HasTable("subdomain_scan_task")
	if !hadLegacyTable {
		fmt.Println("未发现旧子域名任务表，数据库可能已经完成迁移。")
	} else {
		fmt.Printf("旧子域名任务:             %d 条\n", legacyCount)
		fmt.Printf("与统一任务类型冲突:       %d 条（会导致客户端启动失败）\n", conflicts)
	}

	if opts.dryRun {
		fmt.Println("\n[dry-run] 检查完成，未修改数据库。")
		return nil
	}

	backupPath := ""
	if !opts.noBackup {
		backupPath, err = backupDatabase(db, dbPath)
		if err != nil {
			return fmt.Errorf("备份数据库失败: %w\n可加 -no-backup 跳过备份后重试", err)
		}
		fmt.Printf("已备份: %s\n", backupPath)
	}

	if !opts.assumeYes {
		fmt.Print("\n即将执行数据库迁移，按回车继续（Ctrl+C 取消）... ")
		if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
			return fmt.Errorf("已取消: %w", err)
		}
	}

	fmt.Println("\n开始迁移...")
	started := time.Now()
	if err := database.AutoMigrate(db); err != nil {
		hint := ""
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "locked") || strings.Contains(lower, "busy") {
			hint = "\n提示: 数据库被占用，请完全退出 Hephaestus（包括托盘图标）后重试。"
		}
		if backupPath != "" {
			hint += "\n数据库备份: " + backupPath
		}
		return fmt.Errorf("迁移失败: %w%s", err, hint)
	}
	fmt.Printf("迁移完成，用时 %s\n", time.Since(started).Round(time.Millisecond))

	if db.Migrator().HasTable("subdomain_scan_task") {
		return fmt.Errorf("迁移已结束但旧子域名任务表仍然存在，请把本次输出和备份文件反馈给支持人员")
	}
	remainingLegacy, remainingConflicts, err := inspectLegacyTasks(db)
	if err != nil {
		return err
	}
	if remainingLegacy != 0 || remainingConflicts != 0 {
		return fmt.Errorf("迁移后仍有旧数据残留（旧任务 %d 条，冲突 %d 条）", remainingLegacy, remainingConflicts)
	}

	if hadLegacyTable {
		fmt.Println("旧子域名任务表已清理，历史子域名结果已保留。")
	} else {
		fmt.Println("数据库结构已是最新，历史子域名结果已保留。")
	}
	fmt.Println("现在可以重新启动 Hephaestus 客户端。")
	return nil
}

// resolveDatabasePath mirrors the desktop client: an explicit -db wins,
// otherwise the configured SQLite path (or the desktop default) is used.
func resolveDatabasePath(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return filepath.Abs(strings.TrimSpace(override))
	}

	dbConfig, err := config.GetDatabaseConfig()
	if err != nil {
		return "", fmt.Errorf("读取桌面端数据库配置失败: %w", err)
	}
	dbType := strings.ToLower(strings.TrimSpace(dbConfig.Type))
	if dbType != "" && dbType != config.DatabaseTypeSQLite && dbType != "sqlite3" {
		return "", fmt.Errorf("当前配置的数据库类型是 %q，本工具只处理 SQLite 桌面数据库；PostgreSQL 请使用修复版服务端或手动执行 SQL", dbConfig.Type)
	}

	dbPath := strings.TrimSpace(dbConfig.Path)
	if dbPath == "" {
		dbPath = config.GetDefaultSQLitePath()
	}
	return filepath.Abs(dbPath)
}

func openSQLite(dbPath string) (*gorm.DB, error) {
	// Same pragmas as pkg/database so the offline run behaves like startup.
	dsn := fmt.Sprintf(
		"%s?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL&_synchronous=NORMAL&_txlock=immediate",
		dbPath,
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 数据库失败: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 SQLite 连接失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("连接 SQLite 数据库失败: %w", err)
	}
	return db, nil
}

// inspectLegacyTasks returns the number of legacy subdomain tasks and how many
// of them share an ID with a non-subdomain unified task. The second count is
// exactly what used to abort the startup migration.
func inspectLegacyTasks(db *gorm.DB) (int64, int64, error) {
	if !db.Migrator().HasTable("subdomain_scan_task") {
		return 0, 0, nil
	}

	var legacyCount int64
	if err := db.Table("subdomain_scan_task").Count(&legacyCount).Error; err != nil {
		return 0, 0, err
	}

	var conflicts int64
	err := db.Raw(`
		SELECT COUNT(*)
		FROM subdomain_scan_task st
		JOIN (
			SELECT id, MAX(version) AS max_version
			FROM vul_scan_task
			GROUP BY id
		) latest ON latest.id = st.id
		JOIN vul_scan_task vt
			ON vt.id = latest.id AND vt.version = latest.max_version
		WHERE lower(coalesce(vt.scan_type, '')) <> 'subdomain'
	`).Scan(&conflicts).Error
	if err != nil {
		return 0, 0, err
	}
	return legacyCount, conflicts, nil
}

func backupDatabase(db *gorm.DB, dbPath string) (string, error) {
	backupPath := dbPath + ".before-migrate-" + time.Now().Format("20060102-150405")
	if _, err := os.Stat(backupPath); err == nil {
		backupPath = fmt.Sprintf("%s-%d", backupPath, time.Now().UnixNano())
	}

	// VACUUM INTO produces a consistent copy without needing the -wal file.
	if err := db.Exec("VACUUM INTO ?", backupPath).Error; err == nil {
		return backupPath, nil
	}

	// Fallback for environments where VACUUM INTO is unavailable. Only safe
	// while the client is closed, which is the documented precondition.
	_ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error
	if err := copyFile(dbPath, backupPath); err != nil {
		return "", err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + suffix); err == nil {
			_ = copyFile(dbPath+suffix, backupPath+suffix)
		}
	}
	return backupPath, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
