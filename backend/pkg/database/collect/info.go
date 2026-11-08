package collect

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/types"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/microsoft/go-mssqldb"
	"github.com/redis/go-redis/v9"
	_ "github.com/sijms/go-ora/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	defaultRedisDBCount = 16
	redisScanCount      = 200
	redisKeySamplePerDB = 500
)

// GetDatabaseInfo 获取数据库信息（数据库列表和表列表）
func GetDatabaseInfo(conn types.DatabaseConnection) (types.DatabaseInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch conn.Scheme {
	case "mysql":
		return getMySQLInfo(ctx, conn)
	case "postgres":
		return getPostgresInfo(ctx, conn)
	case "mssql":
		return getMSSQLInfo(ctx, conn)
	case "oracle":
		return getOracleInfo(ctx, conn)
	case "mongodb":
		return getMongoDBInfo(ctx, conn)
	case "redis":
		return getRedisInfo(ctx, conn)
	default:
		return nil, fmt.Errorf("不支持的数据库类型: %s", conn.Scheme)
	}
}

// getMySQLInfo 获取 MySQL 数据库信息
func getMySQLInfo(ctx context.Context, conn types.DatabaseConnection) (types.DatabaseInfo, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/mysql?charset=utf8&timeout=%s",
		conn.Username, conn.Password, conn.Host, conn.Port, "10s")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("无法连接MySQL: %w", err)
	}
	defer db.Close()

	info := make(types.DatabaseInfo)

	// 系统数据库列表（跳过）
	systemDBs := []string{"performance_schema", "information_schema", "mysql", "sys"}

	// 获取所有数据库
	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, fmt.Errorf("查询数据库列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			continue
		}

		// 跳过系统数据库
		isSystem := false
		for _, sysDB := range systemDBs {
			if dbName == sysDB {
				isSystem = true
				break
			}
		}
		if isSystem {
			continue
		}

		// 获取该数据库的表列表
		tables, err := getMySQLTables(ctx, db, dbName)
		if err != nil {
			continue
		}
		if len(tables) > 0 {
			info[dbName] = tables
		}
	}

	return info, nil
}

// getMySQLTables 获取 MySQL 表列表
func getMySQLTables(ctx context.Context, db *sql.DB, dbName string) ([]string, error) {
	if err := validateSQLIdentifier(dbName, "database name"); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SHOW TABLES FROM %s", quoteSQLIdentifier(dbName, "`", "`"))
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}

// getPostgresInfo 获取 PostgreSQL 数据库信息
func getPostgresInfo(ctx context.Context, conn types.DatabaseConnection) (types.DatabaseInfo, error) {
	// 先连接到 postgres 数据库获取数据库列表
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/postgres?sslmode=disable&connect_timeout=10",
		conn.Username, conn.Password, conn.Host, conn.Port)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("无法连接PostgreSQL: %w", err)
	}
	defer db.Close()

	info := make(types.DatabaseInfo)

	// 系统数据库列表（跳过）
	systemDBs := []string{"pg_catalog", "template0", "template1", "information_schema", "postgres", "pg_toast"}

	// 获取所有数据库
	rows, err := db.QueryContext(ctx, "SELECT datname FROM pg_database WHERE datistemplate = false")
	if err != nil {
		return nil, fmt.Errorf("查询数据库列表失败: %w", err)
	}
	defer rows.Close()

	var dbNames []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			continue
		}

		// 跳过系统数据库
		isSystem := false
		for _, sysDB := range systemDBs {
			if dbName == sysDB {
				isSystem = true
				break
			}
		}
		if isSystem {
			continue
		}

		dbNames = append(dbNames, dbName)
	}

	// 为每个数据库获取表列表
	for _, dbName := range dbNames {
		tables, err := getPostgresTables(ctx, conn, dbName)
		if err != nil {
			continue
		}
		if len(tables) > 0 {
			info[dbName] = tables
		}
	}

	return info, nil
}

// getPostgresTables 获取 PostgreSQL 表列表
func getPostgresTables(ctx context.Context, conn types.DatabaseConnection, dbName string) ([]string, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		conn.Host, conn.Port, conn.Username, conn.Password, dbName)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'"
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}

// getMSSQLInfo 获取 MSSQL 数据库信息
func getMSSQLInfo(ctx context.Context, conn types.DatabaseConnection) (types.DatabaseInfo, error) {
	dsn := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%d;connection timeout=10;encrypt=disable",
		conn.Host, conn.Username, conn.Password, conn.Port)
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("无法连接MSSQL: %w", err)
	}
	defer db.Close()

	info := make(types.DatabaseInfo)

	// 系统数据库列表（跳过）
	systemDBs := []string{"master", "tempdb", "model", "msdb"}

	// 获取所有数据库
	query := "SELECT name FROM sys.databases WHERE name NOT IN ('master', 'tempdb', 'model', 'msdb')"
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询数据库列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			continue
		}

		// 跳过系统数据库
		isSystem := false
		for _, sysDB := range systemDBs {
			if dbName == sysDB {
				isSystem = true
				break
			}
		}
		if isSystem {
			continue
		}

		// 获取该数据库的表列表
		tables, err := getMSSQLTables(ctx, db, dbName)
		if err != nil {
			continue
		}
		if len(tables) > 0 {
			info[dbName] = tables
		}
	}

	return info, nil
}

// getMSSQLTables 获取 MSSQL 表列表
func getMSSQLTables(ctx context.Context, db *sql.DB, dbName string) ([]string, error) {
	if err := validateSQLIdentifier(dbName, "database name"); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT TABLE_NAME FROM %s.INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE = 'BASE TABLE'", quoteSQLIdentifier(dbName, "[", "]"))
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}

// getOracleInfo 获取 Oracle 数据库信息
func getOracleInfo(ctx context.Context, conn types.DatabaseConnection) (types.DatabaseInfo, error) {
	dsn := fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
		conn.Username, conn.Password, conn.Host, conn.Port, conn.ServerName)
	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return nil, fmt.Errorf("无法连接Oracle: %w", err)
	}
	defer db.Close()

	info := make(types.DatabaseInfo)

	// Oracle 系统用户列表（跳过）
	oracleSystemSchemas := []string{
		"SYS", "SYSTEM", "SYSAUX", "OUTLN", "GSMADMIN_INTERNAL", "DVSYS",
		"DVF", "OLAPSYS", "DBSNMP", "APPQOSSYS", "WMSYS", "OJVMSYS",
		"CTXSYS", "XDB", "ANONYMOUS", "EXFSYS", "LBACSYS", "MDDATA",
		"MDSYS", "MGMT_VIEW", "ORDPLUGINS", "ORDSYS", "SI_INFORMTN_SCHEMA",
		"FLOWS_FILES", "SCOTT", "APEX_PUBLIC_USER", "SPATIAL_CSW_ADMIN_USR",
		"SPATIAL_WFS_ADMIN_USR", "XS$NULL",
	}

	// 获取所有用户（模式）
	query := "SELECT USERNAME FROM ALL_USERS"
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询用户列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var schemaName string
		if err := rows.Scan(&schemaName); err != nil {
			continue
		}

		// 跳过系统用户
		isSystem := false
		schemaUpper := strings.ToUpper(schemaName)
		for _, sysSchema := range oracleSystemSchemas {
			if schemaUpper == sysSchema {
				isSystem = true
				break
			}
		}
		if isSystem {
			continue
		}

		// 获取该用户的表列表
		tables, err := getOracleTables(ctx, db, schemaName)
		if err != nil {
			continue
		}
		if len(tables) > 0 {
			info[schemaName] = tables
		}
	}

	return info, nil
}

// getOracleTables 获取 Oracle 表列表
func getOracleTables(ctx context.Context, db *sql.DB, schemaName string) ([]string, error) {
	if err := validateSQLIdentifier(schemaName, "schema name"); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT TABLE_NAME FROM ALL_TABLES WHERE OWNER = '%s'", schemaName)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}

// getMongoDBInfo 获取 MongoDB 数据库信息
func getMongoDBInfo(ctx context.Context, conn types.DatabaseConnection) (types.DatabaseInfo, error) {
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%d",
		conn.Username, conn.Password, conn.Host, conn.Port)
	if conn.Username == "" && conn.Password == "" {
		uri = fmt.Sprintf("mongodb://%s:%d", conn.Host, conn.Port)
	}

	clientOptions := options.Client().ApplyURI(uri)
	clientOptions.SetConnectTimeout(10 * time.Second)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("无法连接MongoDB: %w", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			// 忽略断开连接的错误
		}
	}()

	// 获取所有数据库名称
	dbNames, err := client.ListDatabaseNames(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("查询数据库列表失败: %w", err)
	}

	info := make(types.DatabaseInfo)

	// 系统数据库列表（跳过）
	systemDBs := []string{"admin", "config", "local"}

	for _, dbName := range dbNames {
		// 跳过系统数据库
		isSystem := false
		for _, sysDB := range systemDBs {
			if dbName == sysDB {
				isSystem = true
				break
			}
		}
		if isSystem {
			continue
		}

		// 获取该数据库的集合（表）列表
		collections, err := getMongoDBCollections(ctx, client, dbName)
		if err != nil {
			continue
		}
		if len(collections) > 0 {
			info[dbName] = collections
		}
	}

	return info, nil
}

// getMongoDBCollections 获取 MongoDB 集合列表
func getMongoDBCollections(ctx context.Context, client *mongo.Client, dbName string) ([]string, error) {
	db := client.Database(dbName)
	collections, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	// 转换为字符串切片
	var result []string
	for _, col := range collections {
		result = append(result, col)
	}

	return result, nil
}

// getRedisInfo 获取 Redis 信息（逻辑库 -> key 列表）
func getRedisInfo(ctx context.Context, conn types.DatabaseConnection) (types.DatabaseInfo, error) {
	info := make(types.DatabaseInfo)
	dbCount := getRedisDBCount(ctx, conn)

	for dbIndex := 0; dbIndex < dbCount; dbIndex++ {
		keys, err := getRedisKeysByDB(ctx, conn, dbIndex)
		if err != nil {
			// Redis Cluster 仅支持 DB 0，遇到 DB 选择错误后直接停止遍历
			if dbIndex > 0 && isRedisDBSelectUnsupported(err) {
				break
			}
			if dbIndex == 0 {
				return nil, fmt.Errorf("查询Redis key失败: %w", err)
			}
			continue
		}

		if len(keys) == 0 {
			continue
		}

		info[fmt.Sprintf("db%d", dbIndex)] = keys
	}

	return info, nil
}

func getRedisDBCount(ctx context.Context, conn types.DatabaseConnection) int {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", conn.Host, conn.Port),
		Username:     conn.Username,
		Password:     conn.Password,
		DB:           0,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})
	defer func() {
		_ = client.Close()
	}()

	config, err := client.ConfigGet(ctx, "databases").Result()
	if err != nil {
		return defaultRedisDBCount
	}

	if raw, ok := config["databases"]; ok {
		dbCount, parseErr := strconv.Atoi(raw)
		if parseErr == nil && dbCount > 0 {
			return dbCount
		}
	}

	return defaultRedisDBCount
}

func getRedisKeysByDB(ctx context.Context, conn types.DatabaseConnection, dbIndex int) ([]string, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", conn.Host, conn.Port),
		Username:     conn.Username,
		Password:     conn.Password,
		DB:           dbIndex,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})
	defer func() {
		_ = client.Close()
	}()

	var (
		cursor uint64
		keys   []string
	)

	for {
		batch, nextCursor, err := client.Scan(ctx, cursor, "*", redisScanCount).Result()
		if err != nil {
			return nil, err
		}

		keys = append(keys, batch...)
		if len(keys) >= redisKeySamplePerDB {
			keys = keys[:redisKeySamplePerDB]
			break
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	sort.Strings(keys)
	return keys, nil
}

func isRedisDBSelectUnsupported(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "select is not allowed in cluster mode") ||
		strings.Contains(message, "db index is out of range")
}
