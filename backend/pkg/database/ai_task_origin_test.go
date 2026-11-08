package database

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBackfillPentestScanTaskOrigins(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "origin.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE pentest_tool_executions (tool_name TEXT, status TEXT, result_json TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, task := range []VulnScanTask{
		{ID: "ai-task", Version: 1, ScanType: "webscan"},
		{ID: "ai-task", Version: 2, ScanType: "webscan"},
		{ID: "user-task", Version: 1, ScanType: "webscan"},
	} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, record := range []struct{ tool, status, result string }{
		{"attack_surface_create_webscan_task", "completed", `{"ok":true,"result":{"taskId":"ai-task"}}`},
		{"attack_surface_create_webscan_task", "completed", `{"ok":false,"result":{"taskId":"user-task"}}`},
		{"attack_surface_get_scan_task", "completed", `{"ok":true,"result":{"taskId":"user-task"}}`},
	} {
		if err := db.Exec(`INSERT INTO pentest_tool_executions (tool_name, status, result_json) VALUES (?, ?, ?)`, record.tool, record.status, record.result).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := backfillPentestScanTaskOrigins(db); err != nil {
			t.Fatal(err)
		}
	}
	var ai []VulnScanTask
	if err := db.Where("id = ?", "ai-task").Find(&ai).Error; err != nil {
		t.Fatal(err)
	}
	if len(ai) != 2 || !ai[0].CreatedByAI || !ai[1].CreatedByAI {
		t.Fatalf("AI task versions not marked: %+v", ai)
	}
	var user VulnScanTask
	if err := db.First(&user, "id = ?", "user-task").Error; err != nil {
		t.Fatal(err)
	}
	if user.CreatedByAI {
		t.Fatal("user task incorrectly marked as AI-created")
	}
}
