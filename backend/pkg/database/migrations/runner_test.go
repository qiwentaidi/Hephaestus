package migrations

import (
	"bytes"
	"context"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRunLogsOnlySummaryAndTotalElapsedTime(t *testing.T) {
	for _, outcome := range []string{"success", "partial", "failure", "timeout"} {
		t.Run(outcome, func(t *testing.T) {
			db := newTestDB(t)
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			t.Cleanup(func() { log.SetOutput(previous) })
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			boom := errors.New("migration failed")
			step := Step{ID: "diagnostic_step", Name: "诊断测试", Up: func(ctx context.Context, tx *gorm.DB) (bool, error) {
				if output.Len() != 0 {
					t.Errorf("unexpected intermediate logs: %s", output.String())
				}
				switch outcome {
				case "partial":
					return false, nil
				case "failure":
					return false, boom
				case "timeout":
					<-ctx.Done()
					return false, ctx.Err()
				default:
					return true, tx.Create(&testMarker{ID: "logged"}).Error
				}
			}}
			err := Run(ctx, db, []Step{step})
			wantOutcome := "数据库迁移本轮结束，总用时 "
			switch outcome {
			case "partial":
				wantOutcome = "诊断测试(diagnostic_step) 已部分完成，下次启动继续"
			case "failure", "timeout":
				wantErr := boom
				if outcome == "timeout" {
					wantErr = context.DeadlineExceeded
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("Run error = %v, want %v", err, wantErr)
				}
				wantOutcome = "诊断测试(diagnostic_step) 失败: "
			}
			if outcome == "success" || outcome == "partial" {
				if err != nil {
					t.Fatal(err)
				}
			}
			if lines := strings.Count(output.String(), "\n"); lines != 1 {
				t.Errorf("got %d log lines, want one summary: %s", lines, output.String())
			}
			for _, want := range []string{wantOutcome, "总用时 "} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("missing %q in logs: %s", want, output.String())
				}
			}
		})
	}
}

type testMarker struct {
	ID string `gorm:"primaryKey;column:id"`
}

func (testMarker) TableName() string { return "migration_test_markers" }

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "runner.db")+"?_foreign_keys=on"), &gorm.Config{
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
	if err := db.AutoMigrate(&testMarker{}); err != nil {
		t.Fatalf("prepare marker table: %v", err)
	}
	return db
}

func TestRunAppliesPendingStepsOnce(t *testing.T) {
	db := newTestDB(t)
	var first, second int
	steps := []Step{
		{ID: "0001", Name: "first", Up: func(context.Context, *gorm.DB) (bool, error) {
			first++
			return true, nil
		}},
		{ID: "0002", Name: "second", Up: func(context.Context, *gorm.DB) (bool, error) {
			second++
			return true, nil
		}},
	}

	if err := Run(context.Background(), db, steps); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first != 1 || second != 1 {
		t.Fatalf("first run invocations = %d/%d, want 1/1", first, second)
	}

	if err := Run(context.Background(), db, steps); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if first != 1 || second != 1 {
		t.Fatalf("second run re-executed completed steps: %d/%d", first, second)
	}

	applied, err := Applied(db)
	if err != nil {
		t.Fatalf("applied: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("applied = %v, want 2 steps", applied)
	}
}

func TestRunAppliesOnlyNewSteps(t *testing.T) {
	db := newTestDB(t)
	var oldRuns, newRuns int
	oldStep := Step{ID: "0001", Name: "old", Up: func(context.Context, *gorm.DB) (bool, error) {
		oldRuns++
		return true, nil
	}}
	if err := Run(context.Background(), db, []Step{oldStep}); err != nil {
		t.Fatalf("first run: %v", err)
	}

	newStep := Step{ID: "0002", Name: "new", Up: func(context.Context, *gorm.DB) (bool, error) {
		newRuns++
		return true, nil
	}}
	if err := Run(context.Background(), db, []Step{oldStep, newStep}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if oldRuns != 1 || newRuns != 1 {
		t.Fatalf("runs = old %d, new %d; want 1/1", oldRuns, newRuns)
	}
}

func TestRunRetriesDeferredStepAndKeepsPartialWork(t *testing.T) {
	db := newTestDB(t)
	attempts := 0
	step := Step{ID: "0001", Name: "deferred", Up: func(_ context.Context, tx *gorm.DB) (bool, error) {
		attempts++
		if attempts == 1 {
			if err := tx.Create(&testMarker{ID: "partial"}).Error; err != nil {
				return false, err
			}
			return false, nil
		}
		return true, nil
	}}

	if err := Run(context.Background(), db, []Step{step}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	applied, err := Applied(db)
	if err != nil {
		t.Fatalf("applied: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("deferred step was recorded as applied: %v", applied)
	}
	var count int64
	if err := db.Model(&testMarker{}).Where("id = ?", "partial").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("partial work was rolled back, count = %d", count)
	}

	if err := Run(context.Background(), db, []Step{step}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if applied, err = Applied(db); err != nil {
		t.Fatalf("applied: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied after retry = %v, want the step", applied)
	}
}

func TestRunRollsBackFailedStep(t *testing.T) {
	db := newTestDB(t)
	boom := errors.New("boom")
	steps := []Step{
		{ID: "0001", Name: "ok", Up: func(context.Context, *gorm.DB) (bool, error) { return true, nil }},
		{ID: "0002", Name: "broken", Up: func(_ context.Context, tx *gorm.DB) (bool, error) {
			if err := tx.Create(&testMarker{ID: "half"}).Error; err != nil {
				return false, err
			}
			return false, boom
		}},
	}

	err := Run(context.Background(), db, steps)
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Run error = %v, want the step error", err)
	}
	var count int64
	if err := db.Model(&testMarker{}).Where("id = ?", "half").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed step left %d rows, want rollback", count)
	}
	applied, err := Applied(db)
	if err != nil {
		t.Fatalf("applied: %v", err)
	}
	if len(applied) != 1 || applied[0] != "0001" {
		t.Fatalf("applied = %v, want only the first step", applied)
	}
}

func TestRunRejectsInvalidSteps(t *testing.T) {
	db := newTestDB(t)
	if err := Run(context.Background(), db, []Step{{ID: "", Up: func(context.Context, *gorm.DB) (bool, error) { return true, nil }}}); err == nil {
		t.Fatalf("empty ID was accepted")
	}
	if err := Run(context.Background(), db, []Step{
		{ID: "same", Name: "a", Up: func(context.Context, *gorm.DB) (bool, error) { return true, nil }},
		{ID: "same", Name: "b", Up: func(context.Context, *gorm.DB) (bool, error) { return true, nil }},
	}); err == nil {
		t.Fatalf("duplicate ID was accepted")
	}
}

func TestRepeatableStepsIgnoreCompletionRecords(t *testing.T) {
	db := newTestDB(t)
	runs := 0
	step := Step{ID: "historical", Up: func(context.Context, *gorm.DB) (bool, error) { runs++; return true, nil }}
	if err := Run(context.Background(), db, []Step{step}); err != nil {
		t.Fatal(err)
	}
	step.Repeatable = true
	for i := 0; i < 2; i++ {
		if err := Run(context.Background(), db, []Step{step}); err != nil {
			t.Fatal(err)
		}
	}
	if runs != 3 {
		t.Fatalf("runs=%d, want 3", runs)
	}
	step.ID = "new_repeatable"
	if err := Run(context.Background(), db, []Step{step}); err != nil {
		t.Fatal(err)
	}
	applied, err := Applied(db)
	if err != nil || len(applied) != 1 || applied[0] != "historical" {
		t.Fatalf("unexpected records: %v, %v", applied, err)
	}
}
