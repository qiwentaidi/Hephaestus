package main

import (
	"errors"
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestDesktopPostgresConnectionFailureMessage(t *testing.T) {
	message, ok := desktopPostgresConnectionFailureMessageForConfig(
		errors.New("无法连接到数据库服务器: 连接 PostgreSQL 服务器失败"),
		config.DatabaseConfig{Type: "postgres", Host: "192.0.2.10", Port: "5432"},
	)
	if !ok {
		t.Fatal("expected PostgreSQL connection failure to be recognized")
	}
	if message != "数据库连接失败：无法连接 PostgreSQL（192.0.2.10:5432）。" {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestDesktopPostgresConnectionFailureMessageIgnoresOtherErrors(t *testing.T) {
	if _, ok := desktopPostgresConnectionFailureMessageForConfig(errors.New("自动迁移失败"), config.DatabaseConfig{Type: "postgres"}); ok {
		t.Fatal("expected non-connection error to be ignored")
	}
	if _, ok := desktopPostgresConnectionFailureMessageForConfig(errors.New("连接数据库失败"), config.DatabaseConfig{Type: "sqlite"}); ok {
		t.Fatal("expected SQLite error to be ignored")
	}
}
