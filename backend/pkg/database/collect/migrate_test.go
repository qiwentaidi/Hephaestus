package collect

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/types"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func migrationTestDB(t *testing.T, driver string) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector
	if driver == "postgres" {
		dsn := os.Getenv("HEPHAESTUS_COLLECT_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("set HEPHAESTUS_COLLECT_TEST_POSTGRES_DSN for isolated PostgreSQL integration tests")
		}
		dialector = postgres.Open(dsn)
	} else {
		dialector = sqlite.Open(filepath.Join(t.TempDir(), "collect.db"))
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
		schema := fmt.Sprintf("collect_test_%d", time.Now().UnixNano())
		if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
		if err := db.Exec("SET search_path TO " + schema).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Minimal real table; no unrelated application migrations or foreign keys.
	if err := db.Exec(`CREATE TABLE db_collect_result (
		id TEXT PRIMARY KEY, connection_nanoid TEXT NOT NULL, table_name TEXT NOT NULL,
		content TEXT NOT NULL, rows_count INTEGER, matched_type TEXT, created_at TIMESTAMP
	)`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

const legacyTable = `<table><thead><tr><th>键 &amp; 值</th></tr></thead><tbody><tr><td><div>&lt;img src=x onerror=alert(1)&gt; 中文 dGVzdA==</div></td></tr></tbody></table>`

func TestCollectMigration(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := migrationTestDB(t, driver)
			structured, err := encodeCollectData(types.CollectResult{Columns: []string{"value"}, Rows: [][]interface{}{{true}}, RowsCount: 1})
			if err != nil {
				t.Fatal(err)
			}
			original := map[string]database.DBCollectResult{}
			// Cross the 100-record batch boundary, including an empty ID.
			for i := 0; i < 103; i++ {
				id := fmt.Sprintf("%03d", i)
				if i == 0 {
					id = ""
				}
				record := database.DBCollectResult{ID: id, ConnectionNanoid: "connection", TableNameField: "db.table", Content: legacyTable, RowsCount: 42, MatchedType: "keyword", CreatedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
				if i == 101 {
					record.Content = structured
				} else if i == 102 {
					record.Content = `<img src=x onerror=alert(1)>`
				}
				if err := db.Create(&record).Error; err != nil {
					t.Fatal(err)
				}
				original[id] = record
			}
			preview, err := MigrateCollectData(context.Background(), db, false)
			if err != nil || preview.Scanned != 103 || preview.Convertible != 101 || preview.Structured != 1 || preview.Failed != 1 || preview.Migrated != 0 {
				t.Fatalf("unexpected dry run: %+v, %v", preview, err)
			}
			var records []database.DBCollectResult
			if err := db.Find(&records).Error; err != nil {
				t.Fatal(err)
			}
			for _, record := range records {
				if record.Content != original[record.ID].Content {
					t.Fatal("dry run modified content")
				}
			}
			applied, err := MigrateCollectData(context.Background(), db, true)
			if err != nil || applied.Migrated != 101 || applied.Failed != 1 || applied.Conflicted != 0 {
				t.Fatalf("unexpected apply: %+v, %v", applied, err)
			}
			if err := db.Find(&records).Error; err != nil {
				t.Fatal(err)
			}
			for _, record := range records {
				before := original[record.ID]
				if record.ConnectionNanoid != before.ConnectionNanoid || record.TableNameField != before.TableNameField || record.RowsCount != before.RowsCount || record.MatchedType != before.MatchedType || !record.CreatedAt.Equal(before.CreatedAt) {
					t.Fatalf("metadata changed: %+v", record)
				}
				if record.ID == "101" || record.ID == "102" {
					if record.Content != before.Content {
						t.Fatal("rewrote structured or failed record")
					}
					continue
				}
				// Decode the persisted value as the API does, without an in-memory cache.
				data, err := decodeCollectData(record.Content, record.RowsCount)
				if err != nil || !reflect.DeepEqual(data.Columns, []string{"键 & 值"}) || data.Rows[0][0] != "<img src=x onerror=alert(1)> 中文 dGVzdA==" {
					t.Fatalf("persisted text corrupted: %+v, %v", data, err)
				}
			}
			repeated, err := MigrateCollectData(context.Background(), db, true)
			if err != nil || repeated.Migrated != 0 || repeated.Structured != 102 || repeated.Failed != 1 {
				t.Fatalf("not idempotent: %+v, %v", repeated, err)
			}
		})
	}
}

func TestCollectMigrationDoesNotOverwriteChangedRecords(t *testing.T) {
	db := migrationTestDB(t, "sqlite")
	record := database.DBCollectResult{ID: "race", ConnectionNanoid: "conn", TableNameField: "t", Content: legacyTable}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	// Simulate another process changing content after the migration's SELECT.
	if err := db.Callback().Update().Before("gorm:update").Register("test:concurrent_change", func(tx *gorm.DB) {
		if err := tx.Exec("UPDATE db_collect_result SET content = ? WHERE id = ?", "changed", record.ID).Error; err != nil {
			t.Error(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	report, err := MigrateCollectData(context.Background(), db, true)
	if err != nil || report.Conflicted != 1 || report.Migrated != 0 {
		t.Fatalf("lost concurrency protection: %+v, %v", report, err)
	}
	if err := db.First(&record, "id = ?", "race").Error; err != nil || record.Content != "changed" {
		t.Fatalf("overwrote changed record: %+v, %v", record, err)
	}
}

func TestCollectMigrationCancellation(t *testing.T) {
	db := migrationTestDB(t, "sqlite")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MigrateCollectData(ctx, db, true); err == nil {
		t.Fatal("ignored cancellation")
	}
}
