package scantask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hephaestus-backend/pkg/agentruntime"
	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/notification"
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/taskqueue"
	"hephaestus-backend/pkg/taskstate"
	"hephaestus-backend/pkg/webscan"
)

var ErrExitPending = errors.New("scan worker exit acknowledgement pending")

func EnqueueLocalTask(taskID string) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return EnqueueLocalTaskWithDB(db, taskID)
}

func EnqueueLocalTaskWithDB(db *gorm.DB, taskID string) error {
	if db == nil {
		return fmt.Errorf("数据库连接不能为空")
	}
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		return fmt.Errorf("获取任务信息失败: %w", err)
	}

	if UsePersistentWorkers(db, task.ScanMode) {
		return agentruntime.EnsureAgentTaskItems(db, &task)
	}
	return taskqueue.GetGlobalQueue().Submit(newLocalScanQueueTask(db, task.ID, task.TaskName))
}

func newLocalScanQueueTask(db *gorm.DB, taskID, taskName string) *taskqueue.Task {
	// The caller may bind db to an HTTP/AI turn context. The queued scan must
	// keep using the same database, but its lifetime is independent of that turn.
	workerDB := db.Session(&gorm.Session{NewDB: true, Context: context.Background()})
	return &taskqueue.Task{
		ID:   taskID,
		Name: taskName,
		Fn: func() error {
			return RunLocalTaskWithDB(workerDB, taskID)
		},
	}
}

func RunLocalTask(taskID string) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return RunLocalTaskWithDB(db, taskID)
}

func RunLocalTaskWithDB(db *gorm.DB, taskID string) error {
	if db == nil {
		return fmt.Errorf("数据库连接不能为空")
	}
	var latestTask database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&latestTask).Error; err != nil {
		return fmt.Errorf("获取任务信息失败: %w", err)
	}
	if UsePersistentWorkers(db, latestTask.ScanMode) {
		if latestTask.PauseRequested {
			return RequestPause(db, latestTask.ID)
		}
		return agentruntime.EnsureAgentTaskItems(db, &latestTask)
	}

	if latestTask.Status != taskstate.StatusRunning && latestTask.Status != taskstate.StatusRecovering {
		logger.Info("跳过非运行态任务执行: id=%s status=%s", latestTask.ID, latestTask.Status)
		return nil
	}

	if latestTask.PauseRequested {
		return FinalizePausedLocalTask(db, &latestTask)
	}

	if ShouldFinalizeRecoveredTask(&latestTask) {
		if err := FinalizeRecoveredTask(db, &latestTask, "任务状态异常，已自动纠正为完成"); err != nil {
			return fmt.Errorf("纠正异常终态失败: %w", err)
		}
		logger.Warning("跳过异常恢复任务并自动完成: id=%s version=%d type=%s", latestTask.ID, latestTask.Version, latestTask.ScanType)
		return nil
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(latestTask.Config), &config); err != nil {
		return fmt.Errorf("解析任务配置失败: %w", err)
	}

	var targets []string
	if err := json.Unmarshal([]byte(latestTask.Targets), &targets); err != nil {
		return fmt.Errorf("解析任务目标失败: %w", err)
	}

	taskCtrl := control.GetTaskController()
	ctx, cancel := taskCtrl.CreateTaskContext(taskID)
	defer cancel()

	stage := latestTask.CurrentStage
	if stage == "" {
		stage = "prepare"
	}
	now := time.Now()
	markRunningResult := db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ? AND pause_requested = ? AND status IN ?", latestTask.ID, latestTask.Version, false, []string{taskstate.StatusRunning, taskstate.StatusRecovering}).
		Updates(map[string]interface{}{
			"status":        taskstate.StatusRunning,
			"current_stage": stage,
			"heartbeat_at":  now,
			"started_at":    now,
			"updated_at":    now,
		})
	if markRunningResult.Error != nil {
		return fmt.Errorf("更新任务运行状态失败: %w", markRunningResult.Error)
	}
	if markRunningResult.RowsAffected == 0 {
		var current database.VulnScanTask
		if err := db.Select("status", "pause_requested").Where("id = ? AND version = ?", latestTask.ID, latestTask.Version).First(&current).Error; err == nil {
			if current.PauseRequested {
				return FinalizePausedLocalTask(db, &latestTask)
			}
			logger.Info("跳过已变更状态的恢复任务执行: id=%s version=%d status=%s", latestTask.ID, latestTask.Version, current.Status)
		}
		return nil
	}
	heartbeatStop := taskstate.StartHeartbeat(ctx, db, latestTask.ID, latestTask.Version)
	defer heartbeatStop()

	taskScanType := latestTask.ScanType
	handler, ok := scanexec.Handlers[taskScanType]
	if !ok {
		handler = webscan.ExecuteLocalScanWithContext
	}

	var err error
	err = handler(ctx, latestTask.ID, targets, config)
	if err != nil {
		if errors.Is(err, taskstate.ErrTaskPaused) {
			if markErr := FinalizePausedLocalTask(db, &latestTask); markErr != nil {
				logger.Warning("更新任务暂停状态失败: %v", markErr)
			}
			return nil
		}
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			var current database.VulnScanTask
			if lookupErr := db.Select("status", "pause_requested").Where("id = ? AND version = ?", latestTask.ID, latestTask.Version).First(&current).Error; lookupErr == nil {
				if current.Status == taskstate.StatusPaused {
					return nil
				}
				if current.PauseRequested {
					if markErr := FinalizePausedLocalTask(db, &latestTask); markErr != nil {
						logger.Warning("更新任务暂停状态失败: %v", markErr)
					}
					return nil
				}
			}
			if markErr := taskstate.MarkCancelled(db, latestTask.ID, latestTask.Version, "任务已被用户取消"); markErr != nil {
				logger.Warning("更新任务取消状态失败: %v", markErr)
			}
			return err
		}

		if markErr := taskstate.MarkFailed(db, latestTask.ID, latestTask.Version, err); markErr != nil {
			logger.Warning("更新任务失败状态失败: %v", markErr)
		}
		go notification.NotifyScanComplete(latestTask.ID, latestTask.TaskName, 0, "failed")
		return err
	}

	if err := taskstate.MarkCompleted(db, latestTask.ID, latestTask.Version); err != nil {
		return fmt.Errorf("更新任务完成状态失败: %w", err)
	}
	if _, syncErr := assetstore.SyncCompletedTask(ctx, db, latestTask.ID, latestTask.Version); syncErr != nil {
		logger.Warning("统一资产目录同步失败: task=%s version=%d err=%v", latestTask.ID, latestTask.Version, syncErr)
	}

	go func() {
		var vulnCount int64
		db.Model(&database.VulnerabilityInfo{}).
			Where("task_id = ? AND version = ?", latestTask.ID, latestTask.Version).
			Count(&vulnCount)
		notification.NotifyScanComplete(
			latestTask.ID,
			latestTask.TaskName,
			int(vulnCount),
			"completed",
		)
	}()
	return nil
}

func FinalizePausedLocalTask(db *gorm.DB, task *database.VulnScanTask) error {
	if db == nil || task == nil {
		return nil
	}

	var current database.VulnScanTask
	if err := db.Select("status").Where("id = ? AND version = ?", task.ID, task.Version).First(&current).Error; err != nil {
		return err
	}
	if current.Status == taskstate.StatusPaused {
		return nil
	}
	return taskstate.MarkPaused(db, task.ID, task.Version, "任务已暂停，等待继续")
}

func ShouldFinalizeRecoveredTask(task *database.VulnScanTask) bool {
	if task == nil {
		return false
	}
	if task.CompletedAt != nil {
		return true
	}
	if task.Progress >= 100 {
		return true
	}

	stage := strings.ToLower(strings.TrimSpace(task.CurrentStage))
	if stage == taskstate.StatusCompleted || stage == "completed" {
		return true
	}

	checkpoint := taskstate.ParseCheckpoint(task.CheckpointJSON)
	if strings.EqualFold(strings.TrimSpace(checkpoint.Stage), taskstate.StatusCompleted) {
		return true
	}
	if checkpoint.Progress >= 100 {
		return true
	}

	return false
}

func FinalizeRecoveredTask(db *gorm.DB, task *database.VulnScanTask, message string) error {
	if db == nil || task == nil {
		return nil
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":          taskstate.StatusCompleted,
		"progress":        100,
		"pause_requested": false,
		"current_stage":   taskstate.StatusCompleted,
		"checkpoint_json": taskstate.MarshalCheckpoint(taskstate.StatusCompleted, 100, nil),
		"heartbeat_at":    nil,
		"updated_at":      now,
		"error_msg":       message,
	}
	if task.CompletedAt == nil {
		updates["completed_at"] = &now
	}

	if err := db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", task.ID, task.Version).
		Updates(updates).Error; err != nil {
		return err
	}

	task.Status = taskstate.StatusCompleted
	task.Progress = 100
	task.CurrentStage = taskstate.StatusCompleted
	task.CheckpointJSON = taskstate.MarshalCheckpoint(taskstate.StatusCompleted, 100, nil)
	task.HeartbeatAt = nil
	task.ErrorMsg = message
	if task.CompletedAt == nil {
		task.CompletedAt = &now
	}
	return nil
}

func AgentResumeUnsupported(task database.VulnScanTask) error {
	if strings.EqualFold(strings.TrimSpace(task.ScanMode), "agent") {
		if db, err := database.GetDB(); err == nil && db.Dialector.Name() == "postgres" {
			return nil
		}
		return fmt.Errorf("Agent 模式任务暂不支持暂停后继续")
	}
	return nil
}

func RequestPause(db *gorm.DB, id string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var task database.VulnScanTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Order("version DESC").First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotCancelable
			}
			return err
		}
		if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" || task.Status == "paused" {
			return nil
		}
		var active int64
		if err := tx.Model(&database.VulnScanTaskItem{}).Where("task_id=? AND version=? AND status='running'", id, task.Version).Count(&active).Error; err != nil {
			return err
		}
		state := "pausing"
		if active == 0 {
			state = "paused"
		}
		return tx.Model(&database.VulnScanTask{}).Where("id=? AND version=?", id, task.Version).Updates(map[string]any{"pause_requested": true, "status": state, "updated_at": time.Now()}).Error
	})
}

func WaitExit(db *gorm.DB, id string, version int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var running int64
		if err := db.WithContext(ctx).Model(&database.VulnScanTaskItem{}).Where("task_id=? AND version=? AND status='running'", id, version).Count(&running).Error; err != nil {
			return fmt.Errorf("%w: %v", ErrExitPending, err)
		}
		if running == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrExitPending
		case <-ticker.C:
		}
	}
}
