package database

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestReconcileCombinationStatusPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("HEPHAESTUS_AGENT_TEST_DSN"))
	if dsn == "" {
		t.Skip("requires HEPHAESTUS_AGENT_TEST_DSN")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "combo_reconcile_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if err := db.AutoMigrate(&VulnScanTask{}, &CombinationTask{}); err != nil {
		t.Fatal(err)
	}

	combos := []CombinationTask{
		{ID: "orphan", Name: "orphan", Targets: "[]", Nodes: "[]", Status: "running"},
		{ID: "cancelled-parent", Name: "cancelled parent", Targets: "[]", Nodes: "[]", Status: "running"},
		{ID: "running-parent", Name: "running parent", Targets: "[]", Nodes: "[]", Status: "running"},
	}
	if err := db.Create(&combos).Error; err != nil {
		t.Fatal(err)
	}
	tasks := []VulnScanTask{
		{ID: "cancelled-parent-task", Version: 1, ScanType: "combination", Status: "cancelled", ParentCombinationID: "cancelled-parent"},
		{ID: "running-parent-task", Version: 1, ScanType: "combination", Status: "running", ParentCombinationID: "running-parent"},
	}
	for index := range tasks {
		if err := db.Create(&tasks[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := reconcileCombinationStatus(db); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"orphan": "cancelled", "cancelled-parent": "cancelled", "running-parent": "running",
	} {
		var combo CombinationTask
		if err := db.First(&combo, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if combo.Status != want {
			t.Fatalf("%s status = %q, want %q", id, combo.Status, want)
		}
	}
}
