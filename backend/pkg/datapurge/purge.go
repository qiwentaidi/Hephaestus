// Package datapurge clears application-owned business data from a database
// while preserving an explicit allowlist of configuration tables.
package datapurge

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
)

var running atomic.Bool

// SetRunning marks a purge as in progress so background schedulers can pause
// creating new work until the cleanup transaction finishes.
func SetRunning(value bool) { running.Store(value) }

// IsRunning reports whether a purge is currently in progress.
func IsRunning() bool { return running.Load() }

// Target describes one database to purge.
type Target struct {
	DB      *sql.DB
	Dialect string
	// Schema is the PostgreSQL schema to inspect. It is ignored for SQLite.
	Schema string
	// Keep lists table names whose rows must survive the purge.
	Keep []string
}

// Result reports the tables that were cleared.
type Result struct {
	Tables []string
}

// MainKeepTables are the system settings, identity, appearance, templates,
// dictionaries, agent nodes and AI prompt configuration stored in the main
// application database. Every other table is business data.
var MainKeepTables = []string{
	"setting",
	"user",
	"custom_theme",
	"scan_template",
	"jsscan_config",
	"space_engine_syntax",
	"agent_python_config",
	"scan_agent_node",
	"scan_agent_host",
	"agent_update",
	"license",
	"ai_custom_strategy_packs",
	"pentest_agent_prompts",
	"pentest_model_routing",
	"schema_migrations",
}

// OrchestrationKeepTables are the configuration tables of the embedded ARTEX
// task-core database. Tasks, explorations, assets, findings and their runtime
// journals are business data.
var OrchestrationKeepTables = []string{
	"settings",
	"llm_profiles",
	"agents",
	"agent_prompts",
	"agent_prompt_vars",
	"agent_visibility",
	"agent_skill_visibility",
	"agent_triggers",
	"task_categories",
	"task_templates",
	"asset_intercept_rules",
	"intercept_rules",
	"mcp_servers",
	"mcp_tools_cache",
	"tools",
	"llm_profile_health",
}

// Purge deletes rows from every table in target that is not in target.Keep.
// The deletion is transactional; a foreign-key violation or SQL error rolls
// back the whole purge.
func Purge(ctx context.Context, target Target) (Result, error) {
	if target.DB == nil {
		return Result{}, fmt.Errorf("数据库连接为空")
	}
	dialect := strings.ToLower(strings.TrimSpace(target.Dialect))
	switch dialect {
	case "sqlite", "sqlite3":
		dialect = "sqlite"
	case "postgres", "postgresql":
		dialect = "postgres"
	default:
		return Result{}, fmt.Errorf("不支持的数据清理数据库类型: %s", target.Dialect)
	}

	keep := make(map[string]struct{}, len(target.Keep))
	for _, table := range target.Keep {
		name := strings.TrimSpace(table)
		if name != "" {
			keep[name] = struct{}{}
		}
	}

	tables, err := listTables(ctx, target.DB, dialect, target.Schema)
	if err != nil {
		return Result{}, err
	}

	clear := make([]string, 0, len(tables))
	for _, table := range tables {
		if _, ok := keep[table]; ok {
			continue
		}
		clear = append(clear, table)
	}
	sort.Strings(clear)
	if len(clear) == 0 {
		return Result{}, nil
	}

	switch dialect {
	case "sqlite":
		err = purgeSQLite(ctx, target.DB, clear)
	case "postgres":
		err = purgePostgres(ctx, target.DB, target.Schema, clear, keep)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Tables: clear}, nil
}

func listTables(ctx context.Context, db *sql.DB, dialect, schema string) ([]string, error) {
	var (
		rows *sql.Rows
		err  error
	)
	switch dialect {
	case "sqlite":
		rows, err = db.QueryContext(ctx, `
SELECT name FROM sqlite_master
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name`)
	case "postgres":
		if strings.TrimSpace(schema) == "" {
			schema = "public"
		}
		rows, err = db.QueryContext(ctx, `
SELECT table_name FROM information_schema.tables
WHERE table_schema = $1 AND table_type = 'BASE TABLE'
ORDER BY table_name`, schema)
	}
	if err != nil {
		return nil, fmt.Errorf("读取数据表列表失败: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("读取数据表名失败: %w", err)
		}
		if strings.TrimSpace(name) != "" {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取数据表列表失败: %w", err)
	}
	return tables, nil
}

func purgeSQLite(ctx context.Context, db *sql.DB, tables []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始数据清理事务失败: %w", err)
	}
	defer tx.Rollback()

	// Delete order is irrelevant with deferred checks. A kept table that still
	// references a cleared row aborts the commit instead of silently breaking
	// the foreign key.
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		return fmt.Errorf("延迟外键检查失败: %w", err)
	}
	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+quoteSQLite(table)); err != nil {
			return fmt.Errorf("清空表 %s 失败: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交数据清理事务失败: %w", err)
	}
	return nil
}

func purgePostgres(ctx context.Context, db *sql.DB, schema string, tables []string, keep map[string]struct{}) error {
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始数据清理事务失败: %w", err)
	}
	defer tx.Rollback()

	clear := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		clear[table] = struct{}{}
	}

	// CASCADE is required because the schema has many foreign keys. Refuse the
	// purge if a preserved table references a table that would be truncated;
	// CASCADE would otherwise clear preserved rows as well.
	rows, err := tx.QueryContext(ctx, `
SELECT child.relname, parent.relname, parent_ns.nspname
FROM pg_constraint c
JOIN pg_class child ON child.oid = c.conrelid
JOIN pg_class parent ON parent.oid = c.confrelid
JOIN pg_namespace child_ns ON child_ns.oid = child.relnamespace
JOIN pg_namespace parent_ns ON parent_ns.oid = parent.relnamespace
WHERE c.contype = 'f' AND child_ns.nspname = $1`, schema)
	if err != nil {
		return fmt.Errorf("检查外键关系失败: %w", err)
	}
	type edge struct{ child, parent, parentSchema string }
	var edges []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.child, &e.parent, &e.parentSchema); err != nil {
			rows.Close()
			return fmt.Errorf("读取外键关系失败: %w", err)
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("读取外键关系失败: %w", err)
	}
	rows.Close()
	for _, e := range edges {
		if e.parentSchema != schema {
			continue
		}
		_, childKept := keep[e.child]
		_, parentCleared := clear[e.parent]
		if childKept && parentCleared {
			return fmt.Errorf("保留表 %s 仍引用待清空表 %s，已取消清理", e.child, e.parent)
		}
	}

	quoted := make([]string, 0, len(tables))
	for _, table := range tables {
		quoted = append(quoted, quotePostgres(schema)+"."+quotePostgres(table))
	}
	statement := "TRUNCATE TABLE " + strings.Join(quoted, ", ") + " RESTART IDENTITY CASCADE"
	if _, err := tx.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("清空数据表失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交数据清理事务失败: %w", err)
	}
	return nil
}

func quoteSQLite(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func quotePostgres(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
