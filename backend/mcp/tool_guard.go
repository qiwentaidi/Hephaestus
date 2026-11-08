package mcp

import (
	"encoding/json"
	"strings"
	"time"

	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/toolguard"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func recordMCPWorkbenchBlockedToolCall(toolName string, rawArgs json.RawMessage, match *toolguard.Match) {
	if match == nil {
		return
	}
	var arguments map[string]any
	if err := json.Unmarshal(rawArgs, &arguments); err != nil {
		return
	}
	taskID := strings.TrimSpace(toolGuardStringValue(arguments["taskId"]))
	if taskID == "" {
		return
	}
	db, task, err := loadMCPWorkbenchTask(taskID)
	if err != nil {
		logger.Warning("记录被拦截的 AI 智脑工具调用失败: %v", err)
		return
	}
	now := time.Now()
	toolCallID := database.GenerateNanoid()
	executionID := database.GenerateNanoid()
	stepID := strings.TrimSpace(toolGuardStringValue(arguments["stepId"]))
	message := toolguard.FormatBlockedMessage(match)
	toolCall := database.AIWorkbenchToolCall{
		ID:            toolCallID,
		TaskID:        task.ID,
		StepID:        stepID,
		ExecutionID:   executionID,
		ToolName:      toolName,
		InputJSON:     marshalMCPWorkbenchJSON(arguments),
		OutputSummary: match.Message,
		Status:        aipkg.WorkbenchToolExecutionStatusBlocked,
		ErrorMessage:  message,
	}
	execution := database.AIWorkbenchToolExecution{
		ID:            executionID,
		TaskID:        task.ID,
		StepID:        stepID,
		ToolCallID:    toolCallID,
		ToolName:      toolName,
		Runtime:       aipkg.WorkbenchToolRuntimeLocalMCP,
		InputSummary:  "工具调用在执行前被安全规则拦截",
		Arguments:     arguments,
		Status:        aipkg.WorkbenchToolExecutionStatusBlocked,
		OutputSummary: match.Message,
		ErrorMessage:  message,
		StartedAt:     &now,
		CompletedAt:   &now,
		Metadata: map[string]any{
			"blocked":     true,
			"ruleId":      match.RuleID,
			"ruleName":    match.RuleName,
			"matchedText": match.MatchedText,
		},
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&toolCall).Error; err != nil {
			return err
		}
		if err := createMCPWorkbenchToolExecution(tx, execution); err != nil {
			return err
		}
		return createMCPWorkbenchEvent(tx, task.ID, stepID, "tool.execution.blocked", "warning", "工具调用已拦截", match.Message, gin.H{
			"toolCallId":  toolCallID,
			"executionId": executionID,
			"toolName":    toolName,
			"ruleId":      match.RuleID,
			"ruleName":    match.RuleName,
			"matchedText": match.MatchedText,
		})
	}); err != nil {
		logger.Warning("记录被拦截的 AI 智脑工具调用失败: %v", err)
	}
}

func toolGuardStringValue(value any) string {
	text, _ := value.(string)
	return text
}
