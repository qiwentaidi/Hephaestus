package ai

import (
	"testing"

	"hephaestus-backend/lib/pentest"
)

func TestOrchestrationRunTargetUsesFirstScopeValue(t *testing.T) {
	run := pentest.PlanRun{ScopeJSON: `["https://example.test/path","10.0.0.1"]`}
	if got := orchestrationRunTarget(run); got != "https://example.test/path" {
		t.Fatalf("target = %q", got)
	}
	if got := orchestrationRunTarget(pentest.PlanRun{ScopeJSON: `[]`}); got != "" {
		t.Fatalf("empty target = %q", got)
	}
}
