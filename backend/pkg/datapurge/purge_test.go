package datapurge

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openPurgeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "purge.db")+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPurgeClearsEveryNonKeptTable(t *testing.T) {
	db := openPurgeTestDB(t)
	for _, statement := range []string{
		`CREATE TABLE setting (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE tasks (id TEXT PRIMARY KEY)`,
		`CREATE TABLE task_items (id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE)`,
		`INSERT INTO setting (key, value) VALUES ('ai', '{}')`,
		`INSERT INTO tasks (id) VALUES ('task-1')`,
		`INSERT INTO task_items (id, task_id) VALUES ('item-1', 'task-1')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	result, err := Purge(context.Background(), Target{
		DB:      db,
		Dialect: "sqlite",
		Keep:    []string{"setting"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tables) != 2 || result.Tables[0] != "task_items" || result.Tables[1] != "tasks" {
		t.Fatalf("unexpected cleared tables: %#v", result.Tables)
	}
	if got := countRows(t, db, "setting"); got != 1 {
		t.Fatalf("setting rows = %d, want 1", got)
	}
	if got := countRows(t, db, "tasks"); got != 0 {
		t.Fatalf("task rows = %d, want 0", got)
	}
	if got := countRows(t, db, "task_items"); got != 0 {
		t.Fatalf("task item rows = %d, want 0", got)
	}
}

func TestPurgeRollsBackWhenKeptTableReferencesClearedRows(t *testing.T) {
	db := openPurgeTestDB(t)
	for _, statement := range []string{
		`CREATE TABLE tasks (id TEXT PRIMARY KEY)`,
		`CREATE TABLE keep_ref (id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id))`,
		`INSERT INTO tasks (id) VALUES ('task-1')`,
		`INSERT INTO keep_ref (id, task_id) VALUES ('ref-1', 'task-1')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	_, err := Purge(context.Background(), Target{
		DB:      db,
		Dialect: "sqlite",
		Keep:    []string{"keep_ref"},
	})
	if err == nil {
		t.Fatal("expected a foreign-key rollback")
	}
	if got := countRows(t, db, "tasks"); got != 1 {
		t.Fatalf("task rows = %d, want 1 after rollback", got)
	}
	if got := countRows(t, db, "keep_ref"); got != 1 {
		t.Fatalf("kept rows = %d, want 1 after rollback", got)
	}
}

func TestPurgeWithOnlyKeptTablesIsNoop(t *testing.T) {
	db := openPurgeTestDB(t)
	if _, err := db.Exec(`CREATE TABLE setting (key TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO setting (key) VALUES ('x')`); err != nil {
		t.Fatal(err)
	}

	result, err := Purge(context.Background(), Target{
		DB:      db,
		Dialect: "sqlite",
		Keep:    []string{"setting"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tables) != 0 {
		t.Fatalf("unexpected cleared tables: %#v", result.Tables)
	}
	if got := countRows(t, db, "setting"); got != 1 {
		t.Fatalf("setting rows = %d, want 1", got)
	}
}

func TestPurgeRejectsUnsupportedDialect(t *testing.T) {
	db := openPurgeTestDB(t)
	_, err := Purge(context.Background(), Target{DB: db, Dialect: "mysql"})
	if err == nil {
		t.Fatal("expected unsupported dialect error")
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + quoteSQLite(table)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
