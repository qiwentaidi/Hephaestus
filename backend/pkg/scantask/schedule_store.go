package scantask

import (
	"errors"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertScanSchedule writes the canonical definition for a logical task. A nil
// spec means the task is manual, so any existing schedule row is removed.
func UpsertScanSchedule(db *gorm.DB, taskID string, config ScheduleConfig, now time.Time) error {
	if db == nil {
		return errors.New("database is nil")
	}
	if strings.TrimSpace(taskID) == "" {
		return errors.New("task id is required")
	}
	if config.Spec == nil {
		return db.Where("task_id = ?", taskID).Delete(&database.ScanSchedule{}).Error
	}
	normalized, err := NormalizeScheduleConfig(config)
	if err != nil {
		return err
	}
	nextRunAt, err := NextRunAt(normalized, now)
	if err != nil {
		return err
	}
	row := database.ScanSchedule{
		ID:        taskID,
		TaskID:    taskID,
		Enabled:   normalized.Enabled,
		Spec:      MarshalScheduleSpec(normalized.Spec),
		NextRunAt: nextRunAt,
		UpdatedAt: now,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "task_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"enabled":     normalized.Enabled,
			"spec":        row.Spec,
			"next_run_at": nextRunAt,
			"revision":    gorm.Expr("revision + 1"),
			"updated_at":  now,
		}),
	}).Create(&row).Error
}

func GetScanSchedule(db *gorm.DB, taskID string) (*database.ScanSchedule, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	var row database.ScanSchedule
	err := db.Where("task_id = ?", taskID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func ScheduleConfigFromScanSchedule(row *database.ScanSchedule) (ScheduleConfig, error) {
	if row == nil {
		return ScheduleConfig{}, nil
	}
	enabled := row.Enabled
	return ScheduleConfigFromSpec(row.Spec, &enabled)
}
