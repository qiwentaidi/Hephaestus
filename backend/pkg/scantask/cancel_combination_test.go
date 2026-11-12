package scantask

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
)

func TestCancelCombinationTaskSyncsCombinationOnPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("HEPHAESTUS_AGENT_TEST_DSN"))
	if dsn == "" {
		t.Skip("requires HEPHAESTUS_AGENT_TEST_DSN")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "cancel_combo_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = admin.Close()
	})
	connURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := connURL.Query()
	query.Set("search_path", schema)
	connURL.RawQuery = query.Encode()
	conn, err := sql.Open("pgx", connURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.VulnScanTaskItem{}, &database.CombinationTask{}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	combo := database.CombinationTask{
		ID: "combination-1", Name: "combo", Targets: "[]", Nodes: "[]",
		Status: "running", CreatedAt: now, UpdatedAt: now,
	}
	parent := database.VulnScanTask{
		ID: "parent-1", Version: 1, TaskName: "combo", ScanType: "combination",
		Status: "running", ParentCombinationID: combo.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}

	if err := CancelTaskWithDB(db, parent.ID); err != nil {
		t.Fatalf("cancel combination task: %v", err)
	}

	var parentAfter database.VulnScanTask
	if err := db.First(&parentAfter, "id = ? AND version = ?", parent.ID, parent.Version).Error; err != nil {
		t.Fatal(err)
	}
	if parentAfter.Status != "cancelled" {
		t.Fatalf("parent status = %q, want cancelled", parentAfter.Status)
	}
	var comboAfter database.CombinationTask
	if err := db.First(&comboAfter, "id = ?", combo.ID).Error; err != nil {
		t.Fatal(err)
	}
	if comboAfter.Status != "cancelled" {
		t.Fatalf("combination status = %q, want cancelled", comboAfter.Status)
	}
	if comboAfter.CompletedAt == nil || comboAfter.ErrorMsg != "任务已取消" || comboAfter.PauseRequested {
		t.Fatalf("combination cancellation fields not synchronized: %#v", comboAfter)
	}
}

func TestCancelCombinationTaskSyncsCombinationOnSQLite(t *testing.T) {
	db := openSQLite(t, t.TempDir()+"/cancel-combo.db")
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.VulnScanTaskItem{}, &database.CombinationTask{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	combo := database.CombinationTask{
		ID: "combination-1", Name: "combo", Targets: "[]", Nodes: "[]",
		Status: "running", CreatedAt: now, UpdatedAt: now,
	}
	parent := database.VulnScanTask{
		ID: "parent-1", Version: 1, TaskName: "combo", ScanType: "combination",
		Status: "running", ParentCombinationID: combo.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	if err := CancelTaskWithDB(db, parent.ID); err != nil {
		t.Fatal(err)
	}
	var comboAfter database.CombinationTask
	if err := db.First(&comboAfter, "id = ?", combo.ID).Error; err != nil {
		t.Fatal(err)
	}
	if comboAfter.Status != "cancelled" {
		t.Fatalf("combination status = %q, want cancelled", comboAfter.Status)
	}
	if comboAfter.CompletedAt == nil || comboAfter.ErrorMsg != "任务已取消" {
		t.Fatalf("combination cancellation fields not synchronized: %#v", comboAfter)
	}
}
