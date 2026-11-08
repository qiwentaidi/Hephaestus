package database

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestBuildPostgresDSNIncludesConnectTimeout(t *testing.T) {
	dsn := buildPostgresDSN("127.0.0.1", "5432", "postgres", "secret", "hephaestus", "disable")
	want := "connect_timeout=" + strconv.Itoa(int(postgresConnectTimeout/time.Second))
	if !strings.Contains(dsn, want) {
		t.Fatalf("buildPostgresDSN() = %q, want %q", dsn, want)
	}
}

func TestStartupMigrationHasIndependentFiveMinuteBudget(t *testing.T) {
	db := openMigrationTestDB(t)
	connectionCtx, cancelConnection := context.WithTimeout(context.Background(), postgresConnectionReadyTimeout)
	cancelConnection()

	// Observe actual migration SQL, not just the timeout constant. Even a
	// canceled connection context must not leak into the migration phase.
	var migrationCtx context.Context
	if err := db.Callback().Raw().Before("gorm:raw").Register("test:migration_deadline", func(tx *gorm.DB) {
		if migrationCtx != nil {
			return
		}
		migrationCtx = tx.Statement.Context
		deadline, ok := migrationCtx.Deadline()
		if !ok {
			t.Error("migration SQL has no deadline")
			return
		}
		if remaining := time.Until(deadline); remaining < 5*time.Minute-5*time.Second || remaining > 5*time.Minute {
			t.Errorf("migration budget = %s, want a fresh five minutes", remaining)
		}
		if err := migrationCtx.Err(); err != nil {
			t.Errorf("migration inherited canceled connection context: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := migrateStartupDatabase(db.WithContext(connectionCtx)); err != nil {
		t.Fatalf("migration after connection cancellation: %v", err)
	}
	if migrationCtx == nil {
		t.Fatal("no migration SQL observed")
	}
	if migrationCtx.Err() != context.Canceled {
		t.Fatal("migration context was not released after startup")
	}
	if err := db.Exec("SELECT 1").Error; err != nil {
		t.Fatalf("runtime database inherited startup cancellation: %v", err)
	}
	if !db.Migrator().HasTable("user") {
		t.Fatal("startup migration did not create the application schema")
	}
}

func TestEnsureDatabaseExistsStopsWhenPostgresHandshakeStalls(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverDone := make(chan struct{})
	defer close(serverDone)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		<-serverDone
	}()

	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener address: %v", err)
	}

	const testConnectTimeout = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	started := time.Now()
	err = ensureDatabaseExistsWithTimeout(
		ctx,
		host,
		port,
		"postgres",
		"postgres",
		"hephaestus",
		"disable",
		testConnectTimeout,
	)
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("ensureDatabaseExists() unexpectedly succeeded against a stalled server")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("ensureDatabaseExists() took %s; connect timeout was not honored", elapsed)
	}
	if !strings.Contains(err.Error(), "连接 PostgreSQL 服务器失败") {
		t.Fatalf("ensureDatabaseExists() error = %q, want PostgreSQL connection failure", err)
	}
}
