package collect

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

var sqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func validateSQLIdentifier(value, label string) error {
	value = strings.TrimSpace(value)
	if !sqlIdentifierPattern.MatchString(value) {
		return fmt.Errorf("invalid %s", label)
	}
	return nil
}

func quoteSQLIdentifier(value, quoteStart, quoteEnd string) string {
	return quoteStart + value + quoteEnd
}

func validateSQLTableReference(schemaName, tableName string) error {
	if err := validateSQLIdentifier(schemaName, "schema/database name"); err != nil {
		return err
	}
	return validateSQLIdentifier(tableName, "table name")
}

// GetTableInfo 获取表数据信息（列、前3行数据、总行数）
func GetTableInfo(conn types.DatabaseConnection, schemaName, tableName string) (types.RowData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch conn.Scheme {
	case "mysql":
		if err := validateSQLTableReference(schemaName, tableName); err != nil {
			return types.RowData{}, err
		}
		return getMySQLTableInfo(ctx, conn, schemaName, tableName)
	case "postgres":
		if err := validateSQLTableReference(schemaName, tableName); err != nil {
			return types.RowData{}, err
		}
		return getPostgresTableInfo(ctx, conn, schemaName, tableName)
	case "mssql":
		if err := validateSQLTableReference(schemaName, tableName); err != nil {
			return types.RowData{}, err
		}
		return getMSSQLTableInfo(ctx, conn, schemaName, tableName)
	case "oracle":
		if err := validateSQLTableReference(schemaName, tableName); err != nil {
			return types.RowData{}, err
		}
		return getOracleTableInfo(ctx, conn, schemaName, tableName)
	case "mongodb":
		return getMongoDBTableInfo(ctx, conn, schemaName, tableName)
	case "redis":
		return getRedisTableInfo(ctx, conn, schemaName, tableName)
	default:
		return types.RowData{}, fmt.Errorf("不支持的数据库类型: %s", conn.Scheme)
	}
}

// getMySQLTableInfo 获取 MySQL 表信息
func getMySQLTableInfo(ctx context.Context, conn types.DatabaseConnection, schemaName, tableName string) (types.RowData, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8&timeout=%s",
		conn.Username, conn.Password, conn.Host, conn.Port, schemaName, "10s")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return types.RowData{}, fmt.Errorf("无法连接MySQL: %w", err)
	}
	defer db.Close()

	return getSQLTableInfo(ctx, db, quoteSQLIdentifier(schemaName, "`", "`"), quoteSQLIdentifier(tableName, "`", "`"), "SELECT COUNT(*) FROM %s", "SELECT * FROM %s LIMIT 3")
}

// getPostgresTableInfo 获取 PostgreSQL 表信息
func getPostgresTableInfo(ctx context.Context, conn types.DatabaseConnection, schemaName, tableName string) (types.RowData, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		conn.Host, conn.Port, conn.Username, conn.Password, schemaName)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return types.RowData{}, fmt.Errorf("无法连接PostgreSQL: %w", err)
	}
	defer db.Close()

	return getSQLTableInfo(ctx, db, quoteSQLIdentifier(schemaName, `"`, `"`), quoteSQLIdentifier(tableName, `"`, `"`), "SELECT COUNT(*) FROM %s", "SELECT * FROM %s LIMIT 3")
}

// getMSSQLTableInfo 获取 MSSQL 表信息
func getMSSQLTableInfo(ctx context.Context, conn types.DatabaseConnection, schemaName, tableName string) (types.RowData, error) {
	dsn := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%d;database=%s;connection timeout=10;encrypt=disable",
		conn.Host, conn.Username, conn.Password, conn.Port, schemaName)
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return types.RowData{}, fmt.Errorf("无法连接MSSQL: %w", err)
	}
	defer db.Close()

	return getSQLTableInfo(ctx, db, quoteSQLIdentifier(schemaName, "[", "]"), quoteSQLIdentifier(tableName, "[", "]"), "SELECT COUNT(*) FROM %s", "SELECT TOP 3 * FROM %s")
}

// getOracleTableInfo 获取 Oracle 表信息
func getOracleTableInfo(ctx context.Context, conn types.DatabaseConnection, schemaName, tableName string) (types.RowData, error) {
	dsn := fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
		conn.Username, conn.Password, conn.Host, conn.Port, conn.ServerName)
	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return types.RowData{}, fmt.Errorf("无法连接Oracle: %w", err)
	}
	defer db.Close()

	return getSQLTableInfo(ctx, db, quoteSQLIdentifier(schemaName, `"`, `"`), quoteSQLIdentifier(tableName, `"`, `"`), "SELECT COUNT(*) FROM %s", "SELECT * FROM %s FETCH FIRST 3 ROWS ONLY")
}

// getSQLTableInfo 通用 SQL 数据库表信息获取
func getSQLTableInfo(ctx context.Context, db *sql.DB, schemaName, tableName, countQuery, selectQuery string) (types.RowData, error) {
	result := types.RowData{}

	// 构建表名（带或不带 schema）
	var fullTableName string
	if schemaName != "" && schemaName != tableName {
		fullTableName = fmt.Sprintf("%s.%s", schemaName, tableName)
	} else {
		fullTableName = tableName
	}

	// 获取列名 - 使用 WHERE 1=0 的方式，兼容所有数据库
	var columns []string
	query := fmt.Sprintf("SELECT * FROM %s WHERE 1=0", fullTableName)
	rows, err := db.QueryContext(ctx, query)
	if err == nil {
		columns, err = rows.Columns()
		rows.Close()
	}

	if err != nil || len(columns) == 0 {
		// 如果无法获取列名，返回空结果
		return types.RowData{
			Columns:   []string{},
			Rows:      [][]interface{}{},
			RowsCount: 0,
		}, nil
	}

	result.Columns = columns

	// 获取总行数
	var rowCount int
	countSQL := fmt.Sprintf(countQuery, fullTableName)
	if err := db.QueryRowContext(ctx, countSQL).Scan(&rowCount); err == nil {
		result.RowsCount = rowCount
	}

	// 获取前3行数据
	selectSQL := fmt.Sprintf(selectQuery, fullTableName)
	rows, err = db.QueryContext(ctx, selectSQL)
	if err != nil {
		return result, fmt.Errorf("查询表数据失败: %w", err)
	}
	defer rows.Close()

	// 解析行数据
	data := make([][]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make([]interface{}, len(columns))
		for i, v := range values {
			// 处理 NULL 值和其他类型
			if v == nil {
				row[i] = nil
			} else {
				row[i] = normalizeDatabaseCell(v)
			}
		}
		data = append(data, row)
	}

	result.Rows = data
	return result, nil
}

// getMongoDBTableInfo 获取 MongoDB 集合（表）信息
func getMongoDBTableInfo(ctx context.Context, conn types.DatabaseConnection, dbName, collectionName string) (types.RowData, error) {
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%d",
		conn.Username, conn.Password, conn.Host, conn.Port)
	if conn.Username == "" && conn.Password == "" {
		uri = fmt.Sprintf("mongodb://%s:%d", conn.Host, conn.Port)
	}

	clientOptions := options.Client().ApplyURI(uri)
	clientOptions.SetConnectTimeout(10 * time.Second)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return types.RowData{}, fmt.Errorf("无法连接MongoDB: %w", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			// 忽略断开连接的错误
		}
	}()

	result := types.RowData{}
	collection := client.Database(dbName).Collection(collectionName)

	// 获取总文档数
	count, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return types.RowData{}, fmt.Errorf("获取文档数量失败: %w", err)
	}
	result.RowsCount = int(count)

	// 获取前3个文档
	cur, err := collection.Find(ctx, bson.M{}, options.Find().SetLimit(3))
	if err != nil {
		return types.RowData{}, fmt.Errorf("查询文档失败: %w", err)
	}
	defer cur.Close(ctx)

	var documents []bson.M
	if err = cur.All(ctx, &documents); err != nil {
		return types.RowData{}, fmt.Errorf("解析文档失败: %w", err)
	}

	// 从文档中提取所有可能的字段名作为列
	columnSet := make(map[string]bool)
	for _, doc := range documents {
		for key := range doc {
			columnSet[key] = true
		}
	}

	columns := make([]string, 0)
	for key := range columnSet {
		columns = append(columns, key)
	}
	result.Columns = columns

	// 将文档转换为行数据
	rows := make([][]interface{}, 0)
	for _, doc := range documents {
		row := make([]interface{}, len(columns))
		for i, col := range columns {
			if val, ok := doc[col]; ok {
				// 将 BSON 值转换为可序列化的类型
				convertedVal := convertMongoValue(val)
				// 如果是字符串类型，限制最大长度（防止内容过长）
				if str, ok := convertedVal.(string); ok && len(str) > 10000 {
					// 超过 10000 字符的字符串截断到 10000 字符
					row[i] = str[:10000] + "...[已截断，原始长度: " + fmt.Sprintf("%d", len(str)) + " 字符]"
				} else {
					row[i] = convertedVal
				}
			} else {
				row[i] = nil
			}
		}
		rows = append(rows, row)
	}

	result.Rows = rows
	return result, nil
}

// getRedisTableInfo 获取 Redis key 信息
func getRedisTableInfo(ctx context.Context, conn types.DatabaseConnection, schemaName, keyName string) (types.RowData, error) {
	dbIndex := parseRedisDBIndex(schemaName)

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

	keyType, err := client.Type(ctx, keyName).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("获取Redis key类型失败: %w", err)
	}
	if keyType == "none" {
		return types.RowData{
			Columns:   []string{"key", "type", "field", "value"},
			Rows:      [][]interface{}{},
			RowsCount: 0,
		}, nil
	}

	switch keyType {
	case "string":
		return getRedisStringRow(ctx, client, keyName)
	case "hash":
		return getRedisHashRows(ctx, client, keyName)
	case "list":
		return getRedisListRows(ctx, client, keyName)
	case "set":
		return getRedisSetRows(ctx, client, keyName)
	case "zset":
		return getRedisZSetRows(ctx, client, keyName)
	case "stream":
		return getRedisStreamRows(ctx, client, keyName)
	default:
		return types.RowData{
			Columns:   []string{"key", "type", "field", "value"},
			Rows:      [][]interface{}{{keyName, keyType, "unsupported", fmt.Sprintf("暂不支持类型 %s 的采样", keyType)}},
			RowsCount: 1,
		}, nil
	}
}

func getRedisStringRow(ctx context.Context, client *redis.Client, key string) (types.RowData, error) {
	value, err := client.Get(ctx, key).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis string失败: %w", err)
	}

	return types.RowData{
		Columns:   []string{"key", "type", "field", "value"},
		RowsCount: 1,
		Rows:      [][]interface{}{{key, "string", "value", truncateRedisString(value)}},
	}, nil
}

func getRedisHashRows(ctx context.Context, client *redis.Client, key string) (types.RowData, error) {
	total, err := client.HLen(ctx, key).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis hash长度失败: %w", err)
	}

	items, _, err := client.HScan(ctx, key, 0, "*", 3).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis hash样本失败: %w", err)
	}

	rows := make([][]interface{}, 0, len(items)/2)
	for i := 0; i+1 < len(items); i += 2 {
		rows = append(rows, []interface{}{key, "hash", items[i], truncateRedisString(items[i+1])})
	}

	return types.RowData{
		Columns:   []string{"key", "type", "field", "value"},
		RowsCount: int(total),
		Rows:      rows,
	}, nil
}

func getRedisListRows(ctx context.Context, client *redis.Client, key string) (types.RowData, error) {
	total, err := client.LLen(ctx, key).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis list长度失败: %w", err)
	}

	values, err := client.LRange(ctx, key, 0, 2).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis list样本失败: %w", err)
	}

	rows := make([][]interface{}, 0, len(values))
	for idx, value := range values {
		rows = append(rows, []interface{}{key, "list", fmt.Sprintf("[%d]", idx), truncateRedisString(value)})
	}

	return types.RowData{
		Columns:   []string{"key", "type", "field", "value"},
		RowsCount: int(total),
		Rows:      rows,
	}, nil
}

func getRedisSetRows(ctx context.Context, client *redis.Client, key string) (types.RowData, error) {
	total, err := client.SCard(ctx, key).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis set长度失败: %w", err)
	}

	values, err := client.SRandMemberN(ctx, key, 3).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis set样本失败: %w", err)
	}

	rows := make([][]interface{}, 0, len(values))
	for idx, value := range values {
		rows = append(rows, []interface{}{key, "set", fmt.Sprintf("member_%d", idx+1), truncateRedisString(value)})
	}

	return types.RowData{
		Columns:   []string{"key", "type", "field", "value"},
		RowsCount: int(total),
		Rows:      rows,
	}, nil
}

func getRedisZSetRows(ctx context.Context, client *redis.Client, key string) (types.RowData, error) {
	total, err := client.ZCard(ctx, key).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis zset长度失败: %w", err)
	}

	values, err := client.ZRangeWithScores(ctx, key, 0, 2).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis zset样本失败: %w", err)
	}

	rows := make([][]interface{}, 0, len(values))
	for _, item := range values {
		rows = append(rows, []interface{}{
			key,
			"zset",
			fmt.Sprintf("%v", item.Member),
			item.Score,
		})
	}

	return types.RowData{
		Columns:   []string{"key", "type", "field", "value"},
		RowsCount: int(total),
		Rows:      rows,
	}, nil
}

func getRedisStreamRows(ctx context.Context, client *redis.Client, key string) (types.RowData, error) {
	total, err := client.XLen(ctx, key).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis stream长度失败: %w", err)
	}

	streamItems, err := client.XRangeN(ctx, key, "-", "+", 3).Result()
	if err != nil {
		return types.RowData{}, fmt.Errorf("读取Redis stream样本失败: %w", err)
	}

	rows := make([][]interface{}, 0, len(streamItems))
	for _, item := range streamItems {
		rows = append(rows, []interface{}{
			key,
			"stream",
			item.ID,
			redisValueToString(item.Values),
		})
	}

	return types.RowData{
		Columns:   []string{"key", "type", "field", "value"},
		RowsCount: int(total),
		Rows:      rows,
	}, nil
}

func parseRedisDBIndex(schemaName string) int {
	clean := strings.ToLower(strings.TrimSpace(schemaName))
	if clean == "" {
		return 0
	}

	if strings.HasPrefix(clean, "db") {
		if idx, err := strconv.Atoi(strings.TrimPrefix(clean, "db")); err == nil && idx >= 0 {
			return idx
		}
	}

	if idx, err := strconv.Atoi(clean); err == nil && idx >= 0 {
		return idx
	}

	return 0
}

func redisValueToString(value interface{}) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return truncateRedisString(fmt.Sprintf("%v", value))
	}
	return truncateRedisString(string(raw))
}

func truncateRedisString(value string) string {
	const maxLen = 10000
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "...[已截断]"
}

// convertMongoValue 将 MongoDB 值转换为可序列化的类型
func convertMongoValue(val interface{}) interface{} {
	switch v := val.(type) {
	case bson.M:
		// 将 BSON 文档转换为 JSON 字符串
		return fmt.Sprintf("%v", v)
	case []interface{}:
		// 将数组转换为 JSON 字符串
		return fmt.Sprintf("%v", v)
	case bson.D:
		return fmt.Sprintf("%v", v)
	default:
		return normalizeDatabaseCell(v)
	}
}

// SQL drivers return text as []byte. Distinguish actual bytes here instead of
// asking the browser to guess whether an arbitrary Redis/string value is Base64.
func normalizeDatabaseCell(value interface{}) interface{} {
	switch v := value.(type) {
	case []byte:
		if utf8.Valid(v) {
			return string(v)
		}
		return map[string]string{"encoding": "base64", "data": base64.StdEncoding.EncodeToString(v)}
	case int64:
		if v > 9007199254740991 || v < -9007199254740991 {
			return strconv.FormatInt(v, 10)
		}
	case uint64:
		if v > 9007199254740991 {
			return strconv.FormatUint(v, 10)
		}
	}
	return value
}
