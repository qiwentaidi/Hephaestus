package agentruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hephaestus-backend/pkg/database"
)

func TestEnsureAgentTaskItemsPostgresRollsBackOnParentUpdateFailure(t *testing.T) {
	db, _, _ := postgresScanWorkerFixture(t)
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	// This path is also called outside task creation when a worker prepares
	// a pre-existing pending task. All batches and the count must be atomic.
	targets := make([]string, 1500)
	for i := range targets {
		targets[i] = fmt.Sprintf("host-%d.example.test", i)
	}
	targets = append(targets, "", "  ")
	raw, err := json.Marshal(targets)
	if err != nil {
		t.Fatal(err)
	}
	task := database.VulnScanTask{ID: "pending-fixture", Version: 1, ScanMode: "local", ScanType: "portscan", Status: "pending", Targets: string(raw), TotalCount: len(targets)}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected parent update failure")
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail_parent_count", func(tx *gorm.DB) {
		if tx.Statement.Table == "vul_scan_task" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentTaskItems(db, &task); !errors.Is(err, injected) {
		t.Fatalf("expected parent update failure, got %v", err)
	}
	var count int64
	if err := db.Model(&database.VulnScanTaskItem{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed preparation left %d items", count)
	}
	if err := db.Callback().Update().Remove("test:fail_parent_count"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentTaskItems(db, &task); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.VulnScanTaskItem{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", task.ID).First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1500 || task.TotalCount != 1500 {
		t.Fatalf("retry did not rebuild the complete target set: items=%d total=%d", count, task.TotalCount)
	}
}
