package mcp

import (
	"errors"
	"strings"
	"time"

	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/taskstate"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	mcpWorkbenchExecutionDefaultLimit  = 20
	mcpWorkbenchExecutionMaxLimit      = 100
	mcpWorkbenchExecutionDefaultWaitMS = 5000
	mcpWorkbenchExecutionMaxWaitMS     = 30000
)

type mcpWorkbenchExecutionListArgs struct {
	TaskID   string `json:"taskId"`
	Status   string `json:"status"`
	ToolName string `json:"toolName"`
	Limit    int    `json:"limit"`
}

type mcpWorkbenchExecutionGetArgs struct {
	TaskID      string `json:"taskId"`
	ExecutionID string `json:"executionId"`
}

type mcpWorkbenchExecutionWaitArgs struct {
	TaskID      string `json:"taskId"`
	ExecutionID string `json:"executionId"`
	TimeoutMS   int    `json:"timeoutMs"`
}

type mcpWorkbenchExecutionCancelArgs struct {
	TaskID      string `json:"taskId"`
	ExecutionID string `json:"executionId"`
}

func mcpCallWorkbenchExecutionList(args mcpWorkbenchExecutionListArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	limit := args.Limit
	if limit <= 0 {
		limit = mcpWorkbenchExecutionDefaultLimit
	}
	if limit > mcpWorkbenchExecutionMaxLimit {
		limit = mcpWorkbenchExecutionMaxLimit
	}
	query := db.Where("task_id = ?", task.ID)
	if status := strings.TrimSpace(args.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if toolName := strings.TrimSpace(args.ToolName); toolName != "" {
		query = query.Where("tool_name = ?", toolName)
	}
	var executions []database.AIWorkbenchToolExecution
	if err := query.Order("created_at DESC").Limit(limit).Find(&executions).Error; err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":     task.ID,
		"executions": executions,
		"count":      len(executions),
		"limit":      limit,
	}), nil
}

func mcpCallWorkbenchExecutionGet(args mcpWorkbenchExecutionGetArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	execution, err := loadMCPWorkbenchToolExecution(db, task.ID, args.ExecutionID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":    task.ID,
		"execution": execution,
	}), nil
}

func mcpCallWorkbenchExecutionWait(args mcpWorkbenchExecutionWaitArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	timeoutMS := args.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = mcpWorkbenchExecutionDefaultWaitMS
	}
	if timeoutMS > mcpWorkbenchExecutionMaxWaitMS {
		timeoutMS = mcpWorkbenchExecutionMaxWaitMS
	}
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	var execution database.AIWorkbenchToolExecution
	for {
		execution, err = loadMCPWorkbenchToolExecution(db, task.ID, args.ExecutionID)
		if err != nil {
			return mcpToolCallResult{}, err
		}
		if mcpWorkbenchToolExecutionStatusIsTerminal(execution.Status) || time.Now().After(deadline) {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	timedOut := !mcpWorkbenchToolExecutionStatusIsTerminal(execution.Status)
	return newMCPToolResult(gin.H{
		"taskId":    task.ID,
		"execution": execution,
		"timedOut":  timedOut,
	}), nil
}

func mcpCallWorkbenchExecutionCancel(args mcpWorkbenchExecutionCancelArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	execution, err := loadMCPWorkbenchToolExecution(db, task.ID, args.ExecutionID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	if !mcpWorkbenchToolExecutionStatusIsTerminal(execution.Status) {
		if err := cancelMCPWorkbenchToolExecution(db, task.ID, execution); err != nil {
			return mcpToolCallResult{}, err
		}
		execution, err = loadMCPWorkbenchToolExecution(db, task.ID, args.ExecutionID)
		if err != nil {
			return mcpToolCallResult{}, err
		}
	}
	return newMCPToolResult(gin.H{
		"taskId":    task.ID,
		"execution": execution,
		"cancelled": execution.Status == aipkg.WorkbenchToolExecutionStatusCancelled,
	}), nil
}

func createMCPWorkbenchToolExecution(db *gorm.DB, execution database.AIWorkbenchToolExecution) error {
	if db == nil || !db.Migrator().HasTable(&database.AIWorkbenchToolExecution{}) {
		return nil
	}
	if strings.TrimSpace(execution.ID) == "" {
		return nil
	}
	if strings.TrimSpace(execution.Runtime) == "" {
		execution.Runtime = aipkg.WorkbenchToolRuntimeLocalMCP
	}
	if strings.TrimSpace(execution.Status) == "" {
		execution.Status = aipkg.WorkbenchToolExecutionStatusRunning
	}
	now := time.Now()
	if execution.StartedAt == nil {
		execution.StartedAt = &now
	}
	return db.Create(&execution).Error
}

func finishMCPWorkbenchToolExecution(db *gorm.DB, executionID string, status string, outputSummary string, resultRef string, errorMessage string, metadata map[string]any) error {
	if db == nil || strings.TrimSpace(executionID) == "" || !db.Migrator().HasTable(&database.AIWorkbenchToolExecution{}) {
		return nil
	}
	var execution database.AIWorkbenchToolExecution
	if err := db.Where("id = ?", executionID).First(&execution).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = aipkg.WorkbenchToolExecutionStatusCompleted
	}
	finishedAt := time.Now()
	execution.Status = status
	execution.OutputSummary = strings.TrimSpace(outputSummary)
	execution.ResultRef = strings.TrimSpace(resultRef)
	execution.ErrorMessage = strings.TrimSpace(errorMessage)
	execution.Metadata = metadata
	if mcpWorkbenchToolExecutionStatusIsTerminal(status) {
		execution.CompletedAt = &finishedAt
		if execution.StartedAt != nil {
			execution.DurationMS = finishedAt.Sub(*execution.StartedAt).Milliseconds()
		}
	}
	return db.Save(&execution).Error
}

func loadMCPWorkbenchToolExecution(db *gorm.DB, taskID string, executionID string) (database.AIWorkbenchToolExecution, error) {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return database.AIWorkbenchToolExecution{}, errors.New("executionId 不能为空")
	}
	var execution database.AIWorkbenchToolExecution
	if err := db.Where("task_id = ? AND id = ?", taskID, executionID).First(&execution).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.AIWorkbenchToolExecution{}, errors.New("工具执行不存在")
		}
		return database.AIWorkbenchToolExecution{}, err
	}
	return execution, nil
}

func cancelMCPWorkbenchToolExecution(db *gorm.DB, taskID string, execution database.AIWorkbenchToolExecution) error {
	now := time.Now()
	if scanTaskID := linkedMCPWorkbenchScanTaskIDFromRef(execution.ResultRef); scanTaskID != "" {
		if err := cancelMCPWorkbenchLinkedScanByID(db, taskID, scanTaskID, now); err != nil {
			return err
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.AIWorkbenchToolExecution{}).
			Where("task_id = ? AND id = ?", taskID, execution.ID).
			Updates(map[string]any{
				"status":         aipkg.WorkbenchToolExecutionStatusCancelled,
				"output_summary": "工具执行已取消。",
				"completed_at":   &now,
				"updated_at":     now,
			}).Error; err != nil {
			return err
		}
		if execution.ToolCallID != "" {
			if err := tx.Model(&database.AIWorkbenchToolCall{}).
				Where("task_id = ? AND id = ?", taskID, execution.ToolCallID).
				Updates(map[string]any{
					"status":         "cancelled",
					"output_summary": "工具执行已取消。",
					"updated_at":     now,
				}).Error; err != nil {
				return err
			}
		}
		return createMCPWorkbenchEvent(tx, taskID, execution.StepID, "tool.execution.cancelled", "warning", "工具执行已取消", execution.ToolName, gin.H{
			"executionId": execution.ID,
			"toolCallId":  execution.ToolCallID,
			"toolName":    execution.ToolName,
			"resultRef":   execution.ResultRef,
		})
	})
}

func cancelMCPWorkbenchLinkedScanByID(db *gorm.DB, taskID string, scanTaskID string, now time.Time) error {
	var scanTask database.VulnScanTask
	err := db.Where("id = ?", strings.TrimSpace(scanTaskID)).Order("version DESC").First(&scanTask).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if isMCPWorkbenchTerminalScanStatus(scanTask.Status) {
		return nil
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.VulnScanTaskItem{}).
			Where("task_id = ? AND status IN ?", scanTask.ID, []string{taskstate.StatusPending, taskstate.StatusRunning}).
			Updates(map[string]any{
				"status":      taskstate.StatusCancelled,
				"finished_at": &now,
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
		return taskstate.MarkCancelled(tx, scanTask.ID, scanTask.Version, "AI 智脑 MCP 工具执行已取消，关联扫描同步取消")
	}); err != nil {
		return err
	}
	_ = control.GetTaskController().CancelTask(scanTask.ID)
	return db.Model(&database.AIWorkbenchToolCall{}).
		Where("task_id = ? AND raw_output_ref = ? AND status = ?", taskID, "webscan:"+scanTask.ID, "running").
		Updates(map[string]any{
			"status":         "cancelled",
			"output_summary": "工具执行已取消，关联扫描任务已同步取消。",
			"updated_at":     now,
		}).Error
}

func linkedMCPWorkbenchScanTaskIDFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "webscan:") {
		return strings.TrimSpace(strings.TrimPrefix(ref, "webscan:"))
	}
	return ""
}

func isMCPWorkbenchTerminalScanStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case taskstate.StatusCompleted, taskstate.StatusFailed, taskstate.StatusCancelled:
		return true
	default:
		return false
	}
}

func mcpWorkbenchToolExecutionStatusIsTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case aipkg.WorkbenchToolExecutionStatusCompleted,
		aipkg.WorkbenchToolExecutionStatusFailed,
		aipkg.WorkbenchToolExecutionStatusCancelled,
		aipkg.WorkbenchToolExecutionStatusSkipped,
		aipkg.WorkbenchToolExecutionStatusHardTimeout,
		aipkg.WorkbenchToolExecutionStatusBlocked:
		return true
	default:
		return false
	}
}
