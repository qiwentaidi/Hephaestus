package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	mcpWorkbenchExecDefaultTimeout = 10
	mcpWorkbenchExecMaxTimeout     = 30
	mcpWorkbenchExecOutputLimit    = 20000
)

var (
	mcpWorkbenchExecAllowedCommands = map[string]bool{
		"curl":     true,
		"dig":      true,
		"echo":     true,
		"host":     true,
		"nslookup": true,
		"openssl":  true,
		"printf":   true,
	}
	mcpWorkbenchExecAllowedEnv = map[string]bool{
		"HTTP_PROXY":  true,
		"HTTPS_PROXY": true,
		"NO_PROXY":    true,
	}
	mcpWorkbenchExecURLPattern = regexp.MustCompile(`https?://[^\s'"]+`)
)

type mcpWorkbenchExecRunArgs struct {
	TaskID         string            `json:"taskId"`
	StepID         string            `json:"stepId"`
	Command        string            `json:"command"`
	Args           []string          `json:"args"`
	Env            map[string]string `json:"env"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
	RiskLevel      string            `json:"riskLevel"`
	ApprovalID     string            `json:"approvalId"`
}

type mcpWorkbenchExecRunResult struct {
	OK         bool              `json:"ok"`
	ExitCode   int               `json:"exitCode"`
	Stdout     string            `json:"stdout"`
	Stderr     string            `json:"stderr"`
	Error      string            `json:"error"`
	Command    string            `json:"command"`
	Args       []string          `json:"args"`
	WorkDir    string            `json:"workDir"`
	ResultPath string            `json:"resultPath"`
	DurationMS int64             `json:"durationMs"`
	Env        map[string]string `json:"env,omitempty"`
}

func mcpCallWorkbenchExecRun(args mcpWorkbenchExecRunArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	command := strings.TrimSpace(args.Command)
	if command == "" {
		return mcpToolCallResult{}, fmt.Errorf("command 不能为空")
	}
	if strings.ContainsAny(command, `/\`) || !mcpWorkbenchExecAllowedCommands[command] {
		return mcpToolCallResult{}, fmt.Errorf("command 不在允许列表: %s", command)
	}
	if err := ensureMCPWorkbenchExecArgsInScope(task, args.Args); err != nil {
		return mcpToolCallResult{}, err
	}
	riskLevel := normalizeMCPWorkbenchRiskLevel(args.RiskLevel)
	if riskLevel == "high" || riskLevel == "critical" {
		if err := ensureMCPWorkbenchApprovalApproved(db, task.ID, args.ApprovalID); err != nil {
			return mcpToolCallResult{}, err
		}
	}
	timeout := args.TimeoutSeconds
	if timeout <= 0 {
		timeout = mcpWorkbenchExecDefaultTimeout
	}
	if timeout > mcpWorkbenchExecMaxTimeout {
		timeout = mcpWorkbenchExecMaxTimeout
	}
	commandPath, err := exec.LookPath(command)
	if err != nil {
		return mcpToolCallResult{}, fmt.Errorf("未找到命令 %s: %w", command, err)
	}
	executionID := database.GenerateNanoid()
	workDir := filepath.Join(config.GetDataDir(), "ai-workbench", "tasks", sanitizeMCPWorkbenchPathSegment(task.ID), "exec", executionID)
	if err := os.MkdirAll(workDir, 0750); err != nil {
		return mcpToolCallResult{}, fmt.Errorf("创建 exec 工作目录失败: %w", err)
	}
	toolCall := database.AIWorkbenchToolCall{
		ID:          database.GenerateNanoid(),
		TaskID:      task.ID,
		StepID:      strings.TrimSpace(args.StepID),
		ExecutionID: executionID,
		ToolName:    "exec_run",
		InputJSON: marshalMCPWorkbenchJSON(map[string]any{
			"command":        command,
			"args":           redactMCPWorkbenchExecArgs(args.Args),
			"env":            sanitizeMCPWorkbenchExecEnv(args.Env),
			"timeoutSeconds": timeout,
			"riskLevel":      riskLevel,
			"approvalId":     strings.TrimSpace(args.ApprovalID),
		}),
		Status: "running",
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&toolCall).Error; err != nil {
			return err
		}
		return createMCPWorkbenchToolExecution(tx, database.AIWorkbenchToolExecution{
			ID:           executionID,
			TaskID:       task.ID,
			StepID:       toolCall.StepID,
			ToolCallID:   toolCall.ID,
			ToolName:     "exec.run",
			Runtime:      aipkg.WorkbenchToolRuntimeLocalMCP,
			InputSummary: strings.Join(append([]string{command}, redactMCPWorkbenchExecArgs(args.Args)...), " "),
			Arguments: map[string]any{
				"command":        command,
				"args":           redactMCPWorkbenchExecArgs(args.Args),
				"env":            sanitizeMCPWorkbenchExecEnv(args.Env),
				"timeoutSeconds": timeout,
				"riskLevel":      riskLevel,
			},
			Status: aipkg.WorkbenchToolExecutionStatusRunning,
			Metadata: map[string]any{
				"toolCallId": toolCall.ID,
				"workDir":    workDir,
			},
		})
	}); err != nil {
		return mcpToolCallResult{}, err
	}
	result := runMCPWorkbenchExec(commandPath, command, args.Args, sanitizeMCPWorkbenchExecEnv(args.Env), workDir, timeout)
	result.ResultPath = filepath.Join(workDir, "result.json")
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	_ = os.WriteFile(result.ResultPath, resultJSON, 0600)

	status := aipkg.WorkbenchToolExecutionStatusCompleted
	toolCall.Status = "completed"
	if !result.OK {
		status = aipkg.WorkbenchToolExecutionStatusFailed
		if strings.Contains(result.Error, "context deadline exceeded") || strings.Contains(result.Error, "执行超时") {
			status = aipkg.WorkbenchToolExecutionStatusHardTimeout
		}
		toolCall.Status = "failed"
		toolCall.ErrorMessage = defaultString(result.Error, result.Stderr)
	}
	summary := buildMCPWorkbenchExecSummary(result)
	if err := db.Transaction(func(tx *gorm.DB) error {
		toolCall.OutputSummary = summary
		toolCall.RawOutputRef = result.ResultPath
		if err := tx.Save(&toolCall).Error; err != nil {
			return err
		}
		if err := finishMCPWorkbenchToolExecution(tx, executionID, status, summary, result.ResultPath, toolCall.ErrorMessage, map[string]any{
			"toolCallId": toolCall.ID,
			"workDir":    workDir,
			"resultPath": result.ResultPath,
			"exitCode":   result.ExitCode,
		}); err != nil {
			return err
		}
		eventType := "exec.completed"
		level := "info"
		title := "受控命令执行完成"
		if toolCall.Status == "failed" {
			eventType = "exec.failed"
			level = "error"
			title = "受控命令执行失败"
		}
		return createMCPWorkbenchEvent(tx, task.ID, toolCall.StepID, eventType, level, title, summary, gin.H{
			"toolCallId":  toolCall.ID,
			"executionId": executionID,
			"resultPath":  result.ResultPath,
			"exitCode":    result.ExitCode,
		})
	}); err != nil {
		return mcpToolCallResult{}, err
	}
	if !result.OK {
		return mcpToolCallResult{}, errors.New(defaultString(result.Error, result.Stderr))
	}
	return newMCPToolResult(gin.H{
		"taskId":      task.ID,
		"executionId": executionID,
		"toolCallId":  toolCall.ID,
		"workDir":     workDir,
		"resultPath":  result.ResultPath,
		"exitCode":    result.ExitCode,
		"stdout":      result.Stdout,
		"stderr":      result.Stderr,
		"durationMs":  result.DurationMS,
	}), nil
}

func runMCPWorkbenchExec(commandPath string, command string, args []string, env map[string]string, workDir string, timeoutSeconds int) mcpWorkbenchExecRunResult {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, commandPath, args...)
	configureMCPWorkbenchCommand(cmd)
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started).Milliseconds()
	result := mcpWorkbenchExecRunResult{
		OK:         err == nil,
		ExitCode:   0,
		Stdout:     truncateMCPWorkbenchText(stdout.String(), mcpWorkbenchExecOutputLimit),
		Stderr:     truncateMCPWorkbenchText(stderr.String(), mcpWorkbenchExecOutputLimit),
		Command:    command,
		Args:       redactMCPWorkbenchExecArgs(args),
		WorkDir:    workDir,
		DurationMS: duration,
		Env:        env,
	}
	if err != nil {
		result.Error = err.Error()
		result.ExitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = "exec 执行超时"
		}
	}
	return result
}

func ensureMCPWorkbenchExecArgsInScope(task database.AIWorkbenchTask, args []string) error {
	scope := append([]string{task.Target}, task.Scope...)
	for _, arg := range args {
		for _, rawURL := range mcpWorkbenchExecURLPattern.FindAllString(arg, -1) {
			inScope, reason := isMCPWorkbenchTargetInScope(rawURL, scope)
			if !inScope {
				return fmt.Errorf("参数 URL 不在授权范围内: %s", reason)
			}
		}
	}
	return nil
}

func sanitizeMCPWorkbenchExecEnv(env map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range env {
		key = strings.ToUpper(strings.TrimSpace(key))
		if !mcpWorkbenchExecAllowedEnv[key] {
			continue
		}
		output[key] = value
	}
	return output
}

func redactMCPWorkbenchExecArgs(args []string) []string {
	output := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		lower := strings.ToLower(arg)
		output = append(output, arg)
		if lower == "-h" || lower == "--header" || lower == "-b" || lower == "--cookie" || lower == "--data" || lower == "--data-raw" || lower == "-d" {
			if index+1 < len(args) {
				index++
				output = append(output, "***")
			}
		}
	}
	return output
}

func buildMCPWorkbenchExecSummary(result mcpWorkbenchExecRunResult) string {
	if !result.OK {
		return fmt.Sprintf("exec 执行失败: exit=%d，error=%s", result.ExitCode, defaultString(result.Error, singleLineMCPWorkbenchText(result.Stderr)))
	}
	preview := singleLineMCPWorkbenchText(result.Stdout)
	if preview == "" {
		preview = singleLineMCPWorkbenchText(result.Stderr)
	}
	return fmt.Sprintf("exec 执行完成: exit=%d，耗时 %dms，输出=%s", result.ExitCode, result.DurationMS, truncateMCPWorkbenchText(preview, 240))
}
