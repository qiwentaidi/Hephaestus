package database

import (
	"testing"
	"time"
)

func TestSyncCancelledCombinationStatus(t *testing.T) {
	db := openMigrationTestDB(t)
	if err := db.AutoMigrate(&VulnScanTask{}, &CombinationTask{}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	combos := []CombinationTask{
		{ID: "cancelled-parent", Name: "cancelled parent", Targets: "[]", Nodes: "[]", Status: "running", CreatedAt: now, UpdatedAt: now},
		{ID: "running-parent", Name: "running parent", Targets: "[]", Nodes: "[]", Status: "running", CreatedAt: now, UpdatedAt: now},
		{ID: "cancelled-child", Name: "cancelled child", Targets: "[]", Nodes: "[]", Status: "running", CreatedAt: now, UpdatedAt: now},
		{ID: "no-parent", Name: "no parent", Targets: "[]", Nodes: "[]", Status: "running", CreatedAt: now, UpdatedAt: now},
		{ID: "already-cancelled", Name: "already cancelled", Targets: "[]", Nodes: "[]", Status: "cancelled", CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&combos).Error; err != nil {
		t.Fatal(err)
	}

	tasks := []VulnScanTask{
		{ID: "parent-cancelled", Version: 1, ScanType: "combination", Status: "cancelled", ParentCombinationID: "cancelled-parent", UpdatedAt: now},
		{ID: "parent-running", Version: 1, ScanType: "combination", Status: "running", ParentCombinationID: "running-parent", UpdatedAt: now},
		{ID: "child-cancelled", Version: 1, ScanType: "webscan", Status: "cancelled", ParentCombinationID: "cancelled-child", UpdatedAt: now},
		{ID: "parent-already-cancelled", Version: 1, ScanType: "combination", Status: "cancelled", ParentCombinationID: "already-cancelled", UpdatedAt: now},
	}
	for index := range tasks {
		if err := db.Create(&tasks[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := syncCancelledCombinationStatus(db); err != nil {
		t.Fatal(err)
	}

	reload := func(id string) CombinationTask {
		t.Helper()
		var combo CombinationTask
		if err := db.First(&combo, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		return combo
	}

	repaired := reload("cancelled-parent")
	if repaired.Status != "cancelled" || repaired.PauseRequested || repaired.ErrorMsg != "任务已取消" || repaired.CompletedAt == nil {
		t.Fatalf("cancelled parent was not repaired: %#v", repaired)
	}
	if got := reload("running-parent"); got.Status != "running" {
		t.Fatalf("running parent changed: %#v", got)
	}
	if got := reload("cancelled-child"); got.Status != "running" {
		t.Fatalf("child cancellation changed its combination: %#v", got)
	}
	if got := reload("no-parent"); got.Status != "running" {
		t.Fatalf("orphan combination changed: %#v", got)
	}
	if got := reload("already-cancelled"); got.Status != "cancelled" || got.ErrorMsg != "" {
		t.Fatalf("already cancelled combination changed: %#v", got)
	}

	if err := syncCancelledCombinationStatus(db); err != nil {
		t.Fatalf("migration is not idempotent: %v", err)
	}
	if got := reload("cancelled-parent"); got.Status != "cancelled" {
		t.Fatalf("second run changed repaired row: %#v", got)
	}
}

func TestSyncCancelledCombinationStatusSkipsMissingTables(t *testing.T) {
	db := openMigrationTestDB(t)
	if err := syncCancelledCombinationStatus(db); err != nil {
		t.Fatalf("missing tables should be a no-op: %v", err)
	}
}
