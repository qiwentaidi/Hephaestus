package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCommandPreflightAndApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collect.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.Exec(`CREATE TABLE db_collect_result (id TEXT PRIMARY KEY, content TEXT, rows_count INTEGER)`).Error; err != nil {
		t.Fatal(err)
	}
	const original = `<table><tr><th>value</th></tr><tr><td>&lt;svg onload=alert(1)&gt;</td></tr></table>`
	if err := db.Exec(`INSERT INTO db_collect_result VALUES (?, ?, ?)`, "saved", original, 1).Error; err != nil {
		t.Fatal(err)
	}
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=rw"}).String()
	if err := run("sqlite", dsn, false); err != nil {
		t.Fatal(err)
	}
	var content string
	if err := db.Raw(`SELECT content FROM db_collect_result WHERE id = ?`, "saved").Scan(&content).Error; err != nil || content != original {
		t.Fatalf("preflight changed record: %q, %v", content, err)
	}
	for i := 0; i < 2; i++ {
		if err := run("sqlite", dsn, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Raw(`SELECT content FROM db_collect_result WHERE id = ?`, "saved").Scan(&content).Error; err != nil || !strings.HasPrefix(content, `{"version":1,`) {
		t.Fatalf("apply did not persist structured data: %q, %v", content, err)
	}
}

func TestCommandRejectsMissingAndUnsafeConfiguration(t *testing.T) {
	for _, tc := range []struct{ driver, dsn string }{
		{"postgres", ""}, {"other", "unused"}, {"sqlite", "/tmp/collect.db"},
		{"sqlite", "file:/tmp/collect.db?mode=rwc"},
	} {
		if err := run(tc.driver, tc.dsn, false); err == nil {
			t.Fatalf("accepted invalid configuration: %+v", tc)
		}
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	dsn := (&url.URL{Scheme: "file", Path: missing, RawQuery: "mode=rw"}).String()
	if err := run("sqlite", dsn, false); err == nil {
		t.Fatal("accepted missing database")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("created missing database: %v", err)
	}
}
