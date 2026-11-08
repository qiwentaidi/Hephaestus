package scan

import (
	"context"
	"errors"
	"fmt"
	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/subdomain"
	"hephaestus-backend/pkg/taskqueue"
	"hephaestus-backend/pkg/taskstate"
	"maps"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func runCombinationModule(
	db *gorm.DB,
	combo *database.CombinationTask,
	parentTask *database.VulnScanTask,
	allNodes []combinationPipelineNode,
	node combinationPipelineNode,
	nodeIndex int,
	targets []string,
	ctxState *combinationContext,
) error {
	handler, ok := scanTypeHandlers[node.ModuleID]
	if !ok || handler == nil {
		if display, exists := moduleDisplayName[node.ModuleID]; exists {
			return fmt.Errorf("模块 %s 暂不支持", display)
		}
		return fmt.Errorf("模块 %s 暂不支持", node.ModuleID)
	}

	taskID := parentTask.ID
	taskCtrl := control.GetTaskController()
	subTaskID := fmt.Sprintf("%d-%s", nodeIndex, node.ModuleID)
	var ctx context.Context
	var cancel context.CancelFunc

	now := time.Now()
	claimed := false
	paused := false
	if err := db.Transaction(func(tx *gorm.DB) error {
		var latest database.CombinationTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", combo.ID).First(&latest).Error; err != nil {
			return err
		}
		latestCtx, err := loadCombinationContext(&latest)
		if err != nil {
			return err
		}
		*combo, *ctxState = latest, *latestCtx
		if latest.PauseRequested || latest.Status == taskstate.StatusPaused {
			if step := latestCtx.stepLog(node.ModuleID); step != nil && step.Status == "running" {
				return nil
			}
			paused = true
			return nil
		}
		if latest.Status == "failed" || latest.Status == "cancelled" || latest.Status == "completed" || latestCtx.Completed[node.ModuleID] {
			return nil
		}
		if step := latestCtx.stepLog(node.ModuleID); step != nil && step.Status != "queued" {
			return nil
		}
		ctxState.Running[node.ModuleID] = true
		ctxState.markStepRunning(node.ModuleID, taskID, now)
		comboUpdates := map[string]interface{}{
			"context": ctxState.serialize(), "status": "running", "updated_at": now,
		}
		if combo.StartedAt == nil {
			comboUpdates["started_at"] = &now
			combo.StartedAt = &now
		}
		if err := tx.Model(&database.CombinationTask{}).Where("id = ?", combo.ID).Updates(comboUpdates).Error; err != nil {
			return err
		}
		taskUpdates := map[string]interface{}{
			"status": "running", "step_index": nodeIndex + 1, "step_module": node.ModuleID, "updated_at": now,
		}
		if parentTask.StartedAt == nil {
			taskUpdates["started_at"] = &now
			parentTask.StartedAt = &now
		}
		if err := tx.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", taskID, parentTask.Version).Updates(taskUpdates).Error; err != nil {
			return err
		}
		// Register after the atomic claim and before its commit. A pause request
		// waiting for this transaction will then cancel the registered context.
		ctx, cancel = taskCtrl.CreateSubTaskContext(taskID, subTaskID)
		claimed = true
		return nil
	}); err != nil {
		if cancel != nil {
			cancel()
		}
		return err
	}
	if paused {
		ctxState.markStepFinished(node.ModuleID, taskstate.StatusPaused, nil, now)
		return finalizeCombinationPause(db, combo, parentTask, node, ctxState, now)
	}
	if !claimed {
		return nil
	}
	defer cancel()

	// Use node config snapshot (from frontend) so each module runs with the intended settings.
	// Also inject targets for compatibility with handlers that read from config.
	config := map[string]interface{}{}
	if node.Config != nil {
		config = maps.Clone(node.Config)
	}
	config["targets"] = targets
	config["isCombination"] = true
	if node.ModuleID == "portscan" && len(ctxState.PortTargetAliases) > 0 {
		config["targetAliases"] = exportPortTargetAliases(ctxState.PortTargetAliases)
	}

	// Backward compatibility: early frontend builds used `aiDetectionEnabled` instead of `openai.enabled`.
	if node.ModuleID == "jsscan" {
		if _, hasOpenAI := config["openai"]; !hasOpenAI {
			if enabled, ok := config["aiDetectionEnabled"].(bool); ok {
				config["openai"] = map[string]interface{}{"enabled": enabled}
			}
		}
	}

	if scantask.UsePersistentWorkers(db, parentTask.ScanMode) {
		err := dispatchCombinationModuleToAgent(ctx, db, parentTask, node, nodeIndex, targets, config)
		finishTime := time.Now()
		delete(ctxState.Running, node.ModuleID)
		if err != nil {
			if errors.Is(err, taskstate.ErrTaskPaused) {
				ctxState.markStepFinished(node.ModuleID, taskstate.StatusPaused, nil, finishTime)
				return finalizeCombinationPause(db, combo, parentTask, node, ctxState, finishTime)
			}
			if errors.Is(err, scantask.ErrExitPending) {
				return err
			}
			status := "failed"
			if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
				status = "cancelled"
			}
			if saveErr := failCombinationModule(db, combo, parentTask, node, ctxState, status, err, finishTime); saveErr != nil {
				return saveErr
			}
			return err
		}

		ctxState.markStepFinished(node.ModuleID, "completed", nil, finishTime)
		if ctxState.Completed == nil {
			ctxState.Completed = make(map[string]bool)
		}
		ctxState.Completed[node.ModuleID] = true
		if _, syncErr := assetstore.SyncCompletedTask(ctx, db, taskID, parentTask.Version); syncErr != nil {
			logger.Warning("统一资产目录同步失败: task=%s version=%d err=%v", taskID, parentTask.Version, syncErr)
		}

		switch node.ModuleID {
		case "subdomain":
			results, _, deriveErr := subdomain.GetAllScanResults(taskID, 1000, "", "")
			if deriveErr != nil {
				return deriveErr
			}
			ctxState.applySubdomainResults(results, allNodes)
		case "portscan":
			webTargets, crackTargets, deriveErr := deriveTargetsFromPortScanResults(db, taskID)
			if deriveErr != nil {
				return deriveErr
			}
			ctxState.WebTargets = webTargets
			ctxState.WebTargetsReady = true
			ctxState.CrackTargets = crackTargets
			ctxState.CrackTargetsReady = true
		case "webscan", "dirscan", "jsscan":
			if len(ctxState.WebTargets) == 0 && len(targets) > 0 {
				ctxState.WebTargets = targets
			}
			ctxState.WebTargetsReady = true
		case "crack":
			if len(ctxState.CrackTargets) == 0 && len(targets) > 0 {
				ctxState.CrackTargets = targets
			}
			ctxState.CrackTargetsReady = true
		}
		return finalizeCombinationProgress(db, combo, parentTask, allNodes, node, ctxState, finishTime)
	}

	limiter := taskqueue.GetGlobalScanLimiter()
	if err := limiter.Acquire(ctx); err != nil {
		finishTime := time.Now()
		delete(ctxState.Running, node.ModuleID)
		if errors.Is(err, context.Canceled) {
			ctxState.markStepFinished(node.ModuleID, taskstate.StatusPaused, nil, finishTime)
			return finalizeCombinationPause(db, combo, parentTask, node, ctxState, finishTime)
		}
		ctxState.markStepFinished(node.ModuleID, "failed", err, finishTime)
		return err
	}
	defer limiter.Release()

	err := handler(ctx, taskID, targets, config)
	finishTime := time.Now()
	delete(ctxState.Running, node.ModuleID)

	if err != nil {
		if errors.Is(err, taskstate.ErrTaskPaused) {
			ctxState.markStepFinished(node.ModuleID, taskstate.StatusPaused, nil, finishTime)
			return finalizeCombinationPause(db, combo, parentTask, node, ctxState, finishTime)
		}
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			if shouldFinalizeCombinationPauseOnCancel(db, combo.ID, parentTask.ID, parentTask.Version) {
				ctxState.markStepFinished(node.ModuleID, taskstate.StatusPaused, nil, finishTime)
				return finalizeCombinationPause(db, combo, parentTask, node, ctxState, finishTime)
			}
		}
		status := "failed"
		if ctx.Err() == context.Canceled {
			status = "cancelled"
		}
		if saveErr := failCombinationModule(db, combo, parentTask, node, ctxState, status, err, finishTime); saveErr != nil {
			return saveErr
		}
		return err
	}

	ctxState.markStepFinished(node.ModuleID, "completed", nil, finishTime)
	if ctxState.Completed == nil {
		ctxState.Completed = make(map[string]bool)
	}
	ctxState.Completed[node.ModuleID] = true
	if _, syncErr := assetstore.SyncCompletedTask(ctx, db, taskID, parentTask.Version); syncErr != nil {
		logger.Warning("统一资产目录同步失败: task=%s version=%d err=%v", taskID, parentTask.Version, syncErr)
	}

	switch node.ModuleID {
	case "subdomain":
		results, _, deriveErr := subdomain.GetAllScanResults(taskID, 1000, "", "")
		if deriveErr != nil {
			return deriveErr
		}
		ctxState.applySubdomainResults(results, allNodes)
	case "portscan":
		webTargets, crackTargets, deriveErr := deriveTargetsFromPortScanResults(db, taskID)
		if deriveErr != nil {
			return deriveErr
		}
		ctxState.WebTargets = webTargets
		ctxState.WebTargetsReady = true
		ctxState.CrackTargets = crackTargets
		ctxState.CrackTargetsReady = true
	case "webscan", "dirscan", "jsscan":
		if len(ctxState.WebTargets) == 0 && len(targets) > 0 {
			ctxState.WebTargets = targets
		}
		ctxState.WebTargetsReady = true
	case "crack":
		if len(ctxState.CrackTargets) == 0 && len(targets) > 0 {
			ctxState.CrackTargets = targets
		}
		ctxState.CrackTargetsReady = true
	}
	return finalizeCombinationProgress(db, combo, parentTask, allNodes, node, ctxState, finishTime)
}

func failCombinationModule(db *gorm.DB, combo *database.CombinationTask, parentTask *database.VulnScanTask, node combinationPipelineNode, ctxState *combinationContext, status string, cause error, now time.Time) error {
	ctxState.markStepFinished(node.ModuleID, status, cause, now)
	return db.Transaction(func(tx *gorm.DB) error {
		var latest database.CombinationTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", combo.ID).First(&latest).Error; err != nil {
			return err
		}
		latestCtx, err := loadCombinationContext(&latest)
		if err != nil {
			return err
		}
		merged := mergeCombinationContext(latestCtx, ctxState)
		delete(merged.Running, node.ModuleID)
		if err := tx.Model(&database.CombinationTask{}).Where("id = ?", combo.ID).Updates(map[string]interface{}{
			"context": merged.serialize(), "status": status, "error_msg": cause.Error(), "updated_at": now,
		}).Error; err != nil {
			return err
		}
		*ctxState = *merged
		return tx.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).Updates(map[string]interface{}{
			"status": status, "scan_type": "combination", "error_msg": cause.Error(), "updated_at": now,
		}).Error
	})
}
