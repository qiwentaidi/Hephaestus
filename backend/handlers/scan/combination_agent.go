package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hephaestus-backend/handlers/agent"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func dispatchCombinationModuleToAgent(
	ctx context.Context,
	db *gorm.DB,
	parentTask *database.VulnScanTask,
	node combinationPipelineNode,
	nodeIndex int,
	targets []string,
	config map[string]interface{},
) error {
	now := time.Now()
	targetsJSON, _ := json.Marshal(targets)
	configJSON, _ := json.Marshal(config)

	updates := map[string]interface{}{
		"scan_mode":         parentTask.ScanMode,
		"scan_type":         "combination",
		"assigned_agent_id": parentTask.AssignedAgentID,
		"worker_agent_id":   "",
		"lease_expires_at":  nil,
		"targets":           string(targetsJSON),
		"config":            string(configJSON),
		"status":            "pending",
		"error_msg":         "",
		"step_index":        nodeIndex + 1,
		"step_module":       node.ModuleID,
		"updated_at":        now,
	}
	if parentTask.StartedAt == nil {
		updates["started_at"] = &now
		parentTask.StartedAt = &now
	}

	itemTask := *parentTask
	itemTask.ScanMode = parentTask.ScanMode
	itemTask.ScanType = node.ModuleID
	itemTask.Targets = string(targetsJSON)
	itemTask.Config = string(configJSON)
	itemTask.Status = taskstate.StatusPending
	itemTask.WorkerAgentID = ""
	itemTask.LeaseExpiresAt = nil
	itemTask.ErrorMsg = ""
	itemTask.StepIndex = nodeIndex + 1
	itemTask.StepModule = node.ModuleID
	itemTask.UpdatedAt = now
	if err := db.Transaction(func(tx *gorm.DB) error {
		var latest database.VulnScanTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).First(&latest).Error; err != nil {
			return err
		}
		if latest.Status == taskstate.StatusCancelled || latest.Status == taskstate.StatusDeleting {
			return context.Canceled
		}
		if err := tx.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).Updates(updates).Error; err != nil {
			return err
		}
		return agent.ResetAgentTaskItems(tx, &itemTask)
	}); err != nil {
		return fmt.Errorf("下发组合任务模块到 Agent 失败: %w", err)
	}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if db.Dialector.Name() == "postgres" {
				pause := shouldFinalizeCombinationPauseOnCancel(db, parentTask.ParentCombinationID, parentTask.ID, parentTask.Version)
				if pause {
					if err := scantask.RequestPause(db, parentTask.ID); err != nil {
						return err
					}
				} else {
					if err := scantask.CancelTaskWithDB(db, parentTask.ID); err != nil && !errors.Is(err, scantask.ErrNotCancelable) {
						return err
					}
				}
				if err := scantask.WaitExit(db, parentTask.ID, parentTask.Version); err != nil {
					return err
				}
				if pause {
					return taskstate.ErrTaskPaused
				}
			}
			return context.Canceled
		case <-ticker.C:
			var current database.VulnScanTask
			if err := db.Select("status", "error_msg", "checkpoint_json").Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).First(&current).Error; err != nil {
				return fmt.Errorf("轮询组合任务模块状态失败: %w", err)
			}
			if taskstate.ScanCheckpoint(current, node.ModuleID, true).Stage == node.ModuleID+":completed" {
				return nil
			}
			switch strings.ToLower(strings.TrimSpace(current.Status)) {
			case "completed":
				return nil
			case "failed":
				if strings.TrimSpace(current.ErrorMsg) == "" {
					return fmt.Errorf("模块 %s 执行失败", node.ModuleID)
				}
				return errors.New(current.ErrorMsg)
			case "cancelled":
				return context.Canceled
			case "paused":
				return taskstate.ErrTaskPaused
			}
		}
	}
}
