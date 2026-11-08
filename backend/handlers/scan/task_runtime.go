package scan

import (
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/gorm"
)

func RecoverInterruptedLocalTasks() {
	db, err := database.GetDB()
	if err != nil {
		logger.Warning("恢复任务时获取数据库失败: %v", err)
		return
	}

	if err := scantask.ReconcilePausedTasksAfterRestart(db, ReconcilePausedCombinationTasksAfterRestart); err != nil {
		logger.Warning("恢复前收敛暂停任务失败: %v", err)
	}

	RecoverInterruptedLocalTasksWithDB(db, time.Now())
}

// reconcilePausedTasksAfterRestart honors a persisted pause intent before any
// interrupted-task recovery is considered. A process restart has already
// stopped the old workers, so these tasks must become paused rather than being
// re-enqueued as running work.

func RecoverInterruptedLocalTasksWithDB(db *gorm.DB, now time.Time) {
	if db == nil {
		return
	}

	deadline := now.Add(-taskstate.HeartbeatTimeout)
	var tasks []database.VulnScanTask
	if err := db.
		Where("scan_mode = ? AND parent_combination_id = '' AND pause_requested = ? AND status IN ?", "local", false, []string{taskstate.StatusRunning, taskstate.StatusRecovering}).
		Find(&tasks).Error; err != nil {
		logger.Warning("查询可恢复任务失败: %v", err)
		return
	}

	for _, task := range tasks {
		if db.Dialector.Name() == "postgres" && task.LeaseExpiresAt != nil && task.LeaseExpiresAt.After(now) {
			continue
		}
		if task.HeartbeatAt != nil && task.HeartbeatAt.After(deadline) {
			continue
		}
		if scantask.ShouldFinalizeRecoveredTask(&task) {
			if err := scantask.FinalizeRecoveredTask(db, &task, "任务恢复前检测到已完成，已自动纠正状态"); err != nil {
				logger.Warning("恢复前纠正任务完成状态失败: task=%s version=%d err=%v", task.ID, task.Version, err)
			} else {
				logger.Warning("恢复前跳过已完成脏任务: id=%s version=%d type=%s", task.ID, task.Version, task.ScanType)
			}
			continue
		}
		if err := taskstate.MarkRecovering(db, task.ID, task.Version, "任务因服务重启待恢复"); err != nil {
			logger.Warning("标记任务恢复状态失败: task=%s version=%d err=%v", task.ID, task.Version, err)
			continue
		}
		if err := db.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", task.ID, task.Version).
			Updates(map[string]interface{}{
				"status":     taskstate.StatusRunning,
				"updated_at": time.Now(),
			}).Error; err != nil {
			logger.Warning("恢复任务重新置为运行态失败: task=%s version=%d err=%v", task.ID, task.Version, err)
			continue
		}
		if err := scantask.EnqueueLocalTask(task.ID); err != nil {
			logger.Warning("恢复任务重新入队失败: task=%s version=%d err=%v", task.ID, task.Version, err)
			_ = taskstate.MarkFailed(db, task.ID, task.Version, err)
			continue
		}
		logger.Info("已恢复本地扫描任务: id=%s version=%d type=%s", task.ID, task.Version, task.ScanType)
	}
}
