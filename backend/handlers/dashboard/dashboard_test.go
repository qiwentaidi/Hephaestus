package dashboard

import (
	"path/filepath"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeduplicateDashboardTasks(t *testing.T) {
	createdAt := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	tasks := []DashboardTask{
		{ID: "task-1", TaskType: "subdomain", Status: "running", CreatedAt: createdAt, UpdatedAt: updatedAt},
		{ID: "task-1", TaskType: "subdomain", Status: "completed", CreatedAt: createdAt, UpdatedAt: updatedAt.Add(time.Minute)},
		{ID: "task-1", TaskType: "webscan", Status: "completed", CreatedAt: createdAt, UpdatedAt: updatedAt},
	}

	got := deduplicateDashboardTasks(tasks)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique dashboard tasks, got %d", len(got))
	}

	for _, task := range got {
		if task.TaskType == "subdomain" && task.Status != "completed" {
			t.Fatalf("expected the newest duplicate task to win, got status %q", task.Status)
		}
	}
}

func TestCountLatestDashboardFingerprintsUsesLatestTaskRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.FingerprintInfo{}); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}

	tasks := []database.VulnScanTask{
		{ID: "task-1", Version: 1, ScanType: "webscan"},
		{ID: "task-1", Version: 2, ScanType: "webscan"},
		{ID: "task-2", Version: 1, ScanType: "webscan"},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatalf("create tasks: %v", err)
	}

	fingerprints := []database.FingerprintInfo{
		{TaskID: "task-1", Version: 1, URL: "https://old.example", Fingerprints: `["old"]`},
		{TaskID: "task-1", Version: 2, URL: "https://current-empty.example", Fingerprints: `[]`},
		{TaskID: "task-1", Version: 2, URL: "https://current.example", Fingerprints: `["nginx"]`},
		{TaskID: "task-2", Version: 1, URL: "https://task-2.example", Fingerprints: ""},
		{TaskID: "orphan", Version: 1, URL: "https://orphan.example", Fingerprints: `["orphan"]`},
	}
	if err := db.Create(&fingerprints).Error; err != nil {
		t.Fatalf("create fingerprints: %v", err)
	}

	got, err := countLatestDashboardFingerprints(db)
	if err != nil {
		t.Fatalf("count fingerprints: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected 3 latest fingerprint result rows, got %d", got)
	}
}
