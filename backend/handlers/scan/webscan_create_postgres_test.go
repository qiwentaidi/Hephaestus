package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hephaestus-backend/handlers/agent"
	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/portpresets"
	"hephaestus-backend/pkg/scantask"
)

func TestCreatePortScanPostgresLargeTargetSet(t *testing.T) {
	_, db, _, _ := testfixture.PostgresWorker(t)
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	if err := db.AutoMigrate(&database.Setting{}); err != nil {
		t.Fatal(err)
	}
	targets := make([]string, 5000)
	for i := range targets {
		targets[i] = fmt.Sprintf("host-%d.example.test", i)
	}
	id, err := scantask.CreateAndEnqueueWithDB(db, "portscan", "large-portscan", "", "", targets, nil, scantask.ScheduleConfig{}, false)
	if err != nil {
		t.Fatal(err)
	}
	var task database.VulnScanTask
	if err := db.Where("id = ?", id).First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != "pending" || task.TotalCount != len(targets) || task.ScanMode != "auto" {
		t.Fatalf("unexpected task: status=%s count=%d mode=%s", task.Status, task.TotalCount, task.ScanMode)
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(task.Config), &config); err != nil {
		t.Fatal(err)
	}
	if config["portRange"] != portpresets.WorkbenchPortRangeTop1000 {
		t.Fatal("default top1000 port range was not preserved")
	}
	// Re-ensuring an already queued task must not duplicate its target items.
	if err := agent.EnsureAgentTaskItems(db, &task); err != nil {
		t.Fatal(err)
	}
	var items []database.VulnScanTaskItem
	if err := db.Where("task_id = ? AND version = ?", id, task.Version).Order("id").Find(&items).Error; err != nil {
		t.Fatal(err)
	}
	if len(items) != len(targets) {
		t.Fatalf("items=%d, want %d", len(items), len(targets))
	}
	for i, item := range items {
		if item.Target != targets[i] || item.ScanType != "portscan" || item.Status != "pending" {
			t.Fatalf("incorrect item %d: %+v", i, item)
		}
	}
}

func TestCreatePortScanPostgresRollsBackFailedShardCreation(t *testing.T) {
	_, db, _, _ := testfixture.PostgresWorker(t)
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	if err := db.AutoMigrate(&database.Setting{}); err != nil {
		t.Fatal(err)
	}
	var batches int
	var insertedBeforeFailure int64
	injected := errors.New("injected shard write failure")
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail_second_shard_batch", func(tx *gorm.DB) {
		if tx.Statement.Table != "vul_scan_task_item" {
			return
		}
		batches++
		if batches == 2 {
			if err := tx.Session(&gorm.Session{NewDB: true}).Model(&database.VulnScanTaskItem{}).Count(&insertedBeforeFailure).Error; err != nil {
				tx.AddError(err)
				return
			}
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	targets := make([]string, 5000)
	for i := range targets {
		targets[i] = fmt.Sprintf("host-%d.example.test", i)
	}
	id, err := scantask.CreateAndEnqueueWithDB(db, "portscan", "failed-portscan", "", "", targets, nil, scantask.ScheduleConfig{}, false)
	if id != "" || !errors.Is(err, injected) || !strings.Contains(err.Error(), "创建 Agent 任务分片失败") {
		t.Fatalf("unexpected creation result: id=%s error=%v", id, err)
	}
	if insertedBeforeFailure == 0 {
		t.Fatal("failure did not occur after a successful shard batch")
	}
	for _, model := range []interface{}{&database.VulnScanTask{}, &database.VulnScanTaskItem{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("failed creation left %d records in %T", count, model)
		}
	}
}
