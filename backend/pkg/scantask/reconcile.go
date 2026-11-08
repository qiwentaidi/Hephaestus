package scantask

import (
	"fmt"

	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/taskstate"
)

func ReconcilePausedTasksAfterRestart(db *gorm.DB, reconcileCombinations func(*gorm.DB) (int, error)) error {
	if db == nil {
		return nil
	}
	if db.Dialector.Name() == "postgres" {
		var tasks []database.VulnScanTask
		if err := db.Where("pause_requested=true AND status IN ?", []string{"running", "recovering", "pausing"}).Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			if err := RequestPause(db, task.ID); err != nil {
				return err
			}
		}
		if reconcileCombinations != nil {
			if _, err := reconcileCombinations(db); err != nil {
				return err
			}
		}
		return nil
	}

	var tasks []database.VulnScanTask
	if err := db.
		Where("scan_mode = ? AND parent_combination_id = '' AND pause_requested = ? AND status IN ?", "local", true, []string{taskstate.StatusRunning, taskstate.StatusRecovering}).
		Find(&tasks).Error; err != nil {
		return fmt.Errorf("查询待收敛暂停任务失败: %w", err)
	}
	for i := range tasks {
		if err := FinalizePausedLocalTask(db, &tasks[i]); err != nil {
			return fmt.Errorf("收敛暂停任务失败: task=%s version=%d: %w", tasks[i].ID, tasks[i].Version, err)
		}
		logger.Info("服务重启后保留任务暂停状态: id=%s version=%d type=%s", tasks[i].ID, tasks[i].Version, tasks[i].ScanType)
	}

	if reconcileCombinations != nil {
		if _, err := reconcileCombinations(db); err != nil {
			return err
		}
	}
	return nil
}
