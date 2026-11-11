package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAutoMigrateRepairsPreviouslyRebuiltSubdomainResults(t *testing.T) {
	db := openMigrationTestDB(t)
	// Older migration builds used tabs in stored DDL, even after the legacy
	// task table was removed. Preserve every field when upgrading this schema.
	if err := db.Exec("CREATE TABLE subdomain_scan_result (\n\tid TEXT PRIMARY KEY,\n\ttask_id TEXT NOT NULL,\n\tdomain TEXT NOT NULL,\n\tsubdomain TEXT NOT NULL,\n\tips TEXT NOT NULL,\n\tsource TEXT,\n\tresolution_error TEXT\n)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO subdomain_scan_result (id, task_id, domain, subdomain, ips, source, resolution_error) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"retained", "old-task", "example.test", "www.example.test", `["203.0.113.1"]`, "DNS", "original DNS error").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := AutoMigrate(db); err != nil {
			t.Fatal(err)
		}
		var got SubdomainScanResult
		if err := db.First(&got, "id = ?", "retained").Error; err != nil {
			t.Fatal(err)
		}
		if got.TaskID != "old-task" || got.Domain != "example.test" || got.Subdomain != "www.example.test" || got.IPs != `["203.0.113.1"]` || got.Source != "DNS" || got.ResolutionError != "original DNS error" {
			t.Fatalf("upgrade lost stored result fields: %#v", got)
		}
		var ddl string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE name = 'subdomain_scan_result'").Scan(&ddl).Error; err != nil {
			t.Fatal(err)
		}
		if strings.Contains(ddl, "\t") {
			t.Fatal("rebuilt DDL still contains tab delimiters")
		}
		// Exercise schema sync again as a future release would, while retaining
		// completed historical steps so the pre-schema repair cannot hide damage.
		if err := db.Exec("DELETE FROM schema_migrations WHERE version LIKE 'schema_sync:%'").Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrateAndDropLegacySubdomainTasksPreservesTasksAndResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.db")+"?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	createLegacySubdomainTables(t, db)
	if err := db.Exec(`INSERT INTO subdomain_scan_task
	(id, status, progress, total_count, current_count, result_count, progress_detail, config, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"legacy-task", "completed", 100, 8, 8, 1, `{"overallProgress":100}`, `{"domains":["example.test"],"taskName":"legacy"}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO subdomain_scan_result
	(id, task_id, domain, subdomain, ips, source, sources, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		"legacy-result", "legacy-task", "example.test", "www.example.test", `[]`, "dns", `["dns"]`).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateAndDropLegacySubdomainTasks(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("subdomain_scan_task") {
		t.Fatal("legacy subdomain task table still exists")
	}
	var task VulnScanTask
	if err := db.Where("id = ?", "legacy-task").First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.ScanType != "subdomain" || task.Status != "completed" || task.CurrentCount != 8 || task.ProgressDetail == "" || task.Targets != `["example.test"]` {
		t.Fatalf("legacy task was not preserved: %#v", task)
	}
	var result SubdomainScanResult
	if err := db.Where("id = ?", "legacy-result").First(&result).Error; err != nil {
		t.Fatal(err)
	}
	if result.TaskID != task.ID || result.Subdomain != "www.example.test" {
		t.Fatalf("legacy result was not preserved: %#v", result)
	}
	var foreignKeys []struct {
		Table string `gorm:"column:table"`
	}
	if err := db.Raw(`PRAGMA foreign_key_list('subdomain_scan_result')`).Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	for _, foreignKey := range foreignKeys {
		if foreignKey.Table == "subdomain_scan_task" {
			t.Fatalf("legacy foreign key still exists: %#v", foreignKeys)
		}
	}
}

func TestAutoMigrateDoesNotRecreateLegacySubdomainTaskTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fresh.db")+"?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("subdomain_scan_task") {
		t.Fatal("fresh migration recreated legacy subdomain task table")
	}
	if !db.Migrator().HasTable(&VulnScanTask{}) || !db.Migrator().HasTable(&SubdomainScanResult{}) {
		t.Fatal("unified subdomain storage tables are missing")
	}
	if !db.Migrator().HasColumn(&VulnScanTask{}, "current_count") || !db.Migrator().HasColumn(&VulnScanTask{}, "progress_detail") {
		t.Fatal("unified task is missing subdomain progress columns")
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migration is not idempotent: %v", err)
	}
}

// Old combination runs executed every pipeline step on the parent task row, so
// their subdomain step left a same-ID legacy task (and its results) while the
// unified row was later marked as a combination task. That is not a conflict and
// must not block startup.
func TestMigrateAndDropLegacySubdomainTasksResolvesTaskTypeConflicts(t *testing.T) {
	cases := []struct {
		name                string
		scanType            string
		parentCombinationID string
	}{
		{name: "finished combination parent", scanType: "combination", parentCombinationID: "combo-1"},
		{name: "interrupted combination step", scanType: "portscan", parentCombinationID: "combo-1"},
		{name: "unrelated task type", scanType: "webscan"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "conflict.db")+"?_foreign_keys=on"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&VulnScanTask{}); err != nil {
				t.Fatal(err)
			}
			createLegacySubdomainTables(t, db)

			now := time.Now()
			existing := VulnScanTask{
				ID: "shared-task", Version: 1, TaskName: "existing", ScanType: tc.scanType,
				ParentCombinationID: tc.parentCombinationID, Status: "completed", Progress: 100,
				TotalCount: 2, CurrentCount: 2, ProgressDetail: `{"keep":true}`,
				Targets: `["example.test"]`, Config: `{}`, CreatedAt: now, UpdatedAt: now,
			}
			if err := db.Create(&existing).Error; err != nil {
				t.Fatal(err)
			}
			insertLegacySubdomainTask(t, db, "shared-task")
			if err := db.Exec(`INSERT INTO subdomain_scan_result
				(id, task_id, domain, subdomain, ips, source, sources, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
				"shared-result", "shared-task", "example.test", "www.example.test", `[]`, "dns", `["dns"]`).Error; err != nil {
				t.Fatal(err)
			}

			if err := migrateAndDropLegacySubdomainTasks(db); err != nil {
				t.Fatalf("migration must not fail on same-ID legacy tasks: %v", err)
			}
			if db.Migrator().HasTable("subdomain_scan_task") {
				t.Fatal("legacy subdomain task table still exists")
			}

			var got VulnScanTask
			if err := db.Where("id = ? AND version = ?", "shared-task", 1).First(&got).Error; err != nil {
				t.Fatal(err)
			}
			if got.ScanType != tc.scanType || got.Progress != 100 || got.CurrentCount != 2 || got.ProgressDetail != `{"keep":true}` {
				t.Fatalf("existing unified task was overwritten: %#v", got)
			}

			var resultCount int64
			if err := db.Model(&SubdomainScanResult{}).Where("task_id = ?", "shared-task").Count(&resultCount).Error; err != nil {
				t.Fatal(err)
			}
			if resultCount != 1 {
				t.Fatalf("legacy subdomain results were dropped: want 1, got %d", resultCount)
			}
		})
	}
}

// The desktop client runs AutoMigrate during startup. A legacy combination
// artifact must not turn that startup path into a hard failure.
func TestAutoMigrateResolvesLegacyCombinationSubdomainConflict(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "startup.db")+"?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	createLegacySubdomainTables(t, db)

	now := time.Now()
	combo := VulnScanTask{
		ID: "combo-parent", Version: 1, TaskName: "combo", ScanType: "combination",
		ParentCombinationID: "combo-1", Status: "completed", Progress: 100,
		TotalCount: 2, CurrentCount: 2, Targets: `["example.test"]`, Config: `{}`,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatal(err)
	}
	insertLegacySubdomainTask(t, db, combo.ID)
	if err := db.Exec(`INSERT INTO subdomain_scan_result
		(id, task_id, domain, subdomain, ips, source, sources, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		"combo-result", combo.ID, "example.test", "www.example.test", `[]`, "dns", `["dns"]`).Error; err != nil {
		t.Fatal(err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("startup migration failed on legacy combination subdomain task: %v", err)
	}
	if db.Migrator().HasTable("subdomain_scan_task") {
		t.Fatal("legacy subdomain task table still exists")
	}
	var got VulnScanTask
	if err := db.Where("id = ? AND version = ?", combo.ID, 1).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScanType != "combination" || got.Progress != 100 || got.CurrentCount != 2 {
		t.Fatalf("combination task was modified: %#v", got)
	}
	var resultCount int64
	if err := db.Model(&SubdomainScanResult{}).Where("task_id = ?", combo.ID).Count(&resultCount).Error; err != nil {
		t.Fatal(err)
	}
	if resultCount != 1 {
		t.Fatalf("combination subdomain results were dropped: want 1, got %d", resultCount)
	}
}

func createLegacySubdomainTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`
CREATE TABLE subdomain_scan_task (
	id TEXT PRIMARY KEY,
	status TEXT NOT NULL DEFAULT 'pending',
	progress INTEGER DEFAULT 0,
	total_count INTEGER DEFAULT 0,
	current_count INTEGER DEFAULT 0,
	result_count INTEGER DEFAULT 0,
	progress_detail TEXT,
	config TEXT NOT NULL,
	error_msg TEXT,
	started_at DATETIME,
	completed_at DATETIME,
	created_at DATETIME,
	updated_at DATETIME
)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
CREATE TABLE subdomain_scan_result (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	domain TEXT NOT NULL,
	subdomain TEXT NOT NULL,
	ips TEXT NOT NULL,
	asset_type TEXT,
	asset_name TEXT,
	source TEXT,
	sources TEXT,
	is_wildcard NUMERIC DEFAULT false,
	created_at DATETIME,
	CONSTRAINT fk_subdomain_scan_result_task FOREIGN KEY (task_id)
		REFERENCES subdomain_scan_task(id) ON DELETE CASCADE
)`).Error; err != nil {
		t.Fatal(err)
	}
}

func insertLegacySubdomainTask(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO subdomain_scan_task
	(id, status, progress, total_count, current_count, result_count, progress_detail, config, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, "completed", 40, 1, 1, 1, `{"overallProgress":40}`, `{"domains":["example.test"],"taskName":"legacy"}`).Error; err != nil {
		t.Fatal(err)
	}
}
