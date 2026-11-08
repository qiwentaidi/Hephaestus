package scan

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"
)

func TestReconcilePausedTasksAfterRestartDoesNotRequeuePauseRequests(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.CombinationTask{}); err != nil {
		t.Fatalf("migrate models: %v", err)
	}

	now := time.Now()
	localTask := database.VulnScanTask{
		ID:             "local-pausing",
		Version:        1,
		TaskName:       "local pausing",
		ScanMode:       "local",
		ScanType:       "webscan",
		Status:         taskstate.StatusRecovering,
		PauseRequested: true,
		Progress:       50,
	}
	combo := database.CombinationTask{
		ID:             "combo-pausing",
		Name:           "combo pausing",
		Targets:        `["https://example.com"]`,
		Nodes:          `[{"id":"node-1","moduleId":"webscan","stageIndex":0}]`,
		Context:        `{"runningModules":{"webscan":true},"steps":[{"module":"webscan","taskId":"combo-parent","status":"running"}]}`,
		Status:         taskstate.StatusRunning,
		PauseRequested: true,
		StartedAt:      &now,
	}
	comboParent := database.VulnScanTask{
		ID:                  "combo-parent",
		Version:             1,
		TaskName:            "combo parent",
		ScanMode:            "local",
		ScanType:            "combination",
		ParentCombinationID: combo.ID,
		Status:              taskstate.StatusRecovering,
		PauseRequested:      true,
		Progress:            50,
	}
	if err := db.Create(&localTask).Error; err != nil {
		t.Fatalf("create local task: %v", err)
	}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatalf("create combo: %v", err)
	}
	if err := db.Create(&comboParent).Error; err != nil {
		t.Fatalf("create combo parent: %v", err)
	}

	if err := scantask.ReconcilePausedTasksAfterRestart(db, ReconcilePausedCombinationTasksAfterRestart); err != nil {
		t.Fatalf("reconcile paused tasks: %v", err)
	}

	var gotLocal database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", localTask.ID, localTask.Version).First(&gotLocal).Error; err != nil {
		t.Fatalf("load local task: %v", err)
	}
	if gotLocal.Status != taskstate.StatusPaused || gotLocal.PauseRequested {
		t.Fatalf("expected local task to remain paused, got status=%q pauseRequested=%t", gotLocal.Status, gotLocal.PauseRequested)
	}

	var gotCombo database.CombinationTask
	if err := db.Where("id = ?", combo.ID).First(&gotCombo).Error; err != nil {
		t.Fatalf("load combo: %v", err)
	}
	if gotCombo.Status != taskstate.StatusPaused || gotCombo.PauseRequested {
		t.Fatalf("expected combo to remain paused, got status=%q pauseRequested=%t", gotCombo.Status, gotCombo.PauseRequested)
	}
	ctx, err := loadCombinationContext(&gotCombo)
	if err != nil {
		t.Fatalf("load combo context: %v", err)
	}
	if len(ctx.Running) != 0 || ctx.stepLog("webscan").Status != taskstate.StatusPaused {
		t.Fatalf("expected all combo modules to acknowledge pause, got running=%#v step=%#v", ctx.Running, ctx.stepLog("webscan"))
	}

	var gotParent database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", comboParent.ID, comboParent.Version).First(&gotParent).Error; err != nil {
		t.Fatalf("load combo parent: %v", err)
	}
	if gotParent.Status != taskstate.StatusPaused || gotParent.PauseRequested {
		t.Fatalf("expected combo parent to remain paused, got status=%q pauseRequested=%t", gotParent.Status, gotParent.PauseRequested)
	}
}
