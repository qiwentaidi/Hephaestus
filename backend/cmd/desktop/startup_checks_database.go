package main

import (
	"fmt"
	"strings"

	"hephaestus-backend/pkg/config"
)

func desktopPostgresConnectionFailureMessage(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	dbConfig, configErr := config.GetDatabaseConfig()
	if configErr != nil {
		return "", false
	}
	return desktopPostgresConnectionFailureMessageForConfig(err, dbConfig)
}

func desktopPostgresConnectionFailureMessageForConfig(err error, dbConfig config.DatabaseConfig) (string, bool) {
	if err == nil || !isPostgresDatabaseType(dbConfig.Type) || !isPostgresConnectionError(err) {
		return "", false
	}

	host := strings.TrimSpace(dbConfig.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(dbConfig.Port)
	if port == "" {
		port = "5432"
	}
	return fmt.Sprintf("数据库连接失败：无法连接 PostgreSQL（%s:%s）。", host, port), true
}

func isPostgresDatabaseType(databaseType string) bool {
	switch strings.ToLower(strings.TrimSpace(databaseType)) {
	case "postgres", "postgresql":
		return true
	default:
		return false
	}
}

func isPostgresConnectionError(err error) bool {
	return strings.Contains(err.Error(), "无法连接到数据库服务器") ||
		strings.Contains(err.Error(), "连接 PostgreSQL 服务器失败") ||
		strings.Contains(err.Error(), "连接数据库失败")
}
