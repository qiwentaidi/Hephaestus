package scantask

import (
	"context"
	"path/filepath"
	"testing"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestQueuedLocalScanSurvivesCreatorContextCancellation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatalf("migrate vuln scan task: %v", err)
	}

	task := database.VulnScanTask{
		ID: "ai-created-scan", Version: 1, TaskName: "AI scan",
		ScanMode: "local", ScanType: "webscan", Status: taskstate.StatusRunning,
		Targets: `["https://example.com"]`, Config: "{}",
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	originalHandler := scanexec.Handlers["webscan"]
	runCalled := false
	scanexec.Handlers["webscan"] = func(ctx context.Context, _ string, _ []string, _ map[string]interface{}) error {
		runCalled = true
		return ctx.Err()
	}
	t.Cleanup(func() { scanexec.Handlers["webscan"] = originalHandler })

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	queued := newLocalScanQueueTask(db.WithContext(requestCtx), task.ID, task.TaskName)
	cancelRequest() // Switching routes ends the AI request before the queued worker starts.
	if err := queued.Fn(); err != nil {
		t.Fatalf("run queued scan after request cancellation: %v", err)
	}
	if !runCalled {
		t.Fatal("scan handler was not called")
	}
	var got database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).First(&got).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if got.Status != taskstate.StatusCompleted {
		t.Fatalf("expected completed task, got %q", got.Status)
	}
}

func TestFinalizePausedLocalTaskMarksRequestedPause(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatalf("migrate vuln scan task: %v", err)
	}

	task := database.VulnScanTask{
		ID:             "task-1",
		Version:        1,
		TaskName:       "pause test",
		ScanMode:       "local",
		ScanType:       "webscan",
		Status:         taskstate.StatusRunning,
		PauseRequested: true,
		Progress:       89,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := FinalizePausedLocalTask(db, &task); err != nil {
		t.Fatalf("finalize paused task: %v", err)
	}

	var got database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).First(&got).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if got.Status != taskstate.StatusPaused {
		t.Fatalf("expected status %q, got %q", taskstate.StatusPaused, got.Status)
	}
	if got.PauseRequested {
		t.Fatalf("expected pause_requested to be cleared")
	}
	if got.Progress != 89 {
		t.Fatalf("expected progress to be preserved, got %d", got.Progress)
	}
}
