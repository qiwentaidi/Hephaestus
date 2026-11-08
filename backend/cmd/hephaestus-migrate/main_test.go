package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRunMigratesLegacySubdomainConflict(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hephaestus.db")

	db, err := gorm.Open(sqlite.Open(dbPath+"?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	createLegacyTables(t, db)

	now := time.Now()
	combo := database.VulnScanTask{
		ID: "combo-parent", Version: 1, TaskName: "combo", ScanType: "combination",
		ParentCombinationID: "combo-1", Status: "completed", Progress: 100,
		TotalCount: 2, CurrentCount: 2, Targets: `["example.test"]`, Config: `{}`,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatal(err)
	}
	insertLegacyTask(t, db, combo.ID)
	insertLegacyTask(t, db, "standalone-subdomain")
	if err := db.Exec(`INSERT INTO subdomain_scan_result
		(id, task_id, domain, subdomain, ips, source, sources, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		"combo-result", combo.ID, "example.test", "www.example.test", `[]`, "dns", `["dns"]`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO subdomain_scan_result
		(id, task_id, domain, subdomain, ips, source, sources, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		"standalone-result", "standalone-subdomain", "example.test", "api.example.test", `[]`, "dns", `["dns"]`).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	if err := run(options{dbPath: dbPath, assumeYes: true}); err != nil {
		t.Fatalf("migration tool failed: %v", err)
	}

	verify, err := gorm.Open(sqlite.Open(dbPath+"?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := verify.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	if verify.Migrator().HasTable("subdomain_scan_task") {
		t.Fatal("legacy subdomain task table still exists")
	}

	var gotCombo database.VulnScanTask
	if err := verify.Where("id = ? AND version = ?", combo.ID, 1).First(&gotCombo).Error; err != nil {
		t.Fatal(err)
	}
	if gotCombo.ScanType != "combination" || gotCombo.Progress != 100 {
		t.Fatalf("combination task was modified: %#v", gotCombo)
	}

	var gotStandalone database.VulnScanTask
	if err := verify.Where("id = ? AND version = ?", "standalone-subdomain", 1).First(&gotStandalone).Error; err != nil {
		t.Fatalf("standalone legacy task was not migrated: %v", err)
	}
	if gotStandalone.ScanType != "subdomain" || gotStandalone.Targets != `["example.test"]` {
		t.Fatalf("standalone legacy task was migrated incorrectly: %#v", gotStandalone)
	}

	var results int64
	if err := verify.Model(&database.SubdomainScanResult{}).Count(&results).Error; err != nil {
		t.Fatal(err)
	}
	if results != 2 {
		t.Fatalf("subdomain results were dropped: want 2, got %d", results)
	}

	backups, err := filepath.Glob(dbPath + ".before-migrate-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected exactly one backup file, got %v", backups)
	}
	if info, err := os.Stat(backups[0]); err != nil || info.Size() == 0 {
		t.Fatalf("backup file is missing or empty: %v", err)
	}
}

func TestResolveDatabasePathRejectsNonSQLite(t *testing.T) {
	configDir := t.TempDir()
	config.ForceSetConfigBaseDir(configDir)
	if err := os.WriteFile(filepath.Join(configDir, "config-desktop.yaml"), []byte("database:\n  type: postgres\n  host: localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveDatabasePath(""); err == nil {
		t.Fatal("expected non-SQLite configuration to be rejected")
	}
}

func createLegacyTables(t *testing.T, db *gorm.DB) {
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

func insertLegacyTask(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO subdomain_scan_task
	(id, status, progress, total_count, current_count, result_count, progress_detail, config, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, "completed", 40, 1, 1, 1, `{"overallProgress":40}`, `{"domains":["example.test"],"taskName":"legacy"}`).Error; err != nil {
		t.Fatal(err)
	}
}
