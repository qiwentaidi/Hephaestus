package ai

import (
	"context"
	"testing"
)

type stubWorkbenchToolRuntime struct{}

func (stubWorkbenchToolRuntime) RuntimeName() string {
	return "stub"
}

func (stubWorkbenchToolRuntime) Submit(ctx context.Context, request WorkbenchToolExecutionRequest) (WorkbenchToolExecutionResult, error) {
	_ = ctx
	return WorkbenchToolExecutionResult{
		ExecutionID:   request.ExecutionID,
		Runtime:       request.Runtime,
		Status:        WorkbenchToolExecutionStatusCompleted,
		OutputSummary: "done",
	}, nil
}

func TestWorkbenchToolRuntimeRegistrySubmit(t *testing.T) {
	registry := NewWorkbenchToolRuntimeRegistry()
	registry.Register(stubWorkbenchToolRuntime{})

	result, err := registry.Submit(context.Background(), "stub", WorkbenchToolExecutionRequest{
		ExecutionID: "exec-1",
		ToolName:    "tool.test",
	})
	if err != nil {
		t.Fatalf("submit runtime: %v", err)
	}
	if result.Status != WorkbenchToolExecutionStatusCompleted {
		t.Fatalf("expected completed result, got %q", result.Status)
	}
	if result.Runtime != "stub" {
		t.Fatalf("expected runtime to be preserved, got %q", result.Runtime)
	}
}

func TestWorkbenchToolRuntimeRegistryRejectsMissingRuntime(t *testing.T) {
	registry := NewWorkbenchToolRuntimeRegistry()

	result, err := registry.Submit(context.Background(), "missing", WorkbenchToolExecutionRequest{
		ExecutionID: "exec-1",
	})
	if err == nil {
		t.Fatal("expected missing runtime error")
	}
	if result.Status != WorkbenchToolExecutionStatusFailed {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
}
