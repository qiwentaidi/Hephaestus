package scantask

import (
	"path/filepath"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func legacyUpgradeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.ScanSchedule{}, &database.ScanScheduleOccurrence{}); err != nil {
		t.Fatal(err)
	}
	// The schema produced by the released legacy version, before scan_schedule
	// existed. Keeping it as raw SQL keeps the migration test independent from
	// the current (clean) VulnScanTask model.
	for _, statement := range []string{
		`CREATE TABLE vul_scan_task (
			id varchar(100) NOT NULL,
			version integer NOT NULL DEFAULT 1,
			scan_type varchar(50) DEFAULT 'webscan',
			status varchar(20) DEFAULT 'pending',
			targets text,
			schedule_mode varchar(20) DEFAULT 'manual',
			schedule_weekday integer DEFAULT 0,
			schedule_weekdays text DEFAULT '',
			schedule_day_of_month integer DEFAULT 0,
			schedule_time varchar(5) DEFAULT '',
			schedule_cron varchar(255) DEFAULT '',
			next_run_at datetime,
			created_at datetime,
			updated_at datetime,
			PRIMARY KEY (id, version)
		)`,
		`CREATE INDEX idx_vul_scan_task_schedule_mode ON vul_scan_task(schedule_mode)`,
		`CREATE INDEX idx_vul_scan_task_next_run_at ON vul_scan_task(next_run_at)`,
		`CREATE TABLE vul_scan_task_schedule_log (
			id integer PRIMARY KEY AUTOINCREMENT,
			task_id varchar(100) NOT NULL,
			version integer DEFAULT 1,
			scan_type varchar(50) DEFAULT 'webscan',
			schedule_mode varchar(20) NOT NULL,
			scheduled_at datetime,
			triggered_at datetime NOT NULL,
			status varchar(20) NOT NULL,
			error_msg text,
			next_run_at datetime,
			created_at datetime
		)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("prepare legacy schema: %v\n%s", err, statement)
		}
	}
	return db
}

func TestMigrateLegacyScheduleDataUpgradesOldSchema(t *testing.T) {
	db := legacyUpgradeDB(t)
	dailyNext := time.Date(2026, 11, 5, 2, 0, 0, 0, time.UTC)
	weeklyNext := time.Date(2026, 11, 6, 2, 30, 0, 0, time.UTC)
	monthlyNext := time.Date(2026, 11, 30, 2, 0, 0, 0, time.UTC)
	cronNext := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		id, scanType, mode, weekdays, clock, cron string
		weekday, day                              int
		next                                      *time.Time
	}{
		{"daily", "webscan", "daily", "", "02:00", "", 0, 0, &dailyNext},
		{"weekly", "webscan", "weekly", "[1,5]", "02:30", "", 1, 0, &weeklyNext},
		{"monthly", "webscan", "monthly", "", "02:00", "", 0, 31, &monthlyNext},
		{"cron", "webscan", "custom", "", "", "0 2 * * 1", 0, 0, &cronNext},
		{"hourly", "webscan", "hourly", "", "", "15 * * * *", 0, 0, &dailyNext},
		{"yearly", "webscan", "yearly", "", "", "0 2 1 6 *", 0, 0, &dailyNext},
		{"daily-cron", "webscan", "daily", "", "", "0 2,14 * * *", 0, 0, &dailyNext},
		{"manual", "webscan", "manual", "", "", "", 0, 0, nil},
	} {
		if err := db.Exec(`
INSERT INTO vul_scan_task (id, version, scan_type, status, targets, schedule_mode, schedule_weekday, schedule_weekdays, schedule_day_of_month, schedule_time, schedule_cron, next_run_at)
VALUES (?, 1, ?, 'pending', '["example.test"]', ?, ?, ?, ?, ?, ?, ?)`,
			row.id, row.scanType, row.mode, row.weekday, row.weekdays, row.day, row.clock, row.cron, row.next,
		).Error; err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}

	dailyDue := time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC)
	for _, log := range []struct {
		taskID, status string
		scheduled      time.Time
	}{
		{"daily", "success", dailyDue},
		{"weekly", "skipped", weeklyNext},
	} {
		if err := db.Exec(`
INSERT INTO vul_scan_task_schedule_log (task_id, version, scan_type, schedule_mode, scheduled_at, triggered_at, status, next_run_at)
VALUES (?, 1, 'webscan', 'daily', ?, ?, ?, ?)`,
			log.taskID, log.scheduled, log.scheduled.Add(30*time.Second), log.status, log.scheduled.Add(24*time.Hour),
		).Error; err != nil {
			t.Fatalf("insert log: %v", err)
		}
	}

	schedules, occurrences, err := MigrateLegacyScheduleData(db)
	if err != nil {
		t.Fatal(err)
	}
	if schedules != 7 || occurrences != 2 {
		t.Fatalf("schedules=%d occurrences=%d", schedules, occurrences)
	}

	check := func(taskID, kind string) database.ScanSchedule {
		t.Helper()
		var row database.ScanSchedule
		if err := db.Where("task_id = ?", taskID).First(&row).Error; err != nil {
			t.Fatalf("%s schedule: %v", taskID, err)
		}
		spec := UnmarshalScheduleSpec(row.Spec)
		if spec == nil || spec.Kind != kind || !row.Enabled {
			t.Fatalf("%s spec=%+v row=%+v", taskID, spec, row)
		}
		return row
	}
	daily := check("daily", ScheduleKindDaily)
	if daily.NextRunAt == nil || !daily.NextRunAt.Equal(dailyNext) {
		t.Fatalf("daily row=%+v", daily)
	}
	if spec := UnmarshalScheduleSpec(daily.Spec); len(spec.Times) != 1 || spec.Times[0] != "02:00" {
		t.Fatalf("daily times=%v", spec.Times)
	}
	weekly := check("weekly", ScheduleKindWeekly)
	if spec := UnmarshalScheduleSpec(weekly.Spec); len(spec.Weekdays) != 2 || spec.Weekdays[1] != 5 {
		t.Fatalf("weekly spec=%+v", spec)
	}
	monthly := check("monthly", ScheduleKindMonthly)
	if spec := UnmarshalScheduleSpec(monthly.Spec); spec.DayOfMonth != 31 {
		t.Fatalf("monthly spec=%+v", spec)
	}
	cron := check("cron", ScheduleKindCron)
	if spec := UnmarshalScheduleSpec(cron.Spec); len(spec.Cron) != 1 || spec.Cron[0] != "0 2 * * 1" {
		t.Fatalf("cron spec=%+v", spec)
	}
	for _, id := range []string{"hourly", "yearly", "daily-cron"} {
		row := check(id, ScheduleKindCron)
		if spec := UnmarshalScheduleSpec(row.Spec); len(spec.Cron) == 0 {
			t.Fatalf("%s cron spec=%+v", id, spec)
		}
	}
	var manual int64
	if err := db.Model(&database.ScanSchedule{}).Where("task_id = ?", "manual").Count(&manual).Error; err != nil || manual != 0 {
		t.Fatalf("manual schedules=%d err=%v", manual, err)
	}

	var occurrence database.ScanScheduleOccurrence
	if err := db.Where("schedule_id = ?", "daily").First(&occurrence).Error; err != nil {
		t.Fatal(err)
	}
	if occurrence.Status != "success" {
		t.Fatalf("daily occurrence=%+v", occurrence)
	}
	// The released log table had no execution version; migrated history keeps
	// the status but cannot invent the executed task version.
	if occurrence.TaskVersion != nil {
		t.Fatalf("legacy occurrence invented a task version: %+v", occurrence)
	}
	occurrence = database.ScanScheduleOccurrence{}
	if err := db.Where("schedule_id = ?", "weekly").First(&occurrence).Error; err != nil {
		t.Fatal(err)
	}
	if occurrence.Status != "skipped" || occurrence.TaskVersion != nil {
		t.Fatalf("weekly occurrence=%+v", occurrence)
	}

	for _, column := range legacyScheduleColumns {
		if db.Migrator().HasColumn(&database.VulnScanTask{}, column) {
			t.Fatalf("legacy column %s was not dropped", column)
		}
	}
	if db.Migrator().HasTable(&legacyScheduleLog{}) {
		t.Fatal("legacy log table was not dropped")
	}

	// The migrated task row must remain usable.
	var id string
	if err := db.Raw(`SELECT id FROM vul_scan_task WHERE id = 'daily'`).Scan(&id).Error; err != nil || id != "daily" {
		t.Fatalf("task row after migration: id=%q err=%v", id, err)
	}

	again, againOccurrences, err := MigrateLegacyScheduleData(db)
	if err != nil || again != 0 || againOccurrences != 0 {
		t.Fatalf("second run schedules=%d occurrences=%d err=%v", again, againOccurrences, err)
	}
}

func TestDatabaseAutoMigrateRunsLegacyUpgradeFirst(t *testing.T) {
	db := legacyUpgradeDB(t)
	next := time.Date(2026, 11, 5, 2, 0, 0, 0, time.UTC)
	if err := db.Exec(`
INSERT INTO vul_scan_task (id, version, scan_type, status, targets, schedule_mode, schedule_weekdays, schedule_time, schedule_cron, next_run_at)
VALUES ('legacy-hook', 1, 'webscan', 'pending', '["example.test"]', 'daily', '', '02:00', '0 2 * * *', ?)`, next).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
INSERT INTO vul_scan_task_schedule_log (task_id, version, scan_type, schedule_mode, scheduled_at, triggered_at, status)
VALUES ('legacy-hook', 1, 'webscan', 'daily', ?, ?, 'success')`, next, next.Add(30*time.Second)).Error; err != nil {
		t.Fatal(err)
	}

	// Production startup uses database.AutoMigrate; the hook must copy the old
	// rows before GORM rebuilds vul_scan_task and drops the legacy columns.
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	var schedule database.ScanSchedule
	if err := db.Where("task_id = ?", "legacy-hook").First(&schedule).Error; err != nil {
		t.Fatalf("schedule not migrated before auto migrate: %v", err)
	}
	if schedule.NextRunAt == nil || !schedule.NextRunAt.Equal(next) {
		t.Fatalf("schedule next run lost: %+v", schedule)
	}
	var occurrences int64
	if err := db.Model(&database.ScanScheduleOccurrence{}).Where("schedule_id = ?", "legacy-hook").Count(&occurrences).Error; err != nil {
		t.Fatal(err)
	}
	if occurrences != 1 {
		t.Fatalf("occurrences=%d", occurrences)
	}
	for _, column := range legacyScheduleColumns {
		if db.Migrator().HasColumn(&database.VulnScanTask{}, column) {
			t.Fatalf("legacy column %s survived auto migrate", column)
		}
	}
}

func TestMigrateDropsScanScheduleTimezoneColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tz.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.ScanSchedule{}, &database.ScanScheduleOccurrence{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`ALTER TABLE scan_schedule ADD COLUMN timezone varchar(64) NOT NULL DEFAULT 'UTC'`).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := MigrateLegacyScheduleData(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn(&database.ScanSchedule{}, "timezone") {
		t.Fatal("scan_schedule.timezone was not dropped")
	}
}
