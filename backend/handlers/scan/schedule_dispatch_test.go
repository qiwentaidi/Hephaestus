package scan

import (
	"path/filepath"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func scheduleDispatchDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "dispatch.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.ScanSchedule{}, &database.ScanScheduleOccurrence{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedScheduledTask(t *testing.T, db *gorm.DB, id, status string, spec *scantask.ScheduleSpec, next time.Time, enabled bool) {
	t.Helper()
	task := database.VulnScanTask{
		ID: id, Version: 1, ScanType: "webscan", ScanMode: "local",
		Status: status, Targets: `["example.test"]`,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	schedule := database.ScanSchedule{
		ID: id, TaskID: id, Enabled: enabled,
		Spec:      scantask.MarshalScheduleSpec(spec),
		NextRunAt: &next, Revision: 1,
	}
	if err := db.Create(&schedule).Error; err != nil {
		t.Fatal(err)
	}
}

func TestScheduleDispatchPolicy(t *testing.T) {
	due, _ := time.Parse(time.RFC3339, "2026-11-03T02:00:00+08:00")
	daily := &scantask.ScheduleSpec{Kind: scantask.ScheduleKindDaily, Times: []string{"02:00"}}
	once := &scantask.ScheduleSpec{Kind: scantask.ScheduleKindOnce, Date: "2026-11-03", Times: []string{"02:00"}}
	for _, tc := range []struct {
		name      string
		status    string
		spec      *scantask.ScheduleSpec
		delay     time.Duration
		wantLog   string
		wantStart int
		wantNext  bool
	}{
		{"on time", taskstate.StatusCompleted, daily, time.Minute, "success", 1, true},
		{"busy", taskstate.StatusRunning, daily, time.Minute, "skipped", 0, true},
		{"missed", taskstate.StatusCompleted, daily, 6 * time.Minute, "expired", 0, true},
		{"paused", taskstate.StatusPaused, daily, time.Minute, "skipped", 0, true},
		{"once", taskstate.StatusCompleted, once, time.Minute, "success", 1, false},
		{"once paused", taskstate.StatusPaused, once, time.Minute, "skipped", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := scheduleDispatchDB(t)
			seedScheduledTask(t, db, "task", tc.status, tc.spec, due, true)
			started := 0
			dispatchDueScheduledTasksWithDB(db, due.Add(tc.delay), func(string) (string, error) {
				started++
				return "", nil
			})
			if started != tc.wantStart {
				t.Fatalf("starts=%d want=%d", started, tc.wantStart)
			}
			var occurrences []database.ScanScheduleOccurrence
			if err := db.Find(&occurrences).Error; err != nil {
				t.Fatal(err)
			}
			if len(occurrences) != 1 || occurrences[0].Status != tc.wantLog {
				t.Fatalf("occurrences=%+v", occurrences)
			}
			if tc.wantStart > 0 && (occurrences[0].TaskVersion == nil || *occurrences[0].TaskVersion != 1) {
				t.Fatalf("occurrence missing task version: %+v", occurrences[0])
			}
			var schedule database.ScanSchedule
			if err := db.Where("task_id = ?", "task").First(&schedule).Error; err != nil {
				t.Fatal(err)
			}
			if tc.wantNext {
				if schedule.NextRunAt == nil || !schedule.NextRunAt.After(due) {
					t.Fatalf("recurring schedule did not advance: %+v", schedule)
				}
			} else if schedule.NextRunAt != nil {
				t.Fatalf("one-time schedule still scheduled: %+v", schedule)
			}
		})
	}
}

func TestScheduleClaimIsIdempotent(t *testing.T) {
	db := scheduleDispatchDB(t)
	due, _ := time.Parse(time.RFC3339, "2026-11-03T02:00:00+08:00")
	daily := &scantask.ScheduleSpec{Kind: scantask.ScheduleKindDaily, Times: []string{"02:00"}}
	seedScheduledTask(t, db, "task", taskstate.StatusCompleted, daily, due, true)
	started := 0
	starter := func(string) (string, error) { started++; return "", nil }
	dispatchDueScheduledTasksWithDB(db, due.Add(time.Minute), starter)
	dispatchDueScheduledTasksWithDB(db, due.Add(time.Minute), starter)
	if started != 1 {
		t.Fatalf("starts=%d want=1", started)
	}
	var occurrences int64
	if err := db.Model(&database.ScanScheduleOccurrence{}).Count(&occurrences).Error; err != nil {
		t.Fatal(err)
	}
	if occurrences != 1 {
		t.Fatalf("occurrences=%d", occurrences)
	}
}

func TestDisabledScheduleDoesNotDispatch(t *testing.T) {
	db := scheduleDispatchDB(t)
	due, _ := time.Parse(time.RFC3339, "2026-11-03T02:00:00+08:00")
	daily := &scantask.ScheduleSpec{Kind: scantask.ScheduleKindDaily, Times: []string{"02:00"}}
	seedScheduledTask(t, db, "task", taskstate.StatusCompleted, daily, due, false)
	started := 0
	dispatchDueScheduledTasksWithDB(db, due.Add(time.Minute), func(string) (string, error) {
		started++
		return "", nil
	})
	if started != 0 {
		t.Fatalf("disabled schedule started %d times", started)
	}
}

func TestUpsertScanScheduleLifecycle(t *testing.T) {
	db := scheduleDispatchDB(t)
	task := database.VulnScanTask{ID: "upsert", Version: 1, ScanType: "webscan", ScanMode: "local", Status: taskstate.StatusCompleted, Targets: `["example.test"]`}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339, "2026-11-03T12:00:00+08:00")
	config := scantask.ScheduleConfig{
		Enabled: true,
		Spec:    &scantask.ScheduleSpec{Kind: scantask.ScheduleKindDaily, Times: []string{"02:00"}},
	}
	if err := scantask.UpsertScanSchedule(db, task.ID, config, now); err != nil {
		t.Fatal(err)
	}
	row, err := scantask.GetScanSchedule(db, task.ID)
	if err != nil || row == nil {
		t.Fatalf("row=%v err=%v", row, err)
	}
	want, _ := time.Parse(time.RFC3339, "2026-11-04T02:00:00+08:00")
	if !row.Enabled || row.NextRunAt == nil || !row.NextRunAt.Equal(want) {
		t.Fatalf("bad enabled row: %+v", row)
	}

	config.Enabled = false
	if err := scantask.UpsertScanSchedule(db, task.ID, config, now); err != nil {
		t.Fatal(err)
	}
	row, _ = scantask.GetScanSchedule(db, task.ID)
	if row.Enabled || row.NextRunAt != nil || row.Spec == "" {
		t.Fatalf("disable should keep the spec: %+v", row)
	}

	config.Spec = nil
	if err := scantask.UpsertScanSchedule(db, task.ID, config, now); err != nil {
		t.Fatal(err)
	}
	row, _ = scantask.GetScanSchedule(db, task.ID)
	if row != nil {
		t.Fatalf("manual task still has a schedule row: %+v", row)
	}
}
