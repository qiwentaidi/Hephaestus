package collect

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hephaestus-backend/pkg/types"

	_ "github.com/go-sql-driver/mysql"          // MySQL driver
	_ "github.com/lib/pq"                       // PostgreSQL driver
	_ "github.com/microsoft/go-mssqldb"         // MSSQL driver
	"github.com/redis/go-redis/v9"              // Redis driver
	_ "github.com/sijms/go-ora/v2"              // Oracle driver
	"go.mongodb.org/mongo-driver/mongo"         // MongoDB driver
	"go.mongodb.org/mongo-driver/mongo/options" // MongoDB options
)

// TestConnection 测试数据库连接
func TestConnection(conn types.DatabaseConnection) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch conn.Scheme {
	case "mysql":
		return testMySQL(ctx, conn)
	case "postgres":
		return testPostgres(ctx, conn)
	case "mssql":
		return testMSSQL(ctx, conn)
	case "oracle":
		return testOracle(ctx, conn)
	case "mongodb":
		return testMongoDB(ctx, conn)
	case "redis":
		return testRedis(ctx, conn)
	default:
		return fmt.Errorf("不支持的数据库类型: %s", conn.Scheme)
	}
}

// testMySQL 测试 MySQL 连接
func testMySQL(ctx context.Context, conn types.DatabaseConnection) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/mysql?charset=utf8&timeout=%s",
		conn.Username, conn.Password, conn.Host, conn.Port, "10s")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("无法连接MySQL: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("MySQL连接测试失败: %w", err)
	}
	return nil
}

// testPostgres 测试 PostgreSQL 连接
func testPostgres(ctx context.Context, conn types.DatabaseConnection) error {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/postgres?sslmode=disable&connect_timeout=10",
		conn.Username, conn.Password, conn.Host, conn.Port)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("无法连接PostgreSQL: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("PostgreSQL连接测试失败: %w", err)
	}
	return nil
}

// testMSSQL 测试 MSSQL 连接
func testMSSQL(ctx context.Context, conn types.DatabaseConnection) error {
	dsn := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%d;connection timeout=10;encrypt=disable",
		conn.Host, conn.Username, conn.Password, conn.Port)
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return fmt.Errorf("无法连接MSSQL: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("MSSQL连接测试失败: %w", err)
	}
	return nil
}

// testOracle 测试 Oracle 连接
func testOracle(ctx context.Context, conn types.DatabaseConnection) error {
	dsn := fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
		conn.Username, conn.Password, conn.Host, conn.Port, conn.ServerName)
	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return fmt.Errorf("无法连接Oracle: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("Oracle连接测试失败: %w", err)
	}
	return nil
}

// testMongoDB 测试 MongoDB 连接
func testMongoDB(ctx context.Context, conn types.DatabaseConnection) error {
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%d",
		conn.Username, conn.Password, conn.Host, conn.Port)
	if conn.Username == "" && conn.Password == "" {
		uri = fmt.Sprintf("mongodb://%s:%d", conn.Host, conn.Port)
	}

	clientOptions := options.Client().ApplyURI(uri)
	clientOptions.SetConnectTimeout(10 * time.Second)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("无法连接MongoDB: %w", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			// 忽略断开连接的错误
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("MongoDB连接测试失败: %w", err)
	}
	return nil
}

// testRedis 测试 Redis 连接
func testRedis(ctx context.Context, conn types.DatabaseConnection) error {
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

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis连接测试失败: %w", err)
	}
	return nil
}
