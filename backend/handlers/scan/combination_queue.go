package scan

import (
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func parseCombinationNodes(raw string) ([]combinationPipelineNode, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("组合任务缺少节点配置")
	}
	var nodes []combinationPipelineNode
	if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
		return nil, fmt.Errorf("解析组合任务节点失败: %w", err)
	}
	return nodes, nil
}

func countCompletedModules(ctx *combinationContext, nodes []combinationPipelineNode) int {
	if ctx == nil || len(nodes) == 0 {
		return 0
	}
	count := 0
	for _, node := range nodes {
		if ctx.Completed != nil && ctx.Completed[node.ModuleID] {
			count++
		}
	}
	return count
}

func dependenciesSatisfied(module string, completed map[string]bool) bool {
	required := combinationDependencies[module]
	for _, dep := range required {
		if !completed[dep] {
			return false
		}
	}
	return true
}

func scheduleNextCombinationNodes(db *gorm.DB, combo *database.CombinationTask, nodes []combinationPipelineNode, ctxState *combinationContext) (int, error) {
	if combo == nil || ctxState == nil {
		return 0, nil
	}
	var ready []int
	err := db.Transaction(func(tx *gorm.DB) error {
		var latest database.CombinationTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", combo.ID).First(&latest).Error; err != nil {
			return err
		}
		latestCtx, err := loadCombinationContext(&latest)
		if err != nil {
			return err
		}
		parentTask, err := getCombinationParentTask(tx, combo.ID)
		if err != nil {
			return err
		}
		latestCtx.ensureStepLogs(nodes, parentTask.ID)
		ready, err = queueCombinationNodesLocked(tx, &latest, nodes, latestCtx)
		if err != nil {
			return err
		}
		*combo, *ctxState = latest, *latestCtx
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, idx := range ready {
		enqueueCombinationStep(combinationEvent{CombinationID: combo.ID, NodeIndex: idx})
	}
	return len(ready), nil
}

func queueCombinationNodesLocked(db *gorm.DB, combo *database.CombinationTask, nodes []combinationPipelineNode, ctxState *combinationContext) ([]int, error) {
	if combo == nil || ctxState == nil {
		return nil, nil
	}
	if combo.Status == "failed" || combo.Status == "cancelled" || combo.Status == "completed" || combo.Status == taskstate.StatusPaused || combo.PauseRequested {
		return nil, nil
	}
	if ctxState.Running == nil {
		ctxState.Running = make(map[string]bool)
	}
	if ctxState.Completed == nil {
		ctxState.Completed = make(map[string]bool)
	}

	activeStage := -1
	for _, node := range nodes {
		if ctxState.Completed[node.ModuleID] {
			continue
		}
		if activeStage == -1 || node.StageIndex < activeStage {
			activeStage = node.StageIndex
		}
	}
	if activeStage == -1 {
		return nil, nil
	}

	ready := make([]int, 0)
	for idx, node := range nodes {
		if node.StageIndex != activeStage {
			continue
		}
		if ctxState.Completed[node.ModuleID] {
			continue
		}
		if ctxState.Running[node.ModuleID] {
			continue
		}
		if dependenciesSatisfied(node.ModuleID, ctxState.Completed) {
			ready = append(ready, idx)
		}
	}
	if len(ready) == 0 {
		return nil, nil
	}
	availableSlots := maxCombinationParallel - len(ctxState.Running)
	if parentTask, err := getCombinationParentTask(db, combo.ID); err == nil {
		if scantask.UsePersistentWorkers(db, parentTask.ScanMode) {
			availableSlots = 1 - len(ctxState.Running)
		}
	}
	if availableSlots <= 0 {
		return nil, nil
	}
	if len(ready) > availableSlots {
		ready = ready[:availableSlots]
	}

	now := time.Now()
	for _, idx := range ready {
		ctxState.Running[nodes[idx].ModuleID] = true
		ctxState.markStepQueued(nodes[idx].ModuleID, combo.ID, now)
	}

	updates := map[string]interface{}{
		"context":    ctxState.serialize(),
		"updated_at": now,
	}
	if combo.Status == "pending" {
		updates["status"] = "running"
		if combo.StartedAt == nil {
			updates["started_at"] = &now
			combo.StartedAt = &now
		}
		combo.Status = "running"
	}
	if err := db.Model(&database.CombinationTask{}).
		Where("id = ?", combo.ID).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	return ready, nil
}
