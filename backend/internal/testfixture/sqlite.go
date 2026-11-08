// Package testfixture 提供跨包测试共用的数据库夹具。
package testfixture

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
)

// SQLiteDB 返回一个已迁移扫描任务与组合任务表的临时 sqlite 库。
func SQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fixture.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.CombinationTask{}); err != nil {
		t.Fatal(err)
	}
	return db
}
