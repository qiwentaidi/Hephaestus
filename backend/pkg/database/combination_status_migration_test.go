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

func TestReconcileCombinationStatus(t *testing.T) {
	db := openMigrationTestDB(t)
	if err := db.AutoMigrate(&VulnScanTask{}, &CombinationTask{}); err != nil {
		t.Fatal(err)
	}

	newCombo := func(id, status string) CombinationTask {
		return CombinationTask{ID: id, Name: id, Targets: "[]", Nodes: "[]", Status: status}
	}
	newParent := func(id string, version int, scanType, status, comboID string) VulnScanTask {
		return VulnScanTask{ID: id, Version: version, ScanType: scanType, Status: status, ParentCombinationID: comboID}
	}

	combos := []CombinationTask{
		newCombo("orphan-running", "running"),
		newCombo("orphan-with-child", "running"),
		newCombo("orphan-paused", "paused"),
		newCombo("parent-cancelled", "running"),
		newCombo("parent-failed", "running"),
		newCombo("parent-completed", "running"),
		newCombo("parent-paused", "running"),
		newCombo("parent-deleting", "running"),
		newCombo("parent-running", "running"),
		newCombo("parent-cancelling", "running"),
		newCombo("parent-latest-running", "running"),
		newCombo("parent-child-cancelled", "running"),
		newCombo("already-terminal", "failed"),
		newCombo("paused-parent-cancelled", "paused"),
	}
	if err := db.Create(&combos).Error; err != nil {
		t.Fatal(err)
	}

	tasks := []VulnScanTask{
		newParent("parent-cancelled-task", 1, "combination", "cancelled", "parent-cancelled"),
		newParent("parent-failed-task", 1, "combination", "failed", "parent-failed"),
		newParent("parent-completed-task", 1, "combination", "completed", "parent-completed"),
		newParent("parent-paused-task", 1, "combination", "paused", "parent-paused"),
		newParent("parent-deleting-task", 1, "combination", "deleting", "parent-deleting"),
		newParent("parent-running-task", 1, "combination", "running", "parent-running"),
		newParent("parent-cancelling-task", 1, "combination", "cancelling", "parent-cancelling"),
		newParent("parent-latest-task", 1, "combination", "cancelled", "parent-latest-running"),
		newParent("parent-latest-task", 2, "combination", "running", "parent-latest-running"),
		newParent("parent-child-cancelled-task", 1, "combination", "running", "parent-child-cancelled"),
		newParent("parent-child-cancelled-web", 1, "webscan", "cancelled", "parent-child-cancelled"),
		newParent("orphan-child-web", 1, "webscan", "running", "orphan-with-child"),
		newParent("parent-already-terminal", 1, "combination", "cancelled", "already-terminal"),
		newParent("paused-parent-cancelled-task", 1, "combination", "cancelled", "paused-parent-cancelled"),
	}
	for index := range tasks {
		if err := db.Create(&tasks[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := reconcileCombinationStatus(db); err != nil {
		t.Fatal(err)
	}

	assertStatus := func(id, want string) CombinationTask {
		t.Helper()
		var combo CombinationTask
		if err := db.First(&combo, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if combo.Status != want {
			t.Fatalf("%s status = %q, want %q", id, combo.Status, want)
		}
		return combo
	}

	for _, id := range []string{"orphan-running", "orphan-with-child", "parent-cancelled", "parent-deleting"} {
		combo := assertStatus(id, "cancelled")
		if combo.CompletedAt == nil || combo.ErrorMsg == "" || combo.PauseRequested {
			t.Fatalf("%s cancellation was not fully synchronized: %#v", id, combo)
		}
	}
	failed := assertStatus("parent-failed", "failed")
	if failed.CompletedAt == nil || failed.ErrorMsg == "" {
		t.Fatalf("failed parent was not synchronized: %#v", failed)
	}
	completed := assertStatus("parent-completed", "completed")
	if completed.CompletedAt == nil {
		t.Fatalf("completed parent was not synchronized: %#v", completed)
	}
	paused := assertStatus("parent-paused", "paused")
	if paused.CompletedAt != nil || paused.PauseRequested {
		t.Fatalf("paused parent was not synchronized: %#v", paused)
	}
	assertStatus("parent-running", "running")
	assertStatus("parent-cancelling", "running")
	assertStatus("parent-latest-running", "running")
	assertStatus("parent-child-cancelled", "running")
	assertStatus("already-terminal", "failed")
	assertStatus("orphan-paused", "paused")
	assertStatus("paused-parent-cancelled", "paused")

	if err := reconcileCombinationStatus(db); err != nil {
		t.Fatalf("migration is not idempotent: %v", err)
	}
	assertStatus("parent-cancelled", "cancelled")
}
