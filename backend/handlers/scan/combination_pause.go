package scan

import (
	"fmt"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/notification"
	"hephaestus-backend/pkg/taskstate"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func finalizeCombinationProgress(
	db *gorm.DB,
	combo *database.CombinationTask,
	parentTask *database.VulnScanTask,
	allNodes []combinationPipelineNode,
	node combinationPipelineNode,
	ctxState *combinationContext,
	finishTime time.Time,
) error {
	if ctxState == nil {
		return fmt.Errorf("组合任务上下文缺失")
	}

	// Parallel modules can finish concurrently and overwrite combo.context.
	// Use row lock + merge to avoid losing Completed/Steps updates.
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	var latestCombo database.CombinationTask
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", combo.ID).
		First(&latestCombo).Error; err != nil {
		tx.Rollback()
		return err
	}

	latestCtx, err := loadCombinationContext(&latestCombo)
	if err != nil {
		tx.Rollback()
		return err
	}
	merged := mergeCombinationContext(latestCtx, ctxState)
	// This module is finished now; ensure running cleared and completed set
	if merged.Running != nil {
		delete(merged.Running, node.ModuleID)
	}
	if merged.Completed == nil {
		merged.Completed = make(map[string]bool)
	}
	merged.Completed[node.ModuleID] = true

	// Keep in-memory state in sync for downstream scheduling decisions.
	*ctxState = *merged

	completedCount := countCompletedModules(merged, allNodes)
	progress := 0
	if len(allNodes) > 0 {
		progress = int(float64(completedCount) / float64(len(allNodes)) * 100)
	}

	comboProgress := progress
	comboStatus := "running"
	var comboCompletedAt *time.Time
	if completedCount >= len(allNodes) {
		comboProgress = 100
		comboStatus = "completed"
		comboCompletedAt = &finishTime
	} else if latestCombo.PauseRequested && len(merged.Running) == 0 {
		comboStatus = taskstate.StatusPaused
	}
	if latestCombo.Status == "failed" || latestCombo.Status == "cancelled" {
		comboStatus = latestCombo.Status
		comboCompletedAt = latestCombo.CompletedAt
	}

	comboUpdates := map[string]interface{}{
		"context":         merged.serialize(),
		"progress":        comboProgress,
		"current_step":    completedCount,
		"total_steps":     len(allNodes),
		"status":          comboStatus,
		"pause_requested": latestCombo.PauseRequested && len(merged.Running) > 0,
		"completed_at":    comboCompletedAt,
		"updated_at":      finishTime,
	}
	if combo.StartedAt == nil {
		comboUpdates["started_at"] = &finishTime
		combo.StartedAt = &finishTime
	}
	if err := tx.Model(&database.CombinationTask{}).
		Where("id = ?", combo.ID).
		Updates(comboUpdates).Error; err != nil {
		tx.Rollback()
		return err
	}

	parentStatus := "running"
	var parentCompletedAt *time.Time
	if comboStatus == "completed" {
		parentStatus = "completed"
		parentCompletedAt = &finishTime
	} else if latestCombo.PauseRequested && len(merged.Running) == 0 {
		parentStatus = taskstate.StatusPaused
	}
	if comboStatus == "failed" || comboStatus == "cancelled" {
		parentStatus = comboStatus
		parentCompletedAt = comboCompletedAt
	}

	parentUpdates := map[string]interface{}{
		"progress":        progress,
		"status":          parentStatus,
		"scan_type":       "combination",
		"step_index":      completedCount,
		"step_module":     node.ModuleID,
		"pause_requested": latestCombo.PauseRequested && len(merged.Running) > 0,
		"completed_at":    parentCompletedAt,
		"updated_at":      finishTime,
	}
	if parentTask.StartedAt == nil {
		parentUpdates["started_at"] = &finishTime
		parentTask.StartedAt = &finishTime
	}
	// Module writers lock this same parent row. Read its latest checkpoint
	// under the lock so the scheduler cannot overwrite newly saved modules.
	var latestParent database.VulnScanTask
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).First(&latestParent).Error; err != nil {
		tx.Rollback()
		return err
	}
	parentUpdates["current_stage"] = "combination"
	parentUpdates["checkpoint_json"] = taskstate.CombinationSummaryCheckpoint(latestParent.CheckpointJSON, progress)
	if err := tx.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).
		Updates(parentUpdates).Error; err != nil {
		tx.Rollback()
		return err
	}

	if comboStatus == "completed" {
		var vulnCount int64
		tx.Model(&database.VulnerabilityInfo{}).
			Where("task_id = ? AND version = ?", parentTask.ID, parentTask.Version).
			Count(&vulnCount)
		go notification.NotifyScanComplete(parentTask.ID, parentTask.TaskName, int(vulnCount), "completed")
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}
	combo.Context = merged.serialize()
	combo.Progress = comboProgress
	combo.CurrentStep = completedCount
	combo.TotalSteps = len(allNodes)
	combo.Status = comboStatus
	combo.PauseRequested = latestCombo.PauseRequested && len(merged.Running) > 0
	combo.CompletedAt = comboCompletedAt
	combo.UpdatedAt = finishTime
	return nil
}

func finalizeCombinationPause(
	db *gorm.DB,
	combo *database.CombinationTask,
	parentTask *database.VulnScanTask,
	node combinationPipelineNode,
	ctxState *combinationContext,
	finishTime time.Time,
) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	var latestCombo database.CombinationTask
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", combo.ID).
		First(&latestCombo).Error; err != nil {
		tx.Rollback()
		return err
	}

	latestCtx, err := loadCombinationContext(&latestCombo)
	if err != nil {
		tx.Rollback()
		return err
	}
	merged := mergeCombinationContext(latestCtx, ctxState)
	if merged.Running != nil {
		delete(merged.Running, node.ModuleID)
	}

	*ctxState = *merged

	comboStatus := taskstate.StatusRunning
	parentStatus := taskstate.StatusRunning
	pauseRequested := latestCombo.PauseRequested
	if len(merged.Running) == 0 {
		comboStatus = taskstate.StatusPaused
		parentStatus = taskstate.StatusPaused
		pauseRequested = false
	}

	if err := tx.Model(&database.CombinationTask{}).
		Where("id = ?", combo.ID).
		Updates(map[string]interface{}{
			"context":         merged.serialize(),
			"status":          comboStatus,
			"pause_requested": pauseRequested,
			"updated_at":      finishTime,
		}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).
		Updates(map[string]interface{}{
			"status":          parentStatus,
			"pause_requested": pauseRequested,
			"scan_type":       "combination",
			"updated_at":      finishTime,
		}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}
	combo.Context = merged.serialize()
	combo.Status = comboStatus
	combo.PauseRequested = pauseRequested
	combo.UpdatedAt = finishTime
	return nil
}

// ReconcilePausedCombinationTasksAfterRestart finalizes pause requests that
// outlived a process restart. Unlike interrupted running tasks, a persisted
// pause request is explicit user intent and must never be re-enqueued.
func ReconcilePausedCombinationTasksAfterRestart(db *gorm.DB) (int, error) {
	if db == nil {
		return 0, nil
	}

	var combos []database.CombinationTask
	if err := db.
		Where("pause_requested = ? AND status IN ?", true, []string{taskstate.StatusRunning, taskstate.StatusRecovering, "pausing"}).
		Find(&combos).Error; err != nil {
		return 0, fmt.Errorf("查询待收敛暂停组合任务失败: %w", err)
	}

	for i := range combos {
		if err := finalizeRestartPausedCombinationTask(db, &combos[i]); err != nil {
			return i, err
		}
	}
	return len(combos), nil
}

func finalizeRestartPausedCombinationTask(db *gorm.DB, combo *database.CombinationTask) error {
	if db == nil || combo == nil {
		return nil
	}

	now := time.Now()
	return db.Transaction(func(tx *gorm.DB) error {
		var latestCombo database.CombinationTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", combo.ID).
			First(&latestCombo).Error; err != nil {
			return err
		}
		if !latestCombo.PauseRequested {
			return nil
		}
		if tx.Dialector.Name() == "postgres" {
			parent, err := getCombinationParentTask(tx, latestCombo.ID)
			if err != nil {
				return err
			}
			var active int64
			if err = tx.Model(&database.VulnScanTaskItem{}).Where("task_id=? AND version=? AND status='running'", parent.ID, parent.Version).Count(&active).Error; err != nil {
				return err
			}
			if active > 0 {
				if err = tx.Model(&database.CombinationTask{}).Where("id=?", latestCombo.ID).Update("status", "pausing").Error; err != nil {
					return err
				}
				return tx.Model(&database.VulnScanTask{}).Where("id=? AND version=?", parent.ID, parent.Version).Updates(map[string]any{"pause_requested": true, "status": "pausing"}).Error
			}
		}

		ctxState, err := loadCombinationContext(&latestCombo)
		if err != nil {
			return err
		}
		for module := range ctxState.Running {
			ctxState.markStepFinished(module, taskstate.StatusPaused, nil, now)
		}
		ctxState.Running = make(map[string]bool)

		parentTask, err := getCombinationParentTask(tx, latestCombo.ID)
		if err != nil {
			return err
		}
		if err := tx.Model(&database.CombinationTask{}).
			Where("id = ?", latestCombo.ID).
			Updates(map[string]interface{}{
				"context":         ctxState.serialize(),
				"status":          taskstate.StatusPaused,
				"pause_requested": false,
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).
			Updates(map[string]interface{}{
				"status":          taskstate.StatusPaused,
				"pause_requested": false,
				"scan_type":       "combination",
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}

		combo.Context = ctxState.serialize()
		combo.Status = taskstate.StatusPaused
		combo.PauseRequested = false
		combo.UpdatedAt = now
		return nil
	})
}
