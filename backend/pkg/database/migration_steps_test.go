package database

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
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

// Fresh databases record completed upgrades and skip them on later startups.
func TestAutoMigrateIsVersionedAndIdempotent(t *testing.T) {
	db := openMigrationTestDB(t)

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("first AutoMigrate: %v", err)
	}
	for _, table := range []string{"user", "vul_scan_task", "db_collect_result", "schema_migrations", "subdomain_scan_result"} {
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
	if collectRows != 1 {
		t.Fatalf("historical collect step recorded %d times, want 1", collectRows)
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

// Current releases exceed the former version ceilings in both runtime modes.
// Upgrades must follow database history even when users skip older releases.
func TestAutoMigrateUpgradesUnrecordedHistoryInEveryMode(t *testing.T) {
	originalMode := config.GetRuntimeMode()
	t.Cleanup(func() { config.SetRuntimeMode(originalMode) })
	for _, mode := range []string{config.RuntimeModeServer, config.RuntimeModeDesktop} {
		t.Run(mode, func(t *testing.T) {
			config.SetRuntimeMode(mode)
			db := collectStartupTestDB(t, "sqlite")
			insertStartupCollectRecord(t, db, "historical", startupLegacyCollectTable)
			// A diagnostic application version is not a migration completion record.
			value, _ := json.Marshal(collectDataMigrationState{Version: "v9.0.0"})
			if err := db.Create(&Setting{Key: collectDataMigrationSettingKey, Value: string(value)}).Error; err != nil {
				t.Fatal(err)
			}
			hookRuns := 0
			oldHook := BeforeAutoMigrate
			BeforeAutoMigrate = func(*gorm.DB) error { hookRuns++; return nil }
			t.Cleanup(func() { BeforeAutoMigrate = oldHook })
			var first string
			for attempt := 0; attempt < 2; attempt++ {
				if err := AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
				var record DBCollectResult
				if err := db.First(&record, "id = ?", "historical").Error; err != nil {
					t.Fatal(err)
				}
				if record.Content == startupLegacyCollectTable {
					t.Fatal("historical data was not upgraded")
				}
				if attempt == 0 {
					first = record.Content
				} else if record.Content != first {
					t.Fatal("second startup rewrote migrated data")
				}
				if hookRuns != 1 {
					t.Fatalf("legacy hook ran %d times", hookRuns)
				}
			}
		})
	}
}

func TestAutoMigratePreservesCompletedStepsAndRepairsSchema(t *testing.T) {
	db := openMigrationTestDB(t)
	// Model an installation that recorded completed history but whose previous
	// schema snapshot omitted the subdomain result model.
	var completed []migrations.Step
	for _, step := range migrationSteps() {
		if step.ID == schemaSyncStepID() {
			step.ID = "schema_sync:previous"
		}
		step.Up = func(context.Context, *gorm.DB) (bool, error) { return true, nil }
		completed = append(completed, step)
	}
	if err := migrations.Run(context.Background(), db, completed); err != nil {
		t.Fatal(err)
	}
	type row struct {
		Version   string
		AppliedAt string
	}
	var before, after []row
	if err := db.Table(migrations.TableName).Select("version, applied_at").Where("version NOT LIKE ?", "schema_sync:%").Order("version").Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(&SubdomainScanResult{}) || !db.Migrator().HasColumn(&SubdomainScanResult{}, "ResolutionError") {
		t.Fatal("schema sync did not repair subdomain storage")
	}
	if err := db.Table(migrations.TableName).Select("version, applied_at").Where("version NOT LIKE ?", "schema_sync:%").Order("version").Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("completed historical migrations were rerun")
	}
}

func TestAutoMigrateRetriesFailedPreSchemaUpgrade(t *testing.T) {
	db := openMigrationTestDB(t)
	oldHook := BeforeAutoMigrate
	t.Cleanup(func() { BeforeAutoMigrate = oldHook })
	boom := errors.New("upgrade interrupted")
	BeforeAutoMigrate = func(*gorm.DB) error { return boom }
	if err := AutoMigrate(db); !errors.Is(err, boom) {
		t.Fatalf("got %v, want migration error", err)
	}
	var count int64
	if err := db.Table(migrations.TableName).Where("version = ?", "0013_legacy_pre_schema").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 || db.Migrator().HasTable(&VulnScanTask{}) {
		t.Fatal("failed upgrade was recorded or schema sync ran ahead")
	}
	calls := 0
	BeforeAutoMigrate = func(*gorm.DB) error { calls++; return nil }
	for range 2 {
		if err := AutoMigrate(db); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("retry calls=%d, want 1", calls)
	}
}
