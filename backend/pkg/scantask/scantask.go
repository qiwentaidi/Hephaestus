// Package scantask 提供扫描任务的通用生命周期操作：模式归一化、取消、恢复与并发计数。
//
// 这些能力同时被 handlers/scan（HTTP 层）和 AI 编排使用，因此下沉到 pkg，
// 避免 AI 侧直接依赖 handlers 包。
package scantask

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/taskstate"
)

// ErrNotCancelable 表示任务不存在或已处于终态，无法取消。
var ErrNotCancelable = errors.New("scan task not found or not cancellable")

func CancelTaskWithDB(db *gorm.DB, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if db == nil {
		return fmt.Errorf("数据库连接不能为空")
	}
	if taskID == "" {
		return ErrNotCancelable
	}
	if db.Dialector.Name() == "postgres" {
		return db.Transaction(func(tx *gorm.DB) error {
			var task database.VulnScanTask
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", taskID).Order("version DESC").First(&task).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrNotCancelable
				}
				return err
			}
			if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" {
				return ErrNotCancelable
			}
			now := time.Now()
			var running int64
			if err := tx.Model(&database.VulnScanTaskItem{}).Where("task_id=? AND version=? AND status='running'", taskID, task.Version).Count(&running).Error; err != nil {
				return err
			}
			if err := tx.Model(&database.VulnScanTaskItem{}).Where("task_id=? AND version=? AND status IN ?", taskID, task.Version, []string{"pending", "paused"}).Updates(map[string]any{"status": "cancelled", "updated_at": now}).Error; err != nil {
				return err
			}
			state := "cancelling"
			if running == 0 {
				state = "cancelled"
			}
			if err := tx.Model(&database.VulnScanTask{}).Where("id=? AND version=?", taskID, task.Version).Updates(map[string]any{"status": state, "pause_requested": false, "updated_at": now}).Error; err != nil {
				return err
			}
			// Keep the combination orchestration row in step with the parent
			// task. The SQLite path below has always done this; PostgreSQL used
			// to leave a cancelled parent next to a running combination.
			if task.ParentCombinationID != "" {
				if err := tx.Model(&database.CombinationTask{}).
					Where("id = ?", task.ParentCombinationID).
					Updates(map[string]any{
						"status":          taskstate.StatusCancelled,
						"pause_requested": false,
						"error_msg":       "任务已取消",
						"completed_at":    now,
						"updated_at":      now,
					}).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := control.GetTaskController().CancelTask(taskID); err != nil {
		// A queued, remote, or already stopped task may have no local context;
		// the durable state transition below remains authoritative.
		logger.Debug("取消任务 context 失败: %v", err)
	}

	var rowsAffected int64
	var comboID string
	var currentTask database.VulnScanTask
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", taskID).Order("version DESC").First(&currentTask).Error; err == nil {
			comboID = currentTask.ParentCombinationID
		}

		var runningAgentItems []database.VulnScanTaskItem
		if err := tx.Where(
			"task_id = ? AND status = ? AND worker_agent_id <> ''",
			taskID, "running",
		).Find(&runningAgentItems).Error; err != nil {
			return err
		}

		commandedAgents := make(map[string]struct{})
		for _, item := range runningAgentItems {
			if item.WorkerAgentID == "" {
				continue
			}
			if _, ok := commandedAgents[item.WorkerAgentID]; ok {
				continue
			}
			commandedAgents[item.WorkerAgentID] = struct{}{}
			cmd := database.ScanAgentCommand{
				AgentID:     item.WorkerAgentID,
				TaskID:      item.TaskID,
				TaskVersion: item.Version,
				Command:     "cancel",
				Status:      "pending",
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cmd).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&database.VulnScanTaskItem{}).
			Where("task_id = ? AND status IN ?", taskID, []string{taskstate.StatusPending, taskstate.StatusRunning}).
			Updates(map[string]interface{}{
				"status":           taskstate.StatusCancelled,
				"lease_expires_at": nil,
				"updated_at":       time.Now(),
			}).Error; err != nil {
			return err
		}

		result := tx.Model(&database.VulnScanTask{}).
			Where("id = ? AND status IN ?", taskID, []string{taskstate.StatusPending, taskstate.StatusRunning, taskstate.StatusPaused, taskstate.StatusRecovering, "cancelling"}).
			Updates(map[string]interface{}{
				"status":           taskstate.StatusCancelled,
				"pause_requested":  false,
				"lease_expires_at": nil,
				"heartbeat_at":     nil,
				"updated_at":       time.Now(),
			})
		if result.Error != nil {
			return result.Error
		}
		rowsAffected = result.RowsAffected
		if rowsAffected == 0 {
			return ErrNotCancelable
		}

		if comboID != "" {
			if err := tx.Model(&database.CombinationTask{}).
				Where("id = ?", comboID).
				Updates(map[string]interface{}{
					"status":          taskstate.StatusCancelled,
					"pause_requested": false,
					"error_msg":       "任务已取消",
					"completed_at":    time.Now(),
					"updated_at":      time.Now(),
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func ResetForResume(tx *gorm.DB, task *database.VulnScanTask) error {
	if tx.Dialector.Name() == "postgres" && UsePersistentWorkers(tx, task.ScanMode) {
		var running int64
		if err := tx.Model(&database.VulnScanTaskItem{}).Where("task_id=? AND version=? AND status='running'", task.ID, task.Version).Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return fmt.Errorf("Scan Worker 尚未确认退出，暂不能继续任务")
		}
		if err := tx.Model(&database.VulnScanTaskItem{}).Where("task_id=? AND version=? AND status='paused'", task.ID, task.Version).Updates(map[string]any{"status": "pending", "worker_agent_id": "", "lease_expires_at": nil, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
	}

	updates := map[string]interface{}{
		"status":           taskstate.StatusRunning,
		"pause_requested":  false,
		"error_msg":        "",
		"worker_agent_id":  "",
		"lease_expires_at": nil,
		"heartbeat_at":     time.Now(),
		"updated_at":       time.Now(),
	}
	return tx.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", task.ID, task.Version).
		Updates(updates).Error
}

func NormalizeMode(scanMode string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(scanMode))
	if mode == "" {
		return "auto", nil
	}
	if mode != "local" && mode != "agent" && mode != "auto" {
		return "", fmt.Errorf("scanMode must be auto, local or agent")
	}
	return mode, nil
}

func NormalizeModeForDB(db *gorm.DB, mode string) (string, error) {
	normalized, err := NormalizeMode(mode)
	if err == nil && normalized == "auto" && db.Dialector.Name() != "postgres" {
		normalized = "local"
	}
	return normalized, err
}

func UsePersistentWorkers(db *gorm.DB, mode string) bool {
	return mode == "agent" || (db.Dialector.Name() == "postgres" && (mode == "auto" || mode == "local"))
}

func AgentRunningCount(db *gorm.DB, id string) int64 {
	if db.Dialector.Name() != "postgres" {
		return 0
	}
	var count int64
	_ = db.Table("pentest_worker_turns").Where("node_id=? AND status IN ('running','cancelling') AND lease_expires_at>CURRENT_TIMESTAMP", id).Count(&count).Error
	return count
}

func ScanRunningCount(db *gorm.DB, id string) int64 {
	var count int64
	_ = db.Model(&database.VulnScanTaskItem{}).Where("worker_agent_id=? AND status='running' AND lease_expires_at>CURRENT_TIMESTAMP", id).Count(&count).Error
	return count
}
