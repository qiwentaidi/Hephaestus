package nodepython

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"hephaestus-backend/pkg/database"
)

func TestNodePythonPostgresValidationAndIsolation(t *testing.T) {
	dsn := os.Getenv("HEPHAESTUS_AGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := "python_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); db, _ := admin.DB(); db.Close() })
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close() })
	if err = db.AutoMigrate(&database.AgentPythonConfig{}); err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python required")
	}
	for _, id := range []string{"node-a", "node-b"} {
		if err = db.Create(&database.AgentPythonConfig{NodeKey: id, Mode: "auto", RequestID: uuid.NewString(), RequestMode: "manual", RequestPath: python, Save: true, Status: "pending"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if err = Process(ctx, db, "node-a"); err != nil {
		t.Fatal(err)
	}
	var a, b database.AgentPythonConfig
	db.First(&a, "node_key=?", "node-a")
	db.First(&b, "node_key=?", "node-b")
	if a.Status != "ready" || a.Path != python || a.Version == "" || b.Status != "pending" {
		t.Fatalf("cross-node state: %+v %+v", a, b)
	}
	if err = db.Model(&a).Updates(map[string]any{"request_id": uuid.NewString(), "request_path": "/missing-python", "status": "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = Process(ctx, db, "node-a"); err != nil {
		t.Fatal(err)
	}
	db.First(&a, "node_key=?", "node-a")
	if a.Status != "error" || a.Path != python || a.Error == "" {
		t.Fatalf("failed validation replaced applied config: %+v", a)
	}
	if err = db.Model(&a).Updates(map[string]any{"request_id": uuid.NewString(), "request_mode": "auto", "request_path": "", "save": false, "status": "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = Process(ctx, db, "node-a"); err != nil {
		t.Fatal(err)
	}
	db.First(&a, "node_key=?", "node-a")
	if a.Status != "ready" || a.Mode != "manual" || a.Path != python {
		t.Fatalf("detect-only changed configuration: %+v", a)
	}
	if Key(database.ScanAgentNode{AgentID: "boot-1", LocalWorker: true, LastSeenHost: "host"}) != Key(database.ScanAgentNode{AgentID: "boot-2", LocalWorker: true, LastSeenHost: "host"}) {
		t.Fatal("local setting lost across restart")
	}
}
