package taskstate

import (
	"context"
	"testing"

	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSaveCheckpointWithoutProgressPreservesTaskProgress(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	task := database.VulnScanTask{ID: "task-1", Version: 1, Progress: 66}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := SaveCheckpointWithoutProgress(db, task.ID, task.Version, "webscan:nuclei", 10, nil); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	var got database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).First(&got).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if got.Progress != 66 {
		t.Fatalf("expected progress to be preserved, got %d", got.Progress)
	}
	if got.CurrentStage != "webscan:nuclei" {
		t.Fatalf("expected current stage to be updated, got %q", got.CurrentStage)
	}
	if got.CheckpointJSON == "" {
		t.Fatal("expected checkpoint json to be saved")
	}
}

func TestMarkCompletedDoesNotOverrideDeletingTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	task := database.VulnScanTask{ID: "task-deleting", Version: 1, Status: StatusDeleting, Progress: 40}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := MarkCompleted(db, task.ID, task.Version); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	var got database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).First(&got).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if got.Status != StatusDeleting {
		t.Fatalf("expected status %q, got %q", StatusDeleting, got.Status)
	}
	if got.Progress != 40 {
		t.Fatalf("expected progress to be preserved, got %d", got.Progress)
	}
}

func TestMarkCompletedDoesNotOverridePausedTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	task := database.VulnScanTask{ID: "task-paused", Version: 1, Status: StatusPaused, Progress: 55}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := MarkCompleted(db, task.ID, task.Version); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	var got database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).First(&got).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if got.Status != StatusPaused {
		t.Fatalf("expected status %q, got %q", StatusPaused, got.Status)
	}
	if got.Progress != 55 {
		t.Fatalf("expected progress to be preserved, got %d", got.Progress)
	}
}

func TestMarkCompletedOnlyCompletesRunningTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	task := database.VulnScanTask{ID: "task-running", Version: 1, Status: StatusRunning, Progress: 80}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := MarkCompleted(db, task.ID, task.Version); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	var got database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).First(&got).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Fatalf("expected status %q, got %q", StatusCompleted, got.Status)
	}
	if got.Progress != 100 {
		t.Fatalf("expected progress 100, got %d", got.Progress)
	}
}

func TestMarkCompletedPreservesStageEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	stageEvents := []map[string]interface{}{
		{"stage": "nuclei", "label": "漏洞扫描 · PoC 检测"},
		{"stage": "complete", "label": "漏洞扫描完成"},
	}
	task := database.VulnScanTask{
		ID:       "task-running-with-events",
		Version:  1,
		Status:   StatusRunning,
		Progress: 80,
		CheckpointJSON: MarshalCheckpoint("webscan:nuclei", 80, map[string]interface{}{
			"stageEvents":   stageEvents,
			"completedJobs": 3,
		}),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := MarkCompleted(db, task.ID, task.Version); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	var got database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).First(&got).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	checkpoint := ParseCheckpoint(got.CheckpointJSON)
	if checkpoint.Stage != "completed" {
		t.Fatalf("expected completed checkpoint stage, got %q", checkpoint.Stage)
	}
	if checkpoint.Progress != 100 {
		t.Fatalf("expected checkpoint progress 100, got %d", checkpoint.Progress)
	}
	if _, ok := checkpoint.Meta["stageEvents"]; !ok {
		t.Fatal("expected stageEvents to be preserved")
	}
	if _, ok := checkpoint.Meta["completedJobs"]; ok {
		t.Fatal("expected completedJobs resume metadata to be dropped")
	}
}

func TestCheckpointAndPauseTreatsDeletingAsCancellation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	task := database.VulnScanTask{ID: "task-deleting", Version: 1, Status: StatusDeleting}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	err = CheckpointAndPause(context.Background(), db, task.ID, task.Version, "webscan:nuclei", 70, nil)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
