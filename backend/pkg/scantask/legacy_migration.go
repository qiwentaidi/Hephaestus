package scantask

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// legacyTaskSchedule is the union of the schedule columns that existed before
// the scan_schedule table. Only the one-time upgrade migration reads it.
type legacyTaskSchedule struct {
	ID                 string     `gorm:"column:id"`
	Version            int        `gorm:"column:version"`
	ScanType           string     `gorm:"column:scan_type"`
	NextRunAt          *time.Time `gorm:"column:next_run_at"`
	ScheduleMode       string     `gorm:"column:schedule_mode"`
	ScheduleWeekday    int        `gorm:"column:schedule_weekday"`
	ScheduleWeekdays   string     `gorm:"column:schedule_weekdays"`
	ScheduleDayOfMonth int        `gorm:"column:schedule_day_of_month"`
	ScheduleTime       string     `gorm:"column:schedule_time"`
	ScheduleCron       string     `gorm:"column:schedule_cron"`
	ScheduleEnabled    *bool      `gorm:"column:schedule_enabled"`
	ScheduleAlarm      string     `gorm:"column:schedule_alarm"`
	ScheduleSpec       string     `gorm:"column:schedule_spec"`
	ScheduleTimezone   string     `gorm:"column:schedule_timezone"`
	ScheduleRevision   int64      `gorm:"column:schedule_revision"`
}

type legacyAlarmSchedule struct {
	Repeat     string   `json:"repeat"`
	Times      []string `json:"times"`
	Date       string   `json:"date"`
	Weekdays   []int    `json:"weekdays"`
	DayOfMonth int      `json:"dayOfMonth"`
	Timezone   string   `json:"timezone"`
	Cron       string   `json:"cron"`
}

type legacyScheduleLog struct {
	ID               uint       `gorm:"column:id"`
	TaskID           string     `gorm:"column:task_id"`
	Version          int        `gorm:"column:version"`
	ScanType         string     `gorm:"column:scan_type"`
	ScheduleMode     string     `gorm:"column:schedule_mode"`
	ScheduledAt      *time.Time `gorm:"column:scheduled_at"`
	TriggeredAt      time.Time  `gorm:"column:triggered_at"`
	Status           string     `gorm:"column:status"`
	ErrorMsg         string     `gorm:"column:error_msg"`
	NextRunAt        *time.Time `gorm:"column:next_run_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	ExecutionVersion *int       `gorm:"column:execution_version"`
}

func (legacyScheduleLog) TableName() string {
	return "vul_scan_task_schedule_log"
}

var legacyScheduleColumns = []string{
	"schedule_mode",
	"schedule_weekday",
	"schedule_weekdays",
	"schedule_day_of_month",
	"schedule_time",
	"schedule_cron",
	"schedule_enabled",
	"schedule_alarm",
	"schedule_spec",
	"schedule_timezone",
	"schedule_revision",
	"schedule_state",
	"next_run_at",
}

// MigrateLegacyScheduleData upgrades a pre-scan_schedule database in one pass:
// read the old columns/log table, write the canonical tables, then drop the
// legacy storage. It is idempotent and returns counts for the startup log.
func MigrateLegacyScheduleData(db *gorm.DB) (int, int, error) {
	if db == nil {
		return 0, 0, fmt.Errorf("database is nil")
	}
	// Create the canonical tables first; the old task table is left untouched
	// until the copy succeeds.
	if err := db.AutoMigrate(&database.ScanSchedule{}, &database.ScanScheduleOccurrence{}); err != nil {
		return 0, 0, err
	}
	// Schedules follow the server timezone; drop the old per-schedule column
	// before inserting new rows.
	if db.Migrator().HasColumn(&database.ScanSchedule{}, "timezone") {
		if err := db.Exec("ALTER TABLE scan_schedule DROP COLUMN timezone").Error; err != nil {
			return 0, 0, fmt.Errorf("删除计划时区列失败: %w", err)
		}
	}
	present := make(map[string]bool, len(legacyScheduleColumns))
	for _, column := range legacyScheduleColumns {
		if db.Migrator().HasColumn(&database.VulnScanTask{}, column) {
			present[column] = true
		}
	}
	hasLogTable := db.Migrator().HasTable(&legacyScheduleLog{})
	if len(present) == 0 && !hasLogTable {
		return 0, 0, nil
	}

	var tasks []legacyTaskSchedule
	if len(present) > 0 {
		// Select only the small schedule columns; loading config/checkpoint for
		// every task during an upgrade would be needlessly expensive.
		columns := []string{"t.id", "t.version", "t.scan_type"}
		for _, column := range legacyScheduleColumns {
			if present[column] && column != "schedule_state" {
				columns = append(columns, "t."+column)
			}
		}
		query := fmt.Sprintf(`
SELECT %s FROM vul_scan_task t
INNER JOIN (
	SELECT id, MAX(version) AS max_version
	FROM vul_scan_task
	GROUP BY id
) latest ON t.id = latest.id AND t.version = latest.max_version
`, strings.Join(columns, ", "))
		if err := db.Raw(query).Scan(&tasks).Error; err != nil {
			return 0, 0, fmt.Errorf("读取旧计划任务失败: %w", err)
		}
	}

	schedules := 0
	for _, task := range tasks {
		config, ok, err := legacyTaskScheduleConfig(task)
		if err != nil {
			return schedules, 0, fmt.Errorf("转换任务 %s 的旧计划失败: %w", task.ID, err)
		}
		if !ok {
			continue
		}
		nextRunAt := task.NextRunAt
		if !config.Enabled {
			nextRunAt = nil
		}
		row := database.ScanSchedule{
			ID:        task.ID,
			TaskID:    task.ID,
			Enabled:   config.Enabled,
			Spec:      MarshalScheduleSpec(config.Spec),
			NextRunAt: nextRunAt,
			Revision:  task.ScheduleRevision,
		}
		result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			return schedules, 0, fmt.Errorf("写入任务 %s 的计划失败: %w", task.ID, result.Error)
		}
		if result.RowsAffected > 0 {
			schedules++
		}
	}

	occurrences := 0
	if hasLogTable {
		var logs []legacyScheduleLog
		if err := db.Raw(`SELECT l.* FROM vul_scan_task_schedule_log l WHERE EXISTS (SELECT 1 FROM vul_scan_task t WHERE t.id = l.task_id)`).Scan(&logs).Error; err != nil {
			return schedules, 0, fmt.Errorf("读取旧调度流水失败: %w", err)
		}
		for _, log := range logs {
			if log.ScheduledAt == nil || strings.TrimSpace(log.Status) == "" {
				continue
			}
			claimedAt := log.TriggeredAt
			if claimedAt.IsZero() {
				claimedAt = log.CreatedAt
			}
			createdAt := log.CreatedAt
			if createdAt.IsZero() {
				createdAt = claimedAt
			}
			var finishedAt *time.Time
			if log.Status != "dispatching" {
				finishedAt = &claimedAt
			}
			row := database.ScanScheduleOccurrence{
				ScheduleID:   log.TaskID,
				ScheduledFor: *log.ScheduledAt,
				Status:       log.Status,
				TaskVersion:  log.ExecutionVersion,
				ScanType:     log.ScanType,
				ScheduleMode: log.ScheduleMode,
				Reason:       log.ErrorMsg,
				NextRunAt:    log.NextRunAt,
				ClaimedAt:    claimedAt,
				FinishedAt:   finishedAt,
				CreatedAt:    createdAt,
			}
			result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if result.Error != nil {
				return schedules, occurrences, fmt.Errorf("写入任务 %s 的调度流水失败: %w", log.TaskID, result.Error)
			}
			if result.RowsAffected > 0 {
				occurrences++
			}
		}
	}

	// Only drop storage after every legacy row has been copied. Any conversion
	// error above returns early and keeps the old data for the next startup.
	if hasLogTable {
		if err := db.Migrator().DropTable(&legacyScheduleLog{}); err != nil {
			logger.Warning("删除旧调度流水表失败: %v", err)
			return schedules, occurrences, err
		}
	}
	if db.Dialector.Name() == "sqlite" {
		dropLegacySQLiteIndexes(db, present)
	}
	for _, column := range legacyScheduleColumns {
		if !present[column] {
			continue
		}
		// GORM's SQLite DropColumn silently ignores unquoted DDL fields, so use
		// the dialect-level statement directly. The column names are internal.
		if err := db.Exec(fmt.Sprintf("ALTER TABLE vul_scan_task DROP COLUMN %s", column)).Error; err != nil {
			logger.Warning("删除旧计划列失败: column=%s err=%v", column, err)
			return schedules, occurrences, fmt.Errorf("删除旧计划列 %s 失败: %w", column, err)
		}
	}
	return schedules, occurrences, nil
}

func legacyTaskScheduleConfig(task legacyTaskSchedule) (ScheduleConfig, bool, error) {
	enabled := task.ScheduleEnabled == nil || *task.ScheduleEnabled
	if strings.TrimSpace(task.ScheduleSpec) != "" {
		spec := UnmarshalScheduleSpec(task.ScheduleSpec)
		if spec == nil {
			return ScheduleConfig{}, false, fmt.Errorf("schedule_spec 无法解析")
		}
		config, err := NormalizeScheduleConfig(ScheduleConfig{Enabled: enabled, Spec: spec})
		if err != nil {
			return ScheduleConfig{}, false, err
		}
		return config, true, nil
	}
	if strings.TrimSpace(task.ScheduleAlarm) != "" {
		var alarm legacyAlarmSchedule
		if err := json.Unmarshal([]byte(task.ScheduleAlarm), &alarm); err != nil {
			return ScheduleConfig{}, false, err
		}
		spec, err := legacyAlarmSpec(alarm)
		if err != nil {
			return ScheduleConfig{}, false, err
		}
		config, err := NormalizeScheduleConfig(ScheduleConfig{Enabled: enabled, Spec: spec})
		if err != nil {
			return ScheduleConfig{}, false, err
		}
		return config, true, nil
	}

	mode := strings.ToLower(strings.TrimSpace(task.ScheduleMode))
	if mode == "" || mode == "manual" || mode == "never" {
		return ScheduleConfig{}, false, nil
	}
	spec, err := legacyModeSpec(task, mode)
	if err != nil {
		return ScheduleConfig{}, false, err
	}
	config, err := NormalizeScheduleConfig(ScheduleConfig{Enabled: enabled, Spec: spec})
	if err != nil {
		return ScheduleConfig{}, false, err
	}
	return config, true, nil
}

func legacyAlarmSpec(alarm legacyAlarmSchedule) (*ScheduleSpec, error) {
	switch strings.ToLower(strings.TrimSpace(alarm.Repeat)) {
	case "once":
		if len(alarm.Times) == 0 {
			return nil, fmt.Errorf("单次计划缺少时间")
		}
		return &ScheduleSpec{Kind: ScheduleKindOnce, Date: alarm.Date, Times: alarm.Times[:1]}, nil
	case "daily":
		return &ScheduleSpec{Kind: ScheduleKindDaily, Times: alarm.Times}, nil
	case "weekdays":
		return &ScheduleSpec{Kind: ScheduleKindWeekdays, Times: alarm.Times}, nil
	case "weekly":
		return &ScheduleSpec{Kind: ScheduleKindWeekly, Weekdays: alarm.Weekdays, Times: alarm.Times}, nil
	case "monthly":
		return &ScheduleSpec{Kind: ScheduleKindMonthly, DayOfMonth: alarm.DayOfMonth, Times: alarm.Times}, nil
	case "custom":
		return &ScheduleSpec{Kind: ScheduleKindCron, Cron: splitCronLines(alarm.Cron)}, nil
	default:
		return nil, fmt.Errorf("未知的旧闹钟规则: %s", alarm.Repeat)
	}
}

func legacyModeSpec(task legacyTaskSchedule, mode string) (*ScheduleSpec, error) {
	clock := strings.TrimSpace(task.ScheduleTime)
	switch mode {
	case "hourly":
		cron := strings.TrimSpace(task.ScheduleCron)
		if cron == "" {
			_, minute, err := parseScheduleClock(clock)
			if err != nil {
				return nil, err
			}
			cron = fmt.Sprintf("%d * * * *", minute)
		}
		return &ScheduleSpec{Kind: ScheduleKindCron, Cron: splitCronLines(cron)}, nil
	case "daily":
		if clock == "" && strings.TrimSpace(task.ScheduleCron) != "" {
			return &ScheduleSpec{Kind: ScheduleKindCron, Cron: splitCronLines(task.ScheduleCron)}, nil
		}
		return &ScheduleSpec{Kind: ScheduleKindDaily, Times: []string{clock}}, nil
	case "weekly":
		weekdays := UnmarshalScheduleWeekdays(task.ScheduleWeekdays, task.ScheduleWeekday)
		if (clock == "" || len(weekdays) == 0) && strings.TrimSpace(task.ScheduleCron) != "" {
			return &ScheduleSpec{Kind: ScheduleKindCron, Cron: splitCronLines(task.ScheduleCron)}, nil
		}
		return &ScheduleSpec{Kind: ScheduleKindWeekly, Weekdays: weekdays, Times: []string{clock}}, nil
	case "monthly":
		if (clock == "" || task.ScheduleDayOfMonth < 1) && strings.TrimSpace(task.ScheduleCron) != "" {
			return &ScheduleSpec{Kind: ScheduleKindCron, Cron: splitCronLines(task.ScheduleCron)}, nil
		}
		return &ScheduleSpec{Kind: ScheduleKindMonthly, DayOfMonth: task.ScheduleDayOfMonth, Times: []string{clock}}, nil
	case "yearly", "custom":
		return &ScheduleSpec{Kind: ScheduleKindCron, Cron: splitCronLines(task.ScheduleCron)}, nil
	default:
		if strings.TrimSpace(task.ScheduleCron) != "" {
			return &ScheduleSpec{Kind: ScheduleKindCron, Cron: splitCronLines(task.ScheduleCron)}, nil
		}
		return nil, fmt.Errorf("不支持的旧计划模式: %s", mode)
	}
}

func dropLegacySQLiteIndexes(db *gorm.DB, present map[string]bool) {
	var indexes []struct {
		Name string `gorm:"column:name"`
	}
	if err := db.Raw(`PRAGMA index_list('vul_scan_task')`).Scan(&indexes).Error; err != nil {
		logger.Warning("读取 vul_scan_task 索引失败: %v", err)
		return
	}
	for _, index := range indexes {
		if strings.HasPrefix(index.Name, "sqlite_") {
			continue
		}
		var columns []struct {
			Name string `gorm:"column:name"`
		}
		if err := db.Raw(fmt.Sprintf("PRAGMA index_info(%q)", index.Name)).Scan(&columns).Error; err != nil {
			continue
		}
		for _, column := range columns {
			if present[column.Name] {
				if err := db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %q", index.Name)).Error; err != nil {
					logger.Warning("删除旧计划索引失败: index=%s err=%v", index.Name, err)
				}
				break
			}
		}
	}
}

func init() {
	database.BeforeAutoMigrate = func(db *gorm.DB) error {
		schedules, occurrences, err := MigrateLegacyScheduleData(db)
		if err != nil {
			return err
		}
		if schedules > 0 || occurrences > 0 {
			logger.Info("升级旧计划数据: %d 条任务计划、%d 条调度流水", schedules, occurrences)
		}
		return nil
	}
}
