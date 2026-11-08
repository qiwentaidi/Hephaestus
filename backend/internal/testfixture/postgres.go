package testfixture

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
)

// PostgresWorker 创建隔离 schema 的 PostgreSQL 任务/worker 夹具。
// 未设置 HEPHAESTUS_AGENT_TEST_DSN 时跳过。
func PostgresWorker(t *testing.T) (*pentest.SQLStore, *gorm.DB, pentest.Conversation, pentest.WorkerTurn) {
	t.Helper()
	dsn := os.Getenv("HEPHAESTUS_AGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL fixture")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "worker_handler_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	db, err := sql.Open("pgx", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := pentest.NewSQLStoreWithDialect(db, "postgres")
	if err = store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = gdb.AutoMigrate(&database.ScanAgentNode{}, &database.VulnScanTask{}, &database.VulnScanTaskItem{}, &database.ScanSchedule{}, &database.ScanScheduleOccurrence{}); err != nil {
		t.Fatal(err)
	}
	if err = gdb.Create(&database.ScanAgentNode{AgentID: "fixture-worker", Enabled: true, AIWorker: true, AIConcurrency: 1, Status: "online", LastHeartbeat: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Mode: pentest.SoloPentestMode, Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnqueueWorkerMessage(ctx, pentest.Message{ConversationID: conv.ID, Content: "Run only a local cancellation fixture; no network probes."}, "fixture", "fixture"); err != nil {
		t.Fatal(err)
	}
	turn, err := store.ClaimWorkerTurn(ctx, "fixture-worker", 30*time.Second)
	if err != nil || turn == nil {
		t.Fatalf("claim %v %v", turn, err)
	}
	return store, gdb, conv, *turn
}
