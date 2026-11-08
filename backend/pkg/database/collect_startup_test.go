package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const startupLegacyCollectTable = `<table><tr><th>value</th></tr><tr><td>&lt;img src=x onerror=alert(1)&gt; 中文</td></tr></table>`

func collectStartupTestDB(t *testing.T, driver string) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector
	if driver == "postgres" {
		dsn := os.Getenv("HEPHAESTUS_COLLECT_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("set HEPHAESTUS_COLLECT_TEST_POSTGRES_DSN for isolated PostgreSQL tests")
		}
		dialector = postgres.Open(dsn)
	} else {
		dialector = sqlite.Open(filepath.Join(t.TempDir(), "startup.db") + "?_foreign_keys=on")
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if driver == "postgres" {
		schema := fmt.Sprintf("collect_startup_%d", time.Now().UnixNano())
		if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
		if err := db.Exec("SET search_path TO " + schema).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AutoMigrate(&Setting{}, &DBConnectionManager{}, &DBCollectResult{}); err != nil {
		t.Fatal(err)
	}
	conn := DBConnectionManager{Nanoid: "connection", Scheme: "postgres", Host: "localhost", Port: 5432}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func insertStartupCollectRecord(t *testing.T, db *gorm.DB, id, content string) {
	t.Helper()
	if err := db.Create(&DBCollectResult{ID: id, ConnectionNanoid: "connection", TableNameField: "db.table", Content: content, RowsCount: 42}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestCollectStartupIgnoresRecordedVersion(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			for _, previous := range []string{"", "v0.5.4", "v0.5.5-rc.1", "v0.5.5", "v0.5.10"} {
				t.Run("from_"+previous, func(t *testing.T) {
					db := collectStartupTestDB(t, driver)
					insertStartupCollectRecord(t, db, "good", startupLegacyCollectTable)
					if previous != "" {
						value, _ := json.Marshal(collectDataMigrationState{Version: previous})
						if err := db.Create(&Setting{Key: collectDataMigrationSettingKey, Value: string(value)}).Error; err != nil {
							t.Fatal(err)
						}
					}
					collectionReads := 0
					if err := db.Callback().Query().Before("gorm:query").Register("test:collection_reads", func(tx *gorm.DB) {
						if tx.Statement.Table == "db_collect_result" {
							collectionReads++
						}
					}); err != nil {
						t.Fatal(err)
					}
					if err := migrateCollectDataOnStartup(db); err != nil {
						t.Fatal(err)
					}
					if collectionReads == 0 {
						t.Fatalf("unexpected collection table scan for %q: %d", previous, collectionReads)
					}
					var record DBCollectResult
					if err := db.First(&record, "id = ?", "good").Error; err != nil {
						t.Fatal(err)
					}
					data, err := DecodeCollectData(record.Content, record.RowsCount)
					if err != nil || record.Content == startupLegacyCollectTable || len(data.Rows) == 0 || data.Rows[0][0] != "<img src=x onerror=alert(1)> 中文" {
						t.Fatalf("automatic conversion failed: %+v, %v", data, err)
					}
					collectionReads = 0
					if err := migrateCollectDataOnStartup(db); err != nil || collectionReads == 0 {
						t.Fatalf("eligible startup did not scan collection data: %d, %v", collectionReads, err)
					}
				})
			}
		})
	}
}

func TestCollectStartupBadRecordDoesNotBlockOrRewrite(t *testing.T) {
	db := collectStartupTestDB(t, "sqlite")
	insertStartupCollectRecord(t, db, "good", startupLegacyCollectTable)
	insertStartupCollectRecord(t, db, "bad", `<img src=x onerror=alert(1)>`)
	if err := migrateCollectDataOnStartup(db); err != nil {
		t.Fatalf("bad historical record blocked startup: %v", err)
	}
	var setting Setting
	if err := db.First(&setting, "key = ?", collectDataMigrationSettingKey).Error; err != nil {
		t.Fatal(err)
	}
	state, err := readCollectMigrationState(setting.Value)
	if err != nil || state.Version != collectDataMigrationVersion || state.Report.Migrated != 1 || state.Report.Failed != 1 || state.Report.Issues[0].ID != "bad" {
		t.Fatalf("missing durable migration outcome: %+v, %v", state, err)
	}
	var record DBCollectResult
	if err := db.First(&record, "id = ?", "bad").Error; err != nil || record.Content != `<img src=x onerror=alert(1)>` {
		t.Fatalf("bad record lost: %+v, %v", record, err)
	}
	if data, err := DecodeCollectData(record.Content, record.RowsCount); err == nil || len(data.Rows) != 0 {
		t.Fatal("bad record became executable table data")
	}
	// Re-running an eligible release preserves malformed content.
	if err := migrateCollectDataOnStartup(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&record, "id = ?", "bad").Error; err != nil || record.Content != `<img src=x onerror=alert(1)>` {
		t.Fatalf("retry changed malformed content: %+v, %v", record, err)
	}
}

func TestCollectStartupMarkerFailureRollsBackAndRetries(t *testing.T) {
	db := collectStartupTestDB(t, "sqlite")
	insertStartupCollectRecord(t, db, "good", startupLegacyCollectTable)
	if err := db.Callback().Update().Before("gorm:update").Register("test:marker_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "setting" {
			tx.AddError(errors.New("simulated marker write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := migrateCollectDataOnStartup(db); err == nil {
		t.Fatal("ignored completion marker failure")
	}
	var record DBCollectResult
	if err := db.First(&record, "id = ?", "good").Error; err != nil || record.Content != startupLegacyCollectTable {
		t.Fatalf("conversion did not roll back: %+v, %v", record, err)
	}
	var count int64
	if err := db.Model(&Setting{}).Where("key = ?", collectDataMigrationSettingKey).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed migration marked complete: %d, %v", count, err)
	}
	if err := db.Callback().Update().Remove("test:marker_failure"); err != nil {
		t.Fatal(err)
	}
	if err := migrateCollectDataOnStartup(db); err != nil {
		t.Fatalf("next startup did not retry: %v", err)
	}
	if err := db.First(&record, "id = ?", "good").Error; err != nil || record.Content == startupLegacyCollectTable {
		t.Fatalf("retry did not migrate: %+v, %v", record, err)
	}
}

func TestAutoMigrateRunsCollectDataUpgrade(t *testing.T) {
	db := collectStartupTestDB(t, "sqlite")
	insertStartupCollectRecord(t, db, "good", startupLegacyCollectTable)
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var record DBCollectResult
	if err := db.First(&record, "id = ?", "good").Error; err != nil || record.Content == startupLegacyCollectTable {
		t.Fatalf("startup migration was not wired: %+v, %v", record, err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second startup failed: %v", err)
	}
}

func TestCollectStartupConcurrentPostgresInstances(t *testing.T) {
	first := collectStartupTestDB(t, "postgres")
	insertStartupCollectRecord(t, first, "good", startupLegacyCollectTable)
	var schema string
	if err := first.Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	second, err := gorm.Open(postgres.Open(os.Getenv("HEPHAESTUS_COLLECT_TEST_POSTGRES_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := second.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// schema is generated by collectStartupTestDB, never supplied externally.
	if err := second.Exec("SET search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	var writes atomic.Int32
	for _, db := range []*gorm.DB{first, second} {
		if err := db.Callback().Update().Before("gorm:update").Register("test:count_conversions", func(tx *gorm.DB) {
			if tx.Statement.Table == "db_collect_result" {
				writes.Add(1)
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, db := range []*gorm.DB{first, second} {
		go func(db *gorm.DB) {
			<-start
			results <- migrateCollectDataOnStartup(db)
		}(db)
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent startup failed: %v", err)
		}
	}
	if writes.Load() != 1 {
		t.Fatalf("concurrent startup repeated conversion: %d", writes.Load())
	}
}
