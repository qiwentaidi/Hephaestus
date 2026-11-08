package agentruntime

import (
	"context"
	"errors"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/portscan"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func saveWorkerPortScanStage(ctx context.Context, db *gorm.DB, payload *TaskPayload, stage, label string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lease := tx.Model(&database.VulnScanTaskItem{}).Select("1").
			Where("id = ? AND task_id = ? AND version = ?", payload.ItemID, payload.ID, payload.Version).
			Where("worker_agent_id = ? AND worker_generation = ?", payload.AgentID, payload.Generation).
			Where("status = 'running' AND lease_expires_at > CURRENT_TIMESTAMP")
		var task database.VulnScanTask
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND version = ? AND status = ? AND pause_requested = ?", payload.ID, payload.Version, taskstate.StatusRunning, false).
			Where("EXISTS (?)", lease).First(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return context.Canceled
		}
		if err != nil {
			return err
		}

		combination := task.ParentCombinationID != ""
		if combination && task.StepModule != "portscan" {
			return context.Canceled
		}
		checkpoint := taskstate.ScanCheckpoint(task, "portscan", combination)
		if !appendWorkerPortScanStage(&checkpoint, stage, label, time.Now()) {
			return nil
		}
		if checkpoint.Stage == "" {
			checkpoint.Stage = "portscan:agent"
		}
		if combination {
			return taskstate.SaveCombinationCheckpoint(tx, task.ID, task.Version, "portscan", checkpoint.Stage, checkpoint.Progress, checkpoint.Meta)
		}
		return tx.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", task.ID, task.Version).
			UpdateColumn("checkpoint_json", taskstate.MarshalCheckpoint(checkpoint.Stage, task.Progress, checkpoint.Meta)).Error
	})
}

// Keep the first occurrence of each phase across targets and retries.
func appendWorkerPortScanStage(checkpoint *taskstate.Checkpoint, stage, label string, now time.Time) bool {
	events, _ := checkpoint.Meta["stageEvents"].([]interface{})
	for _, raw := range events {
		if event, ok := raw.(map[string]interface{}); ok && event["stage"] == stage {
			return false
		}
	}
	if checkpoint.Meta == nil {
		checkpoint.Meta = make(map[string]interface{})
	}
	checkpoint.Meta["stageEvents"] = append(events, map[string]interface{}{"stage": stage, "label": label, "time": now})
	return true
}

func finishWorkerPortScanStages(checkpoint *taskstate.Checkpoint, allNoHosts bool, now time.Time) bool {
	events, _ := checkpoint.Meta["stageEvents"].([]interface{})
	if len(events) == 0 && !allNoHosts {
		return false
	}
	label := "扫描结束"
	if allNoHosts {
		label = "未确认可扫描主机"
	}
	return appendWorkerPortScanStage(checkpoint, portscan.StageComplete, label, now)
}
