// Package migrations applies ordered database upgrades.
//
// The runner is intentionally independent from the database models so that any
// package can reuse it (the current step list lives in pkg/database, where the
// models are defined). A step is identified by a permanent string ID and runs
// inside its own transaction. One-time steps are recorded after success;
// repeatable steps are selected by the caller and ignore completion records.
package migrations

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

// TableName is the bookkeeping table that records which upgrades were applied.
const TableName = "schema_migrations"

// Step is one ordered, idempotent database upgrade.
//
// ID is permanent: never reuse an ID for different work, add a new step
// instead. Up reports whether the upgrade is fully applied. Returning true with
// a nil error records non-repeatable steps; returning false with a nil error commits the
// partial work but leaves the step unrecorded so the next startup retries it
// (used by data conversions that must wait for concurrent changes).
type Step struct {
	ID   string
	Name string
	Up   func(ctx context.Context, db *gorm.DB) (applied bool, err error)
	// OutsideTx runs the upgrade on the shared connection instead of a
	// transaction. Use it only for upgrades that need connection scoped
	// statements (such as SQLite PRAGMA) or that manage their own transaction.
	OutsideTx bool
	// Repeatable steps are selected by the caller (for example by runtime
	// version), never by completion records. Existing records are ignored.
	Repeatable bool
}

type record struct {
	Version    string    `gorm:"column:version;primaryKey;type:varchar(191)"`
	Name       string    `gorm:"column:name;type:varchar(191);not null"`
	AppliedAt  time.Time `gorm:"column:applied_at;not null"`
	DurationMS int64     `gorm:"column:duration_ms;not null;default:0"`
}

func (record) TableName() string { return TableName }

// Run applies repeatable steps and unrecorded one-time steps in slice order.
func Run(ctx context.Context, db *gorm.DB, steps []Step) (err error) {
	if db == nil {
		return fmt.Errorf("数据库迁移需要有效的数据库连接")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	db = db.WithContext(ctx)
	started := time.Now()
	var deferredStep string
	defer func() {
		elapsed := time.Since(started).Round(time.Millisecond)
		if err != nil {
			log.Printf("数据库迁移失败，总用时 %s: %v", elapsed, err)
		} else if deferredStep != "" {
			log.Printf("数据库迁移本轮结束，总用时 %s: %s 已部分完成，下次启动继续", elapsed, deferredStep)
		} else {
			log.Printf("数据库迁移本轮结束，总用时 %s", elapsed)
		}
	}()
	seen := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		if step.ID == "" || step.Up == nil {
			return fmt.Errorf("数据库迁移步骤必须带有唯一 ID 和实现")
		}
		if _, ok := seen[step.ID]; ok {
			return fmt.Errorf("数据库迁移步骤 ID 重复: %s", step.ID)
		}
		seen[step.ID] = struct{}{}
	}

	if err := ensureTable(db); err != nil {
		return err
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}
	if !hasPending(steps, applied) {
		return nil
	}

	unlock, err := acquireLock(ctx, db)
	if err != nil {
		return fmt.Errorf("获取数据库迁移锁失败: %w", err)
	}
	defer unlock()

	// Another instance may have applied the upgrades while we waited.
	if applied, err = appliedVersions(db); err != nil {
		return err
	}
	if !hasPending(steps, applied) {
		return nil
	}

	for _, step := range steps {
		if !step.Repeatable && applied[step.ID] {
			continue
		}
		started := time.Now()
		done, err := runStep(ctx, db, step, started)
		if err != nil {
			return fmt.Errorf("数据库迁移 %s(%s) 失败: %w", step.Name, step.ID, err)
		}
		if !done {
			deferredStep = fmt.Sprintf("%s(%s)", step.Name, step.ID)
			return nil
		}
	}
	return nil
}

func runStep(ctx context.Context, db *gorm.DB, step Step, started time.Time) (bool, error) {
	recordApplied := func(tx *gorm.DB) error {
		if step.Repeatable {
			return nil
		}
		return tx.Create(&record{
			Version:    step.ID,
			Name:       step.Name,
			AppliedAt:  time.Now().UTC(),
			DurationMS: time.Since(started).Milliseconds(),
		}).Error
	}

	if step.OutsideTx {
		applied, err := step.Up(ctx, db.WithContext(ctx))
		if err != nil || !applied {
			return applied, err
		}
		return true, recordApplied(db.WithContext(ctx))
	}

	applied := false
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		appliedNow, upErr := step.Up(ctx, tx)
		if upErr != nil {
			return upErr
		}
		if !appliedNow {
			return nil
		}
		applied = true
		return recordApplied(tx)
	})
	return applied, err
}

// Applied returns the IDs recorded as applied. It is used by diagnostics such
// as the standalone migration command.
func Applied(db *gorm.DB) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库迁移需要有效的数据库连接")
	}
	if err := ensureTable(db); err != nil {
		return nil, err
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(applied))
	for id := range applied {
		ids = append(ids, id)
	}
	return ids, nil
}

func ensureTable(db *gorm.DB) error {
	if db.Migrator().HasTable(TableName) {
		return nil
	}
	if err := db.AutoMigrate(&record{}); err != nil {
		// Another instance may have created the table between the check and the
		// DDL; treat that race as success.
		if db.Migrator().HasTable(TableName) {
			return nil
		}
		return fmt.Errorf("创建数据库迁移记录表失败: %w", err)
	}
	return nil
}

func appliedVersions(db *gorm.DB) (map[string]bool, error) {
	var versions []string
	if err := db.Table(TableName).Pluck("version", &versions).Error; err != nil {
		return nil, fmt.Errorf("读取数据库迁移记录失败: %w", err)
	}
	applied := make(map[string]bool, len(versions))
	for _, version := range versions {
		applied[version] = true
	}
	return applied, nil
}

func hasPending(steps []Step, applied map[string]bool) bool {
	for _, step := range steps {
		if step.Repeatable || !applied[step.ID] {
			return true
		}
	}
	return false
}

// acquireLock serializes primary instances upgrading the same PostgreSQL
// database. The SQLite desktop deployment is single-writer, so it needs no
// extra lock. The advisory lock is session scoped, so it is taken on one
// dedicated connection and released on that same connection.
func acquireLock(ctx context.Context, db *gorm.DB) (func(), error) {
	if db.Dialector == nil || db.Dialector.Name() != "postgres" {
		return func() {}, nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", int64(advisoryLockKey)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", int64(advisoryLockKey))
		_ = conn.Close()
	}, nil
}

const advisoryLockKey int64 = 20261104001
