package ai

import "testing"

func TestBuildWorkbenchRuleInitialPlanCIDRDiscoversBeforeHTTP(t *testing.T) {
	plan := BuildWorkbenchRuleInitialPlan(
		"agent",
		"192.168.2.101/24",
		nil,
		WorkbenchRiskManualApproval,
		[]string{"external_attack_surface"},
	)

	toolIndex := indexWorkbenchPlanStep(plan, "tool_orchestration")
	httpIndex := indexWorkbenchPlanStep(plan, "http_observation")
	if toolIndex < 0 || httpIndex < 0 {
		t.Fatalf("expected tool_orchestration and http_observation in %#v", plan)
	}
	if toolIndex > httpIndex {
		t.Fatalf("CIDR plan should discover services before HTTP observation, got tool=%d http=%d plan=%#v", toolIndex, httpIndex, plan)
	}
	if plan[toolIndex].Title != "识别资产与开放服务" {
		t.Fatalf("unexpected discovery title %q", plan[toolIndex].Title)
	}
}

func TestBuildWorkbenchRuleInitialPlanWebURLObservesBeforeDiscovery(t *testing.T) {
	plan := BuildWorkbenchRuleInitialPlan(
		"agent",
		"https://example.com/login",
		nil,
		WorkbenchRiskManualApproval,
		[]string{"single_web_deep_test"},
	)

	httpIndex := indexWorkbenchPlanStep(plan, "http_observation")
	toolIndex := indexWorkbenchPlanStep(plan, "tool_orchestration")
	if httpIndex < 0 || toolIndex < 0 {
		t.Fatalf("expected http_observation and tool_orchestration in %#v", plan)
	}
	if httpIndex > toolIndex {
		t.Fatalf("Web URL plan should observe HTTP before extra discovery, got http=%d tool=%d plan=%#v", httpIndex, toolIndex, plan)
	}
}

func TestMergeWorkbenchTaskPlanStepsEnforcesDiscoveryBeforeHTTPForCIDR(t *testing.T) {
	profile := DetectWorkbenchTargetProfile("10.0.0.0/24", nil)
	rulePlan := BuildWorkbenchRuleInitialPlan("agent", "10.0.0.0/24", nil, WorkbenchRiskManualApproval, nil)
	modelPlan := []WorkbenchTaskPlanStep{
		{StepType: "scope_review", Title: "授权", RiskLevel: "low"},
		{StepType: "http_observation", Title: "错误前置 HTTP", RiskLevel: "low"},
		{StepType: "tool_orchestration", Title: "服务发现", RiskLevel: "medium"},
		{StepType: "goal_evaluation", Title: "评估", RiskLevel: "low"},
	}

	merged, applied := MergeWorkbenchTaskPlanSteps(profile, modelPlan, rulePlan)
	if !applied {
		t.Fatal("expected model plan to be applied with guardrails")
	}
	toolIndex := indexWorkbenchPlanStep(merged, "tool_orchestration")
	httpIndex := indexWorkbenchPlanStep(merged, "http_observation")
	if toolIndex > httpIndex {
		t.Fatalf("merge should force discovery before HTTP for CIDR, got tool=%d http=%d plan=%#v", toolIndex, httpIndex, merged)
	}
	if merged[len(merged)-1].StepType != "goal_evaluation" {
		t.Fatalf("goal_evaluation should remain last, got %#v", merged)
	}
}

func TestWorkbenchWebURLsFromPortResults(t *testing.T) {
	got := append(
		WorkbenchWebURLsFromPortResults([]int{80}, "192.168.2.10", "http", "nginx"),
		WorkbenchWebURLsFromPortResults([]int{8443}, "192.168.2.10", "https", "nginx")...,
	)
	got = append(got, WorkbenchWebURLsFromPortResults([]int{22}, "192.168.2.10", "ssh", "openssh")...)
	want := []string{"http://192.168.2.10", "https://192.168.2.10:8443"}
	if len(got) != len(want) {
		t.Fatalf("urls = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("urls = %#v, want %#v", got, want)
		}
	}
}

func indexWorkbenchPlanStep(plan []WorkbenchTaskPlanStep, stepType string) int {
	for index, step := range plan {
		if step.StepType == stepType {
			return index
		}
	}
	return -1
}
