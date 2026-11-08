package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/config"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	dbInstance *gorm.DB
	dbInitErr  error
	dbOnce     sync.Once
)

const (
	sqliteBusyTimeoutMS            = 5000
	postgresConnectTimeout         = 5 * time.Second
	postgresConnectionReadyTimeout = 30 * time.Second
	databaseMigrationTimeout       = 5 * time.Minute
)

// sqliteMaxOpenConns SQLite 并发读连接数。
// WAL 模式读读/读写互不阻塞，仅写写串行，8 个连接足以让轮询与扫描写入解耦。
const sqliteMaxOpenConns = 8

func parseGormLogLevel() gormlogger.LogLevel {
	// 默认使用 error，避免大量 record not found / 查询日志干扰
	level := strings.ToLower(strings.TrimSpace(os.Getenv("HEPHAESTUS_GORM_LOG_LEVEL")))
	if level == "" {
		level = strings.ToLower(strings.TrimSpace(os.Getenv("GORM_LOG_LEVEL")))
	}
	switch level {
	case "silent":
		return gormlogger.Silent
	case "warn", "warning":
		return gormlogger.Warn
	case "info", "debug":
		return gormlogger.Info
	case "error", "":
		return gormlogger.Error
	default:
		return gormlogger.Error
	}
}

func buildGormConfig() *gorm.Config {
	logLevel := parseGormLogLevel()
	gormLog := gormlogger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             2 * time.Second,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
	return &gorm.Config{
		Logger: gormLog,
	}
}

// GetDB 获取 GORM 数据库实例（单例模式）
func GetDB() (*gorm.DB, error) {
	dbOnce.Do(func() {
		dbConfig, configErr := config.GetDatabaseConfig()
		if configErr != nil {
			dbInitErr = fmt.Errorf("获取数据库配置失败: %w", configErr)
			return
		}

		if dbInitErr = validateDatabaseConfig(dbConfig); dbInitErr != nil {
			return
		}

		ctx := context.Background()
		cancel := func() {}
		if normalizeDatabaseType(dbConfig.Type) == config.DatabaseTypePostgres {
			ctx, cancel = context.WithTimeout(ctx, postgresConnectionReadyTimeout)
		}
		defer cancel()

		dbInstance, dbInitErr = openDatabase(ctx, dbConfig)
		if dbInitErr != nil {
			return
		}

		if config.GetRuntimeMode() == config.RuntimeModeAgent {
			dbInitErr = checkAgentSchema(dbInstance.WithContext(ctx))
			return
		}
		// Connection readiness has its own budget. Migrations must not inherit
		// its deadline (or consume the time already spent connecting).
		cancel()
		if dbInitErr = migrateStartupDatabase(dbInstance); dbInitErr != nil {
			dbInitErr = fmt.Errorf("自动迁移表结构失败: %w", dbInitErr)
			return
		}
	})

	return dbInstance, dbInitErr
}

func migrateStartupDatabase(db *gorm.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), databaseMigrationTimeout)
	defer cancel()
	return AutoMigrate(db.WithContext(ctx))
}

// The primary owns DDL. Re-running all application migrations on every remote
// node needs hundreds of WAN round trips and races the primary's migrations.
func checkAgentSchema(db *gorm.DB) error {
	for _, field := range []string{"UpdateProtocol", "UpdatePending", "UpdateOS", "UpdateArch"} {
		if !db.Migrator().HasColumn(&ScanAgentNode{}, field) {
			return fmt.Errorf("Agent 更新协调字段尚未就绪，请先启动新版主服务完成迁移")
		}
	}
	for _, table := range []string{"scan_agent_node", "agent_update", "agent_python_config", "vul_scan_task", "vul_scan_task_item", "pentest_worker_turns", "pentest_worker_evidence", "pentest_worker_tool_requests", "pentest_worker_controls", "pentest_worker_evidence_versions"} {
		if !db.Migrator().HasTable(table) {
			return fmt.Errorf("Agent 协调表 %s 尚未就绪，请先启动同版本主服务完成迁移", table)
		}
	}
	if !db.Migrator().HasColumn(&ScanAgentNode{}, "PlatformTokenHash") || !db.Migrator().HasColumn(&ScanAgentNode{}, "PlatformSessionPID") || !db.Migrator().HasColumn(&ScanAgentNode{}, "HostIdentity") || !db.Migrator().HasColumn(&ScanAgentNode{}, "Retired") || !db.Migrator().HasColumn(&ScanAgentNode{}, "AIWorker") || !db.Migrator().HasColumn(&VulnScanTaskItem{}, "WorkerGeneration") {
		return fmt.Errorf("Agent 协调字段尚未就绪，请先启动同版本主服务完成迁移")
	}
	if !db.Migrator().HasColumn(&VulnScanTaskItem{}, "PortScanNoHosts") {
		return fmt.Errorf("端口扫描子任务字段尚未就绪，请先启动同版本主服务完成迁移")
	}
	return nil
}

// GetSQLDB 获取底层 sql.DB 实例（用于需要直接使用 database/sql 的场景）
func GetSQLDB() (*sql.DB, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}
	return db.DB()
}

func openDatabase(ctx context.Context, dbConfig config.DatabaseConfig) (*gorm.DB, error) {
	switch normalizeDatabaseType(dbConfig.Type) {
	case config.DatabaseTypeSQLite:
		return openSQLite(dbConfig)
	case config.DatabaseTypePostgres:
		return openPostgres(ctx, dbConfig)
	default:
		return nil, fmt.Errorf("不支持的数据库类型: %s", dbConfig.Type)
	}
}

func openPostgres(ctx context.Context, dbConfig config.DatabaseConfig) (*gorm.DB, error) {
	sslmode := dbConfig.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}

	if err := ensureDatabaseExists(ctx, dbConfig.Host, dbConfig.Port, dbConfig.User, dbConfig.Password, dbConfig.DBName, sslmode); err != nil {
		return nil, fmt.Errorf("无法连接到数据库服务器: %w", err)
	}

	dsn := buildPostgresDSN(dbConfig.Host, dbConfig.Port, dbConfig.User, dbConfig.Password, dbConfig.DBName, sslmode)

	gormConfig := buildGormConfig()
	// GORM's automatic Ping does not accept a context. Disable it so startup
	// readiness is checked below with the same bounded startup context.
	gormConfig.DisableAutomaticPing = true
	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("打开数据库连接失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层数据库连接失败: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("连接数据库失败 (Ping 测试失败): %w", err)
	}

	return db, nil
}

func openSQLite(dbConfig config.DatabaseConfig) (*gorm.DB, error) {
	dbPath := dbConfig.Path
	if dbPath == "" {
		dbPath = config.GetDefaultSQLitePath()
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("创建 SQLite 数据目录失败: %w", err)
	}

	dsn := buildSQLiteDSN(dbPath)
	db, err := gorm.Open(sqlite.Open(dsn), buildGormConfig())
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 数据库失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 SQLite 底层连接失败: %w", err)
	}

	// PERF: WAL 模式下 SQLite 支持"多读并行 + 单写"，
	// 放开连接池让前端轮询读、心跳、阶段刷新等读操作不再与扫描写入串行排队。
	// 写写冲突由 DSN 中每连接生效的 _busy_timeout 兜底等待。
	sqlDB.SetMaxOpenConns(sqliteMaxOpenConns)
	sqlDB.SetMaxIdleConns(sqliteMaxOpenConns)

	if err := configureSQLiteSession(sqlDB); err != nil {
		return nil, err
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("连接 SQLite 数据库失败: %w", err)
	}

	return db, nil
}

func buildSQLiteDSN(dbPath string) string {
	// PERF: _txlock=immediate 让事务开始即持有写锁，避免多连接下
	// BEGIN DEFERRED 读锁升级写锁的死锁（SQLITE_BUSY 立即返回、不等待 busy_timeout）。
	// GORM 写操作默认包事务，读操作不受影响。
	return fmt.Sprintf(
		"%s?_busy_timeout=%d&_foreign_keys=on&_journal_mode=WAL&_synchronous=NORMAL&_txlock=immediate",
		dbPath,
		sqliteBusyTimeoutMS,
	)
}

func configureSQLiteSession(sqlDB *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		fmt.Sprintf("PRAGMA busy_timeout=%d;", sqliteBusyTimeoutMS),
	}

	for _, pragma := range pragmas {
		if _, err := sqlDB.Exec(pragma); err != nil {
			return fmt.Errorf("初始化 SQLite PRAGMA 失败 (%s): %w", pragma, err)
		}
	}

	return nil
}

func normalizeDatabaseType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "postgres", "postgresql":
		return config.DatabaseTypePostgres
	case "sqlite", "sqlite3":
		return config.DatabaseTypeSQLite
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// ensureDatabaseExists 确保数据库存在，如果不存在则创建
func buildPostgresDSN(host, port, user, password, dbname, sslmode string) string {
	return buildPostgresDSNWithTimeout(host, port, user, password, dbname, sslmode, postgresConnectTimeout)
}

func buildPostgresDSNWithTimeout(host, port, user, password, dbname, sslmode string, connectTimeout time.Duration) string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		host,
		port,
		user,
		password,
		dbname,
		sslmode,
		int(connectTimeout/time.Second),
	)
}

func ensureDatabaseExists(ctx context.Context, host, port, user, password, dbname, sslmode string) error {
	return ensureDatabaseExistsWithTimeout(ctx, host, port, user, password, dbname, sslmode, postgresConnectTimeout)
}

func ensureDatabaseExistsWithTimeout(ctx context.Context, host, port, user, password, dbname, sslmode string, connectTimeout time.Duration) error {
	// 连接到 PostgreSQL 的默认数据库（postgres）来创建目标数据库
	defaultDSN := buildPostgresDSNWithTimeout(host, port, user, password, "postgres", sslmode, connectTimeout)

	adminDB, err := sql.Open("postgres", defaultDSN)
	if err != nil {
		return fmt.Errorf("连接 PostgreSQL 服务器失败（无法打开连接）: %w\n请检查数据库服务是否已启动，以及主机地址和端口是否正确", err)
	}
	defer adminDB.Close()

	// 测试连接（重要：验证数据库服务是否可访问）
	if err = adminDB.PingContext(ctx); err != nil {
		return fmt.Errorf("连接 PostgreSQL 服务器失败（无法建立连接）: %w\n请检查:\n  - 数据库服务是否已启动\n  - 主机地址(%s)和端口(%s)是否正确\n  - 用户名(%s)和密码是否正确\n  - 防火墙是否允许连接", err, host, port, user)
	}

	// 检查数据库是否存在
	var exists bool
	checkQuery := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	err = adminDB.QueryRowContext(ctx, checkQuery, dbname).Scan(&exists)
	if err != nil {
		return fmt.Errorf("检查数据库是否存在失败: %w", err)
	}

	// 如果数据库不存在，创建它
	if !exists {
		// 使用 PostgreSQL 的 quote_ident 函数来安全地转义数据库名
		var quotedDBName string
		err = adminDB.QueryRowContext(ctx, "SELECT quote_ident($1)", dbname).Scan(&quotedDBName)
		if err != nil {
			return fmt.Errorf("转义数据库名失败: %w", err)
		}

		// 创建数据库，使用 UTF8 编码和 template0 作为模板（最干净的模板）
		createQuery := fmt.Sprintf(`CREATE DATABASE %s WITH ENCODING 'UTF8' TEMPLATE template0`, quotedDBName)
		_, err = adminDB.ExecContext(ctx, createQuery)
		if err != nil {
			return fmt.Errorf("创建数据库 %s 失败: %w", dbname, err)
		}
	}

	return nil
}

// validateDatabaseConfig 验证数据库配置是否已填写
func validateDatabaseConfig(dbConfig config.DatabaseConfig) error {
	dbType := normalizeDatabaseType(dbConfig.Type)

	if !config.IsDatabaseTypeAllowed(dbType) {
		return fmt.Errorf("当前运行模式(%s)不允许使用数据库类型: %s", config.GetRuntimeMode(), dbType)
	}

	if dbType == config.DatabaseTypeSQLite {
		if dbConfig.Path == "" {
			dbConfig.Path = config.GetDefaultSQLitePath()
		}
		if dbConfig.Path == "" {
			return fmt.Errorf("SQLite 数据库路径不能为空")
		}
		return nil
	}

	if dbType != config.DatabaseTypePostgres {
		return fmt.Errorf("数据库类型必须为 postgres 或 sqlite，当前值: %s", dbConfig.Type)
	}

	var missingFields []string

	if dbConfig.Host == "" {
		missingFields = append(missingFields, "host")
	}
	if dbConfig.Port == "" {
		missingFields = append(missingFields, "port")
	}
	if dbConfig.User == "" {
		missingFields = append(missingFields, "user")
	}
	if dbConfig.Password == "" {
		missingFields = append(missingFields, "password")
	}
	if dbConfig.DBName == "" {
		missingFields = append(missingFields, "dbname")
	}

	if len(missingFields) > 0 {
		configPath := config.GetResolvedConfigPath()
		return fmt.Errorf(
			"数据库配置不完整，请在配置文件中填写以下字段后重新启动:\n"+
				"  配置文件路径: %s\n"+
				"  缺失字段: %v\n"+
				"  示例配置:\n"+
				"  database:\n"+
				"    type: postgres\n"+
				"    host: localhost\n"+
				"    port: \"5432\"\n"+
				"    user: postgres\n"+
				"    password: your_password\n"+
				"    dbname: hephaestus\n"+
				"    sslmode: disable\n"+
				"  或者:\n"+
				"  database:\n"+
				"    type: sqlite\n"+
				"    path: %s",
			configPath, missingFields, config.GetDefaultSQLitePath())
	}

	return nil
}
