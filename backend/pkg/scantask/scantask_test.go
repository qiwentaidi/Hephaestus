package scantask

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
)

func TestNormalizeMode(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "auto", false},
		{" LOCAL ", "local", false},
		{"agent", "agent", false},
		{"auto", "auto", false},
		{"remote", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeMode(tt.in)
		if (err != nil) != tt.wantErr {
			t.Fatalf("NormalizeMode(%q) error=%v wantErr=%v", tt.in, err, tt.wantErr)
		}
		if !tt.wantErr && got != tt.want {
			t.Fatalf("NormalizeMode(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

func TestUsePersistentWorkers(t *testing.T) {
	sqliteDB := openSQLite(t, ":memory:")
	if !UsePersistentWorkers(sqliteDB, "agent") {
		t.Fatal("agent mode always uses persistent workers")
	}
	if UsePersistentWorkers(sqliteDB, "local") {
		t.Fatal("sqlite local mode must not use persistent workers")
	}
	if UsePersistentWorkers(sqliteDB, "auto") {
		t.Fatal("sqlite auto mode must not use persistent workers")
	}
}

func TestCancelMissingTaskIsNotCancelable(t *testing.T) {
	db := openSQLite(t, t.TempDir()+"/cancel.db")
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.VulnScanTaskItem{}, &database.CombinationTask{}); err != nil {
		t.Fatal(err)
	}
	err := CancelTaskWithDB(db, "missing")
	if !errors.Is(err, ErrNotCancelable) {
		t.Fatalf("expected ErrNotCancelable, got %v", err)
	}
}

func openSQLite(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}
