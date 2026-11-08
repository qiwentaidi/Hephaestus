package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/taskstate"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

type combinationEvent struct {
	CombinationID string
	NodeIndex     int
}

var (
	combinationQueueOnce   sync.Once
	combinationStopOnce    sync.Once
	combinationEvents      chan combinationEvent
	combinationStopCh      chan struct{}
	combinationWG          sync.WaitGroup
	maxCombinationParallel = 3
)

// StartCombinationScheduler 启动组合任务调度器（单进程 worker 池）
func StartCombinationScheduler(workerCount int) {
	if workerCount <= 0 {
		workerCount = 2
	}
	maxCombinationParallel = workerCount

	combinationQueueOnce.Do(func() {
		combinationEvents = make(chan combinationEvent, 128)
		combinationStopCh = make(chan struct{})
		for i := 0; i < workerCount; i++ {
			combinationWG.Add(1)
			go func(id int) {
				defer combinationWG.Done()
				for {
					select {
					case <-combinationStopCh:
						return
					case ev, ok := <-combinationEvents:
						if !ok {
							return
						}
						if err := handleCombinationEvent(ev); err != nil {
							logger.Warning("组合任务事件处理失败: %v", err)
						}
					}
				}
			}(i + 1)
		}
	})
}

// StopCombinationScheduler 停止调度器
func StopCombinationScheduler(ctx context.Context) {
	combinationStopOnce.Do(func() {
		if combinationEvents == nil {
			return
		}
		close(combinationStopCh)
		close(combinationEvents)

		done := make(chan struct{})
		go func() {
			combinationWG.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
		}
	})
}

func enqueueCombinationStep(ev combinationEvent) {
	if combinationEvents == nil {
		logger.Warning("组合任务调度器未启动，无法入队: %+v", ev)
		return
	}
	select {
	case combinationEvents <- ev:
	default:
		go func() {
			combinationEvents <- ev
		}()
	}
}

func handleCombinationEvent(ev combinationEvent) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var combo database.CombinationTask
	if err := db.Where("id = ?", ev.CombinationID).First(&combo).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return fmt.Errorf("查询组合任务失败: %w", err)
	}

	nodes, err := parseCombinationNodes(combo.Nodes)
	if err != nil {
		markCombinationFailed(db, combo.ID, err)
		return err
	}
	if ev.NodeIndex >= len(nodes) {
		return nil
	}

	ctxState, err := loadCombinationContext(&combo)
	if err != nil {
		markCombinationFailed(db, combo.ID, err)
		return err
	}

	node := nodes[ev.NodeIndex]
	if combo.Status == taskstate.StatusPaused || combo.PauseRequested {
		if step := ctxState.stepLog(node.ModuleID); step != nil && step.Status == "running" {
			// A duplicate queue event cannot acknowledge an active worker's exit.
			return nil
		}
		now := time.Now()
		if ctxState.Running != nil {
			delete(ctxState.Running, node.ModuleID)
		}
		ctxState.markStepFinished(node.ModuleID, taskstate.StatusPaused, nil, now)
		if parentTask, parentErr := getCombinationParentTask(db, combo.ID); parentErr == nil {
			return finalizeCombinationPause(db, &combo, parentTask, node, ctxState, now)
		} else {
			return parentErr
		}
	}
	if ctxState.Completed[node.ModuleID] {
		_, schedErr := scheduleNextCombinationNodes(db, &combo, nodes, ctxState)
		return schedErr
	}
	if step := ctxState.stepLog(node.ModuleID); step != nil && step.Status != "queued" {
		return nil
	}
	targets, err := ctxState.targetsFor(node.ModuleID)
	if err != nil {
		markCombinationFailed(db, combo.ID, err)
		return err
	}

	parentTask, err := getCombinationParentTask(db, combo.ID)
	if err != nil {
		markCombinationFailed(db, combo.ID, err)
		return err
	}

	ctxState.ensureStepLogs(nodes, parentTask.ID)

	// 无可用目标时，跳过该模块，不再将整个组合任务置为失败。
	if len(targets) == 0 {
		now := time.Now()
		ctxState.markStepFinished(node.ModuleID, "skipped", fmt.Errorf("无可用目标，已跳过"), now)
		if ctxState.Completed == nil {
			ctxState.Completed = make(map[string]bool)
		}
		ctxState.Completed[node.ModuleID] = true
		if ctxState.Running != nil {
			delete(ctxState.Running, node.ModuleID)
		}

		if err := finalizeCombinationProgress(db, &combo, parentTask, nodes, node, ctxState, now); err != nil {
			markCombinationFailed(db, combo.ID, err)
			return err
		}
		_, schedErr := scheduleNextCombinationNodes(db, &combo, nodes, ctxState)
		return schedErr
	}

	if err := runCombinationModule(db, &combo, parentTask, nodes, node, ev.NodeIndex, targets, ctxState); err != nil {
		markCombinationFailed(db, combo.ID, err)
		return err
	}

	_, schedErr := scheduleNextCombinationNodes(db, &combo, nodes, ctxState)
	return schedErr
}

func getCombinationParentTask(db *gorm.DB, combinationID string) (*database.VulnScanTask, error) {
	var task database.VulnScanTask
	if err := db.Where("parent_combination_id = ?", combinationID).
		Order("version DESC").
		First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("组合任务 %s 缺少关联的任务记录", combinationID)
		}
		return nil, fmt.Errorf("查询组合任务关联任务失败: %w", err)
	}
	return &task, nil
}

func shouldFinalizeCombinationPauseOnCancel(db *gorm.DB, comboID string, taskID string, version int) bool {
	if db == nil {
		return false
	}

	var comboState struct {
		Status         string
		PauseRequested bool
	}
	if err := db.Model(&database.CombinationTask{}).
		Select("status", "pause_requested").
		Where("id = ?", comboID).
		First(&comboState).Error; err == nil {
		if comboState.Status == taskstate.StatusPaused || comboState.PauseRequested {
			return true
		}
	}

	var taskState struct {
		Status         string
		PauseRequested bool
	}
	if err := db.Model(&database.VulnScanTask{}).
		Select("status", "pause_requested").
		Where("id = ? AND version = ?", taskID, version).
		First(&taskState).Error; err == nil {
		if taskState.Status == taskstate.StatusPaused || taskState.PauseRequested {
			return true
		}
	}

	return false
}

func getCombinationByParentTask(db *gorm.DB, task *database.VulnScanTask) (*database.CombinationTask, []combinationPipelineNode, *combinationContext, error) {
	if task == nil {
		return nil, nil, nil, fmt.Errorf("组合任务父记录缺失")
	}
	if strings.TrimSpace(task.ParentCombinationID) == "" {
		return nil, nil, nil, fmt.Errorf("任务未关联组合任务")
	}

	var combo database.CombinationTask
	if err := db.Where("id = ?", task.ParentCombinationID).First(&combo).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, nil, fmt.Errorf("组合任务不存在")
		}
		return nil, nil, nil, fmt.Errorf("查询组合任务失败: %w", err)
	}

	nodes, err := parseCombinationNodes(combo.Nodes)
	if err != nil {
		return nil, nil, nil, err
	}
	ctxState, err := loadCombinationContext(&combo)
	if err != nil {
		return nil, nil, nil, err
	}
	ctxState.ensureStepLogs(nodes, task.ID)
	return &combo, nodes, ctxState, nil
}

func rebuildCombinationContext(combo *database.CombinationTask, nodes []combinationPipelineNode, taskID string) (*combinationContext, error) {
	var targets []string
	if err := json.Unmarshal([]byte(combo.Targets), &targets); err != nil {
		return nil, fmt.Errorf("解析组合任务目标失败: %w", err)
	}
	ctxState := newCombinationContext(targets)
	ctxState.ensureStepLogs(nodes, taskID)
	return ctxState, nil
}

func createNextCombinationTaskVersion(db *gorm.DB, baseTask *database.VulnScanTask) (*database.VulnScanTask, error) {
	var maxVersion int
	if err := db.Raw(`
		SELECT COALESCE(MAX(version), 0)
		FROM vul_scan_task
		WHERE id = ?
	`, baseTask.ID).Scan(&maxVersion).Error; err != nil {
		return nil, fmt.Errorf("获取组合任务最大版本号失败: %w", err)
	}

	newTask := &database.VulnScanTask{
		ID:                  baseTask.ID,
		TaskName:            baseTask.TaskName,
		CreatedByAI:         baseTask.CreatedByAI,
		ScanMode:            baseTask.ScanMode,
		ScanType:            "combination",
		AssignedAgentID:     baseTask.AssignedAgentID,
		WorkerAgentID:       "",
		LeaseExpiresAt:      nil,
		ParentCombinationID: baseTask.ParentCombinationID,
		Targets:             baseTask.Targets,
		Status:              taskstate.StatusPending,
		Progress:            0,
		TotalCount:          baseTask.TotalCount,
		Version:             maxVersion + 1,
		Config:              baseTask.Config,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	if err := db.Create(newTask).Error; err != nil {
		return nil, fmt.Errorf("创建组合任务新版本失败: %w", err)
	}
	return newTask, nil
}
