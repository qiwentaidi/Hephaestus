package ai

import (
	"strings"
	"testing"
)

func TestOrchestrationConversationTitleUsesObjective(t *testing.T) {
	objective := "请对下面URL目标进行渗透测试漏洞挖掘"
	for _, role := range []string{"controller", "planner"} {
		got := orchestrationConversationTitle(role, objective, "接码平台")
		if !strings.Contains(got, objective) {
			t.Fatalf("%s title %q lost the run objective", role, got)
		}
		if strings.Contains(got, "接码平台") {
			t.Fatalf("%s title %q fell back to the project name", role, got)
		}
	}
	if got := orchestrationConversationTitle("controller", "  ", "接码平台"); got != "主控 · 接码平台" {
		t.Fatalf("empty objective fallback = %q", got)
	}
	if got := orchestrationConversationTitle("planner", "第一行\n第二行", ""); got != "Planner · 第一行 第二行" {
		t.Fatalf("objective should be compacted into one line, got %q", got)
	}
}

func TestOrchestrationConversationTitleTruncatesLongObjective(t *testing.T) {
	got := orchestrationConversationTitle("controller", strings.Repeat("目标", 100), "")
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("long objective should be truncated, got %q", got)
	}
}
