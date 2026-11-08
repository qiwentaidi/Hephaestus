package database

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database/migrations"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migrations.db")+"?_foreign_keys=on"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// A fresh database is created; only schema synchronization is recorded.
// Repeated historical upgrades remain idempotent within the version window.
func TestAutoMigrateIsVersionedAndIdempotent(t *testing.T) {
	db := openMigrationTestDB(t)

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("first AutoMigrate: %v", err)
	}
	for _, table := range []string{"user", "vul_scan_task", "db_collect_result", "schema_migrations"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("table %s was not created", table)
		}
	}

	var schemaRows []string
	if err := db.Table("schema_migrations").Where("version LIKE ?", "schema_sync:%").Pluck("version", &schemaRows).Error; err != nil {
		t.Fatalf("read schema rows: %v", err)
	}
	if len(schemaRows) != 1 || schemaRows[0] != schemaSyncStepID() {
		t.Fatalf("schema rows = %v, want exactly %q", schemaRows, schemaSyncStepID())
	}

	var collectRows int64
	if err := db.Table("schema_migrations").Where("version = ?", "0011_collect_data").Count(&collectRows).Error; err != nil {
		t.Fatalf("read collect step: %v", err)
	}
	if collectRows != 0 {
		t.Fatalf("historical collect step recorded %d times, want 0", collectRows)
	}

	var before int64
	if err := db.Table("schema_migrations").Count(&before).Error; err != nil {
		t.Fatalf("count before: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second AutoMigrate: %v", err)
	}
	var after int64
	if err := db.Table("schema_migrations").Count(&after).Error; err != nil {
		t.Fatalf("count after: %v", err)
	}
	if after != before {
		t.Fatalf("second startup applied %d additional migrations", after-before)
	}
}

func TestSchemaSyncStepIDIsDeterministic(t *testing.T) {
	first := schemaSyncStepID()
	second := schemaSyncStepID()
	if first != second {
		t.Fatalf("schema step ID is not deterministic: %q != %q", first, second)
	}
	if len(first) != len("schema_sync:")+16 {
		t.Fatalf("unexpected schema step ID %q", first)
	}
}

func TestRuntimeVersionMigrationBoundary(t *testing.T) {
	cases := []struct {
		name, mode, web, desktop string
		want                     bool
	}{
		{"web_older", config.RuntimeModeServer, "v0.5.5", "v9.0.0", true},
		{"web_equal", config.RuntimeModeServer, "v0.5.6", "v9.0.0", true},
		{"web_prerelease", config.RuntimeModeServer, "v0.5.6-rc.1", "v9.0.0", true},
		{"desktop_build_metadata", config.RuntimeModeDesktop, "v9.0.0", "v3.3.2+build.1", true},
		{"web_newer", config.RuntimeModeServer, "v0.5.7", "v3.3.2", false},
		{"web_numeric_order", config.RuntimeModeServer, "v0.5.10", "v3.3.2", false},
		{"desktop_older", config.RuntimeModeDesktop, "v9.0.0", "v3.3.1", true},
		{"desktop_equal", config.RuntimeModeDesktop, "v9.0.0", "v3.3.2", true},
		{"desktop_newer", config.RuntimeModeDesktop, "v0.5.6", "v3.3.3", false},
		{"desktop_numeric_order", config.RuntimeModeDesktop, "v0.5.6", "v3.3.10", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := collectStartupTestDB(t, "sqlite")
			insertStartupCollectRecord(t, db, "historical", startupLegacyCollectTable)
			// Neither an applied step nor a newer diagnostic marker determines eligibility.
			if err := migrations.Run(context.Background(), db, []migrations.Step{{ID: "0011_collect_data", Up: func(context.Context, *gorm.DB) (bool, error) { return true, nil }}}); err != nil {
				t.Fatal(err)
			}
			value, _ := json.Marshal(collectDataMigrationState{Version: "v9.0.0"})
			if err := db.Create(&Setting{Key: collectDataMigrationSettingKey, Value: string(value)}).Error; err != nil {
				t.Fatal(err)
			}
			hookRuns := 0
			oldHook := BeforeAutoMigrate
			BeforeAutoMigrate = func(*gorm.DB) error { hookRuns++; return nil }
			t.Cleanup(func() { BeforeAutoMigrate = oldHook })
			for attempt := 0; attempt < 2; attempt++ {
				if err := autoMigrateForVersion(db, tc.mode, tc.web, tc.desktop); err != nil {
					t.Fatal(err)
				}
				if !db.Migrator().HasTable("user") {
					t.Fatal("normal schema creation was skipped")
				}
				var record DBCollectResult
				if err := db.First(&record, "id = ?", "historical").Error; err != nil {
					t.Fatal(err)
				}
				if migrated := record.Content != startupLegacyCollectTable; migrated != tc.want {
					t.Fatalf("migrated=%v, want %v", migrated, tc.want)
				}
				if tc.want && hookRuns != attempt+1 {
					t.Fatalf("legacy hook ran %d times", hookRuns)
				}
				if !tc.want && hookRuns != 0 {
					t.Fatal("legacy hook ran above version ceiling")
				}
			}
		})
	}
}

func TestRuntimeVersionMigrationRejectsInvalidVersion(t *testing.T) {
	if err := autoMigrateForVersion(nil, config.RuntimeModeServer, "invalid", "v3.3.2"); err == nil {
		t.Fatal("invalid running version accepted")
	}
}
