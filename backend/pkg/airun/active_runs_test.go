package airun

import (
	"context"
	"testing"
)

func TestActiveRunLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	run := &ActiveRun{Cancel: cancel, Done: make(chan struct{})}

	if !RegisterActiveRun("conv-1", run) {
		t.Fatal("first registration should succeed")
	}
	if RegisterActiveRun("conv-1", &ActiveRun{}) {
		t.Fatal("duplicate registration must fail")
	}
	if !HasActiveRun("conv-1") {
		t.Fatal("run should be visible")
	}
	loaded, ok := LoadActiveRun("conv-1")
	if !ok || loaded != run {
		t.Fatal("LoadActiveRun should return the registered handle")
	}

	if !StopActiveRun("conv-1") {
		t.Fatal("StopActiveRun should cancel the active run")
	}
	if HasActiveRun("conv-1") {
		t.Fatal("run should be removed after stop")
	}
	if ctx.Err() == nil {
		t.Fatal("cancel function should have been called")
	}
}

func TestRemoveActiveRunIfOnlyRemovesMatchingHandle(t *testing.T) {
	first := &ActiveRun{}
	second := &ActiveRun{}
	RegisterActiveRun("conv-2", first)
	t.Cleanup(func() { RemoveActiveRunIf("conv-2", second) })

	RemoveActiveRunIf("conv-2", second)
	if !HasActiveRun("conv-2") {
		t.Fatal("a different handle must not remove the current run")
	}
	RemoveActiveRunIf("conv-2", first)
	if HasActiveRun("conv-2") {
		t.Fatal("matching handle should be removed")
	}
}

func TestNormalizeConversationStatus(t *testing.T) {
	for in, want := range map[string]string{
		"active":    "running",
		"RUNNING":   "running",
		"completed": "completed",
		"weird":     "running",
		"":          "running",
	} {
		if got := NormalizeConversationStatus(in); got != want {
			t.Fatalf("NormalizeConversationStatus(%q)=%q want %q", in, got, want)
		}
	}
}
