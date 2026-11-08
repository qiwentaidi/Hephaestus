package scan

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type scheduledTaskRow struct {
	ScheduleID       string     `gorm:"column:schedule_id"`
	TaskID           string     `gorm:"column:task_id"`
	Version          int        `gorm:"column:version"`
	ScanType         string     `gorm:"column:scan_type"`
	Status           string     `gorm:"column:status"`
	ScheduleSpec     string     `gorm:"column:schedule_spec"`
	ScheduleRevision int64      `gorm:"column:schedule_revision"`
	NextRunAt        *time.Time `gorm:"column:next_run_at"`
}

const (
	taskScheduleLogStatusSuccess = "success"
	taskScheduleLogStatusFailed  = "failed"
)

func dispatchDueScheduledTasks() {
	db, err := database.GetDB()
	if err != nil {
		logger.Warning("周期任务调度器获取数据库失败: %v", err)
		return
	}

	dispatchDueScheduledTasksWithDB(db, time.Now(), startScanTaskByID)
}

const (
	scheduleDispatchGrace = 5 * time.Minute
	scheduleBusyReason    = "task_busy"
	scheduleMissedReason  = "missed_window"
)

// dispatchDueScheduledTasksWithDB is the only trigger path. It claims a due
// occurrence from scan_schedule, records it once, then applies the same
// window/busy policy to every schedule kind.
func dispatchDueScheduledTasksWithDB(db *gorm.DB, now time.Time, starter func(string) (string, error)) {
	if db == nil {
		return
	}
	if starter == nil {
		starter = startScanTaskByID
	}
	reconcileInterruptedScheduleLogs(db, now)

	var dueTasks []scheduledTaskRow
	query := `
SELECT s.id AS schedule_id, s.task_id, s.spec AS schedule_spec,        s.revision AS schedule_revision, s.next_run_at, t.version, t.scan_type, t.status
FROM scan_schedule s
INNER JOIN (
	SELECT id, MAX(version) AS max_version
	FROM vul_scan_task
	GROUP BY id
) latest ON latest.id = s.task_id
INNER JOIN vul_scan_task t ON t.id = latest.id AND t.version = latest.max_version
WHERE s.enabled = true
  AND s.next_run_at IS NOT NULL
  AND s.next_run_at <= ?
  AND t.status <> 'deleting'
ORDER BY s.next_run_at ASC
LIMIT 20`
	if err := db.Raw(query, now).Scan(&dueTasks).Error; err != nil {
		logger.Warning("查询待触发周期任务失败: %v", err)
		return
	}

	for _, dueTask := range dueTasks {
		dispatchScheduledTask(db, dueTask, now, starter)
	}
}

func dispatchScheduledTask(db *gorm.DB, task scheduledTaskRow, now time.Time, starter func(string) (string, error)) {
	if task.NextRunAt == nil {
		return
	}
	config, err := scantask.ScheduleConfigFromSpec(task.ScheduleSpec, nil)
	if err != nil {
		logger.Warning("读取任务计划失败: task=%s err=%v", task.TaskID, err)
		return
	}
	if config.Spec == nil {
		return
	}
	spec, err := scantask.NormalizeScheduleSpec(config.Spec)
	if err != nil {
		logger.Warning("任务计划无效: task=%s err=%v", task.TaskID, err)
		return
	}
	once := spec.Kind == scantask.ScheduleKindOnce
	late := now.Sub(*task.NextRunAt) > scheduleDispatchGrace
	busy := !scantask.IsSchedulableTerminalStatus(task.Status)
	// A one-time alarm that fires while the previous assessment is still
	// running keeps its slot and retries inside the grace window. Repeating
	// schedules skip the slot and move on.
	if once && busy && !late && task.Status != taskstate.StatusPaused {
		return
	}

	status, reason := "dispatching", ""
	switch {
	case late:
		status, reason = "expired", scheduleMissedReason
	case busy:
		status, reason = "skipped", scheduleBusyReason
	}

	var nextRunAt *time.Time
	if !once {
		nextRunAt, err = scantask.NextRunAt(scantask.ScheduleConfig{Enabled: true, Spec: spec}, now)
		if err != nil || nextRunAt == nil {
			logger.Warning("计算周期任务下次执行时间失败: task=%s err=%v", task.TaskID, err)
			// Stop a corrupted schedule instead of retrying every tick; the spec
			// stays on the row so an operator can re-save it.
			_ = db.Model(&database.ScanSchedule{}).
				Where("id = ?", task.ScheduleID).
				Updates(map[string]interface{}{"next_run_at": nil, "updated_at": now}).Error
			return
		}
	}

	occurrence := database.ScanScheduleOccurrence{
		ScheduleID:   task.ScheduleID,
		ScheduledFor: *task.NextRunAt,
		Status:       status,
		ScanType:     task.ScanType,
		ScheduleMode: spec.Kind,
		Reason:       reason,
		NextRunAt:    nextRunAt,
		ClaimedAt:    now,
		CreatedAt:    now,
	}
	claimed := false
	err = db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&database.ScanSchedule{}).
			Where("id = ? AND revision = ? AND next_run_at = ? AND enabled = true", task.ScheduleID, task.ScheduleRevision, task.NextRunAt).
			Updates(map[string]interface{}{
				"next_run_at": nextRunAt,
				"revision":    gorm.Expr("revision + 1"),
				"updated_at":  now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		result = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&occurrence)
		if result.Error != nil {
			return result.Error
		}
		claimed = result.RowsAffected > 0
		return nil
	})
	if err != nil {
		logger.Warning("抢占周期任务失败: task=%s err=%v", task.TaskID, err)
		return
	}
	if !claimed {
		return
	}

	if status != "dispatching" {
		return
	}

	status = taskScheduleLogStatusSuccess
	if _, err := starter(task.TaskID); err != nil {
		logger.Warning("触发周期任务失败: task=%s err=%v", task.TaskID, err)
		status, reason = taskScheduleLogStatusFailed, err.Error()
	}
	var executionVersion *int
	if status == taskScheduleLogStatusSuccess {
		var version int
		if err := db.Model(&database.VulnScanTask{}).Where("id = ?", task.TaskID).Select("MAX(version)").Scan(&version).Error; err == nil {
			executionVersion = &version
		}
	}
	finishedAt := now
	if err := db.Model(&database.ScanScheduleOccurrence{}).Where("id = ?", occurrence.ID).Updates(map[string]interface{}{
		"status":       status,
		"reason":       reason,
		"task_version": executionVersion,
		"finished_at":  finishedAt,
	}).Error; err != nil {
		logger.Warning("更新调度流水失败: task=%s err=%v", task.TaskID, err)
	}
}

func reconcileInterruptedScheduleLogs(db *gorm.DB, now time.Time) {
	cutoff := now.Add(-10 * time.Minute)
	finishedAt := now
	if err := db.Model(&database.ScanScheduleOccurrence{}).
		Where("status = ? AND claimed_at < ?", "dispatching", cutoff).
		Updates(map[string]interface{}{"status": "failed", "reason": "interrupted", "finished_at": finishedAt}).Error; err != nil {
		logger.Warning("收敛中断的调度流水失败: %v", err)
	}
}

func StartScheduledScanDispatcher() {
	if db, err := database.GetDB(); err == nil {
		if schedules, occurrences, err := scantask.MigrateLegacyScheduleData(db); err != nil {
			logger.Warning("迁移旧计划数据失败: %v", err)
		} else if schedules > 0 || occurrences > 0 {
			logger.Info("已迁移 %d 条任务计划、%d 条调度流水", schedules, occurrences)
		}
		reconcileInterruptedScheduleLogs(db, time.Now())
	}
	dispatchDueScheduledTasks()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			dispatchDueScheduledTasks()
		}
	}()
}

func UpdateWebScanTaskScheduleHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}
	updateWebScanTaskScheduleWithDB(c, db)
}

func updateWebScanTaskScheduleWithDB(c *gin.Context, db *gorm.DB) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "schedule is required",
		})
		return
	}
	scheduleSource := raw
	if nested, ok := raw["schedule"].(map[string]interface{}); ok {
		scheduleSource = nested
	}

	schedule, err := scantask.ParseScheduleConfigFromMap(scheduleSource)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	now := time.Now()
	if err := scantask.ValidateScheduleConfig(schedule, now); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	nextRunAt, err := scantask.NextRunAt(schedule, now)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "任务不存在",
		})
		return
	}
	if task.Status == taskstate.StatusDeleting {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务正在删除，无法更新计划",
		})
		return
	}

	if err := scantask.UpsertScanSchedule(db, task.ID, schedule, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新计划任务失败: " + err.Error(),
		})
		return
	}

	data := gin.H{
		"taskId":          task.ID,
		"version":         task.Version,
		"scheduleEnabled": schedule.Enabled,
		"scheduleSpec":    schedule.Spec,
	}
	if nextRunAt != nil {
		data["nextRunAt"] = nextRunAt.Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

func ListWebScanTaskScheduleLogsHandler(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}
	offset := 0
	if rawOffset := strings.TrimSpace(c.Query("offset")); rawOffset != "" {
		if parsed, err := strconv.Atoi(rawOffset); err == nil && parsed > 0 {
			offset = parsed
		}
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var total int64
	if err := db.Model(&database.ScanScheduleOccurrence{}).Where("schedule_id = ?", taskID).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询调度记录失败: " + err.Error(),
		})
		return
	}

	var items []database.ScanScheduleOccurrence
	if err := db.Where("schedule_id = ?", taskID).
		Order("scheduled_for DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询调度记录失败: " + err.Error(),
		})
		return
	}

	result := make([]gin.H, 0, len(items))
	for _, item := range items {
		version := 0
		if item.TaskVersion != nil {
			version = *item.TaskVersion
		}
		result = append(result, gin.H{
			"id":               item.ID,
			"taskId":           item.ScheduleID,
			"version":          version,
			"scanType":         item.ScanType,
			"scheduleMode":     item.ScheduleMode,
			"scheduledAt":      item.ScheduledFor,
			"triggeredAt":      item.ClaimedAt,
			"status":           item.Status,
			"executionVersion": item.TaskVersion,
			"errorMsg":         item.Reason,
			"nextRunAt":        item.NextRunAt,
			"createdAt":        item.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":  result,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}
