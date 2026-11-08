package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

const (
	WorkbenchToolRuntimeInternal = "internal"
	WorkbenchToolRuntimeLocalMCP = "local_mcp"

	WorkbenchToolExecutionStatusQueued      = "queued"
	WorkbenchToolExecutionStatusRunning     = "running"
	WorkbenchToolExecutionStatusCompleted   = "completed"
	WorkbenchToolExecutionStatusFailed      = "failed"
	WorkbenchToolExecutionStatusCancelled   = "cancelled"
	WorkbenchToolExecutionStatusSkipped     = "skipped"
	WorkbenchToolExecutionStatusWaitTimeout = "wait_timeout"
	WorkbenchToolExecutionStatusHardTimeout = "hard_timeout"
	WorkbenchToolExecutionStatusBlocked     = "blocked"
)

// WorkbenchToolExecutionRequest is the runtime-neutral command envelope used by
// AI workbench orchestration. Internal Hephaestus tools and later MCP-backed
// tools must both enter through this shape.
type WorkbenchToolExecutionRequest struct {
	ExecutionID  string         `json:"executionId"`
	TaskID       string         `json:"taskId"`
	StepID       string         `json:"stepId"`
	AgentRunID   string         `json:"agentRunId"`
	ToolCallID   string         `json:"toolCallId"`
	ToolName     string         `json:"toolName"`
	Runtime      string         `json:"runtime"`
	InputSummary string         `json:"inputSummary"`
	Arguments    map[string]any `json:"arguments"`
}

// WorkbenchToolExecutionResult is the normalized execution result returned by a
// tool runtime. ResultRef points to the external raw output, such as webscan:id.
type WorkbenchToolExecutionResult struct {
	ExecutionID   string         `json:"executionId"`
	Runtime       string         `json:"runtime"`
	Status        string         `json:"status"`
	OutputSummary string         `json:"outputSummary"`
	ResultRef     string         `json:"resultRef,omitempty"`
	ErrorMessage  string         `json:"errorMessage,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type WorkbenchToolRuntime interface {
	RuntimeName() string
	Submit(ctx context.Context, request WorkbenchToolExecutionRequest) (WorkbenchToolExecutionResult, error)
}

type WorkbenchToolRuntimeRegistry struct {
	mu       sync.RWMutex
	runtimes map[string]WorkbenchToolRuntime
}

func NewWorkbenchToolRuntimeRegistry() *WorkbenchToolRuntimeRegistry {
	return &WorkbenchToolRuntimeRegistry{runtimes: make(map[string]WorkbenchToolRuntime)}
}

func (r *WorkbenchToolRuntimeRegistry) Register(runtime WorkbenchToolRuntime) {
	if runtime == nil {
		return
	}
	name := strings.TrimSpace(runtime.RuntimeName())
	if name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runtimes[name] = runtime
}

func (r *WorkbenchToolRuntimeRegistry) Submit(ctx context.Context, runtimeName string, request WorkbenchToolExecutionRequest) (WorkbenchToolExecutionResult, error) {
	runtimeName = strings.TrimSpace(runtimeName)
	if runtimeName == "" {
		runtimeName = WorkbenchToolRuntimeInternal
	}
	r.mu.RLock()
	runtime := r.runtimes[runtimeName]
	r.mu.RUnlock()
	if runtime == nil {
		return WorkbenchToolExecutionResult{
			ExecutionID:   request.ExecutionID,
			Runtime:       runtimeName,
			Status:        WorkbenchToolExecutionStatusFailed,
			ErrorMessage:  fmt.Sprintf("tool runtime %q is not registered", runtimeName),
			OutputSummary: fmt.Sprintf("工具运行时 %q 未注册。", runtimeName),
		}, fmt.Errorf("tool runtime %q is not registered", runtimeName)
	}
	request.Runtime = runtimeName
	result, err := runtime.Submit(ctx, request)
	if strings.TrimSpace(result.ExecutionID) == "" {
		result.ExecutionID = request.ExecutionID
	}
	if strings.TrimSpace(result.Runtime) == "" {
		result.Runtime = runtimeName
	}
	return result, err
}
