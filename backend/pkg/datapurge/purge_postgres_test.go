package datapurge

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Set HEPHAESTUS_PURGE_TEST_DSN to run the PostgreSQL-specific purge tests.
// The test creates and drops an isolated schema in that database.
func openPurgePostgres(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("HEPHAESTUS_PURGE_TEST_DSN"))
	if dsn == "" {
		t.Skip("requires HEPHAESTUS_PURGE_TEST_DSN")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema := "purge_test_" + strings.ReplaceAll(fmt.Sprintf("%d", os.Getpid()), "-", "") + "_" + strings.ReplaceAll(t.Name(), "/", "_")
	schema = strings.ToLower(schema)
	if len(schema) > 60 {
		schema = schema[:60]
	}
	if _, err := db.Exec("DROP SCHEMA IF EXISTS " + quotePostgres(schema) + " CASCADE"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE SCHEMA " + quotePostgres(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DROP SCHEMA IF EXISTS " + quotePostgres(schema) + " CASCADE")
	})
	return db, schema
}

func TestPurgePostgresClearsBusinessTables(t *testing.T) {
	db, schema := openPurgePostgres(t)
	statements := []string{
		`CREATE TABLE ` + quotePostgres(schema) + `.setting (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE ` + quotePostgres(schema) + `.tasks (id TEXT PRIMARY KEY)`,
		`CREATE TABLE ` + quotePostgres(schema) + `.task_items (id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES ` + quotePostgres(schema) + `.tasks(id) ON DELETE CASCADE)`,
		`INSERT INTO ` + quotePostgres(schema) + `.setting (key, value) VALUES ('ai', '{}')`,
		`INSERT INTO ` + quotePostgres(schema) + `.tasks (id) VALUES ('task-1')`,
		`INSERT INTO ` + quotePostgres(schema) + `.task_items (id, task_id) VALUES ('item-1', 'task-1')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	result, err := Purge(context.Background(), Target{
		DB:      db,
		Dialect: "postgres",
		Schema:  schema,
		Keep:    []string{"setting"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tables) != 2 || result.Tables[0] != "task_items" || result.Tables[1] != "tasks" {
		t.Fatalf("unexpected cleared tables: %#v", result.Tables)
	}
	if got := postgresCount(t, db, schema, "setting"); got != 1 {
		t.Fatalf("setting rows = %d, want 1", got)
	}
	if got := postgresCount(t, db, schema, "tasks"); got != 0 {
		t.Fatalf("task rows = %d, want 0", got)
	}
	if got := postgresCount(t, db, schema, "task_items"); got != 0 {
		t.Fatalf("task item rows = %d, want 0", got)
	}
}

func TestPurgePostgresRefusesToCascadeIntoKeptTable(t *testing.T) {
	db, schema := openPurgePostgres(t)
	statements := []string{
		`CREATE TABLE ` + quotePostgres(schema) + `.tasks (id TEXT PRIMARY KEY)`,
		`CREATE TABLE ` + quotePostgres(schema) + `.keep_ref (id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES ` + quotePostgres(schema) + `.tasks(id))`,
		`INSERT INTO ` + quotePostgres(schema) + `.tasks (id) VALUES ('task-1')`,
		`INSERT INTO ` + quotePostgres(schema) + `.keep_ref (id, task_id) VALUES ('ref-1', 'task-1')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	_, err := Purge(context.Background(), Target{
		DB:      db,
		Dialect: "postgres",
		Schema:  schema,
		Keep:    []string{"keep_ref"},
	})
	if err == nil {
		t.Fatal("expected a preserved foreign-key reference to abort the purge")
	}
	if got := postgresCount(t, db, schema, "tasks"); got != 1 {
		t.Fatalf("task rows = %d, want 1 after abort", got)
	}
	if got := postgresCount(t, db, schema, "keep_ref"); got != 1 {
		t.Fatalf("kept rows = %d, want 1 after abort", got)
	}
}

func postgresCount(t *testing.T, db *sql.DB, schema, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + quotePostgres(schema) + "." + quotePostgres(table)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
