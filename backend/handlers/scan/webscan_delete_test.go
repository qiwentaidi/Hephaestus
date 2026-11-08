package scan

import (
	"errors"
	"path/filepath"
	"testing"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMarkWebScanTaskDeletingPersistsStatusAndCancelsAgentItems(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.VulnScanTask{},
		&database.ScanSchedule{}, &database.ScanScheduleOccurrence{},
		&database.VulnScanTaskItem{},
		&database.ScanAgentCommand{},
	); err != nil {
		t.Fatalf("migrate models: %v", err)
	}

	tasks := []database.VulnScanTask{
		{ID: "task-delete", Version: 1, Status: taskstate.StatusCompleted, ScanMode: "agent", ScanType: "webscan"},
		{ID: "task-delete", Version: 2, Status: taskstate.StatusRunning, ScanMode: "agent", ScanType: "webscan"},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatalf("create tasks: %v", err)
	}
	item := database.VulnScanTaskItem{
		TaskID:        "task-delete",
		Version:       2,
		ScanType:      "webscan",
		Target:        "https://example.com",
		Status:        taskstate.StatusRunning,
		WorkerAgentID: "agent-1",
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create task item: %v", err)
	}

	alreadyDeleting, err := markWebScanTaskDeleting(db, "task-delete")
	if err != nil {
		t.Fatalf("mark deleting: %v", err)
	}
	if alreadyDeleting {
		t.Fatalf("expected first delete mark to start cleanup")
	}

	var deletingCount int64
	if err := db.Model(&database.VulnScanTask{}).
		Where("id = ? AND status = ?", "task-delete", taskstate.StatusDeleting).
		Count(&deletingCount).Error; err != nil {
		t.Fatalf("count deleting tasks: %v", err)
	}
	if deletingCount != int64(len(tasks)) {
		t.Fatalf("expected %d deleting tasks, got %d", len(tasks), deletingCount)
	}

	var gotItem database.VulnScanTaskItem
	if err := db.Where("id = ?", item.ID).First(&gotItem).Error; err != nil {
		t.Fatalf("load task item: %v", err)
	}
	if gotItem.Status != taskstate.StatusCancelled {
		t.Fatalf("expected item status %q, got %q", taskstate.StatusCancelled, gotItem.Status)
	}

	var command database.ScanAgentCommand
	if err := db.Where("task_id = ? AND task_version = ? AND agent_id = ?", "task-delete", 2, "agent-1").First(&command).Error; err != nil {
		t.Fatalf("load cancel command: %v", err)
	}
	if command.Command != "cancel" || command.Status != "pending" {
		t.Fatalf("expected pending cancel command, got command=%q status=%q", command.Command, command.Status)
	}
}

func TestMarkWebScanTaskDeletingIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.VulnScanTask{},
		&database.ScanSchedule{}, &database.ScanScheduleOccurrence{},
		&database.VulnScanTaskItem{},
		&database.ScanAgentCommand{},
	); err != nil {
		t.Fatalf("migrate models: %v", err)
	}

	task := database.VulnScanTask{
		ID:       "task-delete",
		Version:  1,
		Status:   taskstate.StatusDeleting,
		ScanMode: "local",
		ScanType: "webscan",
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	alreadyDeleting, err := markWebScanTaskDeleting(db, task.ID)
	if err != nil {
		t.Fatalf("mark deleting: %v", err)
	}
	if !alreadyDeleting {
		t.Fatalf("expected repeated delete mark to be idempotent")
	}
}

func TestDeleteWebScanTaskDataRemovesCombinationMetadataForParentOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.VulnScanTask{},
		&database.ScanSchedule{}, &database.ScanScheduleOccurrence{},
		&database.CombinationTask{},
		&database.FingerprintInfo{},
		&database.VulnerabilityInfo{},
		&database.PortScanResult{},
		&database.PortScanResultAsset{},
		&database.SubdomainScanResult{},
		&database.VulnScanTaskItem{},
	); err != nil {
		t.Fatalf("migrate models: %v", err)
	}

	combo := database.CombinationTask{
		ID:      "combo-delete",
		Name:    "Combination to delete",
		Targets: `["example.test"]`,
		Nodes:   `[]`,
		Status:  "completed",
	}
	parent := database.VulnScanTask{
		ID:                  "combo-parent-delete",
		Version:             1,
		ScanType:            "combination",
		ParentCombinationID: combo.ID,
		Status:              "completed",
	}
	child := database.VulnScanTask{
		ID:                  "combo-child-delete",
		Version:             1,
		ScanType:            "webscan",
		ParentCombinationID: combo.ID,
		Status:              "completed",
	}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatalf("create combination: %v", err)
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child task: %v", err)
	}

	if err := deleteWebScanTaskData(db, parent.ID); err != nil {
		t.Fatalf("delete combination parent: %v", err)
	}
	var gotCombo database.CombinationTask
	if err := db.Where("id = ?", combo.ID).First(&gotCombo).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected combination metadata to be deleted, got err=%v row=%+v", err, gotCombo)
	}
	var gotChild database.VulnScanTask
	if err := db.Where("id = ?", child.ID).First(&gotChild).Error; err != nil {
		t.Fatalf("deleting parent removed child task: %v", err)
	}
}
