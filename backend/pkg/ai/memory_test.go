package ai

import (
	"strings"
	"testing"
)

func TestBuildWorkbenchTargetRefs(t *testing.T) {
	refs := buildWorkbenchTargetRefs("https://example.com:8443/app/login", []string{
		"example.com",
		"http://api.example.com/v1",
		"example.com",
	})

	wantContains := []string{
		"https://example.com:8443/app/login",
		"example.com",
		"api.example.com",
	}
	for _, want := range wantContains {
		if !containsString(refs, want) {
			t.Fatalf("expected refs to contain %q, got %#v", want, refs)
		}
	}

	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if seen[ref] {
			t.Fatalf("expected refs to be deduplicated, got duplicate %q in %#v", ref, refs)
		}
		seen[ref] = true
	}
}

func TestWorkbenchTargetContextScoringCapsAtHundred(t *testing.T) {
	ctx := WorkbenchTargetContext{
		FingerprintCount:   30,
		VulnerabilityCount: 4,
		JSAssetCount:       20,
		PortCount:          15,
	}
	if got := ctx.RiskScore(); got != 100 {
		t.Fatalf("RiskScore() = %d, want 100", got)
	}
}

func TestWorkbenchVulnerabilityOrderUsesExistingColumns(t *testing.T) {
	order := workbenchVulnerabilityOrder()
	if strings.Contains(order, "created_at") {
		t.Fatalf("vulnerability_info has no created_at column, got order %q", order)
	}
	if !strings.Contains(order, "task_id") || !strings.Contains(order, "version") {
		t.Fatalf("expected stable vulnerability_info columns in order, got %q", order)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
